package glance

// authorizationPolicy is the immutable authorization view for one application
// generation. Group membership is expanded during construction so request-time
// dashboard checks are simple identity lookups.
type authorizationPolicy struct {
	enabled             bool
	dashboardIdentities map[*dashboard]map[string]struct{}
	pageDashboards      map[*page][]*dashboard
}

func newAuthorizationPolicy(
	groups map[string]authGroupConfig,
	access authAccessConfig,
	dashboards []*dashboard,
) *authorizationPolicy {
	policy := &authorizationPolicy{
		enabled:             len(access.Dashboards) > 0,
		dashboardIdentities: make(map[*dashboard]map[string]struct{}),
		pageDashboards:      make(map[*page][]*dashboard),
	}

	if !policy.enabled {
		return policy
	}

	groupUsers := make(map[string]map[string]struct{}, len(groups))
	for groupName, group := range groups {
		users := make(map[string]struct{}, len(group.Users))
		for _, identity := range group.Users {
			users[identity] = struct{}{}
		}
		groupUsers[groupName] = users
	}

	for _, dashboard := range dashboards {
		dashboardAccess, configured := access.Dashboards[dashboard.Name]
		if configured {
			identities := make(
				map[string]struct{},
				len(dashboardAccess.Users),
			)

			for _, identity := range dashboardAccess.Users {
				identities[identity] = struct{}{}
			}

			for _, groupName := range dashboardAccess.Groups {
				for identity := range groupUsers[groupName] {
					identities[identity] = struct{}{}
				}
			}

			policy.dashboardIdentities[dashboard] = identities
		}

		for _, page := range dashboard.Pages {
			policy.pageDashboards[page] = append(
				policy.pageDashboards[page],
				dashboard,
			)
		}
	}

	return policy
}

func (p *authorizationPolicy) Enabled() bool {
	return p != nil && p.enabled
}

func (p *authorizationPolicy) canAccessDashboard(
	identity string,
	dashboard *dashboard,
) bool {
	if p == nil || !p.enabled {
		return true
	}
	if identity == "" || dashboard == nil {
		return false
	}

	identities, configured := p.dashboardIdentities[dashboard]
	if !configured {
		return false
	}

	_, allowed := identities[identity]
	return allowed
}

func (p *authorizationPolicy) canAccessPage(
	identity string,
	page *page,
) bool {
	if p == nil || !p.enabled {
		return true
	}
	if identity == "" || page == nil {
		return false
	}

	for _, dashboard := range p.pageDashboards[page] {
		if p.canAccessDashboard(identity, dashboard) {
			return true
		}
	}

	return false
}

func (p *authorizationPolicy) authorizedDashboards(
	identity string,
	dashboards []*dashboard,
) []*dashboard {
	if p == nil || !p.enabled {
		return dashboards
	}

	authorized := make([]*dashboard, 0, len(dashboards))
	for _, dashboard := range dashboards {
		if p.canAccessDashboard(identity, dashboard) {
			authorized = append(authorized, dashboard)
		}
	}

	return authorized
}
