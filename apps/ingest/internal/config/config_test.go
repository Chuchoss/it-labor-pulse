package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/config"
)

func TestLoad_AllPagesMode(t *testing.T) {
	t.Setenv("INGEST_MAX_PAGES", "all")
	t.Setenv("INGEST_PER_PAGE", "100")
	t.Setenv("INGEST_RUN_TIMEOUT_SEC", "1800")

	cfg := config.Load()

	require.Zero(t, cfg.MaxPages)
	require.Equal(t, 100, cfg.PerPage)
	require.Equal(t, 30*time.Minute, cfg.RunTimeout)
}

func TestValidateLive_RejectsInvalidPaging(t *testing.T) {
	cfg := config.Config{
		DatabaseURL: "postgres://example.invalid/db",
		HHUserAgent: "LMATest/0.1 (+test@example.com)",
		RunTimeout:  time.Minute,
		MaxPages:    -1,
		PerPage:     101,
	}

	require.ErrorContains(t, cfg.ValidateLive(), "INGEST_MAX_PAGES")
	cfg.MaxPages = 0
	require.ErrorContains(t, cfg.ValidateLive(), "INGEST_PER_PAGE")
}

func TestLoadSchedulerDefaults(t *testing.T) {
	for _, key := range []string{
		"INGEST_SCHEDULER_INTERVAL",
		"INGEST_SCHEDULER_RUN_ON_START",
		"INGEST_SCHEDULER_MAX_PARTITIONS_PER_BATCH",
		"INGEST_SCHEDULER_BACKOFF_INITIAL",
		"INGEST_SCHEDULER_BACKOFF_MAX",
		"INGEST_SCHEDULER_JITTER_PERCENT",
		"INGEST_SCHEDULER_SHUTDOWN_TIMEOUT",
		"INGEST_SCHEDULER_TEST_MODE",
	} {
		t.Setenv(key, "")
	}
	cfg := config.Load()
	require.Equal(t, 30*time.Minute, cfg.Scheduler.Interval)
	require.True(t, cfg.Scheduler.RunOnStart)
	require.Equal(t, 8, cfg.Scheduler.MaxPartitions)
	require.Equal(t, time.Minute, cfg.Scheduler.BackoffInitial)
	require.Equal(t, 15*time.Minute, cfg.Scheduler.BackoffMax)
	require.Equal(t, float64(20), cfg.Scheduler.JitterPercent)
	require.Equal(t, 30*time.Second, cfg.Scheduler.ShutdownTimeout)
}

func TestValidateSchedulerRejectsUnsafeInterval(t *testing.T) {
	cfg := config.Config{
		DatabaseURL: "postgres://example.invalid/db",
		HHUserAgent: "LMATest/0.1 (+test@example.com)",
		RunTimeout:  time.Minute, Scope: "it", MaxPages: 5, PerPage: 100,
		ITMaxDepth: 1, ITMaxParts: 1, ITMaxReqs: 2,
		Scheduler: config.SchedulerConfig{
			Interval: time.Minute, BackoffInitial: time.Minute,
			BackoffMax: time.Minute, JitterPercent: 20,
			ShutdownTimeout: time.Second, MaxPartitions: 1,
		},
	}
	require.ErrorContains(t, cfg.ValidateScheduler(), "at least")
	cfg.Scheduler.TestMode = true
	require.NoError(t, cfg.ValidateScheduler())
}
