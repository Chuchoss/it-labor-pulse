package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestNewPoolConfigBoundsOrphanTransactions(t *testing.T) {
	cfg, err := newPoolConfig("postgres://user:password@localhost:5432/db?sslmode=disable")
	require.NoError(t, err)

	require.EqualValues(t, 1, cfg.MaxConns)
	require.Equal(t, "30s", cfg.ConnConfig.RuntimeParams["statement_timeout"])
	require.Equal(t, "5s", cfg.ConnConfig.RuntimeParams["lock_timeout"])
	require.Equal(t, "15s", cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"])
	require.Equal(t, "lma-ingest", cfg.ConnConfig.RuntimeParams["application_name"])
}

func TestRetryTransientStopsWhenAttemptContextExpires(t *testing.T) {
	calls := 0
	err := retryTransientWithTimeout(
		context.Background(),
		3,
		10*time.Millisecond,
		func(ctx context.Context) error {
			calls++
			<-ctx.Done()
			return ctx.Err()
		},
	)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, calls)
}

func TestSanitizeDBErrorKeepsOnlySQLState(t *testing.T) {
	original := &pgconn.PgError{
		Code:    "55P03",
		Message: "canceling statement due to lock timeout",
		Detail:  "sensitive detail",
	}

	got := sanitizeDBError("save page", original)

	require.EqualError(t, got, "store save page: postgres error (sqlstate=55P03)")
	require.False(t, errors.Is(got, original))
}
