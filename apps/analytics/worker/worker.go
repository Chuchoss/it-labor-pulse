// Package worker builds Phase 1 market-demand snapshots from completed ingest cycles.
package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// MethodVersion identifies search-observation-based daily demand snapshots.
	MethodVersion = "vacancy_demand_v2"

	dailyLockKey  int64 = 549004802
	weeklyLockKey int64 = 549004803
)

// ErrLockUnavailable means another analytics process owns the advisory lock.
var ErrLockUnavailable = errors.New("analytics advisory lock unavailable")

// Result summarizes one idempotent analytics operation.
type Result struct {
	RunID          string
	RunType        string
	Target         time.Time
	SourceCycleID  string
	Rows           int64
	SourceDayCount int
	Complete       bool
	Skipped        bool
	SkipReason     string
}

// Worker owns analytics snapshot writes.
type Worker struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL without exposing the DSN in returned errors.
func Open(ctx context.Context, databaseURL string) (*Worker, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("analytics: DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("analytics: invalid database configuration")
	}
	cfg.MaxConns = 3
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 30 * time.Second
	cfg.HealthCheckPeriod = 15 * time.Second
	cfg.ConnConfig.RuntimeParams["application_name"] = "lma-analytics"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "2min"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "5s"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "30s"
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 15 * time.Second
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("analytics: database connection failed")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("analytics: database ping failed")
	}
	return &Worker{pool: pool}, nil
}

// Close releases the database pool.
func (w *Worker) Close() {
	if w != nil && w.pool != nil {
		w.pool.Close()
	}
}

type cycle struct {
	ID          string
	Source      string
	CycleEnd    time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	CycleDate   time.Time
}

// RunDaily creates or deterministically replaces a daily snapshot. Empty
// cycleID selects the latest eligible completed all-IT cycle.
func (w *Worker) RunDaily(ctx context.Context, cycleID string) (Result, error) {
	var result Result
	err := w.withLock(ctx, dailyLockKey, func(ctx context.Context) error {
		selected, found, err := w.selectCycle(ctx, cycleID)
		if err != nil {
			return err
		}
		if !found {
			target := time.Now().UTC().Truncate(24 * time.Hour)
			runID, recordErr := w.recordSkip(ctx, "daily_snapshot", target, "hh", "no_complete_cycle")
			result = Result{
				RunID: runID, RunType: "daily_snapshot", Target: target,
				Skipped: true, SkipReason: "no_complete_cycle",
			}
			return recordErr
		}
		result, err = w.writeDaily(ctx, selected)
		return err
	})
	return result, err
}

func (w *Worker) selectCycle(ctx context.Context, cycleID string) (cycle, bool, error) {
	var selected cycle
	var row pgx.Row
	if strings.TrimSpace(cycleID) != "" {
		row = w.pool.QueryRow(ctx, `
			SELECT id::text, source, cycle_end, started_at, completed_at, cycle_date
			FROM ingest_cycles
			WHERE id = $1::uuid AND scope = 'daily_discovery' AND status = 'complete'
			  AND method_version = $2
		`, cycleID, MethodVersion)
	} else {
		row = w.pool.QueryRow(ctx, `
			SELECT c.id::text, c.source, c.cycle_end, c.started_at, c.completed_at, c.cycle_date
			FROM ingest_cycles c
			WHERE c.scope = 'daily_discovery' AND c.status = 'complete'
			  AND c.method_version = $1
			  AND NOT EXISTS (
				SELECT 1
				FROM analytics_runs ar
				WHERE ar.run_type = 'daily_snapshot'
				  AND ar.source_cycle_id = c.id
				  AND ar.method_version = $1
				  AND ar.status = 'success'
			  )
			ORDER BY c.cycle_end
			LIMIT 1
		`, MethodVersion)
	}
	err := row.Scan(
		&selected.ID, &selected.Source, &selected.CycleEnd,
		&selected.StartedAt, &selected.CompletedAt, &selected.CycleDate,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return cycle{}, false, nil
	}
	if err != nil {
		return cycle{}, false, fmt.Errorf("analytics: select completed cycle failed")
	}
	return selected, true, nil
}

func (w *Worker) writeDaily(ctx context.Context, selected cycle) (Result, error) {
	target := dayUTC(selected.CycleDate)
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Result{}, fmt.Errorf("analytics: begin daily transaction failed")
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var runID string
	err = tx.QueryRow(ctx, `
		INSERT INTO analytics_runs (
			run_type, target_period_start, source, source_cycle_id, status,
			method_version, started_at, finished_at, row_count,
			source_day_count, error_category, error_message
		) VALUES (
			'daily_snapshot', $1, $2, $3::uuid, 'running',
			$4, now(), NULL, 0, 0, NULL, NULL
		)
		ON CONFLICT (run_type, target_period_start, source, method_version)
		DO UPDATE SET
			source_cycle_id = EXCLUDED.source_cycle_id,
			status = 'running',
			started_at = now(),
			finished_at = NULL,
			row_count = 0,
			source_day_count = 0,
			error_category = NULL,
			error_message = NULL
		RETURNING id::text
	`, target, selected.Source, selected.ID, MethodVersion).Scan(&runID)
	if err != nil {
		return Result{}, fmt.Errorf("analytics: start daily run failed")
	}

	if _, err = tx.Exec(ctx, `
		DELETE FROM vacancy_demand_daily
		WHERE snapshot_date = $1 AND source = $2 AND method_version = $3
	`, target, selected.Source, MethodVersion); err != nil {
		return Result{}, fmt.Errorf("analytics: clear daily snapshot failed")
	}

	tag, err := tx.Exec(ctx, `
		WITH cycle AS (
			SELECT id, source, cycle_end, cycle_date
			FROM ingest_cycles
			WHERE id = $1::uuid AND status = 'complete'
			  AND scope = 'daily_discovery' AND method_version = $4
		), base AS (
			SELECT
				o.source, o.role_group, rei.region_id, o.published_at,
				CASE WHEN o.salary_eligible THEN o.salary_mid_rub_net END AS salary_rub_net
			FROM cycle c
			JOIN ingest_cycle_observations o ON o.cycle_id = c.id
			LEFT JOIN region_external_ids rei
			  ON rei.source = o.source AND rei.external_id = o.external_region_id
		), aggregate_rows AS (
			SELECT
				role_group, 'all_regions'::text AS aggregation_level,
				NULL::uuid AS region_id,
				count(*)::integer AS active_count,
				count(*) FILTER (
					WHERE published_at >= $2::date
					  AND published_at < ($2::date + interval '1 day')
				)::integer AS published_count,
				count(salary_rub_net)::integer AS vacancies_with_salary,
				percentile_cont(0.5) WITHIN GROUP (ORDER BY salary_rub_net)
					FILTER (WHERE salary_rub_net IS NOT NULL) AS median_salary_rub_net,
				(count(salary_rub_net)::numeric / NULLIF(count(*), 0)) AS salary_coverage
			FROM base
			GROUP BY role_group
			UNION ALL
			SELECT
				role_group, 'region'::text, region_id,
				count(*)::integer,
				count(*) FILTER (
					WHERE published_at >= $2::date
					  AND published_at < ($2::date + interval '1 day')
				)::integer,
				count(salary_rub_net)::integer,
				percentile_cont(0.5) WITHIN GROUP (ORDER BY salary_rub_net)
					FILTER (WHERE salary_rub_net IS NOT NULL),
				(count(salary_rub_net)::numeric / NULLIF(count(*), 0))
			FROM base
			WHERE region_id IS NOT NULL
			GROUP BY role_group, region_id
		)
		INSERT INTO vacancy_demand_daily (
			snapshot_date, source, role_group, aggregation_level, region_id,
			active_count, published_count, vacancies_with_salary,
			median_salary_rub_net, cycle_complete, source_cycle_id,
			analytics_run_id, method_version, observed_at,
			salary_method, salary_coverage
		)
		SELECT
			$2, c.source, a.role_group, a.aggregation_level, a.region_id,
			a.active_count, a.published_count, a.vacancies_with_salary,
			a.median_salary_rub_net, true, c.id,
			$3::uuid, $4, c.cycle_end,
			'hh_search_shared_normalizer_v1', coalesce(a.salary_coverage, 0)
		FROM aggregate_rows a
		CROSS JOIN cycle c
	`, selected.ID, target, runID, MethodVersion)
	if err != nil {
		return Result{}, fmt.Errorf("analytics: build daily snapshot failed")
	}
	rows := tag.RowsAffected()

	if _, err = tx.Exec(ctx, `
		UPDATE analytics_runs
		SET status = 'success', finished_at = now(), row_count = $2
		WHERE id = $1::uuid
	`, runID, rows); err != nil {
		return Result{}, fmt.Errorf("analytics: finish daily run failed")
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("analytics: commit daily snapshot failed")
	}
	return Result{
		RunID: runID, RunType: "daily_snapshot", Target: target,
		SourceCycleID: selected.ID, Rows: rows, SourceDayCount: 1, Complete: true,
	}, nil
}

// RunWeekly rolls up one ISO week from daily snapshots. Missing days produce
// an explicitly incomplete row set.
func (w *Worker) RunWeekly(ctx context.Context, weekStart time.Time, source string) (Result, error) {
	if source == "" {
		source = "hh"
	}
	weekStart = dayUTC(weekStart)
	if weekStart.Weekday() != time.Monday {
		return Result{}, fmt.Errorf("analytics: week start must be Monday")
	}
	var result Result
	err := w.withLock(ctx, weeklyLockKey, func(ctx context.Context) error {
		var err error
		result, err = w.writeWeekly(ctx, weekStart, source)
		return err
	})
	return result, err
}

func (w *Worker) writeWeekly(ctx context.Context, weekStart time.Time, source string) (Result, error) {
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Result{}, fmt.Errorf("analytics: begin weekly transaction failed")
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var availableDays int
	if err = tx.QueryRow(ctx, `
		SELECT count(DISTINCT snapshot_date)
		FROM vacancy_demand_daily
		WHERE source = $1
		  AND method_version = $2
		  AND snapshot_date >= $3
		  AND snapshot_date < ($3::date + 7)
		  AND cycle_complete
	`, source, MethodVersion, weekStart).Scan(&availableDays); err != nil {
		return Result{}, fmt.Errorf("analytics: count weekly source days failed")
	}
	if availableDays == 0 {
		runID, recordErr := w.recordSkipTx(
			ctx, tx, "weekly_rollup", weekStart, source, "no_daily_snapshots",
		)
		if recordErr != nil {
			return Result{}, recordErr
		}
		if err = tx.Commit(ctx); err != nil {
			return Result{}, fmt.Errorf("analytics: commit weekly skip failed")
		}
		return Result{
			RunID: runID, RunType: "weekly_rollup", Target: weekStart,
			Skipped: true, SkipReason: "no_daily_snapshots",
		}, nil
	}

	var runID string
	err = tx.QueryRow(ctx, `
		INSERT INTO analytics_runs (
			run_type, target_period_start, source, status, method_version,
			started_at, finished_at, row_count, source_day_count,
			error_category, error_message
		) VALUES (
			'weekly_rollup', $1, $2, 'running', $3,
			now(), NULL, 0, $4, NULL, NULL
		)
		ON CONFLICT (run_type, target_period_start, source, method_version)
		DO UPDATE SET
			status = 'running',
			started_at = now(),
			finished_at = NULL,
			row_count = 0,
			source_day_count = EXCLUDED.source_day_count,
			error_category = NULL,
			error_message = NULL
		RETURNING id::text
	`, weekStart, source, MethodVersion, availableDays).Scan(&runID)
	if err != nil {
		return Result{}, fmt.Errorf("analytics: start weekly run failed")
	}
	if _, err = tx.Exec(ctx, `
		DELETE FROM vacancy_demand_weekly
		WHERE week_start = $1 AND source = $2 AND method_version = $3
	`, weekStart, source, MethodVersion); err != nil {
		return Result{}, fmt.Errorf("analytics: clear weekly rollup failed")
	}

	tag, err := tx.Exec(ctx, `
		WITH base AS (
			SELECT *
			FROM vacancy_demand_daily
			WHERE source = $1
			  AND method_version = $2
			  AND snapshot_date >= $3
			  AND snapshot_date < ($3::date + 7)
			  AND cycle_complete
		), latest AS (
			SELECT DISTINCT ON (role_group, aggregation_level, region_id)
				role_group, aggregation_level, region_id,
				active_count, observed_at
			FROM base
			ORDER BY role_group, aggregation_level, region_id,
				snapshot_date DESC, observed_at DESC
		), rolled AS (
			SELECT
				role_group, aggregation_level, region_id,
				sum(published_count)::integer AS published_count,
				sum(vacancies_with_salary)::integer AS vacancies_with_salary,
				percentile_cont(0.5) WITHIN GROUP (ORDER BY median_salary_rub_net)
					FILTER (WHERE median_salary_rub_net IS NOT NULL)
					AS median_salary_rub_net,
				count(DISTINCT snapshot_date)::smallint AS source_daily_count
			FROM base
			GROUP BY role_group, aggregation_level, region_id
		)
		INSERT INTO vacancy_demand_weekly (
			week_start, source, role_group, aggregation_level, region_id,
			active_count, published_count, vacancies_with_salary,
			median_salary_rub_net, source_daily_count, complete,
			analytics_run_id, method_version, observed_at
		)
		SELECT
			$3, $1, r.role_group, r.aggregation_level, r.region_id,
			l.active_count, r.published_count, r.vacancies_with_salary,
			r.median_salary_rub_net, r.source_daily_count,
			(r.source_daily_count = 7), $4::uuid, $2, l.observed_at
		FROM rolled r
		JOIN latest l
		  ON l.role_group = r.role_group
		 AND l.aggregation_level = r.aggregation_level
		 AND l.region_id IS NOT DISTINCT FROM r.region_id
	`, source, MethodVersion, weekStart, runID)
	if err != nil {
		return Result{}, fmt.Errorf("analytics: build weekly rollup failed")
	}
	rows := tag.RowsAffected()
	if _, err = tx.Exec(ctx, `
		UPDATE analytics_runs
		SET status = 'success', finished_at = now(), row_count = $2,
			source_day_count = $3
		WHERE id = $1::uuid
	`, runID, rows, availableDays); err != nil {
		return Result{}, fmt.Errorf("analytics: finish weekly run failed")
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("analytics: commit weekly rollup failed")
	}
	return Result{
		RunID: runID, RunType: "weekly_rollup", Target: weekStart,
		Rows: rows, SourceDayCount: availableDays, Complete: availableDays == 7,
	}, nil
}

// Backfill processes all currently known complete cycles, then all affected ISO weeks.
func (w *Worker) Backfill(ctx context.Context) ([]Result, error) {
	results := make([]Result, 0)
	for {
		result, err := w.RunDaily(ctx, "")
		if err != nil {
			return results, err
		}
		if result.Skipped {
			break
		}
		results = append(results, result)
	}
	rows, err := w.pool.Query(ctx, `
		SELECT DISTINCT date_trunc('week', snapshot_date)::date
		FROM vacancy_demand_daily
		WHERE source = 'hh' AND method_version = $1
		ORDER BY 1
	`, MethodVersion)
	if err != nil {
		return results, fmt.Errorf("analytics: list backfill weeks failed")
	}
	var weeks []time.Time
	for rows.Next() {
		var week time.Time
		if err := rows.Scan(&week); err != nil {
			rows.Close()
			return results, fmt.Errorf("analytics: scan backfill week failed")
		}
		weeks = append(weeks, week)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return results, fmt.Errorf("analytics: iterate backfill weeks failed")
	}
	rows.Close()
	for _, week := range weeks {
		result, err := w.RunWeekly(ctx, week, "hh")
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (w *Worker) withLock(ctx context.Context, key int64, run func(context.Context) error) error {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("analytics: acquire lock connection failed")
	}
	defer conn.Release()
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&acquired); err != nil {
		return fmt.Errorf("analytics: advisory lock failed")
	}
	if !acquired {
		return ErrLockUnavailable
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, key)
	}()
	return run(ctx)
}

func (w *Worker) recordSkip(
	ctx context.Context,
	runType string,
	target time.Time,
	source string,
	category string,
) (string, error) {
	var id string
	err := w.pool.QueryRow(ctx, skipSQL, runType, target, source, MethodVersion, category).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("analytics: record skipped run failed")
	}
	return id, nil
}

func (w *Worker) recordSkipTx(
	ctx context.Context,
	tx pgx.Tx,
	runType string,
	target time.Time,
	source string,
	category string,
) (string, error) {
	var id string
	err := tx.QueryRow(ctx, skipSQL, runType, target, source, MethodVersion, category).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("analytics: record skipped run failed")
	}
	return id, nil
}

const skipSQL = `
	INSERT INTO analytics_runs (
		run_type, target_period_start, source, status, method_version,
		started_at, finished_at, row_count, source_day_count,
		error_category, error_message
	) VALUES ($1, $2, $3, 'skipped', $4, now(), now(), 0, 0, $5, NULL)
	ON CONFLICT (run_type, target_period_start, source, method_version)
	DO UPDATE SET
		status = CASE
			WHEN analytics_runs.status = 'success' THEN analytics_runs.status
			ELSE 'skipped'
		END,
		finished_at = CASE
			WHEN analytics_runs.status = 'success' THEN analytics_runs.finished_at
			ELSE now()
		END,
		error_category = CASE
			WHEN analytics_runs.status = 'success' THEN analytics_runs.error_category
			ELSE EXCLUDED.error_category
		END,
		error_message = NULL
	RETURNING id::text
`

func dayUTC(value time.Time) time.Time {
	y, m, d := value.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// MondayUTC returns the ISO week start containing value.
func MondayUTC(value time.Time) time.Time {
	day := dayUTC(value)
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}
