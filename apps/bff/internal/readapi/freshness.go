package readapi

import "time"

const FreshnessWindow = 24 * time.Hour

// IsFresh reports whether a vacancy was first observed by ingest in the
// inclusive rolling UTC window ending at the server-provided now. Null and
// future timestamps are stale.
func IsFresh(firstObservedAt *time.Time, now time.Time) bool {
	if firstObservedAt == nil {
		return false
	}
	observed := firstObservedAt.UTC()
	now = now.UTC()
	return !observed.After(now) && !observed.Before(now.Add(-FreshnessWindow))
}

func VacancyStatus(isActive bool, inactiveReason string) string {
	if isActive {
		return "active"
	}
	if inactiveReason == "missing_from_complete_cycle" {
		return "missing_from_last_complete_cycle"
	}
	return "inactive"
}
