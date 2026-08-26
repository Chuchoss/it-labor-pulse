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

	var roleID, regionID, skillID string
	err = tx.QueryRow(ctx, `
		INSERT INTO roles (slug, title, family)
		VALUES ($1, 'Synthetic Backend Role', 'backend')
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

	repository := NewPostgres(tx)
	page, err := repository.ListVacancies(ctx, readapi.VacancyFilter{
		Query:      "Synthetic Go",
		OnlyActive: true,
		Page:       readapi.Page{Number: 1, Size: 10},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.Data, 1)
	require.Equal(t, vacancyID, page.Data[0].ID)
	require.Equal(t, []string{"Synthetic Skill"}, page.Data[0].Skills)

	filter := readapi.AnalyticsFilter{
		Period: readapi.Period{
			From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		},
		Source: source,
	}
	summary, err := repository.Dashboard(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 1, summary.VacanciesActive)
	require.EqualValues(t, 1, summary.VacanciesNew)
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
	require.EqualValues(t, 1, role.VacanciesCount)

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
	require.EqualValues(t, 1, region.VacanciesCount)

	salaries, err := repository.SalaryTrends(ctx, dimensionFilter, "month")
	require.NoError(t, err)
	require.Len(t, salaries.Points, 1)
	require.Equal(t, 150000.0, salaries.Points[0].Median)

	demand, err := repository.DemandTrends(ctx, dimensionFilter, "month")
	require.NoError(t, err)
	require.NotEmpty(t, demand.Points)

	skills, err := repository.TopSkills(ctx, dimensionFilter, 10)
	require.NoError(t, err)
	require.Len(t, skills.Data, 1)
	require.Equal(t, skillID, skills.Data[0].SkillID)
	require.Equal(t, 1.0, skills.Data[0].Share)
}
