package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/assistant"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// localStore is an explicit test-only fallback.
type localStore struct{}

func (localStore) TryLock(context.Context) (func() error, bool, error) {
	return func() error { return nil }, true, nil
}
func (localStore) Users(context.Context) ([]assistant.WorkerUser, error) { return nil, nil }
func (localStore) Candidates(context.Context, string, time.Time, int) ([]assistant.WorkerCandidate, error) {
	return nil, nil
}
func (localStore) SaveMatch(context.Context, assistant.WorkerMatch) (bool, error) { return false, nil }
func (localStore) SaveDelivery(context.Context, assistant.WorkerDelivery) (bool, error) {
	return false, nil
}
func (localStore) AdvanceCursor(context.Context, string, time.Time, string) error { return nil }

func main() {
	once := flag.Bool("once", false, "run one bounded scan and exit")
	allowExternal := flag.Bool("allow-external", false, "allow explicitly enabled external AI validation")
	flag.Parse()
	_ = godotenv.Load()
	log := logging.New(logging.Options{Service: "assistant-worker", Env: envOr("APP_ENV", "local"), Level: envOr("LOG_LEVEL", "info")})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := assistant.WorkerOptions{
		Source:    envOr("ASSISTANT_SOURCE", "hh"),
		BatchSize: intEnv("ASSISTANT_BATCH_SIZE", 25),
		Log:       log,
		Cutoff:    time.Now().UTC().Add(-24 * time.Hour),
		AIBudget:  intEnv("ASSISTANT_MAX_AI_CALLS_PER_RUN", 20),
	}
	var store assistant.WorkerStore = localStore{}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			log.Error("assistant_worker_database_config_failed", "kind", "invalid_config")
			os.Exit(1)
		}
		cfg.MaxConns = 3
		cfg.ConnConfig.RuntimeParams["application_name"] = "lma-assistant-worker"
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10s"
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			log.Error("assistant_worker_database_open_failed", "kind", "open")
			os.Exit(1)
		}
		defer pool.Close()
		store = assistant.NewPostgresRepository(pool)
		log.Info("assistant_worker_store", "kind", "postgres")
	} else if envBool("ASSISTANT_LOCAL_STORE", false) && isLocalEnv() {
		log.Warn("assistant_worker_store", "kind", "local_test_fallback")
	} else {
		log.Error("assistant_worker_store_missing", "kind", "persistent_store_required")
		os.Exit(1)
	}
	if *allowExternal && (!envBool("ASSISTANT_AI_LIVE_TEST", false) || !envBool("ASSISTANT_AI_ENABLED", false)) {
		log.Error("assistant_worker_external_gate_denied", "kind", "requires_ai_enabled_and_live_test")
		os.Exit(1)
	}
	if envBool("ASSISTANT_AI_ENABLED", false) {
		cfg := assistant.LoadConfig()
		provider, err := assistant.NewDeepSeek(assistant.DeepSeekConfig{
			APIKey: cfg.DeepSeekAPIKey, BaseURL: cfg.DeepSeekBaseURL,
			Model: cfg.DeepSeekModel, Timeout: cfg.Timeout, MaxTokens: 600,
		}, nil)
		if err != nil {
			log.Error("assistant_worker_ai_config_failed", "kind", "provider_not_configured")
			os.Exit(1)
		}
		opts.AIProvider = provider
		opts.AIThreshold = 0.5
		log.Warn("assistant_worker_external_ai_enabled", "max_calls", 1)
	}
	run := func() error {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		stats, err := assistant.RunOnce(runCtx, store, opts)
		if err == nil {
			log.Info("assistant_worker_summary", "stats", stats)
		}
		return err
	}
	if *once {
		if err := run(); err != nil {
			log.Error("assistant_worker_failed", "kind", "run")
			os.Exit(1)
		}
		return
	}
	interval := durationEnv("ASSISTANT_WORKER_INTERVAL", 30*time.Minute)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := run(); err != nil {
			log.Error("assistant_worker_failed", "kind", "run")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch os.Getenv(key) {
	case "true", "TRUE", "1":
		return true
	case "false", "FALSE", "0":
		return false
	default:
		return fallback
	}
}

func isLocalEnv() bool {
	value := envOr("APP_ENV", "local")
	return value == "local" || value == "dev"
}
func intEnv(key string, fallback int) int {
	var value int
	if _, err := fmt.Sscan(os.Getenv(key), &value); err != nil || value < 1 || value > 100 {
		return fallback
	}
	return value
}
func durationEnv(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err != nil || value < time.Minute {
		return fallback
	}
	return value
}
