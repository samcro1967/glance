package glance

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGuestSessionResolutionRequiresEnabledCanonicalIdentity(t *testing.T) {
	app := newAuthTestApplication(t)
	now := time.Now()
	makeRequest := func(principal, displayName, identity string) *http.Request {
		t.Helper()
		token, err := generateSessionTokenV4(authMethodGuest, principal, displayName, identity, app.authSecretKey, now)
		if err != nil {
			t.Fatalf("generating guest token: %v", err)
		}
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.AddCookie(&http.Cookie{Name: AUTH_SESSION_COOKIE_NAME, Value: token})
		return request
	}
	valid := makeRequest(guestSessionPrincipal, guestSessionDisplayName, guestSessionIdentity)
	if _, ok := app.resolveAuthenticatedSession(valid, now); ok {
		t.Fatal("disabled guest token was accepted")
	}
	app.Config.Auth.Guest.Enabled = true
	if _, ok := app.resolveAuthenticatedSession(valid, now); ok {
		t.Fatal("guest token accepted without dashboard grants")
	}
	app.Config.Auth.Guest.Dashboards = map[string][]string{"public": nil}
	session, ok := app.resolveAuthenticatedSession(valid, now)
	if !ok || session.Method != authMethodGuest || session.AuthorizationIdentity != guestSessionIdentity {
		t.Fatal("enabled canonical guest session was rejected")
	}
	for _, test := range []struct{ principal, displayName, identity string }{
		{"test-user", guestSessionDisplayName, guestSessionIdentity},
		{guestSessionPrincipal, "Administrator", guestSessionIdentity},
		{guestSessionPrincipal, guestSessionDisplayName, "test-user"},
	} {
		if _, ok := app.resolveAuthenticatedSession(makeRequest(test.principal, test.displayName, test.identity), now); ok {
			t.Fatalf("accepted noncanonical guest session: %+v", test)
		}
	}
	app.Config.Auth.Guest.Enabled = false
	if _, ok := app.resolveAuthenticatedSession(valid, now); ok {
		t.Fatal("guest session survived disabling")
	}
}
