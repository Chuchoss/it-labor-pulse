package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/hh"
	"github.com/Chuchoss/it-labor-pulse/apps/ingest/internal/store"
)

type discoverySource struct {
	searchCalls int
}

func (s *discoverySource) ProfessionalRoles(
	_ context.Context,
	_ string,
) ([]hh.ProfessionalRole, error) {
	roles := hh.CollectedRoles()
	out := make([]hh.ProfessionalRole, 0, len(roles))
	for _, role := range roles {
		out = append(out, hh.ProfessionalRole{ID: role.ID, Name: role.ExpectedName})
	}
	return out, nil
}

func (s *discoverySource) SearchVacancies(
	_ context.Context,
	query hh.SearchQuery,
) (hh.SearchPage, error) {
	s.searchCalls++
	if query.PerPage == 1 {
		return hh.SearchPage{Found: 1, Page: 0, Pages: 1, PerPage: 1}, nil
	}
	raw := []byte(fmt.Sprintf(`{
		"found":1,"page":%d,"pages":1,"per_page":100,
		"items":[{
			"id":"same-id","name":"Example role",
			"published_at":"2026-08-25T12:00:00+0300",
			"area":{"id":"1","name":"Москва"},
			"professional_roles":[
				{"id":"148","name":"Системный аналитик"},
				{"id":"96","name":"Программист, разработчик"}
			],
			"salary":{"from":100000,"to":120000,"currency":"RUR","gross":false}
		}]
	}`, query.Page))
	return hh.ParseSearchPage(raw)
}

func TestDailyDiscoveryDeduplicatesAndUsesItemDimensions(t *testing.T) {
	source := &discoverySource{}
	memory := store.NewMemory()
	cutoff := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	result, err := RunDailyDiscovery(
		context.Background(), source, memory, nil,
		DiscoveryOptions{
			Area: "113", PerPage: 100, MaxDepth: 4, MaxPartitions: 100,
			MaxRequests: 100, CutoffAt: cutoff,
			CycleDate: cutoff.AddDate(0, 0, -1), ObservedAt: cutoff.Add(time.Hour),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatal("cycle must be complete")
	}
	if got := len(memory.Observations); got != 1 {
		t.Fatalf("deduplicated observations = %d, want 1", got)
	}
	var observation store.DiscoveryObservation
	for _, value := range memory.Observations {
		observation = value
	}
	if observation.ExternalRegionID != "1" {
		t.Fatalf("region = %q, want per-item area 1", observation.ExternalRegionID)
	}
	if observation.PrimaryRoleExternalID != "96" ||
		observation.RoleGroup != hh.RoleGroupSoftwareDevelopment {
		t.Fatalf("primary role = %s/%s", observation.PrimaryRoleExternalID, observation.RoleGroup)
	}
	if !observation.SalaryEligible || observation.SalaryMidRubNet == nil ||
		*observation.SalaryMidRubNet != 110000 {
		t.Fatalf("salary normalization = %#v", observation)
	}
	second, err := RunDailyDiscovery(
		context.Background(), source, memory, nil,
		DiscoveryOptions{
			Area: "113", PerPage: 100, MaxDepth: 4, MaxPartitions: 100,
			MaxRequests: 100, CutoffAt: cutoff,
			CycleDate: cutoff.AddDate(0, 0, -1), ObservedAt: cutoff.Add(2 * time.Hour),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.CycleID != result.CycleID || len(memory.Observations) != 1 {
		t.Fatalf("idempotent rerun = cycle %s, observations %d", second.CycleID, len(memory.Observations))
	}
}
