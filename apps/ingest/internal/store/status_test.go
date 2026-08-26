package store

import (
	"context"
	"testing"
	"time"

	"github.com/Chuchoss/it-labor-pulse/libs/go-common/normalize"
)

func TestMemoryDiscoveryStatusReconciliation(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	memory.Vacancies["hh|seen"] = VacancyWrite{
		Vacancy: normalize.CanonicalVacancy{Source: "hh", ExternalID: "seen", IsActive: false},
	}
	memory.Vacancies["hh|missing"] = VacancyWrite{
		Vacancy: normalize.CanonicalVacancy{Source: "hh", ExternalID: "missing", IsActive: true},
	}
	cycleID := "cycle"
	memory.Cycles[cycleID] = Cycle{
		ID: cycleID, Source: "hh", Scope: "daily_discovery", Status: "complete",
	}
	memory.Observations[cycleID+"|seen"] = DiscoveryObservation{
		ExternalID: "seen", ObservedAt: time.Date(2026, 8, 26, 1, 0, 0, 0, time.UTC),
	}

	result, err := memory.ReconcileDiscoveryStatuses(ctx, cycleID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reactivated != 1 || result.Deactivated != 1 {
		t.Fatalf("reconciliation = %+v, want one reactivation and deactivation", result)
	}
	if memory.Vacancies["hh|seen"].Vacancy.IsActive {
		// expected: the previously inactive vacancy is reactivated.
	} else {
		t.Fatal("seen vacancy was not reactivated")
	}
	if memory.Vacancies["hh|missing"].Vacancy.IsActive {
		t.Fatal("missing vacancy remained active")
	}
}

func TestMemoryPartialDiscoveryDoesNotReconcile(t *testing.T) {
	memory := NewMemory()
	memory.Vacancies["hh|old"] = VacancyWrite{
		Vacancy: normalize.CanonicalVacancy{Source: "hh", ExternalID: "old", IsActive: true},
	}
	memory.Cycles["partial"] = Cycle{
		ID: "partial", Source: "hh", Scope: "daily_discovery", Status: "running",
	}
	if _, err := memory.ReconcileDiscoveryStatuses(context.Background(), "partial"); err == nil {
		t.Fatal("partial cycle unexpectedly reconciled")
	}
	if !memory.Vacancies["hh|old"].Vacancy.IsActive {
		t.Fatal("partial cycle changed vacancy status")
	}
}
