//go:build integration

package assistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
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

func TestSafeHardFilterPipeline(t *testing.T) {
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
		SELECT u.id::text FROM assistant_users u WHERE u.external_subject='local-dev-user'
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	preference, err := repo.CurrentPreferences(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	p := toPreferences(preference)
	roleLabels := make([]string, 0, len(p.ApprovedRoles))
	for _, id := range p.ApprovedRoles {
		if role, ok := approvedRolePolicy[id]; ok {
			roleLabels = append(roleLabels, id+":"+role.Label)
		} else {
			roleLabels = append(roleLabels, id+":unknown")
		}
	}
	regionKinds := map[string]int{}
	for _, region := range p.Regions {
		switch {
		case looksLikeUUID(region):
			regionKinds["uuid"]++
		case looksLikeHHNumeric(region):
			regionKinds["hh_numeric"]++
		default:
			regionKinds["label"]++
		}
	}
	salarySet := p.MinSalaryRUB != nil
	salaryValue := 0.0
	if salarySet {
		salaryValue = *p.MinSalaryRUB
	}
	t.Logf("SAFE_PREF version=%d roles=%s specialization=%s include_leadership=%t remote_only=%t required_skills=%v excluded_skills=%v regions_count=%d region_kinds=%v region_labels=%v min_salary_set=%t min_salary_rub=%.0f",
		preference.Version, strings.Join(roleLabels, ","), string(p.Specialization),
		p.IncludeLeadership, p.RemoteOnly, p.RequiredSkills, p.ExcludedSkills,
		len(p.Regions), regionKinds, p.Regions, salarySet, salaryValue)

	status, err := repo.AnalysisStatus(ctx, userID, true)
	if err != nil {
		t.Fatal(err)
	}
	skipReason := ""
	if status.AISkipReason != nil {
		skipReason = *status.AISkipReason
	}
	t.Logf("SAFE_RUN state=%s ruleset=%s run_pref=%d current_pref=%d processed=%d total=%d eligible=%d matched=%d skipped=%d ai_status=%s ai_skip=%s ai_eligible=%d ai_calls=%d ai_http=%d ai_batches=%d ai_succeeded=%d ai_matches=%d ai_reviews=%d ai_rejects=%d ai_failures=%d ai_skipped=%d worker=%s offline=%t",
		status.State, status.MethodVersion, status.PreferenceVersion, status.CurrentPreferenceVersion,
		status.Processed, status.Total, status.Eligible, status.Matched, status.Skipped,
		status.AIStatus, skipReason, status.AIEligible, status.AICalls, status.AIHTTPAttempts,
		status.AIBatches, status.AISucceeded, status.AIMatches, status.AIReviews, status.AIRejects,
		status.AIFailures, status.AISkipped, status.WorkerState, status.WorkerOffline)

	var aiEnabled bool
	_ = pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT ai_enabled FROM assistant_automation_settings WHERE user_id=$1::uuid), false)
	`, userID).Scan(&aiEnabled)
	t.Logf("SAFE_AUTOMATION ai_enabled=%t", aiEnabled)

	var listed int
	matches, err := repo.ListMatches(ctx, userID, 100)
	if err != nil {
		t.Fatal(err)
	}
	listedMatch, listedReview := 0, 0
	for _, item := range matches {
		listed++
		if item.Decision == "match" {
			listedMatch++
		} else if item.Decision == "review" {
			listedReview++
		}
	}
	t.Logf("SAFE_UI_LIST listed=%d match=%d review=%d", listed, listedMatch, listedReview)

	var stored struct {
		detMatch, detReview, detReject                         int
		aiMatch, aiReview, aiReject                            int
		prefilterReject, idListMatch, idListOmitted, idListRev int
		currentRunAI, otherRunAI                               int
	}
	if err := pool.QueryRow(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		), current_run AS (
			SELECT latest.id
			FROM assistant_runs latest
			JOIN current_preference p ON p.id=latest.preference_id
			WHERE latest.user_id=$1::uuid
			  AND latest.ruleset_version=$2
			  AND latest.state NOT IN ('superseded','disabled','failed')
			ORDER BY latest.created_at DESC LIMIT 1
		)
		SELECT
			count(*) FILTER (WHERE m.method='deterministic' AND m.decision='match'),
			count(*) FILTER (WHERE m.method='deterministic' AND m.decision='review'),
			count(*) FILTER (WHERE m.method='deterministic' AND m.decision='reject'),
			count(*) FILTER (WHERE m.method='ai' AND m.decision='match'),
			count(*) FILTER (WHERE m.method='ai' AND m.decision='review'),
			count(*) FILTER (WHERE m.method='ai' AND m.decision='reject'),
			count(*) FILTER (WHERE m.method='ai' AND m.prompt_version='hard-gate-prefilter-v1'),
			count(*) FILTER (WHERE m.method='ai' AND m.rationale='id_list_match'),
			count(*) FILTER (WHERE m.method='ai' AND m.rationale='omitted'),
			count(*) FILTER (WHERE m.method='ai' AND m.decision='review' AND m.prompt_version='batch-v7-id-list'),
			count(*) FILTER (WHERE m.run_id = (SELECT id FROM current_run)),
			count(*) FILTER (WHERE m.method='ai' AND (m.run_id IS DISTINCT FROM (SELECT id FROM current_run)))
		FROM vacancy_match_results m
		JOIN current_preference p ON p.id=m.preference_id
		WHERE m.user_id=$1::uuid AND m.ruleset_version=$2
	`, userID, SpecializationRulesVersion).Scan(
		&stored.detMatch, &stored.detReview, &stored.detReject,
		&stored.aiMatch, &stored.aiReview, &stored.aiReject,
		&stored.prefilterReject, &stored.idListMatch, &stored.idListOmitted, &stored.idListRev,
		&stored.currentRunAI, &stored.otherRunAI,
	); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_STORED det_match=%d det_review=%d det_reject=%d ai_match=%d ai_review=%d ai_reject=%d prefilter=%d id_list_match=%d id_list_omitted=%d id_list_review=%d current_run_rows=%d other_run_ai=%d",
		stored.detMatch, stored.detReview, stored.detReject, stored.aiMatch, stored.aiReview, stored.aiReject,
		stored.prefilterReject, stored.idListMatch, stored.idListOmitted, stored.idListRev,
		stored.currentRunAI, stored.otherRunAI)

	var regionOverlapUUID, regionOverlapTitle, vacancyRegionKnown, vacancyRegionUnknown, salaryKnown, salaryUnknown int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE v.region_id IS NOT NULL),
			count(*) FILTER (WHERE v.region_id IS NULL),
			count(*) FILTER (WHERE v.salary_mid IS NOT NULL),
			count(*) FILTER (WHERE v.salary_mid IS NULL),
			count(*) FILTER (WHERE v.region_id::text = ANY($1::text[])),
			count(*) FILTER (WHERE EXISTS (
				SELECT 1 FROM regions r WHERE r.id=v.region_id AND (
					r.name = ANY($1::text[]) OR r.code = ANY($1::text[])
				)
			) OR EXISTS (
				SELECT 1 FROM region_external_ids x
				WHERE x.region_id=v.region_id AND x.external_id = ANY($1::text[])
			))
		FROM vacancies v
		JOIN sources src ON src.code=v.source AND src.is_active
		WHERE v.is_active AND v.deleted_at IS NULL
	`, emptyStrings(p.Regions)).Scan(&vacancyRegionKnown, &vacancyRegionUnknown, &salaryKnown, &salaryUnknown,
		&regionOverlapUUID, &regionOverlapTitle); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_REGION_SALARY_BASE vacancy_region_known=%d vacancy_region_unknown=%d salary_known=%d salary_unknown=%d pref_region_uuid_hits=%d pref_region_title_hits=%d",
		vacancyRegionKnown, vacancyRegionUnknown, salaryKnown, salaryUnknown, regionOverlapUUID, regionOverlapTitle)

	run := AssistantRun{UserID: userID, PreferenceID: preference.ID, SnapshotCutoff: time.Now().UTC().Add(time.Hour)}
	counts := map[string]int{}
	seen := map[string]bool{}
	now := time.Now().UTC()
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
			vacancy := candidate.Vacancy
			if vacancy.Title == "" {
				vacancy.Title = candidate.Title
			}
			if vacancy.Description == "" {
				vacancy.Description = candidate.Description
			}
			counts["eligible"]++
			traceHardFilterFunnel(vacancy, p, now, counts)
		}
		last := candidates[len(candidates)-1]
		run.CursorCreatedAt = &last.CreatedAt
		run.CursorVacancyID = last.ID
	}
	t.Logf("SAFE_FUNNEL eligible=%d after_excluded=%d after_leadership=%d after_role=%d after_specialization=%d after_salary=%d after_region=%d after_remote=%d after_required=%d det_match=%d det_review=%d det_reject=%d ai_would_see=%d already_final_reject=%d",
		counts["eligible"], counts["after_excluded"], counts["after_leadership"], counts["after_role"],
		counts["after_specialization"], counts["after_salary"], counts["after_region"], counts["after_remote"],
		counts["after_required"], counts["det_match"], counts["det_review"], counts["det_reject"],
		counts["ai_would_see"], counts["already_final_reject"])
	t.Logf("SAFE_FUNNEL_DROPS excluded=%d leadership=%d role_reject=%d role_unknown=%d spec_reject=%d spec_unknown=%d salary_reject=%d salary_unknown=%d region_reject=%d region_unknown=%d remote_reject=%d remote_unknown=%d required_reject=%d required_unknown=%d",
		counts["drop_excluded"], counts["drop_leadership"], counts["drop_role"], counts["role_unknown"],
		counts["drop_specialization"], counts["spec_unknown"], counts["drop_salary"], counts["salary_unknown"],
		counts["drop_region"], counts["region_unknown"], counts["drop_remote"], counts["remote_unknown"],
		counts["drop_required"], counts["required_unknown"])
	t.Logf("SAFE_ROLE_SPLIT role_pass=%d role_unknown=%d role_reject=%d role96_in_scopes=%d role_scopes_empty=%d primary_only_unofficial=%d",
		counts["role_pass"], counts["role_unknown"], counts["drop_role"], counts["role96"],
		counts["role_empty"], counts["role_primary_unofficial"])

	hiddenByTitle := 0
	reviewUnknown := map[string]int{}
	for _, item := range matches {
		if isAuditLeadershipTitle(item.Title) {
			hiddenByTitle++
		}
		for _, unknown := range item.Unknowns {
			reviewUnknown[unknown]++
		}
	}
	t.Logf("SAFE_LISTED_DETAIL hidden_by_leadership_title=%d unknown_buckets=%v", hiddenByTitle, reviewUnknown)
	for _, item := range matches {
		t.Logf("SAFE_LISTED_ITEM decision=%s method=%s stage=%s unknown_count=%d conflict_count=%d",
			item.Decision, item.Method, item.Stage, len(item.Unknowns), len(item.Conflicts))
	}

	var reviewJobsComplete, reviewJobsFailed, reviewJobsNone int
	if err := pool.QueryRow(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		), reviews AS (
			SELECT m.vacancy_id, m.vacancy_revision
			FROM vacancy_match_results m
			JOIN current_preference p ON p.id=m.preference_id
			WHERE m.user_id=$1::uuid AND m.method='deterministic' AND m.decision='review'
			  AND m.ruleset_version=$2
		)
		SELECT
			count(*) FILTER (WHERE j.status='complete'),
			count(*) FILTER (WHERE j.status='failed'),
			count(*) FILTER (WHERE j.id IS NULL)
		FROM reviews r
		LEFT JOIN assistant_ai_jobs j
		  ON j.user_id=$1::uuid AND j.preference_id=(SELECT id FROM current_preference)
		 AND j.vacancy_id=r.vacancy_id AND j.vacancy_revision=r.vacancy_revision
	`, userID, SpecializationRulesVersion).Scan(&reviewJobsComplete, &reviewJobsFailed, &reviewJobsNone); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_REVIEW_JOBS complete=%d failed=%d missing=%d", reviewJobsComplete, reviewJobsFailed, reviewJobsNone)

	reviewResultRows, err := pool.Query(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		), reviews AS (
			SELECT m.vacancy_id, m.vacancy_revision
			FROM vacancy_match_results m
			JOIN current_preference p ON p.id=m.preference_id
			WHERE m.user_id=$1::uuid AND m.method='deterministic' AND m.decision='review'
			  AND m.ruleset_version=$2
		)
		SELECT COALESCE(m.ruleset_version, '(null)'), m.method, m.decision,
			COALESCE(m.prompt_version, '(null)'), COALESCE(m.provider, '(null)'), count(*)
		FROM reviews r
		JOIN vacancy_match_results m
		  ON m.user_id=$1::uuid AND m.preference_id=(SELECT id FROM current_preference)
		 AND m.vacancy_id=r.vacancy_id
		GROUP BY 1, 2, 3, 4, 5
		ORDER BY 6 DESC
	`, userID, SpecializationRulesVersion)
	if err != nil {
		t.Fatal(err)
	}
	for reviewResultRows.Next() {
		var ruleset, method, decision, prompt, provider string
		var n int
		if err := reviewResultRows.Scan(&ruleset, &method, &decision, &prompt, &provider, &n); err != nil {
			t.Fatal(err)
		}
		t.Logf("SAFE_REVIEW_RESULTS ruleset=%s method=%s decision=%s prompt=%s provider=%s n=%d",
			ruleset, method, decision, prompt, provider, n)
	}
	reviewResultRows.Close()

	var jobProvider, jobModel, jobStatus string
	var jobAttempts int
	jobRows, err := pool.Query(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		), reviews AS (
			SELECT m.vacancy_id, m.vacancy_revision
			FROM vacancy_match_results m
			JOIN current_preference p ON p.id=m.preference_id
			WHERE m.user_id=$1::uuid AND m.method='deterministic' AND m.decision='review'
			  AND m.ruleset_version=$2
		)
		SELECT COALESCE(j.provider, '(null)'), COALESCE(j.model, '(null)'), j.status, j.attempts, count(*)
		FROM reviews r
		JOIN assistant_ai_jobs j
		  ON j.user_id=$1::uuid AND j.preference_id=(SELECT id FROM current_preference)
		 AND j.vacancy_id=r.vacancy_id AND j.vacancy_revision=r.vacancy_revision
		GROUP BY 1, 2, 3, 4
	`, userID, SpecializationRulesVersion)
	if err != nil {
		t.Fatal(err)
	}
	for jobRows.Next() {
		var n int
		if err := jobRows.Scan(&jobProvider, &jobModel, &jobStatus, &jobAttempts, &n); err != nil {
			t.Fatal(err)
		}
		t.Logf("SAFE_REVIEW_JOB_META provider=%s model=%s status=%s attempts=%d n=%d", jobProvider, jobModel, jobStatus, jobAttempts, n)
	}
	jobRows.Close()

	rows, err := pool.Query(ctx, `
		SELECT COALESCE(prompt_version, '(null)'), COALESCE(provider, '(null)'),
			CASE
				WHEN COALESCE(rationale, '') IN ('omitted', 'id_list_match', 'hard_gate_prefilter') THEN rationale
				WHEN COALESCE(rationale, '') = '' THEN '(empty)'
				ELSE '(other)'
			END AS rationale_kind,
			decision, count(*)
		FROM vacancy_match_results m
		JOIN vacancy_preferences p ON p.id=m.preference_id
		WHERE m.user_id=$1::uuid AND m.method='ai' AND m.ruleset_version=$2
		  AND p.id=(
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		  )
		GROUP BY 1, 2, 3, 4
		ORDER BY 5 DESC
	`, userID, SpecializationRulesVersion)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var prompt, provider, rationale, decision string
		var n int
		if err := rows.Scan(&prompt, &provider, &rationale, &decision, &n); err != nil {
			t.Fatal(err)
		}
		t.Logf("SAFE_AI_BUCKET prompt=%s provider=%s rationale=%s decision=%s n=%d", prompt, provider, rationale, decision, n)
	}
	rows.Close()

	var pairedRejectReject, pairedRejectReview, pairedRejectMatch, pairedReviewReject, pairedReviewReview, pairedReviewMatch, omittedConflict int
	if err := pool.QueryRow(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		)
		SELECT
			count(*) FILTER (WHERE d.decision='reject' AND a.decision='reject'),
			count(*) FILTER (WHERE d.decision='reject' AND a.decision='review'),
			count(*) FILTER (WHERE d.decision='reject' AND a.decision='match'),
			count(*) FILTER (WHERE d.decision='review' AND a.decision='reject'),
			count(*) FILTER (WHERE d.decision='review' AND a.decision='review'),
			count(*) FILTER (WHERE d.decision='review' AND a.decision='match'),
			count(*) FILTER (WHERE a.conflicts @> '["omitted_from_id_list"]'::jsonb)
		FROM vacancy_match_results a
		JOIN vacancy_match_results d
		  ON d.user_id=a.user_id AND d.preference_id=a.preference_id
		 AND d.vacancy_id=a.vacancy_id AND d.vacancy_revision=a.vacancy_revision
		 AND d.run_id IS NOT DISTINCT FROM a.run_id AND d.method='deterministic'
		JOIN current_preference p ON p.id=a.preference_id
		WHERE a.user_id=$1::uuid AND a.method='ai' AND a.ruleset_version=$2
	`, userID, SpecializationRulesVersion).Scan(
		&pairedRejectReject, &pairedRejectReview, &pairedRejectMatch,
		&pairedReviewReject, &pairedReviewReview, &pairedReviewMatch, &omittedConflict,
	); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_AI_VS_DET det_reject_ai_reject=%d det_reject_ai_review=%d det_reject_ai_match=%d det_review_ai_reject=%d det_review_ai_review=%d det_review_ai_match=%d omitted_conflict=%d",
		pairedRejectReject, pairedRejectReview, pairedRejectMatch, pairedReviewReject, pairedReviewReview, pairedReviewMatch, omittedConflict)

	roleRows, err := pool.Query(ctx, `
		SELECT COALESCE(a.pattern, '(none)'), count(DISTINCT v.id)
		FROM vacancies v
		JOIN sources src ON src.code=v.source AND src.is_active
		LEFT JOIN vacancy_role_scopes s ON s.vacancy_id=v.id AND s.scope='vacancy_listing'
		LEFT JOIN role_aliases a ON a.role_id=s.role_id AND a.source=v.source AND a.pattern ~ '^[0-9]+$'
		WHERE v.is_active AND v.deleted_at IS NULL
		GROUP BY 1
		ORDER BY 2 DESC
		LIMIT 15
	`)
	if err != nil {
		t.Fatal(err)
	}
	for roleRows.Next() {
		var pattern string
		var n int
		if err := roleRows.Scan(&pattern, &n); err != nil {
			t.Fatal(err)
		}
		t.Logf("SAFE_ROLE_PATTERN pattern=%s vacancies=%d", pattern, n)
	}
	roleRows.Close()
}

func TestSafeFrontendRoleLatticeDryRun(t *testing.T) {
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
		SELECT u.id::text FROM assistant_users u WHERE u.external_subject='local-dev-user'
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	preference, err := repo.CurrentPreferences(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	p := toPreferences(preference)
	t.Logf("SAFE_LATTICE_CRITERIA preference_version=%d developer_role=%t specialization=%s include_leadership=%t remote_only=%t required_react=%t min_salary_set=%t regions_count=%d ruleset=%s",
		preference.Version, contains(p.ApprovedRoles, "96"), string(p.Specialization), p.IncludeLeadership,
		p.RemoteOnly, containsFold(p.RequiredSkills, "react"), p.MinSalaryRUB != nil, len(p.Regions),
		SpecializationRulesVersion)

	var eligible, remoteTrue int
	if err := pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE v.is_remote IS TRUE)
		FROM vacancies v
		JOIN sources src ON src.code=v.source AND src.is_active
		WHERE v.is_active AND v.deleted_at IS NULL
	`).Scan(&eligible, &remoteTrue); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_LATTICE_SQL eligible=%d remote_true=%d", eligible, remoteTrue)

	roleRows, err := pool.Query(ctx, `
		WITH remote AS (
			SELECT v.id
			FROM vacancies v
			JOIN sources src ON src.code=v.source AND src.is_active
			WHERE v.is_active AND v.deleted_at IS NULL AND v.is_remote IS TRUE
		)
		SELECT COALESCE(a.pattern, '(none)'), count(DISTINCT r.id)
		FROM remote r
		LEFT JOIN vacancy_role_scopes s ON s.vacancy_id=r.id AND s.scope='vacancy_listing'
		LEFT JOIN role_aliases a ON a.role_id=s.role_id AND a.source='hh' AND a.pattern ~ '^[0-9]+$'
		GROUP BY 1
		ORDER BY 2 DESC, 1
	`)
	if err != nil {
		t.Fatal(err)
	}
	for roleRows.Next() {
		var pattern string
		var n int
		if err := roleRows.Scan(&pattern, &n); err != nil {
			t.Fatal(err)
		}
		t.Logf("SAFE_LATTICE_SQL_REMOTE_ROLE role_id=%s vacancies=%d", pattern, n)
	}
	roleRows.Close()

	var sqlReact, sqlFrontend, sqlNotLead, sqlIntersect, sqlHas96, sqlNo96 int
	if err := pool.QueryRow(ctx, `
		WITH eligible AS (
			SELECT v.id, v.title, COALESCE(v.description_text, '') AS description_text
			FROM vacancies v
			JOIN sources src ON src.code=v.source AND src.is_active
			WHERE v.is_active AND v.deleted_at IS NULL AND v.is_remote IS TRUE
		), react AS (
			SELECT e.id
			FROM eligible e
			WHERE (
				EXISTS (
					SELECT 1 FROM vacancy_skills vs
					JOIN skills sk ON sk.id=vs.skill_id
					WHERE vs.vacancy_id=e.id
					  AND sk.name !~* 'react[[:space:]-]*native'
					  AND sk.slug !~* 'react[[:space:]-]*native'
					  AND (
						lower(sk.slug) IN ('react','react.js','reactjs','react-js')
						OR lower(sk.name) IN ('react','react.js','reactjs')
						OR sk.name ~* '(^|[^[:alnum:]])(react\.js|reactjs|react)([^[:alnum:]]|$)'
						OR sk.slug ~* '(^|[^[:alnum:]])(react\.js|reactjs|react)([^[:alnum:]]|$)'
					  )
				)
				OR (
					e.title ~* '(^|[^[:alnum:]])(react\.js|reactjs|react)([^[:alnum:]]|$)'
					AND e.title !~* 'react[[:space:]-]*native'
				)
				OR (
					e.description_text ~* '(^|[^[:alnum:]])(react\.js|reactjs|react)([^[:alnum:]]|$)'
					AND e.description_text !~* 'react[[:space:]-]*native'
				)
			)
		), frontend AS (
			SELECT e.id
			FROM eligible e
			WHERE (
				e.title ~* '(^|[^[:alnum:]])(front[[:space:]-]?end|фронт[[:space:]-]?энд|фронтенд)([^[:alnum:]]|$)'
				OR EXISTS (
					SELECT 1 FROM vacancy_skills vs
					JOIN skills sk ON sk.id=vs.skill_id
					WHERE vs.vacancy_id=e.id
					  AND (
						sk.slug ~* '(front-?end|react|vue|angular|javascript|typescript)'
						OR sk.name ~* '(front[[:space:]-]?end|фронтенд|react|vue|angular|javascript|typescript)'
					  )
					  AND sk.name !~* 'react[[:space:]-]*native'
				)
			)
			AND e.title !~* 'full[[:space:]-]?stack|фулл[[:space:]-]?ст'
			AND e.title !~* '(^|[^[:alnum:]])(back[[:space:]-]?end|бэкенд)([^[:alnum:]]|$)'
		), not_lead AS (
			SELECT e.id
			FROM eligible e
			WHERE e.title !~* 'team[[:space:]-]?lead|tech(?:nical)?[[:space:]-]?lead|(^|[^[:alnum:]])lead[[:space:]]+(developer|engineer|front)|тим[[:space:]-]?лид|тех[[:space:]-]?лид|руководител|head[[:space:]-]?of|(^|[^[:alnum:]])cto([^[:alnum:]]|$)|директор'
			  AND NOT EXISTS (
				SELECT 1 FROM vacancy_role_scopes s
				JOIN role_aliases a ON a.role_id=s.role_id AND a.source='hh' AND a.pattern='104'
				WHERE s.vacancy_id=e.id AND s.scope='vacancy_listing'
			  )
		), intersected AS (
			SELECT r.id
			FROM react r
			JOIN frontend f ON f.id=r.id
			JOIN not_lead n ON n.id=r.id
		)
		SELECT
			(SELECT count(*) FROM react),
			(SELECT count(*) FROM frontend),
			(SELECT count(*) FROM not_lead),
			(SELECT count(*) FROM intersected),
			(SELECT count(*) FROM intersected i WHERE EXISTS (
				SELECT 1 FROM vacancy_role_scopes s
				JOIN role_aliases a ON a.role_id=s.role_id AND a.source='hh' AND a.pattern='96'
				WHERE s.vacancy_id=i.id AND s.scope='vacancy_listing'
			)),
			(SELECT count(*) FROM intersected i WHERE NOT EXISTS (
				SELECT 1 FROM vacancy_role_scopes s
				JOIN role_aliases a ON a.role_id=s.role_id AND a.source='hh' AND a.pattern='96'
				WHERE s.vacancy_id=i.id AND s.scope='vacancy_listing'
			))
	`).Scan(&sqlReact, &sqlFrontend, &sqlNotLead, &sqlIntersect, &sqlHas96, &sqlNo96); err != nil {
		t.Fatal(err)
	}
	t.Logf("SAFE_LATTICE_SQL_INTERSECT remote_react=%d remote_frontend=%d remote_not_lead=%d remote_react_frontend_ic=%d has_role_96=%d without_role_96=%d",
		sqlReact, sqlFrontend, sqlNotLead, sqlIntersect, sqlHas96, sqlNo96)

	sqlRoleRows, err := pool.Query(ctx, `
		WITH eligible AS (
			SELECT v.id, v.title, COALESCE(v.description_text, '') AS description_text
			FROM vacancies v
			JOIN sources src ON src.code=v.source AND src.is_active
			WHERE v.is_active AND v.deleted_at IS NULL AND v.is_remote IS TRUE
		), react AS (
			SELECT e.id FROM eligible e
			WHERE (
				EXISTS (
					SELECT 1 FROM vacancy_skills vs
					JOIN skills sk ON sk.id=vs.skill_id
					WHERE vs.vacancy_id=e.id
					  AND sk.name !~* 'react[[:space:]-]*native'
					  AND sk.slug !~* 'react[[:space:]-]*native'
					  AND (
						lower(sk.slug) IN ('react','react.js','reactjs','react-js')
						OR lower(sk.name) IN ('react','react.js','reactjs')
						OR sk.name ~* '(^|[^[:alnum:]])(react\.js|reactjs|react)([^[:alnum:]]|$)'
						OR sk.slug ~* '(^|[^[:alnum:]])(react\.js|reactjs|react)([^[:alnum:]]|$)'
					  )
				)
				OR (e.title ~* '(^|[^[:alnum:]])(react\.js|reactjs|react)([^[:alnum:]]|$)' AND e.title !~* 'react[[:space:]-]*native')
				OR (e.description_text ~* '(^|[^[:alnum:]])(react\.js|reactjs|react)([^[:alnum:]]|$)' AND e.description_text !~* 'react[[:space:]-]*native')
			)
		), frontend AS (
			SELECT e.id FROM eligible e
			WHERE (
				e.title ~* '(^|[^[:alnum:]])(front[[:space:]-]?end|фронт[[:space:]-]?энд|фронтенд)([^[:alnum:]]|$)'
				OR EXISTS (
					SELECT 1 FROM vacancy_skills vs
					JOIN skills sk ON sk.id=vs.skill_id
					WHERE vs.vacancy_id=e.id
					  AND (sk.slug ~* '(front-?end|react|vue|angular|javascript|typescript)'
						OR sk.name ~* '(front[[:space:]-]?end|фронтенд|react|vue|angular|javascript|typescript)')
					  AND sk.name !~* 'react[[:space:]-]*native'
				)
			)
			AND e.title !~* 'full[[:space:]-]?stack|фулл[[:space:]-]?ст'
			AND e.title !~* '(^|[^[:alnum:]])(back[[:space:]-]?end|бэкенд)([^[:alnum:]]|$)'
		), not_lead AS (
			SELECT e.id FROM eligible e
			WHERE e.title !~* 'team[[:space:]-]?lead|tech(?:nical)?[[:space:]-]?lead|(^|[^[:alnum:]])lead[[:space:]]+(developer|engineer|front)|тим[[:space:]-]?лид|тех[[:space:]-]?лид|руководител|head[[:space:]-]?of|(^|[^[:alnum:]])cto([^[:alnum:]]|$)|директор'
			  AND NOT EXISTS (
				SELECT 1 FROM vacancy_role_scopes s
				JOIN role_aliases a ON a.role_id=s.role_id AND a.source='hh' AND a.pattern='104'
				WHERE s.vacancy_id=e.id AND s.scope='vacancy_listing'
			  )
		), intersected AS (
			SELECT r.id FROM react r JOIN frontend f ON f.id=r.id JOIN not_lead n ON n.id=r.id
		)
		SELECT COALESCE(a.pattern, '(none)'), count(DISTINCT i.id)
		FROM intersected i
		LEFT JOIN vacancy_role_scopes s ON s.vacancy_id=i.id AND s.scope='vacancy_listing'
		LEFT JOIN role_aliases a ON a.role_id=s.role_id AND a.source='hh' AND a.pattern ~ '^[0-9]+$'
		GROUP BY 1
		ORDER BY 2 DESC, 1
	`)
	if err != nil {
		t.Fatal(err)
	}
	for sqlRoleRows.Next() {
		var pattern string
		var n int
		if err := sqlRoleRows.Scan(&pattern, &n); err != nil {
			t.Fatal(err)
		}
		t.Logf("SAFE_LATTICE_SQL_INTERSECT_ROLE role_id=%s vacancies=%d", pattern, n)
	}
	sqlRoleRows.Close()

	run := AssistantRun{UserID: userID, PreferenceID: preference.ID, SnapshotCutoff: time.Now().UTC().Add(time.Hour)}
	counts := map[string]int{}
	seen := map[string]bool{}
	now := time.Now().UTC()
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
			vacancy := candidate.Vacancy
			if vacancy.Title == "" {
				vacancy.Title = candidate.Title
			}
			if vacancy.Description == "" {
				vacancy.Description = candidate.Description
			}
			vacancy.RoleIDs = officialRoleIDs(vacancy)
			counts["eligible"]++
			after := Match(vacancy, p, now)
			counts["after_"+string(after.Decision)]++
			if catalogRoleHardRejectV3(vacancy, p) {
				counts["v3_role_reject"]++
			}
			classification := ClassifyVacancy(vacancy)
			skillMap := make(map[string]bool, len(vacancy.Skills))
			for _, skill := range vacancy.Skills {
				skillMap[normalizeSkill(skill)] = true
			}
			remoteOK := vacancy.IsRemote != nil && *vacancy.IsRemote
			reactOK := hasExplicitSkill(vacancy, "react", skillMap)
			frontendOK := classification.Specialization == SpecializationFrontend && specializationProven(classification)
			if remoteOK && reactOK && frontendOK && !classification.Leadership {
				counts["intersect"]++
				switch {
				case contains(vacancy.RoleIDs, "96"):
					counts["intersect_role_96"]++
				case len(vacancy.RoleIDs) == 0:
					counts["intersect_role_empty"]++
				default:
					counts["intersect_role_other"]++
				}
				for _, roleID := range vacancy.RoleIDs {
					counts["intersect_role_id_"+roleID]++
				}
				beforeReject := catalogRoleHardRejectV3(vacancy, p)
				if beforeReject {
					counts["intersect_v3_role_reject"]++
					counts["intersect_v3_decision_reject"]++
				} else {
					counts["intersect_v3_decision_"+string(after.Decision)]++
				}
				counts["intersect_v4_decision_"+string(after.Decision)]++
				if beforeReject && after.Decision == DecisionMatch {
					counts["rescued_to_match"]++
				}
				if beforeReject && after.Decision == DecisionReview {
					counts["rescued_to_review"]++
				}
				if beforeReject && after.Decision == DecisionReject {
					counts["still_reject_other_gate"]++
				}
			}
		}
		last := candidates[len(candidates)-1]
		run.CursorCreatedAt = &last.CreatedAt
		run.CursorVacancyID = last.ID
	}

	t.Logf("SAFE_LATTICE_GO_INTERSECT n=%d role_96=%d role_other=%d role_empty=%d",
		counts["intersect"], counts["intersect_role_96"], counts["intersect_role_other"], counts["intersect_role_empty"])
	for _, roleID := range []string{"96", "104", "124", "148", "150", "156", "164"} {
		if n := counts["intersect_role_id_"+roleID]; n > 0 {
			t.Logf("SAFE_LATTICE_GO_INTERSECT_ROLE role_id=%s vacancies=%d", roleID, n)
		}
	}
	t.Logf("SAFE_LATTICE_BEFORE_ON_INTERSECT v3_role_reject=%d v3_match=%d v3_review=%d v3_reject=%d",
		counts["intersect_v3_role_reject"], counts["intersect_v3_decision_match"],
		counts["intersect_v3_decision_review"], counts["intersect_v3_decision_reject"])
	t.Logf("SAFE_LATTICE_AFTER_ON_INTERSECT v4_match=%d v4_review=%d v4_reject=%d rescued_to_match=%d rescued_to_review=%d still_reject_other=%d",
		counts["intersect_v4_decision_match"], counts["intersect_v4_decision_review"],
		counts["intersect_v4_decision_reject"], counts["rescued_to_match"],
		counts["rescued_to_review"], counts["still_reject_other_gate"])
	t.Logf("SAFE_LATTICE_DRY_RUN_ALL eligible=%d match=%d review=%d reject=%d v3_role_reject_all=%d",
		counts["eligible"], counts["after_match"], counts["after_review"], counts["after_reject"], counts["v3_role_reject"])
}

func isAuditLeadershipTitle(title string) bool {
	return regexp.MustCompile(`(?i)team[\s-]?lead|tech(?:nical)?[\s-]?lead|\blead[\s-]+(?:developer|engineer|front)|тим[\s-]?лид|тех[\s-]?лид|руководител|head[\s-]?of|\bcto\b|директор`).MatchString(title)
}

func emptyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func looksLikeUUID(value string) bool {
	return uuidRE.MatchString(strings.TrimSpace(value))
}

func looksLikeHHNumeric(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func traceHardFilterFunnel(v Vacancy, p Preferences, now time.Time, counts map[string]int) {
	v.RoleIDs = officialRoleIDs(v)
	if len(v.RoleIDs) == 0 {
		counts["role_empty"]++
	}
	if contains(v.RoleIDs, "96") {
		counts["role96"]++
	}
	if strings.TrimSpace(v.RoleID) != "" {
		if _, ok := approvedRolePolicy[v.RoleID]; !ok && len(v.RoleIDs) == 0 {
			counts["role_primary_unofficial"]++
		}
	}
	skills := make(map[string]bool, len(v.Skills))
	for _, s := range v.Skills {
		skills[normalizeSkill(s)] = true
	}
	remaining := true
	for _, excluded := range p.ExcludedSkills {
		if skills[strings.ToLower(strings.TrimSpace(excluded))] {
			counts["drop_excluded"]++
			remaining = false
			break
		}
	}
	if remaining {
		counts["after_excluded"]++
	} else {
		finalizeFunnelDecision(v, p, now, counts)
		return
	}

	classification := ClassifyVacancy(v)
	if classification.Leadership && !p.IncludeLeadership {
		counts["drop_leadership"]++
		finalizeFunnelDecision(v, p, now, counts)
		return
	}
	counts["after_leadership"]++

	if p.Specialization != "" {
		switch {
		case classification.Specialization == SpecializationUnknown:
			counts["after_specialization"]++
			counts["spec_unknown"]++
		case classification.Specialization != p.Specialization:
			counts["drop_specialization"]++
			finalizeFunnelDecision(v, p, now, counts)
			return
		default:
			counts["after_specialization"]++
		}
	} else {
		counts["after_specialization"]++
	}

	switch {
	case len(p.ApprovedRoles) == 0:
		counts["after_role"]++
		counts["role_pass"]++
	case len(v.RoleIDs) > 0 && overlaps(p.ApprovedRoles, v.RoleIDs):
		counts["after_role"]++
		counts["role_pass"]++
	case p.Specialization != "" && classification.Specialization == p.Specialization && specializationProven(classification):
		counts["after_role"]++
		counts["role_pass"]++
		counts["role_bypassed_by_spec"]++
	case p.Specialization != "":
		counts["after_role"]++
		counts["role_unknown"]++
	case len(v.RoleIDs) == 0:
		counts["after_role"]++
		counts["role_unknown"]++
	case contains(p.ApprovedRoles, "96") && classification.Specialization == SpecializationFrontend && specializationProven(classification):
		counts["after_role"]++
		counts["role_pass"]++
		counts["role_bypassed_by_spec"]++
	default:
		counts["drop_role"]++
		finalizeFunnelDecision(v, p, now, counts)
		return
	}

	if p.MinSalaryRUB != nil {
		if v.SalaryRUB == nil {
			counts["after_salary"]++
			counts["salary_unknown"]++
		} else if *v.SalaryRUB < *p.MinSalaryRUB {
			counts["drop_salary"]++
			finalizeFunnelDecision(v, p, now, counts)
			return
		} else {
			counts["after_salary"]++
		}
	} else {
		counts["after_salary"]++
	}

	if len(p.Regions) > 0 {
		if v.RegionID == "" {
			counts["after_region"]++
			counts["region_unknown"]++
		} else if !contains(p.Regions, v.RegionID) {
			counts["drop_region"]++
			finalizeFunnelDecision(v, p, now, counts)
			return
		} else {
			counts["after_region"]++
		}
	} else {
		counts["after_region"]++
	}

	if p.RemoteOnly {
		if v.IsRemote == nil {
			counts["after_remote"]++
			counts["remote_unknown"]++
		} else if !*v.IsRemote {
			counts["drop_remote"]++
			finalizeFunnelDecision(v, p, now, counts)
			return
		} else {
			counts["after_remote"]++
		}
	} else {
		counts["after_remote"]++
	}

	requiredDropped := false
	for _, required := range p.RequiredSkills {
		normalized := normalizeSkill(required)
		if hasExplicitSkill(v, normalized, skills) {
			continue
		}
		if strings.TrimSpace(v.Title) == "" && len(v.Skills) == 0 && strings.TrimSpace(v.Description) == "" {
			counts["required_unknown"]++
			continue
		}
		counts["drop_required"]++
		requiredDropped = true
		break
	}
	if requiredDropped {
		finalizeFunnelDecision(v, p, now, counts)
		return
	}
	counts["after_required"]++
	finalizeFunnelDecision(v, p, now, counts)
}

func finalizeFunnelDecision(v Vacancy, p Preferences, now time.Time, counts map[string]int) {
	result := Match(v, p, now)
	counts["det_"+string(result.Decision)]++
	if result.Decision == DecisionReject {
		counts["already_final_reject"]++
	} else {
		counts["ai_would_see"]++
	}
}
