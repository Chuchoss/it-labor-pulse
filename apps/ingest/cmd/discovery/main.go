package main

import (
	"context"
	"errors"
	"flag"
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
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

func main() {
	once := flag.Bool("once", false, "run or resume one daily discovery cycle")
	flag.Parse()
	_ = godotenv.Load()
	cfg := config.Load()
	if err := cfg.ValidateDiscovery(); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	log := logging.New(logging.Options{
		Service: "ingest-discovery", Env: cfg.AppEnv, Level: cfg.LogLevel,
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

	run := func(parent context.Context) error {
		lock, acquired, err := st.TryDiscoveryLock(parent)
		if err != nil {
			return err
		}
		if !acquired {
			log.Info("discovery_trigger_skipped", "reason", "lock")
			return nil
		}
		defer func() { _ = lock.Release(parent) }()

		now := time.Now().UTC()
		cutoff := dayUTC(now)
		cycleDate := cutoff.AddDate(0, 0, -1)
		if pending, found, pendingErr := st.PendingDiscoveryCycle(
			parent, hh.SourceCode, pipeline.DiscoveryMethodVersion,
		); pendingErr != nil {
			return pendingErr
		} else if found {
			cycleDate, cutoff = pending.CycleDate, pending.CutoffAt
			log.Info("discovery_cycle_resuming",
				"cycle_id", pending.ID,
				"cycle_date", cycleDate.Format(time.DateOnly),
				"completed_pages", pending.CompletedPages,
				"expected_pages", pending.ExpectedPages,
			)
		}
		runCtx, cancel := context.WithTimeout(parent, cfg.Discovery.RunTimeout)
		defer cancel()
		client, err := hh.NewClient(hh.ClientOptions{
			BaseURL: cfg.HHBaseURL, UserAgent: cfg.HHUserAgent,
			AppToken: cfg.HHAppToken, PageDelay: cfg.PageDelay,
			MaxRequests: cfg.Discovery.MaxRequests,
		})
		if err != nil {
			return err
		}
		rates, err := st.LoadFXRates(
			runCtx,
			time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			now,
		)
		if err != nil {
			return err
		}
		normalizeOpts := normalize.DefaultOptions()
		normalizeOpts.FX = rates
		result, err := pipeline.RunDailyDiscovery(runCtx, client, st, log, pipeline.DiscoveryOptions{
			Area: cfg.ITArea, PerPage: cfg.PerPage,
			MaxDepth: cfg.ITMaxDepth, MaxPartitions: cfg.ITMaxParts,
			MaxRequests: cfg.Discovery.MaxRequests,
			CycleDate:   cycleDate, CutoffAt: cutoff, ObservedAt: now,
			Normalize: normalizeOpts,
		})
		if err != nil {
			return err
		}
		if !result.Complete {
			return fmt.Errorf("discovery cycle did not complete")
		}
		snapshotCtx, cancelSnapshot := context.WithTimeout(parent, 2*time.Minute)
		defer cancelSnapshot()
		snapshot, err := analyticsWorker.RunDaily(snapshotCtx, result.CycleID)
		if err != nil {
			return fmt.Errorf("trigger discovery snapshot: %w", err)
		}
		log.Info("discovery_snapshot_finished",
			"cycle_id", result.CycleID,
			"cycle_date", result.CycleDate.Format(time.DateOnly),
			"analytics_run_id", snapshot.RunID,
			"rows", snapshot.Rows,
			"method_version", analytics.MethodVersion,
		)
		deleted, cleanupErr := st.CleanupDiscoveryObservations(
			parent, result.CycleDate.AddDate(0, 0, -35),
		)
		if cleanupErr != nil {
			log.Warn("discovery_retention_cleanup_failed",
				"error_category", "database",
				"error", cleanupErr.Error(),
			)
		} else {
			log.Info("discovery_retention_cleanup_finished",
				"deleted_observations", deleted,
				"retention_days", 35,
			)
		}
		return nil
	}

	log.Info("discovery_scheduler_started",
		"mode", map[bool]string{true: "once", false: "daily"}[*once],
		"utc_hour", cfg.Discovery.UTCHour,
		"request_ceiling", cfg.Discovery.MaxRequests,
		"run_timeout_ms", cfg.Discovery.RunTimeout.Milliseconds(),
		"advisory_lock_key", store.DiscoveryAdvisoryLockKey,
	)
	if *once {
		if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("discovery_run_failed",
				"error_category", "dependency",
				"error", err.Error(),
			)
			os.Exit(1)
		}
		return
	}

	runNow := cfg.Discovery.RunOnStart
	for {
		if runNow {
			err := run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Error("discovery_run_failed",
					"error_category", "dependency",
					"error", err.Error(),
				)
				if !wait(ctx, cfg.Discovery.RetryInterval) {
					return
				}
				runNow = true
				continue
			}
		}
		runNow = true
		next := nextDaily(time.Now().UTC(), cfg.Discovery.UTCHour)
		log.Info("discovery_next_run_scheduled", "next_run_at", next.Format(time.RFC3339))
		if !wait(ctx, time.Until(next)) {
			return
		}
	}
}

func dayUTC(value time.Time) time.Time {
	y, m, d := value.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func nextDaily(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
