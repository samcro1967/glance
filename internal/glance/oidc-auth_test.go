package glance

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"golang.org/x/oauth2"
)

func newOIDCTestApplication(t *testing.T) *application {
	t.Helper()

	app := newAuthTestApplication(t)
	app.Config.Auth.Users = nil
	app.Config.Auth.OIDC = oidcConfig{
		Issuer:       "https://issuer.example.test",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
	}
	app.Config.Server.BaseURL = "/glance"
	app.oidc = &oidcRuntime{
		issuer: app.Config.Auth.OIDC.Issuer,
	}

	return app
}

func TestOIDCPKCEChallengeUsesS256(t *testing.T) {
	verifier := "test-pkce-verifier"
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])

	if got := oidcPKCEChallenge(verifier); got != want {
		t.Fatalf("oidcPKCEChallenge() = %q, want %q", got, want)
	}
}

func TestOIDCFlowEncryptionRoundTrip(t *testing.T) {
	app := newOIDCTestApplication(t)
	now := time.Now()

	want := oidcFlow{
		State:        "state-value",
		Nonce:        "nonce-value",
		PKCEVerifier: "pkce-value",
		RedirectURL:  "https://glance.example.test/glance/auth/oidc/callback",
		Expires:      now.Add(time.Minute).Unix(),
	}

	encoded, err := encryptOIDCFlow(want, app.authSecretKey)
	if err != nil {
		t.Fatalf("encryptOIDCFlow() error = %v", err)
	}

	if strings.Contains(encoded, want.PKCEVerifier) {
		t.Fatal("encrypted OIDC flow exposes PKCE verifier")
	}

	got, err := decryptOIDCFlow(encoded, app.authSecretKey, now)
	if err != nil {
		t.Fatalf("decryptOIDCFlow() error = %v", err)
	}

	if got != want {
		t.Fatalf("decryptOIDCFlow() = %#v, want %#v", got, want)
	}
}

func TestOIDCFlowEncryptionRejectsTampering(t *testing.T) {
	app := newOIDCTestApplication(t)
	now := time.Now()

	encoded, err := encryptOIDCFlow(oidcFlow{
		State:        "state",
		Nonce:        "nonce",
		PKCEVerifier: "pkce",
		RedirectURL:  "https://glance.example.test/glance/auth/oidc/callback",
		Expires:      now.Add(time.Minute).Unix(),
	}, app.authSecretKey)
	if err != nil {
		t.Fatalf("encryptOIDCFlow() error = %v", err)
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decoding encrypted flow: %v", err)
	}

	payload[len(payload)-1] ^= 0x01
	tampered := base64.RawURLEncoding.EncodeToString(payload)

	if _, err := decryptOIDCFlow(tampered, app.authSecretKey, now); err == nil {
		t.Fatal("decryptOIDCFlow() accepted tampered flow")
	}
}

func TestOIDCFlowEncryptionRejectsExpiredFlow(t *testing.T) {
	app := newOIDCTestApplication(t)
	now := time.Now()

	encoded, err := encryptOIDCFlow(oidcFlow{
		State:        "state",
		Nonce:        "nonce",
		PKCEVerifier: "pkce",
		RedirectURL:  "https://glance.example.test/glance/auth/oidc/callback",
		Expires:      now.Add(-time.Second).Unix(),
	}, app.authSecretKey)
	if err != nil {
		t.Fatalf("encryptOIDCFlow() error = %v", err)
	}

	if _, err := decryptOIDCFlow(encoded, app.authSecretKey, now); err == nil {
		t.Fatal("decryptOIDCFlow() accepted expired flow")
	}
}

func TestOIDCCallbackFlowValidationLogDoesNotExposeErrorDetail(t *testing.T) {
	app := newOIDCTestApplication(t)

	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/auth/oidc/callback",
		nil,
	)
	req.AddCookie(&http.Cookie{
		Name:  OIDC_FLOW_COOKIE_NAME,
		Value: "not-valid-base64!",
	})
	rec := httptest.NewRecorder()

	app.handleOIDCCallbackRequest(rec, req)

	logs := logOutput.String()
	if !strings.Contains(logs, "stage=flow_validation") ||
		!strings.Contains(logs, "reason=invalid_or_expired_flow") {
		t.Fatalf("flow validation log missing safe classification: %s", logs)
	}
	if strings.Contains(logs, "decoding OIDC flow") ||
		strings.Contains(logs, "illegal base64") ||
		strings.Contains(logs, "not-valid-base64") {
		t.Fatalf("flow validation log exposes error detail: %s", logs)
	}
}

func TestOIDCRedirectURL(t *testing.T) {
	tests := []struct {
		name           string
		proxied        bool
		trustedProxies []string
		remoteAddr     string
		forwardedProto string
		explicit       string
		want           string
	}{
		{
			name:       "direct HTTP",
			remoteAddr: "192.0.2.10:1234",
			want:       "http://glance.example.test/glance/auth/oidc/callback",
		},
		{
			name:           "untrusted forwarded HTTPS ignored",
			proxied:        true,
			trustedProxies: []string{"192.0.2.20"},
			remoteAddr:     "192.0.2.10:1234",
			forwardedProto: "https",
			want:           "http://glance.example.test/glance/auth/oidc/callback",
		},
		{
			name:           "trusted forwarded HTTPS",
			proxied:        true,
			trustedProxies: []string{"192.0.2.10"},
			remoteAddr:     "192.0.2.10:1234",
			forwardedProto: "https",
			want:           "https://glance.example.test/glance/auth/oidc/callback",
		},
		{
			name:     "explicit redirect wins",
			explicit: "https://public.example.test/oidc/callback",
			want:     "https://public.example.test/oidc/callback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newProxyTrustTestApplication(
				t,
				tt.proxied,
				tt.trustedProxies,
			)
			app.Config.Server.BaseURL = "/glance"
			app.Config.Auth.OIDC.RedirectURL = tt.explicit

			req := httptest.NewRequest(
				http.MethodGet,
				"http://glance.example.test/glance/auth/oidc/login",
				nil,
			)
			req.RemoteAddr = tt.remoteAddr
			req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)

			if got := app.oidcRedirectURL(req); got != tt.want {
				t.Fatalf("oidcRedirectURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOIDCFlowCookieSecurity(t *testing.T) {
	app := newOIDCTestApplication(t)

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/auth/oidc/login",
		nil,
	)
	rec := httptest.NewRecorder()
	expires := time.Now().Add(OIDC_FLOW_VALID_PERIOD)

	app.setOIDCFlowCookie(rec, req, "encrypted-flow", expires)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != OIDC_FLOW_COOKIE_NAME {
		t.Errorf("cookie name = %q, want %q", cookie.Name, OIDC_FLOW_COOKIE_NAME)
	}
	if cookie.Value != "encrypted-flow" {
		t.Errorf("cookie value = %q, want encrypted flow", cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Error("OIDC flow cookie is not HttpOnly")
	}
	if !cookie.Secure {
		t.Error("OIDC flow cookie is not Secure for HTTPS request")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Path != "/glance/auth/oidc/callback" {
		t.Errorf("Path = %q, want callback-only path", cookie.Path)
	}
	if cookie.Domain != "" {
		t.Errorf("Domain = %q, want empty", cookie.Domain)
	}
}

func TestOIDCFlowCookieDeletion(t *testing.T) {
	app := newOIDCTestApplication(t)

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/auth/oidc/callback",
		nil,
	)
	rec := httptest.NewRecorder()

	app.clearOIDCFlowCookie(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != OIDC_FLOW_COOKIE_NAME {
		t.Errorf("cookie name = %q, want %q", cookie.Name, OIDC_FLOW_COOKIE_NAME)
	}
	if cookie.Value != "" {
		t.Errorf("deleted cookie value = %q, want empty", cookie.Value)
	}
	if cookie.MaxAge >= 0 {
		t.Errorf("deleted cookie MaxAge = %d, want negative", cookie.MaxAge)
	}
	if !cookie.HttpOnly || !cookie.Secure {
		t.Error("deleted OIDC flow cookie lost security attributes")
	}
	if cookie.Path != "/glance/auth/oidc/callback" {
		t.Errorf("deleted cookie Path = %q, want callback-only path", cookie.Path)
	}
}

func TestOIDCOnlyLoginRendersLandingPage(t *testing.T) {
	app := newOIDCTestApplication(t)

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/login",
		nil,
	)
	rec := httptest.NewRecorder()

	app.handleLoginPageRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "/glance/auth/oidc/login") {
		t.Fatal("login page does not contain OIDC login link")
	}
	if !strings.Contains(body, "SIGN IN WITH SSO") {
		t.Fatal("login page does not contain generic SSO label")
	}
}

func TestOIDCDeniedLoginRendersSafeMessage(t *testing.T) {
	app := newOIDCTestApplication(t)

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/login?reason=oidc_not_authorized",
		nil,
	)
	rec := httptest.NewRecorder()

	app.handleLoginPageRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "This account is not authorized to access Glance.") {
		t.Fatal("login page does not contain authorization denial message")
	}
	if !strings.Contains(body, "/glance/auth/oidc/login") {
		t.Fatal("denied login page does not retain OIDC sign-in link")
	}
	if !strings.Contains(body, `searchParams.delete("reason")`) {
		t.Fatal("denied login page does not clear one-shot authorization reason from browser URL")
	}
	if strings.Contains(body, "denied@example.test") ||
		strings.Contains(body, "user@example.test") {
		t.Fatal("denied login page exposes identity or allowlist data")
	}
}

type oidcCallbackTestProvider struct {
	server   *oidctest.Server
	token    string
	requests int
}

func (p *oidcCallbackTestProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/token" {
		p.server.ServeHTTP(w, r)
		return
	}

	p.requests++

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	if r.Form.Get("code") != "test-code" {
		http.Error(w, "invalid code", http.StatusBadRequest)
		return
	}

	if r.Form.Get("code_verifier") != "test-pkce" {
		http.Error(w, "invalid PKCE verifier", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "test-access-token",
		"token_type":   "Bearer",
		"id_token":     p.token,
	})
}

func newOIDCCallbackTestApplication(
	t *testing.T,
	nonce string,
) (*application, *oidcCallbackTestProvider, func()) {
	return newOIDCCallbackTestApplicationWithEmail(t, nonce, "user@example.test")
}

func newOIDCCallbackTestApplicationWithEmail(
	t *testing.T,
	nonce string,
	email string,
) (*application, *oidcCallbackTestProvider, func()) {
	return newOIDCCallbackTestApplicationWithIdentity(t, nonce, email, true)
}

func newOIDCCallbackTestApplicationWithIdentity(
	t *testing.T,
	nonce string,
	email string,
	emailVerified bool,
) (*application, *oidcCallbackTestProvider, func()) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating OIDC test key: %v", err)
	}

	providerServer := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{
			{
				PublicKey: privateKey.Public(),
				KeyID:     "test-key",
				Algorithm: oidc.RS256,
			},
		},
	}

	handler := &oidcCallbackTestProvider{
		server: providerServer,
	}
	httpServer := httptest.NewServer(handler)
	providerServer.SetIssuer(httpServer.URL)

	now := time.Now()
	claims := map[string]any{
		"iss":   httpServer.URL,
		"aud":   "test-client",
		"sub":   "test-subject",
		"exp":   now.Add(time.Hour).Unix(),
		"iat":   now.Unix(),
		"nonce": nonce,
	}
	if email != "" {
		claims["email"] = email
		claims["email_verified"] = emailVerified
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		httpServer.Close()
		t.Fatalf("encoding ID token claims: %v", err)
	}

	handler.token = oidctest.SignIDToken(
		privateKey,
		"test-key",
		oidc.RS256,
		string(claimsJSON),
	)

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, httpServer.URL)
	if err != nil {
		httpServer.Close()
		t.Fatalf("creating OIDC test provider: %v", err)
	}

	app := newAuthTestApplication(t)
	app.Config.Auth.Users = nil
	app.Config.Server.BaseURL = "/glance"
	app.Config.Auth.OIDC = oidcConfig{
		Issuer:       httpServer.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
	}
	app.oidc = &oidcRuntime{
		issuer:   httpServer.URL,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{
			ClientID: "test-client",
		}),
		oauthConfig: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Endpoint:     provider.Endpoint(),
			Scopes: []string{
				oidc.ScopeOpenID,
			},
		},
	}

	return app, handler, httpServer.Close
}

func oidcCallbackTestFlowCookie(
	t *testing.T,
	app *application,
	state string,
	nonce string,
) *http.Cookie {
	t.Helper()

	encoded, err := encryptOIDCFlow(oidcFlow{
		State:        state,
		Nonce:        nonce,
		PKCEVerifier: "test-pkce",
		RedirectURL:  "https://glance.example.test/glance/auth/oidc/callback",
		Expires:      time.Now().Add(time.Minute).Unix(),
	}, app.authSecretKey)
	if err != nil {
		t.Fatalf("encrypting callback test flow: %v", err)
	}

	return &http.Cookie{
		Name:  OIDC_FLOW_COOKIE_NAME,
		Value: encoded,
	}
}

func TestOIDCCallbackCreatesAuthorizedSession(t *testing.T) {
	app, provider, cleanup := newOIDCCallbackTestApplication(t, "test-nonce")
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/auth/oidc/callback?state=test-state&code=test-code",
		nil,
	)
	req.AddCookie(
		oidcCallbackTestFlowCookie(t, app, "test-state", "test-nonce"),
	)

	rec := httptest.NewRecorder()
	app.handleOIDCCallbackRequest(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("callback status = %d, want %d; body=%q", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	if provider.requests != 1 {
		t.Fatalf("token endpoint requests = %d, want 1", provider.requests)
	}

	if got := rec.Header().Get("Location"); got != "/glance/" {
		t.Fatalf("callback Location = %q, want %q", got, "/glance/")
	}

	var sessionCookie *http.Cookie
	var clearedFlowCookie bool
	for _, cookie := range rec.Result().Cookies() {
		switch cookie.Name {
		case AUTH_SESSION_COOKIE_NAME:
			sessionCookie = cookie
		case OIDC_FLOW_COOKIE_NAME:
			if cookie.MaxAge < 0 {
				clearedFlowCookie = true
			}
		}
	}

	if !clearedFlowCookie {
		t.Fatal("callback did not clear OIDC flow cookie")
	}
	if sessionCookie == nil {
		t.Fatal("callback did not create session cookie")
	}
	if !sessionCookie.HttpOnly || !sessionCookie.Secure {
		t.Fatal("OIDC session cookie does not retain security attributes")
	}

	authReq := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/",
		nil,
	)
	authReq.AddCookie(sessionCookie)
	authRec := httptest.NewRecorder()

	if !app.isAuthorized(authRec, authReq) {
		t.Fatal("OIDC callback session is not accepted by session gate")
	}

	verified, err := verifySessionTokenV3(
		sessionCookie.Value,
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("verifying callback V3 session: %v", err)
	}
	if verified.Method != authMethodOIDC {
		t.Fatalf("session method = %v, want OIDC", verified.Method)
	}
	if verified.DisplayName != "user@example.test" {
		t.Fatalf(
			"session display name = %q, want %q",
			verified.DisplayName,
			"user@example.test",
		)
	}

	principal, err := decodeOIDCPrincipal(verified.Principal)
	if err != nil {
		t.Fatalf("decoding callback principal: %v", err)
	}
	if principal.Issuer != app.oidc.issuer {
		t.Errorf("principal issuer = %q, want %q", principal.Issuer, app.oidc.issuer)
	}
	if principal.Subject != "test-subject" {
		t.Errorf("principal subject = %q, want %q", principal.Subject, "test-subject")
	}
}

func TestOIDCCallbackAllowedUsers(t *testing.T) {
	tests := []struct {
		name         string
		email        string
		allowedUsers []string
		wantStatus   int
		wantSession  bool
	}{
		{
			name:        "unrestricted when omitted",
			email:       "user@example.test",
			wantStatus:  http.StatusSeeOther,
			wantSession: true,
		},
		{
			name:         "allowed exact match",
			email:        "user@example.test",
			allowedUsers: []string{"user@example.test"},
			wantStatus:   http.StatusSeeOther,
			wantSession:  true,
		},
		{
			name:         "allowed case insensitive trimmed match",
			email:        "User@Example.Test",
			allowedUsers: []string{" user@example.test "},
			wantStatus:   http.StatusSeeOther,
			wantSession:  true,
		},
		{
			name:         "denied user",
			email:        "denied@example.test",
			allowedUsers: []string{"user@example.test"},
			wantStatus:   http.StatusSeeOther,
		},
		{
			name:         "missing email fails closed",
			allowedUsers: []string{"user@example.test"},
			wantStatus:   http.StatusSeeOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, cleanup := newOIDCCallbackTestApplicationWithEmail(t, "test-nonce", tt.email)
			defer cleanup()

			app.Config.Auth.OIDC.AllowedUsers = tt.allowedUsers

			req := httptest.NewRequest(
				http.MethodGet,
				"https://glance.example.test/glance/auth/oidc/callback?state=test-state&code=test-code",
				nil,
			)
			req.AddCookie(oidcCallbackTestFlowCookie(t, app, "test-state", "test-nonce"))

			rec := httptest.NewRecorder()
			app.handleOIDCCallbackRequest(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%q", rec.Code, tt.wantStatus, rec.Body.String())
			}

			var sessionCookie *http.Cookie
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == AUTH_SESSION_COOKIE_NAME {
					sessionCookie = cookie
				}
			}

			if tt.wantSession && sessionCookie == nil {
				t.Fatal("authorized callback did not create session cookie")
			}
			if !tt.wantSession && sessionCookie != nil {
				t.Fatal("rejected callback created session cookie")
			}
			if !tt.wantSession {
				wantLocation := "/glance/login?reason=oidc_not_authorized"
				if got := rec.Header().Get("Location"); got != wantLocation {
					t.Fatalf("redirect location = %q, want %q", got, wantLocation)
				}
			}
		})
	}
}

func TestOIDCCallbackAllowedUsersRejectsUnverifiedEmail(t *testing.T) {
	app, _, cleanup := newOIDCCallbackTestApplicationWithIdentity(
		t,
		"test-nonce",
		"user@example.test",
		false,
	)
	defer cleanup()

	app.Config.Auth.OIDC.AllowedUsers = []string{"user@example.test"}

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/auth/oidc/callback?state=test-state&code=test-code",
		nil,
	)
	req.AddCookie(oidcCallbackTestFlowCookie(t, app, "test-state", "test-nonce"))

	rec := httptest.NewRecorder()
	app.handleOIDCCallbackRequest(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/glance/login?reason=oidc_not_authorized" {
		t.Fatalf("redirect location = %q, want %q", got, "/glance/login?reason=oidc_not_authorized")
	}

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == AUTH_SESSION_COOKIE_NAME {
			t.Fatal("unverified email callback created session cookie")
		}
	}
}

func TestOIDCCallbackRejectsWrongStateBeforeExchange(t *testing.T) {
	app, provider, cleanup := newOIDCCallbackTestApplication(t, "test-nonce")
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/auth/oidc/callback?state=wrong-state&code=test-code",
		nil,
	)
	req.AddCookie(
		oidcCallbackTestFlowCookie(t, app, "test-state", "test-nonce"),
	)

	rec := httptest.NewRecorder()
	app.handleOIDCCallbackRequest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if provider.requests != 0 {
		t.Fatalf("token endpoint requests = %d, want 0", provider.requests)
	}

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == OIDC_FLOW_COOKIE_NAME && cookie.MaxAge < 0 {
			return
		}
	}
	t.Fatal("wrong-state callback did not clear OIDC flow cookie")
}

func TestOIDCCallbackRejectsWrongNonce(t *testing.T) {
	app, provider, cleanup := newOIDCCallbackTestApplication(t, "token-nonce")
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodGet,
		"https://glance.example.test/glance/auth/oidc/callback?state=test-state&code=test-code",
		nil,
	)
	req.AddCookie(
		oidcCallbackTestFlowCookie(t, app, "test-state", "flow-nonce"),
	)

	rec := httptest.NewRecorder()
	app.handleOIDCCallbackRequest(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if provider.requests != 1 {
		t.Fatalf("token endpoint requests = %d, want 1", provider.requests)
	}

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == AUTH_SESSION_COOKIE_NAME && cookie.Value != "" {
			t.Fatal("wrong-nonce callback created an authenticated session")
		}
	}
}
