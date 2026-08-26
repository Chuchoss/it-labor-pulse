package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ContentHash computes sha256 hex of the canonical subset used for dedup.
// Includes: title, salary_*, area (region external id), employer_id, skills set, published_at.
// Does not include collected_at. Skills are sorted for order-independence.
func ContentHash(v CanonicalVacancy) string {
	skills := make([]string, 0, len(v.Skills))
	for _, s := range v.Skills {
		id := s.SkillID
		if id == "" {
			id = s.RawName
		}
		skills = append(skills, strings.ToLower(strings.TrimSpace(id)))
	}
	sort.Strings(skills)

	var b strings.Builder
	b.WriteString(v.Title)
	b.WriteByte('|')
	b.WriteString(v.SourceURL)
	b.WriteByte('|')
	writeFloatPtr(&b, v.SalaryFrom)
	b.WriteByte('|')
	writeFloatPtr(&b, v.SalaryTo)
	b.WriteByte('|')
	b.WriteString(v.SalaryCurrency)
	b.WriteByte('|')
	writeBoolPtr(&b, v.SalaryGross)
	b.WriteByte('|')
	writeFloatPtr(&b, v.SalaryMid)
	b.WriteByte('|')
	writeFloatPtr(&b, v.SalaryMidRub)
	b.WriteByte('|')
	if v.SalaryRateDate != nil {
		b.WriteString(v.SalaryRateDate.UTC().Format(time.DateOnly))
	}
	b.WriteByte('|')
	b.WriteString(v.SalaryRateProvider)
	b.WriteByte('|')
	b.WriteString(v.RegionExternalID)
	b.WriteByte('|')
	b.WriteString(v.EmployerExternalID)
	b.WriteByte('|')
	b.WriteString(strings.Join(skills, ","))
	b.WriteByte('|')
	if !v.PublishedAt.IsZero() {
		b.WriteString(v.PublishedAt.UTC().Format(time.RFC3339))
	}

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func writeFloatPtr(b *strings.Builder, p *float64) {
	if p == nil {
		b.WriteString("null")
		return
	}
	b.WriteString(fmt.Sprintf("%.4f", *p))
}

func writeBoolPtr(b *strings.Builder, p *bool) {
	if p == nil {
		b.WriteString("null")
		return
	}
	if *p {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
}
