package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Chuchoss/it-labor-pulse/apps/fx/cbr"
)

const AdvisoryLockKey int64 = 0x4c4d414658

type Postgres struct {
	pool *pgxpool.Pool
}

type Lock struct {
	conn *pgxpool.Conn
}

type ReconcileResult struct {
	VacanciesUpdated    int64
	VacanciesMissing    int64
	ObservationsUpdated int64
	ObservationsMissing int64
	SourceLinksMissing  int64
}

func Open(ctx context.Context, databaseURL string) (*Postgres, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("fx store: DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("fx store: invalid database configuration")
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["application_name"] = "lma-fx-sync"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "60s"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("fx store: connect failed")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("fx store: ping failed")
	}
	return &Postgres{pool: pool}, nil
}

func (s *Postgres) Close() { s.pool.Close() }

func (s *Postgres) TryLock(ctx context.Context) (*Lock, bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	var acquired bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, AdvisoryLockKey).Scan(&acquired)
	if err != nil || !acquired {
		conn.Release()
		return nil, acquired, err
	}
	return &Lock{conn: conn}, true, nil
}

func (l *Lock) Release(ctx context.Context) {
	if l == nil || l.conn == nil {
		return
	}
	_, _ = l.conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, AdvisoryLockKey)
	l.conn.Release()
	l.conn = nil
}

func (s *Postgres) StartRun(ctx context.Context, from, to time.Time) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO fx_sync_runs (provider, requested_from, requested_to, status)
		VALUES ('cbr', $1, $2, 'running')
		RETURNING id::text
	`, from, to).Scan(&id)
	return id, err
}

func (s *Postgres) FinishRun(
	ctx context.Context,
	id, status string,
	dates int,
	rates int64,
	category string,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE fx_sync_runs
		SET status = $2, fetched_dates = $3, upserted_rates = $4,
			finished_at = now(), error_category = NULLIF($5, '')
		WHERE id = $1::uuid
	`, id, status, dates, rates, category)
	return err
}

func (s *Postgres) NeededDates(ctx context.Context, now time.Time) ([]time.Time, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT needed_date
		FROM (
			SELECT published_at::date AS needed_date
			FROM vacancies
			WHERE salary_currency IS NOT NULL
			  AND trim(salary_currency) <> 'RUB'
			  AND published_at >= $1::date - 35
			UNION
			SELECT published_at::date
			FROM ingest_cycle_observations
			WHERE salary_currency IS NOT NULL
			  AND trim(salary_currency) <> 'RUB'
			  AND salary_rate_date IS NULL
			  AND published_at >= $1::date - 35
			UNION
			SELECT snapshot_date FROM vacancy_demand_daily WHERE snapshot_date >= $1::date - 35
		) dates
		WHERE needed_date IS NOT NULL
		ORDER BY needed_date
		LIMIT 60
	`, now.UTC().Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("fx needed dates: %w", err)
	}
	defer rows.Close()
	var result []time.Time
	for rows.Next() {
		var date time.Time
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("fx needed date scan: %w", err)
		}
		result = append(result, date.UTC())
	}
	return result, rows.Err()
}

func (s *Postgres) Upsert(ctx context.Context, rates []cbr.Rate, fetchedAt time.Time) (int64, error) {
	batch := &pgx.Batch{}
	for _, rate := range rates {
		batch.Queue(`
			INSERT INTO fx_rates (
				rate_date, provider, base_currency, quote_currency,
				rub_per_unit, nominal, source_value, fetched_at,
				source_revision, provenance
			) VALUES ($1, 'cbr', 'RUB', $2, $3::numeric, $4, $5::numeric, $6, $7, $8)
			ON CONFLICT (provider, rate_date, base_currency, quote_currency) DO UPDATE SET
				rub_per_unit = EXCLUDED.rub_per_unit,
				nominal = EXCLUDED.nominal,
				source_value = EXCLUDED.source_value,
				fetched_at = EXCLUDED.fetched_at,
				source_revision = EXCLUDED.source_revision,
				provenance = EXCLUDED.provenance
		`, rate.Date, rate.Currency, rate.RubPerUnit, rate.Nominal, rate.SourceValue,
			fetchedAt.UTC(), rate.Revision, rate.ProvenanceURL)
	}
	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()
	var affected int64
	for range rates {
		tag, err := results.Exec()
		if err != nil {
			return affected, fmt.Errorf("fx upsert: %w", err)
		}
		affected += tag.RowsAffected()
	}
	return affected, results.Close()
}

func (s *Postgres) Reconcile(ctx context.Context) (ReconcileResult, error) {
	var result ReconcileResult
	tag, err := s.pool.Exec(ctx, `
		WITH resolved AS (
			SELECT v.id, r.rate_date, r.provider, r.rub_per_unit
			FROM vacancies v
			JOIN LATERAL (
				SELECT rate_date, provider, rub_per_unit
				FROM fx_rates
				WHERE provider = 'cbr'
				  AND quote_currency = trim(v.salary_currency)
				  AND rate_date <= v.published_at::date
				  AND rate_date >= v.published_at::date - 7
				ORDER BY rate_date DESC
				LIMIT 1
			) r ON trim(v.salary_currency) <> 'RUB'
			WHERE v.published_at IS NOT NULL
		)
		UPDATE vacancies v
		SET salary_mid = round((
				coalesce((v.salary_from + v.salary_to) / 2, v.salary_from, v.salary_to)
				* CASE WHEN v.salary_gross THEN 0.87 ELSE 1 END
				* resolved.rub_per_unit
			)::numeric, 2),
			salary_from_rub_net = round((
				v.salary_from * CASE WHEN v.salary_gross THEN 0.87 ELSE 1 END
				* resolved.rub_per_unit
			)::numeric, 2),
			salary_to_rub_net = round((
				v.salary_to * CASE WHEN v.salary_gross THEN 0.87 ELSE 1 END
				* resolved.rub_per_unit
			)::numeric, 2),
			salary_rate_date = resolved.rate_date,
			salary_rate_provider = resolved.provider,
			updated_at = now()
		FROM resolved
		WHERE v.id = resolved.id
		  AND (
			v.salary_rate_date IS DISTINCT FROM resolved.rate_date
			OR v.salary_rate_provider IS DISTINCT FROM resolved.provider
		  )
	`)
	if err != nil {
		return result, fmt.Errorf("fx reconcile vacancies: %w", err)
	}
	result.VacanciesUpdated = tag.RowsAffected()

	tag, err = s.pool.Exec(ctx, `
		WITH resolved AS (
			SELECT o.cycle_id, o.source, o.external_id, r.rate_date, r.provider, r.rub_per_unit
			FROM ingest_cycle_observations o
			JOIN LATERAL (
				SELECT rate_date, provider, rub_per_unit
				FROM fx_rates
				WHERE provider = 'cbr'
				  AND quote_currency = trim(o.salary_currency)
				  AND rate_date <= o.published_at::date
				  AND rate_date >= o.published_at::date - 7
				ORDER BY rate_date DESC LIMIT 1
			) r ON trim(o.salary_currency) <> 'RUB'
		)
		UPDATE ingest_cycle_observations o
		SET salary_mid_rub_net = round((
				coalesce((o.salary_from + o.salary_to) / 2, o.salary_from, o.salary_to)
				* CASE WHEN o.salary_gross THEN 0.87 ELSE 1 END
				* resolved.rub_per_unit
			)::numeric, 2),
			salary_eligible = (
				coalesce((o.salary_from + o.salary_to) / 2, o.salary_from, o.salary_to)
				* CASE WHEN o.salary_gross THEN 0.87 ELSE 1 END
				* resolved.rub_per_unit BETWEEN 10000 AND 2000000
			),
			salary_rate_date = resolved.rate_date,
			salary_rate_provider = resolved.provider
		FROM resolved
		WHERE o.cycle_id = resolved.cycle_id
		  AND o.source = resolved.source
		  AND o.external_id = resolved.external_id
		  AND o.salary_rate_date IS DISTINCT FROM resolved.rate_date
	`)
	if err != nil {
		return result, fmt.Errorf("fx reconcile observations: %w", err)
	}
	result.ObservationsUpdated = tag.RowsAffected()

	err = s.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (
				WHERE salary_currency IS NOT NULL AND trim(salary_currency) <> 'RUB'
				  AND salary_rate_date IS NULL
			),
			count(*) FILTER (WHERE source = 'hh' AND source_url IS NULL)
		FROM vacancies
	`).Scan(&result.VacanciesMissing, &result.SourceLinksMissing)
	if err != nil {
		return result, fmt.Errorf("fx reconcile counts: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM ingest_cycle_observations
		WHERE salary_currency IS NOT NULL AND trim(salary_currency) <> 'RUB'
		  AND salary_mid_rub_net IS NULL
	`).Scan(&result.ObservationsMissing)
	return result, err
}
