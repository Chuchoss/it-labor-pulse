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
	ID           string
	Version      int
	Note         string
	HardCriteria map[string]any
	SoftCriteria map[string]any
	Weights      map[string]float64
	ActiveFrom   time.Time
	ArchivedAt   *time.Time
}

type AnalysisStatus struct {
	AIConfigured      bool       `json:"ai_configured"`
	State             string     `json:"state"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	LastCheckedAt     time.Time  `json:"last_checked_at"`
	Processed         int        `json:"processed"`
	Eligible          int        `json:"eligible"`
	Matched           int        `json:"matched"`
	AICalls           int        `json:"ai_calls"`
	Skipped           int        `json:"skipped"`
	ErrorCategory     *string    `json:"error_category,omitempty"`
	RequestID         *string    `json:"request_id,omitempty"`
	CursorSource      string     `json:"cursor_source"`
	CursorObservedAt  *time.Time `json:"cursor_observed_at,omitempty"`
	PendingCandidates bool       `json:"pending_candidates"`
	Provider          *string    `json:"provider,omitempty"`
	Model             *string    `json:"model,omitempty"`
	PromptVersion     *string    `json:"prompt_version,omitempty"`
	MethodVersion     string     `json:"method_version"`
}

var ErrInvalidPreferences = errors.New("invalid assistant preferences")

type MatchRecord struct {
	VacancyID  string
	Title      string
	SourceURL  *string
	Decision   Decision
	Score      float64
	Method     string
	Confidence *string
	Reasons    []string
	Unknowns   []string
	Conflicts  []string
	Evidence   []string
}

type TelegramStatus struct {
	Configured                          bool
	Linked                              bool
	OptedIn                             bool
	Pending, Sent, Failed, DeadLettered int
	LastError                           string
}

type AutomationSettings struct {
	AIEnabled         bool       `json:"ai_enabled"`
	TelegramEnabled   bool       `json:"telegram_enabled"`
	ActivationAt      *time.Time `json:"activation_at,omitempty"`
	MaxAICallsPerHour int        `json:"max_ai_calls_per_hour"`
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
	return p, nil
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
	err := r.db.QueryRow(ctx, `SELECT state, started_at, finished_at, last_checked_at,
		processed, eligible, matched, ai_calls, skipped, error_category, request_id,
		cursor_source, cursor_observed_at, pending_candidates, provider, model, prompt_version
		FROM assistant_runs WHERE user_id = $1::uuid ORDER BY created_at DESC LIMIT 1`, userID).Scan(
		&s.State, &s.StartedAt, &s.FinishedAt, &s.LastCheckedAt, &s.Processed, &s.Eligible,
		&s.Matched, &s.AICalls, &s.Skipped, &s.ErrorCategory, &s.RequestID, &s.CursorSource,
		&cursorAt, &s.PendingCandidates, &s.Provider, &s.Model, &s.PromptVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		s.State = "never_run"
		s.LastCheckedAt = time.Now().UTC()
	} else if err != nil {
		return AnalysisStatus{}, fmt.Errorf("get assistant status: %w", err)
	}
	s.AIConfigured = aiConfigured
	if !aiConfigured && (s.State == "never_run" || s.State == "queued") {
		s.State = "disabled"
	}
	s.CursorObservedAt = cursorAt
	s.MethodVersion = "deterministic-v1"
	return s, nil
}

func (r *PostgresRepository) QueueAnalysis(ctx context.Context, userID, requestID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `INSERT INTO assistant_runs (user_id, state, request_id)
		SELECT $1::uuid, 'queued', NULLIF($2, '')
		WHERE NOT EXISTS (SELECT 1 FROM assistant_runs WHERE user_id = $1::uuid AND state IN ('queued','running'))
		RETURNING id::text`, userID, requestID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("analysis already running")
	}
	if err != nil {
		return "", fmt.Errorf("queue assistant analysis: %w", err)
	}
	return id, nil
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
			RETURNING version, note, hard_criteria, soft_criteria, weights, active_from
		)
		SELECT version, note, hard_criteria, soft_criteria, weights, active_from FROM inserted
	`
	if requestID != "" {
		query = `
			WITH locked AS (
				SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
			), existing AS (
				SELECT p.version, p.note, p.hard_criteria, p.soft_criteria, p.weights, p.active_from
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
			SELECT version, note, hard_criteria, soft_criteria, weights, active_from FROM existing
			UNION ALL
			SELECT version, note, hard_criteria, soft_criteria, weights, active_from FROM inserted
		`
		err = r.db.QueryRow(ctx, query, userID, requestID, p.Note, string(hard), string(soft), string(weights)).
			Scan(&result.Version, &result.Note, &resultHard, &resultSoft, &resultWeights, &result.ActiveFrom)
	} else {
		err = r.db.QueryRow(ctx, query, userID, p.Note, string(hard), string(soft), string(weights)).
			Scan(&result.Version, &result.Note, &resultHard, &resultSoft, &resultWeights, &result.ActiveFrom)
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
		SELECT m.vacancy_id::text, v.title, v.source_url, m.decision, m.score::float8,
			m.method, m.confidence, m.rationale, m.unknowns, m.conflicts, m.evidence_ids
		FROM vacancy_match_results m
		JOIN vacancies v ON v.id = m.vacancy_id
		WHERE m.user_id = $1::uuid AND m.decision = 'match'
		ORDER BY m.created_at DESC LIMIT $2
	`, userID, limit)
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
			&item.Method, &confidence, &rationale, &unknowns, &conflicts, &evidence); err != nil {
			return nil, fmt.Errorf("scan assistant match: %w", err)
		}
		item.Confidence = confidence
		item.Reasons = splitRationale(rationale)
		if err := json.Unmarshal(unknowns, &item.Unknowns); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(conflicts, &item.Conflicts); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(evidence, &item.Evidence); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
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
		SELECT ai_enabled, telegram_enabled, activation_at, max_ai_calls_per_hour
		FROM assistant_automation_settings WHERE user_id = $1::uuid
	`, userID).Scan(&settings.AIEnabled, &settings.TelegramEnabled, &settings.ActivationAt, &settings.MaxAICallsPerHour)
	if errors.Is(err, pgx.ErrNoRows) {
		return AutomationSettings{MaxAICallsPerHour: 20}, nil
	}
	if err != nil {
		return AutomationSettings{}, fmt.Errorf("get assistant automation settings: %w", err)
	}
	return settings, nil
}

func (r *PostgresRepository) SaveAutomationSettings(ctx context.Context, userID string, settings AutomationSettings) (AutomationSettings, error) {
	if settings.MaxAICallsPerHour < 1 || settings.MaxAICallsPerHour > 100 {
		settings.MaxAICallsPerHour = 20
	}
	if settings.AIEnabled && settings.ActivationAt == nil {
		now := time.Now().UTC()
		settings.ActivationAt = &now
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO assistant_automation_settings
			(user_id, ai_enabled, telegram_enabled, activation_at, max_ai_calls_per_hour)
		VALUES ($1::uuid, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			ai_enabled = EXCLUDED.ai_enabled,
			telegram_enabled = EXCLUDED.telegram_enabled,
			activation_at = CASE
				WHEN EXCLUDED.ai_enabled AND NOT assistant_automation_settings.ai_enabled
					THEN COALESCE(EXCLUDED.activation_at, now())
				WHEN NOT EXCLUDED.ai_enabled THEN NULL
				ELSE assistant_automation_settings.activation_at
			END,
			max_ai_calls_per_hour = EXCLUDED.max_ai_calls_per_hour,
			updated_at = now()
	`, userID, settings.AIEnabled, settings.TelegramEnabled, settings.ActivationAt, settings.MaxAICallsPerHour)
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
			FROM vacancy_preferences WHERE user_id = u.id
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
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *PostgresRepository) Candidates(ctx context.Context, source string, cutoff time.Time, limit int) ([]WorkerCandidate, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
		WITH recovered AS (
			UPDATE assistant_work_items
			SET status = 'pending', lease_until = NULL, claimed_by = NULL, updated_at = now()
			WHERE status = 'processing' AND lease_until < now()
			RETURNING id
		), claimed AS (
			UPDATE assistant_work_items
			SET status = 'processing', lease_until = now() + interval '10 minutes',
			    claimed_by = 'assistant-worker', updated_at = now()
			WHERE id IN (
				SELECT id FROM assistant_work_items
				WHERE source = $1 AND status IN ('pending', 'failed')
				  AND available_at <= now()
				ORDER BY available_at, id
				LIMIT $3
				FOR UPDATE SKIP LOCKED
			)
			RETURNING source, external_id
		)
		SELECT v.id::text, v.source, v.external_id, v.title, v.source_url,
			v.salary_mid, v.role_id::text, v.region_id::text, v.published_at,
			v.collected_at, COALESCE(array_agg(s.slug) FILTER (WHERE s.slug IS NOT NULL), '{}')
		FROM vacancies v
		JOIN claimed w ON w.source = v.source AND w.external_id = v.external_id
		LEFT JOIN vacancy_skills vs ON vs.vacancy_id = v.id
		LEFT JOIN skills s ON s.id = vs.skill_id
		WHERE v.source = $1 AND v.is_active AND v.deleted_at IS NULL
			AND COALESCE(v.published_at, v.collected_at) >= $2
		GROUP BY v.id
		ORDER BY v.collected_at, v.external_id
		LIMIT $3
	`, source, cutoff, limit)
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
		var published *time.Time
		var skills []string
		if err := rows.Scan(&c.ID, &c.Source, &c.ExternalID, &c.Title, &sourceURL,
			&salary, &roleID, &regionID, &published, &c.ObservedAt, &skills); err != nil {
			return nil, fmt.Errorf("scan assistant candidate: %w", err)
		}
		if sourceURL != nil {
			c.SourceURL = *sourceURL
		}
		c.Vacancy = Vacancy{ID: c.ID, Title: c.Title, SalaryRUB: salary, PublishedAt: published, Skills: skills}
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

func (r *PostgresRepository) CompleteWorkItem(ctx context.Context, source, externalID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE assistant_work_items
		SET status = 'done', completed_at = now(), lease_until = NULL, updated_at = now()
		WHERE source = $1 AND external_id = $2 AND status = 'processing'
	`, source, externalID)
	return err
}

func (r *PostgresRepository) SaveMatch(ctx context.Context, match WorkerMatch) (bool, error) {
	unknowns, _ := json.Marshal(match.Result.Unknowns)
	conflicts, _ := json.Marshal(match.Result.Conflicts)
	evidence, _ := json.Marshal(match.Result.Evidence)
	tag, err := r.db.Exec(ctx, `
		INSERT INTO vacancy_match_results
			(user_id, preference_id, vacancy_id, decision, method, score, rationale,
			 evidence_ids, conflicts, unknowns, provider, model, input_snapshot_hash)
		SELECT $1::uuid, p.id, $2::uuid, $3, COALESCE(NULLIF($10, ''), 'deterministic'), $4, $5,
			$6::jsonb, $7::jsonb, $8::jsonb, NULLIF($11, ''), NULLIF($12, ''), $13
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $9
		ON CONFLICT DO NOTHING
	`, match.UserID, match.VacancyID, string(match.Result.Decision), match.Result.Score,
		strings.Join(match.Result.Reasons, "; "), string(evidence), string(conflicts), string(unknowns),
		match.PreferenceVersion, match.Method, match.Provider, match.Model, match.InputSnapshotHash)
	if err != nil {
		return false, fmt.Errorf("save assistant match: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *PostgresRepository) SaveAIResult(ctx context.Context, match WorkerMatch, output MatchOutput) error {
	inputHash := match.InputSnapshotHash
	_, err := r.db.Exec(ctx, `
		INSERT INTO assistant_ai_jobs
			(user_id, preference_id, vacancy_id, status, provider, model, input_snapshot_hash, finished_at)
		SELECT $1::uuid, p.id, $2::uuid, 'complete', $3, $4, $5, now()
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $6
		ON CONFLICT (user_id, preference_id, vacancy_id) DO UPDATE SET
			status = EXCLUDED.status, provider = EXCLUDED.provider, model = EXCLUDED.model,
			input_snapshot_hash = EXCLUDED.input_snapshot_hash, finished_at = EXCLUDED.finished_at
	`, match.UserID, match.VacancyID, match.Provider, match.Model, inputHash, match.PreferenceVersion)
	if err != nil {
		return fmt.Errorf("save assistant AI job: %w", err)
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO vacancy_match_results
			(user_id, preference_id, vacancy_id, decision, method, score, confidence,
			 rationale, evidence_ids, conflicts, unknowns, provider, model, input_snapshot_hash)
		SELECT $1::uuid, p.id, $2::uuid, $3, 'ai', $4, $5, $6, $7::jsonb, $8::jsonb,
			$9::jsonb, $10, $11, $12
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $13
		ON CONFLICT DO NOTHING
	`, match.UserID, match.VacancyID, output.Decision, output.Score, output.Confidence,
		output.Rationale, jsonArray(output.Evidence), jsonArray(output.Conflicts),
		jsonArray(output.Unknowns), match.Provider, match.Model, inputHash, match.PreferenceVersion)
	if err != nil {
		return fmt.Errorf("save assistant AI result: %w", err)
	}
	return nil
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
			WHERE d.user_id = c.user_id AND c.linked_at IS NOT NULL AND c.revoked_at IS NULL AND c.opted_in
			  AND d.status IN ('pending','failed') AND d.dead_letter_at IS NULL
			  AND (d.next_attempt_at IS NULL OR d.next_attempt_at <= now())
			  AND (d.cooldown_until IS NULL OR d.cooldown_until <= now())
			  AND d.id IN (
				SELECT id FROM telegram_deliveries
				WHERE status IN ('pending','failed') AND dead_letter_at IS NULL
				  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
				ORDER BY created_at, id LIMIT $2 FOR UPDATE SKIP LOCKED
			  )
			RETURNING d.id, d.user_id, d.vacancy_id, d.attempts, c.chat_id
		)
		SELECT x.id::text, x.user_id::text, x.vacancy_id::text, x.attempts, x.chat_id,
		       v.title, v.source_url, v.salary_from, v.salary_to, m.score::float8,
		       m.confidence, m.rationale
		FROM claimed x
		JOIN vacancies v ON v.id = x.vacancy_id
		JOIN vacancy_match_results m ON m.user_id = x.user_id AND m.vacancy_id = x.vacancy_id
		    AND m.decision = 'match'
	`, workerID, limit, fmt.Sprintf("%d seconds", int(lease.Seconds())))
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
