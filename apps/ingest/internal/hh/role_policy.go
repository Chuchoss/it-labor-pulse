package hh

import (
	"fmt"
	"sort"
)

const (
	RoleGroupSoftwareDevelopment = "software_development"
	RoleGroupAnalytics           = "analytics"
	RoleGroupQualityAssurance    = "quality_assurance"
	RoleGroupITManagement        = "it_management"
	ScopeVacancyListing          = "vacancy_listing"
	ScopeManagementAnalytics     = "management_analytics"
)

// AllowedRole describes one official HH professional role admitted to the
// Phase 1 vacancy product. IDs and expected names are validated against the
// live catalog before planning.
type AllowedRole struct {
	ID           string
	ExpectedName string
	Group        string
	Priority     int
	Scopes       []string
}

var allowedRoles = map[string]AllowedRole{
	"96":  {ID: "96", ExpectedName: "Программист, разработчик", Group: RoleGroupSoftwareDevelopment, Priority: 10, Scopes: []string{ScopeVacancyListing}},
	"104": {ID: "104", ExpectedName: "Руководитель группы разработки", Group: RoleGroupSoftwareDevelopment, Priority: 20, Scopes: []string{ScopeVacancyListing, ScopeManagementAnalytics}},
	"148": {ID: "148", ExpectedName: "Системный аналитик", Group: RoleGroupAnalytics, Priority: 30, Scopes: []string{ScopeVacancyListing, ScopeManagementAnalytics}},
	"150": {ID: "150", ExpectedName: "Бизнес-аналитик", Group: RoleGroupAnalytics, Priority: 40, Scopes: []string{ScopeVacancyListing, ScopeManagementAnalytics}},
	"156": {ID: "156", ExpectedName: "BI-аналитик, аналитик данных", Group: RoleGroupAnalytics, Priority: 50, Scopes: []string{ScopeVacancyListing, ScopeManagementAnalytics}},
	"164": {ID: "164", ExpectedName: "Продуктовый аналитик", Group: RoleGroupAnalytics, Priority: 60, Scopes: []string{ScopeVacancyListing, ScopeManagementAnalytics}},
	"124": {ID: "124", ExpectedName: "Тестировщик", Group: RoleGroupQualityAssurance, Priority: 70, Scopes: []string{ScopeVacancyListing}},
}

var managementOnlyRoles = map[string]AllowedRole{
	"10":  {ID: "10", ExpectedName: "Аналитик", Group: RoleGroupITManagement, Priority: 100, Scopes: []string{ScopeManagementAnalytics}},
	"36":  {ID: "36", ExpectedName: "Директор по информационным технологиям (CIO)", Group: RoleGroupITManagement, Priority: 110, Scopes: []string{ScopeManagementAnalytics}},
	"73":  {ID: "73", ExpectedName: "Менеджер продукта", Group: RoleGroupITManagement, Priority: 120, Scopes: []string{ScopeManagementAnalytics}},
	"107": {ID: "107", ExpectedName: "Руководитель проектов", Group: RoleGroupITManagement, Priority: 130, Scopes: []string{ScopeManagementAnalytics}},
	"125": {ID: "125", ExpectedName: "Технический директор (CTO)", Group: RoleGroupITManagement, Priority: 140, Scopes: []string{ScopeManagementAnalytics}},
	"157": {ID: "157", ExpectedName: "Руководитель отдела аналитики", Group: RoleGroupITManagement, Priority: 150, Scopes: []string{ScopeManagementAnalytics}},
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

// CollectedRoles is the bounded union of listing and management analytics roles.
func CollectedRoles() []AllowedRole {
	result := AllowedRoles()
	for _, role := range managementOnlyRoles {
		result = append(result, role)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Priority < result[j].Priority })
	return result
}

// CollectedRole looks up one approved official role without title heuristics.
func CollectedRole(id string) (AllowedRole, bool) {
	for _, role := range CollectedRoles() {
		if role.ID == id {
			return role, true
		}
	}
	return AllowedRole{}, false
}

// FilterAllowedRoles validates the official catalog names and returns only
// roles admitted by policy. A rename is a deliberate review event, not a
// reason to silently broaden or empty the crawl.
func FilterAllowedRoles(catalog []ProfessionalRole) ([]ProfessionalRole, error) {
	return filterRoles(catalog, AllowedRoles())
}

// FilterCollectedRoles validates every official role used by the bounded planner.
func FilterCollectedRoles(catalog []ProfessionalRole) ([]ProfessionalRole, error) {
	return filterRoles(catalog, CollectedRoles())
}

func filterRoles(catalog []ProfessionalRole, policies []AllowedRole) ([]ProfessionalRole, error) {
	byID := make(map[string]ProfessionalRole, len(catalog))
	for _, role := range catalog {
		byID[role.ID] = role
	}
	result := make([]ProfessionalRole, 0, len(policies))
	for _, policy := range policies {
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

// CollectedProfessionalRoles returns every approved official role attached to
// a vacancy, deduplicated and ordered. Title matching is deliberately absent.
func CollectedProfessionalRoles(ids []string) []AllowedRole {
	policy := make(map[string]AllowedRole, len(allowedRoles)+len(managementOnlyRoles))
	for id, role := range allowedRoles {
		policy[id] = role
	}
	for id, role := range managementOnlyRoles {
		policy[id] = role
	}
	seen := map[string]struct{}{}
	result := make([]AllowedRole, 0, len(ids))
	for _, id := range ids {
		role, ok := policy[id]
		if !ok {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, role)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Priority < result[j].Priority })
	return result
}
