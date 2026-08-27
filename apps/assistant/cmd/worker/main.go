package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/assistant"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// localStore is an explicit test-only fallback.
type localStore struct{}

type limitedTransport struct {
	mu        sync.Mutex
	remaining int
	base      http.RoundTripper
}

func (t *limitedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if t.remaining <= 0 {
		t.mu.Unlock()
		return nil, fmt.Errorf("controlled external request limit reached")
	}
	t.remaining--
	t.mu.Unlock()
	return t.base.RoundTrip(request)
}

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
	maxSnapshotPages := flag.Int("max-snapshot-pages", 0, "pause a manual run after this many durable pages")
	maxExternalHTTP := flag.Int("max-external-http", 0, "cap external HTTP calls for a controlled verification")
	inspectRun := flag.String("inspect-run", "", "print safe runtime metadata for one assistant run")
	recoverRun := flag.String("recover-run", "", "requeue one unfinished superseded run without resetting progress")
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
	if *maxSnapshotPages > 0 {
		opts.MaxSnapshotPages = *maxSnapshotPages
	}
	var store assistant.WorkerStore = localStore{}
	var pool *pgxpool.Pool
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			log.Error("assistant_worker_database_config_failed", "kind", "invalid_config")
			os.Exit(1)
		}
		cfg.MaxConns = 3
		cfg.ConnConfig.RuntimeParams["application_name"] = "lma-assistant-worker"
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10s"
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
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
	if *inspectRun != "" || *recoverRun != "" {
		if pool == nil {
			log.Error("assistant_worker_inspect_failed", "category", "persistent_store_required")
			os.Exit(1)
		}
		if *inspectRun != "" && *recoverRun != "" {
			log.Error("assistant_worker_run_control_failed", "category", "conflicting_flags")
			os.Exit(1)
		}
		if *recoverRun != "" {
			if err := recoverAssistantRun(ctx, pool, *recoverRun, log); err != nil {
				log.Error("assistant_worker_run_control_failed", "category", "database")
				os.Exit(1)
			}
			return
		}
		if err := inspectAssistantRun(ctx, pool, *inspectRun, log); err != nil {
			log.Error("assistant_worker_inspect_failed", "category", "database")
			os.Exit(1)
		}
		return
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
		var providerClient *http.Client
		if *maxExternalHTTP > 0 {
			providerClient = &http.Client{
				Timeout:   cfg.Timeout,
				Transport: &limitedTransport{remaining: *maxExternalHTTP, base: http.DefaultTransport},
			}
		}
		provider, err := assistant.NewDeepSeek(assistant.DeepSeekConfig{
			APIKey: cfg.DeepSeekAPIKey, BaseURL: cfg.DeepSeekBaseURL,
			Model: cfg.DeepSeekModel, Timeout: cfg.Timeout, MaxTokens: cfg.AIMaxOutputTokens,
			MaxAttempts: 3, MinInterval: time.Second,
			MaxBatchSize: cfg.AIMaxBatchSize, InputTokenBudget: cfg.AIInputTokenBudget,
		}, providerClient)
		if err != nil {
			log.Error("assistant_worker_ai_config_failed", "kind", "provider_not_configured")
			os.Exit(1)
		}
		opts.AIProvider = provider
		if opts.BatchSize > cfg.AIMaxBatchSize {
			opts.BatchSize = cfg.AIMaxBatchSize
		}
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
	run := func() (err error) {
		defer func() {
			if recover() != nil {
				log.Error("assistant_worker_run_panic", "category", "panic")
				err = fmt.Errorf("assistant worker run panic")
			}
		}()
		runCtx := ctx
		cancel := func() {}
		if timeout := optionalDurationEnv("ASSISTANT_RUN_TIMEOUT"); timeout > 0 {
			runCtx, cancel = context.WithTimeout(ctx, timeout)
		}
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
	log.Info("assistant_worker_started", "poll_interval_ms", interval.Milliseconds(), "batch_size", opts.BatchSize)
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

func optionalDurationEnv(key string) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < time.Second {
		return 0
	}
	return parsed
}

func recoverAssistantRun(ctx context.Context, pool *pgxpool.Pool, runID string, log interface {
	Info(string, ...any)
}) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var userID string
	err = tx.QueryRow(ctx, `
		SELECT user_id::text FROM assistant_runs
		WHERE id=$1::uuid AND state='superseded'
		  AND superseded_from_state='running' AND processed < snapshot_total
		FOR UPDATE
	`, runID).Scan(&userID)
	if err != nil {
		return err
	}
	disabled, err := tx.Exec(ctx, `
		UPDATE assistant_runs
		SET state='disabled', lease_until=NULL, finished_at=now(), last_checked_at=now(),
			error_category='operator_recovered_previous_run', worker_phase='idle',
			worker_retry_category=NULL, worker_retry_until=NULL
		WHERE user_id=$1::uuid AND id<>$2::uuid AND state IN ('queued','running','paused')
	`, userID, runID)
	if err != nil {
		return err
	}
	recovered, err := tx.Exec(ctx, `
		UPDATE assistant_runs
		SET state='queued', lease_until=NULL, finished_at=NULL, last_checked_at=now(),
			error_category=NULL, superseded_at=NULL, superseded_by_preference_id=NULL,
			superseded_from_state=NULL, worker_heartbeat_at=NULL, worker_phase='idle',
			worker_retry_category=NULL, worker_retry_until=NULL
		WHERE id=$1::uuid AND state='superseded'
	`, runID)
	if err != nil {
		return err
	}
	if recovered.RowsAffected() != 1 {
		return fmt.Errorf("assistant run is not recoverable")
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Info("assistant_worker_run_recovered", "run_id", runID,
		"conflicting_runs_disabled", disabled.RowsAffected())
	return nil
}

func inspectAssistantRun(ctx context.Context, pool *pgxpool.Pool, runID string, log interface {
	Info(string, ...any)
}) error {
	var state, phase, supersededFrom string
	var processed, total, aiCalls, aiSucceeded, aiFailures, httpAttempts, retries, batches int
	var preferenceVersion, currentPreferenceVersion int
	var progressAge, leaseRemaining, heartbeatAge *int
	err := pool.QueryRow(ctx, `
		SELECT r.state, r.processed, r.snapshot_total, r.ai_calls, r.ai_succeeded, r.ai_failures,
			ai_http_attempts, ai_retries, ai_batches, worker_phase,
			EXTRACT(EPOCH FROM now()-last_checked_at)::integer,
			CASE WHEN lease_until IS NULL THEN NULL
			     ELSE EXTRACT(EPOCH FROM lease_until-now())::integer END,
			CASE WHEN worker_heartbeat_at IS NULL THEN NULL
			     ELSE EXTRACT(EPOCH FROM now()-worker_heartbeat_at)::integer END,
			run_preference.version, current_preference.version,
			COALESCE(r.superseded_from_state, '')
		FROM assistant_runs r
		JOIN vacancy_preferences run_preference ON run_preference.id=r.preference_id
		JOIN LATERAL (
			SELECT version FROM vacancy_preferences
			WHERE user_id=r.user_id AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		) current_preference ON true
		WHERE r.id=$1::uuid
	`, runID).Scan(&state, &processed, &total, &aiCalls, &aiSucceeded, &aiFailures,
		&httpAttempts, &retries, &batches, &phase, &progressAge, &leaseRemaining, &heartbeatAge,
		&preferenceVersion, &currentPreferenceVersion, &supersededFrom)
	if err != nil {
		return err
	}
	var advisoryHolders, blockedSessions, aiAttempts, aiComplete, activeRuns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM pg_locks
		WHERE locktype='advisory' AND granted AND objid=549004802
	`).Scan(&advisoryHolders); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM pg_stat_activity WHERE wait_event_type='Lock'
	`).Scan(&blockedSessions); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(sum(j.attempts),0)::integer,
		       count(*) FILTER (WHERE j.status='complete')::integer
		FROM assistant_ai_jobs j
		JOIN assistant_runs r ON r.user_id=j.user_id AND r.preference_id=j.preference_id
		WHERE r.id=$1::uuid
	`, runID).Scan(&aiAttempts, &aiComplete); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer
		FROM assistant_runs active
		WHERE active.user_id=(SELECT user_id FROM assistant_runs WHERE id=$1::uuid)
		  AND active.state IN ('queued','running','paused')
	`, runID).Scan(&activeRuns); err != nil {
		return err
	}
	log.Info("assistant_worker_run_metadata",
		"run_id", runID, "state", state, "processed", processed, "total", total,
		"ai_calls", aiCalls, "ai_succeeded", aiSucceeded, "ai_failures", aiFailures,
		"ai_http_attempts", httpAttempts, "ai_retries", retries, "ai_batches", batches,
		"worker_phase", phase, "progress_age_seconds", progressAge,
		"lease_remaining_seconds", leaseRemaining, "heartbeat_age_seconds", heartbeatAge,
		"preference_version", preferenceVersion, "current_preference_version", currentPreferenceVersion,
		"superseded_from_state", supersededFrom,
		"advisory_lock_holders", advisoryHolders, "blocked_sessions", blockedSessions,
		"active_runs_for_user", activeRuns,
		"ai_job_attempts", aiAttempts, "ai_jobs_complete", aiComplete)
	return nil
}
