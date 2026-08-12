package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/httpx"
)

// Options configures the BFF HTTP server.
type Options struct {
	Addr  string
	Log   *slog.Logger
	DB    DBPinger
	Redis RedisPinger
}

// New builds an http.Server with Phase 0 routes and correlation middleware.
func New(opts Options) *http.Server {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", Health(log, opts.DB, opts.Redis))

	handler := httpx.Middleware(log)(mux)

	return &http.Server{
		Addr:              opts.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
