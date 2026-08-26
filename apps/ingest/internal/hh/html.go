package hh

import (
	"html"
	"regexp"
	"strings"
)

const MaxDescriptionRunes = 12000

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	reBreaks      = regexp.MustCompile(`(?i)</?(?:br|p|div|li|h[1-6])(?:\s[^>]*)?>`)
	reTags        = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces      = regexp.MustCompile(`\s+`)
)

// SanitizeDescription converts untrusted HH HTML to bounded plain text.
// Contact/PII redaction is out of scope; do not log full descriptions.
func SanitizeDescription(s string, maxRunes int) (string, bool) {
	if s == "" {
		return "", false
	}
	s = reScriptStyle.ReplaceAllString(s, " ")
	s = reBreaks.ReplaceAllString(s, "\n")
	s = reTags.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\x00", " ")
	s = reSpaces.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if maxRunes > 0 && len(runes) > maxRunes {
		return strings.TrimSpace(string(runes[:maxRunes])), true
	}
	return s, false
}

// StripHTML preserves the adapter's legacy API while applying the canonical bound.
func StripHTML(s string) string {
	text, _ := SanitizeDescription(s, MaxDescriptionRunes)
	return text
}
