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
	Version      int
	Note         string
	HardCriteria map[string]any
	SoftCriteria map[string]any
	Weights      map[string]float64
	ActiveFrom   time.Time
}

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
	Configured bool
	Linked     bool
	OptedIn    bool
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
		SELECT version, note, hard_criteria, soft_criteria, weights, active_from
		FROM vacancy_preferences
		WHERE user_id = $1::uuid
		ORDER BY version DESC LIMIT 1
	`, userID).Scan(&p.Version, &p.Note, &hard, &soft, &weights, &p.ActiveFrom)
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

func validatePreferences(p PreferenceRecord) ([]byte, []byte, []byte, error) {
	if len([]rune(p.Note)) > 2000 {
		return nil, nil, nil, errors.New("assistant note is too long")
	}
	hard, err := json.Marshal(p.HardCriteria)
	if err != nil || len(hard) > 32*1024 {
		return nil, nil, nil, errors.New("invalid hard criteria")
	}
	soft, err := json.Marshal(p.SoftCriteria)
	if err != nil || len(soft) > 32*1024 {
		return nil, nil, nil, errors.New("invalid soft criteria")
	}
	weights, err := json.Marshal(p.Weights)
	if err != nil || len(weights) > 8*1024 {
		return nil, nil, nil, errors.New("invalid weights")
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
		WITH next_version AS (
			SELECT COALESCE(max(version), 0) + 1 AS version
			FROM vacancy_preferences WHERE user_id = $1::uuid
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
	return TelegramStatus{Configured: configured, Linked: linked, OptedIn: optedIn}, nil
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
		WITH cursor AS (
			SELECT observed_at, external_id FROM assistant_cursors WHERE source = $1
		)
		SELECT v.id::text, v.source, v.external_id, v.title, v.source_url,
			v.salary_mid, v.role_id::text, v.region_id::text, v.published_at,
			v.collected_at, COALESCE(array_agg(s.slug) FILTER (WHERE s.slug IS NOT NULL), '{}')
		FROM vacancies v
		LEFT JOIN vacancy_skills vs ON vs.vacancy_id = v.id
		LEFT JOIN skills s ON s.id = vs.skill_id
		WHERE v.source = $1 AND v.is_active AND v.deleted_at IS NULL
			AND COALESCE(v.published_at, v.collected_at) >= $2
			AND (
				NOT EXISTS (SELECT 1 FROM cursor)
				OR (v.collected_at, v.external_id) >
					((SELECT observed_at FROM cursor), (SELECT external_id FROM cursor))
			)
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

func (r *PostgresRepository) SaveMatch(ctx context.Context, match WorkerMatch) (bool, error) {
	unknowns, _ := json.Marshal(match.Result.Unknowns)
	conflicts, _ := json.Marshal(match.Result.Conflicts)
	evidence, _ := json.Marshal(match.Result.Evidence)
	tag, err := r.db.Exec(ctx, `
		INSERT INTO vacancy_match_results
			(user_id, preference_id, vacancy_id, decision, method, score, rationale,
			 evidence_ids, conflicts, unknowns)
		SELECT $1::uuid, p.id, $2::uuid, $3, 'deterministic', $4, $5, $6::jsonb, $7::jsonb, $8::jsonb
		FROM vacancy_preferences p
		WHERE p.user_id = $1::uuid AND p.version = $9
		ON CONFLICT DO NOTHING
	`, match.UserID, match.VacancyID, string(match.Result.Decision), match.Result.Score,
		strings.Join(match.Result.Reasons, "; "), string(evidence), string(conflicts), string(unknowns),
		match.PreferenceVersion)
	if err != nil {
		return false, fmt.Errorf("save assistant match: %w", err)
	}
	return tag.RowsAffected() == 1, nil
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
