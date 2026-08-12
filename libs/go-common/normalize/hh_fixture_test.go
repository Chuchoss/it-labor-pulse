package normalize_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
	"github.com/stretchr/testify/require"
)

// hhVacancy is a minimal HH-shaped payload for fixture → Draft mapping (adapter stub).
// Does not apply shared salary/currency rules.
type hhVacancy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Area *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"area"`
	Salary *struct {
		From     *float64 `json:"from"`
		To       *float64 `json:"to"`
		Currency string   `json:"currency"`
		Gross    *bool    `json:"gross"`
	} `json:"salary"`
	Employer *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"employer"`
	KeySkills []struct {
		Name string `json:"name"`
	} `json:"key_skills"`
	ProfessionalRoles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"professional_roles"`
	Schedule *struct {
		ID string `json:"id"`
	} `json:"schedule"`
	WorkFormat []struct {
		ID string `json:"id"`
	} `json:"work_format"`
	Description string `json:"description"`
	PublishedAt string `json:"published_at"`
	Archived    bool   `json:"archived"`
}

type hhSearchPage struct {
	Items []hhVacancy `json:"items"`
}

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

func readHHFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(moduleRoot(t), "testdata", "hh", name)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

func draftFromHHVacancy(t *testing.T, v hhVacancy, collectedAt time.Time) normalize.Draft {
	t.Helper()
	d := normalize.Draft{
		SchemaVersion:   normalize.SchemaVersionV1,
		Source:          "hh",
		ExternalID:      v.ID,
		Title:           v.Name,
		CollectedAt:     collectedAt,
		DescriptionText: stripTagsLite(v.Description),
	}
	if v.Area != nil {
		d.RegionExternalID = v.Area.ID
		d.RegionName = v.Area.Name
	}
	if v.Employer != nil {
		d.EmployerExternalID = v.Employer.ID
		d.EmployerName = v.Employer.Name
	}
	if v.Salary != nil {
		d.SalaryFrom = v.Salary.From
		d.SalaryTo = v.Salary.To
		d.SalaryCurrencyRaw = v.Salary.Currency
		d.SalaryGross = v.Salary.Gross
	}
	for _, s := range v.KeySkills {
		d.SkillsRaw = append(d.SkillsRaw, s.Name)
	}
	for _, r := range v.ProfessionalRoles {
		d.ProfessionalRoleIDs = append(d.ProfessionalRoleIDs, r.ID)
	}
	if v.Schedule != nil {
		d.ScheduleID = v.Schedule.ID
	}
	for _, wf := range v.WorkFormat {
		d.WorkFormatIDs = append(d.WorkFormatIDs, wf.ID)
	}
	if v.PublishedAt != "" {
		ts, err := time.Parse("2006-01-02T15:04:05-0700", v.PublishedAt)
		require.NoError(t, err, "published_at")
		d.PublishedAt = ts
	}
	active := !v.Archived
	d.IsActiveHint = &active
	return d
}

func draftFromFixture(t *testing.T, name string, collectedAt time.Time) normalize.Draft {
	t.Helper()
	var v hhVacancy
	require.NoError(t, json.Unmarshal(readHHFixture(t, name), &v))
	require.NotEmpty(t, v.ID)
	return draftFromHHVacancy(t, v, collectedAt)
}

func stripTagsLite(s string) string {
	// minimal strip for remote keyword tests; full HTML policy lives in adapter
	out := make([]byte, 0, len(s))
	inTag := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out = append(out, s[i])
			}
		}
	}
	return string(out)
}

func f64(v float64) *float64 { return &v }
func b(v bool) *bool         { return &v }
