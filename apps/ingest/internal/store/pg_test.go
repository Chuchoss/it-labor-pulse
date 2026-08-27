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

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

type fakePageTx struct {
	execErr       error
	execCalls     int
	commitCalls   int
	rollbackCalls int
	committed     bool
}

type stubRow struct {
	values []any
	err    error
}

func (r stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, value := range r.values {
		switch target := dest[i].(type) {
		case *string:
			*target = value.(string)
		case *[]byte:
			*target = value.([]byte)
		default:
			panic("unsupported scan target")
		}
	}
	return nil
}

type unchangedVacancyDB struct {
	queryCalls int
	updateArgs []any
	updateSQL  string
}

func (f *unchangedVacancyDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "UPDATE vacancies") {
		f.updateArgs = args
		f.updateSQL = sql
	}
	return pgconn.CommandTag{}, nil
}

func (f *unchangedVacancyDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected Query call")
}

func (f *unchangedVacancyDB) QueryRow(context.Context, string, ...any) pgx.Row {
	f.queryCalls++
	switch f.queryCalls {
	case 1:
		return stubRow{values: []any{"00000000-0000-0000-0000-000000000002"}}
	case 2:
		return stubRow{values: []any{
			"00000000-0000-0000-0000-000000000010",
			[]byte{1},
		}}
	default:
		panic("unexpected QueryRow call")
	}
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

	require.EqualValues(t, 2, cfg.MaxConns) // advisory-lock session + sequential ingest
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
		func(context.Context) { t.Fatal("healthy transaction must not discard its connection") },
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
		func(context.Context) { t.Fatal("successful rollback must not discard its connection") },
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

func TestUnchangedVacancyReconcilesCanonicalRegionReference(t *testing.T) {
	db := &unchangedVacancyDB{}
	changed, err := upsertVacancy(context.Background(), db, VacancyWrite{
		Vacancy: normalize.CanonicalVacancy{
			Source:           "hh",
			ExternalID:       "900002",
			Title:            "Fixture vacancy",
			RegionExternalID: "2",
			CollectedAt:      time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
			IsActive:         true,
			ContentHash:      "01",
		},
		RegionName: "Санкт-Петербург",
	})

	require.NoError(t, err)
	require.False(t, changed)
	require.Contains(t, db.updateSQL, "COALESCE($8, is_remote)")
	require.Len(t, db.updateArgs, 8)
	regionID, ok := db.updateArgs[3].(*string)
	require.True(t, ok)
	require.Equal(t, "00000000-0000-0000-0000-000000000002", *regionID)
}
