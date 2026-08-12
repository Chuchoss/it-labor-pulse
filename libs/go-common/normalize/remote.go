package normalize

import (
	"strings"
)

// DetectRemote applies rules from docs/architecture/15-normalization-rules.md.
// Strong signals alone → true. Weak keyword signals need a second signal.
func DetectRemote(scheduleID string, workFormatIDs []string, regionName, title, description string) bool {
	strong := 0
	weak := 0

	if isRemoteSchedule(scheduleID) {
		strong++
	}
	for _, id := range workFormatIDs {
		if isRemoteWorkFormat(id) {
			strong++
			break
		}
	}
	if isRemoteRegionName(regionName) {
		strong++
	}

	if hasRemoteKeyword(title) {
		weak++
	}
	if hasRemoteKeyword(description) {
		weak++
	}

	if strong > 0 {
		return true
	}
	// weak alone is not enough; need a second signal
	return weak >= 2
}

func hasRemoteKeyword(s string) bool {
	text := strings.ToLower(s)
	text = strings.ReplaceAll(text, "ё", "е")
	return strings.Contains(text, "удаленн") || containsWord(text, "remote")
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

func isRemoteRegionName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, "ё", "е")
	switch {
	case n == "удаленно", n == "удаленная", n == "remote", n == "remote work":
		return true
	case strings.Contains(n, "удален"):
		return true
	default:
		return false
	}
}

func containsWord(text, word string) bool {
	word = strings.ToLower(word)
	if word == "" {
		return false
	}
	// crude word boundary for Latin remote
	idx := strings.Index(text, word)
	for idx >= 0 {
		beforeOK := idx == 0 || !isWordChar(rune(text[idx-1]))
		after := idx + len(word)
		afterOK := after >= len(text) || !isWordChar(rune(text[after]))
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(text[idx+1:], word)
		if next < 0 {
			return false
		}
		idx = idx + 1 + next
	}
	return false
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}
