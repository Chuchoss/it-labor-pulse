//go:build integration

package assistant

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func TestSafeCurrentFrontendAudit(t *testing.T) {
	if os.Getenv("ASSISTANT_SAFE_AUDIT") != "1" {
		t.Skip("safe audit not requested")
	}
	_ = godotenv.Load("../../../.env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := NewPostgresRepository(pool)

	var userID, preferenceID string
	if err := pool.QueryRow(ctx, `
		SELECT u.id::text, p.id::text
		FROM assistant_users u
		JOIN LATERAL (
			SELECT id FROM vacancy_preferences
			WHERE user_id=u.id AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		) p ON true
		WHERE u.external_subject='local-dev-user'
	`).Scan(&userID, &preferenceID); err != nil {
		t.Fatal(err)
	}
	preference, err := repo.CurrentPreferences(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	preference.HardCriteria["specialization"] = "frontend"
	preference.HardCriteria["include_leadership"] = false
	p := toPreferences(preference)

	run := AssistantRun{
		UserID: userID, PreferenceID: preferenceID,
		SnapshotCutoff: time.Now().UTC().Add(time.Hour),
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	classifications := map[string]Classification{}
	for {
		candidates, err := repo.SnapshotCandidates(ctx, run, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(candidates) == 0 {
			break
		}
		for _, candidate := range candidates {
			if seen[candidate.ID] {
				continue
			}
			seen[candidate.ID] = true
			classification := ClassifyVacancy(candidate.Vacancy)
			classifications[candidate.ID] = classification
			counts["derived_"+string(classification.Specialization)]++
			if classification.Leadership {
				counts["derived_lead"]++
			}
			result := Match(candidate.Vacancy, p, time.Now().UTC())
			counts["decision_"+string(result.Decision)]++
			for _, conflict := range result.Conflicts {
				switch conflict {
				case "specialization:backend":
					counts["excluded_backend"]++
				case "specialization:fullstack":
					counts["excluded_fullstack"]++
				case "leadership_excluded":
					counts["excluded_lead"]++
				}
			}
		}
		last := candidates[len(candidates)-1]
		run.CursorCreatedAt = &last.CreatedAt
		run.CursorVacancyID = last.ID
	}

	var role96, role96And104, deterministicMatches, aiMatches, aiRejectedOverDeterministic int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(DISTINCT v.id) FILTER (WHERE EXISTS (
				SELECT 1 FROM vacancy_role_scopes x JOIN role_aliases a ON a.role_id=x.role_id
				WHERE x.vacancy_id=v.id AND a.source='hh' AND a.pattern='96'
			)),
			count(DISTINCT v.id) FILTER (WHERE EXISTS (
				SELECT 1 FROM vacancy_role_scopes x JOIN role_aliases a ON a.role_id=x.role_id
				WHERE x.vacancy_id=v.id AND a.source='hh' AND a.pattern='96'
			) AND EXISTS (
				SELECT 1 FROM vacancy_role_scopes x JOIN role_aliases a ON a.role_id=x.role_id
				WHERE x.vacancy_id=v.id AND a.source='hh' AND a.pattern='104'
			))
		FROM vacancies v WHERE v.is_active AND v.deleted_at IS NULL
	`).Scan(&role96, &role96And104); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		)
		SELECT
			count(*) FILTER (WHERE method='deterministic' AND decision='match'),
			count(*) FILTER (WHERE method='ai' AND decision='match'),
			count(*) FILTER (WHERE method='ai' AND decision IN ('reject','review') AND EXISTS (
				SELECT 1 FROM vacancy_match_results d
				WHERE d.user_id=m.user_id AND d.preference_id=m.preference_id
				  AND d.vacancy_id=m.vacancy_id AND d.vacancy_revision=m.vacancy_revision
				  AND d.method='deterministic' AND d.decision='match'
			))
		FROM vacancy_match_results m
		WHERE preference_id=(SELECT id FROM current_preference)
	`, userID).Scan(&deterministicMatches, &aiMatches, &aiRejectedOverDeterministic); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		)
		SELECT vacancy_id::text, decision, method
		FROM vacancy_match_results
		WHERE preference_id=(SELECT id FROM current_preference)
		  AND decision IN ('match','review')
	`, userID)
	if err != nil {
		t.Fatal(err)
	}
	resultCounts := map[string]int{}
	for rows.Next() {
		var vacancyID, decision, method string
		if err := rows.Scan(&vacancyID, &decision, &method); err != nil {
			t.Fatal(err)
		}
		classification := classifications[vacancyID]
		resultCounts[decision+"_"+method+"_"+string(classification.Specialization)]++
		if classification.Leadership {
			resultCounts[decision+"_"+method+"_lead"]++
		} else {
			resultCounts[decision+"_"+method+"_nonlead"]++
		}
	}
	rows.Close()
	t.Logf("SAFE_COUNTS decisions match=%d review=%d reject=%d excluded_backend=%d excluded_fullstack=%d excluded_lead=%d",
		counts["decision_match"], counts["decision_review"], counts["decision_reject"],
		counts["excluded_backend"], counts["excluded_fullstack"], counts["excluded_lead"])
	t.Logf("SAFE_TAXONOMY frontend=%d backend=%d fullstack=%d mobile=%d other=%d unknown=%d lead=%d role96=%d role96_and_104=%d",
		counts["derived_frontend"], counts["derived_backend"], counts["derived_fullstack"],
		counts["derived_mobile"], counts["derived_other"], counts["derived_unknown"],
		counts["derived_lead"], role96, role96And104)
	t.Logf("SAFE_OLD_RESULTS deterministic_match=%d ai_match=%d ai_reject_or_review_over_deterministic=%d",
		deterministicMatches, aiMatches, aiRejectedOverDeterministic)
	t.Logf("SAFE_RESULT_TAXONOMY match_det_frontend=%d match_det_backend=%d match_det_fullstack=%d match_det_unknown=%d match_det_lead=%d match_det_nonlead=%d match_ai_frontend=%d match_ai_backend=%d match_ai_fullstack=%d match_ai_unknown=%d match_ai_lead=%d match_ai_nonlead=%d review_ai_frontend=%d review_ai_backend=%d review_ai_fullstack=%d review_ai_unknown=%d review_ai_lead=%d review_ai_nonlead=%d",
		resultCounts["match_deterministic_frontend"], resultCounts["match_deterministic_backend"],
		resultCounts["match_deterministic_fullstack"], resultCounts["match_deterministic_unknown"],
		resultCounts["match_deterministic_lead"], resultCounts["match_deterministic_nonlead"],
		resultCounts["match_ai_frontend"], resultCounts["match_ai_backend"], resultCounts["match_ai_fullstack"],
		resultCounts["match_ai_unknown"], resultCounts["match_ai_lead"], resultCounts["match_ai_nonlead"],
		resultCounts["review_ai_frontend"], resultCounts["review_ai_backend"], resultCounts["review_ai_fullstack"],
		resultCounts["review_ai_unknown"], resultCounts["review_ai_lead"], resultCounts["review_ai_nonlead"])

	legacyRows, err := pool.Query(ctx, `
		SELECT vacancy_id::text, method
		FROM vacancy_match_results
		WHERE user_id=$1::uuid AND decision='match'
		ORDER BY created_at DESC LIMIT 100
	`, userID)
	if err != nil {
		t.Fatal(err)
	}
	legacyVisible := map[string]int{}
	for legacyRows.Next() {
		var vacancyID, method string
		if err := legacyRows.Scan(&vacancyID, &method); err != nil {
			t.Fatal(err)
		}
		classification := classifications[vacancyID]
		legacyVisible[method+"_"+string(classification.Specialization)]++
		if classification.Leadership {
			legacyVisible[method+"_lead"]++
		} else {
			legacyVisible[method+"_nonlead"]++
		}
	}
	legacyRows.Close()
	t.Logf("SAFE_OLD_VISIBLE det_frontend=%d det_backend=%d det_fullstack=%d det_other=%d det_unknown=%d det_lead=%d det_nonlead=%d ai_frontend=%d ai_backend=%d ai_fullstack=%d ai_other=%d ai_unknown=%d ai_lead=%d ai_nonlead=%d",
		legacyVisible["deterministic_frontend"], legacyVisible["deterministic_backend"],
		legacyVisible["deterministic_fullstack"], legacyVisible["deterministic_other"],
		legacyVisible["deterministic_unknown"], legacyVisible["deterministic_lead"],
		legacyVisible["deterministic_nonlead"], legacyVisible["ai_frontend"],
		legacyVisible["ai_backend"], legacyVisible["ai_fullstack"], legacyVisible["ai_other"],
		legacyVisible["ai_unknown"], legacyVisible["ai_lead"], legacyVisible["ai_nonlead"])
}
