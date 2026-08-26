//go:build integration

package worker

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
)

func TestDailySnapshotFromCompleteSyntheticCycle(t *testing.T) {
	_ = godotenv.Load("../../../.env")
	databaseURL := os.Getenv("ANALYTICS_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("ANALYTICS_TEST_DATABASE_URL or DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	w, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(w.Close)

	suffix := time.Now().UTC().UnixNano()
	source := fmt.Sprintf("analytics-test-%d", suffix)
	var roleID, regionID string
	_, err = w.pool.Exec(ctx, `INSERT INTO sources (code, name) VALUES ($1, 'Synthetic analytics source')`, source)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		_, _ = w.pool.Exec(cleanupCtx, `DELETE FROM vacancy_demand_weekly WHERE source = $1`, source)
		_, _ = w.pool.Exec(cleanupCtx, `DELETE FROM vacancy_demand_daily WHERE source = $1`, source)
		_, _ = w.pool.Exec(cleanupCtx, `DELETE FROM analytics_runs WHERE source = $1`, source)
		_, _ = w.pool.Exec(cleanupCtx, `DELETE FROM ingest_cycles WHERE source = $1`, source)
		_, _ = w.pool.Exec(cleanupCtx, `DELETE FROM region_external_ids WHERE source = $1`, source)
		_, _ = w.pool.Exec(cleanupCtx, `DELETE FROM sources WHERE code = $1`, source)
		if roleID != "" {
			_, _ = w.pool.Exec(cleanupCtx, `DELETE FROM roles WHERE id = $1::uuid`, roleID)
		}
		if regionID != "" {
			_, _ = w.pool.Exec(cleanupCtx, `DELETE FROM regions WHERE id = $1::uuid`, regionID)
		}
	})

	err = w.pool.QueryRow(ctx, `
		INSERT INTO roles (slug, title, family)
		VALUES ($1, 'Synthetic analytics role', 'software_development')
		RETURNING id::text
	`, fmt.Sprintf("analytics-role-%d", suffix)).Scan(&roleID)
	require.NoError(t, err)
	err = w.pool.QueryRow(ctx, `
		INSERT INTO regions (code, name)
		VALUES ($1, 'Synthetic analytics region')
		RETURNING id::text
	`, fmt.Sprintf("analytics-region-%d", suffix)).Scan(&regionID)
	require.NoError(t, err)
	_, err = w.pool.Exec(ctx, `
		INSERT INTO region_external_ids (source, external_id, region_id)
		VALUES ($1, 'synthetic-area', $2::uuid)
	`, source, regionID)
	require.NoError(t, err)

	cycleDate := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	cycleEnd := cycleDate.AddDate(0, 0, 1)
	started := cycleEnd.Add(-time.Hour)
	completed := cycleEnd.Add(time.Hour)
	var cycleID string
	err = w.pool.QueryRow(ctx, `
		INSERT INTO ingest_cycles (
			source, scope, scope_hash, cycle_end, status, partition_count,
			completed_partitions, started_at, completed_at, cycle_date, cutoff_at,
			expected_pages, completed_pages, method_version
		) VALUES (
			$1, 'daily_discovery', repeat('b', 64), $2, 'complete', 1, 1, $3, $4,
			$5, $2, 1, 1, $6
		)
		RETURNING id::text
	`, source, cycleEnd, started, completed, cycleDate, MethodVersion).Scan(&cycleID)
	require.NoError(t, err)
	_, err = w.pool.Exec(ctx, `
		INSERT INTO ingest_cycle_observations (
			cycle_id, source, external_id, published_at,
			external_region_id, primary_role_external_id, role_group,
			external_role_ids, salary_from, salary_to, salary_currency,
			salary_gross, salary_mid_rub_net, salary_eligible, observed_at
		) VALUES (
			$1::uuid, $2, 'synthetic-1', '2026-08-26T10:00:00Z',
			'synthetic-area', '96', 'software_development',
			ARRAY['96'], 100000, 200000, 'RUB', false, 150000, true, $3
		)
	`, cycleID, source, cycleEnd)
	require.NoError(t, err)

	first, err := w.RunDaily(ctx, cycleID)
	require.NoError(t, err)
	require.EqualValues(t, 2, first.Rows)
	second, err := w.RunDaily(ctx, cycleID)
	require.NoError(t, err)
	require.EqualValues(t, first.Rows, second.Rows)

	var rows, active, published int
	err = w.pool.QueryRow(ctx, `
		SELECT count(*), max(active_count), max(published_count)
		FROM vacancy_demand_daily
		WHERE source = $1 AND snapshot_date = '2026-08-26'
		  AND method_version = $2
	`, source, MethodVersion).Scan(&rows, &active, &published)
	require.NoError(t, err)
	require.Equal(t, 2, rows)
	require.Equal(t, 1, active)
	require.Equal(t, 1, published)
}
