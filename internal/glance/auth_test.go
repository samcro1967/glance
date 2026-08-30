package glance

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func newAuthTestApplication(t *testing.T) *application {
	t.Helper()

	secretString, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatalf("generating auth secret: %v", err)
	}

	secret, err := base64.StdEncoding.DecodeString(secretString)
	if err != nil {
		t.Fatalf("decoding auth secret: %v", err)
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("test-password"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hashing test password: %v", err)
	}

	username := "test-user"
	usernameHash, err := computeUsernameHash(username, secret)
	if err != nil {
		t.Fatalf("computing username hash: %v", err)
	}

	app := &application{
		RequiresAuth:           true,
		authSecretKey:          secret,
		usernameHashToUsername: map[string]string{string(usernameHash): username},
		failedAuthAttempts:     make(map[string]*failedAuthAttempt),
	}

	app.Config.Auth.Users = map[string]*user{
		username: {
			PasswordHash: passwordHash,
		},
	}

	return app
}

func authTestSessionToken(
	t *testing.T,
	app *application,
	username string,
	now time.Time,
) string {
	t.Helper()

	token, err := generateSessionToken(username, app.authSecretKey, now)
	if err != nil {
		t.Fatalf("generating session token: %v", err)
	}

	return token
}

func TestAuthTokenGenerationAndVerification(t *testing.T) {
	secret, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatalf("Failed to generate secret key: %v", err)
	}

	secretBytes, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("Failed to decode secret key: %v", err)
	}

	if len(secretBytes) != AUTH_SECRET_KEY_LENGTH {
		t.Fatalf("Secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}

	now := time.Now()
	username := "admin"

	token, err := generateSessionToken(username, secretBytes, now)
	if err != nil {
		t.Fatalf("Failed to generate session token: %v", err)
	}

	usernameHashBytes, shouldRegen, err := verifySessionToken(
		token,
		secretBytes,
		now,
	)
	if err != nil {
		t.Fatalf("Failed to verify session token: %v", err)
	}

	if shouldRegen {
		t.Fatal("Token should not need to be regenerated immediately after generation")
	}

	computedUsernameHash, err := computeUsernameHash(username, secretBytes)
	if err != nil {
		t.Fatalf("Failed to compute username hash: %v", err)
	}

	if !bytes.Equal(usernameHashBytes, computedUsernameHash) {
		t.Fatal("Username hash does not match the expected value")
	}

	timeRightAfterRegenPeriod := now.Add(
		AUTH_TOKEN_VALID_PERIOD - AUTH_TOKEN_REGEN_BEFORE + 2*time.Second,
	)

	_, shouldRegen, err = verifySessionToken(
		token,
		secretBytes,
		timeRightAfterRegenPeriod,
	)
	if err != nil {
		t.Fatalf(
			"Token verification should not fail during regeneration period, err: %v",
			err,
		)
	}

	if !shouldRegen {
		t.Fatal("Token should have been marked for regeneration")
	}

	_, _, err = verifySessionToken(
		token,
		secretBytes,
		now.Add(AUTH_TOKEN_VALID_PERIOD+2*time.Second),
	)
	if err == nil {
		t.Fatal("Expected token verification to fail after token expiration")
	}

	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("Failed to decode token: %v", err)
	}

	for i := range len(decodedToken) {
		tampered := make([]byte, len(decodedToken))
		copy(tampered, decodedToken)
		tampered[i]++

		_, _, err = verifySessionToken(
			base64.StdEncoding.EncodeToString(tampered),
			secretBytes,
			now,
		)
		if err == nil {
			t.Fatalf(
				"Expected token verification to fail for tampered token at index %d",
				i,
			)
		}
	}
}

func TestAuthTokenRejectsInvalidInputs(t *testing.T) {
	validSecret := make([]byte, AUTH_SECRET_KEY_LENGTH)

	tests := []struct {
		name   string
		token  string
		secret []byte
	}{
		{
			name:   "invalid base64",
			token:  "not-base64",
			secret: validSecret,
		},
		{
			name:   "invalid token length",
			token:  base64.StdEncoding.EncodeToString([]byte("short")),
			secret: validSecret,
		},
		{
			name: "invalid secret length",
			token: base64.StdEncoding.EncodeToString(
				make([]byte, AUTH_TOKEN_DATA_LENGTH+32),
			),
			secret: []byte("short"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := verifySessionToken(
				tt.token,
				tt.secret,
				time.Now(),
			)
			if err == nil {
				t.Fatal("expected token verification to fail")
			}
		})
	}

	if _, err := generateSessionToken(
		"test-user",
		[]byte("short"),
		time.Now(),
	); err == nil {
		t.Fatal("expected token generation with invalid secret length to fail")
	}

	if _, err := computeUsernameHash(
		"test-user",
		[]byte("short"),
	); err == nil {
		t.Fatal("expected username hashing with invalid secret length to fail")
	}
}

func TestIsAuthorizedWhenAuthenticationDisabled(t *testing.T) {
	app := &application{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	if !app.isAuthorized(rec, req) {
		t.Fatal("expected request to be authorized when authentication is disabled")
	}
}

func TestIsAuthorizedRejectsMissingInvalidAndUnknownSessions(t *testing.T) {
	app := newAuthTestApplication(t)

	tests := []struct {
		name  string
		token string
	}{
		{
			name: "missing session",
		},
		{
			name:  "invalid session",
			token: "invalid-token",
		},
		{
			name: "unknown user",
			token: authTestSessionToken(
				t,
				app,
				"unknown-user",
				time.Now(),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			if tt.token != "" {
				req.AddCookie(&http.Cookie{
					Name:  AUTH_SESSION_COOKIE_NAME,
					Value: tt.token,
				})
			}

			rec := httptest.NewRecorder()

			if app.isAuthorized(rec, req) {
				t.Fatal("expected request to be unauthorized")
			}
		})
	}
}

func TestIsAuthorizedAcceptsValidSession(t *testing.T) {
	app := newAuthTestApplication(t)

	token := authTestSessionToken(
		t,
		app,
		"test-user",
		time.Now(),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})

	rec := httptest.NewRecorder()

	if !app.isAuthorized(rec, req) {
		t.Fatal("expected valid session to be authorized")
	}

	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf(
			"expected valid fresh session not to be regenerated, got %d cookies",
			len(cookies),
		)
	}
}

func TestIsAuthorizedRegeneratesAgingSession(t *testing.T) {
	app := newAuthTestApplication(t)

	tokenCreatedAt := time.Now().Add(
		-(AUTH_TOKEN_VALID_PERIOD - AUTH_TOKEN_REGEN_BEFORE + time.Hour),
	)

	token := authTestSessionToken(
		t,
		app,
		"test-user",
		tokenCreatedAt,
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})

	rec := httptest.NewRecorder()

	if !app.isAuthorized(rec, req) {
		t.Fatal("expected aging valid session to remain authorized")
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf(
			"expected one regenerated session cookie, got %d",
			len(cookies),
		)
	}

	if cookies[0].Name != AUTH_SESSION_COOKIE_NAME {
		t.Fatalf(
			"regenerated cookie name = %q, want %q",
			cookies[0].Name,
			AUTH_SESSION_COOKIE_NAME,
		)
	}

	if cookies[0].Value == "" {
		t.Fatal("expected regenerated session cookie to contain a token")
	}

	if cookies[0].Value == token {
		t.Fatal("expected regenerated session cookie to contain a new token")
	}
}

func TestHandleUnauthorizedResponse(t *testing.T) {
	app := newAuthTestApplication(t)
	app.Config.Server.BaseURL = "/glance"

	t.Run("redirect", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			"/glance/",
			nil,
		)
		rec := httptest.NewRecorder()

		if !app.handleUnauthorizedResponse(
			rec,
			req,
			redirectToLogin,
		) {
			t.Fatal("expected request to be handled as unauthorized")
		}

		if rec.Code != http.StatusSeeOther {
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusSeeOther,
			)
		}

		if location := rec.Header().Get("Location"); location != "/glance/login" {
			t.Fatalf(
				"Location = %q, want %q",
				location,
				"/glance/login",
			)
		}
	})

	t.Run("json", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			"/glance/api/widgets/1",
			nil,
		)
		rec := httptest.NewRecorder()

		if !app.handleUnauthorizedResponse(
			rec,
			req,
			showUnauthorizedJSON,
		) {
			t.Fatal("expected request to be handled as unauthorized")
		}

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusUnauthorized,
			)
		}

		if body := rec.Body.String(); body != `{"error": "Unauthorized"}` {
			t.Fatalf(
				"body = %q, want unauthorized JSON response",
				body,
			)
		}
	})

	t.Run("authorized", func(t *testing.T) {
		token := authTestSessionToken(
			t,
			app,
			"test-user",
			time.Now(),
		)

		req := httptest.NewRequest(
			http.MethodGet,
			"/glance/",
			nil,
		)
		req.AddCookie(&http.Cookie{
			Name:  AUTH_SESSION_COOKIE_NAME,
			Value: token,
		})

		rec := httptest.NewRecorder()

		if app.handleUnauthorizedResponse(
			rec,
			req,
			redirectToLogin,
		) {
			t.Fatal("expected authorized request not to be handled")
		}
	})
}

func TestSetAuthSessionCookie(t *testing.T) {
	app := &application{}
	app.Config.Server.BaseURL = "/glance"

	tests := []struct {
		name       string
		proto      string
		wantSecure bool
	}{
		{
			name:       "http",
			proto:      "http",
			wantSecure: false,
		},
		{
			name:       "https",
			proto:      "https",
			wantSecure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodGet,
				"/",
				nil,
			)
			req.Header.Set("X-Forwarded-Proto", tt.proto)

			rec := httptest.NewRecorder()
			expires := time.Now().Add(time.Hour)

			app.setAuthSessionCookie(
				rec,
				req,
				"test-token",
				expires,
			)

			cookies := rec.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf(
					"expected one cookie, got %d",
					len(cookies),
				)
			}

			cookie := cookies[0]

			if cookie.Name != AUTH_SESSION_COOKIE_NAME {
				t.Fatalf(
					"cookie name = %q, want %q",
					cookie.Name,
					AUTH_SESSION_COOKIE_NAME,
				)
			}

			if cookie.Value != "test-token" {
				t.Fatalf(
					"cookie value = %q, want %q",
					cookie.Value,
					"test-token",
				)
			}

			if cookie.Path != "/glance/" {
				t.Fatalf(
					"cookie path = %q, want %q",
					cookie.Path,
					"/glance/",
				)
			}

			if cookie.Secure != tt.wantSecure {
				t.Fatalf(
					"cookie Secure = %v, want %v",
					cookie.Secure,
					tt.wantSecure,
				)
			}

			if !cookie.HttpOnly {
				t.Fatal("expected cookie to be HttpOnly")
			}

			if cookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf(
					"cookie SameSite = %v, want %v",
					cookie.SameSite,
					http.SameSiteLaxMode,
				)
			}
		})
	}
}

func TestHandleLogoutRequest(t *testing.T) {
	app := &application{}
	app.Config.Server.BaseURL = "/glance"

	req := httptest.NewRequest(
		http.MethodGet,
		"/glance/logout",
		nil,
	)
	rec := httptest.NewRecorder()

	app.handleLogoutRequest(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusSeeOther,
		)
	}

	if location := rec.Header().Get("Location"); location != "/glance/login" {
		t.Fatalf(
			"Location = %q, want %q",
			location,
			"/glance/login",
		)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf(
			"expected one logout cookie, got %d",
			len(cookies),
		)
	}

	cookie := cookies[0]

	if cookie.Name != AUTH_SESSION_COOKIE_NAME {
		t.Fatalf(
			"cookie name = %q, want %q",
			cookie.Name,
			AUTH_SESSION_COOKIE_NAME,
		)
	}

	if cookie.Value != "" {
		t.Fatalf(
			"logout cookie value = %q, want empty",
			cookie.Value,
		)
	}

	if !cookie.Expires.Before(time.Now()) {
		t.Fatalf(
			"logout cookie expiration %v is not in the past",
			cookie.Expires,
		)
	}
}

func TestHandleAuthenticationAttemptRejectsInvalidRequest(t *testing.T) {
	app := newAuthTestApplication(t)

	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        `{"username":"test-user","password":"test-password"}`,
		},
		{
			name:        "malformed json",
			contentType: "application/json",
			body:        `{`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/authenticate",
				strings.NewReader(tt.body),
			)
			req.Header.Set(
				"Content-Type",
				tt.contentType,
			)
			req.RemoteAddr = "192.0.2.1:12345"

			rec := httptest.NewRecorder()

			app.handleAuthenticationAttempt(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf(
					"status = %d, want %d",
					rec.Code,
					http.StatusBadRequest,
				)
			}
		})
	}
}

func TestHandleAuthenticationAttemptSuccess(t *testing.T) {
	app := newAuthTestApplication(t)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/authenticate",
		strings.NewReader(
			`{"username":"test-user","password":"test-password"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.RemoteAddr = "192.0.2.1:12345"

	rec := httptest.NewRecorder()

	app.handleAuthenticationAttempt(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusOK,
		)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf(
			"expected one session cookie, got %d",
			len(cookies),
		)
	}

	cookie := cookies[0]

	if cookie.Name != AUTH_SESSION_COOKIE_NAME {
		t.Fatalf(
			"cookie name = %q, want %q",
			cookie.Name,
			AUTH_SESSION_COOKIE_NAME,
		)
	}

	if cookie.Value == "" {
		t.Fatal("expected session cookie to contain a token")
	}

	if !cookie.Secure {
		t.Fatal("expected HTTPS login session cookie to be Secure")
	}

	verifyReq := httptest.NewRequest(
		http.MethodGet,
		"/",
		nil,
	)
	verifyReq.AddCookie(cookie)

	verifyRec := httptest.NewRecorder()

	if !app.isAuthorized(verifyRec, verifyReq) {
		t.Fatal(
			"expected login session cookie to authorize subsequent request",
		)
	}

	app.authAttemptsMu.Lock()
	_, failedAttemptExists := app.failedAuthAttempts["192.0.2.1"]
	app.authAttemptsMu.Unlock()

	if failedAttemptExists {
		t.Fatal(
			"expected successful login to clear failed attempts for client",
		)
	}
}

func TestHandleAuthenticationAttemptClearsPreviousFailures(t *testing.T) {
	app := newAuthTestApplication(t)

	const clientIP = "192.0.2.1"

	app.failedAuthAttempts[clientIP] = &failedAuthAttempt{
		attempts: 3,
		first:    time.Now(),
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/authenticate",
		strings.NewReader(
			`{"username":"test-user","password":"test-password"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = clientIP + ":12345"

	rec := httptest.NewRecorder()

	app.handleAuthenticationAttempt(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusOK,
		)
	}

	app.authAttemptsMu.Lock()
	_, exists := app.failedAuthAttempts[clientIP]
	app.authAttemptsMu.Unlock()

	if exists {
		t.Fatal(
			"expected successful login to clear previous failed attempts",
		)
	}
}
