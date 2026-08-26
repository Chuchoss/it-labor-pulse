package readapi

import (
	"testing"
	"time"
)

func TestIsFresh(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   *time.Time
		want bool
	}{
		{name: "inside", at: timePointer(now.Add(-4 * time.Hour)), want: true},
		{name: "exact boundary", at: timePointer(now.Add(-FreshnessWindow)), want: true},
		{name: "outside", at: timePointer(now.Add(-FreshnessWindow - time.Second)), want: false},
		{name: "now", at: timePointer(now), want: true},
		{name: "future clock skew", at: timePointer(now.Add(time.Second)), want: false},
		{name: "timezone offset", at: timePointer(now.Add(-4 * time.Hour).In(time.FixedZone("MSK", 3*60*60))), want: true},
		{name: "missing", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsFresh(test.at, now); got != test.want {
				t.Fatalf("IsFresh() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestVacancyStatus(t *testing.T) {
	tests := []struct {
		name, reason, want string
		active             bool
	}{
		{name: "active vacancy", active: true, want: "active"},
		{name: "missing from complete cycle", reason: "missing_from_complete_cycle", want: "missing_from_last_complete_cycle"},
		{name: "detail inactive", reason: "detail_not_found", want: "inactive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := VacancyStatus(test.active, test.reason); got != test.want {
				t.Fatalf("VacancyStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
