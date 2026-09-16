package glance

import "testing"

func newAuthorizationPolicyTestTopology() (
	[]*dashboard,
	map[string]*dashboard,
	map[string]*page,
) {
	pages := map[string]*page{
		"home":       {Slug: "home"},
		"sports":     {Slug: "sports"},
		"monitoring": {Slug: "monitoring"},
		"docker":     {Slug: "docker"},
		"orphan":     {Slug: "orphan"},
	}

	dashboardsByName := map[string]*dashboard{
		"Default": {
			Name:  "Default",
			Pages: []*page{pages["home"], pages["sports"]},
		},
		"Admin": {
			Name:  "Admin",
			Slug:  "admin",
			Pages: []*page{pages["monitoring"], pages["docker"], pages["sports"]},
		},
		"Personal": {
			Name:  "Personal",
			Slug:  "personal",
			Pages: []*page{pages["home"]},
		},
	}

	dashboards := []*dashboard{
		dashboardsByName["Default"],
		dashboardsByName["Admin"],
		dashboardsByName["Personal"],
	}

	return dashboards, dashboardsByName, pages
}

func TestAuthorizationPolicyDisabledPreservesLegacyAccess(t *testing.T) {
	dashboards, dashboardsByName, pages := newAuthorizationPolicyTestTopology()

	policy := newAuthorizationPolicy(
		nil,
		authAccessConfig{},
		dashboards,
	)

	if policy.Enabled() {
		t.Fatal("authorization policy enabled without dashboard access rules")
	}
	if !policy.canAccessDashboard("", dashboardsByName["Default"]) {
		t.Fatal("disabled policy denied dashboard")
	}
	if !policy.canAccessPage("", pages["orphan"]) {
		t.Fatal("disabled policy denied page")
	}

	got := policy.authorizedDashboards("", dashboards)
	if len(got) != len(dashboards) {
		t.Fatalf(
			"disabled policy returned %d dashboards, want %d",
			len(got),
			len(dashboards),
		)
	}
}

func TestAuthorizationPolicyDirectDashboardGrant(t *testing.T) {
	dashboards, dashboardsByName, _ := newAuthorizationPolicyTestTopology()

	policy := newAuthorizationPolicy(
		nil,
		authAccessConfig{
			Dashboards: map[string]authDashboardAccessConfig{
				"Default": {
					Users: []string{"mark"},
				},
			},
		},
		dashboards,
	)

	if !policy.canAccessDashboard("mark", dashboardsByName["Default"]) {
		t.Fatal("directly granted identity denied Default dashboard")
	}
	if policy.canAccessDashboard("kellie", dashboardsByName["Default"]) {
		t.Fatal("ungranted identity allowed Default dashboard")
	}
	if policy.canAccessDashboard("mark", dashboardsByName["Admin"]) {
		t.Fatal("identity allowed dashboard without access rule")
	}
}

func TestAuthorizationPolicyGroupDashboardGrant(t *testing.T) {
	dashboards, dashboardsByName, _ := newAuthorizationPolicyTestTopology()

	policy := newAuthorizationPolicy(
		map[string]authGroupConfig{
			"family": {
				Users: []string{"mark", "kellie"},
			},
		},
		authAccessConfig{
			Dashboards: map[string]authDashboardAccessConfig{
				"Default": {
					Groups: []string{"family"},
				},
			},
		},
		dashboards,
	)

	for _, identity := range []string{"mark", "kellie"} {
		if !policy.canAccessDashboard(
			identity,
			dashboardsByName["Default"],
		) {
			t.Fatalf(
				"group identity %q denied Default dashboard",
				identity,
			)
		}
	}
}

func TestAuthorizationPolicyCombinesDirectAndGroupGrants(t *testing.T) {
	dashboards, dashboardsByName, _ := newAuthorizationPolicyTestTopology()

	policy := newAuthorizationPolicy(
		map[string]authGroupConfig{
			"admins": {
				Users: []string{"mark"},
			},
		},
		authAccessConfig{
			Dashboards: map[string]authDashboardAccessConfig{
				"Admin": {
					Users:  []string{"direct-user"},
					Groups: []string{"admins"},
				},
			},
		},
		dashboards,
	)

	for _, identity := range []string{"mark", "direct-user"} {
		if !policy.canAccessDashboard(
			identity,
			dashboardsByName["Admin"],
		) {
			t.Fatalf(
				"granted identity %q denied Admin dashboard",
				identity,
			)
		}
	}
}

func TestAuthorizationPolicyPageAccessUsesAnyContainingDashboard(t *testing.T) {
	dashboards, _, pages := newAuthorizationPolicyTestTopology()

	policy := newAuthorizationPolicy(
		nil,
		authAccessConfig{
			Dashboards: map[string]authDashboardAccessConfig{
				"Default": {
					Users: []string{"family-user"},
				},
				"Admin": {
					Users: []string{"admin-user"},
				},
			},
		},
		dashboards,
	)

	tests := []struct {
		name     string
		identity string
		page     string
		want     bool
	}{
		{
			name:     "Default-only page allowed",
			identity: "family-user",
			page:     "home",
			want:     true,
		},
		{
			name:     "Admin-only page denied",
			identity: "family-user",
			page:     "monitoring",
			want:     false,
		},
		{
			name:     "shared page through Default",
			identity: "family-user",
			page:     "sports",
			want:     true,
		},
		{
			name:     "shared page through Admin",
			identity: "admin-user",
			page:     "sports",
			want:     true,
		},
		{
			name:     "orphan page denied",
			identity: "family-user",
			page:     "orphan",
			want:     false,
		},
		{
			name: "empty identity denied",
			page: "home",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.canAccessPage(
				tt.identity,
				pages[tt.page],
			); got != tt.want {
				t.Fatalf(
					"canAccessPage(%q, %q) = %t, want %t",
					tt.identity,
					tt.page,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestAuthorizationPolicyAuthorizedDashboardsPreservesOrder(t *testing.T) {
	dashboards, _, _ := newAuthorizationPolicyTestTopology()

	policy := newAuthorizationPolicy(
		nil,
		authAccessConfig{
			Dashboards: map[string]authDashboardAccessConfig{
				"Default": {
					Users: []string{"mark"},
				},
				"Admin": {
					Users: []string{"mark"},
				},
				"Personal": {
					Users: []string{"other-user"},
				},
			},
		},
		dashboards,
	)

	got := policy.authorizedDashboards("mark", dashboards)
	if len(got) != 2 {
		t.Fatalf("authorized dashboards = %d, want 2", len(got))
	}
	if got[0] != dashboards[0] || got[1] != dashboards[1] {
		t.Fatal("authorized dashboard filtering did not preserve configured order")
	}
}

func TestAuthorizationPolicyIgnoresConfiguredDashboardAbsentFromRuntime(
	t *testing.T,
) {
	dashboards, dashboardsByName, _ := newAuthorizationPolicyTestTopology()

	policy := newAuthorizationPolicy(
		nil,
		authAccessConfig{
			Dashboards: map[string]authDashboardAccessConfig{
				"Default": {
					Users: []string{"mark"},
				},
				"Ignored": {
					Users: []string{"mark"},
				},
			},
		},
		dashboards,
	)

	if len(policy.dashboardIdentities) != 1 {
		t.Fatalf(
			"compiled dashboard grants = %d, want 1",
			len(policy.dashboardIdentities),
		)
	}
	if !policy.canAccessDashboard("mark", dashboardsByName["Default"]) {
		t.Fatal("runtime Default dashboard grant was lost")
	}
}

func TestAuthorizationPolicyNilTargetsFailClosedWhenEnabled(t *testing.T) {
	dashboards, _, _ := newAuthorizationPolicyTestTopology()

	policy := newAuthorizationPolicy(
		nil,
		authAccessConfig{
			Dashboards: map[string]authDashboardAccessConfig{
				"Default": {
					Users: []string{"mark"},
				},
			},
		},
		dashboards,
	)

	if policy.canAccessDashboard("mark", nil) {
		t.Fatal("enabled policy allowed nil dashboard")
	}
	if policy.canAccessPage("mark", nil) {
		t.Fatal("enabled policy allowed nil page")
	}
}
