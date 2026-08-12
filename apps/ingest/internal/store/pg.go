package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PG is a Postgres Store implementation.
type PG struct {
	pool *pgxpool.Pool
}

// Open connects using DATABASE_URL. Never log the DSN.
func Open(ctx context.Context, databaseURL string) (*PG, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("store: DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: parse database url: %w", err)
	}
	// Keep pool small for managed session poolers (e.g. Supabase ~15 slots shared).
	cfg.MaxConns = 2
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 30 * time.Second
	cfg.HealthCheckPeriod = 15 * time.Second
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 15 * time.Second
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &PG{pool: pool}, nil
}

func isTransientDBErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"wsarecv", "i/o timeout", "connection reset", "broken pipe",
		"unexpected eof", "server closed the connection", "conn closed",
		"max clients reached", "eof", "statement timeout", "57014",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func retryTransient(ctx context.Context, attempts int, op func(context.Context) error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = op(ctx)
		if err == nil || !isTransientDBErr(err) {
			return err
		}
		// Backoff for flaky managed poolers / mid-flight TCP drops.
		backoff := time.Duration(1<<min(i, 4)) * time.Second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Close closes the pool.
func (s *PG) Close() error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}

// CreateRun inserts ingest_runs.
func (s *PG) CreateRun(ctx context.Context, run Run) error {
	params, err := json.Marshal(run.Params)
	if err != nil {
		return fmt.Errorf("store create run params: %w", err)
	}
	if params == nil {
		params = []byte("{}")
	}
	err = retryTransient(ctx, 8, func(ctx context.Context) error {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO ingest_runs (id, source, mode, status, params, requested_by, started_at, created_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, NULLIF($6, ''), $7, now())
		`, run.ID, run.Source, run.Mode, run.Status, params, run.RequestedBy, run.StartedAt)
		return err
	})
	if err != nil {
		return fmt.Errorf("store create run: %w", err)
	}
	return nil
}

// FinishRun updates terminal status/stats.
func (s *PG) FinishRun(ctx context.Context, id, status string, stats Stats, errMsg string) error {
	raw, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("store finish run stats: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE ingest_runs
		SET status = $2,
		    finished_at = now(),
		    stats = $3::jsonb,
		    error_message = NULLIF($4, '')
		WHERE id = $1
	`, id, status, raw, errMsg)
	if err != nil {
		return fmt.Errorf("store finish run: %w", err)
	}
	return nil
}

// RecordError appends ingest_run_errors (message must not contain secrets/PII).
func (s *PG) RecordError(ctx context.Context, runID, externalID, stage, message string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ingest_run_errors (run_id, external_id, stage, message)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4)
	`, runID, externalID, stage, truncate(message, 1000))
	if err != nil {
		return fmt.Errorf("store record error: %w", err)
	}
	return nil
}

// GetCheckpoint reads ingest_checkpoints.
func (s *PG) GetCheckpoint(ctx context.Context, source, scopeHash string) (string, bool, error) {
	var cursor *string
	err := retryTransient(ctx, 8, func(ctx context.Context) error {
		return s.pool.QueryRow(ctx, `
			SELECT cursor FROM ingest_checkpoints WHERE source = $1 AND scope_hash = $2
		`, source, scopeHash).Scan(&cursor)
	})
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store get checkpoint: %w", err)
	}
	if cursor == nil {
		return "", true, nil
	}
	return *cursor, true, nil
}

// SavePage upserts vacancies and advances checkpoint atomically.
func (s *PG) SavePage(ctx context.Context, source, scopeHash, nextCursor string, items []VacancyWrite) (int, int, error) {
	var upserted, unchanged int
	err := retryTransient(ctx, 8, func(ctx context.Context) error {
		u, un, err := s.savePageOnce(ctx, source, scopeHash, nextCursor, items)
		if err != nil {
			return err
		}
		upserted, unchanged = u, un
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return upserted, unchanged, nil
}

func (s *PG) savePageOnce(ctx context.Context, source, scopeHash, nextCursor string, items []VacancyWrite) (int, int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("store begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	upserted, unchanged := 0, 0
	for _, item := range items {
		changed, err := upsertVacancy(ctx, tx, item)
		if err != nil {
			return 0, 0, err
		}
		if changed {
			upserted++
		} else {
			unchanged++
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO ingest_checkpoints (source, scope_hash, cursor, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (source, scope_hash) DO UPDATE
		SET cursor = EXCLUDED.cursor, updated_at = now()
	`, source, scopeHash, nextCursor)
	if err != nil {
		return 0, 0, fmt.Errorf("store checkpoint: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("store commit: %w", err)
	}
	return upserted, unchanged, nil
}

func upsertVacancy(ctx context.Context, tx pgx.Tx, item VacancyWrite) (changed bool, err error) {
	v := item.Vacancy
	var employerID *string
	if v.EmployerExternalID != "" {
		id, err := upsertEmployer(ctx, tx, v.Source, v.EmployerExternalID, v.EmployerName)
		if err != nil {
			return false, err
		}
		employerID = &id
	}

	var regionID *string
	if v.RegionExternalID != "" {
		rname := item.RegionName
		if rname == "" {
			rname = v.RegionExternalID
		}
		id, err := upsertRegion(ctx, tx, v.Source, v.RegionExternalID, rname)
		if err != nil {
			return false, err
		}
		regionID = &id
	}

	skillIDs, err := upsertSkills(ctx, tx, v.Skills)
	if err != nil {
		return false, err
	}

	hashBytes, err := decodeHash(v.ContentHash)
	if err != nil {
		return false, err
	}

	var existingHash []byte
	var vacancyID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, content_hash FROM vacancies WHERE source = $1 AND external_id = $2
	`, v.Source, v.ExternalID).Scan(&vacancyID, &existingHash)
	if err != nil && err != pgx.ErrNoRows {
		return false, fmt.Errorf("store select vacancy: %w", err)
	}

	if err == nil && bytesEqual(existingHash, hashBytes) {
		_, err = tx.Exec(ctx, `
			UPDATE vacancies
			SET collected_at = $2, is_active = $3, deleted_at = NULL, updated_at = now()
			WHERE id = $1::uuid
		`, vacancyID, v.CollectedAt.UTC(), v.IsActive)
		if err != nil {
			return false, fmt.Errorf("store touch vacancy: %w", err)
		}
		return false, nil
	}

	var raw any
	if len(item.RawPayload) > 0 && json.Valid(item.RawPayload) {
		raw = item.RawPayload
	}

	var roleID any
	if v.RoleID != nil && *v.RoleID != "" {
		roleID = *v.RoleID
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO vacancies (
			source, external_id, title, employer_id, role_id, region_id,
			salary_from, salary_to, salary_currency, salary_gross, salary_mid,
			description_text, published_at, collected_at, is_active, deleted_at,
			content_hash, raw_payload, updated_at
		) VALUES (
			$1, $2, $3, $4::uuid, $5::uuid, $6::uuid,
			$7, $8, NULLIF($9, ''), $10, $11,
			NULLIF($12, ''), $13, $14, $15, NULL,
			$16, $17::jsonb, now()
		)
		ON CONFLICT (source, external_id) DO UPDATE SET
			title = EXCLUDED.title,
			employer_id = EXCLUDED.employer_id,
			role_id = EXCLUDED.role_id,
			region_id = EXCLUDED.region_id,
			salary_from = EXCLUDED.salary_from,
			salary_to = EXCLUDED.salary_to,
			salary_currency = EXCLUDED.salary_currency,
			salary_gross = EXCLUDED.salary_gross,
			salary_mid = EXCLUDED.salary_mid,
			description_text = EXCLUDED.description_text,
			published_at = EXCLUDED.published_at,
			collected_at = EXCLUDED.collected_at,
			is_active = EXCLUDED.is_active,
			deleted_at = NULL,
			content_hash = EXCLUDED.content_hash,
			raw_payload = EXCLUDED.raw_payload,
			updated_at = now()
		RETURNING id::text
	`,
		v.Source, v.ExternalID, v.Title, employerID, roleID, regionID,
		v.SalaryFrom, v.SalaryTo, v.SalaryCurrency, v.SalaryGross, salaryMidForStore(v),
		truncate(v.DescriptionText, 20000), nullTime(v.PublishedAt), v.CollectedAt.UTC(), v.IsActive,
		hashBytes, raw,
	).Scan(&vacancyID)
	if err != nil {
		return false, fmt.Errorf("store upsert vacancy: %w", err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM vacancy_skills WHERE vacancy_id = $1::uuid`, vacancyID)
	if err != nil {
		return false, fmt.Errorf("store clear vacancy_skills: %w", err)
	}
	for _, sid := range skillIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO vacancy_skills (vacancy_id, skill_id)
			VALUES ($1::uuid, $2::uuid)
			ON CONFLICT DO NOTHING
		`, vacancyID, sid)
		if err != nil {
			return false, fmt.Errorf("store vacancy_skills: %w", err)
		}
	}
	return true, nil
}

func salaryMidForStore(v normalize.CanonicalVacancy) *float64 {
	if v.ExcludeFromSalaryAgg {
		// Keep demand; omit mid used for salary analytics when excluded.
		return nil
	}
	if v.SalaryMidRub != nil {
		return v.SalaryMidRub
	}
	return v.SalaryMid
}

func upsertEmployer(ctx context.Context, tx pgx.Tx, source, externalID, name string) (string, error) {
	if name == "" {
		name = externalID
	}
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO employers (source, external_id, name, is_active)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (source, external_id) DO UPDATE
		SET name = EXCLUDED.name, is_active = true
		RETURNING id::text
	`, source, externalID, truncate(name, 500)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("store upsert employer: %w", err)
	}
	return id, nil
}

func upsertRegion(ctx context.Context, tx pgx.Tx, source, externalID, name string) (string, error) {
	if name == "" {
		name = externalID
	}
	var id string
	err := tx.QueryRow(ctx, `
		SELECT region_id::text FROM region_external_ids WHERE source = $1 AND external_id = $2
	`, source, externalID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("store region lookup: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO regions (code, name, country_code, is_active)
		VALUES ($1, $2, 'RU', true)
		ON CONFLICT (country_code, code) DO UPDATE
		SET name = EXCLUDED.name, is_active = true, updated_at = now()
		RETURNING id::text
	`, externalID, truncate(name, 500)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("store upsert region: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO region_external_ids (source, external_id, region_id)
		VALUES ($1, $2, $3::uuid)
		ON CONFLICT (source, external_id) DO UPDATE SET region_id = EXCLUDED.region_id
	`, source, externalID, id)
	if err != nil {
		return "", fmt.Errorf("store region_external_ids: %w", err)
	}
	return id, nil
}

func upsertSkills(ctx context.Context, tx pgx.Tx, skills []normalize.SkillRef) ([]string, error) {
	ids := make([]string, 0, len(skills))
	seen := make(map[string]struct{}, len(skills))
	for _, sk := range skills {
		slug := sk.SkillID
		if slug == "" {
			slug = normalize.NormalizeSkillName(sk.RawName)
		}
		if slug == "" {
			continue
		}
		name := sk.RawName
		if name == "" {
			name = slug
		}
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO skills (slug, name, is_active)
			VALUES ($1, $2, true)
			ON CONFLICT (slug) DO UPDATE
			SET name = COALESCE(NULLIF(skills.name, ''), EXCLUDED.name), is_active = true
			RETURNING id::text
		`, slug, truncate(name, 200)).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("store upsert skill: %w", err)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func decodeHash(hexStr string) ([]byte, error) {
	if hexStr == "" {
		return nil, nil
	}
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("store content_hash: %w", err)
	}
	return b, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}
