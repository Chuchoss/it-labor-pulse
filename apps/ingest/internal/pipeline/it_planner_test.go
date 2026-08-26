package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/pipeline"
)

type planningSource struct {
	roles []hh.ProfessionalRole
	found func(hh.SearchQuery) int
}

func (s planningSource) ProfessionalRoles(context.Context, string) ([]hh.ProfessionalRole, error) {
	return s.roles, nil
}

func (s planningSource) SearchVacancies(_ context.Context, q hh.SearchQuery) (hh.SearchPage, error) {
	found := s.found(q)
	return hh.SearchPage{Found: found, Page: 0, Pages: 1, PerPage: q.PerPage}, nil
}

func TestPlanITSelectsOfficialRolesAndSplitsWithoutDateGaps(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(9 * time.Second)
	src := planningSource{
		roles: []hh.ProfessionalRole{{ID: "96", Name: "Developer"}, {ID: "10", Name: "Analyst"}},
		found: func(q hh.SearchQuery) int {
			if q.DateFrom.IsZero() {
				return 2001
			}
			return 1000
		},
	}
	plan, err := pipeline.PlanIT(context.Background(), src, pipeline.ITPlanOptions{
		Area: "113", Earliest: from, Now: to, MaxDepth: 4, MaxPartitions: 10, MaxRequests: 20,
	})
	require.NoError(t, err)
	require.Len(t, plan.Roles, 2)
	require.Len(t, plan.Partitions, 4)
	require.Equal(t, "10", plan.Partitions[0].RoleID)
	for i := 0; i < len(plan.Partitions); i += 2 {
		left, right := plan.Partitions[i], plan.Partitions[i+1]
		require.Equal(t, left.To.Add(time.Second), right.From)
		require.Equal(t, from, left.From)
		require.Equal(t, to, right.To)
	}
}

func TestPlanITKeepsPartitionAtCap(t *testing.T) {
	src := planningSource{
		roles: []hh.ProfessionalRole{{ID: "96", Name: "Developer"}},
		found: func(hh.SearchQuery) int { return hh.MaxSearchResults },
	}
	plan, err := pipeline.PlanIT(context.Background(), src, pipeline.ITPlanOptions{MaxRequests: 5})
	require.NoError(t, err)
	require.Len(t, plan.Partitions, 1)
	require.Equal(t, hh.MaxSearchResults, plan.EstimatedResults)
}

func TestPlanITFailsAtSafetyCeilings(t *testing.T) {
	src := planningSource{
		roles: []hh.ProfessionalRole{{ID: "96", Name: "Developer"}},
		found: func(hh.SearchQuery) int { return hh.MaxSearchResults + 1 },
	}
	_, err := pipeline.PlanIT(context.Background(), src, pipeline.ITPlanOptions{
		Earliest: time.Unix(0, 0), Now: time.Unix(10, 0), MaxDepth: 1, MaxPartitions: 10, MaxRequests: 10,
	})
	require.ErrorContains(t, err, "max depth")

	_, err = pipeline.PlanIT(context.Background(), src, pipeline.ITPlanOptions{
		Earliest: time.Unix(0, 0), Now: time.Unix(10, 0), MaxDepth: 10, MaxPartitions: 10, MaxRequests: 1,
	})
	require.ErrorContains(t, err, "request safety ceiling")
}
