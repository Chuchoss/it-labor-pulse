package normalize_test

import (
	"testing"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"

	"github.com/stretchr/testify/require"
)

func TestRoleMatch(t *testing.T) {
	t.Parallel()
	m := normalize.MapRoleMatcher{
		RoleByExternalID: map[string]map[string]string{
			"hh": {"96": "go-developer"},
		},
		TitlePatterns: []normalize.TitleRoleRule{
			{Pattern: "qa engineer", RoleID: "qa-engineer", Contains: true},
		},
	}
	id, ok := m.MatchRole("hh", []string{"96"}, "Anything")
	require.True(t, ok)
	require.Equal(t, "go-developer", id)

	id, ok = m.MatchRole("hh", nil, "Junior QA Engineer")
	require.True(t, ok)
	require.Equal(t, "qa-engineer", id)

	_, ok = m.MatchRole("hh", nil, "Unknown title")
	require.False(t, ok)
}

func TestSkillDedupAndSlug(t *testing.T) {
	t.Parallel()
	collected := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	opts := normalize.DefaultOptions()
	opts.Skills = normalize.SlugSkillMatcher{
		Aliases: map[string]string{"go": "go", "golang": "go"},
	}
	res, err := normalize.Normalize(normalize.Draft{
		Source: "hh", ExternalID: "1", Title: "Dev",
		SkillsRaw:   []string{"Go", "golang", "Kafka", "Go"},
		CollectedAt: collected,
	}, opts)
	require.NoError(t, err)
	require.Len(t, res.Vacancy.Skills, 2)
	require.Equal(t, "go", res.Vacancy.Skills[0].SkillID)
	require.False(t, res.Vacancy.Skills[0].IsNew)
	require.Equal(t, "kafka", res.Vacancy.Skills[1].SkillID)
	require.True(t, res.Vacancy.Skills[1].IsNew)
}

func TestProgrammingLanguageAliasesDoNotCollapseAmbiguousCategories(t *testing.T) {
	t.Parallel()
	opts := normalize.DefaultOptions()
	opts.Skills = normalize.SlugSkillMatcher{Aliases: map[string]string{
		"javascript": "javascript", "js": "javascript",
		"typescript": "typescript", "ts": "typescript",
		"c#": "c-sharp", "c-sharp": "c-sharp", "csharp": "c-sharp",
		"sql": "sql", "html": "html", "css": "css", "bash": "bash", "1c": "1c",
	}}
	result, err := normalize.Normalize(normalize.Draft{
		Source: "hh", ExternalID: "taxonomy", Title: "Synthetic",
		CollectedAt: time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC),
		SkillsRaw: []string{
			"JavaScript", "JS", "TypeScript", "TS", "C#", "C Sharp", "CSharp",
			"SQL", "HTML", "CSS", "Bash", "1C",
		},
	}, opts)
	require.NoError(t, err)
	require.Equal(t, []string{
		"javascript", "typescript", "c-sharp", "sql", "html", "css", "bash", "1c",
	}, skillIDs(result.Vacancy.Skills))
}

func skillIDs(skills []normalize.SkillRef) []string {
	result := make([]string, 0, len(skills))
	for _, skill := range skills {
		result = append(result, skill.SkillID)
	}
	return result
}

func TestNormalize_roleUnmappedMetric(t *testing.T) {
	t.Parallel()
	res, err := normalize.Normalize(normalize.Draft{
		Source: "hh", ExternalID: "1", Title: "Mystery Role",
		CollectedAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
	}, normalize.DefaultOptions())
	require.NoError(t, err)
	require.Nil(t, res.Vacancy.RoleID)
	require.True(t, res.Metrics.RoleUnmapped)
}

func TestNormalizeTitle(t *testing.T) {
	t.Parallel()
	require.Equal(t, "senior go developer", normalize.NormalizeTitle("  Senior Go-Developer!!! "))
	require.Equal(t, "удаленная работа", normalize.NormalizeTitle("Удалённая работа"))
}
