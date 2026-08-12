package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// DBPinger is the optional Postgres check for health.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// RedisPinger is the optional Redis check for health.
type RedisPinger interface {
	Ping(ctx context.Context) error
}

// HealthResponse is the GET /api/v1/health body (OpenAPI status + optional checks).
// When DATABASE_URL / REDIS_URL is configured and the dependency is unreachable,
// status is "degraded" and checks.database / checks.redis is "down".
// HTTP status stays 200 so edge liveness can stay simple in Phase 0.
type HealthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Health handles GET /api/v1/health.
func Health(log *slog.Logger, db DBPinger, rdb RedisPinger) http.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		resp := HealthResponse{Status: "ok"}
		if db != nil || rdb != nil {
			resp.Checks = map[string]string{}
		}
		if db != nil {
			resp.Checks["database"] = "up"
			if err := db.Ping(r.Context()); err != nil {
				resp.Status = "degraded"
				resp.Checks["database"] = "down"
				log.Warn("health_database_down", "err", err.Error())
			}
		}
		if rdb != nil {
			resp.Checks["redis"] = "up"
			if err := rdb.Ping(r.Context()); err != nil {
				resp.Status = "degraded"
				resp.Checks["redis"] = "down"
				log.Warn("health_redis_down", "err", err.Error())
			}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}
