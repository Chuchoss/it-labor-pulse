package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/config"
	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/db"
	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/httpserver"
	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/readapi"
	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/redisx"
	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/repository"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/assistant"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
)

func main() {
	// Local DX: load .env if present; never override already-set env vars.
	_ = godotenv.Load()

	cfg := config.Load()
	log := logging.New(logging.Options{
		Service: "bff",
		Env:     cfg.AppEnv,
		Level:   cfg.LogLevel,
	})
	slog.SetDefault(log)

	var dbPinger db.Pinger
	var readService httpserver.ReadService
	var assistantRepository httpserver.AssistantRepository
	if cfg.DatabaseURL != "" {
		p, err := db.Open(cfg.DatabaseURL)
		if err != nil {
			// DSN itself is never logged.
			log.Error("database_open_failed", "err", err.Error())
			os.Exit(1)
		}
		dbPinger = p
		readService = readapi.NewService(repository.NewPostgres(p.Pool()))
		assistantRepository = assistant.NewPostgresRepository(p.Pool())
		defer func() { _ = dbPinger.Close() }()
		log.Info("database_configured")
	} else {
		log.Info("database_not_configured")
	}
	if cfg.AssistantEnabled && assistantRepository == nil &&
		!cfg.AssistantLocalStore && cfg.AppEnv != "local" && cfg.AppEnv != "dev" {
		log.Error("assistant_store_missing", "kind", "persistent_store_required")
		os.Exit(1)
	}
	if cfg.AssistantEnabled && assistantRepository == nil && !cfg.AssistantLocalStore {
		log.Error("assistant_store_missing", "kind", "set_database_url_or_assistant_local_store")
		os.Exit(1)
	}

	var redisPinger redisx.Pinger
	if cfg.RedisURL != "" {
		p, err := redisx.Open(cfg.RedisURL)
		if err != nil {
			// URL itself is never logged (may contain password).
			log.Error("redis_open_failed", "err", err.Error())
			os.Exit(1)
		}
		redisPinger = p
		defer func() { _ = redisPinger.Close() }()
		log.Info("redis_configured")
	} else {
		log.Info("redis_not_configured")
	}
	var telegramSender assistant.DeliveryTelegramClient
	if cfg.AssistantTelegramEnabled {
		telegramConfig := assistant.LoadConfig()
		telegramSender, _ = assistant.NewTelegram(telegramConfig.TelegramBotToken, nil)
	}

	srv := httpserver.New(httpserver.Options{
		Addr:        cfg.HTTPAddr,
		Log:         log,
		DB:          dbPinger,
		Redis:       redisPinger,
		ReadService: readService,
		Assistant: httpserver.AssistantOptions{
			Enabled:            cfg.AssistantEnabled,
			DevAuthEnabled:     cfg.AssistantDevAuthEnabled && (cfg.AppEnv == "local" || cfg.AppEnv == "dev"),
			DevSubject:         cfg.AssistantDevSubject,
			Repository:         assistantRepository,
			TelegramConfigured: cfg.AssistantTelegramEnabled,
			AIConfigured:       cfg.AssistantAIEnabled,
			TelegramSender:     telegramSender,
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("bff_listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("bff_shutdown_started")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("bff_shutdown_error", "err", err.Error())
			os.Exit(1)
		}
		log.Info("bff_shutdown_complete")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("bff_serve_failed", "err", err.Error())
			os.Exit(1)
		}
	}
}
