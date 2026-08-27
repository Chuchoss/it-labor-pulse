package normalize_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"

	"github.com/stretchr/testify/require"
)

func TestFixtures_salaryGolden(t *testing.T) {
	t.Parallel()
	collected := time.Date(2026, 8, 10, 7, 0, 0, 0, time.UTC) // UTC date = 2026-08-10

	baseOpts := func() normalize.Options {
		opts := normalize.DefaultOptions()
		opts.Roles = normalize.MapRoleMatcher{
			RoleByExternalID: map[string]map[string]string{
				"hh": {"96": "go-developer"},
			},
			TitlePatterns: []normalize.TitleRoleRule{
				{Pattern: "go developer", RoleID: "go-developer", Contains: true},
				{Pattern: "qa", RoleID: "qa-engineer", Contains: true},
				{Pattern: "data engineer", RoleID: "data-engineer", Contains: true},
				{Pattern: "backend", RoleID: "backend-developer", Contains: true},
			},
		}
		opts.Regions = normalize.MapRegionMatcher{
			"hh": {
				"1": "ru-moscow",
				"2": "ru-spb",
			},
		}
		opts.Skills = normalize.SlugSkillMatcher{
			Aliases: map[string]string{
				"go":         "go",
				"postgresql": "postgresql",
				"kafka":      "kafka",
				"kubernetes": "kubernetes",
			},
		}
		return opts
	}

	t.Run("salary_absent", func(t *testing.T) {
		t.Parallel()
		d := draftFromFixture(t, "salary_absent.json", collected)
		require.Equal(t, "900010", d.ExternalID)
		require.Nil(t, d.SalaryFrom)
		require.Nil(t, d.SalaryTo)

		res, err := normalize.Normalize(d, baseOpts())
		require.NoError(t, err)
		require.Nil(t, res.Vacancy.SalaryMid)
		require.Nil(t, res.Vacancy.SalaryMidRub)
		require.True(t, res.Vacancy.ExcludeFromSalaryAgg)
		require.True(t, res.Vacancy.IsActive)
		require.NotEmpty(t, res.Vacancy.ContentHash)
	})

	t.Run("salary_invalid_outlier", func(t *testing.T) {
		t.Parallel()
		d := draftFromFixture(t, "salary_invalid_outlier.json", collected)
		// adapter passes source facts; normalizer nulls invalid from=0
		require.NotNil(t, d.SalaryFrom)
		require.Equal(t, 0.0, *d.SalaryFrom)
		require.NotNil(t, d.SalaryTo)
		require.Equal(t, 5_000_000.0, *d.SalaryTo)

		res, err := normalize.Normalize(d, baseOpts())
		require.NoError(t, err)
		require.True(t, res.Metrics.SalaryInvalid)
		require.Nil(t, res.Vacancy.SalaryFrom)
		require.NotNil(t, res.Vacancy.SalaryTo)
		require.InDelta(t, 5_000_000.0, *res.Vacancy.SalaryTo, 1e-6)
		require.NotNil(t, res.Vacancy.SalaryMid)
		require.InDelta(t, 5_000_000.0, *res.Vacancy.SalaryMid, 1e-6)
		require.NotNil(t, res.Vacancy.SalaryMidRub)
		require.InDelta(t, 5_000_000.0, *res.Vacancy.SalaryMidRub, 1e-6)
		require.Equal(t, "RUB", res.Vacancy.SalaryCurrency)
		require.True(t, res.Vacancy.ExcludeFromSalaryAgg)
		require.True(t, res.Vacancy.IsActive)
	})

	t.Run("salary_fx_miss", func(t *testing.T) {
		t.Parallel()
		d := draftFromFixture(t, "salary_fx_miss.json", collected)
		require.Equal(t, "USD", d.SalaryCurrencyRaw)

		opts := baseOpts()
		// no FX rates, no static fallback (stage/prod behavior)
		res, err := normalize.Normalize(d, opts)
		require.NoError(t, err)
		require.Equal(t, "USD", res.Vacancy.SalaryCurrency)
		require.NotNil(t, res.Vacancy.SalaryMid)
		require.InDelta(t, 4500.0, *res.Vacancy.SalaryMid, 1e-6)
		require.Nil(t, res.Vacancy.SalaryMidRub)
		require.True(t, res.Metrics.FXMiss)
		require.True(t, res.Vacancy.ExcludeFromSalaryAgg)
	})

	t.Run("salary_rur_to_rub", func(t *testing.T) {
		t.Parallel()
		d := draftFromFixture(t, "salary_rur_to_rub.json", collected)
		require.Equal(t, "RUR", d.SalaryCurrencyRaw)

		res, err := normalize.Normalize(d, baseOpts())
		require.NoError(t, err)
		require.Equal(t, "RUB", res.Vacancy.SalaryCurrency)
		require.NotNil(t, res.Vacancy.SalaryMid)
		require.InDelta(t, 150_000.0, *res.Vacancy.SalaryMid, 1e-6)
		require.NotNil(t, res.Vacancy.SalaryMidRub)
		require.InDelta(t, 150_000.0, *res.Vacancy.SalaryMidRub, 1e-6)
		require.False(t, res.Vacancy.ExcludeFromSalaryAgg)
	})

	t.Run("vacancy_detail", func(t *testing.T) {
		t.Parallel()
		d := draftFromFixture(t, "vacancy_detail.json", collected)
		require.Equal(t, "900001", d.ExternalID)
		require.Equal(t, "RUR", d.SalaryCurrencyRaw)
		require.NotNil(t, d.SalaryGross)
		require.True(t, *d.SalaryGross)

		res, err := normalize.Normalize(d, baseOpts())
		require.NoError(t, err)
		v := res.Vacancy
		require.Equal(t, "RUB", v.SalaryCurrency)
		// mid gross 350000 → net 350000*0.87 = 304500
		require.NotNil(t, v.SalaryMid)
		require.InDelta(t, 304_500.0, *v.SalaryMid, 1e-6)
		require.NotNil(t, v.SalaryMidRub)
		require.InDelta(t, 304_500.0, *v.SalaryMidRub, 1e-6)
		require.False(t, v.ExcludeFromSalaryAgg)
		require.NotNil(t, v.IsRemote)
		require.True(t, *v.IsRemote)
		require.NotNil(t, v.RoleID)
		require.Equal(t, "go-developer", *v.RoleID)
		require.NotNil(t, v.RegionID)
		require.Equal(t, "ru-moscow", *v.RegionID)
		require.Len(t, v.Skills, 4)
		require.NotEmpty(t, v.ContentHash)
	})

	t.Run("vacancy_search_page", func(t *testing.T) {
		t.Parallel()
		var page hhSearchPage
		require.NoError(t, json.Unmarshal(readHHFixture(t, "vacancy_search_page.json"), &page))
		require.GreaterOrEqual(t, len(page.Items), 1)

		opts := baseOpts()
		for _, item := range page.Items {
			d := draftFromHHVacancy(t, item, collected)
			res, err := normalize.Normalize(d, opts)
			require.NoError(t, err, "item %s", d.ExternalID)
			require.Equal(t, d.ExternalID, res.Vacancy.ExternalID)
			require.NotEmpty(t, res.Vacancy.ContentHash)
			if d.SalaryCurrencyRaw != "" {
				require.NotEqual(t, "RUR", res.Vacancy.SalaryCurrency)
			}
		}
	})
}
