//go:build integration

package assistant

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
)

func TestPostgresPreferencesRoundTripAcrossConnections(t *testing.T) {
	_ = godotenv.Load("../../../.env")
	databaseURL := os.Getenv("ASSISTANT_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("ASSISTANT_TEST_DATABASE_URL or DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn1, err := pgx.Connect(ctx, databaseURL)
	require.NoError(t, err)
	defer conn1.Close(context.Background())
	conn2, err := pgx.Connect(ctx, databaseURL)
	require.NoError(t, err)
	defer conn2.Close(context.Background())

	subject := fmt.Sprintf("assistant-integration-%d", time.Now().UnixNano())
	repo1 := NewPostgresRepository(conn1)
	userID, err := repo1.EnsureUser(ctx, subject)
	require.NoError(t, err)
	defer func() {
		_, _ = conn1.Exec(context.Background(), `DELETE FROM assistant_users WHERE id = $1::uuid`, userID)
	}()

	want := PreferenceRecord{
		Note:         "synthetic integration profile",
		HardCriteria: map[string]any{"approved_roles": []any{"96"}},
		SoftCriteria: map[string]any{},
		Weights:      map[string]float64{"salary": 1},
	}
	saved, err := repo1.SavePreferences(ctx, userID, "integration-request", want)
	require.NoError(t, err)
	require.Equal(t, 1, saved.Version)
	require.NotEmpty(t, saved.ID)

	loaded, err := NewPostgresRepository(conn2).CurrentPreferences(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, saved.Version, loaded.Version)
	require.Equal(t, want.Note, loaded.Note)
	require.Equal(t, want.HardCriteria, loaded.HardCriteria)
	require.Equal(t, want.SoftCriteria, loaded.SoftCriteria)
	require.Equal(t, want.Weights, loaded.Weights)
}

func TestPostgresLegacyPreferenceCreatesNormalizedImmutableVersion(t *testing.T) {
	_ = godotenv.Load("../../../.env")
	databaseURL := os.Getenv("ASSISTANT_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("ASSISTANT_TEST_DATABASE_URL or DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	require.NoError(t, err)
	defer conn.Close(context.Background())

	repo := NewPostgresRepository(conn)
	userID, err := repo.EnsureUser(ctx, fmt.Sprintf("assistant-legacy-%d", time.Now().UnixNano()))
	require.NoError(t, err)
	defer func() {
		_, _ = conn.Exec(context.Background(), `DELETE FROM assistant_users WHERE id = $1::uuid`, userID)
	}()
	var oldID string
	require.NoError(t, conn.QueryRow(ctx, `INSERT INTO vacancy_preferences
		(user_id, version, note, hard_criteria, soft_criteria, weights)
		VALUES ($1::uuid, 1, 'synthetic legacy profile', '{"role":"backend"}', '{}', '{}')
		RETURNING id::text`, userID).Scan(&oldID))

	current, err := repo.CurrentPreferences(ctx, userID)
	require.NoError(t, err)
	require.True(t, current.LegacyRoleUpgraded)
	require.Equal(t, []any{"96"}, current.HardCriteria["approved_roles"])

	saved, err := repo.SavePreferences(ctx, userID, "legacy-normalize", current)
	require.NoError(t, err)
	require.Equal(t, 2, saved.Version)
	require.NotContains(t, saved.HardCriteria, "role")

	var oldHard, newHard string
	require.NoError(t, conn.QueryRow(ctx, `SELECT hard_criteria::text
		FROM vacancy_preferences WHERE id = $1::uuid`, oldID).Scan(&oldHard))
	require.NoError(t, conn.QueryRow(ctx, `SELECT hard_criteria::text
		FROM vacancy_preferences WHERE id = $1::uuid`, saved.ID).Scan(&newHard))
	require.Contains(t, oldHard, `"role"`)
	require.NotContains(t, oldHard, `"approved_roles"`)
	require.NotContains(t, newHard, `"role"`)
	require.Contains(t, newHard, `"approved_roles"`)
}

func TestPostgresManualSnapshotScansEligibleVacanciesAndDeduplicatesOutbox(t *testing.T) {
	_ = godotenv.Load("../../../.env")
	databaseURL := os.Getenv("ASSISTANT_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("ASSISTANT_TEST_DATABASE_URL or DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()
	repo := NewPostgresRepository(pool)
	var baselineEligible int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM vacancies v
		JOIN sources s ON s.code=v.source AND s.is_active
		WHERE v.is_active AND v.deleted_at IS NULL`).Scan(&baselineEligible))

	suffix := time.Now().UnixNano()
	activeSource := fmt.Sprintf("assistant-test-active-%d", suffix)
	inactiveSource := fmt.Sprintf("assistant-test-inactive-%d", suffix)
	_, err = pool.Exec(ctx, `INSERT INTO sources (code, name, is_active)
		VALUES ($1, 'Synthetic active', true), ($2, 'Synthetic inactive', false)`,
		activeSource, inactiveSource)
	require.NoError(t, err)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM assistant_work_items WHERE source IN ($1, $2)`,
			activeSource, inactiveSource)
		_, _ = pool.Exec(context.Background(), `DELETE FROM vacancies WHERE source IN ($1, $2)`,
			activeSource, inactiveSource)
		_, _ = pool.Exec(context.Background(), `DELETE FROM sources WHERE code IN ($1, $2)`,
			activeSource, inactiveSource)
	}()

	userID, err := repo.EnsureUser(ctx, fmt.Sprintf("assistant-snapshot-%d", suffix))
	require.NoError(t, err)
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM assistant_users WHERE id = $1::uuid`, userID)
	}()
	_, err = repo.SavePreferences(ctx, userID, "snapshot-preference", PreferenceRecord{
		Note: "synthetic snapshot profile", HardCriteria: map[string]any{},
		SoftCriteria: map[string]any{}, Weights: map[string]float64{},
	})
	require.NoError(t, err)

	before := time.Now().UTC().Add(-time.Minute)
	_, err = pool.Exec(ctx, `
		INSERT INTO vacancies (source, external_id, title, collected_at, created_at, is_active, deleted_at)
		VALUES
			($1, 'eligible', 'Synthetic eligible vacancy', $3, $3, true, NULL),
			($1, 'inactive', 'Synthetic inactive vacancy', $3, $3, false, NULL),
			($1, 'deleted', 'Synthetic deleted vacancy', $3, $3, true, $3),
			($2, 'source-disabled', 'Synthetic disabled-source vacancy', $3, $3, true, NULL)
	`, activeSource, inactiveSource, before)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO assistant_work_items (source, external_id)
		VALUES ($1, 'eligible')`, activeSource)
	require.NoError(t, err)

	runID, err := repo.QueueAnalysis(ctx, userID, "snapshot-run", false)
	require.NoError(t, err)
	require.NotEmpty(t, runID)
	replayedRunID, err := repo.QueueAnalysis(ctx, userID, "snapshot-run", false)
	require.NoError(t, err)
	require.Equal(t, runID, replayedRunID)
	_, err = repo.QueueAnalysis(ctx, userID, "another-run", false)
	require.Error(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO vacancies
		(source, external_id, title, collected_at, created_at, is_active)
		VALUES ($1, 'later', 'Synthetic later vacancy', now(), now(), true)`, activeSource)
	require.NoError(t, err)

	stats, err := RunOnce(ctx, repo, WorkerOptions{BatchSize: 25, Now: time.Now().UTC()})
	require.NoError(t, err)
	require.Equal(t, baselineEligible+1, stats.Processed)
	require.Equal(t, baselineEligible+1, stats.Matched)

	status, err := repo.AnalysisStatus(ctx, userID, false)
	require.NoError(t, err)
	require.Equal(t, "succeeded", status.State)
	require.Equal(t, baselineEligible+1, status.Total)
	require.Equal(t, baselineEligible+1, status.Processed)

	incremental, err := RunOnce(ctx, repo, WorkerOptions{
		Source: activeSource, Cutoff: before.Add(-time.Minute), BatchSize: 1, Now: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, incremental.Processed)
	var resultCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_match_results
		WHERE user_id=$1::uuid`, userID).Scan(&resultCount))
	require.Equal(t, baselineEligible+1, resultCount)
	var workStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM assistant_work_items
		WHERE source=$1 AND external_id='eligible'`, activeSource).Scan(&workStatus))
	require.Equal(t, "done", workStatus)
}
