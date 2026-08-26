package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

const (
	dbOperationTimeout = 30 * time.Second
	rollbackTimeout    = 5 * time.Second
)

// PG is a Postgres Store implementation.
type PG struct {
	pool *pgxpool.Pool
}

// DBTX is the query surface shared by a pool connection and a transaction.
// Transaction-scoped helpers must receive this interface instead of PG/pool.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type pageTx interface {
	DBTX
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type dbStageError struct {
	stage string
	err   error
}

func (e *dbStageError) Error() string { return e.stage + ": " + e.err.Error() }
func (e *dbStageError) Unwrap() error { return e.err }

func atDBStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &dbStageError{stage: stage, err: err}
}

// Open connects using DATABASE_URL. Never log the DSN.
func Open(ctx context.Context, databaseURL string) (*PG, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("store: DATABASE_URL is required")
	}
	cfg, err := newPoolConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: invalid database configuration")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, sanitizeDBError("connect", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, sanitizeDBError("ping", err)
	}
	return &PG{pool: pool}, nil
}

func newPoolConfig(databaseURL string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	// The scheduler reserves one session for its advisory lock while the
	// sequential ingest pipeline uses the other connection.
	cfg.MaxConns = 2
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 30 * time.Second
	cfg.HealthCheckPeriod = 15 * time.Second
	cfg.ConnConfig.RuntimeParams["application_name"] = "lma-ingest"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30s"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "5s"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "15s"
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 15 * time.Second
	}
	return cfg, nil
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
		"lock timeout", "55p03",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func retryTransient(ctx context.Context, attempts int, op func(context.Context) error) error {
	return retryTransientWithTimeout(ctx, attempts, dbOperationTimeout, op)
}

func retryTransientWithTimeout(
	ctx context.Context,
	attempts int,
	timeout time.Duration,
	op func(context.Context) error,
) error {
	var err error
	for i := 0; i < attempts; i++ {
		attemptCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		err = op(attemptCtx)
		cancel()
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

func sanitizeDBError(operation string, err error) error {
	if err == nil {
		return nil
	}
	stage := ""
	var stageErr *dbStageError
	if errors.As(err, &stageErr) {
		stage = " at " + stageErr.stage
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("store %s%s: postgres error (sqlstate=%s)", operation, stage, pgErr.Code)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("store %s%s: database operation timed out", operation, stage)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("store %s%s: database operation canceled", operation, stage)
	}
	root := err
	for errors.Unwrap(root) != nil {
		root = errors.Unwrap(root)
	}
	return fmt.Errorf("store %s%s: database operation failed (kind=%T)", operation, stage, root)
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
	err = retryTransient(ctx, 3, func(ctx context.Context) error {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO ingest_runs (id, source, mode, status, params, requested_by, started_at, created_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, NULLIF($6, ''), $7, now())
		`, run.ID, run.Source, run.Mode, run.Status, string(params), run.RequestedBy, run.StartedAt)
		return err
	})
	if err != nil {
		return sanitizeDBError("create run", err)
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
	`, id, status, string(raw), errMsg)
	if err != nil {
		return sanitizeDBError("finish run", err)
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
		return sanitizeDBError("record error", err)
	}
	return nil
}

// GetCheckpoint reads ingest_checkpoints.
func (s *PG) GetCheckpoint(ctx context.Context, source, scopeHash string) (string, bool, error) {
	var cursor *string
	err := retryTransient(ctx, 3, func(ctx context.Context) error {
		return s.pool.QueryRow(ctx, `
			SELECT cursor FROM ingest_checkpoints WHERE source = $1 AND scope_hash = $2
		`, source, scopeHash).Scan(&cursor)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, sanitizeDBError("get checkpoint", err)
	}
	if cursor == nil {
		return "", true, nil
	}
	return *cursor, true, nil
}

// StartCycle creates or resumes the durable marker for an all-IT plan.
func (s *PG) StartCycle(ctx context.Context, cycle Cycle) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ingest_cycles (
			source, scope, scope_hash, cycle_end, status,
			partition_count, completed_partitions, started_at
		) VALUES ($1, $2, $3, $4, 'running', $5, $6, now())
		ON CONFLICT (source, scope_hash, cycle_end) DO UPDATE
		SET partition_count = EXCLUDED.partition_count
		RETURNING id::text
	`, cycle.Source, cycle.Scope, cycle.ScopeHash, cycle.CycleEnd.UTC(),
		cycle.PartitionCount, cycle.CompletedPartitions).Scan(&id)
	if err != nil {
		return "", sanitizeDBError("start cycle", err)
	}
	return id, nil
}

// UpdateCycleProgress persists aggregate progress without declaring completeness.
func (s *PG) UpdateCycleProgress(ctx context.Context, id string, completedPartitions int) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ingest_cycles
		SET completed_partitions = $2
		WHERE id = $1::uuid AND status = 'running'
	`, id, completedPartitions)
	if err != nil {
		return sanitizeDBError("update cycle progress", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("store update cycle progress: cycle is not running")
	}
	return nil
}

// CompleteCycle is the only transition that proves full all-IT coverage.
func (s *PG) CompleteCycle(ctx context.Context, id string, completedPartitions int) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ingest_cycles
		SET completed_partitions = $2,
			status = 'complete',
			completed_at = COALESCE(completed_at, now())
		WHERE id = $1::uuid
		  AND status IN ('running', 'complete')
		  AND partition_count = $2
	`, id, completedPartitions)
	if err != nil {
		return sanitizeDBError("complete cycle", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("store complete cycle: partition coverage is incomplete")
	}
	return nil
}

// SyncRoles persists official source role ids as canonical roles plus source aliases.
func (s *PG) SyncRoles(ctx context.Context, source string, roles []SourceRole) (map[string]string, error) {
	result := make(map[string]string, len(roles))
	err := retryTransientWithTimeout(ctx, 3, 0, func(ctx context.Context) error {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return atDBStage("begin role sync", err)
		}
		defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
		for _, role := range roles {
			if role.ExternalID == "" || role.Title == "" {
				continue
			}
			slug := source + "-" + role.Family + "-" + role.ExternalID
			var id string
			if err := tx.QueryRow(ctx, `
				INSERT INTO roles (slug, title, family, is_active)
				VALUES ($1, $2, NULLIF($3, ''), true)
				ON CONFLICT (slug) DO UPDATE
				SET title = EXCLUDED.title, family = EXCLUDED.family, is_active = true
				RETURNING id::text
			`, slug, truncate(role.Title, 500), role.Family).Scan(&id); err != nil {
				return atDBStage("upsert role", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO role_aliases (role_id, pattern, source)
				VALUES ($1::uuid, $2, $3)
				ON CONFLICT (source, pattern) DO UPDATE SET role_id = EXCLUDED.role_id
			`, id, role.ExternalID, source); err != nil {
				return atDBStage("upsert role alias", err)
			}
			result[role.ExternalID] = id
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return nil, sanitizeDBError("sync roles", err)
	}
	return result, nil
}

// SavePage upserts vacancies and advances checkpoint atomically.
func (s *PG) SavePage(ctx context.Context, source, scopeHash, nextCursor string, items []VacancyWrite) (int, int, error) {
	var upserted, unchanged int
	// A page performs many statements. The caller bounds the complete run and
	// PostgreSQL bounds each statement; a 30s deadline on the whole page can
	// cancel a healthy transaction midway through a high-latency pooler.
	err := retryTransientWithTimeout(ctx, 3, 0, func(ctx context.Context) error {
		u, un, err := s.savePageOnce(ctx, source, scopeHash, nextCursor, items)
		if err != nil {
			return err
		}
		upserted, unchanged = u, un
		return nil
	})
	if err != nil {
		return 0, 0, sanitizeDBError("save page", err)
	}
	return upserted, unchanged, nil
}

func (s *PG) savePageOnce(ctx context.Context, source, scopeHash, nextCursor string, items []VacancyWrite) (int, int, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return 0, 0, atDBStage("acquire", err)
	}
	released := false
	defer func() {
		if !released {
			conn.Release()
		}
	}()

	tx, err := conn.Conn().Begin(ctx)
	if err != nil {
		return 0, 0, atDBStage("begin", err)
	}

	return savePageInTx(ctx, tx, source, scopeHash, nextCursor, items, func(ctx context.Context) {
		rawConn := conn.Hijack()
		released = true
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		_ = rawConn.Close(cleanupCtx)
	})
}

func savePageInTx(
	ctx context.Context,
	tx pageTx,
	source, scopeHash, nextCursor string,
	items []VacancyWrite,
	onRollbackFailure func(context.Context),
) (int, int, error) {
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanupCtx); rollbackErr != nil &&
			!errors.Is(rollbackErr, pgx.ErrTxClosed) {
			if onRollbackFailure != nil {
				onRollbackFailure(cleanupCtx)
			}
		}
	}()

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

	_, err := tx.Exec(ctx, `
		INSERT INTO ingest_checkpoints (source, scope_hash, cursor, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (source, scope_hash) DO UPDATE
		SET cursor = EXCLUDED.cursor, updated_at = now()
	`, source, scopeHash, nextCursor)
	if err != nil {
		return 0, 0, atDBStage("checkpoint", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, atDBStage("commit", err)
	}
	return upserted, unchanged, nil
}

func upsertVacancy(ctx context.Context, tx DBTX, item VacancyWrite) (changed bool, err error) {
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

	var roleID any
	if v.RoleID != nil && *v.RoleID != "" {
		roleID = *v.RoleID
	}

	var existingHash []byte
	var vacancyID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, content_hash FROM vacancies WHERE source = $1 AND external_id = $2
	`, v.Source, v.ExternalID).Scan(&vacancyID, &existingHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, atDBStage("select vacancy", err)
	}

	if err == nil && bytes.Equal(existingHash, hashBytes) {
		_, err = tx.Exec(ctx, `
			UPDATE vacancies
			SET collected_at = $2,
			    is_active = $3,
			    region_id = $4::uuid,
			    role_id = $5::uuid,
			    employer_id = $6::uuid,
			    deleted_at = NULL,
			    updated_at = now()
			WHERE id = $1::uuid
		`, vacancyID, v.CollectedAt.UTC(), v.IsActive, regionID, roleID, employerID)
		if err != nil {
			return false, atDBStage("touch vacancy", err)
		}
		return false, nil
	}

	var raw any
	if len(item.RawPayload) > 0 && json.Valid(item.RawPayload) {
		raw = string(item.RawPayload)
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
		return false, atDBStage("upsert vacancy", err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM vacancy_skills WHERE vacancy_id = $1::uuid`, vacancyID)
	if err != nil {
		return false, atDBStage("clear vacancy skills", err)
	}
	for _, sid := range skillIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO vacancy_skills (vacancy_id, skill_id)
			VALUES ($1::uuid, $2::uuid)
			ON CONFLICT DO NOTHING
		`, vacancyID, sid)
		if err != nil {
			return false, atDBStage("link vacancy skill", err)
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

func upsertEmployer(ctx context.Context, tx DBTX, source, externalID, name string) (string, error) {
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
		return "", atDBStage("upsert employer", err)
	}
	return id, nil
}

func upsertRegion(ctx context.Context, tx DBTX, source, externalID, name string) (string, error) {
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
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", atDBStage("lookup region", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO regions (code, name, country_code, is_active)
		VALUES ($1, $2, 'RU', true)
		ON CONFLICT (country_code, code) DO UPDATE
		SET name = EXCLUDED.name, is_active = true, updated_at = now()
		RETURNING id::text
	`, externalID, truncate(name, 500)).Scan(&id)
	if err != nil {
		return "", atDBStage("upsert region", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO region_external_ids (source, external_id, region_id)
		VALUES ($1, $2, $3::uuid)
		ON CONFLICT (source, external_id) DO UPDATE SET region_id = EXCLUDED.region_id
	`, source, externalID, id)
	if err != nil {
		return "", atDBStage("map region", err)
	}
	return id, nil
}

func upsertSkills(ctx context.Context, tx DBTX, skills []normalize.SkillRef) ([]string, error) {
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
			return nil, atDBStage("upsert skill", err)
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
