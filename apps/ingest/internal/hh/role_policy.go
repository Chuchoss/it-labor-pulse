package hh

import (
	"fmt"
	"sort"
)

const (
	RoleGroupSoftwareDevelopment = "software_development"
	RoleGroupAnalytics           = "analytics"
	RoleGroupQualityAssurance    = "quality_assurance"
)

// AllowedRole describes one official HH professional role admitted to the
// Phase 1 vacancy product. IDs and expected names are validated against the
// live catalog before planning.
type AllowedRole struct {
	ID           string
	ExpectedName string
	Group        string
	Priority     int
}

var allowedRoles = map[string]AllowedRole{
	"96":  {ID: "96", ExpectedName: "Программист, разработчик", Group: RoleGroupSoftwareDevelopment, Priority: 10},
	"104": {ID: "104", ExpectedName: "Руководитель группы разработки", Group: RoleGroupSoftwareDevelopment, Priority: 20},
	"148": {ID: "148", ExpectedName: "Системный аналитик", Group: RoleGroupAnalytics, Priority: 30},
	"150": {ID: "150", ExpectedName: "Бизнес-аналитик", Group: RoleGroupAnalytics, Priority: 40},
	"156": {ID: "156", ExpectedName: "BI-аналитик, аналитик данных", Group: RoleGroupAnalytics, Priority: 50},
	"164": {ID: "164", ExpectedName: "Продуктовый аналитик", Group: RoleGroupAnalytics, Priority: 60},
	"124": {ID: "124", ExpectedName: "Тестировщик", Group: RoleGroupQualityAssurance, Priority: 70},
}

// AllowedRoles returns a stable, priority-ordered copy of the Phase 1 policy.
func AllowedRoles() []AllowedRole {
	result := make([]AllowedRole, 0, len(allowedRoles))
	for _, role := range allowedRoles {
		result = append(result, role)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Priority < result[j].Priority })
	return result
}

// FilterAllowedRoles validates the official catalog names and returns only
// roles admitted by policy. A rename is a deliberate review event, not a
// reason to silently broaden or empty the crawl.
func FilterAllowedRoles(catalog []ProfessionalRole) ([]ProfessionalRole, error) {
	byID := make(map[string]ProfessionalRole, len(catalog))
	for _, role := range catalog {
		byID[role.ID] = role
	}
	result := make([]ProfessionalRole, 0, len(allowedRoles))
	for _, policy := range AllowedRoles() {
		role, ok := byID[policy.ID]
		if !ok {
			return nil, fmt.Errorf("hh role policy: allowed role %s missing from official IT catalog", policy.ID)
		}
		if role.Name != policy.ExpectedName {
			return nil, fmt.Errorf("hh role policy: role %s catalog name changed", policy.ID)
		}
		if role.SearchDeprecated {
			return nil, fmt.Errorf("hh role policy: role %s is search-deprecated", policy.ID)
		}
		result = append(result, role)
	}
	return result, nil
}

// AllowedProfessionalRole selects the highest-priority allowed role from a
// vacancy's official HH role IDs. Missing and out-of-scope mappings are false.
func AllowedProfessionalRole(ids []string) (AllowedRole, bool) {
	var selected AllowedRole
	found := false
	for _, id := range ids {
		candidate, ok := allowedRoles[id]
		if ok && (!found || candidate.Priority < selected.Priority) {
			selected, found = candidate, true
		}
	}
	return selected, found
}
