package checkpoint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// ScopeHash returns a stable SHA-256 hex of normalized ingest params.
func ScopeHash(source, mode, area, text string, perPage int, extra ...string) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(source)),
		strings.ToLower(strings.TrimSpace(mode)),
		strings.TrimSpace(area),
		strings.ToLower(strings.TrimSpace(text)),
		strconv.Itoa(perPage),
	}
	for _, value := range extra {
		parts = append(parts, strings.ToLower(strings.TrimSpace(value)))
	}
	norm := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// ParseCursorPage parses checkpoint cursor as next page index (default 0).
func ParseCursorPage(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(cursor)
	if err != nil {
		return 0, fmt.Errorf("checkpoint cursor: %w", err)
	}
	if n < 0 {
		return 0, fmt.Errorf("checkpoint cursor: negative page")
	}
	return n, nil
}

// FormatCursor encodes the next page index.
func FormatCursor(page int) string {
	return strconv.Itoa(page)
}

// PageOutcome decides whether the checkpoint may advance after a page.
type PageOutcome struct {
	// AllOK is true when every draft on the page normalized and upserted.
	AllOK bool
	// CurrentPage is the page that was just processed.
	CurrentPage int
	// TotalPages is HH "pages" field (exclusive upper bound of page index).
	TotalPages int
	// ItemCount is number of items on the page.
	ItemCount int
}

// Decision is the pure checkpoint decision after a page attempt.
type Decision struct {
	Advance        bool
	NextCursor     string
	Terminal       bool
	TerminalReason string
}

// Decide returns whether to advance the checkpoint and the next cursor.
// On failure, Advance=false and cursor must not be written.
func Decide(o PageOutcome) Decision {
	if !o.AllOK {
		return Decision{Advance: false}
	}
	if o.ItemCount == 0 {
		return Decision{
			Advance:        true,
			NextCursor:     FormatCursor(0),
			Terminal:       true,
			TerminalReason: "empty_page",
		}
	}
	next := o.CurrentPage + 1
	if o.TotalPages > 0 && next >= o.TotalPages {
		return Decision{
			Advance:        true,
			NextCursor:     FormatCursor(0),
			Terminal:       true,
			TerminalReason: "last_page",
		}
	}
	return Decision{
		Advance:    true,
		NextCursor: FormatCursor(next),
		Terminal:   false,
	}
}
