package hh

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeDescriptionRemovesUnsafeHTMLAndDecodesEntities(t *testing.T) {
	got, truncated := SanitizeDescription(
		`<style>.x{}</style><p>Go&nbsp;&amp; PostgreSQL</p><script>alert(1)</script><div>удалённо</div>`,
		100,
	)
	if truncated || got != "Go & PostgreSQL удалённо" {
		t.Fatalf("got %q truncated=%v", got, truncated)
	}
}

func TestSanitizeDescriptionTruncatesOnRuneBoundary(t *testing.T) {
	got, truncated := SanitizeDescription(strings.Repeat("я", 10), 7)
	if !truncated || utf8.RuneCountInString(got) != 7 || !utf8.ValidString(got) {
		t.Fatalf("got %q truncated=%v", got, truncated)
	}
}
