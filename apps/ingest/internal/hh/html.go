package hh

import (
	"regexp"
	"strings"
)

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	reTags        = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces      = regexp.MustCompile(`\s+`)
)

// StripHTML removes script/style blocks and tags; collapses whitespace.
// Contact/PII redaction is out of scope; do not log full descriptions.
func StripHTML(s string) string {
	if s == "" {
		return ""
	}
	s = reScriptStyle.ReplaceAllString(s, " ")
	s = reTags.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
