package normalize_test

import (
	"testing"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
	"github.com/stretchr/testify/require"
)

func TestContentHash_skillsOrderIndependent(t *testing.T) {
	t.Parallel()
	pub := time.Date(2026, 8, 10, 7, 15, 0, 0, time.UTC)
	a := normalize.CanonicalVacancy{
		Title:              "Go Dev",
		SalaryFrom:         f64(100),
		SalaryTo:           f64(200),
		SalaryCurrency:     "RUB",
		SalaryGross:        b(false),
		SalaryMid:          f64(150),
		RegionExternalID:   "1",
		EmployerExternalID: "e1",
		Skills: []normalize.SkillRef{
			{SkillID: "go"},
			{SkillID: "kafka"},
		},
		PublishedAt: pub,
	}
	bVac := a
	bVac.Skills = []normalize.SkillRef{
		{SkillID: "kafka"},
		{SkillID: "go"},
	}
	require.Equal(t, normalize.ContentHash(a), normalize.ContentHash(bVac))
}

func TestContentHash_ignoresCollectedAt(t *testing.T) {
	t.Parallel()
	pub := time.Date(2026, 8, 10, 7, 15, 0, 0, time.UTC)
	base := normalize.CanonicalVacancy{
		Title:              "Go Dev",
		RegionExternalID:   "1",
		EmployerExternalID: "e1",
		PublishedAt:        pub,
		CollectedAt:        time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
	}
	other := base
	other.CollectedAt = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	require.Equal(t, normalize.ContentHash(base), normalize.ContentHash(other))
}

func TestContentHash_salaryChange(t *testing.T) {
	t.Parallel()
	a := normalize.CanonicalVacancy{
		Title: "Go Dev", SalaryMid: f64(100), SalaryCurrency: "RUB",
		EmployerExternalID: "e1", RegionExternalID: "1",
	}
	bVac := a
	bVac.SalaryMid = f64(200)
	require.NotEqual(t, normalize.ContentHash(a), normalize.ContentHash(bVac))
}
