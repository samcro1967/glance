package glance

// authorizationPolicy is the immutable authorization view for one application
// generation. Group membership is expanded during construction so request-time
// dashboard checks are simple identity lookups.
type authorizationPolicy struct {
	enabled                bool
	unrestrictedNamedUsers bool
	dashboardIdentities    map[*dashboard]map[string]struct{}
	pageDashboards         map[*page][]*dashboard
	dashboardPages         map[*dashboard]map[*page]struct{}
	guestDashboardPages    map[*dashboard]map[*page]struct{}
}

func newAuthorizationPolicy(
	groups map[string]authGroupConfig,
	access authAccessConfig,
	dashboards []*dashboard,
) *authorizationPolicy {
	policy := &authorizationPolicy{
		enabled:                len(access.Dashboards) > 0,
		unrestrictedNamedUsers: len(access.Dashboards) == 0,
		dashboardIdentities:    make(map[*dashboard]map[string]struct{}),
		pageDashboards:         make(map[*page][]*dashboard),
		dashboardPages:         make(map[*dashboard]map[*page]struct{}),
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
			if len(dashboardAccess.Pages) > 0 {
				allowed := make(map[*page]struct{}, len(dashboardAccess.Pages))
				for _, candidate := range dashboard.Pages {
					for _, slug := range dashboardAccess.Pages {
						if candidate.Slug == slug {
							allowed[candidate] = struct{}{}
						}
					}
				}
				policy.dashboardPages[dashboard] = allowed
			}
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

// grantGuest adds the configured anonymous principal to the existing policy.
// The same dashboard and page authorization checks serve guest and named users.
func (p *authorizationPolicy) grantGuest(guest authGuestConfig, dashboards []*dashboard) {
	if !guest.Enabled {
		return
	}
	p.enabled = true
	for _, dashboard := range dashboards {
		pages, granted := guest.Dashboards[dashboard.Name]
		if !granted {
			continue
		}
		if p.dashboardIdentities[dashboard] == nil {
			p.dashboardIdentities[dashboard] = make(map[string]struct{})
		}
		p.dashboardIdentities[dashboard][guestSessionIdentity] = struct{}{}
		if len(pages) > 0 {
			allowed := make(map[*page]struct{}, len(pages))
			for _, candidate := range dashboard.Pages {
				for _, slug := range pages {
					if candidate.Slug == slug {
						allowed[candidate] = struct{}{}
					}
				}
			}
			// Existing named-user page rules and guest page rules must not
			// share a single dashboard-wide allowlist; use guest-specific data.
			if p.guestDashboardPages == nil {
				p.guestDashboardPages = newGuestDashboardPageMap()
			}
			p.guestDashboardPages[dashboard] = allowed
		}
		for _, candidate := range dashboard.Pages {
			p.pageDashboards[candidate] = appendUniqueDashboard(p.pageDashboards[candidate], dashboard)
		}
	}
}

func newGuestDashboardPageMap() map[*dashboard]map[*page]struct{} {
	return make(map[*dashboard]map[*page]struct{})
}

func appendUniqueDashboard(existing []*dashboard, dashboard *dashboard) []*dashboard {
	for _, item := range existing {
		if item == dashboard {
			return existing
		}
	}
	return append(existing, dashboard)
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
	if identity != guestSessionIdentity && p.unrestrictedNamedUsers {
		return true
	}

	identities, configured := p.dashboardIdentities[dashboard]
	if !configured {
		return false
	}

	_, allowed := identities[identity]
	return allowed
}

// canAccessDashboardPage enforces a page grant in its requested dashboard context.
func (p *authorizationPolicy) canAccessDashboardPage(identity string, dashboard *dashboard, page *page) bool {
	if dashboard == nil || page == nil || !p.canAccessDashboard(identity, dashboard) {
		return false
	}
	for _, candidate := range dashboard.Pages {
		if candidate == page {
			if identity != guestSessionIdentity && p.unrestrictedNamedUsers {
				return true
			}
			if identity == guestSessionIdentity {
				if allowed, restricted := p.guestDashboardPages[dashboard]; restricted {
					_, ok := allowed[page]
					return ok
				}
				return true
			}
			if allowed, restricted := p.dashboardPages[dashboard]; restricted {
				_, ok := allowed[page]
				return ok
			}
			return true
		}
	}
	return false
}

func (p *authorizationPolicy) authorizedPages(identity string, dashboard *dashboard) []*page {
	if dashboard == nil {
		return nil
	}
	result := make([]*page, 0, len(dashboard.Pages))
	for _, candidate := range dashboard.Pages {
		if p.canAccessDashboardPage(identity, dashboard, candidate) {
			result = append(result, candidate)
		}
	}
	return result
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
	if identity != guestSessionIdentity && p.unrestrictedNamedUsers {
		return true
	}

	for _, dashboard := range p.pageDashboards[page] {
		if p.canAccessDashboardPage(identity, dashboard, page) {
			return true
		}
	}

	return false
}

func (p *authorizationPolicy) canAccessAllDashboards(
	identity string,
	dashboards []*dashboard,
) bool {
	if p == nil || !p.enabled {
		return true
	}
	if identity == "" {
		return false
	}

	for _, dashboard := range dashboards {
		if !p.canAccessDashboard(identity, dashboard) {
			return false
		}
	}

	return true
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
