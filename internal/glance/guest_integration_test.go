package glance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGuestAuthorizationGrantsAndIsolation(t *testing.T) {
	dashboards, byName, pages := newAuthorizationPolicyTestTopology()
	policy := newAuthorizationPolicy(nil, authAccessConfig{}, dashboards)
	policy.grantGuest(authGuestConfig{Enabled: true, Dashboards: map[string][]string{
		"Default": {"home"}, "Personal": nil,
	}}, dashboards)
	if !policy.Enabled() {
		t.Fatal("guest grants must enable policy")
	}
	if !policy.canAccessDashboard(guestSessionIdentity, byName["Default"]) {
		t.Fatal("guest default dashboard denied")
	}
	if policy.canAccessDashboard(guestSessionIdentity, byName["Admin"]) {
		t.Fatal("guest admin dashboard granted")
	}
	if !policy.canAccessDashboardPage(guestSessionIdentity, byName["Default"], pages["home"]) {
		t.Fatal("guest allowed page denied")
	}
	if policy.canAccessDashboardPage(guestSessionIdentity, byName["Default"], pages["sports"]) {
		t.Fatal("guest excluded page granted")
	}
	if !policy.canAccessDashboardPage(guestSessionIdentity, byName["Personal"], pages["home"]) {
		t.Fatal("guest all-pages dashboard denied")
	}
	if policy.canAccessPage(guestSessionIdentity, pages["monitoring"]) {
		t.Fatal("guest restricted page granted")
	}
	if policy.canAccessPage(guestSessionIdentity, pages["orphan"]) {
		t.Fatal("guest orphan page granted")
	}
	if !policy.canAccessDashboard("other-user", byName["Default"]) {
		t.Fatal("guest configuration restricted an otherwise unrestricted named user")
	}
	if !policy.canAccessDashboardPage("other-user", byName["Default"], pages["sports"]) {
		t.Fatal("named user incorrectly inherited guest page restrictions")
	}
}

func TestGuestAuthorizationDoesNotChangeNamedUserPageRules(t *testing.T) {
	dashboards, byName, pages := newAuthorizationPolicyTestTopology()
	policy := newAuthorizationPolicy(nil, authAccessConfig{Dashboards: map[string]authDashboardAccessConfig{
		"Default": {Users: []string{"alice"}, Pages: []string{"sports"}},
	}}, dashboards)
	policy.grantGuest(authGuestConfig{Enabled: true, Dashboards: map[string][]string{"Default": {"home"}}}, dashboards)
	if !policy.canAccessDashboardPage("alice", byName["Default"], pages["sports"]) {
		t.Fatal("named user grant lost")
	}
	if policy.canAccessDashboardPage("alice", byName["Default"], pages["home"]) {
		t.Fatal("named user gained guest page")
	}
	if !policy.canAccessDashboardPage(guestSessionIdentity, byName["Default"], pages["home"]) {
		t.Fatal("guest grant lost")
	}
	if policy.canAccessDashboardPage(guestSessionIdentity, byName["Default"], pages["sports"]) {
		t.Fatal("guest gained named-user page")
	}
}

func TestGuestLoginEndpointAndCookie(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	app := &application{RequiresAuth: true, authSecretKey: secret}
	app.Config.Auth.Guest = authGuestConfig{Enabled: true, Dashboards: map[string][]string{"Default": nil}}
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/guest", nil)
	req.Header.Set("Origin", "http://example.com")
	recorder := httptest.NewRecorder()
	app.handleGuestAuthentication(recorder, req)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("guest login status %d", recorder.Code)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("guest login missing HttpOnly cookie")
	}
	verifyReq := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	verifyReq.AddCookie(cookies[0])
	session, ok := app.resolveAuthenticatedSession(verifyReq, time.Now())
	if !ok || session.Method != authMethodGuest || session.AuthorizationIdentity != guestSessionIdentity {
		t.Fatal("guest session did not resolve")
	}
	app.Config.Auth.Guest.Enabled = false
	if _, ok := app.resolveAuthenticatedSession(verifyReq, time.Now()); ok {
		t.Fatal("disabled guest session remained valid")
	}
}

func TestGuestLoginRejectsCrossOriginAndDisabled(t *testing.T) {
	app := &application{RequiresAuth: true, authSecretKey: make([]byte, AUTH_SECRET_KEY_LENGTH)}
	app.Config.Auth.Guest = authGuestConfig{Enabled: true, Dashboards: map[string][]string{"Default": nil}}
	for _, origin := range []string{"https://attacker.example", "http://attacker.example"} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/guest", strings.NewReader(""))
		req.Header.Set("Origin", origin)
		recorder := httptest.NewRecorder()
		app.handleGuestAuthentication(recorder, req)
		if recorder.Code != http.StatusForbidden || len(recorder.Result().Cookies()) != 0 {
			t.Fatalf("cross-origin %s accepted", origin)
		}
	}
	app.Config.Auth.Guest.Enabled = false
	recorder := httptest.NewRecorder()
	app.handleGuestAuthentication(recorder, httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/guest", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("disabled guest endpoint status %d", recorder.Code)
	}
}

func TestGuestGrantsPreserveUnrestrictedNamedUsers(t *testing.T) {
	dashboards, byName, pages := newAuthorizationPolicyTestTopology()
	policy := newAuthorizationPolicy(nil, authAccessConfig{}, dashboards)
	policy.grantGuest(authGuestConfig{Enabled: true, Dashboards: map[string][]string{"Default": {"home"}}}, dashboards)
	if !policy.canAccessDashboard("alice", byName["Admin"]) {
		t.Fatal("named user lost unrestricted dashboard")
	}
	if !policy.canAccessPage("alice", pages["orphan"]) {
		t.Fatal("named user lost legacy page access")
	}
	if !policy.canAccessDashboardPage("alice", byName["Default"], pages["sports"]) {
		t.Fatal("named user inherited guest page restrictions")
	}
}

func TestGuestAuthorizationExplicitNamedAccessPolicy(t *testing.T) {
	dashboards, byName, pages := newAuthorizationPolicyTestTopology()

	unrestricted := newAuthorizationPolicy(nil, authAccessConfig{}, dashboards)
	unrestricted.grantGuest(authGuestConfig{
		Enabled:    true,
		Dashboards: map[string][]string{"Default": {"home"}},
	}, dashboards)

	if !unrestricted.unrestrictedNamedUsers {
		t.Fatal("guest-only policy must preserve unrestricted named users")
	}
	if !unrestricted.canAccessDashboard("alice", byName["Admin"]) {
		t.Fatal("unrestricted named user denied Admin dashboard")
	}
	if !unrestricted.canAccessPage("alice", pages["orphan"]) {
		t.Fatal("unrestricted named user denied orphan page")
	}

	restricted := &authorizationPolicy{
		enabled: true,
		dashboardIdentities: map[*dashboard]map[string]struct{}{
			byName["Default"]: {"alice": {}},
		},
		pageDashboards: map[*page][]*dashboard{
			pages["home"]: {byName["Default"]},
		},
	}

	if restricted.unrestrictedNamedUsers {
		t.Fatal("manually constructed policy unexpectedly unrestricted")
	}
	if restricted.canAccessDashboard("alice", byName["Admin"]) {
		t.Fatal("restricted policy allowed ungranted dashboard")
	}
	if restricted.canAccessPage("alice", pages["orphan"]) {
		t.Fatal("restricted policy allowed orphan page")
	}
}

func TestGuestLoginOriginSchemeMatchesTrustedRequest(t *testing.T) {
	app := &application{
		RequiresAuth:  true,
		authSecretKey: make([]byte, AUTH_SECRET_KEY_LENGTH),
	}
	app.Config.Auth.Guest = authGuestConfig{
		Enabled:    true,
		Dashboards: map[string][]string{"Default": nil},
	}

	cases := []struct {
		name       string
		requestURL string
		origin     string
		want       int
	}{
		{"HTTP matching", "http://example.com/api/auth/guest", "http://example.com", http.StatusSeeOther},
		{"HTTP rejects HTTPS origin", "http://example.com/api/auth/guest", "https://example.com", http.StatusForbidden},
		{"HTTPS matching", "https://example.com/api/auth/guest", "https://example.com", http.StatusSeeOther},
		{"HTTPS rejects HTTP origin", "https://example.com/api/auth/guest", "http://example.com", http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.requestURL, nil)
			req.Header.Set("Origin", tc.origin)
			recorder := httptest.NewRecorder()
			app.handleGuestAuthentication(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.want)
			}
		})
	}
}

func TestGuestLoginOriginSchemeBehindTrustedProxy(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		origin     string
		want       int
	}{
		{
			name:       "trusted proxy accepts HTTPS origin",
			remoteAddr: "192.0.2.10:1234",
			origin:     "https://example.com",
			want:       http.StatusSeeOther,
		},
		{
			name:       "trusted proxy rejects HTTP origin",
			remoteAddr: "192.0.2.10:1234",
			origin:     "http://example.com",
			want:       http.StatusForbidden,
		},
		{
			name:       "untrusted proxy rejects HTTPS origin",
			remoteAddr: "203.0.113.10:1234",
			origin:     "https://example.com",
			want:       http.StatusForbidden,
		},
		{
			name:       "untrusted proxy accepts HTTP origin",
			remoteAddr: "203.0.113.10:1234",
			origin:     "http://example.com",
			want:       http.StatusSeeOther,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newProxyTrustTestApplication(
				t,
				true,
				[]string{"192.0.2.0/24"},
			)
			app.RequiresAuth = true
			app.authSecretKey = make([]byte, AUTH_SECRET_KEY_LENGTH)
			app.Config.Auth.Guest = authGuestConfig{
				Enabled:    true,
				Dashboards: map[string][]string{"Default": nil},
			}

			req := httptest.NewRequest(
				http.MethodPost,
				"http://example.com/api/auth/guest",
				nil,
			)
			req.RemoteAddr = tc.remoteAddr
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("Origin", tc.origin)

			recorder := httptest.NewRecorder()
			app.handleGuestAuthentication(recorder, req)

			if recorder.Code != tc.want {
				t.Fatalf(
					"status = %d, want %d",
					recorder.Code,
					tc.want,
				)
			}
		})
	}
}
