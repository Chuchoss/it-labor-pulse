package hh_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
	"github.com/stretchr/testify/require"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found")
		dir = parent
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "testdata", "hh", name))
	require.NoError(t, err)
	return raw
}

func TestParseSearchPage(t *testing.T) {
	page, err := hh.ParseSearchPage(readFixture(t, "vacancy_search_page.json"))
	require.NoError(t, err)
	require.Equal(t, 2, page.Found)
	require.Equal(t, 0, page.Page)
	require.Equal(t, 1, page.Pages)
	require.Equal(t, 20, page.PerPage)
	require.Len(t, page.Items, 2)
	require.Equal(t, "900001", page.Items[0].ID)
	require.Equal(t, "900002", page.Items[1].ID)
}

func TestDraftFromDetail(t *testing.T) {
	collected := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	d, err := hh.DraftFromDetail(readFixture(t, "vacancy_detail.json"), collected)
	require.NoError(t, err)
	require.Equal(t, normalize.SchemaVersionV1, d.SchemaVersion)
	require.Equal(t, "hh", d.Source)
	require.Equal(t, "900001", d.ExternalID)
	require.Equal(t, "Senior Go Developer", d.Title)
	require.Equal(t, "70001", d.EmployerExternalID)
	require.Equal(t, "1", d.RegionExternalID)
	require.Equal(t, "Москва", d.RegionName)
	require.NotNil(t, d.SalaryFrom)
	require.InDelta(t, 300000, *d.SalaryFrom, 0.01)
	require.NotNil(t, d.SalaryTo)
	require.InDelta(t, 400000, *d.SalaryTo, 0.01)
	require.Equal(t, "RUR", d.SalaryCurrencyRaw)
	require.NotNil(t, d.SalaryGross)
	require.True(t, *d.SalaryGross)
	require.Equal(t, []string{"Go", "PostgreSQL", "Kafka", "Kubernetes"}, d.SkillsRaw)
	require.Equal(t, []string{"96"}, d.ProfessionalRoleIDs)
	require.Equal(t, "remote", d.ScheduleID)
	require.Contains(t, d.WorkFormatIDs, "REMOTE")
	require.NotContains(t, d.DescriptionText, "<p>")
	require.Contains(t, d.DescriptionText, "Senior Go")
	require.False(t, d.PublishedAt.IsZero())
	require.NotNil(t, d.IsActiveHint)
	require.True(t, *d.IsActiveHint)
	require.True(t, json.Valid(d.RawPayload))
}

func TestDraftFromDetail_SalaryAbsent(t *testing.T) {
	d, err := hh.DraftFromDetail(readFixture(t, "salary_absent.json"), time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, "900010", d.ExternalID)
	require.Nil(t, d.SalaryFrom)
	require.Nil(t, d.SalaryTo)
	require.Empty(t, d.SalaryCurrencyRaw)
	require.Nil(t, d.SalaryGross)
}

func TestDraftFromDetail_OutlierRawPassthrough(t *testing.T) {
	d, err := hh.DraftFromDetail(readFixture(t, "salary_invalid_outlier.json"), time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, d.SalaryFrom)
	require.InDelta(t, 0, *d.SalaryFrom, 0.01)
	require.NotNil(t, d.SalaryTo)
	require.InDelta(t, 5_000_000, *d.SalaryTo, 0.01)
}

func TestDraftFromDetail_RURRaw(t *testing.T) {
	d, err := hh.DraftFromDetail(readFixture(t, "salary_rur_to_rub.json"), time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, "RUR", d.SalaryCurrencyRaw)

	res, err := normalize.Normalize(d, normalize.DefaultOptions())
	require.NoError(t, err)
	require.Equal(t, "RUB", res.Vacancy.SalaryCurrency)
}

func TestStripHTML_RemovesScript(t *testing.T) {
	in := `<p>Hello</p><script>alert(1)</script><style>.x{}</style><b>World</b>`
	out := hh.StripHTML(in)
	require.Equal(t, "Hello World", out)
	require.NotContains(t, out, "alert")
}
