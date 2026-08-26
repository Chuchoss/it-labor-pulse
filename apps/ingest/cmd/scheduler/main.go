package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	analytics "github.com/Chuchoss/it-labor-pulse/apps/analytics/worker"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/config"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/pipeline"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/scheduler"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	if err := cfg.ValidateScheduler(); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	log := logging.New(logging.Options{
		Service: "scheduler",
		Env:     cfg.AppEnv,
		Level:   cfg.LogLevel,
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database_open_failed", "error_category", "database")
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	analyticsWorker, err := analytics.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("analytics_database_open_failed", "error_category", "database")
		os.Exit(1)
	}
	defer analyticsWorker.Close()

	engine := &scheduler.Engine{
		Config: scheduler.Config{
			Interval:        cfg.Scheduler.Interval,
			RunOnStart:      cfg.Scheduler.RunOnStart,
			BackoffInitial:  cfg.Scheduler.BackoffInitial,
			BackoffMax:      cfg.Scheduler.BackoffMax,
			JitterPercent:   cfg.Scheduler.JitterPercent,
			ShutdownTimeout: cfg.Scheduler.ShutdownTimeout,
		},
		Log: log,
		Batch: scheduler.WithLock(func(parent context.Context) (scheduler.ReleaseLock, bool, error) {
			lock, acquired, err := st.TrySchedulerLock(parent)
			if err != nil {
				return nil, false, err
			}
			if !acquired {
				return nil, false, nil
			}
			return lock.Release, true, nil
		}, func(parent context.Context, schedulerRunID string) (scheduler.BatchResult, error) {

			runCtx, cancel := context.WithTimeout(parent, cfg.RunTimeout)
			defer cancel()
			client, err := hh.NewClient(hh.ClientOptions{
				BaseURL: cfg.HHBaseURL, UserAgent: cfg.HHUserAgent,
				AppToken: cfg.HHAppToken, PageDelay: cfg.PageDelay,
				MaxRequests: cfg.ITMaxReqs,
			})
			var result pipeline.ITBatchResult
			if err == nil {
				var rates normalize.RateTable
				rates, err = st.LoadFXRates(
					runCtx,
					time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Now().UTC(),
				)
				normalizeOpts := normalize.DefaultOptions()
				normalizeOpts.FX = rates
				if err != nil {
					return scheduler.BatchResult{}, err
				}
				result, err = pipeline.RunITBatch(runCtx, client, st, log, pipeline.ITBatchOptions{
					Area: cfg.ITArea, PerPage: cfg.PerPage,
					MaxDepth: cfg.ITMaxDepth, MaxPartitions: cfg.ITMaxParts,
					MaxBatchParts: cfg.Scheduler.MaxPartitions, MaxPagesPerPart: cfg.MaxPages,
					MaxRequests: cfg.ITMaxReqs, RequestedBy: schedulerRunID,
					Normalize: normalizeOpts,
				})
			}
			if err == nil && result.CycleComplete {
				snapshotCtx, cancelSnapshot := context.WithTimeout(parent, 2*time.Minute)
				snapshot, snapshotErr := analyticsWorker.RunDaily(snapshotCtx, result.CycleID)
				cancelSnapshot()
				if snapshotErr != nil {
					log.Error("analytics_cycle_trigger_failed",
						"source_cycle_id", result.CycleID,
						"error_category", "database",
					)
				} else {
					log.Info("analytics_cycle_trigger_finished",
						"source_cycle_id", result.CycleID,
						"analytics_run_id", snapshot.RunID,
						"rows", snapshot.Rows,
						"method_version", analytics.MethodVersion,
					)
				}
			}
			return scheduler.BatchResult{
				IngestRunID:   result.LastRunID,
				CycleComplete: result.CycleComplete,
				Stats: scheduler.Stats{
					Fetched: result.Stats.Fetched, Upserted: result.Stats.Upserted,
					Unchanged: result.Stats.Unchanged, Excluded: result.Stats.Excluded,
					Errors: result.Stats.Errors, Pages: result.Stats.Pages,
				},
			}, err
		}),
	}

	log.Info("scheduler_started",
		"interval_ms", cfg.Scheduler.Interval.Milliseconds(),
		"run_on_start", cfg.Scheduler.RunOnStart,
		"next_run_at", time.Now().Add(cfg.Scheduler.Interval).UTC().Format(time.RFC3339),
		"request_ceiling", cfg.ITMaxReqs,
		"partition_ceiling", cfg.Scheduler.MaxPartitions,
		"shutdown_timeout_ms", cfg.Scheduler.ShutdownTimeout.Milliseconds(),
		"advisory_lock_key", store.SchedulerAdvisoryLockKey,
	)
	if err := engine.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("scheduler_stopped", "error_category", "shutdown")
		os.Exit(1)
	}
	log.Info("scheduler_stopped")
}
