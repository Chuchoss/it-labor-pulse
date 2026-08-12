package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// DBPinger is the optional dependency check for health.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// HealthResponse is the GET /api/v1/health body (OpenAPI status + optional checks).
// When DATABASE_URL is configured and Postgres is unreachable, status is "degraded"
// and checks.database is "down". HTTP status stays 200 so edge liveness can stay simple in Phase 0.
type HealthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Health handles GET /api/v1/health.
func Health(log *slog.Logger, db DBPinger) http.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		resp := HealthResponse{Status: "ok"}
		if db != nil {
			resp.Checks = map[string]string{"database": "up"}
			if err := db.Ping(r.Context()); err != nil {
				resp.Status = "degraded"
				resp.Checks["database"] = "down"
				log.Warn("health_database_down", "err", err.Error())
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
