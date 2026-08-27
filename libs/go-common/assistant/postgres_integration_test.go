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
		Note: "synthetic integration profile",
		HardCriteria: map[string]any{
			"approved_roles": []any{"96"}, "specialization": "frontend", "include_leadership": false,
		},
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

func TestPostgresListMatchesAppliesAIFinalPrecedence(t *testing.T) {
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID, err := repo.EnsureUser(ctx, "assistant-precedence-"+suffix)
	require.NoError(t, err)
	defer func() {
		_, _ = conn.Exec(context.Background(), `DELETE FROM assistant_users WHERE id=$1::uuid`, userID)
		_, _ = conn.Exec(context.Background(), `DELETE FROM vacancies WHERE source='hh' AND external_id LIKE $1`, "synthetic-precedence-"+suffix+"%")
	}()
	preference, err := repo.SavePreferences(ctx, userID, "precedence", PreferenceRecord{
		HardCriteria: map[string]any{"approved_roles": []any{"96"}, "specialization": "frontend"},
		SoftCriteria: map[string]any{}, Weights: map[string]float64{},
	})
	require.NoError(t, err)
	var runID string
	require.NoError(t, conn.QueryRow(ctx, `
		INSERT INTO assistant_runs
			(user_id, preference_id, state, snapshot_cutoff, ruleset_version)
		VALUES ($1::uuid, $2::uuid, 'succeeded', now(), $3)
		RETURNING id::text
	`, userID, preference.ID, SpecializationRulesVersion).Scan(&runID))
	vacancyIDs := make([]string, 4)
	for i := range vacancyIDs {
		require.NoError(t, conn.QueryRow(ctx, `
			INSERT INTO vacancies (source, external_id, title, collected_at, is_active)
			VALUES ('hh', $1, $2, now(), true) RETURNING id::text
		`, fmt.Sprintf("synthetic-precedence-%s-%d", suffix, i), fmt.Sprintf("Synthetic frontend %d", i)).
			Scan(&vacancyIDs[i]))
		decision := DecisionMatch
		unknowns := []string{}
		if i == 2 {
			decision = DecisionReview
			unknowns = []string{"remote"}
		}
		_, err = repo.SaveMatch(ctx, WorkerMatch{
			UserID: userID, VacancyID: vacancyIDs[i], PreferenceVersion: preference.Version,
			RunID: runID, VacancyRevision: 1, Result: Result{Decision: decision, Score: .8, Unknowns: unknowns},
		})
		require.NoError(t, err)
	}
	require.NoError(t, repo.SaveAIResult(ctx, WorkerMatch{
		UserID: userID, VacancyID: vacancyIDs[0], PreferenceVersion: preference.Version,
		RunID: runID, VacancyRevision: 1, Method: "ai", Provider: "fake", Model: "fake", PromptVersion: "test",
	}, MatchOutput{Decision: "reject", Score: .1, Confidence: "high"}))
	require.NoError(t, repo.SaveAIResult(ctx, WorkerMatch{
		UserID: userID, VacancyID: vacancyIDs[1], PreferenceVersion: preference.Version,
		RunID: runID, VacancyRevision: 1, Method: "ai", Provider: "fake", Model: "fake", PromptVersion: "test",
	}, MatchOutput{Decision: "match", Score: .9, Confidence: "high"}))

	matches, err := repo.ListMatches(ctx, userID, 10)
	require.NoError(t, err)
	require.Len(t, matches, 3)
	stages := map[string]string{}
	decisions := map[string]Decision{}
	for _, match := range matches {
		stages[match.VacancyID] = match.Stage
		decisions[match.VacancyID] = match.Decision
	}
	require.NotContains(t, stages, vacancyIDs[0])
	require.Equal(t, "confirmed", stages[vacancyIDs[1]])
	require.Equal(t, "preliminary", stages[vacancyIDs[2]])
	require.Equal(t, "preliminary", stages[vacancyIDs[3]])
	require.Equal(t, DecisionMatch, decisions[vacancyIDs[1]])
	require.Equal(t, DecisionReview, decisions[vacancyIDs[2]])
	require.Equal(t, DecisionMatch, decisions[vacancyIDs[3]])
}

func TestPostgresListMatchesKeepsEarlierSucceededRunAfterNewerRun(t *testing.T) {
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID, err := repo.EnsureUser(ctx, "assistant-stale-run-"+suffix)
	require.NoError(t, err)
	defer func() {
		_, _ = conn.Exec(context.Background(), `DELETE FROM assistant_users WHERE id=$1::uuid`, userID)
		_, _ = conn.Exec(context.Background(), `DELETE FROM vacancies WHERE source='hh' AND external_id LIKE $1`, "synthetic-stale-run-"+suffix+"%")
	}()
	preference, err := repo.SavePreferences(ctx, userID, "stale-run", PreferenceRecord{
		HardCriteria: map[string]any{"approved_roles": []any{"96"}},
		SoftCriteria: map[string]any{}, Weights: map[string]float64{},
	})
	require.NoError(t, err)
	var firstRun, secondRun, vacancyID string
	require.NoError(t, conn.QueryRow(ctx, `
		INSERT INTO assistant_runs
			(user_id, preference_id, state, snapshot_cutoff, ruleset_version, created_at)
		VALUES ($1::uuid, $2::uuid, 'succeeded', now(), $3, now() - interval '1 hour')
		RETURNING id::text
	`, userID, preference.ID, SpecializationRulesVersion).Scan(&firstRun))
	require.NoError(t, conn.QueryRow(ctx, `
		INSERT INTO vacancies (source, external_id, title, collected_at, is_active)
		VALUES ('hh', $1, 'Synthetic frontend review', now(), true) RETURNING id::text
	`, "synthetic-stale-run-"+suffix).Scan(&vacancyID))
	created, err := repo.SaveMatch(ctx, WorkerMatch{
		UserID: userID, VacancyID: vacancyID, PreferenceVersion: preference.Version,
		RunID: firstRun, VacancyRevision: 1,
		Result: Result{Decision: DecisionReview, Score: .4, Unknowns: []string{"specialization"}},
	})
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, conn.QueryRow(ctx, `
		INSERT INTO assistant_runs
			(user_id, preference_id, state, snapshot_cutoff, ruleset_version, created_at)
		VALUES ($1::uuid, $2::uuid, 'succeeded', now(), $3, now())
		RETURNING id::text
	`, userID, preference.ID, SpecializationRulesVersion).Scan(&secondRun))
	matches, err := repo.ListMatches(ctx, userID, 10)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.Equal(t, DecisionReview, matches[0].Decision)
	require.Equal(t, vacancyID, matches[0].VacancyID)

	rebound, err := repo.SaveMatch(ctx, WorkerMatch{
		UserID: userID, VacancyID: vacancyID, PreferenceVersion: preference.Version,
		RunID: secondRun, VacancyRevision: 1,
		Result: Result{Decision: DecisionReview, Score: .4, Unknowns: []string{"specialization"}},
	})
	require.NoError(t, err)
	require.False(t, rebound)
	var boundRun string
	require.NoError(t, conn.QueryRow(ctx, `
		SELECT run_id::text FROM vacancy_match_results
		WHERE user_id=$1::uuid AND vacancy_id=$2::uuid AND method='deterministic'
	`, userID, vacancyID).Scan(&boundRun))
	require.Equal(t, secondRun, boundRun)
	matches, err = repo.ListMatches(ctx, userID, 10)
	require.NoError(t, err)
	require.Len(t, matches, 1)
}

func TestPostgresAIResultExistsIsScopedToCurrentRuleset(t *testing.T) {
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userID, err := repo.EnsureUser(ctx, "assistant-ruleset-ai-"+suffix)
	require.NoError(t, err)
	defer func() {
		_, _ = conn.Exec(context.Background(), `DELETE FROM assistant_users WHERE id=$1::uuid`, userID)
		_, _ = conn.Exec(context.Background(), `DELETE FROM vacancies WHERE source='hh' AND external_id LIKE $1`, "synthetic-ruleset-ai-"+suffix+"%")
	}()
	preference, err := repo.SavePreferences(ctx, userID, "ruleset-ai", PreferenceRecord{
		HardCriteria: map[string]any{"approved_roles": []any{"96"}},
		SoftCriteria: map[string]any{}, Weights: map[string]float64{},
	})
	require.NoError(t, err)
	var vacancyID string
	require.NoError(t, conn.QueryRow(ctx, `
		INSERT INTO vacancies (source, external_id, title, collected_at, is_active)
		VALUES ('hh', $1, 'Synthetic frontend', now(), true) RETURNING id::text
	`, "synthetic-ruleset-ai-"+suffix).Scan(&vacancyID))
	_, err = conn.Exec(ctx, `
		INSERT INTO assistant_ai_jobs
			(user_id, preference_id, vacancy_id, vacancy_revision, status, provider, attempts, finished_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 1, 'complete', 'deepseek', 1, now())
	`, userID, preference.ID, vacancyID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `
		INSERT INTO vacancy_match_results
			(user_id, preference_id, vacancy_id, decision, method, score, rationale,
			 evidence_ids, conflicts, unknowns, provider, prompt_version, vacancy_revision, ruleset_version)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'review', 'ai', 0.4, 'legacy',
			'[]'::jsonb, '[]'::jsonb, '[]'::jsonb, 'deepseek', 'batch-v5-hard-gates', 1, 'assistant-hard-gates-v2')
	`, userID, preference.ID, vacancyID)
	require.NoError(t, err)
	exists, err := repo.AIResultExists(ctx, userID, preference.Version, vacancyID, 1)
	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, repo.SaveAIResult(ctx, WorkerMatch{
		UserID: userID, VacancyID: vacancyID, PreferenceVersion: preference.Version,
		VacancyRevision: 1, Method: "ai", Provider: "fake", Model: "fake", PromptVersion: "batch-v7-id-list",
	}, MatchOutput{Decision: "review", Score: .45, Confidence: "medium"}))
	exists, err = repo.AIResultExists(ctx, userID, preference.Version, vacancyID, 1)
	require.NoError(t, err)
	require.True(t, exists)
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
	require.LessOrEqual(t, stats.Matched, stats.Processed)
	var syntheticMatched bool
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM vacancy_match_results m
			JOIN vacancies v ON v.id=m.vacancy_id
			WHERE m.user_id=$1::uuid AND v.source=$2 AND v.external_id='eligible'
			  AND m.method='deterministic' AND m.decision='match'
		)
	`, userID, activeSource).Scan(&syntheticMatched))
	require.True(t, syntheticMatched)

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
	require.Equal(t, stats.Processed, resultCount)
	var workStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM assistant_work_items
		WHERE source=$1 AND external_id='eligible'`, activeSource).Scan(&workStatus))
	require.Equal(t, "done", workStatus)
}
