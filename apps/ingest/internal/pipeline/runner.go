package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/checkpoint"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

// Source fetches HH search pages and vacancy details.
type Source interface {
	SearchVacancies(ctx context.Context, q hh.SearchQuery) (hh.SearchPage, error)
	GetVacancyRaw(ctx context.Context, id string) ([]byte, error)
}

// Params configures one ingest run.
type Params struct {
	Area        string
	Text        string
	Mode        string
	MaxPages    int
	PerPage     int
	RequestedBy string
}

// Runner orchestrates HH → draft → normalize → UPSERT with checkpoints.
type Runner struct {
	Source Source
	Store  store.Store
	Log    *slog.Logger
	Now    func() time.Time
	Opts   normalize.Options
}

// Result is the outcome of Run.
type Result struct {
	RunID  string
	Status string
	Stats  store.Stats
}

// Run executes an incremental/full HH ingest until max pages or terminal page.
func (r *Runner) Run(ctx context.Context, p Params) (Result, error) {
	if r.Source == nil || r.Store == nil {
		return Result{}, fmt.Errorf("pipeline: source and store are required")
	}
	log := r.Log
	if log == nil {
		log = slog.Default()
	}
	nowFn := r.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	mode := p.Mode
	if mode == "" {
		mode = "incremental"
	}
	maxPages := p.MaxPages
	if maxPages <= 0 {
		maxPages = 5
	}
	opts := r.Opts
	if opts.Roles == nil && opts.Skills == nil && opts.Regions == nil && opts.FX == nil {
		opts = normalize.DefaultOptions()
	}

	runID := newRunID()
	started := nowFn().UTC()
	scope := checkpoint.ScopeHash(hh.SourceCode, mode, p.Area, p.Text)

	if err := r.Store.CreateRun(ctx, store.Run{
		ID:          runID,
		Source:      hh.SourceCode,
		Mode:        mode,
		Status:      store.StatusRunning,
		Params:      map[string]any{"area": p.Area, "text": p.Text, "max_pages": maxPages},
		RequestedBy: p.RequestedBy,
		StartedAt:   &started,
	}); err != nil {
		return Result{}, fmt.Errorf("pipeline create run: %w", err)
	}

	log.Info("ingest_run_started",
		"ingest_run_id", runID,
		"source", hh.SourceCode,
		"mode", mode,
		"area", p.Area,
		"text", p.Text,
	)

	stats := store.Stats{}
	cursor, _, err := r.Store.GetCheckpoint(ctx, hh.SourceCode, scope)
	if err != nil {
		_ = r.Store.FinishRun(ctx, runID, store.StatusFailed, stats, "checkpoint read failed")
		return Result{RunID: runID, Status: store.StatusFailed, Stats: stats}, fmt.Errorf("pipeline checkpoint: %w", err)
	}
	pageIdx, err := checkpoint.ParseCursorPage(cursor)
	if err != nil {
		_ = r.Store.FinishRun(ctx, runID, store.StatusFailed, stats, "bad checkpoint cursor")
		return Result{RunID: runID, Status: store.StatusFailed, Stats: stats}, err
	}

	status := store.StatusSuccess
	var fatal error

	for pagesDone := 0; pagesDone < maxPages; pagesDone++ {
		search, err := r.Source.SearchVacancies(ctx, hh.SearchQuery{
			Text:    p.Text,
			Area:    p.Area,
			Page:    pageIdx,
			PerPage: p.PerPage,
		})
		if err != nil {
			stats.Errors++
			_ = r.Store.RecordError(ctx, runID, "", "fetch", err.Error())
			status = store.StatusFailed
			fatal = err
			break
		}
		stats.Pages++

		writes := make([]store.VacancyWrite, 0, len(search.Items))
		pageOK := true
		for _, item := range search.Items {
			stats.Fetched++
			raw, err := r.Source.GetVacancyRaw(ctx, item.ID)
			if err != nil {
				stats.Errors++
				pageOK = false
				_ = r.Store.RecordError(ctx, runID, item.ID, "fetch", err.Error())
				log.Warn("ingest_vacancy_fetch_failed",
					"ingest_run_id", runID,
					"source", hh.SourceCode,
					"vacancy_external_id", item.ID,
					"err", err.Error(),
				)
				continue
			}
			draft, err := hh.DraftFromDetail(raw, nowFn().UTC())
			if err != nil {
				stats.Errors++
				pageOK = false
				_ = r.Store.RecordError(ctx, runID, item.ID, "adapt", err.Error())
				continue
			}
			res, err := normalize.Normalize(draft, opts)
			if err != nil {
				stats.Errors++
				pageOK = false
				_ = r.Store.RecordError(ctx, runID, item.ID, "normalize", err.Error())
				continue
			}
			writes = append(writes, store.VacancyWrite{
				Vacancy:    res.Vacancy,
				RegionName: draft.RegionName,
				RawPayload: draft.RawPayload,
			})
		}

		dec := checkpoint.Decide(checkpoint.PageOutcome{
			AllOK:       pageOK,
			CurrentPage: pageIdx,
			TotalPages:  search.Pages,
			ItemCount:   len(search.Items),
		})
		if !dec.Advance {
			status = store.StatusPartial
			if stats.Errors > 0 && len(writes) == 0 {
				status = store.StatusFailed
			}
			log.Warn("ingest_page_not_committed",
				"ingest_run_id", runID,
				"source", hh.SourceCode,
				"page", pageIdx,
				"errors", stats.Errors,
			)
			break
		}

		up, un, err := r.Store.SavePage(ctx, hh.SourceCode, scope, dec.NextCursor, writes)
		if err != nil {
			stats.Errors++
			_ = r.Store.RecordError(ctx, runID, "", "upsert", err.Error())
			status = store.StatusFailed
			fatal = err
			break
		}
		stats.Upserted += up
		stats.Unchanged += un
		pageIdx, _ = checkpoint.ParseCursorPage(dec.NextCursor)

		log.Info("ingest_page_committed",
			"ingest_run_id", runID,
			"source", hh.SourceCode,
			"page", pageIdx-1,
			"upserted", up,
			"unchanged", un,
		)

		if dec.Terminal {
			break
		}
	}

	if fatal != nil && status == store.StatusSuccess {
		status = store.StatusFailed
	}
	if stats.Errors > 0 && status == store.StatusSuccess {
		status = store.StatusPartial
	}

	errMsg := ""
	if fatal != nil {
		errMsg = truncate(fatal.Error(), 500)
	}
	if err := r.Store.FinishRun(ctx, runID, status, stats, errMsg); err != nil {
		if fatal != nil {
			return Result{RunID: runID, Status: status, Stats: stats},
				fmt.Errorf("pipeline operation failed (%w); finish run: %w", fatal, err)
		}
		return Result{RunID: runID, Status: status, Stats: stats}, fmt.Errorf("pipeline finish run: %w", err)
	}
	log.Info("ingest_run_finished",
		"ingest_run_id", runID,
		"source", hh.SourceCode,
		"status", status,
		"fetched", stats.Fetched,
		"upserted", stats.Upserted,
		"errors", stats.Errors,
	)
	out := Result{RunID: runID, Status: status, Stats: stats}
	if fatal != nil {
		return out, fatal
	}
	return out, nil
}

func newRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("ing_%d_%s", time.Now().UTC().Unix(), hex.EncodeToString(b[:]))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
