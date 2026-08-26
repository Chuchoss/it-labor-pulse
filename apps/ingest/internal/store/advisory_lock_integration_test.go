//go:build integration

package store

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerAdvisoryLockAcrossPools(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for integration test")
	}
	ctx := context.Background()
	first, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	defer first.Close()
	second, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	defer second.Close()

	lock1, acquired, err := first.TrySchedulerLock(ctx)
	require.NoError(t, err)
	require.True(t, acquired)

	_, acquired, err = second.TrySchedulerLock(ctx)
	require.NoError(t, err)
	require.False(t, acquired)

	require.NoError(t, lock1.Release(ctx))
	lock2, acquired, err := second.TrySchedulerLock(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, lock2.Release(ctx))
}
