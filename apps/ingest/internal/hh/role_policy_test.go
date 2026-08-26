package hh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllowedRolePolicyMatchesOfficialCatalogFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "hh", "professional_roles_it.json"))
	require.NoError(t, err)
	var catalog professionalRoleCatalog
	require.NoError(t, json.Unmarshal(raw, &catalog))
	require.Len(t, catalog.Categories, 1)

	roles, err := FilterAllowedRoles(catalog.Categories[0].Roles)
	require.NoError(t, err)
	require.Equal(t, []string{"96", "104", "148", "150", "156", "164", "124"}, roleIDs(roles))
	collected, err := FilterCollectedRoles(catalog.Categories[0].Roles)
	require.NoError(t, err)
	require.Equal(
		t,
		[]string{"96", "104", "148", "150", "156", "164", "124", "10", "36", "73", "107", "125", "157"},
		roleIDs(collected),
	)

	groups := map[string]string{}
	for _, role := range AllowedRoles() {
		groups[role.ID] = role.Group
	}
	require.Equal(t, RoleGroupSoftwareDevelopment, groups["96"])
	require.Equal(t, RoleGroupSoftwareDevelopment, groups["104"])
	require.Equal(t, RoleGroupAnalytics, groups["148"])
	require.Equal(t, RoleGroupAnalytics, groups["150"])
	require.Equal(t, RoleGroupAnalytics, groups["156"])
	require.Equal(t, RoleGroupAnalytics, groups["164"])
	require.Equal(t, RoleGroupQualityAssurance, groups["124"])
}

func TestAllowedProfessionalRoleIsConservativeAndDeterministic(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want string
		ok   bool
	}{
		{name: "developer", ids: []string{"96"}, want: "96", ok: true},
		{name: "lead_and_developer_prefers_developer", ids: []string{"104", "96"}, want: "96", ok: true},
		{name: "qa", ids: []string{"124"}, want: "124", ok: true},
		{name: "missing", ids: nil},
		{name: "sales_content_project_excluded", ids: []string{"70", "3", "107"}},
		{name: "generic_analyst_excluded", ids: []string{"10"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := AllowedProfessionalRole(tt.ids)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got.ID)
		})
	}
}

func TestManagementScopeUsesOnlyApprovedOfficialRoles(t *testing.T) {
	roles := CollectedProfessionalRoles([]string{"70", "3", "107", "73", "157", "107"})
	require.Equal(t, []string{"73", "107", "157"}, allowedRoleIDs(roles))

	listing, ok := AllowedProfessionalRole([]string{"73", "107"})
	require.False(t, ok)
	require.Empty(t, listing.ID)

	overlap := CollectedProfessionalRoles([]string{"104", "148"})
	require.Equal(t, []string{"104", "148"}, allowedRoleIDs(overlap))
}

func roleIDs(roles []ProfessionalRole) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		result = append(result, role.ID)
	}
	return result
}

func allowedRoleIDs(roles []AllowedRole) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		result = append(result, role.ID)
	}
	return result
}
