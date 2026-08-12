package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubPinger struct {
	err error
}

func (s stubPinger) Ping(context.Context) error { return s.err }

func TestHealth(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name       string
		db         DBPinger
		wantStatus string
		wantDB     string
		wantChecks bool
	}{
		{
			name:       "ok_without_db",
			wantStatus: "ok",
			wantChecks: false,
		},
		{
			name:       "ok_with_db",
			db:         stubPinger{},
			wantStatus: "ok",
			wantDB:     "up",
			wantChecks: true,
		},
		{
			name:       "degraded_db_down",
			db:         stubPinger{err: errors.New("connection refused")},
			wantStatus: "degraded",
			wantDB:     "down",
			wantChecks: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			rec := httptest.NewRecorder()
			Health(log, tt.db).ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")

			var got HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tt.wantStatus, got.Status)
			if tt.wantChecks {
				require.Equal(t, tt.wantDB, got.Checks["database"])
			} else {
				require.Empty(t, got.Checks)
			}
		})
	}
}
