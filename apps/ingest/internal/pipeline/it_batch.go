package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/checkpoint"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

const itCycleCursorVersion = 1

// ITBatchOptions bounds one resumable all-IT scheduler batch.
type ITBatchOptions struct {
	Area            string
	PerPage         int
	MaxDepth        int
	MaxPartitions   int
	MaxBatchParts   int
	MaxPagesPerPart int
	MaxRequests     int
	Now             time.Time
	RequestedBy     string
}

// ITBatchResult summarizes one bounded all-IT batch.
type ITBatchResult struct {
	LastRunID      string
	CycleID        string
	Stats          store.Stats
	CycleComplete  bool
	CompletedParts int
	RemainingParts int
}

// ITBatchSource is the HH surface needed by planning and execution.
type ITBatchSource interface {
	Source
	RoleCatalogSource
}

type itCycleCursor struct {
	Version    int               `json:"version"`
	CycleID    string            `json:"cycle_id"`
	CycleEnd   time.Time         `json:"cycle_end"`
	Next       int               `json:"next_partition"`
	Complete   bool              `json:"complete"`
	Partitions []SearchPartition `json:"partitions"`
}

// RunITBatch continues a persisted partition plan without exceeding MaxRequests.
func RunITBatch(
	ctx context.Context,
	client ITBatchSource,
	st store.Store,
	log *slog.Logger,
	opts ITBatchOptions,
) (ITBatchResult, error) {
	if client == nil || st == nil {
		return ITBatchResult{}, fmt.Errorf("it batch: client and store are required")
	}
	if log == nil {
		log = slog.Default()
	}
	if opts.Area == "" {
		opts.Area = "113"
	}
	if opts.PerPage < 1 || opts.PerPage > hh.MaxPerPage {
		return ITBatchResult{}, fmt.Errorf("it batch: per page must be between 1 and %d", hh.MaxPerPage)
	}
	if opts.MaxRequests < 2 {
		return ITBatchResult{}, fmt.Errorf("it batch: max requests must be at least 2")
	}
	if opts.MaxBatchParts < 1 {
		return ITBatchResult{}, fmt.Errorf("it batch: max batch partitions must be positive")
	}
	if opts.MaxPagesPerPart < 1 {
		return ITBatchResult{}, fmt.Errorf("it batch: max pages per partition must be positive")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	opts.Now = opts.Now.UTC().Truncate(time.Second)
	if opts.RequestedBy == "" {
		opts.RequestedBy = "scheduler"
	}

	cycleScope := checkpoint.ScopeHash(hh.SourceCode, "it-cycle-v1", opts.Area, "", opts.PerPage)
	cursorRaw, ok, err := st.GetCheckpoint(ctx, hh.SourceCode, cycleScope)
	if err != nil {
		return ITBatchResult{}, fmt.Errorf("it batch checkpoint read: %w", err)
	}
	var cycle itCycleCursor
	requestsRemaining := opts.MaxRequests
	if ok && cursorRaw != "" {
		if err := json.Unmarshal([]byte(cursorRaw), &cycle); err != nil ||
			cycle.Version != itCycleCursorVersion ||
			cycle.Next < 0 || cycle.Next > len(cycle.Partitions) {
			return ITBatchResult{}, fmt.Errorf("it batch checkpoint is invalid")
		}
	}
	if !ok || cursorRaw == "" || cycle.Complete {
		plan, err := PlanIT(ctx, client, ITPlanOptions{
			Area:          opts.Area,
			Now:           opts.Now,
			MaxDepth:      opts.MaxDepth,
			MaxPartitions: opts.MaxPartitions,
			MaxRequests:   opts.MaxRequests,
		})
		if err != nil {
			return ITBatchResult{}, fmt.Errorf("it batch plan: %w", err)
		}
		cycle = itCycleCursor{
			Version: itCycleCursorVersion, CycleEnd: opts.Now,
			Partitions: plan.Partitions,
		}
		cycle.CycleID, err = st.StartCycle(ctx, store.Cycle{
			Source: hh.SourceCode, Scope: "all_it", ScopeHash: cycleScope,
			CycleEnd: cycle.CycleEnd, PartitionCount: len(cycle.Partitions),
		})
		if err != nil {
			return ITBatchResult{}, fmt.Errorf("it batch cycle start: %w", err)
		}
		requestsRemaining -= plan.ProbeRequests
		if err := saveITCycle(ctx, st, cycleScope, cycle); err != nil {
			return ITBatchResult{}, err
		}
		log.Info("it_cycle_started",
			"cycle_end", cycle.CycleEnd.Format(time.RFC3339),
			"partitions", len(cycle.Partitions),
			"estimated_results", plan.EstimatedResults,
			"probe_requests", plan.ProbeRequests,
		)
	}
	if cycle.CycleID == "" {
		cycle.CycleID, err = st.StartCycle(ctx, store.Cycle{
			Source: hh.SourceCode, Scope: "all_it", ScopeHash: cycleScope,
			CycleEnd: cycle.CycleEnd, PartitionCount: len(cycle.Partitions),
			CompletedPartitions: cycle.Next,
		})
		if err != nil {
			return ITBatchResult{}, fmt.Errorf("it batch cycle resume: %w", err)
		}
		if err := saveITCycle(ctx, st, cycleScope, cycle); err != nil {
			return ITBatchResult{}, err
		}
	}

	allowedRoles := hh.CollectedRoles()
	sourceRoles := make([]store.SourceRole, 0, len(allowedRoles))
	for _, role := range allowedRoles {
		sourceRoles = append(sourceRoles, store.SourceRole{
			ExternalID: role.ID, Title: role.ExpectedName, Family: role.Group, Scopes: role.Scopes,
		})
	}
	roleIDs, err := st.SyncRoles(ctx, hh.SourceCode, sourceRoles)
	if err != nil {
		return ITBatchResult{}, fmt.Errorf("it batch role sync: %w", err)
	}
	normalizeOpts := normalize.DefaultOptions()
	normalizeOpts.Roles = normalize.MapRoleMatcher{
		RoleByExternalID: map[string]map[string]string{hh.SourceCode: roleIDs},
	}

	result := ITBatchResult{CycleID: cycle.CycleID}
	for cycle.Next < len(cycle.Partitions) {
		if result.CompletedParts >= opts.MaxBatchParts {
			break
		}
		part := cycle.Partitions[cycle.Next]
		pages := max(1, (part.Found+opts.PerPage-1)/opts.PerPage)
		affordablePages := requestsRemaining / (opts.PerPage + 1)
		if part.Found == 0 && requestsRemaining > 0 {
			affordablePages = 1
		}
		if affordablePages < 1 {
			if result.Stats.Pages == 0 {
				return result, fmt.Errorf("it batch: request budget cannot fund one page")
			}
			break
		}
		maxPages := min(min(pages, affordablePages), opts.MaxPagesPerPart)
		runner := &Runner{Source: client, Store: st, Log: log, Opts: normalizeOpts}
		run, runErr := runner.Run(ctx, Params{
			Area: part.Area, ProfessionalRole: part.RoleID,
			DateFrom: part.From, DateTo: part.To,
			Mode: "incremental", MaxPages: maxPages, PerPage: opts.PerPage,
			RequestedBy: opts.RequestedBy,
		})
		result.LastRunID = run.RunID
		addStats(&result.Stats, run.Stats)
		requestsRemaining -= run.Stats.Fetched + run.Stats.Pages
		if runErr != nil {
			return result, runErr
		}
		if !run.Completed {
			if run.Status == store.StatusSuccess {
				break
			}
			return result, fmt.Errorf("it batch: partition incomplete (status=%s)", run.Status)
		}
		cycle.Next++
		result.CompletedParts++
		if err := saveITCycle(ctx, st, cycleScope, cycle); err != nil {
			return result, err
		}
		if err := st.UpdateCycleProgress(ctx, cycle.CycleID, cycle.Next); err != nil {
			return result, fmt.Errorf("it batch cycle progress: %w", err)
		}
	}

	if cycle.Next == len(cycle.Partitions) {
		if err := st.CompleteCycle(ctx, cycle.CycleID, cycle.Next); err != nil {
			return result, fmt.Errorf("it batch cycle complete: %w", err)
		}
		cycle.Complete = true
		if err := saveITCycle(ctx, st, cycleScope, cycle); err != nil {
			return result, err
		}
		result.CycleComplete = true
		log.Info("it_cycle_completed",
			"cycle_id", cycle.CycleID,
			"cycle_end", cycle.CycleEnd.Format(time.RFC3339),
			"partitions", len(cycle.Partitions),
		)
	}
	result.RemainingParts = len(cycle.Partitions) - cycle.Next
	log.Info("it_batch_finished",
		"ingest_run_id", result.LastRunID,
		"completed_partitions", result.CompletedParts,
		"remaining_partitions", result.RemainingParts,
		"cycle_complete", result.CycleComplete,
		"fetched", result.Stats.Fetched,
		"upserted", result.Stats.Upserted,
		"unchanged", result.Stats.Unchanged,
		"excluded_out_of_scope", result.Stats.Excluded,
		"errors", result.Stats.Errors,
	)
	return result, nil
}

func saveITCycle(ctx context.Context, st store.Store, scope string, cycle itCycleCursor) error {
	raw, err := json.Marshal(cycle)
	if err != nil {
		return fmt.Errorf("it batch checkpoint encode: %w", err)
	}
	if _, _, err := st.SavePage(ctx, hh.SourceCode, scope, string(raw), nil); err != nil {
		return fmt.Errorf("it batch checkpoint save: %w", err)
	}
	return nil
}

func addStats(dst *store.Stats, src store.Stats) {
	dst.Fetched += src.Fetched
	dst.Upserted += src.Upserted
	dst.Unchanged += src.Unchanged
	dst.Excluded += src.Excluded
	dst.Errors += src.Errors
	dst.Pages += src.Pages
}
