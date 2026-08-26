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
	RunTimeout  time.Duration

	// FixtureDir when set enables offline fixture mode (testdata/hh).
	FixtureDir string
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
		MaxPages:    envInt("INGEST_MAX_PAGES", 5),
		RunTimeout:  time.Duration(envInt("INGEST_RUN_TIMEOUT_SEC", 300)) * time.Second,
		FixtureDir:  strings.TrimSpace(os.Getenv("INGEST_FIXTURE_DIR")),
	}
	delayMS := envInt("INGEST_PAGE_DELAY_MS", 350)
	if delayMS < 0 {
		delayMS = 0
	}
	cfg.PageDelay = time.Duration(delayMS) * time.Millisecond
	return cfg
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
