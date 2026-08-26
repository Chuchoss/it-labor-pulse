package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	analytics "github.com/Chuchoss/it-labor-pulse/apps/analytics/worker"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
)

func main() {
	_ = godotenv.Load()
	mode := flag.String("mode", "daily", "daily, weekly, backfill or scheduler")
	cycleID := flag.String("cycle-id", "", "completed ingest cycle UUID for daily mode")
	date := flag.String("date", "", "target date YYYY-MM-DD for weekly mode")
	flag.Parse()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	log := logging.New(logging.Options{
		Service: "analytics",
		Env:     envOr("APP_ENV", "local"),
		Level:   envOr("LOG_LEVEL", "info"),
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	w, err := analytics.Open(ctx, databaseURL)
	if err != nil {
		log.Error("analytics_database_open_failed", "error_category", "database")
		os.Exit(1)
	}
	defer w.Close()

	switch *mode {
	case "daily":
		err = runDaily(ctx, w, log, *cycleID)
	case "weekly":
		target, parseErr := parseWeek(*date)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", parseErr)
			os.Exit(2)
		}
		err = runWeekly(ctx, w, log, target)
	case "backfill":
		var results []analytics.Result
		results, err = w.Backfill(ctx)
		if err == nil {
			log.Info("analytics_backfill_finished", "runs", len(results))
		}
	case "scheduler":
		err = runScheduler(ctx, w, log)
	default:
		fmt.Fprintln(os.Stderr, "config: mode must be daily, weekly, backfill or scheduler")
		os.Exit(2)
	}
	if errors.Is(err, analytics.ErrLockUnavailable) {
		log.Info("analytics_run_skipped", "reason", "lock")
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("analytics_run_failed", "error_category", "database")
		os.Exit(1)
	}
}

type infoLogger interface {
	Info(string, ...any)
}

func runDaily(ctx context.Context, w *analytics.Worker, log infoLogger, cycleID string) error {
	timeout := durationOr("ANALYTICS_RUN_TIMEOUT", 2*time.Minute)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := w.RunDaily(runCtx, cycleID)
	if err != nil {
		return err
	}
	logResult(log, result)
	return nil
}

func runWeekly(ctx context.Context, w *analytics.Worker, log infoLogger, week time.Time) error {
	timeout := durationOr("ANALYTICS_RUN_TIMEOUT", 2*time.Minute)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := w.RunWeekly(runCtx, week, "hh")
	if err != nil {
		return err
	}
	logResult(log, result)
	return nil
}

func runScheduler(ctx context.Context, w *analytics.Worker, log infoLogger) error {
	interval := durationOr("ANALYTICS_SCHEDULER_INTERVAL", 30*time.Minute)
	if interval < 10*time.Minute && !boolOr("ANALYTICS_SCHEDULER_TEST_MODE", false) {
		return fmt.Errorf("analytics scheduler interval must be at least 10m")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	run := func() {
		if err := runDaily(ctx, w, log, ""); err != nil && !errors.Is(err, analytics.ErrLockUnavailable) {
			log.Info("analytics_scheduler_tick_failed", "error_category", "database")
		}
		if err := runWeekly(ctx, w, log, analytics.MondayUTC(time.Now())); err != nil &&
			!errors.Is(err, analytics.ErrLockUnavailable) {
			log.Info("analytics_scheduler_tick_failed", "error_category", "database")
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			run()
		}
	}
}

func logResult(log infoLogger, result analytics.Result) {
	log.Info("analytics_run_finished",
		"analytics_run_id", result.RunID,
		"run_type", result.RunType,
		"target_period_start", result.Target.Format(time.DateOnly),
		"source_cycle_id", result.SourceCycleID,
		"rows", result.Rows,
		"source_day_count", result.SourceDayCount,
		"complete", result.Complete,
		"skipped", result.Skipped,
		"skip_reason", result.SkipReason,
		"method_version", analytics.MethodVersion,
	)
}

func parseWeek(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return analytics.MondayUTC(time.Now()), nil
	}
	value, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must use YYYY-MM-DD")
	}
	week := analytics.MondayUTC(value)
	if !week.Equal(value) {
		return time.Time{}, fmt.Errorf("date must be an ISO Monday")
	}
	return week, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationOr(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func boolOr(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
