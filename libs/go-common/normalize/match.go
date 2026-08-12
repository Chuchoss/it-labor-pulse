package normalize

import (
	"regexp"
	"strings"
	"unicode"
)

// MapRoleMatcher matches by external role id first, then normalized title contains/equals.
// RoleByExternalID: source → externalRoleID → roleID
// TitlePatterns: ordered rules; first match wins (priority).
type MapRoleMatcher struct {
	RoleByExternalID map[string]map[string]string
	TitlePatterns    []TitleRoleRule
}

// TitleRoleRule is a title → role_id rule (equals or contains on normalized title).
type TitleRoleRule struct {
	Pattern  string
	RoleID   string
	Contains bool
}

// MatchRole implements RoleMatcher.
func (m MapRoleMatcher) MatchRole(source string, professionalRoleIDs []string, title string) (string, bool) {
	if m.RoleByExternalID != nil {
		if byID := m.RoleByExternalID[source]; byID != nil {
			for _, id := range professionalRoleIDs {
				if roleID, ok := byID[id]; ok && roleID != "" {
					return roleID, true
				}
			}
		}
	}
	norm := NormalizeTitle(title)
	for _, rule := range m.TitlePatterns {
		p := NormalizeTitle(rule.Pattern)
		if p == "" {
			continue
		}
		if rule.Contains {
			if strings.Contains(norm, p) {
				return rule.RoleID, true
			}
			continue
		}
		if norm == p {
			return rule.RoleID, true
		}
	}
	return "", false
}

// MapRegionMatcher maps source → regionExternalID → regionID.
type MapRegionMatcher map[string]map[string]string

// MatchRegion implements RegionMatcher.
func (m MapRegionMatcher) MatchRegion(source, regionExternalID string) (string, bool) {
	if m == nil {
		return "", false
	}
	byID := m[source]
	if byID == nil {
		return "", false
	}
	id, ok := byID[regionExternalID]
	return id, ok && id != ""
}

// SlugSkillMatcher MVP stub: known aliases or slugify + mark isNew.
type SlugSkillMatcher struct {
	Aliases map[string]string // normalized raw → skill_id
}

// MatchSkill implements SkillMatcher.
func (m SlugSkillMatcher) MatchSkill(_, rawName string) (skillID string, isNew bool) {
	key := NormalizeSkillName(rawName)
	if key == "" {
		return "", false
	}
	if m.Aliases != nil {
		if id, ok := m.Aliases[key]; ok && id != "" {
			return id, false
		}
	}
	return key, true
}

// NormalizeTitle lowercases, ё→е, trims punctuation/spaces.
func NormalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

var nonSkill = regexp.MustCompile(`[^a-z0-9а-я+#.]+`)

// NormalizeSkillName produces a stable slug-like key for alias lookup / upsert.
func NormalizeSkillName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	s = nonSkill.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func matchSkills(matcher SkillMatcher, source string, raw []string) []SkillRef {
	if matcher == nil {
		matcher = SlugSkillMatcher{}
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]SkillRef, 0, len(raw))
	for _, name := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id, isNew := matcher.MatchSkill(source, name)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, SkillRef{SkillID: id, RawName: name, IsNew: isNew})
	}
	return out
}
