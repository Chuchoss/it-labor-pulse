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
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
)

func main() {
	// Local DX: load .env if present; never override already-set env vars.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config_load_failed", "err", err.Error())
		os.Exit(1)
	}

	log := logging.New(logging.Options{
		Service: "bff",
		Env:     cfg.AppEnv,
		Level:   cfg.LogLevel,
	})
	slog.SetDefault(log)

	var pinger db.Pinger
	if cfg.DatabaseURL != "" {
		p, err := db.Open(cfg.DatabaseURL)
		if err != nil {
			// DSN itself is never logged.
			log.Error("database_open_failed", "err", err.Error())
			os.Exit(1)
		}
		pinger = p
		defer func() { _ = pinger.Close() }()
		log.Info("database_configured")
	} else {
		log.Info("database_not_configured")
	}

	srv := httpserver.New(httpserver.Options{
		Addr: cfg.HTTPAddr,
		Log:  log,
		DB:   pinger,
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
