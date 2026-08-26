package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/readapi"
)

const marketMethodVersion = "vacancy_demand_v2"

type displayRate struct {
	factor   float64
	date     *string
	provider string
}

type DBTX interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Postgres struct {
	db DBTX
}

func NewPostgres(db DBTX) *Postgres {
	return &Postgres{db: db}
}

func (p *Postgres) Dashboard(ctx context.Context, filter readapi.AnalyticsFilter) (readapi.DashboardSummary, error) {
	args := analyticsArgs(filter)
	var result readapi.DashboardSummary
	err := p.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE v.is_active AND v.published_at < ($2::date + interval '1 day')),
			count(*) FILTER (WHERE v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')),
			coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY v.salary_mid)
				FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000
					AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')), 0)::float8,
			count(v.salary_mid) FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000
				AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')),
			current_timestamp
		FROM vacancies v
		WHERE v.deleted_at IS NULL
			AND EXISTS (SELECT 1 FROM roles sr WHERE sr.id = v.role_id
				AND sr.family IN ('software_development', 'analytics', 'quality_assurance'))
			AND ($3 = '' OR v.role_id = $3::uuid)
			AND ($4 = '' OR v.region_id = $4::uuid)
			AND ($5 = '' OR v.source = $5)
	`, args...).Scan(
		&result.VacanciesActive,
		&result.VacanciesNew,
		&result.MedianSalary,
		&result.SalarySample,
		&result.GeneratedAt,
	)
	if err != nil {
		return readapi.DashboardSummary{}, fmt.Errorf("query dashboard summary: %w", err)
	}

	result.Period = periodResponse(filter.Period)
	rate, err := p.currentDisplayRate(ctx, filter.Currency)
	if err != nil {
		return readapi.DashboardSummary{}, err
	}
	result.MedianSalary = convertMoney(result.MedianSalary, rate)
	result.SalaryCurrency = displayCurrency(filter.Currency)
	result.SalaryRateDate = rate.date
	result.SalaryRateProvider = rate.provider
	result.Cache = "MISS"
	result.TopRoles = []readapi.RoleCount{}
	result.TopRegions = []readapi.RegionCount{}

	roleRows, err := p.db.Query(ctx, `
		SELECT r.id::text, r.title, count(*)
		FROM vacancies v
		JOIN roles r ON r.id = v.role_id
		WHERE v.deleted_at IS NULL AND v.is_active
			AND r.family IN ('software_development', 'analytics', 'quality_assurance')
			AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
			AND ($3 = '' OR v.role_id = $3::uuid)
			AND ($4 = '' OR v.region_id = $4::uuid)
			AND ($5 = '' OR v.source = $5)
		GROUP BY r.id, r.title
		ORDER BY count(*) DESC, r.title, r.id
		LIMIT 5
	`, args...)
	if err != nil {
		return readapi.DashboardSummary{}, fmt.Errorf("query dashboard top roles: %w", err)
	}
	for roleRows.Next() {
		var item readapi.RoleCount
		if err := roleRows.Scan(&item.RoleID, &item.Title, &item.Count); err != nil {
			roleRows.Close()
			return readapi.DashboardSummary{}, fmt.Errorf("scan dashboard top role: %w", err)
		}
		result.TopRoles = append(result.TopRoles, item)
	}
	if err := roleRows.Err(); err != nil {
		return readapi.DashboardSummary{}, fmt.Errorf("iterate dashboard top roles: %w", err)
	}

	regionRows, err := p.db.Query(ctx, `
		SELECT r.id::text, r.name, count(*)
		FROM vacancies v
		JOIN regions r ON r.id = v.region_id
		WHERE v.deleted_at IS NULL AND v.is_active
			AND EXISTS (SELECT 1 FROM roles sr WHERE sr.id = v.role_id
				AND sr.family IN ('software_development', 'analytics', 'quality_assurance'))
			AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
			AND ($3 = '' OR v.role_id = $3::uuid)
			AND ($4 = '' OR v.region_id = $4::uuid)
			AND ($5 = '' OR v.source = $5)
		GROUP BY r.id, r.name
		ORDER BY count(*) DESC, r.name, r.id
		LIMIT 5
	`, args...)
	if err != nil {
		return readapi.DashboardSummary{}, fmt.Errorf("query dashboard top regions: %w", err)
	}
	for regionRows.Next() {
		var item readapi.RegionCount
		if err := regionRows.Scan(&item.RegionID, &item.Title, &item.Count); err != nil {
			regionRows.Close()
			return readapi.DashboardSummary{}, fmt.Errorf("scan dashboard top region: %w", err)
		}
		result.TopRegions = append(result.TopRegions, item)
	}
	if err := regionRows.Err(); err != nil {
		return readapi.DashboardSummary{}, fmt.Errorf("iterate dashboard top regions: %w", err)
	}
	return result, nil
}

func (p *Postgres) ListRoles(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	page readapi.Page,
	sortBy string,
) (readapi.RolePage, error) {
	order := "vacancies_count DESC"
	if sortBy == "median_salary" {
		order = "median_salary DESC"
	}
	args := []any{
		filter.Period.From.Format(time.DateOnly),
		filter.Period.To.Format(time.DateOnly),
		filter.RegionID,
		filter.Source,
		page.Size,
		(page.Number - 1) * page.Size,
	}
	rows, err := p.db.Query(ctx, `
		WITH stats AS (
			SELECT r.id, r.title,
				count(*) AS vacancies_count,
				coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY v.salary_mid)
					FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8 AS median_salary,
				coalesce(percentile_cont(0.25) WITHIN GROUP (ORDER BY v.salary_mid)
					FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8 AS p25_salary,
				coalesce(percentile_cont(0.75) WITHIN GROUP (ORDER BY v.salary_mid)
					FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8 AS p75_salary
			FROM roles r
			JOIN vacancies v ON v.role_id = r.id
			WHERE r.is_active AND v.deleted_at IS NULL AND v.is_active
				AND r.family IN ('software_development', 'analytics', 'quality_assurance')
				AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
				AND ($3 = '' OR v.region_id = $3::uuid)
				AND ($4 = '' OR v.source = $4)
			GROUP BY r.id, r.title
		)
		SELECT id::text, title, vacancies_count, median_salary, p25_salary, p75_salary, count(*) OVER()
		FROM stats
		ORDER BY `+order+`, title, id
		LIMIT $5 OFFSET $6
	`, args...)
	if err != nil {
		return readapi.RolePage{}, fmt.Errorf("query roles: %w", err)
	}
	defer rows.Close()

	result := readapi.RolePage{Data: []readapi.RoleStat{}, Page: page.Number, PageSize: page.Size}
	for rows.Next() {
		var item readapi.RoleStat
		if err := rows.Scan(
			&item.RoleID,
			&item.Title,
			&item.VacanciesCount,
			&item.MedianSalary,
			&item.P25Salary,
			&item.P75Salary,
			&result.Total,
		); err != nil {
			return readapi.RolePage{}, fmt.Errorf("scan role: %w", err)
		}
		result.Data = append(result.Data, item)
	}
	if err := rows.Err(); err != nil {
		return readapi.RolePage{}, fmt.Errorf("iterate roles: %w", err)
	}
	rate, err := p.currentDisplayRate(ctx, filter.Currency)
	if err != nil {
		return readapi.RolePage{}, err
	}
	for index := range result.Data {
		result.Data[index].MedianSalary = convertMoney(result.Data[index].MedianSalary, rate)
		result.Data[index].P25Salary = convertMoney(result.Data[index].P25Salary, rate)
		result.Data[index].P75Salary = convertMoney(result.Data[index].P75Salary, rate)
		result.Data[index].Currency = displayCurrency(filter.Currency)
		result.Data[index].RateDate = rate.date
		result.Data[index].RateProvider = rate.provider
	}
	return result, nil
}

func (p *Postgres) GetRole(
	ctx context.Context,
	id string,
	filter readapi.AnalyticsFilter,
) (readapi.RoleStat, error) {
	args := []any{
		filter.Period.From.Format(time.DateOnly),
		filter.Period.To.Format(time.DateOnly),
		id,
		filter.RegionID,
	}
	var item readapi.RoleStat
	err := p.db.QueryRow(ctx, `
		SELECT r.id::text, r.title,
			count(v.id),
			coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY v.salary_mid)
				FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8,
			coalesce(percentile_cont(0.25) WITHIN GROUP (ORDER BY v.salary_mid)
				FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8,
			coalesce(percentile_cont(0.75) WITHIN GROUP (ORDER BY v.salary_mid)
				FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8
		FROM roles r
		LEFT JOIN vacancies v ON v.role_id = r.id
			AND v.deleted_at IS NULL
			AND v.is_active
			AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
			AND ($4 = '' OR v.region_id = $4::uuid)
		WHERE r.id = $3::uuid AND r.is_active
			AND r.family IN ('software_development', 'analytics', 'quality_assurance')
		GROUP BY r.id, r.title
	`, args...).Scan(
		&item.RoleID,
		&item.Title,
		&item.VacanciesCount,
		&item.MedianSalary,
		&item.P25Salary,
		&item.P75Salary,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return readapi.RoleStat{}, readapi.ErrNotFound
	}
	if err != nil {
		return readapi.RoleStat{}, fmt.Errorf("query role: %w", err)
	}
	rate, err := p.currentDisplayRate(ctx, filter.Currency)
	if err != nil {
		return readapi.RoleStat{}, err
	}
	item.MedianSalary = convertMoney(item.MedianSalary, rate)
	item.P25Salary = convertMoney(item.P25Salary, rate)
	item.P75Salary = convertMoney(item.P75Salary, rate)
	item.Currency = displayCurrency(filter.Currency)
	item.RateDate = rate.date
	item.RateProvider = rate.provider
	return item, nil
}

func (p *Postgres) ListRegions(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	page readapi.Page,
) (readapi.RegionPage, error) {
	args := []any{
		filter.Period.From.Format(time.DateOnly),
		filter.Period.To.Format(time.DateOnly),
		filter.RoleID,
		page.Size,
		(page.Number - 1) * page.Size,
	}
	rows, err := p.db.Query(ctx, `
		WITH stats AS (
			SELECT r.id, r.name,
				count(*) AS vacancies_count,
				coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY v.salary_mid)
					FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8 AS median_salary,
				coalesce(percentile_cont(0.25) WITHIN GROUP (ORDER BY v.salary_mid)
					FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8 AS p25_salary,
				coalesce(percentile_cont(0.75) WITHIN GROUP (ORDER BY v.salary_mid)
					FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8 AS p75_salary
			FROM regions r
			JOIN vacancies v ON v.region_id = r.id
			WHERE r.is_active AND v.deleted_at IS NULL AND v.is_active
				AND EXISTS (SELECT 1 FROM roles sr WHERE sr.id = v.role_id
					AND sr.family IN ('software_development', 'analytics', 'quality_assurance'))
				AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
				AND ($3 = '' OR v.role_id = $3::uuid)
			GROUP BY r.id, r.name
		)
		SELECT id::text, name, vacancies_count, median_salary, p25_salary, p75_salary, count(*) OVER()
		FROM stats
		ORDER BY vacancies_count DESC, name, id
		LIMIT $4 OFFSET $5
	`, args...)
	if err != nil {
		return readapi.RegionPage{}, fmt.Errorf("query regions: %w", err)
	}
	defer rows.Close()

	result := readapi.RegionPage{Data: []readapi.RegionStat{}, Page: page.Number, PageSize: page.Size}
	for rows.Next() {
		var item readapi.RegionStat
		if err := rows.Scan(
			&item.RegionID,
			&item.Title,
			&item.VacanciesCount,
			&item.MedianSalary,
			&item.P25Salary,
			&item.P75Salary,
			&result.Total,
		); err != nil {
			return readapi.RegionPage{}, fmt.Errorf("scan region: %w", err)
		}
		result.Data = append(result.Data, item)
	}
	if err := rows.Err(); err != nil {
		return readapi.RegionPage{}, fmt.Errorf("iterate regions: %w", err)
	}
	rate, err := p.currentDisplayRate(ctx, filter.Currency)
	if err != nil {
		return readapi.RegionPage{}, err
	}
	for index := range result.Data {
		result.Data[index].MedianSalary = convertMoney(result.Data[index].MedianSalary, rate)
		result.Data[index].P25Salary = convertMoney(result.Data[index].P25Salary, rate)
		result.Data[index].P75Salary = convertMoney(result.Data[index].P75Salary, rate)
		result.Data[index].Currency = displayCurrency(filter.Currency)
		result.Data[index].RateDate = rate.date
		result.Data[index].RateProvider = rate.provider
	}
	return result, nil
}

func (p *Postgres) GetRegion(
	ctx context.Context,
	id string,
	filter readapi.AnalyticsFilter,
) (readapi.RegionStat, error) {
	args := []any{
		filter.Period.From.Format(time.DateOnly),
		filter.Period.To.Format(time.DateOnly),
		filter.RoleID,
		id,
	}
	var item readapi.RegionStat
	err := p.db.QueryRow(ctx, `
		SELECT r.id::text, r.name,
			count(v.id),
			coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY v.salary_mid)
				FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8,
			coalesce(percentile_cont(0.25) WITHIN GROUP (ORDER BY v.salary_mid)
				FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8,
			coalesce(percentile_cont(0.75) WITHIN GROUP (ORDER BY v.salary_mid)
				FILTER (WHERE v.salary_mid BETWEEN 10000 AND 2000000), 0)::float8
		FROM regions r
		LEFT JOIN vacancies v ON v.region_id = r.id
			AND v.deleted_at IS NULL
			AND v.is_active
			AND EXISTS (SELECT 1 FROM roles sr WHERE sr.id = v.role_id
				AND sr.family IN ('software_development', 'analytics', 'quality_assurance'))
			AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
			AND ($3 = '' OR v.role_id = $3::uuid)
		WHERE r.id = $4::uuid AND r.is_active
		GROUP BY r.id, r.name
	`, args...).Scan(
		&item.RegionID,
		&item.Title,
		&item.VacanciesCount,
		&item.MedianSalary,
		&item.P25Salary,
		&item.P75Salary,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return readapi.RegionStat{}, readapi.ErrNotFound
	}
	if err != nil {
		return readapi.RegionStat{}, fmt.Errorf("query region: %w", err)
	}
	rate, err := p.currentDisplayRate(ctx, filter.Currency)
	if err != nil {
		return readapi.RegionStat{}, err
	}
	item.MedianSalary = convertMoney(item.MedianSalary, rate)
	item.P25Salary = convertMoney(item.P25Salary, rate)
	item.P75Salary = convertMoney(item.P75Salary, rate)
	item.Currency = displayCurrency(filter.Currency)
	item.RateDate = rate.date
	item.RateProvider = rate.provider
	return item, nil
}

func (p *Postgres) SalaryTrends(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	grain string,
) (readapi.SalaryTrends, error) {
	rateTable, err := p.displayRateTable(ctx, filter.Currency, filter.Period.From, filter.Period.To)
	if err != nil {
		return readapi.SalaryTrends{}, err
	}
	args := append(analyticsArgs(filter), grain)
	rows, err := p.db.Query(ctx, `
		SELECT to_char(date_trunc($6, v.published_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD'),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY v.salary_mid)::float8,
			percentile_cont(0.25) WITHIN GROUP (ORDER BY v.salary_mid)::float8,
			percentile_cont(0.75) WITHIN GROUP (ORDER BY v.salary_mid)::float8,
			count(*)
		FROM vacancies v
		WHERE v.deleted_at IS NULL
			AND EXISTS (SELECT 1 FROM roles sr WHERE sr.id = v.role_id
				AND sr.family IN ('software_development', 'analytics', 'quality_assurance'))
			AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
			AND ($3 = '' OR v.role_id = $3::uuid)
			AND ($4 = '' OR v.region_id = $4::uuid)
			AND ($5 = '' OR v.source = $5)
			AND v.salary_mid BETWEEN 10000 AND 2000000
		GROUP BY date_trunc($6, v.published_at AT TIME ZONE 'UTC')
		ORDER BY date_trunc($6, v.published_at AT TIME ZONE 'UTC')
	`, args...)
	if err != nil {
		return readapi.SalaryTrends{}, fmt.Errorf("query salary trends: %w", err)
	}
	defer rows.Close()

	result := readapi.SalaryTrends{
		Grain: grain, Currency: displayCurrency(filter.Currency),
		Points: []readapi.SalaryPoint{},
	}
	for rows.Next() {
		var point readapi.SalaryPoint
		if err := rows.Scan(&point.PeriodStart, &point.Median, &point.P25, &point.P75, &point.SampleSize); err != nil {
			return readapi.SalaryTrends{}, fmt.Errorf("scan salary trend: %w", err)
		}
		target, _ := time.Parse(time.DateOnly, point.PeriodStart)
		rate := historicalRateFromTable(filter.Currency, target, rateTable)
		if rate.factor == 0 {
			point.Median, point.P25, point.P75 = nil, nil, nil
			point.CoverageWarning = "fx_rate_unavailable"
		} else {
			point.Median = convertMoneyPointer(point.Median, rate)
			point.P25 = convertMoneyPointer(point.P25, rate)
			point.P75 = convertMoneyPointer(point.P75, rate)
			point.RateDate = rate.date
			point.RateProvider = rate.provider
		}
		result.Points = append(result.Points, point)
	}
	if err := rows.Err(); err != nil {
		return readapi.SalaryTrends{}, fmt.Errorf("iterate salary trends: %w", err)
	}
	return result, nil
}

func (p *Postgres) DemandTrends(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	grain string,
) (readapi.DemandTrends, error) {
	rateTable, err := p.displayRateTable(ctx, filter.Currency, filter.Period.From, filter.Period.To)
	if err != nil {
		return readapi.DemandTrends{}, err
	}
	source := filter.Source
	if source == "" {
		source = "hh"
	}
	args := []any{
		filter.Period.From.Format(time.DateOnly),
		filter.Period.To.Format(time.DateOnly),
		filter.RoleGroup,
		filter.RegionID,
		source,
		marketMethodVersion,
	}
	query := `
		SELECT to_char(snapshot_date, 'YYYY-MM-DD'),
			sum(active_count)::bigint,
			sum(published_count)::bigint,
			max(median_salary_rub_net)::float8,
			bool_and(cycle_complete),
			1
		FROM vacancy_demand_daily
		WHERE snapshot_date >= $1::date AND snapshot_date <= $2::date
		  AND ($3 = '' OR role_group = $3)
		  AND (
			($4 = '' AND aggregation_level = 'all_regions' AND region_id IS NULL)
			OR ($4 <> '' AND aggregation_level = 'region' AND region_id = $4::uuid)
		  )
		  AND source = $5
		  AND method_version = $6
		  AND cycle_complete
		GROUP BY snapshot_date
		ORDER BY snapshot_date
	`
	if grain == "week" {
		query = `
			SELECT to_char(week_start, 'YYYY-MM-DD'),
				sum(active_count)::bigint,
				sum(published_count)::bigint,
				max(median_salary_rub_net)::float8,
				bool_and(complete),
				min(source_daily_count)::integer
			FROM vacancy_demand_weekly
			WHERE week_start >= date_trunc('week', $1::date)::date
			  AND week_start <= $2::date
			  AND ($3 = '' OR role_group = $3)
			  AND (
				($4 = '' AND aggregation_level = 'all_regions' AND region_id IS NULL)
				OR ($4 <> '' AND aggregation_level = 'region' AND region_id = $4::uuid)
			  )
			  AND source = $5
			  AND method_version = $6
			  AND complete
			GROUP BY week_start
			ORDER BY week_start
		`
	}
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return readapi.DemandTrends{}, fmt.Errorf("query demand trends: %w", err)
	}
	defer rows.Close()

	result := readapi.DemandTrends{
		Grain: grain, Status: "no_complete_snapshots", Source: source,
		MethodVersion: marketMethodVersion, Points: []readapi.DemandPoint{},
	}
	for rows.Next() {
		var point readapi.DemandPoint
		if err := rows.Scan(
			&point.PeriodStart,
			&point.ActiveCount,
			&point.PublishedCount,
			&point.MedianSalary,
			&point.Complete,
			&point.SourceDayCount,
		); err != nil {
			return readapi.DemandTrends{}, fmt.Errorf("scan demand trend: %w", err)
		}
		point.NewCount = point.PublishedCount
		target, _ := time.Parse(time.DateOnly, point.PeriodStart)
		rate := historicalRateFromTable(filter.Currency, target, rateTable)
		point.Currency = displayCurrency(filter.Currency)
		if rate.factor == 0 {
			point.MedianSalary = nil
			point.CoverageWarning = "fx_rate_unavailable"
		} else {
			point.MedianSalary = convertMoneyPointer(point.MedianSalary, rate)
			point.RateDate = rate.date
			point.RateProvider = rate.provider
		}
		result.Points = append(result.Points, point)
	}
	if err := rows.Err(); err != nil {
		return readapi.DemandTrends{}, fmt.Errorf("iterate demand trends: %w", err)
	}
	if len(result.Points) > 0 {
		result.Status = "ready"
	}
	return result, nil
}

func (p *Postgres) TrendsCoverage(ctx context.Context) (readapi.TrendsCoverage, error) {
	result := readapi.TrendsCoverage{
		Status: "collecting", Source: "hh", AvailableYears: []int{},
		Regions: []readapi.CoverageRegion{},
	}
	var first, last, publicationFrom, publicationTo, latestIncomplete *time.Time
	err := p.db.QueryRow(ctx, `
		SELECT
			min(snapshot_date),
			max(snapshot_date),
			min(snapshot_date) FILTER (WHERE published_count > 0),
			max(snapshot_date) FILTER (WHERE published_count > 0),
			count(DISTINCT snapshot_date) FILTER (WHERE cycle_complete),
			(
				SELECT count(DISTINCT week_start)
				FROM vacancy_demand_weekly
				WHERE source = 'hh' AND method_version = $1 AND complete
			),
			(
				SELECT max(cycle_end)
				FROM ingest_cycles
				WHERE source = 'hh' AND scope = 'daily_discovery'
				  AND status = 'complete' AND method_version = $1
			),
			coalesce((
				SELECT greatest(
					(
						CASE
							WHEN (now() AT TIME ZONE 'UTC')::time >= time '01:00'
							THEN current_date - 1
							ELSE current_date - 2
						END
					) - min(cycle_date) + 1,
					0
				)
				FROM ingest_cycles
				WHERE source = 'hh' AND scope = 'daily_discovery'
				  AND method_version = $1
			), 0),
			coalesce((
				SELECT count(*)
				FROM ingest_cycles
				WHERE source = 'hh' AND scope = 'daily_discovery'
				  AND method_version = $1 AND status IN ('running', 'failed')
			), 0),
			(
				SELECT max(cycle_date)
				FROM ingest_cycles
				WHERE source = 'hh' AND scope = 'daily_discovery'
				  AND method_version = $1 AND status IN ('running', 'failed')
			),
			CASE
				WHEN now() AT TIME ZONE 'UTC'
					< date_trunc('day', now() AT TIME ZONE 'UTC') + interval '1 hour'
				THEN date_trunc('day', now() AT TIME ZONE 'UTC') + interval '1 hour'
				ELSE date_trunc('day', now() AT TIME ZONE 'UTC') + interval '25 hours'
			END AT TIME ZONE 'UTC'
		FROM vacancy_demand_daily
		WHERE source = 'hh' AND method_version = $1
		  AND aggregation_level = 'all_regions'
	`, marketMethodVersion).Scan(
		&first,
		&last,
		&publicationFrom,
		&publicationTo,
		&result.CompleteDailyCount,
		&result.CompleteWeeklyCount,
		&result.LatestCompleteCycle,
		&result.ExpectedDailyCount,
		&result.IncompleteDailyCount,
		&latestIncomplete,
		&result.NextScheduledCycle,
	)
	if err != nil {
		return readapi.TrendsCoverage{}, fmt.Errorf("query trends coverage: %w", err)
	}
	if first != nil {
		result.FirstObservation = datePointer(*first)
		result.LastObservation = datePointer(*last)
		result.MethodVersion = marketMethodVersion
		result.Status = "ready"
	}
	result.MissedDailyCount = result.ExpectedDailyCount -
		result.CompleteDailyCount - result.IncompleteDailyCount
	if result.MissedDailyCount < 0 {
		result.MissedDailyCount = 0
	}
	if result.MissedDailyCount > 0 || result.IncompleteDailyCount > 0 {
		result.Status = "degraded"
	}
	if publicationFrom != nil {
		result.PublicationFrom = datePointer(*publicationFrom)
		result.PublicationTo = datePointer(*publicationTo)
	}
	if latestIncomplete != nil {
		result.LatestIncompleteDate = datePointer(*latestIncomplete)
	}

	yearRows, err := p.db.Query(ctx, `
		SELECT DISTINCT extract(year FROM snapshot_date)::integer
		FROM vacancy_demand_daily
		WHERE source = 'hh' AND method_version = $1 AND cycle_complete
		ORDER BY 1
	`, marketMethodVersion)
	if err != nil {
		return readapi.TrendsCoverage{}, fmt.Errorf("query coverage years: %w", err)
	}
	for yearRows.Next() {
		var year int
		if err := yearRows.Scan(&year); err != nil {
			yearRows.Close()
			return readapi.TrendsCoverage{}, fmt.Errorf("scan coverage year: %w", err)
		}
		result.AvailableYears = append(result.AvailableYears, year)
	}
	if err := yearRows.Err(); err != nil {
		yearRows.Close()
		return readapi.TrendsCoverage{}, fmt.Errorf("iterate coverage years: %w", err)
	}
	yearRows.Close()

	regionRows, err := p.db.Query(ctx, `
		SELECT DISTINCT r.id::text, r.name
		FROM vacancy_demand_daily d
		JOIN regions r ON r.id = d.region_id
		WHERE d.source = 'hh'
		  AND d.method_version = $1
		  AND d.aggregation_level = 'region'
		ORDER BY r.name, r.id::text
	`, marketMethodVersion)
	if err != nil {
		return readapi.TrendsCoverage{}, fmt.Errorf("query coverage regions: %w", err)
	}
	defer regionRows.Close()
	for regionRows.Next() {
		var region readapi.CoverageRegion
		if err := regionRows.Scan(&region.RegionID, &region.Name); err != nil {
			return readapi.TrendsCoverage{}, fmt.Errorf("scan coverage region: %w", err)
		}
		result.Regions = append(result.Regions, region)
	}
	if err := regionRows.Err(); err != nil {
		return readapi.TrendsCoverage{}, fmt.Errorf("iterate coverage regions: %w", err)
	}
	return result, nil
}

func (p *Postgres) TopSkills(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	page readapi.Page,
) (readapi.TopSkills, error) {
	args := append(analyticsArgs(filter), page.Size, (page.Number-1)*page.Size)
	rows, err := p.db.Query(ctx, `
		WITH base AS (
			SELECT v.id
			FROM vacancies v
			WHERE v.deleted_at IS NULL AND v.is_active
				AND EXISTS (SELECT 1 FROM roles sr WHERE sr.id = v.role_id
					AND sr.family IN ('software_development', 'analytics', 'quality_assurance'))
				AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
				AND ($3 = '' OR v.role_id = $3::uuid)
				AND ($4 = '' OR v.region_id = $4::uuid)
				AND ($5 = '' OR v.source = $5)
		), total AS (
			SELECT count(*)::float8 AS value FROM base
		), ranked AS (
			SELECT
				s.id::text AS skill_id,
				s.name,
				count(*)::bigint AS vacancy_count,
				coalesce(count(*) / nullif(total.value, 0), 0)::float8 AS share
			FROM base
			JOIN vacancy_skills vs ON vs.vacancy_id = base.id
			JOIN skills s ON s.id = vs.skill_id AND s.is_active
			CROSS JOIN total
			GROUP BY s.id, s.name, total.value
		), page_data AS (
			SELECT skill_id, name, vacancy_count, share
			FROM ranked
			ORDER BY vacancy_count DESC, name, skill_id
			LIMIT $6 OFFSET $7
		)
		SELECT
			page_data.skill_id,
			page_data.name,
			page_data.vacancy_count,
			page_data.share,
			metadata.total
		FROM (SELECT count(*)::bigint AS total FROM ranked) metadata
		LEFT JOIN page_data ON true
		ORDER BY page_data.vacancy_count DESC NULLS LAST, page_data.name, page_data.skill_id
	`, args...)
	if err != nil {
		return readapi.TopSkills{}, fmt.Errorf("query top skills: %w", err)
	}
	defer rows.Close()

	result := readapi.TopSkills{
		Data:     []readapi.SkillStat{},
		Page:     page.Number,
		PageSize: page.Size,
	}
	for rows.Next() {
		var skillID, name pgtype.Text
		var count pgtype.Int8
		var share pgtype.Float8
		if err := rows.Scan(&skillID, &name, &count, &share, &result.Total); err != nil {
			return readapi.TopSkills{}, fmt.Errorf("scan top skill: %w", err)
		}
		if skillID.Valid {
			result.Data = append(result.Data, readapi.SkillStat{
				SkillID: skillID.String,
				Name:    name.String,
				Count:   count.Int64,
				Share:   share.Float64,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return readapi.TopSkills{}, fmt.Errorf("iterate top skills: %w", err)
	}
	return result, nil
}

const minimumRankingSalarySample = 5

func (p *Postgres) ProgrammingLanguages(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	page readapi.Page,
	metric readapi.RankingMetric,
) (readapi.RankingPage, error) {
	dimension := `
		SELECT DISTINCT v.id AS vacancy_id, v.salary_mid, s.id, s.name
		FROM vacancies v
		JOIN vacancy_skills vs ON vs.vacancy_id = v.id
		JOIN skills s ON s.id = vs.skill_id
			AND s.is_active AND s.skill_kind = 'programming_language'
		WHERE v.deleted_at IS NULL AND v.is_active
			AND EXISTS (
				SELECT 1 FROM vacancy_role_scopes scope
				WHERE scope.vacancy_id = v.id AND scope.scope = 'vacancy_listing'
			)
			AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
			AND ($3 = '' OR v.role_id = $3::uuid)
			AND ($4 = '' OR v.region_id = $4::uuid)
			AND ($5 = '' OR v.source = $5)
	`
	return p.ranking(ctx, filter, page, metric, dimension, "programming languages")
}

func (p *Postgres) ManagementRoles(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	page readapi.Page,
	metric readapi.RankingMetric,
) (readapi.RankingPage, error) {
	dimension := `
		SELECT DISTINCT v.id AS vacancy_id, v.salary_mid, r.id, r.title AS name
		FROM vacancies v
		JOIN vacancy_role_scopes scope ON scope.vacancy_id = v.id
			AND scope.scope = 'management_analytics'
		JOIN roles r ON r.id = scope.role_id AND r.is_active
		WHERE v.deleted_at IS NULL AND v.is_active
			AND v.published_at >= $1::date AND v.published_at < ($2::date + interval '1 day')
			AND ($3 = '' OR v.role_id = $3::uuid)
			AND ($4 = '' OR v.region_id = $4::uuid)
			AND ($5 = '' OR v.source = $5)
	`
	return p.ranking(ctx, filter, page, metric, dimension, "management roles")
}

func (p *Postgres) ranking(
	ctx context.Context,
	filter readapi.AnalyticsFilter,
	page readapi.Page,
	metric readapi.RankingMetric,
	dimensionSQL string,
	label string,
) (readapi.RankingPage, error) {
	order := "vacancy_count DESC, name, id"
	if metric == readapi.RankingBySalary {
		order = "median_salary_rub DESC, salary_sample_size DESC, vacancy_count DESC, name, id"
	}
	args := append(
		analyticsArgs(filter),
		minimumRankingSalarySample,
		page.Size,
		(page.Number-1)*page.Size,
		minimumRankingSalarySample,
	)
	rows, err := p.db.Query(ctx, `
		WITH dimension AS (`+dimensionSQL+`),
		denominator AS (
			SELECT count(DISTINCT vacancy_id)::bigint AS value FROM dimension
		), stats AS (
			SELECT id, name,
				count(DISTINCT vacancy_id)::bigint AS vacancy_count,
				count(DISTINCT vacancy_id) FILTER (
					WHERE salary_mid BETWEEN 10000 AND 2000000
				)::bigint AS salary_sample_size,
				percentile_cont(0.5) WITHIN GROUP (ORDER BY salary_mid)
					FILTER (WHERE salary_mid BETWEEN 10000 AND 2000000)::float8 AS median_salary_rub
			FROM dimension
			GROUP BY id, name
		), eligible AS (
			SELECT stats.*,
				coalesce(vacancy_count::float8 / nullif(denominator.value, 0), 0)::float8 AS share
			FROM stats CROSS JOIN denominator
			WHERE $6::integer = 0 OR salary_sample_size >= $6
		), ranked AS (
			SELECT *,
				row_number() OVER (ORDER BY `+order+`)::bigint AS rank,
				count(*) OVER()::bigint AS total
			FROM eligible
		), page_data AS (
			SELECT * FROM ranked
			ORDER BY `+order+`
			LIMIT $7 OFFSET $8
		)
		SELECT page_data.id::text, page_data.name, page_data.rank,
			page_data.vacancy_count, page_data.share,
			CASE WHEN page_data.salary_sample_size >= $9 THEN page_data.median_salary_rub END,
			page_data.salary_sample_size,
			coalesce(page_data.total, (SELECT count(*) FROM eligible)),
			denominator.value
		FROM denominator
		LEFT JOIN page_data ON true
		ORDER BY page_data.rank NULLS LAST
	`, argsForRanking(args, metric)...)
	if err != nil {
		return readapi.RankingPage{}, fmt.Errorf("query %s ranking: %w", label, err)
	}
	defer rows.Close()

	result := readapi.RankingPage{
		Data: []readapi.RankingItem{}, Metric: metric,
		MinSalarySampleSize: minimumRankingSalarySample,
		Page:                page.Number, PageSize: page.Size,
	}
	for rows.Next() {
		var id, name pgtype.Text
		var rank, count, sample pgtype.Int8
		var share, salary pgtype.Float8
		if err := rows.Scan(
			&id, &name, &rank, &count, &share, &salary, &sample,
			&result.Total, &result.Denominator,
		); err != nil {
			return readapi.RankingPage{}, fmt.Errorf("scan %s ranking: %w", label, err)
		}
		if !id.Valid {
			continue
		}
		item := readapi.RankingItem{
			ID: id.String, Name: name.String, Rank: rank.Int64,
			VacancyCount: count.Int64, Share: share.Float64,
			SalarySampleSize: sample.Int64,
		}
		if salary.Valid {
			value := salary.Float64
			item.MedianSalaryRUB = &value
		}
		result.Data = append(result.Data, item)
	}
	if err := rows.Err(); err != nil {
		return readapi.RankingPage{}, fmt.Errorf("iterate %s ranking: %w", label, err)
	}
	rate, err := p.currentDisplayRate(ctx, filter.Currency)
	if err != nil {
		return readapi.RankingPage{}, err
	}
	result.Currency = displayCurrency(filter.Currency)
	result.RateDate = rate.date
	result.RateProvider = rate.provider
	for index := range result.Data {
		result.Data[index].MedianSalary = convertMoneyPointer(
			result.Data[index].MedianSalaryRUB,
			rate,
		)
	}
	return result, nil
}

func argsForRanking(args []any, metric readapi.RankingMetric) []any {
	minimum := 0
	if metric == readapi.RankingBySalary {
		minimum = minimumRankingSalarySample
	}
	args[5] = minimum
	return args
}

func (p *Postgres) ListVacancies(ctx context.Context, filter readapi.VacancyFilter) (readapi.VacancyPage, error) {
	rate, err := p.currentDisplayRate(ctx, filter.Currency)
	if err != nil {
		return readapi.VacancyPage{}, err
	}
	salaryMin := toCanonicalRUB(filter.SalaryMin, rate)
	salaryMax := toCanonicalRUB(filter.SalaryMax, rate)
	args := []any{
		filter.Query, nonNilStrings(filter.RoleIDs), nonNilStrings(filter.RegionIDs), filter.Source, filter.OnlyActive,
		salaryMin, salaryMax, nonNilStrings(filter.SkillIDs),
	}
	conditions := `
		v.deleted_at IS NULL
			AND EXISTS (
				SELECT 1 FROM vacancy_role_scopes listing_scope
				WHERE listing_scope.vacancy_id = v.id
				  AND listing_scope.scope = 'vacancy_listing'
			)
			AND ($1 = '' OR v.title ILIKE '%' || $1 || '%')
			AND (cardinality($2::uuid[]) = 0 OR v.role_id = ANY($2::uuid[]))
			AND (cardinality($3::uuid[]) = 0 OR v.region_id = ANY($3::uuid[]))
			AND ($4 = '' OR v.source = $4)
			AND (NOT $5 OR v.is_active)
			AND ($6::float8 IS NULL OR v.salary_mid >= $6)
			AND ($7::float8 IS NULL OR v.salary_mid <= $7)
			AND (
				cardinality($8::uuid[]) = 0
				OR EXISTS (
					SELECT 1
					FROM vacancy_skills filter_vs
					WHERE filter_vs.vacancy_id = v.id
					  AND filter_vs.skill_id = ANY($8::uuid[])
				)
			)
	`
	var total int64
	if err := p.db.QueryRow(ctx, `SELECT count(*) FROM vacancies v WHERE `+conditions, args...).Scan(&total); err != nil {
		return readapi.VacancyPage{}, fmt.Errorf("count vacancies: %w", err)
	}

	args = append(args, filter.Page.Size, (filter.Page.Number-1)*filter.Page.Size)
	rows, err := p.db.Query(ctx, `
		SELECT v.id::text, v.source, src.name, v.source_url, v.external_id, v.title,
			v.role_id::text, v.region_id::text,
			v.salary_from_rub_net::float8,
			v.salary_to_rub_net::float8,
			CASE WHEN v.salary_mid IS NOT NULL THEN $11::text END,
			CASE WHEN v.salary_mid IS NOT NULL THEN false END,
			v.salary_from_rub_net::float8, v.salary_to_rub_net::float8,
			v.published_at, v.first_observed_at,
			(v.published_at IS NOT NULL
				AND v.published_at <= current_timestamp
				AND v.published_at >= current_timestamp - interval '24 hours'),
			v.is_active,
			coalesce(array_agg(s.name ORDER BY s.name) FILTER (WHERE s.id IS NOT NULL), ARRAY[]::text[])
		FROM vacancies v
		JOIN sources src ON src.code = v.source
		LEFT JOIN vacancy_skills vs ON vs.vacancy_id = v.id
		LEFT JOIN skills s ON s.id = vs.skill_id
		WHERE `+conditions+`
		GROUP BY v.id, src.name
		ORDER BY v.published_at DESC NULLS LAST, v.id
		LIMIT $9 OFFSET $10
	`, append(args, displayCurrency(filter.Currency))...)
	if err != nil {
		return readapi.VacancyPage{}, fmt.Errorf("query vacancies: %w", err)
	}
	defer rows.Close()

	result := readapi.VacancyPage{
		Data:     []readapi.Vacancy{},
		Page:     filter.Page.Number,
		PageSize: filter.Page.Size,
		Total:    total,
	}
	for rows.Next() {
		var item readapi.Vacancy
		if err := rows.Scan(
			&item.ID,
			&item.Source,
			&item.SourceName,
			&item.SourceURL,
			&item.ExternalID,
			&item.Title,
			&item.RoleID,
			&item.RegionID,
			&item.SalaryFrom,
			&item.SalaryTo,
			&item.SalaryCurrency,
			&item.SalaryGross,
			&item.SalaryFromRUBNet,
			&item.SalaryToRUBNet,
			&item.PublishedAt,
			&item.FirstObservedAt,
			&item.IsFresh,
			&item.IsActive,
			&item.Skills,
		); err != nil {
			return readapi.VacancyPage{}, fmt.Errorf("scan vacancy: %w", err)
		}
		if item.PublishedAt != nil {
			publishedAt := item.PublishedAt.UTC()
			item.PublishedAt = &publishedAt
		}
		if item.FirstObservedAt != nil {
			firstObservedAt := item.FirstObservedAt.UTC()
			item.FirstObservedAt = &firstObservedAt
		}
		item.SalaryFrom = convertMoneyPointer(item.SalaryFromRUBNet, rate)
		item.SalaryTo = convertMoneyPointer(item.SalaryToRUBNet, rate)
		item.SalaryRateDate = rate.date
		item.SalaryRateProvider = rate.provider
		result.Data = append(result.Data, item)
	}
	if err := rows.Err(); err != nil {
		return readapi.VacancyPage{}, fmt.Errorf("iterate vacancies: %w", err)
	}
	return result, nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func analyticsArgs(filter readapi.AnalyticsFilter) []any {
	return []any{
		filter.Period.From.Format(time.DateOnly),
		filter.Period.To.Format(time.DateOnly),
		filter.RoleID,
		filter.RegionID,
		filter.Source,
	}
}

func periodResponse(period readapi.Period) readapi.PeriodResponse {
	return readapi.PeriodResponse{
		From: period.From.Format(time.DateOnly),
		To:   period.To.Format(time.DateOnly),
	}
}

func datePointer(value time.Time) *string {
	formatted := value.UTC().Format(time.DateOnly)
	return &formatted
}

var supportedDisplayCurrencies = []readapi.CurrencyRate{
	{Code: "RUB", Label: "Российский рубль", Symbol: "₽", Available: true},
	{Code: "USD", Label: "Доллар США", Symbol: "$"},
	{Code: "EUR", Label: "Евро", Symbol: "€"},
	{Code: "CNY", Label: "Китайский юань", Symbol: "¥"},
	{Code: "KZT", Label: "Казахстанский тенге", Symbol: "₸"},
	{Code: "AMD", Label: "Армянский драм", Symbol: "֏"},
}

func (p *Postgres) Currencies(ctx context.Context) (readapi.CurrenciesResponse, error) {
	result := readapi.CurrenciesResponse{
		BaseCurrency: "RUB",
		Rates:        []readapi.CurrencyRate{supportedDisplayCurrencies[0]},
	}
	rows, err := p.db.Query(ctx, `
		SELECT DISTINCT ON (quote_currency)
			quote_currency, rate_date, provider,
			(current_date - rate_date)::integer
		FROM fx_rates
		WHERE quote_currency = ANY($1)
		ORDER BY quote_currency, rate_date DESC, provider
	`, displayCurrencyCodes()[1:])
	if err != nil {
		return readapi.CurrenciesResponse{}, fmt.Errorf("query currencies: %w", err)
	}
	defer rows.Close()
	metadata := make(map[string]readapi.CurrencyRate, len(supportedDisplayCurrencies)-1)
	for _, currency := range supportedDisplayCurrencies[1:] {
		metadata[currency.Code] = currency
	}
	for rows.Next() {
		var code, provider string
		var date time.Time
		var stale int
		if err := rows.Scan(&code, &date, &provider, &stale); err != nil {
			return readapi.CurrenciesResponse{}, fmt.Errorf("scan currency: %w", err)
		}
		item := metadata[code]
		formatted := date.UTC().Format(time.DateOnly)
		item.RateDate = &formatted
		item.Provider = provider
		item.StaleDays = &stale
		item.Available = stale <= 7
		metadata[code] = item
	}
	if err := rows.Err(); err != nil {
		return readapi.CurrenciesResponse{}, fmt.Errorf("iterate currencies: %w", err)
	}
	for _, code := range displayCurrencyCodes()[1:] {
		result.Rates = append(result.Rates, metadata[code])
	}
	return result, nil
}

func displayCurrencyCodes() []string {
	codes := make([]string, 0, len(supportedDisplayCurrencies))
	for _, currency := range supportedDisplayCurrencies {
		codes = append(codes, currency.Code)
	}
	return codes
}

func (p *Postgres) currentDisplayRate(ctx context.Context, currency string) (displayRate, error) {
	currency = displayCurrency(currency)
	if currency == "RUB" {
		return displayRate{factor: 1}, nil
	}
	var rate displayRate
	var date time.Time
	err := p.db.QueryRow(ctx, `
		SELECT rub_per_unit::float8, rate_date, provider
		FROM fx_rates
		WHERE quote_currency = $1 AND provider = 'cbr'
		  AND rate_date >= current_date - 7
		ORDER BY rate_date DESC
		LIMIT 1
	`, currency).Scan(&rate.factor, &date, &rate.provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return displayRate{}, nil
	}
	if err != nil {
		return displayRate{}, fmt.Errorf("query display FX rate: %w", err)
	}
	formatted := date.UTC().Format(time.DateOnly)
	rate.date = &formatted
	return rate, nil
}

func (p *Postgres) displayRateTable(
	ctx context.Context,
	currency string,
	from, to time.Time,
) (map[string]displayRate, error) {
	if displayCurrency(currency) == "RUB" {
		return nil, nil
	}
	rows, err := p.db.Query(ctx, `
		SELECT rate_date, rub_per_unit::float8, provider
		FROM fx_rates
		WHERE quote_currency = $1 AND provider = 'cbr'
		  AND rate_date BETWEEN $2::date - 7 AND $3::date
		ORDER BY rate_date
	`, displayCurrency(currency), from, to)
	if err != nil {
		return nil, fmt.Errorf("query historical FX rates: %w", err)
	}
	defer rows.Close()
	result := map[string]displayRate{}
	for rows.Next() {
		var date time.Time
		var rate displayRate
		if err := rows.Scan(&date, &rate.factor, &rate.provider); err != nil {
			return nil, fmt.Errorf("scan historical FX rate: %w", err)
		}
		formatted := date.UTC().Format(time.DateOnly)
		rate.date = &formatted
		result[formatted] = rate
	}
	return result, rows.Err()
}

func historicalRateFromTable(
	currency string,
	target time.Time,
	rates map[string]displayRate,
) displayRate {
	if displayCurrency(currency) == "RUB" {
		return displayRate{factor: 1}
	}
	for days := 0; days <= 7; days++ {
		date := target.UTC().AddDate(0, 0, -days).Format(time.DateOnly)
		if rate, ok := rates[date]; ok {
			return rate
		}
	}
	return displayRate{}
}

func displayCurrency(currency string) string {
	switch currency {
	case "USD", "EUR", "CNY", "KZT", "AMD":
		return currency
	default:
		return "RUB"
	}
}

func convertMoney(value float64, rate displayRate) float64 {
	if rate.factor <= 0 {
		return 0
	}
	return math.Round(value/rate.factor*100) / 100
}

func convertMoneyPointer(value *float64, rate displayRate) *float64 {
	if value == nil || rate.factor <= 0 {
		return nil
	}
	converted := convertMoney(*value, rate)
	return &converted
}

func toCanonicalRUB(value *float64, rate displayRate) *float64 {
	if value == nil || rate.factor <= 0 {
		return nil
	}
	converted := math.Round(*value*rate.factor*100) / 100
	return &converted
}
