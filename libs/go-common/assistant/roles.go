package assistant

import (
	"fmt"
	"sort"
	"strings"
)

// ApprovedRole is an official HH professional role admitted to assistant
// matching. ID is the stable HH catalog ID, not a database UUID or title.
type ApprovedRole struct {
	ID    string
	Label string
	Group string
}

var approvedRolePolicy = map[string]ApprovedRole{
	"96":  {ID: "96", Label: "Программист, разработчик", Group: "software_development"},
	"104": {ID: "104", Label: "Руководитель группы разработки", Group: "software_development"},
	"148": {ID: "148", Label: "Системный аналитик", Group: "analytics"},
	"150": {ID: "150", Label: "Бизнес-аналитик", Group: "analytics"},
	"156": {ID: "156", Label: "BI-аналитик, аналитик данных", Group: "analytics"},
	"164": {ID: "164", Label: "Продуктовый аналитик", Group: "analytics"},
	"124": {ID: "124", Label: "Тестировщик", Group: "quality_assurance"},
}

var legacyRoleAliases = map[string][]string{
	"backend":             {"96"},
	"backend developer":   {"96"},
	"frontend":            {"96"},
	"frontend developer":  {"96"},
	"fullstack":           {"96"},
	"full stack":          {"96"},
	"fullstack developer": {"96"},
	"developer":           {"96"},
	"programmer":          {"96"},
	"software developer":  {"96"},
	"team lead":           {"104"},
	"teamlead":            {"104"},
	"lead developer":      {"104"},
	"qa":                  {"124"},
	"qa engineer":         {"124"},
	"tester":              {"124"},
	"quality assurance":   {"124"},
	"system analyst":      {"148"},
	"systems analyst":     {"148"},
	"business analyst":    {"150"},
	"bi analyst":          {"156"},
	"data analyst":        {"156"},
	"product analyst":     {"164"},
}

// ApprovedRoles returns the stable official assistant role policy.
func ApprovedRoles() []ApprovedRole {
	order := []string{"96", "104", "148", "150", "156", "164", "124"}
	result := make([]ApprovedRole, 0, len(order))
	for _, id := range order {
		result = append(result, approvedRolePolicy[id])
	}
	return result
}

// NormalizeLegacyRole converts only explicitly recognized legacy aliases.
func NormalizeLegacyRole(value string) ([]string, error) {
	alias := strings.ToLower(strings.TrimSpace(value))
	alias = strings.NewReplacer("_", " ", "-", " ").Replace(alias)
	alias = strings.Join(strings.Fields(alias), " ")
	roles, ok := legacyRoleAliases[alias]
	if !ok {
		return nil, fmt.Errorf("%w: hard_criteria.role %q is unknown; choose approved_roles", ErrInvalidPreferences, value)
	}
	return append([]string(nil), roles...), nil
}

// NormalizePreferenceRoles returns a copy suitable for matching or writing.
// It never mutates the supplied immutable preference version.
func NormalizePreferenceRoles(p PreferenceRecord) (PreferenceRecord, bool, error) {
	normalized := p
	normalized.HardCriteria = cloneCriteria(p.HardCriteria)
	legacy, exists := normalized.HardCriteria["role"]
	if !exists {
		return normalized, false, nil
	}
	value, ok := legacy.(string)
	if !ok {
		return PreferenceRecord{}, false, fmt.Errorf("%w: hard_criteria.role must be a recognized string alias", ErrInvalidPreferences)
	}
	mapped, err := NormalizeLegacyRole(value)
	if err != nil {
		return PreferenceRecord{}, false, err
	}
	current, err := approvedRoleIDs(normalized.HardCriteria["approved_roles"])
	if err != nil {
		return PreferenceRecord{}, false, err
	}
	normalized.HardCriteria["approved_roles"] = stringValues(mergeRoleIDs(current, mapped))
	delete(normalized.HardCriteria, "role")
	normalized.LegacyRoleUpgraded = true
	return normalized, true, nil
}

func validateApprovedRoles(value any) error {
	ids, err := approvedRoleIDs(value)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := approvedRolePolicy[id]; !ok {
			return fmt.Errorf("%w: hard_criteria.approved_roles contains unsupported role %q", ErrInvalidPreferences, id)
		}
	}
	return nil
}

func approvedRoleIDs(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	var values []string
	switch items := value.(type) {
	case []any:
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: hard_criteria.approved_roles must contain strings", ErrInvalidPreferences)
			}
			values = append(values, strings.TrimSpace(text))
		}
	case []string:
		for _, item := range items {
			values = append(values, strings.TrimSpace(item))
		}
	default:
		return nil, fmt.Errorf("%w: hard_criteria.approved_roles must be an array", ErrInvalidPreferences)
	}
	return mergeRoleIDs(nil, values), nil
}

func mergeRoleIDs(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, id := range append(append([]string(nil), left...), right...) {
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func stringValues(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

func cloneCriteria(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
