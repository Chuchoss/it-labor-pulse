package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type fakePageTx struct {
	execErr       error
	execCalls     int
	commitCalls   int
	rollbackCalls int
	committed     bool
}

func (f *fakePageTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	f.execCalls++
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakePageTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected Query call")
}

func (f *fakePageTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("unexpected QueryRow call")
}

func (f *fakePageTx) Commit(context.Context) error {
	f.commitCalls++
	f.committed = true
	return nil
}

func (f *fakePageTx) Rollback(context.Context) error {
	f.rollbackCalls++
	if f.committed {
		return pgx.ErrTxClosed
	}
	return nil
}

func TestNewPoolConfigBoundsOrphanTransactions(t *testing.T) {
	cfg, err := newPoolConfig("postgres://user:password@localhost:5432/db?sslmode=disable")
	require.NoError(t, err)

	require.EqualValues(t, 1, cfg.MaxConns)
	require.Equal(t, "30s", cfg.ConnConfig.RuntimeParams["statement_timeout"])
	require.Equal(t, "5s", cfg.ConnConfig.RuntimeParams["lock_timeout"])
	require.Equal(t, "15s", cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"])
	require.Equal(t, "lma-ingest", cfg.ConnConfig.RuntimeParams["application_name"])
}

func TestSavePageInTxUsesHeldTransactionWithSingleConnection(t *testing.T) {
	tx := &fakePageTx{}

	upserted, unchanged, err := savePageInTx(
		context.Background(),
		tx,
		"hh",
		strings.Repeat("a", 64),
		"1",
		nil,
		func() { t.Fatal("healthy transaction must not discard its connection") },
	)

	require.NoError(t, err)
	require.Zero(t, upserted)
	require.Zero(t, unchanged)
	require.Equal(t, 1, tx.execCalls)
	require.Equal(t, 1, tx.commitCalls)
	require.Equal(t, 1, tx.rollbackCalls)
}

func TestSavePageInTxRollsBackOnCheckpointError(t *testing.T) {
	checkpointErr := errors.New("checkpoint failed")
	tx := &fakePageTx{execErr: checkpointErr}

	_, _, err := savePageInTx(
		context.Background(),
		tx,
		"hh",
		strings.Repeat("b", 64),
		"1",
		nil,
		func() { t.Fatal("successful rollback must not discard its connection") },
	)

	require.ErrorIs(t, err, checkpointErr)
	require.Equal(t, 1, tx.execCalls)
	require.Zero(t, tx.commitCalls)
	require.Equal(t, 1, tx.rollbackCalls)
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

func TestRetryTransientWithZeroTimeoutUsesCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var operationDeadline time.Time
	err := retryTransientWithTimeout(ctx, 1, 0, func(attemptCtx context.Context) error {
		var ok bool
		operationDeadline, ok = attemptCtx.Deadline()
		require.True(t, ok)
		return nil
	})

	require.NoError(t, err)
	callerDeadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.Equal(t, callerDeadline, operationDeadline)
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
