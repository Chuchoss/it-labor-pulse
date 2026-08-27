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
	allowTelegram := flag.Bool("allow-telegram", false, "allow explicitly enabled Telegram delivery")
	pauseRun := flag.String("pause-run", "", "pause an active assistant run without completing it")
	resumeRun := flag.String("resume-run", "", "queue the same paused assistant run for resumption")
	probeExternal := flag.Bool("probe-external", false, "make exactly one synthetic provider health call")
	providerTimeout := flag.Duration("provider-timeout", 0, "override provider timeout for this worker")
	batchSize := flag.Int("batch-size", 0, "override durable processing batch size")
	flag.Parse()
	_ = godotenv.Overload()
	log := logging.New(logging.Options{Service: "assistant-worker", Env: envOr("APP_ENV", "local"), Level: envOr("LOG_LEVEL", "info")})
	envFile, _ := godotenv.Read()
	log.Info("assistant_worker_environment", "env_file_database_selected",
		envFile["DATABASE_URL"] != "" && os.Getenv("DATABASE_URL") == envFile["DATABASE_URL"])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := assistant.WorkerOptions{
		Source:          envOr("ASSISTANT_SOURCE", "hh"),
		BatchSize:       intEnv("ASSISTANT_BATCH_SIZE", 25),
		Log:             log,
		Cutoff:          time.Now().UTC().Add(-24 * time.Hour),
		TelegramEnabled: envBool("ASSISTANT_TELEGRAM_ENABLED", false) && *allowTelegram,
	}
	if *batchSize > 0 && *batchSize <= 100 {
		opts.BatchSize = *batchSize
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
	if *pauseRun != "" || *resumeRun != "" {
		if *pauseRun != "" && *resumeRun != "" {
			log.Error("assistant_worker_run_control_failed", "kind", "conflicting_flags")
			os.Exit(1)
		}
		control, ok := store.(assistant.AssistantRunControlStore)
		if !ok {
			log.Error("assistant_worker_run_control_failed", "kind", "persistent_store_required")
			os.Exit(1)
		}
		var err error
		action, runID := "pause", *pauseRun
		if *resumeRun != "" {
			action, runID = "resume", *resumeRun
			err = control.ResumeAssistantRun(ctx, runID)
		} else {
			err = control.PauseAssistantRun(ctx, runID)
		}
		if err != nil {
			log.Error("assistant_worker_run_control_failed", "kind", action)
			os.Exit(1)
		}
		log.Info("assistant_worker_run_control_complete", "action", action, "run_id", runID)
		return
	}
	if *allowExternal && (!envBool("ASSISTANT_AI_LIVE_TEST", false) || !envBool("ASSISTANT_AI_ENABLED", false)) {
		log.Error("assistant_worker_external_gate_denied", "kind", "requires_ai_enabled_and_live_test")
		os.Exit(1)
	}
	if envBool("ASSISTANT_AI_ENABLED", false) && *allowExternal {
		cfg := assistant.LoadConfig()
		if *providerTimeout > 0 {
			cfg.Timeout = *providerTimeout
		}
		provider, err := assistant.NewDeepSeek(assistant.DeepSeekConfig{
			APIKey: cfg.DeepSeekAPIKey, BaseURL: cfg.DeepSeekBaseURL,
			Model: cfg.DeepSeekModel, Timeout: cfg.Timeout, MaxTokens: 1800,
			MaxAttempts: 3, MinInterval: time.Second,
		}, nil)
		if err != nil {
			log.Error("assistant_worker_ai_config_failed", "kind", "provider_not_configured")
			os.Exit(1)
		}
		opts.AIProvider = provider
		log.Warn("assistant_worker_external_ai_enabled", "request_count", "unlimited")
	}
	if *probeExternal {
		detailed, ok := opts.AIProvider.(assistant.DetailedAIProvider)
		if !ok {
			log.Error("assistant_worker_ai_probe_failed", "category", "provider_unavailable")
			os.Exit(1)
		}
		_, stats, err := detailed.CompleteDetailed(ctx, assistant.Request{
			InputSnapshot: "VACANCY_DATA_BEGIN\nSynthetic provider health check\nVACANCY_DATA_END\nFACTS:\nEVIDENCE_IDS:\nvacancy:title\n",
			Evidence:      map[string]bool{"vacancy:title": true},
		})
		if err != nil {
			log.Error("assistant_worker_ai_probe_failed", "category", stats.Category,
				"http_attempts", stats.HTTPAttempts, "retries", stats.Retries,
				"latency_ms", stats.Latency.Milliseconds())
			os.Exit(1)
		}
		log.Info("assistant_worker_ai_probe_complete", "http_attempts", stats.HTTPAttempts,
			"retries", stats.Retries, "latency_ms", stats.Latency.Milliseconds())
		return
	}
	run := func() error {
		runCtx, cancel := context.WithTimeout(ctx, durationEnv("ASSISTANT_RUN_TIMEOUT", 10*time.Minute))
		defer cancel()
		stats, err := assistant.RunOnce(runCtx, store, opts)
		if err == nil {
			log.Info("assistant_worker_summary", "stats", stats)
		}
		if err == nil && opts.TelegramEnabled {
			deliveryStore, ok := store.(assistant.DeliveryStore)
			if !ok {
				return fmt.Errorf("telegram delivery store is not configured")
			}
			cfg := assistant.LoadConfig()
			client, clientErr := assistant.NewTelegram(cfg.TelegramBotToken, nil)
			if clientErr != nil {
				return clientErr
			}
			deliveryCtx, deliveryCancel := context.WithTimeout(ctx, 30*time.Second)
			defer deliveryCancel()
			deliveryStats, deliveryErr := assistant.RunDeliveryOnce(deliveryCtx, deliveryStore, client,
				assistant.DeliveryOptions{BatchSize: opts.BatchSize, Log: log})
			if deliveryErr == nil {
				log.Info("assistant_delivery_summary", "stats", deliveryStats)
			}
			return deliveryErr
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
	if err != nil || value < time.Second {
		return fallback
	}
	return value
}
