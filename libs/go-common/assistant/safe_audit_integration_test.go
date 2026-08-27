//go:build integration

package assistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
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
	citedCandidates := map[string]WorkerCandidate{}
	citedHashes := map[string]string{
		"bfd8a96417e13d39ed234f382391b4338503900274f5108d8a6ef88e38b52d6b": "cited_technical_director",
		"9e23df3bdf8ee9baa1a27a599deb9a0d48dc8ad5f351fb61fb69df244b4cc3d6": "cited_lead_frontend",
	}
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
			sum := sha256.Sum256([]byte(candidate.Title))
			if label, ok := citedHashes[hex.EncodeToString(sum[:])]; ok {
				citedCandidates[label] = candidate
			}
			classification := ClassifyVacancy(candidate.Vacancy)
			classifications[candidate.ID] = classification
			counts["derived_"+string(classification.Specialization)]++
			if len(candidate.Vacancy.RoleIDs) == 0 {
				counts["role_unknown"]++
			} else if overlaps(p.ApprovedRoles, candidate.Vacancy.RoleIDs) {
				counts["role_pass"]++
			} else {
				counts["role_reject"]++
			}
			switch classification.Specialization {
			case SpecializationFrontend:
				counts["frontend_match"]++
			case SpecializationUnknown:
				counts["frontend_review"]++
			default:
				counts["frontend_reject"]++
			}
			if classification.Leadership {
				counts["derived_lead"]++
			}
			if candidate.Vacancy.IsRemote == nil {
				counts["remote_unknown"]++
			} else if *candidate.Vacancy.IsRemote {
				counts["remote_true"]++
			} else {
				counts["remote_false"]++
			}
			skillMap := make(map[string]bool, len(candidate.Vacancy.Skills))
			for _, skill := range candidate.Vacancy.Skills {
				skillMap[normalizeSkill(skill)] = true
			}
			react := hasExplicitSkill(candidate.Vacancy, "react", skillMap)
			contentMissing := strings.TrimSpace(candidate.Vacancy.Title) == "" &&
				len(candidate.Vacancy.Skills) == 0 && strings.TrimSpace(candidate.Vacancy.Description) == ""
			if react {
				counts["react_explicit"]++
			} else if contentMissing {
				counts["react_unknown"]++
			} else {
				counts["react_missing"]++
			}
			rolePass := len(p.ApprovedRoles) == 0 || len(candidate.Vacancy.RoleIDs) == 0 ||
				overlaps(p.ApprovedRoles, candidate.Vacancy.RoleIDs)
			frontendPlausible := classification.Specialization == SpecializationFrontend ||
				classification.Specialization == SpecializationUnknown
			remotePlausible := candidate.Vacancy.IsRemote == nil || *candidate.Vacancy.IsRemote
			if rolePass && frontendPlausible && !classification.Leadership && remotePlausible && (react || contentMissing) {
				if classification.Specialization == SpecializationFrontend && candidate.Vacancy.IsRemote != nil &&
					*candidate.Vacancy.IsRemote && react {
					counts["plausible_confirmed"]++
				} else {
					counts["plausible_review"]++
				}
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
	t.Logf("SAFE_HARD_GATES role_pass=%d role_unknown=%d role_reject=%d frontend_match=%d frontend_review=%d frontend_reject=%d leadership_excluded=%d remote_true=%d remote_false=%d remote_unknown=%d react_explicit=%d react_missing=%d react_unknown=%d plausible_confirmed=%d plausible_review=%d",
		counts["role_pass"], counts["role_unknown"], counts["role_reject"],
		counts["frontend_match"], counts["frontend_review"], counts["frontend_reject"],
		counts["derived_lead"], counts["remote_true"], counts["remote_false"], counts["remote_unknown"],
		counts["react_explicit"], counts["react_missing"], counts["react_unknown"],
		counts["plausible_confirmed"], counts["plausible_review"])
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
	for _, label := range []string{"cited_technical_director", "cited_lead_frontend"} {
		candidate, ok := citedCandidates[label]
		if !ok {
			t.Logf("SAFE_CITED label=%s found=false", label)
			continue
		}
		classification := ClassifyVacancy(candidate.Vacancy)
		result := Match(candidate.Vacancy, p, time.Now().UTC())
		skillMap := map[string]bool{}
		for _, skill := range candidate.Vacancy.Skills {
			skillMap[normalizeSkill(skill)] = true
		}
		remote := "unknown"
		if candidate.Vacancy.IsRemote != nil {
			remote = strconv.FormatBool(*candidate.Vacancy.IsRemote)
		}
		var aiDecision, aiConfidence, promptVersion string
		var resultRunID *string
		_ = pool.QueryRow(ctx, `
			SELECT decision, COALESCE(confidence, ''), COALESCE(prompt_version, ''), run_id::text
			FROM vacancy_match_results
			WHERE user_id=$1::uuid AND preference_id=$2::uuid AND vacancy_id=$3::uuid AND method='ai'
			ORDER BY created_at DESC LIMIT 1
		`, userID, preferenceID, candidate.ID).Scan(&aiDecision, &aiConfidence, &promptVersion, &resultRunID)
		t.Logf("SAFE_CITED label=%s found=true preference_version=%d result_run_linked=%t specialization=%s leadership=%t remote=%s react_explicit=%t deterministic=%s conflicts=%v unknowns=%v ai_decision=%s ai_confidence=%s prompt_version=%s",
			label, preference.Version, resultRunID != nil, classification.Specialization,
			classification.Leadership, remote, hasExplicitSkill(candidate.Vacancy, "react", skillMap),
			result.Decision, result.Conflicts, result.Unknowns, aiDecision, aiConfidence, promptVersion)
	}
}

func TestSafeRunDecisionAudit(t *testing.T) {
	runID := os.Getenv("ASSISTANT_SAFE_AUDIT_RUN_ID")
	if os.Getenv("ASSISTANT_SAFE_AUDIT") != "1" || runID == "" {
		t.Skip("safe run audit not requested")
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

	var run AssistantRun
	var state string
	var cursorAt *time.Time
	var specialization string
	var includeLeadership, remoteOnly, notePresent bool
	var approvedRoles, regions, requiredSkills, excludedSkills int
	err = pool.QueryRow(ctx, `
		SELECT r.id::text, r.user_id::text, r.preference_id::text, r.snapshot_cutoff,
		       r.snapshot_total, r.processed, r.snapshot_cursor_created_at,
		       COALESCE(r.snapshot_cursor_vacancy_id::text, ''), r.state,
		       COALESCE(p.hard_criteria->>'specialization', ''),
		       COALESCE((p.hard_criteria->>'include_leadership')::boolean, false),
		       COALESCE((p.hard_criteria->>'remote_only')::boolean, false),
		       NULLIF(BTRIM(p.note), '') IS NOT NULL,
		       COALESCE(jsonb_array_length(p.hard_criteria->'approved_roles'), 0),
		       COALESCE(jsonb_array_length(p.hard_criteria->'regions'), 0),
		       COALESCE(jsonb_array_length(p.hard_criteria->'required_skills'), 0),
		       COALESCE(jsonb_array_length(p.hard_criteria->'excluded_skills'), 0)
		FROM assistant_runs r
		JOIN vacancy_preferences p ON p.id=r.preference_id
		WHERE r.id=$1::uuid
	`, runID).Scan(&run.ID, &run.UserID, &run.PreferenceID, &run.SnapshotCutoff,
		&run.Total, &run.Processed, &cursorAt, &run.CursorVacancyID, &state,
		&specialization, &includeLeadership, &remoteOnly, &notePresent,
		&approvedRoles, &regions, &requiredSkills, &excludedSkills)
	if err != nil {
		t.Fatal(err)
	}
	run.CursorCreatedAt = cursorAt
	users, err := repo.UsersForAssistantRun(ctx, run)
	if err != nil || len(users) != 1 {
		t.Fatalf("load snapshotted preference: users=%d err=%v", len(users), err)
	}
	t.Logf("SAFE_RUN state=%s processed=%d total=%d specialization=%s include_leadership=%t remote_only=%t note_present=%t approved_roles=%d regions=%d required_skills=%d excluded_skills=%d",
		state, run.Processed, run.Total, specialization, includeLeadership, remoteOnly, notePresent,
		approvedRoles, regions, requiredSkills, excludedSkills)

	auditRun := run
	auditRun.CursorCreatedAt = nil
	auditRun.CursorVacancyID = ""
	p := toPreferences(users[0].Preference)
	counts := map[string]int{}
	done := false
	for !done {
		candidates, candidateErr := repo.SnapshotCandidates(ctx, auditRun, 100)
		if candidateErr != nil {
			t.Fatal(candidateErr)
		}
		if len(candidates) == 0 {
			break
		}
		for _, candidate := range candidates {
			if cursorAt != nil && (candidate.CreatedAt.After(*cursorAt) ||
				(candidate.CreatedAt.Equal(*cursorAt) && strings.Compare(candidate.ID, run.CursorVacancyID) > 0)) {
				done = true
				break
			}
			result := Match(candidate.Vacancy, p, time.Now().UTC())
			counts["decision_"+string(result.Decision)]++
			for _, conflict := range result.Conflicts {
				switch {
				case conflict == "role":
					counts["reason_role"]++
				case strings.HasPrefix(conflict, "specialization:"):
					counts["reason_specialization"]++
				case conflict == "leadership_excluded":
					counts["reason_leadership"]++
				case conflict == "remote_only":
					counts["reason_remote"]++
				case conflict == "minimum_salary":
					counts["reason_salary"]++
				case strings.HasPrefix(conflict, "excluded_skill:"):
					counts["reason_skills"]++
				case conflict == "region":
					counts["reason_region"]++
				}
			}
		}
		last := candidates[len(candidates)-1]
		auditRun.CursorCreatedAt = &last.CreatedAt
		auditRun.CursorVacancyID = last.ID
	}
	t.Logf("SAFE_DETERMINISTIC match=%d review=%d reject=%d role=%d specialization=%d leadership=%d remote=%d salary=%d skills=%d region=%d",
		counts["decision_match"], counts["decision_review"], counts["decision_reject"],
		counts["reason_role"], counts["reason_specialization"], counts["reason_leadership"],
		counts["reason_remote"], counts["reason_salary"], counts["reason_skills"], counts["reason_region"])

	var aiMatch, aiReview, aiReject, parsedResults, completeJobs, failedJobs, invalidJobs int
	err = pool.QueryRow(ctx, `
		WITH scope AS (
			SELECT v.id, v.analysis_revision
			FROM vacancies v
			WHERE v.created_at <= $2
			  AND ($3::timestamptz IS NULL OR (v.created_at, v.id) <= ($3, NULLIF($4, '')::uuid))
		), results AS (
			SELECT m.decision
			FROM vacancy_match_results m JOIN scope s
			  ON s.id=m.vacancy_id AND s.analysis_revision=m.vacancy_revision
			WHERE m.preference_id=$1::uuid AND m.method='ai'
		), jobs AS (
			SELECT j.status, j.error_code
			FROM assistant_ai_jobs j JOIN scope s
			  ON s.id=j.vacancy_id AND s.analysis_revision=j.vacancy_revision
			WHERE j.preference_id=$1::uuid
		)
		SELECT
			count(*) FILTER (WHERE decision='match'),
			count(*) FILTER (WHERE decision='review'),
			count(*) FILTER (WHERE decision='reject'),
			count(*),
			(SELECT count(*) FROM jobs WHERE status='complete'),
			(SELECT count(*) FROM jobs WHERE status='failed'),
			(SELECT count(*) FROM jobs WHERE status='failed' AND error_code='invalid_response')
		FROM results
	`, run.PreferenceID, run.SnapshotCutoff, cursorAt, run.CursorVacancyID).
		Scan(&aiMatch, &aiReview, &aiReject, &parsedResults, &completeJobs, &failedJobs, &invalidJobs)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_AI parsed_match=%d parsed_review=%d parsed_reject=%d parsed_total=%d provider_complete_jobs=%d final_failed_jobs=%d invalid_jobs=%d missing_parsed_for_complete=%d",
		aiMatch, aiReview, aiReject, parsedResults, completeJobs, failedJobs, invalidJobs, completeJobs-parsedResults)
}

func TestControlledLiveSemanticFixtures(t *testing.T) {
	if os.Getenv("ASSISTANT_LIVE_FIXTURES") != "1" {
		t.Skip("controlled live fixtures not requested")
	}
	_ = godotenv.Load("../../../.env")
	cfg := LoadConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	provider, err := NewDeepSeek(DeepSeekConfig{
		APIKey: cfg.DeepSeekAPIKey, BaseURL: cfg.DeepSeekBaseURL, Model: cfg.DeepSeekModel,
		Timeout: cfg.Timeout, MaxTokens: 5000, MaxAttempts: 1, MaxBatchSize: 5,
		MaxConcurrency: 1, InputTokenBudget: 30000,
	}, nil)
	if err != nil {
		t.Fatal("provider configuration unavailable")
	}
	evidence := map[string]bool{"vacancy:title": true, "vacancy:description": true, "preferences": true}
	fixtures := []struct {
		id, title, description, want string
	}{
		{"frontend-ic-remote", "Frontend Developer", "Individual contributor. React and TypeScript. Fully remote.", "match"},
		{"backend", "Backend Developer", "Server-side Go and PostgreSQL. Fully remote.", "reject"},
		{"fullstack", "Fullstack Developer", "Frontend and backend ownership. React and Go. Fully remote.", "reject"},
		{"lead", "Frontend Team Lead", "Leads a frontend team. React and TypeScript. Fully remote.", "reject"},
		{"frontend-ambiguous", "Software Developer", "Web product development; exact specialization and remote format are not stated.", "review"},
	}
	items := make([]BatchItem, 0, len(fixtures))
	for _, fixture := range fixtures {
		items = append(items, BatchItem{
			ID: fixture.id,
			InputSnapshot: MinimizedInput(fixture.title, fixture.description, map[string]string{
				"requested_specialization": "frontend",
				"include_leadership":       "false",
			}, evidence),
			Evidence: evidence,
		})
	}
	result, err := provider.CompleteBatchDetailed(ctx, BatchRequest{
		SharedPreferences: `{"hard_criteria":{"approved_roles":["96"],"specialization":"frontend","include_leadership":false,"remote_only":true,"required_skills":["react"]}}`,
		Items:             items,
	})
	if err != nil {
		t.Fatalf("controlled live batch failed: category=%s", providerErrorCategory(err))
	}
	if result.Stats.HTTPAttempts != 1 {
		t.Fatalf("controlled live batch used %d HTTP attempts", result.Stats.HTTPAttempts)
	}
	for _, fixture := range fixtures {
		got, ok := result.Outputs[fixture.id]
		if !ok || got.Decision != fixture.want {
			t.Errorf("fixture=%s decision=%s want=%s", fixture.id, got.Decision, fixture.want)
		}
	}
	t.Logf("SAFE_LIVE_FIXTURES http_attempts=%d match=%d review=%d reject=%d",
		result.Stats.HTTPAttempts, countFixtureDecisions(result.Outputs, "match"),
		countFixtureDecisions(result.Outputs, "review"), countFixtureDecisions(result.Outputs, "reject"))
}

func countFixtureDecisions(outputs map[string]MatchOutput, decision string) int {
	count := 0
	for _, output := range outputs {
		if output.Decision == decision {
			count++
		}
	}
	return count
}

func TestSafeIndependentMatchFunnel(t *testing.T) {
	if os.Getenv("ASSISTANT_SAFE_AUDIT") != "1" {
		t.Skip("safe audit not requested")
	}
	_ = godotenv.Load("../../../.env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := NewPostgresRepository(pool)

	var userID string
	if err := pool.QueryRow(ctx, `
		SELECT u.id::text
		FROM assistant_users u
		WHERE u.external_subject='local-dev-user'
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	preference, err := repo.CurrentPreferences(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	p := toPreferences(preference)
	hard := preference.HardCriteria
	t.Logf("SAFE_CRITERIA preference_version=%d developer_role=%t specialization=%s include_leadership=%t remote_only=%t required_skills_count=%d required_react=%t excluded_skills_count=%d regions_count=%d min_salary_set=%t approved_roles_count=%d ruleset=%s",
		preference.Version,
		contains(p.ApprovedRoles, "96"),
		stringValue(hard["specialization"]),
		p.IncludeLeadership,
		p.RemoteOnly,
		len(p.RequiredSkills),
		containsFold(p.RequiredSkills, "react"),
		len(p.ExcludedSkills),
		len(p.Regions),
		p.MinSalaryRUB != nil,
		len(p.ApprovedRoles),
		SpecializationRulesVersion,
	)

	status, err := repo.AnalysisStatus(ctx, userID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_UI_RUN state=%s preference_version=%d current_preference_version=%d method_version=%s ruleset_constant=%s ai_matches=%d matched=%d processed=%d total=%d worker_state=%s worker_offline=%t run_present=%t",
		status.State, status.PreferenceVersion, status.CurrentPreferenceVersion, status.MethodVersion,
		SpecializationRulesVersion, status.AIMatches, status.Matched, status.Processed, status.Total,
		status.WorkerState, status.WorkerOffline, strings.TrimSpace(status.RunID) != "")

	var listedAI, listedDet int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE m.method='ai' AND m.decision='match'),
			count(*) FILTER (WHERE m.method='deterministic' AND m.decision='match')
		FROM vacancy_match_results m
		JOIN vacancy_preferences p ON p.id=m.preference_id
		JOIN assistant_runs ar ON ar.id=m.run_id
		WHERE m.user_id=$1::uuid
		  AND p.id = (
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		  )
		  AND m.ruleset_version=$2
		  AND ar.ruleset_version=$2
		  AND ar.state NOT IN ('superseded','disabled','failed')
		  AND ar.id = (
			SELECT latest.id FROM assistant_runs latest
			WHERE latest.user_id=$1::uuid
			  AND latest.preference_id=p.id
			  AND latest.ruleset_version=$2
			  AND latest.state NOT IN ('superseded','disabled','failed')
			ORDER BY latest.created_at DESC LIMIT 1
		  )
	`, userID, SpecializationRulesVersion).Scan(&listedAI, &listedDet); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_LISTING_SCOPE current_ruleset_ai_match=%d current_ruleset_det_match=%d", listedAI, listedDet)

	var eligible, remoteTrue, remoteFalse, remoteUnknown, emptySkills, emptyDesc, emptyBoth int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE v.is_remote IS TRUE),
			count(*) FILTER (WHERE v.is_remote IS FALSE),
			count(*) FILTER (WHERE v.is_remote IS NULL),
			count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM vacancy_skills vs WHERE vs.vacancy_id=v.id)),
			count(*) FILTER (WHERE COALESCE(BTRIM(v.description_text), '') = ''),
			count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM vacancy_skills vs WHERE vs.vacancy_id=v.id)
				AND COALESCE(BTRIM(v.description_text), '') = '')
		FROM vacancies v
		JOIN sources src ON src.code=v.source AND src.is_active
		WHERE v.is_active AND v.deleted_at IS NULL
	`).Scan(&eligible, &remoteTrue, &remoteFalse, &remoteUnknown, &emptySkills, &emptyDesc, &emptyBoth); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_ELIGIBLE total=%d remote_true=%d remote_false=%d remote_unknown=%d empty_skills=%d empty_description=%d empty_skills_and_description=%d",
		eligible, remoteTrue, remoteFalse, remoteUnknown, emptySkills, emptyDesc, emptyBoth)

	var remoteUnknownNoPayload, remoteUnknownOfficialTrue, remoteFalseOfficialTrue, remoteUnknownOfficialFalse int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE v.is_remote IS NULL AND v.raw_payload IS NULL),
			count(*) FILTER (WHERE v.is_remote IS NULL AND (
				lower(COALESCE(v.raw_payload->'schedule'->>'id', '')) IN ('remote', 'remote_work')
				OR EXISTS (
					SELECT 1 FROM jsonb_array_elements(COALESCE(v.raw_payload->'work_format', '[]'::jsonb)) AS wf
					WHERE upper(COALESCE(wf->>'id', '')) IN ('REMOTE', 'REMOTE_WORK')
				)
			)),
			count(*) FILTER (WHERE v.is_remote IS FALSE AND (
				lower(COALESCE(v.raw_payload->'schedule'->>'id', '')) IN ('remote', 'remote_work')
				OR EXISTS (
					SELECT 1 FROM jsonb_array_elements(COALESCE(v.raw_payload->'work_format', '[]'::jsonb)) AS wf
					WHERE upper(COALESCE(wf->>'id', '')) IN ('REMOTE', 'REMOTE_WORK')
				)
			)),
			count(*) FILTER (WHERE v.is_remote IS NULL AND v.raw_payload IS NOT NULL AND (
				NULLIF(v.raw_payload->'schedule'->>'id', '') IS NOT NULL
				OR jsonb_array_length(COALESCE(v.raw_payload->'work_format', '[]'::jsonb)) > 0
			) AND NOT (
				lower(COALESCE(v.raw_payload->'schedule'->>'id', '')) IN ('remote', 'remote_work')
				OR EXISTS (
					SELECT 1 FROM jsonb_array_elements(COALESCE(v.raw_payload->'work_format', '[]'::jsonb)) AS wf
					WHERE upper(COALESCE(wf->>'id', '')) IN ('REMOTE', 'REMOTE_WORK')
				)
			))
		FROM vacancies v
		JOIN sources src ON src.code=v.source AND src.is_active
		WHERE v.is_active AND v.deleted_at IS NULL
	`).Scan(&remoteUnknownNoPayload, &remoteUnknownOfficialTrue, &remoteFalseOfficialTrue, &remoteUnknownOfficialFalse); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_REMOTE_PAYLOAD_GAP unknown_no_payload=%d unknown_but_official_remote=%d false_but_official_remote=%d unknown_with_non_remote_official_format=%d",
		remoteUnknownNoPayload, remoteUnknownOfficialTrue, remoteFalseOfficialTrue, remoteUnknownOfficialFalse)

	rows, err := pool.Query(ctx, `
		SELECT COALESCE(NULLIF(s.slug, ''), 'empty_slug'), count(DISTINCT vs.vacancy_id)
		FROM skills s
		JOIN vacancy_skills vs ON vs.skill_id=s.id
		JOIN vacancies v ON v.id=vs.vacancy_id
		JOIN sources src ON src.code=v.source AND src.is_active
		WHERE v.is_active AND v.deleted_at IS NULL
		  AND (s.slug ~* 'react' OR s.name ~* 'react')
		GROUP BY 1
		ORDER BY 2 DESC
		LIMIT 20
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var n int
		if err := rows.Scan(&slug, &n); err != nil {
			t.Fatal(err)
		}
		t.Logf("SAFE_REACT_SKILL_SLUG slug=%s vacancies=%d", slug, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	reactTitle := `(?:^|[^[:alnum:]])(?:react\.js|reactjs|react)(?:$|[^[:alnum:]])`
	reactNative := `(?:^|[^[:alnum:]])react[[:space:]-]*native(?:$|[^[:alnum:]])`
	reactSkillSlug := `^(react|react\.js|reactjs|react-js)$`
	reactSkillName := `(?:^|[^[:alnum:]])(?:react\.js|reactjs|react)(?:$|[^[:alnum:]])`
	frontend := `(?:^|[^[:alnum:]])(?:front[[:space:]-]?end|фронт[[:space:]-]?энд|фронтенд)(?:$|[^[:alnum:]])`
	backend := `(?:^|[^[:alnum:]])(?:back[[:space:]-]?end|бэк[[:space:]-]?энд|бэкенд|backend)(?:$|[^[:alnum:]])`
	fullstack := `(?:^|[^[:alnum:]])(?:full[[:space:]-]?stack|фулл[[:space:]-]?ст[еэ]к|фулст[еэ]к)(?:$|[^[:alnum:]])`
	leadMgmt := `(?:^|[^[:alnum:]])(?:team[[:space:]-]?lead(?:er)?|tech(?:nical)?[[:space:]-]?lead|lead[[:space:]]?(?:developer|engineer)|lead[[:space:]-]?front[[:space:]-]?end|тим[[:space:]-]?лид|тех[[:space:]-]?лид|руководител[[:alpha:]]*|head[[:space:]-]?of|cto|chief[[:space:]]+technology[[:space:]]+officer|engineering[[:space:]]+director|director[[:space:]]+of[[:space:]]+(?:engineering|development)|техническ[[:alpha:]]*[[:space:]]+директор[[:alpha:]]*|директор[[:alpha:]]*[[:space:]]+по[[:space:]]+разработк[[:alpha:]]*)(?:$|[^[:alnum:]])`
	vedushiy := `(?:^|[^[:alnum:]])ведущ[[:alpha:]]*(?:$|[^[:alnum:]])`
	seniorIC := `(?:^|[^[:alnum:]])(?:senior|старш[[:alpha:]]*|ведущ[[:alpha:]]*)(?:$|[^[:alnum:]])`

	var sqlRemote, sqlReactAny, sqlReactWeb, sqlReactNative, sqlFrontend, sqlBackend, sqlFullstack, sqlLeadMgmt, sqlVedushiy, sqlSeniorIC int
	var sqlIntersectCurrentLead, sqlIntersectIC, sqlIntersectVedushiyIC int
	if err := pool.QueryRow(ctx, `
		WITH eligible AS (
			SELECT v.id, v.title, COALESCE(v.description_text, '') AS description, v.is_remote,
				COALESCE((
					SELECT string_agg(s.slug, ' ')
					FROM vacancy_skills vs JOIN skills s ON s.id=vs.skill_id
					WHERE vs.vacancy_id=v.id
				), '') AS skill_slugs,
				COALESCE((
					SELECT string_agg(s.name, ' ')
					FROM vacancy_skills vs JOIN skills s ON s.id=vs.skill_id
					WHERE vs.vacancy_id=v.id
				), '') AS skill_names
			FROM vacancies v
			JOIN sources src ON src.code=v.source AND src.is_active
			WHERE v.is_active AND v.deleted_at IS NULL
		), marked AS (
			SELECT *,
				title ~* $1 OR description ~* $1 OR skill_names ~* $3 OR skill_slugs ~* $2 AS react_any,
				(
					(title ~* $1 AND title !~* $4) OR
					(description ~* $1 AND description !~* $4) OR
					(skill_names ~* $3 AND skill_names !~* $4) OR
					skill_slugs ~* $2
				) AS react_web,
				title ~* $4 OR description ~* $4 OR skill_names ~* $4 OR skill_slugs ~* 'react-native' AS react_native,
				title ~* $5 AS frontend_title,
				title ~* $6 AS backend_title,
				title ~* $7 AS fullstack_title,
				title ~* $8 AS lead_mgmt_title,
				title ~* $9 AS vedushiy_title,
				title ~* $10 AS senior_ic_title,
				is_remote IS TRUE AS remote_true
			FROM eligible
		)
		SELECT
			count(*) FILTER (WHERE remote_true),
			count(*) FILTER (WHERE react_any),
			count(*) FILTER (WHERE react_web),
			count(*) FILTER (WHERE react_native),
			count(*) FILTER (WHERE frontend_title AND NOT backend_title AND NOT fullstack_title),
			count(*) FILTER (WHERE backend_title AND NOT frontend_title AND NOT fullstack_title),
			count(*) FILTER (WHERE fullstack_title),
			count(*) FILTER (WHERE lead_mgmt_title),
			count(*) FILTER (WHERE vedushiy_title),
			count(*) FILTER (WHERE senior_ic_title AND NOT lead_mgmt_title),
			count(*) FILTER (WHERE remote_true AND react_web AND frontend_title AND NOT backend_title AND NOT fullstack_title AND NOT lead_mgmt_title AND NOT vedushiy_title),
			count(*) FILTER (WHERE remote_true AND react_web AND frontend_title AND NOT backend_title AND NOT fullstack_title AND NOT lead_mgmt_title),
			count(*) FILTER (WHERE remote_true AND react_web AND frontend_title AND NOT backend_title AND NOT fullstack_title AND NOT lead_mgmt_title AND vedushiy_title)
		FROM marked
	`, reactTitle, reactSkillSlug, reactSkillName, reactNative, frontend, backend, fullstack, leadMgmt, vedushiy, seniorIC).
		Scan(&sqlRemote, &sqlReactAny, &sqlReactWeb, &sqlReactNative, &sqlFrontend, &sqlBackend, &sqlFullstack,
			&sqlLeadMgmt, &sqlVedushiy, &sqlSeniorIC, &sqlIntersectCurrentLead, &sqlIntersectIC, &sqlIntersectVedushiyIC); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_SQL_PATTERNS react_title_or_desc=%s react_skill_slug=%s react_skill_name=%s react_native_excluded=%s frontend_title=%s backend_title=%s fullstack_title=%s lead_mgmt=%s vedushiy=%s",
		reactTitle, reactSkillSlug, reactSkillName, reactNative, frontend, backend, fullstack, leadMgmt, vedushiy)
	t.Logf("SAFE_SQL_FUNNEL eligible=%d remote_true=%d react_any=%d react_web=%d react_native=%d frontend_title_only=%d backend_title_only=%d fullstack_title=%d lead_mgmt_title=%d vedushiy_title=%d senior_ic_not_mgmt=%d",
		eligible, sqlRemote, sqlReactAny, sqlReactWeb, sqlReactNative, sqlFrontend, sqlBackend, sqlFullstack, sqlLeadMgmt, sqlVedushiy, sqlSeniorIC)
	t.Logf("SAFE_SQL_INTERSECT remote+react_web+frontend_title+not_backend+not_fullstack+not_mgmt_not_vedushiy=%d same_allow_vedushiy=%d vedushiy_subset=%d",
		sqlIntersectCurrentLead, sqlIntersectIC, sqlIntersectVedushiyIC)

	run := AssistantRun{UserID: userID, PreferenceID: preference.ID, SnapshotCutoff: time.Now().UTC().Add(time.Hour)}
	counts := map[string]int{}
	seen := map[string]bool{}
	vedushiyOnly := compileAliases(`ведущ[\pL]*`)
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
			counts["loaded"]++
			if len(candidate.Vacancy.Skills) == 0 {
				counts["loaded_empty_skills"]++
			}
			if strings.TrimSpace(candidate.Vacancy.Description) == "" {
				counts["loaded_empty_description"]++
			}
			if candidate.Vacancy.IsRemote == nil {
				counts["loaded_remote_unknown"]++
			} else if *candidate.Vacancy.IsRemote {
				counts["loaded_remote_true"]++
			} else {
				counts["loaded_remote_false"]++
			}
			classification := ClassifyVacancy(candidate.Vacancy)
			if classification.Leadership {
				counts["matcher_lead"]++
			} else {
				counts["matcher_ic"]++
			}
			switch classification.Specialization {
			case SpecializationFrontend:
				counts["matcher_frontend"]++
			case SpecializationBackend:
				counts["matcher_backend"]++
			case SpecializationFullstack:
				counts["matcher_fullstack"]++
			default:
				counts["matcher_spec_"+string(classification.Specialization)]++
			}
			skillMap := make(map[string]bool, len(candidate.Vacancy.Skills))
			for _, skill := range candidate.Vacancy.Skills {
				skillMap[normalizeSkill(skill)] = true
			}
			if hasExplicitSkill(candidate.Vacancy, "react", skillMap) {
				counts["matcher_react"]++
			}
			remoteOK := candidate.Vacancy.IsRemote != nil && *candidate.Vacancy.IsRemote
			frontendOK := classification.Specialization == SpecializationFrontend
			titleFrontend := classifyText(candidate.Vacancy.Title) == SpecializationFrontend
			titleClass := classifyText(candidate.Vacancy.Title)
			leadOK := !classification.Leadership
			reactOK := hasExplicitSkill(candidate.Vacancy, "react", skillMap)
			reactTitle := hasExplicitSkill(Vacancy{Title: candidate.Vacancy.Title}, "react", map[string]bool{})
			vedushiyLead := hasAlias(candidate.Vacancy.Title, vedushiyOnly)
			mgmtLead := classification.Leadership && !vedushiyLead
			if vedushiyLead {
				for _, rule := range specializationAliases.leadership {
					if strings.Contains(rule.String(), "ведущ") {
						continue
					}
					if rule.MatchString(candidate.Vacancy.Title) {
						mgmtLead = true
						break
					}
				}
			}
			if titleFrontend {
				counts["title_frontend"]++
			}
			if reactTitle {
				counts["react_in_title"]++
			}
			if remoteOK && reactTitle {
				counts["remote_react_title"]++
			}
			if remoteOK && reactTitle && !mgmtLead {
				counts["remote_react_title_not_mgmt"]++
			}
			if remoteOK && reactTitle && !mgmtLead && !vedushiyLead {
				counts["remote_react_title_not_mgmt_not_vedushiy"]++
			}
			if remoteOK && reactOK && titleFrontend && !mgmtLead {
				counts["remote_react_titlefrontend_not_mgmt"]++
			}
			roleBucket := "other"
			switch {
			case len(candidate.Vacancy.RoleIDs) == 0:
				roleBucket = "empty"
			case contains(candidate.Vacancy.RoleIDs, "96"):
				roleBucket = "96"
			case contains(candidate.Vacancy.RoleIDs, "104"):
				roleBucket = "104"
			case contains(candidate.Vacancy.RoleIDs, "124"):
				roleBucket = "124"
			case contains(candidate.Vacancy.RoleIDs, "148"):
				roleBucket = "148"
			case contains(candidate.Vacancy.RoleIDs, "150"):
				roleBucket = "150"
			case contains(candidate.Vacancy.RoleIDs, "156"):
				roleBucket = "156"
			case contains(candidate.Vacancy.RoleIDs, "164"):
				roleBucket = "164"
			}
			counts["role_bucket_"+roleBucket]++
			if remoteOK && reactOK && frontendOK {
				counts["rrf_role_"+roleBucket]++
				counts["rrf_title_spec_"+string(titleClass)]++
				if classification.Leadership {
					counts["rrf_lead"]++
					if vedushiyLead && !mgmtLead {
						counts["rrf_lead_vedushiy_only"]++
					}
					if mgmtLead {
						counts["rrf_lead_mgmt"]++
					}
					if contains(candidate.Vacancy.RoleIDs, "104") {
						counts["rrf_lead_role_104"]++
					}
				}
			}
			if remoteOK && reactOK && frontendOK && leadOK {
				counts["intersect_current_rules"]++
				counts["intersect_role_"+roleBucket]++
				counts["intersect_title_spec_"+string(titleClass)]++
				result := Match(candidate.Vacancy, p, time.Now().UTC())
				counts["intersect_decision_"+string(result.Decision)]++
				if result.Decision == DecisionReject && len(result.Conflicts) > 0 {
					counts["intersect_first_reject_"+result.Conflicts[0]]++
				}
				for _, unknown := range result.Unknowns {
					counts["intersect_unknown_"+unknown]++
				}
			}
			if remoteOK && reactOK && frontendOK {
				counts["intersect_ignore_lead"]++
				if classification.Leadership {
					counts["intersect_rejected_only_by_current_lead"]++
				}
			}
			result := Match(candidate.Vacancy, p, time.Now().UTC())
			counts["all_decision_"+string(result.Decision)]++
			if result.Decision == DecisionReject && len(result.Conflicts) > 0 {
				counts["all_first_reject_"+result.Conflicts[0]]++
			}
			if result.Decision == DecisionReview {
				for _, unknown := range result.Unknowns {
					counts["review_unknown_"+unknown]++
				}
				if roleBucket == "96" {
					counts["review_role_96"]++
				}
			}
			if roleBucket == "96" {
				if remoteOK {
					counts["r96_remote"]++
				}
				if candidate.Vacancy.IsRemote == nil {
					counts["r96_remote_unknown"]++
				}
				if reactOK {
					counts["r96_react"]++
				}
				if frontendOK {
					counts["r96_frontend"]++
				}
				if leadOK {
					counts["r96_ic"]++
				}
				if remoteOK && reactOK {
					counts["r96_remote_react"]++
				}
				if remoteOK && reactOK && frontendOK {
					counts["r96_remote_react_frontend"]++
				}
				if remoteOK && reactOK && frontendOK && leadOK {
					counts["r96_remote_react_frontend_ic"]++
				}
				if candidate.Vacancy.IsRemote == nil && reactOK && frontendOK && leadOK {
					counts["r96_react_frontend_ic_remote_unknown"]++
				}
				if candidate.Vacancy.IsRemote != nil && !*candidate.Vacancy.IsRemote && reactOK && frontendOK && leadOK {
					counts["r96_react_frontend_ic_office"]++
				}
				if result.Decision == DecisionReject && len(result.Conflicts) > 0 {
					counts["r96_first_reject_"+result.Conflicts[0]]++
				}
				counts["r96_decision_"+string(result.Decision)]++
			}
		}
		last := candidates[len(candidates)-1]
		run.CursorCreatedAt = &last.CreatedAt
		run.CursorVacancyID = last.ID
	}
	t.Logf("SAFE_MATCHER_LOAD loaded=%d empty_skills=%d empty_description=%d remote_true=%d remote_false=%d remote_unknown=%d",
		counts["loaded"], counts["loaded_empty_skills"], counts["loaded_empty_description"],
		counts["loaded_remote_true"], counts["loaded_remote_false"], counts["loaded_remote_unknown"])
	t.Logf("SAFE_MATCHER_DERIVED frontend=%d backend=%d fullstack=%d unknown=%d other=%d mobile=%d lead=%d ic=%d react=%d",
		counts["matcher_frontend"], counts["matcher_backend"], counts["matcher_fullstack"],
		counts["matcher_spec_unknown"], counts["matcher_spec_other"], counts["matcher_spec_mobile"],
		counts["matcher_lead"], counts["matcher_ic"], counts["matcher_react"])
	t.Logf("SAFE_INTERSECT_CURRENT remote+react+frontend+not_current_lead=%d match=%d review=%d reject=%d ignore_lead=%d rejected_only_by_lead=%d",
		counts["intersect_current_rules"], counts["intersect_decision_match"], counts["intersect_decision_review"],
		counts["intersect_decision_reject"], counts["intersect_ignore_lead"], counts["intersect_rejected_only_by_current_lead"])
	t.Logf("SAFE_ALL_DECISIONS match=%d review=%d reject=%d",
		counts["all_decision_match"], counts["all_decision_review"], counts["all_decision_reject"])
	t.Logf("SAFE_REVIEW_UNKNOWNS remote=%d role=%d specialization=%d spec_description_only=%d role96=%d",
		counts["review_unknown_remote"], counts["review_unknown_role"], counts["review_unknown_specialization"],
		counts["review_unknown_specialization_description_only"], counts["review_role_96"])
	t.Logf("SAFE_ROLE96 remote=%d remote_unknown=%d react=%d frontend=%d ic=%d remote_react=%d remote_react_frontend=%d remote_react_frontend_ic=%d react_frontend_ic_remote_unknown=%d react_frontend_ic_office=%d decision_match=%d decision_review=%d decision_reject=%d",
		counts["r96_remote"], counts["r96_remote_unknown"], counts["r96_react"], counts["r96_frontend"], counts["r96_ic"],
		counts["r96_remote_react"], counts["r96_remote_react_frontend"], counts["r96_remote_react_frontend_ic"],
		counts["r96_react_frontend_ic_remote_unknown"], counts["r96_react_frontend_ic_office"],
		counts["r96_decision_match"], counts["r96_decision_review"], counts["r96_decision_reject"])
	t.Logf("SAFE_TITLE_SIGNALS title_frontend=%d react_in_title=%d remote_react_title=%d remote_react_title_not_mgmt=%d remote_react_title_not_mgmt_not_vedushiy=%d remote_react_titlefrontend_not_mgmt=%d",
		counts["title_frontend"], counts["react_in_title"], counts["remote_react_title"],
		counts["remote_react_title_not_mgmt"], counts["remote_react_title_not_mgmt_not_vedushiy"],
		counts["remote_react_titlefrontend_not_mgmt"])
	t.Logf("SAFE_ROLE_ALL empty=%d r96=%d r104=%d r124=%d r148=%d r150=%d r156=%d r164=%d other=%d",
		counts["role_bucket_empty"], counts["role_bucket_96"], counts["role_bucket_104"],
		counts["role_bucket_124"], counts["role_bucket_148"], counts["role_bucket_150"],
		counts["role_bucket_156"], counts["role_bucket_164"], counts["role_bucket_other"])
	t.Logf("SAFE_RRF remote+react+frontend role_empty=%d r96=%d r104=%d r124=%d r148=%d r150=%d r156=%d r164=%d other=%d lead=%d lead_vedushiy_only=%d lead_mgmt=%d lead_role_104=%d title_frontend=%d title_unknown=%d title_fullstack=%d title_backend=%d title_mobile=%d",
		counts["rrf_role_empty"], counts["rrf_role_96"], counts["rrf_role_104"], counts["rrf_role_124"],
		counts["rrf_role_148"], counts["rrf_role_150"], counts["rrf_role_156"], counts["rrf_role_164"],
		counts["rrf_role_other"], counts["rrf_lead"], counts["rrf_lead_vedushiy_only"], counts["rrf_lead_mgmt"],
		counts["rrf_lead_role_104"], counts["rrf_title_spec_frontend"], counts["rrf_title_spec_unknown"],
		counts["rrf_title_spec_fullstack"], counts["rrf_title_spec_backend"], counts["rrf_title_spec_mobile"])
	t.Logf("SAFE_INTERSECT_ROLES empty=%d r96=%d r104=%d r124=%d r148=%d r150=%d r156=%d r164=%d other=%d title_frontend=%d title_unknown=%d title_fullstack=%d title_backend=%d title_mobile=%d",
		counts["intersect_role_empty"], counts["intersect_role_96"], counts["intersect_role_104"],
		counts["intersect_role_124"], counts["intersect_role_148"], counts["intersect_role_150"],
		counts["intersect_role_156"], counts["intersect_role_164"], counts["intersect_role_other"],
		counts["intersect_title_spec_frontend"], counts["intersect_title_spec_unknown"],
		counts["intersect_title_spec_fullstack"], counts["intersect_title_spec_backend"],
		counts["intersect_title_spec_mobile"])
	for key, value := range counts {
		if strings.HasPrefix(key, "all_first_reject_") || strings.HasPrefix(key, "intersect_first_reject_") || strings.HasPrefix(key, "intersect_unknown_") || strings.HasPrefix(key, "r96_first_reject_") || strings.HasPrefix(key, "review_unknown_") {
			t.Logf("SAFE_REASON %s=%d", key, value)
		}
	}
}

func TestSafePauseActiveManualRun(t *testing.T) {
	if os.Getenv("ASSISTANT_SAFE_PAUSE") != "1" {
		t.Skip("safe pause not requested")
	}
	_ = godotenv.Load("../../../.env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := NewPostgresRepository(pool)
	var runID, state string
	var processed, total int
	err = pool.QueryRow(ctx, `
		SELECT id::text, state, processed, snapshot_total
		FROM assistant_runs
		WHERE state IN ('queued','running')
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&runID, &state, &processed, &total)
	if err != nil {
		t.Fatalf("no active run to pause: %v", err)
	}
	t.Logf("SAFE_PAUSE_BEFORE state=%s processed=%d total=%d", state, processed, total)
	if err := repo.PauseAssistantRun(ctx, runID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM assistant_runs WHERE id=$1::uuid`, runID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_PAUSE_AFTER state=%s", state)
}

func TestSafeSupersedeStaleRulesetRuns(t *testing.T) {
	if os.Getenv("ASSISTANT_SAFE_SUPERSEDE") != "1" {
		t.Skip("safe supersede not requested")
	}
	_ = godotenv.Load("../../../.env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := NewPostgresRepository(pool)
	var userID, runID, state, ruleset string
	err = pool.QueryRow(ctx, `
		SELECT u.id::text, ar.id::text, ar.state, ar.ruleset_version
		FROM assistant_users u
		JOIN assistant_runs ar ON ar.user_id=u.id
		WHERE u.external_subject='local-dev-user'
		  AND ar.state IN ('queued','running','paused','failed','succeeded')
		  AND ar.ruleset_version <> $1
		  AND (ar.state='succeeded' OR ar.processed < ar.snapshot_total)
		ORDER BY ar.created_at DESC
		LIMIT 1
	`, SpecializationRulesVersion).Scan(&userID, &runID, &state, &ruleset)
	if err != nil {
		t.Fatalf("no stale ruleset run to supersede: %v", err)
	}
	t.Logf("SAFE_SUPERSEDE_BEFORE state=%s ruleset_stale=%t current_ruleset=%s",
		state, ruleset != SpecializationRulesVersion, SpecializationRulesVersion)
	if err := repo.SupersedeAssistantRun(ctx, userID, runID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM assistant_runs WHERE id=$1::uuid`, runID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_SUPERSEDE_AFTER state=%s", state)
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), needle) {
			return true
		}
	}
	return false
}
