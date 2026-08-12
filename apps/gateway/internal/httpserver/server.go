package httpserver

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/gateway/internal/proxy"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/httpx"
)

// Options configures the gateway HTTP server.
type Options struct {
	Addr        string
	BFFUpstream *url.URL
	Log         *slog.Logger
}

// New builds an http.Server: edge CORS + correlation, local /healthz,
// reverse proxy /api/* → BFF. No business logic here (ADR 010).
func New(opts Options) *http.Server {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"gateway"}`))
	})
	mux.Handle("/", proxy.NewAPIReverseProxy(opts.BFFUpstream))

	// Order: CORS → rate-limit stub → correlation → routes.
	var handler http.Handler = mux
	handler = httpx.Middleware(log)(handler)
	handler = rateLimitStub(handler)
	handler = corsStub(handler)

	return &http.Server{
		Addr:              opts.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// corsStub is a Phase 0 permissive CORS edge policy (tighten in Target).
func corsStub(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-Id, Traceparent, X-Admin-Token")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id, Traceparent")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimitStub is a no-op placeholder for edge rate limiting (Target).
func rateLimitStub(next http.Handler) http.Handler {
	return next
}
