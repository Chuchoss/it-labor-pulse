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
		name        string
		db          DBPinger
		rdb         RedisPinger
		wantStatus  string
		wantDB      string
		wantRedis   string
		wantChecks  bool
		wantDBKey   bool
		wantRedisKey bool
	}{
		{
			name:       "ok_without_deps",
			wantStatus: "ok",
			wantChecks: false,
		},
		{
			name:         "ok_with_db",
			db:           stubPinger{},
			wantStatus:   "ok",
			wantDB:       "up",
			wantChecks:   true,
			wantDBKey:    true,
			wantRedisKey: false,
		},
		{
			name:         "ok_with_redis",
			rdb:          stubPinger{},
			wantStatus:   "ok",
			wantRedis:    "up",
			wantChecks:   true,
			wantDBKey:    false,
			wantRedisKey: true,
		},
		{
			name:         "ok_with_both",
			db:           stubPinger{},
			rdb:          stubPinger{},
			wantStatus:   "ok",
			wantDB:       "up",
			wantRedis:    "up",
			wantChecks:   true,
			wantDBKey:    true,
			wantRedisKey: true,
		},
		{
			name:         "degraded_db_down",
			db:           stubPinger{err: errors.New("connection refused")},
			wantStatus:   "degraded",
			wantDB:       "down",
			wantChecks:   true,
			wantDBKey:    true,
			wantRedisKey: false,
		},
		{
			name:         "degraded_redis_down",
			rdb:          stubPinger{err: errors.New("connection refused")},
			wantStatus:   "degraded",
			wantRedis:    "down",
			wantChecks:   true,
			wantDBKey:    false,
			wantRedisKey: true,
		},
		{
			name:         "degraded_redis_down_db_up",
			db:           stubPinger{},
			rdb:          stubPinger{err: errors.New("timeout")},
			wantStatus:   "degraded",
			wantDB:       "up",
			wantRedis:    "down",
			wantChecks:   true,
			wantDBKey:    true,
			wantRedisKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			rec := httptest.NewRecorder()
			Health(log, tt.db, tt.rdb).ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")

			var got HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tt.wantStatus, got.Status)
			if !tt.wantChecks {
				require.Empty(t, got.Checks)
				return
			}
			if tt.wantDBKey {
				require.Equal(t, tt.wantDB, got.Checks["database"])
			} else {
				_, ok := got.Checks["database"]
				require.False(t, ok)
			}
			if tt.wantRedisKey {
				require.Equal(t, tt.wantRedis, got.Checks["redis"])
			} else {
				_, ok := got.Checks["redis"]
				require.False(t, ok)
			}
		})
	}
}
