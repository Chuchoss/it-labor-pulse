package readapi

import "time"

const FreshnessWindow = 24 * time.Hour

// IsFresh reports whether a vacancy was published on its source in the
// inclusive rolling UTC window ending at the server-provided now. Null and
// future timestamps are stale.
func IsFresh(publishedAt *time.Time, now time.Time) bool {
	if publishedAt == nil {
		return false
	}
	published := publishedAt.UTC()
	now = now.UTC()
	return !published.After(now) && !published.Before(now.Add(-FreshnessWindow))
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
