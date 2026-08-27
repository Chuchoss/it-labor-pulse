package normalize

import (
	"strings"
)

// DetectRemote uses only official source work-format fields. A nil result means
// that neither schedule nor work_format supplied a fact; text is not guessed.
func DetectRemote(scheduleID string, workFormatIDs []string, _, _, _ string) *bool {
	if isRemoteSchedule(scheduleID) {
		return boolPointer(true)
	}
	for _, id := range workFormatIDs {
		if isRemoteWorkFormat(id) {
			return boolPointer(true)
		}
	}
	if strings.TrimSpace(scheduleID) != "" || len(workFormatIDs) > 0 {
		return boolPointer(false)
	}
	return nil
}

func boolPointer(value bool) *bool {
	return &value
}

func isRemoteSchedule(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "remote", "remote_work", "удаленная", "удалённая":
		return true
	default:
		return false
	}
}

func isRemoteWorkFormat(id string) bool {
	switch strings.ToUpper(strings.TrimSpace(id)) {
	case "REMOTE", "REMOTE_WORK":
		return true
	default:
		return false
	}
}
