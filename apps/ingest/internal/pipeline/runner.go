package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/checkpoint"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

const hardMaxPages = 100

// Source fetches HH search pages and vacancy details.
type Source interface {
	SearchVacancies(ctx context.Context, q hh.SearchQuery) (hh.SearchPage, error)
	GetVacancyRaw(ctx context.Context, id string) ([]byte, error)
}

// Params configures one ingest run.
type Params struct {
	Area             string
	Text             string
	ProfessionalRole string
	DateFrom         time.Time
	DateTo           time.Time
	Mode             string
	MaxPages         int
	PerPage          int
	RequestedBy      string
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
	RunID     string
	Status    string
	Stats     store.Stats
	Completed bool
}

// ResolvePageLimit returns the bounded number of pages this run may request.
// requested=0 means all pages available for this query.
func ResolvePageLimit(requested, perPage int) int {
	if perPage <= 0 || perPage > hh.MaxPerPage {
		perPage = hh.MaxPerPage
	}
	depthPages := (hh.MaxSearchResults + perPage - 1) / perPage
	limit := min(depthPages, hardMaxPages)
	if requested > 0 {
		limit = min(limit, requested)
	}
	return limit
}

// Run executes an incremental/full HH ingest until the configured guard,
// HH's search-depth cap, or the API-reported terminal page.
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
	perPage := p.PerPage
	if perPage <= 0 || perPage > hh.MaxPerPage {
		perPage = hh.MaxPerPage
	}
	pageLimit := ResolvePageLimit(p.MaxPages, perPage)
	opts := r.Opts
	if opts.Roles == nil && opts.Skills == nil && opts.Regions == nil && opts.FX == nil {
		opts = normalize.DefaultOptions()
	}

	runID := newRunID()
	started := nowFn().UTC()
	scope := checkpoint.ScopeHash(
		hh.SourceCode, mode, p.Area, p.Text, perPage,
		p.ProfessionalRole, formatScopeTime(p.DateFrom), formatScopeTime(p.DateTo),
	)

	if err := r.Store.CreateRun(ctx, store.Run{
		ID:     runID,
		Source: hh.SourceCode,
		Mode:   mode,
		Status: store.StatusRunning,
		Params: map[string]any{
			"area": p.Area, "text": p.Text, "professional_role": p.ProfessionalRole,
			"date_from": formatScopeTime(p.DateFrom), "date_to": formatScopeTime(p.DateTo),
			"max_pages": p.MaxPages, "per_page": perPage,
		},
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
		"professional_role", p.ProfessionalRole,
		"max_pages", p.MaxPages,
		"per_page", perPage,
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
	completed := false

	for pagesDone := 0; pagesDone < pageLimit && pageIdx < ResolvePageLimit(0, perPage); pagesDone++ {
		search, err := r.Source.SearchVacancies(ctx, hh.SearchQuery{
			Text:             p.Text,
			Area:             p.Area,
			ProfessionalRole: p.ProfessionalRole,
			DateFrom:         p.DateFrom,
			DateTo:           p.DateTo,
			Page:             pageIdx,
			PerPage:          perPage,
		})
		if err != nil {
			stats.Errors++
			_ = r.Store.RecordError(ctx, runID, "", "fetch", err.Error())
			status = store.StatusFailed
			fatal = err
			break
		}
		stats.Pages++
		apiPages := min(search.Pages, ResolvePageLimit(0, perPage))
		log.Info("ingest_search_page_fetched",
			"ingest_run_id", runID,
			"source", hh.SourceCode,
			"page", search.Page,
			"reported_pages", search.Pages,
			"bounded_pages", apiPages,
			"reported_found", search.Found,
		)

		writes := make([]store.VacancyWrite, 0, len(search.Items))
		pageOK := true
		for _, item := range search.Items {
			stats.Fetched++
			raw, err := r.Source.GetVacancyRaw(ctx, item.ID)
			if err != nil {
				stats.Errors++
				disappeared := errors.Is(err, hh.ErrNotFound)
				if !disappeared {
					pageOK = false
				}
				_ = r.Store.RecordError(ctx, runID, item.ID, "fetch", err.Error())
				log.Warn("ingest_vacancy_fetch_failed",
					"ingest_run_id", runID,
					"source", hh.SourceCode,
					"disappeared", disappeared,
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
			scopeRoles := hh.CollectedProfessionalRoles(draft.ProfessionalRoleIDs)
			if len(scopeRoles) == 0 {
				stats.Excluded++
				continue
			}
			// Keep normalization deterministic for multi-role vacancies and do
			// not allow a secondary out-of-scope alias to become canonical.
			primaryRole, listingScope := hh.AllowedProfessionalRole(draft.ProfessionalRoleIDs)
			if !listingScope {
				primaryRole = scopeRoles[0]
			}
			draft.ProfessionalRoleIDs = []string{primaryRole.ID}
			res, err := normalize.Normalize(draft, opts)
			if err != nil {
				stats.Errors++
				pageOK = false
				_ = r.Store.RecordError(ctx, runID, item.ID, "normalize", err.Error())
				continue
			}
			writes = append(writes, store.VacancyWrite{
				Vacancy:      res.Vacancy,
				RegionName:   draft.RegionName,
				RawPayload:   draft.RawPayload,
				ScopeRoleIDs: roleExternalIDs(scopeRoles),
			})
		}

		dec := checkpoint.Decide(checkpoint.PageOutcome{
			AllOK:       pageOK,
			CurrentPage: pageIdx,
			TotalPages:  apiPages,
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
			completed = true
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
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancelFinish()
	if err := r.Store.FinishRun(finishCtx, runID, status, stats, errMsg); err != nil {
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
		"excluded_out_of_scope", stats.Excluded,
	)
	out := Result{RunID: runID, Status: status, Stats: stats, Completed: completed}
	if fatal != nil {
		return out, fatal
	}
	return out, nil
}

func roleExternalIDs(roles []hh.AllowedRole) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		result = append(result, role.ID)
	}
	return result
}

func formatScopeTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Truncate(time.Second).Format(time.RFC3339)
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
