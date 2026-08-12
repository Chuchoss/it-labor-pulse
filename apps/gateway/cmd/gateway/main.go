package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Chuchoss/it-labor-pulse/apps/gateway/internal/config"
	"github.com/Chuchoss/it-labor-pulse/apps/gateway/internal/httpserver"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/logging"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config_load_failed", "err", err.Error())
		os.Exit(1)
	}

	log := logging.New(logging.Options{
		Service: "gateway",
		Env:     cfg.AppEnv,
		Level:   cfg.LogLevel,
	})
	slog.SetDefault(log)

	upstream, err := url.Parse(cfg.BFFUpstream)
	if err != nil {
		log.Error("bff_upstream_parse_failed", "err", err.Error())
		os.Exit(1)
	}

	srv := httpserver.New(httpserver.Options{
		Addr:        cfg.HTTPAddr,
		BFFUpstream: upstream,
		Log:         log,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("gateway_listening", "addr", cfg.HTTPAddr, "bff_upstream", cfg.BFFUpstream)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("gateway_shutdown_started")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("gateway_shutdown_error", "err", err.Error())
			os.Exit(1)
		}
		log.Info("gateway_shutdown_complete")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("gateway_serve_failed", "err", err.Error())
			os.Exit(1)
		}
	}
}
