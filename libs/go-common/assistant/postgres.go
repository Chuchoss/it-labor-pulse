package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the small database surface needed by the assistant repository.
// It is intentionally compatible with pgxpool.Pool and transactions.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PreferenceRecord struct {
	ID                             string
	Version                        int
	Note                           string
	HardCriteria                   map[string]any
	SoftCriteria                   map[string]any
	Weights                        map[string]float64
	ActiveFrom                     time.Time
	ArchivedAt                     *time.Time
	LegacyRoleUpgraded             bool
	LegacySpecializationSuggestion Specialization
}

type AnalysisStatus struct {
	RunID                         string     `json:"run_id,omitempty"`
	AIConfigured                  bool       `json:"ai_configured"`
	AIStatus                      string     `json:"ai_status"`
	AISkipReason                  *string    `json:"ai_skip_reason,omitempty"`
	State                         string     `json:"state"`
	StartedAt                     *time.Time `json:"started_at,omitempty"`
	FinishedAt                    *time.Time `json:"finished_at,omitempty"`
	LastCheckedAt                 time.Time  `json:"last_checked_at"`
	Processed                     int        `json:"processed"`
	Total                         int        `json:"total"`
	Eligible                      int        `json:"eligible"`
	Matched                       int        `json:"matched"`
	AICalls                       int        `json:"ai_calls"`
	AIEligible                    int        `json:"ai_eligible"`
	AISucceeded                   int        `json:"ai_succeeded"`
	AIMatches                     int        `json:"ai_matches"`
	AIReviews                     int        `json:"ai_reviews"`
	AIRejects                     int        `json:"ai_rejects"`
	AIFailures                    int        `json:"ai_failures"`
	AISkipped                     int        `json:"ai_skipped"`
	AIHTTPAttempts                int        `json:"ai_http_attempts"`
	AIRetries                     int        `json:"ai_retries"`
	AIBatches                     int        `json:"ai_batches"`
	AIPromptTokens                int        `json:"ai_prompt_tokens"`
	AICompletionTokens            int        `json:"ai_completion_tokens"`
	AICachedTokens                int        `json:"ai_cached_tokens"`
	AIRateLimit                   int        `json:"ai_rate_limit"`
	AITimeouts                    int        `json:"ai_timeouts"`
	AIInvalidResponses            int        `json:"ai_invalid_responses"`
	AIAuth                        int        `json:"ai_auth"`
	AIQuota                       int        `json:"ai_quota"`
	AIServer                      int        `json:"ai_server"`
	AINetwork                     int        `json:"ai_network"`
	AIContextLimit                int        `json:"ai_context_limit"`
	AIContentFilter               int        `json:"ai_content_filter"`
	AIInvalidRequest              int        `json:"ai_invalid_request"`
	Skipped                       int        `json:"skipped"`
	ErrorCategory                 *string    `json:"error_category,omitempty"`
	RequestID                     *string    `json:"request_id,omitempty"`
	CursorSource                  string     `json:"cursor_source"`
	CursorObservedAt              *time.Time `json:"cursor_observed_at,omitempty"`
	PendingCandidates             bool       `json:"pending_candidates"`
	Provider                      *string    `json:"provider,omitempty"`
	Model                         *string    `json:"model,omitempty"`
	PromptVersion                 *string    `json:"prompt_version,omitempty"`
	MethodVersion                 string     `json:"method_version"`
	RulesetVersion                string     `json:"ruleset_version,omitempty"`
	PreferenceVersion             int        `json:"preference_version,omitempty"`
	CurrentPreferenceVersion      int        `json:"current_preference_version,omitempty"`
	SupersededByPreferenceVersion *int       `json:"superseded_by_preference_version,omitempty"`
	SupersededFromState           string     `json:"superseded_from_state,omitempty"`
	WorkerHeartbeatAt             *time.Time `json:"worker_heartbeat_at,omitempty"`
	WorkerPhase                   string     `json:"worker_phase,omitempty"`
	WorkerRetryCategory           *string    `json:"worker_retry_category,omitempty"`
	WorkerRetryUntil              *time.Time `json:"worker_retry_until,omitempty"`
	WorkerActiveBatches           int        `json:"worker_active_batches"`
	WorkerConcurrency             int        `json:"worker_concurrency"`
	WorkerOffline                 bool       `json:"worker_offline"`
	WorkerStalled                 bool       `json:"worker_stalled"`
	WorkerInstanceID              string     `json:"worker_instance_id,omitempty"`
	WorkerStartedAt               *time.Time `json:"worker_started_at,omitempty"`
	WorkerVersion                 string     `json:"worker_version,omitempty"`
	WorkerMode                    string     `json:"worker_mode,omitempty"`
	WorkerState                   string     `json:"worker_state,omitempty"`
	WorkerLastSeenAt              *time.Time `json:"worker_last_seen_at,omitempty"`
	ServerTime                    *time.Time `json:"server_time,omitempty"`
}

type AssistantRun struct {
	ID                                                           string
	UserID                                                       string
	PreferenceID                                                 string
	SnapshotCutoff                                               time.Time
	Total                                                        int
	Processed                                                    int
	Eligible                                                     int
	Matched                                                      int
	AICalls                                                      int
	AIEligible                                                   int
	AISucceeded                                                  int
	AIMatches                                                    int
	AIReviews                                                    int
	AIRejects                                                    int
	AIFailures                                                   int
	AISkipped                                                    int
	AIHTTPAttempts, AIRetries, AIBatches                         int
	AIPromptTokens, AICompletionTokens, AICachedTokens           int
	AIRateLimit, AITimeouts, AIInvalidResponses, AIAuth, AIQuota int
	AIServer, AINetwork, AIContextLimit, AIContentFilter         int
	AIInvalidRequest                                             int
	AIStatus                                                     string
	AISkipReason                                                 string
	Skipped                                                      int
	CursorCreatedAt                                              *time.Time
	CursorVacancyID                                              string
}

type AssistantRunStore interface {
	ClaimAssistantRun(context.Context) (AssistantRun, bool, error)
	CompleteAssistantRun(context.Context, string, string, WorkerStats, string) error
}

type AssistantRunHeartbeatStore interface {
	HeartbeatAssistantRun(context.Context, string, string, string, *time.Time, int, int) error
}

type AssistantRunControlStore interface {
	PauseAssistantRun(context.Context, string) error
	ResumeAssistantRun(context.Context, string) error
	SupersedeAssistantRun(context.Context, string, string) error
}

type ScopedWorkerStore interface {
	UsersForAssistantRun(context.Context, AssistantRun) ([]WorkerUser, error)
}

var ErrInvalidPreferences = errors.New("invalid assistant preferences")

type MatchRecord struct {
	VacancyID  string   `json:"vacancy_id"`
	Title      string   `json:"title"`
	SourceURL  *string  `json:"source_url"`
	Decision   Decision `json:"decision"`
	Score      float64  `json:"score"`
	Method     string   `json:"method"`
	Stage      string   `json:"stage"`
	Confidence *string  `json:"confidence"`
	Reasons    []string `json:"reasons"`
	Unknowns   []string `json:"unknowns"`
	Conflicts  []string `json:"conflicts"`
	Evidence   []string `json:"evidence_ids"`
}

type TelegramStatus struct {
	Configured                          bool
	Linked                              bool
	OptedIn                             bool
	Pending, Sent, Failed, DeadLettered int
	LastError                           string
}

type AutomationSettings struct {
	AIEnabled       bool       `json:"ai_enabled"`
	TelegramEnabled bool       `json:"telegram_enabled"`
	ActivationAt    *time.Time `json:"activation_at,omitempty"`
}

// PostgresRepository persists preferences and reads assistant results without
// exposing secrets or raw vacancy descriptions.
type PostgresRepository struct {
	db       DBTX
	poolConn *pgxpool.Pool
}

func NewPostgresRepository(db DBTX) *PostgresRepository {
	pool, _ := db.(*pgxpool.Pool)
	return &PostgresRepository{db: db, poolConn: pool}
}

func (r *PostgresRepository) EnsureUser(ctx context.Context, subject string) (string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" || len(subject) > 255 {
		return "", errors.New("assistant subject is required")
	}
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO assistant_users (external_subject)
		VALUES ($1)
		ON CONFLICT (external_subject) DO UPDATE SET external_subject = EXCLUDED.external_subject
		RETURNING id::text
	`, subject).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("ensure assistant user: %w", err)
	}
	return id, nil
}

func (r *PostgresRepository) CurrentPreferences(ctx context.Context, userID string) (PreferenceRecord, error) {
	var p PreferenceRecord
	var hard, soft, weights []byte
	err := r.db.QueryRow(ctx, `
		SELECT id::text, version, note, hard_criteria, soft_criteria, weights, active_from, archived_at
		FROM vacancy_preferences
		WHERE user_id = $1::uuid AND archived_at IS NULL
		ORDER BY version DESC LIMIT 1
	`, userID).Scan(&p.ID, &p.Version, &p.Note, &hard, &soft, &weights, &p.ActiveFrom, &p.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PreferenceRecord{HardCriteria: map[string]any{}, SoftCriteria: map[string]any{}, Weights: map[string]float64{}}, nil
	}
	if err != nil {
		return PreferenceRecord{}, fmt.Errorf("get assistant preferences: %w", err)
	}
	if err := json.Unmarshal(hard, &p.HardCriteria); err != nil {
		return PreferenceRecord{}, fmt.Errorf("decode hard criteria: %w", err)
	}
	if err := json.Unmarshal(soft, &p.SoftCriteria); err != nil {
		return PreferenceRecord{}, fmt.Errorf("decode soft criteria: %w", err)
	}
	if err := json.Unmarshal(weights, &p.Weights); err != nil {
		return PreferenceRecord{}, fmt.Errorf("decode weights: %w", err)
	}
	normalized, _, err := NormalizePreferenceRoles(p)
	if err != nil {
		return PreferenceRecord{}, err
	}
	return normalized, nil
}

func (r *PostgresRepository) ListPreferences(ctx context.Context, userID string) ([]PreferenceRecord, error) {
	rows, err := r.db.Query(ctx, `SELECT id::text, version, note, hard_criteria, soft_criteria, weights, active_from, archived_at
		FROM vacancy_preferences WHERE user_id = $1::uuid ORDER BY version DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list assistant preferences: %w", err)
	}
	defer rows.Close()
	result := make([]PreferenceRecord, 0)
	for rows.Next() {
		var p PreferenceRecord
		var hard, soft, weights []byte
		if err := rows.Scan(&p.ID, &p.Version, &p.Note, &hard, &soft, &weights, &p.ActiveFrom, &p.ArchivedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(hard, &p.HardCriteria); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(soft, &p.SoftCriteria); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(weights, &p.Weights); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) ArchivePreference(ctx context.Context, userID, preferenceID string) error {
	tag, err := r.db.Exec(ctx, `UPDATE vacancy_preferences SET archived_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND archived_at IS NULL`, preferenceID, userID)
	if err != nil {
		return fmt.Errorf("archive assistant preference: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *PostgresRepository) AnalysisStatus(ctx context.Context, userID string, aiConfigured bool) (AnalysisStatus, error) {
	var s AnalysisStatus
	var cursorAt *time.Time
	err := r.db.QueryRow(ctx, `SELECT ar.id::text, ar.state, ar.started_at, ar.finished_at, ar.last_checked_at,
		processed, snapshot_total, eligible, matched, ai_calls, ai_eligible, ai_succeeded,
		ai_matches, ai_reviews, ai_rejects, ai_failures, ai_skipped, ai_status, ai_skip_reason,
		ai_http_attempts, ai_retries, ai_batches, ai_prompt_tokens, ai_completion_tokens, ai_cached_tokens,
		ai_rate_limit, ai_timeouts, ai_invalid_responses,
		ai_auth, ai_quota, ai_server, ai_network, ai_context_limit, ai_content_filter, ai_invalid_request,
		skipped, error_category, request_id,
		cursor_source, cursor_observed_at, pending_candidates, provider, model, prompt_version,
		run_preference.version, current_preference.version, superseding_preference.version,
		COALESCE(ar.superseded_from_state, ''), ar.ruleset_version,
		ar.worker_heartbeat_at, ar.worker_phase, ar.worker_retry_category, ar.worker_retry_until,
		ar.worker_active_batches, ar.worker_concurrency
		FROM assistant_runs ar
		JOIN vacancy_preferences run_preference ON run_preference.id=ar.preference_id
		JOIN LATERAL (
			SELECT id, version FROM vacancy_preferences
			WHERE user_id=ar.user_id AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		) current_preference ON true
		LEFT JOIN vacancy_preferences superseding_preference
			ON superseding_preference.id=ar.superseded_by_preference_id
		WHERE ar.user_id = $1::uuid ORDER BY ar.created_at DESC LIMIT 1`, userID).Scan(
		&s.RunID, &s.State, &s.StartedAt, &s.FinishedAt, &s.LastCheckedAt, &s.Processed, &s.Total, &s.Eligible,
		&s.Matched, &s.AICalls, &s.AIEligible, &s.AISucceeded,
		&s.AIMatches, &s.AIReviews, &s.AIRejects, &s.AIFailures, &s.AISkipped, &s.AIStatus, &s.AISkipReason,
		&s.AIHTTPAttempts, &s.AIRetries, &s.AIBatches, &s.AIPromptTokens, &s.AICompletionTokens,
		&s.AICachedTokens, &s.AIRateLimit, &s.AITimeouts, &s.AIInvalidResponses,
		&s.AIAuth, &s.AIQuota, &s.AIServer, &s.AINetwork, &s.AIContextLimit, &s.AIContentFilter,
		&s.AIInvalidRequest,
		&s.Skipped, &s.ErrorCategory, &s.RequestID, &s.CursorSource,
		&cursorAt, &s.PendingCandidates, &s.Provider, &s.Model, &s.PromptVersion,
		&s.PreferenceVersion, &s.CurrentPreferenceVersion, &s.SupersededByPreferenceVersion,
		&s.SupersededFromState, &s.RulesetVersion, &s.WorkerHeartbeatAt, &s.WorkerPhase,
		&s.WorkerRetryCategory, &s.WorkerRetryUntil, &s.WorkerActiveBatches, &s.WorkerConcurrency)
	if errors.Is(err, pgx.ErrNoRows) {
		s.State = "never_run"
		s.AIStatus = "not_run"
		s.LastCheckedAt = time.Now().UTC()
	} else if err != nil {
		return AnalysisStatus{}, fmt.Errorf("get assistant status: %w", err)
	}
	s.AIConfigured = aiConfigured
	if !aiConfigured && s.State == "never_run" {
		s.State = "disabled"
	}
	s.CursorObservedAt = cursorAt
	s.MethodVersion = SpecializationRulesVersion
	if err := r.loadWorkerAvailability(ctx, &s); err != nil {
		return AnalysisStatus{}, err
	}
	s.WorkerStalled = s.State == "running" && s.ServerTime != nil &&
		s.ServerTime.Sub(s.LastCheckedAt) > 2*time.Minute
	return s, nil
}

func (r *PostgresRepository) loadWorkerAvailability(ctx context.Context, status *AnalysisStatus) error {
	ttl := fmt.Sprintf("%f seconds", WorkerAvailabilityTTL.Seconds())
	var startedAt, lastSeenAt, serverTime time.Time
	err := r.db.QueryRow(ctx, `
		SELECT instance_id::text, started_at, version, mode, state, last_seen_at,
			clock_timestamp(),
			state = 'stopping' OR last_seen_at < clock_timestamp() - $1::interval
		FROM assistant_worker_instances
		ORDER BY
			(state <> 'stopping' AND last_seen_at >= clock_timestamp() - $1::interval) DESC,
			last_seen_at DESC
		LIMIT 1
	`, ttl).Scan(
		&status.WorkerInstanceID, &startedAt, &status.WorkerVersion, &status.WorkerMode,
		&status.WorkerState, &lastSeenAt, &serverTime, &status.WorkerOffline,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		status.WorkerOffline = true
		status.WorkerState = "offline"
		if err := r.db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&serverTime); err != nil {
			return fmt.Errorf("get assistant worker server time: %w", err)
		}
		status.ServerTime = &serverTime
		return nil
	}
	if err != nil {
		return fmt.Errorf("get assistant worker availability: %w", err)
	}
	status.WorkerStartedAt = &startedAt
	status.WorkerLastSeenAt = &lastSeenAt
	status.ServerTime = &serverTime
	return nil
}

func (r *PostgresRepository) QueueAnalysis(ctx context.Context, userID, requestID string, aiConfigured bool) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		WITH locked AS (
			SELECT pg_advisory_xact_lock(hashtextextended($1, 1))
		), existing_request AS (
			SELECT id FROM assistant_runs CROSS JOIN locked
			WHERE user_id = $1::uuid AND request_id = NULLIF($2, '')
		), active AS (
			SELECT id FROM assistant_runs
			WHERE user_id = $1::uuid AND state IN ('queued','running','paused')
		), preference AS (
			SELECT id FROM vacancy_preferences
			WHERE user_id = $1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		), scope AS (
			SELECT clock_timestamp() AS cutoff, count(v.id)::integer AS total
			FROM vacancies v JOIN sources s ON s.code = v.source AND s.is_active
			WHERE v.is_active AND v.deleted_at IS NULL
		), automation AS (
			SELECT COALESCE((
				SELECT ai_enabled FROM assistant_automation_settings WHERE user_id = $1::uuid
			), false) AS ai_enabled
		), inserted AS (
			INSERT INTO assistant_runs
				(user_id, state, request_id, preference_id, snapshot_cutoff, snapshot_total,
				 ai_status, ai_skip_reason, ruleset_version)
			SELECT $1::uuid, 'queued', NULLIF($2, ''), preference.id, scope.cutoff, scope.total,
				CASE WHEN NOT $3 THEN 'skipped' WHEN NOT automation.ai_enabled THEN 'skipped' ELSE 'pending' END,
				CASE WHEN NOT $3 THEN 'server_disabled'
				     WHEN NOT automation.ai_enabled THEN 'user_opt_out' END,
				$4
			FROM preference CROSS JOIN scope CROSS JOIN automation
			WHERE NOT EXISTS (SELECT 1 FROM existing_request)
			  AND NOT EXISTS (SELECT 1 FROM active)
			RETURNING id
		)
		SELECT id::text FROM existing_request
		UNION ALL SELECT id::text FROM inserted
		LIMIT 1
	`, userID, requestID, aiConfigured, SpecializationRulesVersion).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("analysis already running")
	}
	if err != nil {
		return "", fmt.Errorf("queue assistant analysis: %w", err)
	}
	return id, nil
}

func (r *PostgresRepository) AnalysisSnapshotTotal(ctx context.Context) (int, error) {
	var total int
	err := r.db.QueryRow(ctx, `
		SELECT count(v.id)::integer
		FROM vacancies v
		JOIN sources s ON s.code = v.source AND s.is_active
		WHERE v.is_active AND v.deleted_at IS NULL
	`).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count assistant snapshot: %w", err)
	}
	return total, nil
}

func (r *PostgresRepository) ClaimAssistantRun(ctx context.Context) (AssistantRun, bool, error) {
	var run AssistantRun
	err := r.db.QueryRow(ctx, `
		UPDATE assistant_runs
		SET state = 'running', started_at = COALESCE(started_at, now()),
			last_checked_at = now(), lease_until = now() + interval '10 minutes',
			worker_heartbeat_at=now(), worker_phase='processing',
			worker_retry_category=NULL, worker_retry_until=NULL
		WHERE id = (
			SELECT id FROM assistant_runs
			WHERE state = 'queued' OR (state = 'running' AND lease_until < now())
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		RETURNING id::text, user_id::text, preference_id::text, snapshot_cutoff,
			snapshot_total, processed, eligible, matched, ai_calls, ai_eligible, ai_succeeded,
			ai_matches, ai_reviews, ai_rejects, ai_failures, ai_skipped, skipped, ai_status, COALESCE(ai_skip_reason, ''),
			ai_http_attempts, ai_retries, ai_batches, ai_prompt_tokens, ai_completion_tokens, ai_cached_tokens,
			ai_rate_limit, ai_timeouts, ai_invalid_responses,
			ai_auth, ai_quota, ai_server, ai_network, ai_context_limit, ai_content_filter, ai_invalid_request,
			snapshot_cursor_created_at, COALESCE(snapshot_cursor_vacancy_id::text, '')
	`).Scan(&run.ID, &run.UserID, &run.PreferenceID, &run.SnapshotCutoff, &run.Total,
		&run.Processed, &run.Eligible, &run.Matched, &run.AICalls, &run.AIEligible,
		&run.AISucceeded, &run.AIMatches, &run.AIReviews, &run.AIRejects,
		&run.AIFailures, &run.AISkipped, &run.Skipped,
		&run.AIStatus, &run.AISkipReason,
		&run.AIHTTPAttempts, &run.AIRetries, &run.AIBatches, &run.AIPromptTokens,
		&run.AICompletionTokens, &run.AICachedTokens, &run.AIRateLimit, &run.AITimeouts,
		&run.AIInvalidResponses, &run.AIAuth, &run.AIQuota, &run.AIServer, &run.AINetwork,
		&run.AIContextLimit, &run.AIContentFilter, &run.AIInvalidRequest,
		&run.CursorCreatedAt, &run.CursorVacancyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AssistantRun{}, false, nil
	}
	if err != nil {
		return AssistantRun{}, false, fmt.Errorf("claim assistant run: %w", err)
	}
	return run, true, nil
}

func (r *PostgresRepository) PauseAssistantRun(ctx context.Context, runID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE assistant_runs
		SET state='paused', lease_until=NULL, finished_at=NULL, last_checked_at=now()
		WHERE id=$1::uuid
		  AND (state IN ('queued', 'running') OR (state='failed' AND processed < snapshot_total))
	`, runID)
	if err != nil {
		return fmt.Errorf("pause assistant run: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("assistant run is not active")
	}
	return nil
}

func (r *PostgresRepository) ResumeAssistantRun(ctx context.Context, runID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE assistant_runs
		SET state='queued', lease_until=NULL, finished_at=NULL, last_checked_at=now()
		WHERE id=$1::uuid AND state='paused'
	`, runID)
	if err != nil {
		return fmt.Errorf("resume assistant run: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("assistant run is not paused")
	}
	return nil
}

func (r *PostgresRepository) SupersedeAssistantRun(ctx context.Context, userID, runID string) error {
	tag, err := r.db.Exec(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		)
		UPDATE assistant_runs ar
		SET superseded_from_state=ar.state, state='superseded',
			finished_at=COALESCE(ar.finished_at, now()), superseded_at=now(),
			superseded_by_preference_id=current_preference.id,
			lease_until=NULL, last_checked_at=now(),
			error_category=CASE
				WHEN ar.preference_id <> current_preference.id THEN 'preferences_changed'
				ELSE 'ruleset_changed'
			END,
			ai_status=CASE
				WHEN ar.state='succeeded' THEN ar.ai_status
				WHEN ar.ai_calls > 0 THEN 'partial' ELSE 'skipped'
			END,
			ai_skip_reason=CASE
				WHEN ar.state='succeeded' THEN ar.ai_skip_reason
				WHEN ar.ai_calls > 0 THEN NULL ELSE 'preferences_changed'
			END
		FROM current_preference
		WHERE ar.id=$2::uuid AND ar.user_id=$1::uuid
		  AND (ar.preference_id <> current_preference.id OR ar.ruleset_version <> $3)
		  AND ar.state IN ('queued','running','paused','failed','succeeded')
		  AND (ar.state='succeeded' OR ar.processed < ar.snapshot_total)
	`, userID, runID, SpecializationRulesVersion)
	if err != nil {
		return fmt.Errorf("supersede assistant run: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("assistant run cannot be superseded")
	}
	return nil
}

func (r *PostgresRepository) CompleteAssistantRun(ctx context.Context, runID, state string, stats WorkerStats, errorCategory string) error {
	if state != "succeeded" && state != "failed" && state != "disabled" {
		return errors.New("invalid assistant run state")
	}
	_, err := r.db.Exec(ctx, `
		UPDATE assistant_runs
		SET state = $2, finished_at = now(), last_checked_at = now(),
			processed = $3, eligible = $4, matched = $5, ai_calls = $6,
			ai_matches = $7, ai_failures = $8, ai_skipped = $9,
			skipped = $10, error_category = NULLIF($11, ''), lease_until = NULL,
			ai_eligible = $12, ai_succeeded = $13, ai_status = $14,
			ai_skip_reason = NULLIF($15, ''), ai_http_attempts=$16, ai_retries=$17,
			ai_rate_limit=$18, ai_timeouts=$19, ai_invalid_responses=$20, ai_auth=$21,
			ai_quota=$22, ai_server=$23, ai_network=$24, ai_context_limit=$25,
			ai_content_filter=$26, ai_invalid_request=$27, ai_batches=$28,
			ai_prompt_tokens=$29, ai_completion_tokens=$30, ai_cached_tokens=$31,
			ai_reviews=$32, ai_rejects=$33, worker_active_batches=0,
			worker_heartbeat_at=now(), worker_phase='idle',
			worker_retry_category=NULL, worker_retry_until=NULL
		WHERE id = $1::uuid AND state='running'
	`, runID, state, stats.Processed, stats.Eligible, stats.Matched, stats.AICalls,
		stats.AIMatches, stats.AIFailures, stats.AISkipped, stats.Skipped, errorCategory,
		stats.AIEligible, stats.AISucceeded, stats.AIStatus, stats.AISkipReason,
		stats.AIHTTPAttempts, stats.AIRetries, stats.AIRateLimit, stats.AITimeouts,
		stats.AIInvalidResponses, stats.AIAuth, stats.AIQuota, stats.AIServer, stats.AINetwork,
		stats.AIContextLimit, stats.AIContentFilter, stats.AIInvalidRequest, stats.AIBatches,
		stats.AIPromptTokens, stats.AICompletionTokens, stats.AICachedTokens,
		stats.AIReviews, stats.AIRejects)
	if err != nil {
		return fmt.Errorf("complete assistant run: %w", err)
	}
	return nil
}

func (r *PostgresRepository) HeartbeatAssistantRun(
	ctx context.Context,
	runID, phase, retryCategory string,
	retryUntil *time.Time,
	activeBatches, concurrency int,
) error {
	switch phase {
	case "processing", "provider_request", "backoff", "stopping":
	default:
		phase = "processing"
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE assistant_runs
		SET worker_heartbeat_at=now(), worker_phase=$2,
			worker_retry_category=NULLIF($3, ''), worker_retry_until=$4,
			worker_active_batches=$5, worker_concurrency=$6,
			lease_until=now() + interval '10 minutes'
		WHERE id=$1::uuid AND state='running'
	`, runID, phase, retryCategory, retryUntil, max(activeBatches, 0), max(concurrency, 0))
	if err != nil {
		return fmt.Errorf("heartbeat assistant run: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("assistant run lease is no longer owned")
	}
	return nil
}

func (r *PostgresRepository) UpsertWorkerProcessHeartbeat(
	ctx context.Context,
	heartbeat WorkerProcessHeartbeat,
) error {
	_, err := r.db.Exec(ctx, `
		WITH pruned AS (
			DELETE FROM assistant_worker_instances
			WHERE instance_id <> $1::uuid
			  AND last_seen_at < clock_timestamp() - $6::interval
			RETURNING 1
		)
		INSERT INTO assistant_worker_instances
			(instance_id, started_at, version, mode, state, last_seen_at)
		SELECT $1::uuid, $2, $3, $4, $5, clock_timestamp()
		FROM (SELECT count(*) FROM pruned) AS cleanup
		ON CONFLICT (instance_id) DO UPDATE SET
			version=EXCLUDED.version, mode=EXCLUDED.mode, state=EXCLUDED.state,
			last_seen_at=clock_timestamp()
	`, heartbeat.InstanceID, heartbeat.StartedAt, heartbeat.Version, heartbeat.Mode,
		heartbeat.State, fmt.Sprintf("%f seconds", WorkerAvailabilityTTL.Seconds()))
	if err != nil {
		return fmt.Errorf("upsert assistant worker process heartbeat: %w", err)
	}
	return nil
}

func (r *PostgresRepository) StopWorkerProcess(ctx context.Context, instanceID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE assistant_worker_instances
		SET state='stopping', last_seen_at=clock_timestamp()
		WHERE instance_id=$1::uuid
	`, instanceID)
	if err != nil {
		return fmt.Errorf("stop assistant worker process: %w", err)
	}
	return nil
}

func validatePreferences(p PreferenceRecord) ([]byte, []byte, []byte, error) {
	if len([]rune(p.Note)) > 2000 {
		return nil, nil, nil, fmt.Errorf("%w: note is too long", ErrInvalidPreferences)
	}
	if p.HardCriteria == nil {
		p.HardCriteria = map[string]any{}
	}
	if p.SoftCriteria == nil {
		p.SoftCriteria = map[string]any{}
	}
	if p.Weights == nil {
		p.Weights = map[string]float64{}
	}
	for key := range p.HardCriteria {
		switch key {
		case "approved_roles", "specialization", "include_leadership", "regions", "required_skills", "excluded_skills", "remote_only", "min_salary_rub":
		default:
			return nil, nil, nil, fmt.Errorf("%w: hard_criteria.%s is unsupported; use approved_roles", ErrInvalidPreferences, key)
		}
	}
	if err := validateApprovedRoles(p.HardCriteria["approved_roles"]); err != nil {
		return nil, nil, nil, err
	}
	if err := validateSpecialization(p.HardCriteria["specialization"]); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: hard_criteria.specialization is unsupported", ErrInvalidPreferences)
	}
	if value, exists := p.HardCriteria["include_leadership"]; exists {
		if _, ok := value.(bool); !ok {
			return nil, nil, nil, fmt.Errorf("%w: hard_criteria.include_leadership must be boolean", ErrInvalidPreferences)
		}
	}
	for key := range p.Weights {
		switch key {
		case "role", "salary", "region", "skills":
		default:
			return nil, nil, nil, fmt.Errorf("%w: weights.%s is unsupported", ErrInvalidPreferences, key)
		}
	}
	for key := range p.SoftCriteria {
		return nil, nil, nil, fmt.Errorf("%w: soft_criteria.%s is not used by deterministic matcher", ErrInvalidPreferences, key)
	}
	hard, err := json.Marshal(p.HardCriteria)
	if err != nil || len(hard) > 32*1024 {
		return nil, nil, nil, fmt.Errorf("%w: invalid hard criteria", ErrInvalidPreferences)
	}
	soft, err := json.Marshal(p.SoftCriteria)
	if err != nil || len(soft) > 32*1024 {
		return nil, nil, nil, fmt.Errorf("%w: invalid soft criteria", ErrInvalidPreferences)
	}
	weights, err := json.Marshal(p.Weights)
	if err != nil || len(weights) > 8*1024 {
		return nil, nil, nil, fmt.Errorf("%w: invalid weights", ErrInvalidPreferences)
	}
	return hard, soft, weights, nil
}

// SavePreferences appends exactly one immutable version per request. The
// caller may retry safely using requestID; an empty requestID means no replay
// key was supplied.
func (r *PostgresRepository) SavePreferences(ctx context.Context, userID, requestID string, p PreferenceRecord) (PreferenceRecord, error) {
	p, _, err := NormalizePreferenceRoles(p)
	if err != nil {
		return PreferenceRecord{}, err
	}
	hard, soft, weights, err := validatePreferences(p)
	if err != nil {
		return PreferenceRecord{}, err
	}
	var result PreferenceRecord
	var resultHard, resultSoft, resultWeights []byte
	requestID = strings.TrimSpace(requestID)
	query := `
		WITH locked AS (
			SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
		), next_version AS (
			SELECT COALESCE(max(version), 0) + 1 AS version
			FROM vacancy_preferences CROSS JOIN locked
			WHERE user_id = $1::uuid
		), inserted AS (
			INSERT INTO vacancy_preferences
				(user_id, version, note, hard_criteria, soft_criteria, weights)
			SELECT $1::uuid, version, $2, $3::jsonb, $4::jsonb, $5::jsonb
			FROM next_version
			RETURNING id::text, version, note, hard_criteria, soft_criteria, weights, active_from
		)
		SELECT id::text, version, note, hard_criteria, soft_criteria, weights, active_from FROM inserted
	`
	if requestID != "" {
		query = `
			WITH locked AS (
				SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
			), existing AS (
				SELECT p.id::text, p.version, p.note, p.hard_criteria, p.soft_criteria, p.weights, p.active_from
				FROM assistant_preference_requests q
				JOIN vacancy_preferences p ON p.id = q.preference_id
				CROSS JOIN locked
				WHERE q.user_id = $1::uuid AND q.request_id = $2
			), next_version AS (
				SELECT COALESCE(max(version), 0) + 1 AS version FROM vacancy_preferences
				WHERE user_id = $1::uuid AND NOT EXISTS (SELECT 1 FROM existing)
			), inserted AS (
				INSERT INTO vacancy_preferences
					(user_id, version, note, hard_criteria, soft_criteria, weights)
				SELECT $1::uuid, version, $3, $4::jsonb, $5::jsonb, $6::jsonb
				FROM next_version
				WHERE NOT EXISTS (SELECT 1 FROM existing)
				RETURNING id, version, note, hard_criteria, soft_criteria, weights, active_from
			), recorded AS (
				INSERT INTO assistant_preference_requests (user_id, request_id, preference_id)
				SELECT $1::uuid, $2, id FROM inserted
				RETURNING preference_id
			)
			SELECT id, version, note, hard_criteria, soft_criteria, weights, active_from FROM existing
			UNION ALL
			SELECT id::text, version, note, hard_criteria, soft_criteria, weights, active_from FROM inserted
		`
		err = r.db.QueryRow(ctx, query, userID, requestID, p.Note, string(hard), string(soft), string(weights)).
			Scan(&result.ID, &result.Version, &result.Note, &resultHard, &resultSoft, &resultWeights, &result.ActiveFrom)
	} else {
		err = r.db.QueryRow(ctx, query, userID, p.Note, string(hard), string(soft), string(weights)).
			Scan(&result.ID, &result.Version, &result.Note, &resultHard, &resultSoft, &resultWeights, &result.ActiveFrom)
	}
	if err != nil {
		return PreferenceRecord{}, fmt.Errorf("save assistant preferences: %w", err)
	}
	if err := json.Unmarshal(resultHard, &result.HardCriteria); err != nil {
		return PreferenceRecord{}, err
	}
	if err := json.Unmarshal(resultSoft, &result.SoftCriteria); err != nil {
		return PreferenceRecord{}, err
	}
	if err := json.Unmarshal(resultWeights, &result.Weights); err != nil {
		return PreferenceRecord{}, err
	}
	return result, nil
}

func (r *PostgresRepository) ListMatches(ctx context.Context, userID string, limit int) ([]MatchRecord, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		WITH current_preference AS (
			SELECT id FROM vacancy_preferences
			WHERE user_id=$1::uuid AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		), scoped AS (
			SELECT m.vacancy_id, m.decision, m.score, m.method, m.confidence, m.rationale,
				m.unknowns, m.conflicts, m.evidence_ids, m.created_at, m.vacancy_revision,
				v.title, v.source_url
			FROM vacancy_match_results m
			JOIN vacancies v ON v.id = m.vacancy_id
			JOIN current_preference p ON p.id=m.preference_id
			WHERE m.user_id = $1::uuid
			  AND m.ruleset_version=$3
			  AND m.decision IN ('match','review')
			  AND (
				m.run_id IS NULL
				OR EXISTS (
					SELECT 1 FROM assistant_runs ar
					WHERE ar.id = m.run_id
					  AND ar.preference_id = p.id
					  AND ar.ruleset_version = $3
					  AND ar.state NOT IN ('superseded','disabled','failed')
				)
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM vacancy_match_results hard
				WHERE hard.user_id=m.user_id AND hard.preference_id=m.preference_id
				  AND hard.vacancy_id=m.vacancy_id AND hard.vacancy_revision=m.vacancy_revision
				  AND hard.ruleset_version=$3
				  AND hard.decision = 'reject'
			  )
		), ranked AS (
			SELECT vacancy_id::text, title, source_url, decision, score::float8, method,
				CASE WHEN method='ai' AND decision='match' THEN 'confirmed' ELSE 'preliminary' END AS stage,
				confidence, rationale, unknowns, conflicts, evidence_ids,
				ROW_NUMBER() OVER (
					PARTITION BY vacancy_id
					ORDER BY CASE WHEN decision='match' THEN 0 ELSE 1 END,
						CASE WHEN method='ai' THEN 0 ELSE 1 END,
						created_at DESC
				) AS rn
			FROM scoped
		)
		SELECT vacancy_id, title, source_url, decision, score, method, stage,
			confidence, rationale, unknowns, conflicts, evidence_ids
		FROM ranked
		WHERE rn=1
		ORDER BY CASE WHEN decision='match' THEN 0 ELSE 1 END, vacancy_id
		LIMIT $2
	`, userID, limit, SpecializationRulesVersion)
	if err != nil {
		return nil, fmt.Errorf("list assistant matches: %w", err)
	}
	defer rows.Close()
	result := make([]MatchRecord, 0)
	for rows.Next() {
		var item MatchRecord
		var rationale string
		var confidence *string
		var unknowns, conflicts, evidence []byte
		if err := rows.Scan(&item.VacancyID, &item.Title, &item.SourceURL, &item.Decision, &item.Score,
			&item.Method, &item.Stage, &confidence, &rationale, &unknowns, &conflicts, &evidence); err != nil {
			return nil, fmt.Errorf("scan assistant match: %w", err)
		}
		item.Confidence = confidence
		if err := json.Unmarshal(unknowns, &item.Unknowns); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(conflicts, &item.Conflicts); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(evidence, &item.Evidence); err != nil {
			return nil, err
		}
		item.Reasons = safeMatchReasons(item.Method, rationale, item.Evidence, item.Conflicts, item.Unknowns)
		result = append(result, item)
	}
	return result, rows.Err()
}

func safeMatchReasons(method, _ string, evidence, conflicts, unknowns []string) []string {
	if method == "ai" {
		labels := []struct{ prefix, text string }{
			{"criterion:specialization:", "Frontend подтверждён"},
			{"criterion:required_skill:react:", "React подтверждён явно"},
			{"criterion:remote:", "Удалённый формат подтверждён"},
			{"criterion:leadership:", "Руководящая роль исключена"},
		}
		reasons := make([]string, 0, len(labels))
		for _, label := range labels {
			for _, value := range evidence {
				if strings.HasPrefix(value, label.prefix) {
					reasons = append(reasons, label.text)
					break
				}
			}
		}
		if len(reasons) > 0 {
			return reasons
		}
		return []string{"AI подтвердил hard-критерии"}
	}
	reviewReasons := map[string]string{
		"remote":                          "Удалённый формат неизвестен",
		"role":                            "Официальная роль не подтверждена",
		"specialization":                  "Специализация неясна",
		"specialization_description_only": "Специализация только из описания",
		"required_skill:React":            "Явный React не подтверждён в title/навыках",
		"required_skill:react":            "Явный React не подтверждён в title/навыках",
	}
	reasons := make([]string, 0, len(unknowns))
	for _, unknown := range unknowns {
		if text, ok := reviewReasons[unknown]; ok {
			reasons = append(reasons, text)
			continue
		}
		if strings.HasPrefix(unknown, "required_skill:") {
			reasons = append(reasons, "Обязательный навык не подтверждён явно")
		}
	}
	for _, value := range evidence {
		if strings.HasPrefix(value, "specialization:") {
			reasons = append(reasons, "Специализация подтверждена")
			break
		}
	}
	if len(reasons) > 0 {
		return reasons
	}
	if len(conflicts) == 0 {
		return []string{"Предварительно подходит по фильтрам"}
	}
	return []string{}
}

func splitRationale(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	return []string{value}
}

func (r *PostgresRepository) TelegramStatus(ctx context.Context, userID string, configured bool) (TelegramStatus, error) {
	var linked, optedIn bool
	err := r.db.QueryRow(ctx, `
		SELECT (revoked_at IS NULL AND linked_at IS NOT NULL), opted_in
		FROM telegram_connections WHERE user_id = $1::uuid
	`, userID).Scan(&linked, &optedIn)
	if errors.Is(err, pgx.ErrNoRows) {
		return TelegramStatus{Configured: configured}, nil
	}
	if err != nil {
		return TelegramStatus{}, fmt.Errorf("get telegram status: %w", err)
	}
	var pending, sent, failed, dead int
	var lastError *string
	_ = r.db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status IN ('pending','failed') AND dead_letter_at IS NULL),
		count(*) FILTER (WHERE status='sent'), count(*) FILTER (WHERE status='failed' AND dead_letter_at IS NULL),
		count(*) FILTER (WHERE dead_letter_at IS NOT NULL), (array_agg(last_error ORDER BY updated_at DESC)
		FILTER (WHERE last_error IS NOT NULL))[1] FROM telegram_deliveries WHERE user_id=$1::uuid`, userID).
		Scan(&pending, &sent, &failed, &dead, &lastError)
	return TelegramStatus{Configured: configured, Linked: linked, OptedIn: optedIn,
		Pending: pending, Sent: sent, Failed: failed, DeadLettered: dead, LastError: sanitizeStatusError(lastError)}, nil
}

func sanitizeStatusError(value *string) string {
	if value == nil {
		return ""
	}
	switch {
	case strings.Contains(*value, "timeout"):
		return "Telegram request timeout (outcome ambiguous)"
	case strings.HasPrefix(*value, "telegram status "):
		return *value
	default:
		return "Telegram request failed"
	}
}

func (r *PostgresRepository) IssueTelegramLink(ctx context.Context, userID, token string, expires time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO telegram_link_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2::uuid, $3)
	`, LinkTokenHash(token), userID, expires)
	return err
}

func (r *PostgresRepository) ConfirmTelegramLink(ctx context.Context, token string, chatID int64) error {
	if chatID == 0 {
		return errors.New("telegram chat id is required")
	}
	tag, err := r.db.Exec(ctx, `
		WITH consumed AS (
			UPDATE telegram_link_tokens
			SET used_at = now()
			WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
			RETURNING user_id
		)
		INSERT INTO telegram_connections (user_id, chat_id, linked_at, revoked_at, opted_in, updated_at)
		SELECT user_id, $2, now(), NULL, false, now() FROM consumed
		ON CONFLICT (user_id) DO UPDATE SET chat_id=EXCLUDED.chat_id,
			linked_at=EXCLUDED.linked_at, revoked_at=NULL, opted_in=false, updated_at=now()
	`, LinkTokenHash(token), chatID)
	if err != nil {
		return fmt.Errorf("confirm telegram link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("invalid or expired telegram link")
	}
	return nil
}

func (r *PostgresRepository) RevokeTelegram(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE telegram_connections SET revoked_at=now(), opted_in=false, updated_at=now()
		WHERE user_id=$1::uuid
	`, userID)
	return err
}

func (r *PostgresRepository) SetTelegramOptIn(ctx context.Context, userID string, optedIn bool) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE telegram_connections
		SET opted_in = $2, updated_at = now()
		WHERE user_id = $1::uuid AND revoked_at IS NULL AND linked_at IS NOT NULL
	`, userID, optedIn)
	if err != nil {
		return fmt.Errorf("set telegram opt-in: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("telegram connection is not verified")
	}
	return nil
}

func (r *PostgresRepository) AutomationSettings(ctx context.Context, userID string) (AutomationSettings, error) {
	var settings AutomationSettings
	err := r.db.QueryRow(ctx, `
		SELECT ai_enabled, telegram_enabled, activation_at
		FROM assistant_automation_settings WHERE user_id = $1::uuid
	`, userID).Scan(&settings.AIEnabled, &settings.TelegramEnabled, &settings.ActivationAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AutomationSettings{}, nil
	}
	if err != nil {
		return AutomationSettings{}, fmt.Errorf("get assistant automation settings: %w", err)
	}
	return settings, nil
}

func (r *PostgresRepository) SaveAutomationSettings(ctx context.Context, userID string, settings AutomationSettings) (AutomationSettings, error) {
	if settings.AIEnabled && settings.ActivationAt == nil {
		now := time.Now().UTC()
		settings.ActivationAt = &now
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO assistant_automation_settings
			(user_id, ai_enabled, telegram_enabled, activation_at, max_ai_calls_per_hour)
		VALUES ($1::uuid, $2, $3, $4, 0)
		ON CONFLICT (user_id) DO UPDATE SET
			ai_enabled = EXCLUDED.ai_enabled,
			telegram_enabled = EXCLUDED.telegram_enabled,
			activation_at = CASE
				WHEN EXCLUDED.ai_enabled AND NOT assistant_automation_settings.ai_enabled
					THEN COALESCE(EXCLUDED.activation_at, now())
				WHEN NOT EXCLUDED.ai_enabled THEN NULL
				ELSE assistant_automation_settings.activation_at
			END,
			max_ai_calls_per_hour = 0,
			updated_at = now()
	`, userID, settings.AIEnabled, settings.TelegramEnabled, settings.ActivationAt)
	if err != nil {
		return AutomationSettings{}, fmt.Errorf("save assistant automation settings: %w", err)
	}
	return r.AutomationSettings(ctx, userID)
}

func (r *PostgresRepository) Users(ctx context.Context) ([]WorkerUser, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id::text, p.id::text, p.version, p.note, p.hard_criteria,
			p.soft_criteria, p.weights, p.active_from
		FROM assistant_users u
		JOIN LATERAL (
			SELECT id, version, note, hard_criteria, soft_criteria, weights, active_from
			FROM vacancy_preferences WHERE user_id = u.id AND archived_at IS NULL
			ORDER BY version DESC LIMIT 1
		) p ON true
		ORDER BY u.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list assistant users: %w", err)
	}
	defer rows.Close()
	users := make([]WorkerUser, 0)
	for rows.Next() {
		var user WorkerUser
		var preferenceID string
		var hard, soft, weights []byte
		if err := rows.Scan(&user.ID, &preferenceID, &user.Preference.Version, &user.Preference.Note,
			&hard, &soft, &weights, &user.Preference.ActiveFrom); err != nil {
			return nil, fmt.Errorf("scan assistant user: %w", err)
		}
		if err := json.Unmarshal(hard, &user.Preference.HardCriteria); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(soft, &user.Preference.SoftCriteria); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(weights, &user.Preference.Weights); err != nil {
			return nil, err
		}
		normalized, _, err := NormalizePreferenceRoles(user.Preference)
		if err != nil {
			return nil, err
		}
		user.Preference = normalized
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *PostgresRepository) UsersForAssistantRun(ctx context.Context, run AssistantRun) ([]WorkerUser, error) {
	var user WorkerUser
	var hard, soft, weights []byte
	err := r.db.QueryRow(ctx, `
		SELECT p.user_id::text, p.version, p.note, p.hard_criteria,
			p.soft_criteria, p.weights, p.active_from
		FROM vacancy_preferences p
		WHERE p.id = $1::uuid AND p.user_id = $2::uuid
	`, run.PreferenceID, run.UserID).Scan(&user.ID, &user.Preference.Version, &user.Preference.Note,
		&hard, &soft, &weights, &user.Preference.ActiveFrom)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load assistant run preference: %w", err)
	}
	if err := json.Unmarshal(hard, &user.Preference.HardCriteria); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(soft, &user.Preference.SoftCriteria); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(weights, &user.Preference.Weights); err != nil {
		return nil, err
	}
	normalized, _, err := NormalizePreferenceRoles(user.Preference)
	if err != nil {
		return nil, err
	}
	user.Preference = normalized
	return []WorkerUser{user}, nil
}

func (r *PostgresRepository) SnapshotCandidates(ctx context.Context, run AssistantRun, limit int) ([]WorkerCandidate, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
		SELECT v.id::text, v.source, v.external_id, v.title, v.source_url,
			v.salary_mid, COALESCE(ra.pattern, v.role_id::text),
			COALESCE(array_agg(DISTINCT scope_alias.pattern)
				FILTER (WHERE scope_alias.pattern IS NOT NULL), '{}'),
			v.region_id::text, v.is_remote,
			v.published_at, v.collected_at, v.created_at, COALESCE(v.description_text, ''),
			v.description_truncated, v.analysis_revision,
			COALESCE(array_agg(DISTINCT sk.slug) FILTER (WHERE sk.slug IS NOT NULL), '{}')
			  || COALESCE(array_agg(DISTINCT sk.name) FILTER (WHERE sk.name IS NOT NULL), '{}'),
			EXISTS (
				SELECT 1 FROM assistant_ai_jobs j
				WHERE j.user_id=$5::uuid AND j.preference_id=$6::uuid AND j.vacancy_id=v.id
				  AND j.vacancy_revision=v.analysis_revision AND j.status='failed'
				  AND j.error_code='provider_failed' AND j.attempts < 5
			) AS ai_retry
		FROM vacancies v
		JOIN sources src ON src.code = v.source AND src.is_active
		LEFT JOIN role_aliases ra ON ra.role_id = v.role_id AND ra.source = v.source
			AND ra.pattern ~ '^[0-9]+$'
		LEFT JOIN vacancy_role_scopes vrs ON vrs.vacancy_id = v.id
			AND vrs.scope = 'vacancy_listing'
		LEFT JOIN role_aliases scope_alias ON scope_alias.role_id = vrs.role_id
			AND scope_alias.source = v.source AND scope_alias.pattern ~ '^[0-9]+$'
		LEFT JOIN vacancy_skills vs ON vs.vacancy_id = v.id
		LEFT JOIN skills sk ON sk.id = vs.skill_id
		WHERE v.is_active AND v.deleted_at IS NULL AND v.created_at <= $1
		  AND (
			$2::timestamptz IS NULL OR (v.created_at, v.id) >
				($2::timestamptz, NULLIF($3, '')::uuid)
			OR EXISTS (
				SELECT 1 FROM assistant_ai_jobs j
				WHERE j.user_id=$5::uuid AND j.preference_id=$6::uuid AND j.vacancy_id=v.id
				  AND j.vacancy_revision=v.analysis_revision AND j.status='failed'
				  AND j.error_code='provider_failed' AND j.attempts < 5
			)
		  )
		GROUP BY v.id, ra.pattern
		ORDER BY v.created_at, v.id
		LIMIT $4
	`, run.SnapshotCutoff, run.CursorCreatedAt, run.CursorVacancyID, limit, run.UserID, run.PreferenceID)
	if err != nil {
		return nil, fmt.Errorf("list assistant snapshot candidates: %w", err)
	}
	defer rows.Close()
	candidates := make([]WorkerCandidate, 0, limit)
	for rows.Next() {
		var c WorkerCandidate
		var salary *float64
		var sourceURL, roleID, regionID *string
		var isRemote *bool
		var published *time.Time
		var roleIDs, skills []string
		if err := rows.Scan(&c.ID, &c.Source, &c.ExternalID, &c.Title, &sourceURL,
			&salary, &roleID, &roleIDs, &regionID, &isRemote, &published, &c.ObservedAt, &c.CreatedAt,
			&c.Description, &c.DescriptionTruncated, &c.Revision, &skills, &c.AIRetry); err != nil {
			return nil, fmt.Errorf("scan assistant snapshot candidate: %w", err)
		}
		if sourceURL != nil {
			c.SourceURL = *sourceURL
		}
		c.Vacancy = Vacancy{
			ID: c.ID, Title: c.Title, RoleIDs: roleIDs, SalaryRUB: salary,
			IsRemote: isRemote, PublishedAt: published, Skills: skills, Description: c.Description,
		}
		if roleID != nil {
			c.Vacancy.RoleID = *roleID
		}
		if regionID != nil {
			c.Vacancy.RegionID = *regionID
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func (r *PostgresRepository) UpdateAssistantRunProgress(
	ctx context.Context,
	runID string,
	stats WorkerStats,
	last *WorkerCandidate,
) error {
	var cursorAt any
	var cursorID any
	if last != nil {
		cursorAt = last.CreatedAt
		cursorID = last.ID
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE assistant_runs SET processed=$2, eligible=$3, matched=$4, ai_calls=$5,
			ai_matches=$6, ai_failures=$7, ai_skipped=$8, skipped=$9,
			snapshot_cursor_created_at=COALESCE($10, snapshot_cursor_created_at),
			snapshot_cursor_vacancy_id=COALESCE($11::uuid, snapshot_cursor_vacancy_id),
			last_checked_at=now(), lease_until=now() + interval '10 minutes',
			ai_eligible=$12, ai_succeeded=$13, ai_status=$14,
			ai_skip_reason=NULLIF($15, ''), ai_http_attempts=$16, ai_retries=$17,
			ai_rate_limit=$18, ai_timeouts=$19, ai_invalid_responses=$20, ai_auth=$21,
			ai_quota=$22, ai_server=$23, ai_network=$24, ai_context_limit=$25,
			ai_content_filter=$26, ai_invalid_request=$27, ai_batches=$28,
			ai_prompt_tokens=$29, ai_completion_tokens=$30, ai_cached_tokens=$31,
			ai_reviews=$32, ai_rejects=$33
		WHERE id=$1::uuid AND state='running'
	`, runID, stats.Processed, stats.Eligible, stats.Matched, stats.AICalls,
		stats.AIMatches, stats.AIFailures, stats.AISkipped, stats.Skipped, cursorAt, cursorID,
		stats.AIEligible, stats.AISucceeded, stats.AIStatus, stats.AISkipReason,
		stats.AIHTTPAttempts, stats.AIRetries, stats.AIRateLimit, stats.AITimeouts,
		stats.AIInvalidResponses, stats.AIAuth, stats.AIQuota, stats.AIServer, stats.AINetwork,
		stats.AIContextLimit, stats.AIContentFilter, stats.AIInvalidRequest, stats.AIBatches,
		stats.AIPromptTokens, stats.AICompletionTokens, stats.AICachedTokens,
		stats.AIReviews, stats.AIRejects)
	if err != nil {
		return fmt.Errorf("update assistant run progress: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("assistant run lease is no longer owned")
	}
	return nil
}

func (r *PostgresRepository) Candidates(ctx context.Context, source string, _ time.Time, limit int) ([]WorkerCandidate, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
		WITH recovered AS (
			UPDATE assistant_work_items
			SET status = 'pending', lease_until = NULL, claimed_by = NULL, updated_at = now()
			WHERE status = 'processing' AND lease_until < now()
			RETURNING id
		), superseded AS (
			UPDATE assistant_work_items w
			SET status = 'done', completed_at = now(), updated_at = now()
			FROM vacancies v
			WHERE w.source = v.source AND w.external_id = v.external_id
			  AND w.status IN ('pending', 'failed')
			  AND w.vacancy_revision < v.analysis_revision
			RETURNING w.id
		), claimed AS (
			UPDATE assistant_work_items
			SET status = 'processing', lease_until = now() + interval '10 minutes',
			    claimed_by = 'assistant-worker', updated_at = now()
			WHERE id IN (
				SELECT id FROM assistant_work_items
				WHERE source = $1 AND status IN ('pending', 'failed')
				  AND available_at <= now()
				ORDER BY available_at, id
				LIMIT $2
				FOR UPDATE SKIP LOCKED
			)
			RETURNING source, external_id, vacancy_revision
		)
		SELECT v.id::text, v.source, v.external_id, v.title, v.source_url,
			v.salary_mid, COALESCE(ra.pattern, v.role_id::text),
			COALESCE(array_agg(DISTINCT scope_alias.pattern)
				FILTER (WHERE scope_alias.pattern IS NOT NULL), '{}'),
			v.region_id::text, v.is_remote, v.published_at,
			v.collected_at, COALESCE(v.description_text, ''), v.description_truncated,
			v.analysis_revision,
			COALESCE(array_agg(DISTINCT s.slug) FILTER (WHERE s.slug IS NOT NULL), '{}')
			  || COALESCE(array_agg(DISTINCT s.name) FILTER (WHERE s.name IS NOT NULL), '{}')
		FROM vacancies v
		JOIN claimed w ON w.source = v.source AND w.external_id = v.external_id
			AND w.vacancy_revision = v.analysis_revision
		LEFT JOIN role_aliases ra ON ra.role_id = v.role_id AND ra.source = 'hh'
			AND ra.pattern ~ '^[0-9]+$'
		LEFT JOIN vacancy_role_scopes vrs ON vrs.vacancy_id = v.id
			AND vrs.scope = 'vacancy_listing'
		LEFT JOIN role_aliases scope_alias ON scope_alias.role_id = vrs.role_id
			AND scope_alias.source = v.source AND scope_alias.pattern ~ '^[0-9]+$'
		LEFT JOIN vacancy_skills vs ON vs.vacancy_id = v.id
		LEFT JOIN skills s ON s.id = vs.skill_id
		WHERE v.source = $1 AND v.is_active AND v.deleted_at IS NULL
		GROUP BY v.id, ra.pattern
		ORDER BY v.collected_at, v.external_id
		LIMIT $2
	`, source, limit)
	if err != nil {
		return nil, fmt.Errorf("list assistant candidates: %w", err)
	}
	defer rows.Close()
	candidates := make([]WorkerCandidate, 0, limit)
	for rows.Next() {
		var c WorkerCandidate
		var salary *float64
		var sourceURL *string
		var roleID, regionID *string
		var isRemote *bool
		var published *time.Time
		var roleIDs, skills []string
		if err := rows.Scan(&c.ID, &c.Source, &c.ExternalID, &c.Title, &sourceURL,
			&salary, &roleID, &roleIDs, &regionID, &isRemote, &published, &c.ObservedAt, &c.Description,
			&c.DescriptionTruncated, &c.Revision, &skills); err != nil {
			return nil, fmt.Errorf("scan assistant candidate: %w", err)
		}
		if sourceURL != nil {
			c.SourceURL = *sourceURL
		}
		c.Vacancy = Vacancy{
			ID: c.ID, Title: c.Title, RoleIDs: roleIDs, SalaryRUB: salary,
			IsRemote: isRemote, PublishedAt: published, Skills: skills, Description: c.Description,
		}
		if roleID != nil {
			c.Vacancy.RoleID = *roleID
		}
		if regionID != nil {
			c.Vacancy.RegionID = *regionID
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func (r *PostgresRepository) CompleteWorkItem(ctx context.Context, source, externalID string, revision int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE assistant_work_items
		SET status = 'done', completed_at = now(), lease_until = NULL, updated_at = now()
		WHERE source = $1 AND external_id = $2 AND vacancy_revision = $3 AND status = 'processing'
	`, source, externalID, max(revision, 1))
	return err
}

func (r *PostgresRepository) RetryWorkItem(
	ctx context.Context,
	source, externalID string,
	revision int,
	errorCode string,
) error {
	_, err := r.db.Exec(ctx, `
		UPDATE assistant_work_items
		SET status = CASE WHEN attempts + 1 >= 5 THEN 'done' ELSE 'failed' END,
			attempts = LEAST(attempts + 1, 20),
			available_at = now() + make_interval(secs => LEAST(300, 5 * (1 << LEAST(attempts, 5)))),
			last_error = $4, dead_letter_at = CASE WHEN attempts + 1 >= 5 THEN now() ELSE NULL END,
			lease_until = NULL, claimed_by = NULL, updated_at = now()
		WHERE source = $1 AND external_id = $2 AND vacancy_revision = $3 AND status = 'processing'
	`, source, externalID, max(revision, 1), errorCode)
	return err
}

func (r *PostgresRepository) DeferWorkItem(
	ctx context.Context,
	source, externalID string,
	revision int,
	availableAt time.Time,
) error {
	_, err := r.db.Exec(ctx, `
		UPDATE assistant_work_items
		SET status = 'pending', available_at = $4, lease_until = NULL,
			claimed_by = NULL, updated_at = now()
		WHERE source = $1 AND external_id = $2 AND vacancy_revision = $3
		  AND status = 'processing'
	`, source, externalID, max(revision, 1), availableAt)
	return err
}

func (r *PostgresRepository) SaveMatch(ctx context.Context, match WorkerMatch) (bool, error) {
	unknowns, _ := json.Marshal(match.Result.Unknowns)
	conflicts, _ := json.Marshal(match.Result.Conflicts)
	evidence, _ := json.Marshal(match.Result.Evidence)
	var created bool
	err := r.db.QueryRow(ctx, `
		INSERT INTO vacancy_match_results
			(user_id, preference_id, vacancy_id, decision, method, score, rationale,
			 evidence_ids, conflicts, unknowns, provider, model, prompt_version, input_snapshot_hash,
			 vacancy_revision, run_id, ruleset_version)
		SELECT $1::uuid, p.id, $2::uuid, $3, COALESCE(NULLIF($10, ''), 'deterministic'), $4, $5,
			$6::jsonb, $7::jsonb, $8::jsonb, NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14, $15,
			NULLIF($16, '')::uuid, $17
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $9
		ON CONFLICT (user_id, preference_id, vacancy_id, vacancy_revision, method, ruleset_version, (COALESCE(provider, '')), (COALESCE(model, '')), (COALESCE(prompt_version, '')))
		DO UPDATE SET
			run_id = COALESCE(EXCLUDED.run_id, vacancy_match_results.run_id),
			decision = EXCLUDED.decision,
			score = EXCLUDED.score,
			rationale = EXCLUDED.rationale,
			evidence_ids = EXCLUDED.evidence_ids,
			conflicts = EXCLUDED.conflicts,
			unknowns = EXCLUDED.unknowns,
			input_snapshot_hash = COALESCE(EXCLUDED.input_snapshot_hash, vacancy_match_results.input_snapshot_hash)
		RETURNING (xmax = 0)
	`, match.UserID, match.VacancyID, string(match.Result.Decision), match.Result.Score,
		strings.Join(match.Result.Reasons, "; "), string(evidence), string(conflicts), string(unknowns),
		match.PreferenceVersion, match.Method, match.Provider, match.Model, match.PromptVersion,
		match.InputSnapshotHash, max(match.VacancyRevision, 1), match.RunID, SpecializationRulesVersion).Scan(&created)
	if err != nil {
		return false, fmt.Errorf("save assistant match: %w", err)
	}
	return created, nil
}

func (r *PostgresRepository) SaveAIResult(ctx context.Context, match WorkerMatch, output MatchOutput) error {
	inputHash := match.InputSnapshotHash
	_, err := r.db.Exec(ctx, `
		INSERT INTO assistant_ai_jobs
			(user_id, preference_id, vacancy_id, vacancy_revision, status, provider, model,
			 input_snapshot_hash, attempts, finished_at, ruleset_version)
		SELECT $1::uuid, p.id, $2::uuid, $7, 'complete', $3, $4, $5, 1, now(), $8
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $6
		ON CONFLICT (user_id, preference_id, vacancy_id, vacancy_revision, ruleset_version) DO UPDATE SET
			status = EXCLUDED.status, provider = EXCLUDED.provider, model = EXCLUDED.model,
			input_snapshot_hash = EXCLUDED.input_snapshot_hash, attempts = assistant_ai_jobs.attempts + 1,
			error_code = NULL, finished_at = EXCLUDED.finished_at
	`, match.UserID, match.VacancyID, match.Provider, match.Model, inputHash,
		match.PreferenceVersion, max(match.VacancyRevision, 1), SpecializationRulesVersion)
	if err != nil {
		return fmt.Errorf("save assistant AI job: %w", err)
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO vacancy_match_results
			(user_id, preference_id, vacancy_id, decision, method, score, confidence,
			 rationale, evidence_ids, conflicts, unknowns, provider, model, prompt_version,
			 input_snapshot_hash, vacancy_revision, run_id, ruleset_version)
		SELECT $1::uuid, p.id, $2::uuid, $3, 'ai', $4, $5, $6, $7::jsonb, $8::jsonb,
			$9::jsonb, $10, $11, $12, $13, $15, NULLIF($16, '')::uuid, $17
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $14
		ON CONFLICT (user_id, preference_id, vacancy_id, vacancy_revision, method, ruleset_version, (COALESCE(provider, '')), (COALESCE(model, '')), (COALESCE(prompt_version, '')))
		DO UPDATE SET
			run_id = COALESCE(EXCLUDED.run_id, vacancy_match_results.run_id),
			decision = EXCLUDED.decision,
			score = EXCLUDED.score,
			confidence = EXCLUDED.confidence,
			rationale = EXCLUDED.rationale,
			evidence_ids = EXCLUDED.evidence_ids,
			conflicts = EXCLUDED.conflicts,
			unknowns = EXCLUDED.unknowns,
			input_snapshot_hash = COALESCE(EXCLUDED.input_snapshot_hash, vacancy_match_results.input_snapshot_hash)
	`, match.UserID, match.VacancyID, output.Decision, output.Score, output.Confidence,
		output.Rationale, jsonArray(output.Evidence), jsonArray(output.Conflicts),
		jsonArray(output.Unknowns), match.Provider, match.Model, match.PromptVersion, inputHash,
		match.PreferenceVersion, max(match.VacancyRevision, 1), match.RunID, SpecializationRulesVersion)
	if err != nil {
		return fmt.Errorf("save assistant AI result: %w", err)
	}
	return nil
}

func (r *PostgresRepository) AIResultExists(
	ctx context.Context,
	userID string,
	preferenceVersion int,
	vacancyID string,
	vacancyRevision int,
) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM vacancy_match_results m
			JOIN vacancy_preferences p ON p.id = m.preference_id
			WHERE m.user_id = $1::uuid AND p.version = $2 AND m.vacancy_id = $3::uuid
			  AND m.vacancy_revision = $4 AND m.method = 'ai' AND m.ruleset_version = $5
		) OR EXISTS (
			SELECT 1 FROM assistant_ai_jobs j
			JOIN vacancy_preferences p ON p.id = j.preference_id
			WHERE j.user_id = $1::uuid AND p.version = $2 AND j.vacancy_id = $3::uuid
			  AND j.vacancy_revision = $4 AND j.status = 'failed' AND j.attempts >= 5
			  AND j.ruleset_version = $5
		)
	`, userID, preferenceVersion, vacancyID, max(vacancyRevision, 1), SpecializationRulesVersion).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) SaveAIFailure(ctx context.Context, match WorkerMatch, errorCode string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO assistant_ai_jobs
			(user_id, preference_id, vacancy_id, vacancy_revision, status, provider, model,
			 input_snapshot_hash, attempts, error_code, finished_at, ruleset_version)
		SELECT $1::uuid, p.id, $2::uuid, $7, 'failed', $3, $4, $5, 1, $8, now(), $9
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $6
		ON CONFLICT (user_id, preference_id, vacancy_id, vacancy_revision, ruleset_version) DO UPDATE SET
			status = 'failed', attempts = LEAST(assistant_ai_jobs.attempts + 1, 5),
			error_code = EXCLUDED.error_code, finished_at = now()
	`, match.UserID, match.VacancyID, match.Provider, match.Model, match.InputSnapshotHash,
		match.PreferenceVersion, max(match.VacancyRevision, 1), errorCode, SpecializationRulesVersion)
	return err
}

func jsonArray(values []string) string {
	data, _ := json.Marshal(values)
	return string(data)
}

func (r *PostgresRepository) SaveDelivery(ctx context.Context, delivery WorkerDelivery) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO telegram_deliveries (user_id, preference_id, vacancy_id, status)
		SELECT $1::uuid, p.id, $2::uuid, 'pending'
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $3
		ON CONFLICT DO NOTHING
	`, delivery.UserID, delivery.VacancyID, delivery.PreferenceVersion)
	if err != nil {
		return false, fmt.Errorf("save telegram delivery: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *PostgresRepository) TelegramEligible(ctx context.Context, userID string) (bool, error) {
	var eligible bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM telegram_connections
			WHERE user_id = $1::uuid AND linked_at IS NOT NULL
			  AND revoked_at IS NULL AND opted_in
		)
	`, userID).Scan(&eligible)
	if err != nil {
		return false, fmt.Errorf("check telegram eligibility: %w", err)
	}
	return eligible, nil
}

func (r *PostgresRepository) VerifiedTelegramChat(ctx context.Context, userID string) (int64, error) {
	var chatID int64
	err := r.db.QueryRow(ctx, `SELECT chat_id FROM telegram_connections
		WHERE user_id=$1::uuid AND chat_id IS NOT NULL AND linked_at IS NOT NULL
		AND revoked_at IS NULL AND opted_in`, userID).Scan(&chatID)
	if err != nil {
		return 0, errors.New("telegram connection is not verified")
	}
	return chatID, nil
}

func (r *PostgresRepository) TryDeliveryLock(ctx context.Context) (func() error, bool, error) {
	if r.poolConn == nil {
		return nil, false, errors.New("telegram delivery requires pgxpool")
	}
	conn, err := r.poolConn.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("telegram delivery acquire connection: %w", err)
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(549004803)`).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("telegram delivery lock: %w", err)
	}
	if !acquired {
		conn.Release()
		return func() error { return nil }, false, nil
	}
	return func() error {
		defer conn.Release()
		var released bool
		if err := conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock(549004803)`).Scan(&released); err != nil {
			return err
		}
		return nil
	}, true, nil
}

func (r *PostgresRepository) ClaimDeliveries(ctx context.Context, workerID string, limit int, lease time.Duration) ([]Delivery, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	rows, err := r.db.Query(ctx, `
		WITH recovered AS (
			UPDATE telegram_deliveries SET status = 'pending', lease_until = NULL, claimed_by = NULL
			WHERE status IN ('pending','failed') AND lease_until < now()
			RETURNING id
		), claimed AS (
			UPDATE telegram_deliveries d
			SET status = 'failed', lease_until = now() + $3::interval, claimed_by = $1,
			    attempts = d.attempts + 1
			FROM telegram_connections c
			JOIN assistant_automation_settings a ON a.user_id = d.user_id AND a.telegram_enabled
			JOIN vacancy_match_results m ON m.user_id = d.user_id AND m.vacancy_id = d.vacancy_id
			    AND m.preference_id = d.preference_id AND m.decision = 'match'
			    AND m.method='ai' AND m.ruleset_version=$4
			WHERE d.user_id = c.user_id AND c.linked_at IS NOT NULL AND c.revoked_at IS NULL AND c.opted_in
			  AND d.preference_id=(
				SELECT p.id FROM vacancy_preferences p
				WHERE p.user_id=d.user_id AND p.archived_at IS NULL
				ORDER BY p.version DESC LIMIT 1
			  )
			  AND (m.run_id IS NULL OR EXISTS (
				SELECT 1 FROM assistant_runs ar
				WHERE ar.id=m.run_id AND ar.ruleset_version=$4
				  AND ar.state NOT IN ('superseded','disabled','failed')
			  ))
			  AND d.status IN ('pending','failed') AND d.dead_letter_at IS NULL
			  AND (d.next_attempt_at IS NULL OR d.next_attempt_at <= now())
			  AND (d.cooldown_until IS NULL OR d.cooldown_until <= now())
			  AND d.id IN (
				SELECT id FROM telegram_deliveries
				WHERE status IN ('pending','failed') AND dead_letter_at IS NULL
				  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
				ORDER BY created_at, id LIMIT $2 FOR UPDATE SKIP LOCKED
			  )
			RETURNING d.id, d.user_id, d.preference_id, d.vacancy_id, d.attempts, c.chat_id
		)
		SELECT x.id::text, x.user_id::text, x.vacancy_id::text, x.attempts, x.chat_id,
		       v.title, v.source_url, v.salary_from, v.salary_to, m.score::float8,
		       m.confidence, m.rationale
		FROM claimed x
		JOIN vacancies v ON v.id = x.vacancy_id
		JOIN vacancy_match_results m ON m.user_id = x.user_id AND m.vacancy_id = x.vacancy_id
		    AND m.preference_id=x.preference_id AND m.decision = 'match'
		    AND m.method='ai' AND m.ruleset_version=$4
	`, workerID, limit, fmt.Sprintf("%d seconds", int(lease.Seconds())), SpecializationRulesVersion)
	if err != nil {
		return nil, fmt.Errorf("claim telegram deliveries: %w", err)
	}
	defer rows.Close()
	result := make([]Delivery, 0, limit)
	for rows.Next() {
		var d Delivery
		var from, to *float64
		var confidence *string
		var rationale string
		if err := rows.Scan(&d.ID, &d.UserID, &d.VacancyID, &d.Attempts, &d.ChatID, &d.Title, &d.SourceURL,
			&from, &to, &d.Score, &confidence, &rationale); err != nil {
			return nil, err
		}
		if from != nil && to != nil {
			d.Salary = fmt.Sprintf("%.0f–%.0f ₽", *from, *to)
		}
		if confidence != nil {
			d.Confidence = *confidence
		}
		d.Reasons = splitRationale(rationale)
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) MarkDeliverySent(ctx context.Context, id string, messageID int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE telegram_deliveries SET status='sent', provider_message_id=$2, sent_at=now(),
			lease_until=NULL, claimed_by=NULL, last_error=NULL
		WHERE id=$1::uuid AND dead_letter_at IS NULL
	`, id, messageID)
	return err
}

func (r *PostgresRepository) MarkDeliveryFailed(ctx context.Context, id, lastError string, next time.Time, dead bool) error {
	_, err := r.db.Exec(ctx, `
		UPDATE telegram_deliveries SET status='failed', last_error=$2, next_attempt_at=$3,
			dead_letter_at=CASE WHEN $4 THEN now() ELSE dead_letter_at END,
			lease_until=NULL, claimed_by=NULL
		WHERE id=$1::uuid
	`, id, truncate(lastError, 500), next, dead)
	return err
}

func truncate(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func (r *PostgresRepository) AdvanceCursor(ctx context.Context, source string, observedAt time.Time, externalID string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO assistant_cursors (source, observed_at, external_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (source) DO UPDATE SET observed_at = EXCLUDED.observed_at,
			external_id = EXCLUDED.external_id, updated_at = now()
		WHERE (assistant_cursors.observed_at, assistant_cursors.external_id) <
			(EXCLUDED.observed_at, EXCLUDED.external_id)
	`, source, observedAt, externalID)
	return err
}

// TryLock takes a session-level advisory lock on a dedicated pooled connection.
// The connection is held until the returned release function is called.
func (r *PostgresRepository) TryLock(ctx context.Context) (func() error, bool, error) {
	if r.poolConn == nil {
		return nil, false, errors.New("assistant worker requires pgxpool")
	}
	conn, err := r.poolConn.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("assistant worker acquire connection: %w", err)
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(549004802)`).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("assistant worker lock: %w", err)
	}
	if !acquired {
		conn.Release()
		return func() error { return nil }, false, nil
	}
	return func() error {
		defer conn.Release()
		var released bool
		if err := conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock(549004802)`).Scan(&released); err != nil {
			return err
		}
		if !released {
			return errors.New("assistant worker lock was not held")
		}
		return nil
	}, true, nil
}

// TryProcessLock prevents multiple continuous assistant worker processes while
// leaving the shorter run lock independent for one-shot and operational tools.
func (r *PostgresRepository) TryProcessLock(ctx context.Context) (func() error, bool, error) {
	if r.poolConn == nil {
		return nil, false, errors.New("assistant worker process lock requires pgxpool")
	}
	conn, err := r.poolConn.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("assistant worker process acquire connection: %w", err)
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(549004804)`).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("assistant worker process lock: %w", err)
	}
	if !acquired {
		conn.Release()
		return func() error { return nil }, false, nil
	}
	return func() error {
		defer conn.Release()
		var released bool
		if err := conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock(549004804)`).Scan(&released); err != nil {
			return err
		}
		if !released {
			return errors.New("assistant worker process lock was not held")
		}
		return nil
	}, true, nil
}
