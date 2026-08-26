//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/apps/bff/internal/readapi"
)

func TestPostgresReadQueries(t *testing.T) {
	_ = godotenv.Load("../../../../.env")
	databaseURL := os.Getenv("BFF_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("BFF_TEST_DATABASE_URL or DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close(context.Background())) }()

	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(context.Background())) }()

	suffix := time.Now().UTC().UnixNano()
	source := fmt.Sprintf("bff-test-%d", suffix)
	_, err = tx.Exec(ctx, `INSERT INTO sources (code, name) VALUES ($1, 'BFF integration source')`, source)
	require.NoError(t, err)

	var roleID, regionID, skillID, alphaSkillID, betaSkillID string
	err = tx.QueryRow(ctx, `
		INSERT INTO roles (slug, title, family)
		VALUES ($1, 'Synthetic Backend Role', 'software_development')
		RETURNING id::text
	`, fmt.Sprintf("bff-role-%d", suffix)).Scan(&roleID)
	require.NoError(t, err)
	err = tx.QueryRow(ctx, `
		INSERT INTO regions (code, name)
		VALUES ($1, 'Synthetic Region')
		RETURNING id::text
	`, fmt.Sprintf("bff-region-%d", suffix)).Scan(&regionID)
	require.NoError(t, err)
	err = tx.QueryRow(ctx, `
		INSERT INTO skills (slug, name)
		VALUES ($1, 'Synthetic Skill')
		RETURNING id::text
	`, fmt.Sprintf("bff-skill-%d", suffix)).Scan(&skillID)
	require.NoError(t, err)
	err = tx.QueryRow(ctx, `
		INSERT INTO skills (slug, name)
		VALUES ($1, 'Alpha Synthetic Skill')
		RETURNING id::text
	`, fmt.Sprintf("bff-alpha-skill-%d", suffix)).Scan(&alphaSkillID)
	require.NoError(t, err)
	err = tx.QueryRow(ctx, `
		INSERT INTO skills (slug, name)
		VALUES ($1, 'Beta Synthetic Skill')
		RETURNING id::text
	`, fmt.Sprintf("bff-beta-skill-%d", suffix)).Scan(&betaSkillID)
	require.NoError(t, err)

	published := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	var vacancyID string
	err = tx.QueryRow(ctx, `
		INSERT INTO vacancies (
			source, external_id, title, role_id, region_id,
			salary_from, salary_to, salary_currency, salary_gross, salary_mid,
			published_at, collected_at, is_active
		) VALUES (
			$1, $2, 'Synthetic Go Vacancy', $3::uuid, $4::uuid,
			100000, 200000, 'RUB', false, 150000,
			$5, $5, true
		)
		RETURNING id::text
	`, source, fmt.Sprintf("vacancy-%d", suffix), roleID, regionID, published).Scan(&vacancyID)
	require.NoError(t, err)
	_, err = tx.Exec(
		ctx,
		`INSERT INTO vacancy_skills (vacancy_id, skill_id) VALUES ($1::uuid, $2::uuid)`,
		vacancyID,
		skillID,
	)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO vacancy_skills (vacancy_id, skill_id)
		VALUES ($1::uuid, $2::uuid), ($1::uuid, $3::uuid)
	`, vacancyID, alphaSkillID, betaSkillID)
	require.NoError(t, err)
	var secondVacancyID string
	err = tx.QueryRow(ctx, `
		INSERT INTO vacancies (
			source, external_id, title, role_id, region_id,
			published_at, collected_at, is_active
		) VALUES ($1, $2, 'Synthetic Missing Salary', $3::uuid, $4::uuid, $5, $5, true)
		RETURNING id::text
	`, source, fmt.Sprintf("missing-salary-%d", suffix), roleID, regionID, published).Scan(&secondVacancyID)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO vacancy_skills (vacancy_id, skill_id)
		VALUES ($1::uuid, $2::uuid), ($1::uuid, $3::uuid)
	`, secondVacancyID, alphaSkillID, betaSkillID)
	require.NoError(t, err)

	var cycleID, analyticsRunID string
	err = tx.QueryRow(ctx, `
		INSERT INTO ingest_cycles (
			source, scope, scope_hash, cycle_end, status, partition_count,
			completed_partitions, started_at, completed_at
		) VALUES (
			$1, 'all_it', repeat('a', 64), $2::timestamptz, 'complete', 1, 1,
			$2::timestamptz - interval '1 hour', $2::timestamptz
		)
		RETURNING id::text
	`, source, published).Scan(&cycleID)
	require.NoError(t, err)
	err = tx.QueryRow(ctx, `
		INSERT INTO analytics_runs (
			run_type, target_period_start, source, source_cycle_id,
			status, method_version, finished_at, row_count
		) VALUES (
			'daily_snapshot', $1::date, $2, $3::uuid,
			'success', 'vacancy_demand_v1', now(), 2
		)
		RETURNING id::text
	`, published, source, cycleID).Scan(&analyticsRunID)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `
		INSERT INTO vacancy_demand_daily (
			snapshot_date, source, role_group, aggregation_level, region_id,
			active_count, published_count, vacancies_with_salary,
			median_salary_rub_net, cycle_complete, source_cycle_id,
			analytics_run_id, method_version, observed_at
		) VALUES
			($1::date, $2, 'software_development', 'all_regions', NULL,
			 2, 2, 1, 150000, true, $3::uuid, $4::uuid, 'vacancy_demand_v1', $1),
			($1::date, $2, 'software_development', 'region', $5::uuid,
			 2, 2, 1, 150000, true, $3::uuid, $4::uuid, 'vacancy_demand_v1', $1)
	`, published, source, cycleID, analyticsRunID, regionID)
	require.NoError(t, err)

	repository := NewPostgres(tx)
	page, err := repository.ListVacancies(ctx, readapi.VacancyFilter{
		Query:      "Synthetic Go",
		RoleIDs:    []string{roleID},
		RegionIDs:  []string{regionID},
		SkillIDs:   []string{skillID},
		SalaryMin:  floatPointer(100000),
		SalaryMax:  floatPointer(200000),
		OnlyActive: true,
		Page:       readapi.Page{Number: 1, Size: 10},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.Data, 1)
	require.Equal(t, vacancyID, page.Data[0].ID)
	require.Equal(
		t,
		[]string{"Alpha Synthetic Skill", "Beta Synthetic Skill", "Synthetic Skill"},
		page.Data[0].Skills,
	)

	allRoleRows, err := repository.ListVacancies(ctx, readapi.VacancyFilter{
		RoleIDs: []string{roleID},
		Page:    readapi.Page{Number: 1, Size: 10},
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, allRoleRows.Total)
	salaryRows, err := repository.ListVacancies(ctx, readapi.VacancyFilter{
		RoleIDs:   []string{roleID},
		SalaryMin: floatPointer(100000),
		Page:      readapi.Page{Number: 1, Size: 10},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, salaryRows.Total)

	noSalaryMatch, err := repository.ListVacancies(ctx, readapi.VacancyFilter{
		SalaryMin: floatPointer(200001),
		RoleIDs:   []string{roleID},
		Page:      readapi.Page{Number: 1, Size: 10},
	})
	require.NoError(t, err)
	require.Zero(t, noSalaryMatch.Total)

	filter := readapi.AnalyticsFilter{
		Period: readapi.Period{
			From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		},
		Source: source,
	}
	summary, err := repository.Dashboard(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 2, summary.VacanciesActive)
	require.EqualValues(t, 2, summary.VacanciesNew)
	require.Equal(t, 150000.0, summary.MedianSalary)
	require.EqualValues(t, 1, summary.SalarySample)

	roles, err := repository.ListRoles(ctx, filter, readapi.Page{Number: 1, Size: 10}, "count")
	require.NoError(t, err)
	require.EqualValues(t, 1, roles.Total)
	require.Len(t, roles.Data, 1)
	require.Equal(t, roleID, roles.Data[0].RoleID)

	role, err := repository.GetRole(ctx, roleID, filter)
	require.NoError(t, err)
	require.Equal(t, roleID, role.RoleID)
	require.EqualValues(t, 2, role.VacanciesCount)

	dimensionFilter := filter
	dimensionFilter.Source = ""
	dimensionFilter.RoleID = roleID
	dimensionFilter.RegionID = regionID

	regions, err := repository.ListRegions(ctx, dimensionFilter, readapi.Page{Number: 1, Size: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, regions.Total)
	require.Len(t, regions.Data, 1)
	require.Equal(t, regionID, regions.Data[0].RegionID)

	region, err := repository.GetRegion(ctx, regionID, dimensionFilter)
	require.NoError(t, err)
	require.Equal(t, regionID, region.RegionID)
	require.EqualValues(t, 2, region.VacanciesCount)

	salaries, err := repository.SalaryTrends(ctx, dimensionFilter, "month")
	require.NoError(t, err)
	require.Len(t, salaries.Points, 1)
	require.Equal(t, 150000.0, salaries.Points[0].Median)

	demandFilter := dimensionFilter
	demandFilter.Source = source
	demandFilter.RoleGroup = "software_development"
	demand, err := repository.DemandTrends(ctx, demandFilter, "day")
	require.NoError(t, err)
	require.NotEmpty(t, demand.Points)
	require.EqualValues(t, 2, demand.Points[0].PublishedCount)

	skills, err := repository.TopSkills(ctx, dimensionFilter, readapi.Page{Number: 1, Size: 1})
	require.NoError(t, err)
	require.Len(t, skills.Data, 1)
	require.EqualValues(t, 3, skills.Total)
	require.Equal(t, alphaSkillID, skills.Data[0].SkillID)
	require.EqualValues(t, 2, skills.Data[0].Count)
	require.Equal(t, 1.0, skills.Data[0].Share)

	nextSkills, err := repository.TopSkills(ctx, dimensionFilter, readapi.Page{Number: 2, Size: 1})
	require.NoError(t, err)
	require.EqualValues(t, 3, nextSkills.Total)
	require.Len(t, nextSkills.Data, 1)
	require.Equal(t, betaSkillID, nextSkills.Data[0].SkillID)

	endSkills, err := repository.TopSkills(ctx, dimensionFilter, readapi.Page{Number: 4, Size: 1})
	require.NoError(t, err)
	require.EqualValues(t, 3, endSkills.Total)
	require.Empty(t, endSkills.Data)
}

func floatPointer(value float64) *float64 { return &value }
