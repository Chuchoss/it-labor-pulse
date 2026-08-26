package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds ingest runtime settings from environment.
type Config struct {
	DatabaseURL string
	AppEnv      string
	LogLevel    string

	HHUserAgent string
	HHBaseURL   string
	HHAppToken  string

	DefaultArea string
	DefaultText string
	PageDelay   time.Duration
	MaxPages    int
	PerPage     int
	RunTimeout  time.Duration
	Scope       string
	ITArea      string
	ITMaxDepth  int
	ITMaxParts  int
	ITMaxReqs   int

	// FixtureDir when set enables offline fixture mode (testdata/hh).
	FixtureDir string

	Scheduler SchedulerConfig
}

// SchedulerConfig contains dedicated scheduler process settings.
type SchedulerConfig struct {
	Interval        time.Duration
	RunOnStart      bool
	MaxPartitions   int
	BackoffInitial  time.Duration
	BackoffMax      time.Duration
	JitterPercent   float64
	ShutdownTimeout time.Duration
	TestMode        bool
}

// Load reads ingest config from env.
func Load() Config {
	cfg := Config{
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		AppEnv:      envOr("APP_ENV", "local"),
		LogLevel:    envOr("LOG_LEVEL", "info"),
		HHUserAgent: strings.TrimSpace(os.Getenv("HH_USER_AGENT")),
		HHBaseURL:   envOr("HH_BASE_URL", "https://api.hh.ru"),
		HHAppToken:  strings.TrimSpace(os.Getenv("HH_APP_TOKEN")),
		DefaultArea: envOr("INGEST_DEFAULT_AREA", "1"),
		DefaultText: envOr("INGEST_DEFAULT_TEXT", "golang"),
		MaxPages:    envMaxPages("INGEST_MAX_PAGES", 5),
		PerPage:     envInt("INGEST_PER_PAGE", 100),
		RunTimeout:  time.Duration(envInt("INGEST_RUN_TIMEOUT_SEC", 1800)) * time.Second,
		Scope:       envOr("INGEST_SCOPE", "query"),
		ITArea:      envOr("INGEST_IT_AREA", "113"),
		ITMaxDepth:  envInt("INGEST_IT_MAX_DEPTH", 32),
		ITMaxParts:  envInt("INGEST_IT_MAX_PARTITIONS", 512),
		ITMaxReqs:   envInt("INGEST_IT_MAX_REQUESTS", 500),
		FixtureDir:  strings.TrimSpace(os.Getenv("INGEST_FIXTURE_DIR")),
		Scheduler: SchedulerConfig{
			Interval:        envDuration("INGEST_SCHEDULER_INTERVAL", 30*time.Minute),
			RunOnStart:      envBool("INGEST_SCHEDULER_RUN_ON_START", true),
			MaxPartitions:   envInt("INGEST_SCHEDULER_MAX_PARTITIONS_PER_BATCH", 8),
			BackoffInitial:  envDuration("INGEST_SCHEDULER_BACKOFF_INITIAL", time.Minute),
			BackoffMax:      envDuration("INGEST_SCHEDULER_BACKOFF_MAX", 15*time.Minute),
			JitterPercent:   envFloat("INGEST_SCHEDULER_JITTER_PERCENT", 20),
			ShutdownTimeout: envDuration("INGEST_SCHEDULER_SHUTDOWN_TIMEOUT", 30*time.Second),
			TestMode:        envBool("INGEST_SCHEDULER_TEST_MODE", false),
		},
	}
	delayMS := envInt("INGEST_PAGE_DELAY_MS", 350)
	if delayMS < 0 {
		delayMS = 0
	}
	cfg.PageDelay = time.Duration(delayMS) * time.Millisecond
	return cfg
}

// ValidateScheduler validates both live ingest and scheduler-specific settings.
func (c Config) ValidateScheduler() error {
	if err := c.ValidateLive(); err != nil {
		return err
	}
	if c.Scope != "it" {
		return fmt.Errorf("scheduler requires INGEST_SCOPE=it")
	}
	if c.MaxPages < 1 {
		return fmt.Errorf("scheduler requires positive INGEST_MAX_PAGES")
	}
	if c.ITMaxReqs < c.PerPage+1 {
		return fmt.Errorf("INGEST_IT_MAX_REQUESTS must allow at least one search page and its details")
	}
	minInterval := 10 * time.Minute
	if c.Scheduler.TestMode {
		minInterval = time.Millisecond
	}
	if c.Scheduler.Interval < minInterval {
		return fmt.Errorf("INGEST_SCHEDULER_INTERVAL must be at least %s", minInterval)
	}
	if c.Scheduler.BackoffInitial <= 0 {
		return fmt.Errorf("INGEST_SCHEDULER_BACKOFF_INITIAL must be positive")
	}
	if c.Scheduler.MaxPartitions < 1 {
		return fmt.Errorf("INGEST_SCHEDULER_MAX_PARTITIONS_PER_BATCH must be positive")
	}
	if c.Scheduler.BackoffMax < c.Scheduler.BackoffInitial {
		return fmt.Errorf("INGEST_SCHEDULER_BACKOFF_MAX must be at least initial backoff")
	}
	if c.Scheduler.JitterPercent < 0 || c.Scheduler.JitterPercent > 100 {
		return fmt.Errorf("INGEST_SCHEDULER_JITTER_PERCENT must be between 0 and 100")
	}
	if c.Scheduler.ShutdownTimeout <= 0 {
		return fmt.Errorf("INGEST_SCHEDULER_SHUTDOWN_TIMEOUT must be positive")
	}
	return nil
}

// ValidateLive requires UA + DATABASE_URL for live HH ingest.
func (c Config) ValidateLive() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.FixtureDir == "" && c.HHUserAgent == "" {
		return fmt.Errorf("HH_USER_AGENT is required for live ingest")
	}
	if c.RunTimeout <= 0 {
		return fmt.Errorf("INGEST_RUN_TIMEOUT_SEC must be positive")
	}
	if c.MaxPages < 0 {
		return fmt.Errorf("INGEST_MAX_PAGES must be 0/all or a positive integer")
	}
	if c.PerPage < 1 || c.PerPage > 100 {
		return fmt.Errorf("INGEST_PER_PAGE must be between 1 and 100")
	}
	if c.Scope != "query" && c.Scope != "it" {
		return fmt.Errorf("INGEST_SCOPE must be query or it")
	}
	if c.ITMaxDepth < 1 || c.ITMaxParts < 1 || c.ITMaxReqs < 1 {
		return fmt.Errorf("INGEST_IT safety ceilings must be positive")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envMaxPages(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if strings.EqualFold(v, "all") {
		return 0
	}
	return envInt(key, fallback)
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return -1
	}
	return d
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envFloat(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return -1
	}
	return n
}
