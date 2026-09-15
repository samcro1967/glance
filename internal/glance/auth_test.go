package glance

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
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

func TestOIDCPrincipalRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		issuer  string
		subject string
	}{
		{
			name:    "standard",
			issuer:  "https://accounts.example.test",
			subject: "subject-123",
		},
		{
			name:    "delimiter characters remain opaque",
			issuer:  "https://example.test/realm|one:two",
			subject: "subject|with:delimiters/and?query=yes",
		},
		{
			name:    "unicode",
			issuer:  "https://example.test/realm",
			subject: "user-世界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := encodeOIDCPrincipal(tt.issuer, tt.subject)
			if err != nil {
				t.Fatalf("encoding OIDC principal: %v", err)
			}

			decoded, err := decodeOIDCPrincipal(encoded)
			if err != nil {
				t.Fatalf("decoding OIDC principal: %v", err)
			}

			if decoded.Issuer != tt.issuer {
				t.Fatalf("issuer = %q, want %q", decoded.Issuer, tt.issuer)
			}
			if decoded.Subject != tt.subject {
				t.Fatalf("subject = %q, want %q", decoded.Subject, tt.subject)
			}
		})
	}
}

func TestOIDCPrincipalRejectsInvalidInputs(t *testing.T) {
	if _, err := encodeOIDCPrincipal("", "subject"); err == nil {
		t.Fatal("expected empty issuer to be rejected")
	}
	if _, err := encodeOIDCPrincipal("https://example.test", ""); err == nil {
		t.Fatal("expected empty subject to be rejected")
	}

	tests := []struct {
		name      string
		principal string
	}{
		{
			name:      "invalid base64",
			principal: "%%%",
		},
		{
			name:      "too short",
			principal: base64.RawURLEncoding.EncodeToString([]byte{0, 1, 1}),
		},
		{
			name: "empty issuer",
			principal: base64.RawURLEncoding.EncodeToString(
				[]byte{0, 0, 1, 2},
			),
		},
		{
			name: "issuer length exceeds payload",
			principal: base64.RawURLEncoding.EncodeToString(
				[]byte{0, 10, 1, 2},
			),
		},
		{
			name: "missing subject",
			principal: base64.RawURLEncoding.EncodeToString(
				append([]byte{0, 3}, []byte("iss")...),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeOIDCPrincipal(tt.principal); err == nil {
				t.Fatal("expected invalid OIDC principal to be rejected")
			}
		})
	}
}

func TestAuthTokenV3GenerationAndVerification(t *testing.T) {
	secretString, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatalf("generating auth secret: %v", err)
	}

	secret, err := base64.StdEncoding.DecodeString(secretString)
	if err != nil {
		t.Fatalf("decoding auth secret: %v", err)
	}

	now := time.Now()

	tests := []struct {
		name        string
		method      authMethod
		principal   string
		displayName string
	}{
		{
			name:        "local",
			method:      authMethodLocal,
			principal:   "test-user",
			displayName: "test-user",
		},
		{
			name:        "oidc",
			method:      authMethodOIDC,
			principal:   "canonical-oidc-principal",
			displayName: "user@example.test",
		},
		{
			name:        "oidc without display identity",
			method:      authMethodOIDC,
			principal:   "canonical-oidc-principal",
			displayName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := generateSessionTokenV3(
				tt.method,
				tt.principal,
				tt.displayName,
				secret,
				now,
			)
			if err != nil {
				t.Fatalf("generating V3 session token: %v", err)
			}

			verified, err := verifySessionTokenV3(token, secret, now)
			if err != nil {
				t.Fatalf("verifying V3 session token: %v", err)
			}

			if verified.Method != tt.method {
				t.Fatalf("method = %d, want %d", verified.Method, tt.method)
			}
			if verified.Principal != tt.principal {
				t.Fatalf(
					"principal = %q, want %q",
					verified.Principal,
					tt.principal,
				)
			}
			if verified.DisplayName != tt.displayName {
				t.Fatalf(
					"display name = %q, want %q",
					verified.DisplayName,
					tt.displayName,
				)
			}
			if verified.ShouldRegen {
				t.Fatal("new V3 token should not require regeneration")
			}
		})
	}
}

func TestAuthTokenV3RegenerationAndExpiration(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()

	token, err := generateSessionTokenV3(
		authMethodOIDC,
		"test-principal",
		"user@example.test",
		secret,
		now,
	)
	if err != nil {
		t.Fatalf("generating V3 token: %v", err)
	}

	regenTime := now.Add(
		AUTH_TOKEN_VALID_PERIOD - AUTH_TOKEN_REGEN_BEFORE + 2*time.Second,
	)

	verified, err := verifySessionTokenV3(token, secret, regenTime)
	if err != nil {
		t.Fatalf("verifying V3 token during regeneration period: %v", err)
	}
	if !verified.ShouldRegen {
		t.Fatal("V3 token should require regeneration")
	}
	if verified.DisplayName != "user@example.test" {
		t.Fatalf(
			"display name = %q, want %q",
			verified.DisplayName,
			"user@example.test",
		)
	}

	_, err = verifySessionTokenV3(
		token,
		secret,
		now.Add(AUTH_TOKEN_VALID_PERIOD+2*time.Second),
	)
	if err == nil {
		t.Fatal("expected expired V3 token to be rejected")
	}
}

func TestAuthTokenV3RejectsTampering(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()

	token, err := generateSessionTokenV3(
		authMethodOIDC,
		"test-principal",
		"user@example.test",
		secret,
		now,
	)
	if err != nil {
		t.Fatalf("generating V3 token: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding V3 token: %v", err)
	}

	for i := range len(decoded) {
		tampered := append([]byte(nil), decoded...)
		tampered[i]++

		_, err := verifySessionTokenV3(
			base64.StdEncoding.EncodeToString(tampered),
			secret,
			now,
		)
		if err == nil {
			t.Fatalf(
				"expected tampered V3 token at index %d to be rejected",
				i,
			)
		}
	}
}

func TestAuthTokenV3RejectsInvalidInputs(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()

	tests := []struct {
		name        string
		method      authMethod
		principal   string
		displayName string
		secret      []byte
	}{
		{
			name:        "invalid method",
			method:      authMethod(99),
			principal:   "test-principal",
			displayName: "test-user",
			secret:      secret,
		},
		{
			name:        "empty principal",
			method:      authMethodOIDC,
			principal:   "",
			displayName: "test-user",
			secret:      secret,
		},
		{
			name:        "principal too long",
			method:      authMethodOIDC,
			principal:   strings.Repeat("p", AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH+1),
			displayName: "test-user",
			secret:      secret,
		},
		{
			name:        "display name too long",
			method:      authMethodOIDC,
			principal:   "test-principal",
			displayName: strings.Repeat("d", AUTH_TOKEN_V3_MAX_DISPLAY_LENGTH+1),
			secret:      secret,
		},
		{
			name:        "invalid secret",
			method:      authMethodOIDC,
			principal:   "test-principal",
			displayName: "test-user",
			secret:      []byte("short"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := generateSessionTokenV3(
				tt.method,
				tt.principal,
				tt.displayName,
				tt.secret,
				now,
			); err == nil {
				t.Fatal("expected invalid V3 token input to be rejected")
			}
		})
	}
}

func TestAuthTokenV3CannotVerifyAsV2(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()

	token, err := generateSessionTokenV3(
		authMethodOIDC,
		"test-principal",
		"user@example.test",
		secret,
		now,
	)
	if err != nil {
		t.Fatalf("generating V3 token: %v", err)
	}

	if _, err := verifySessionTokenV2(token, secret, now); err == nil {
		t.Fatal("expected V3 token to be rejected by V2 verifier")
	}
}

func TestAuthTokenV2GenerationAndVerification(t *testing.T) {
	secretString, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatalf("generating auth secret: %v", err)
	}

	secret, err := base64.StdEncoding.DecodeString(secretString)
	if err != nil {
		t.Fatalf("decoding auth secret: %v", err)
	}

	now := time.Now()

	tests := []struct {
		name      string
		method    authMethod
		principal string
	}{
		{
			name:      "local",
			method:    authMethodLocal,
			principal: "test-user",
		},
		{
			name:      "oidc",
			method:    authMethodOIDC,
			principal: "https://accounts.example.test|subject-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := generateSessionTokenV2(
				tt.method,
				tt.principal,
				secret,
				now,
			)
			if err != nil {
				t.Fatalf("generating V2 session token: %v", err)
			}

			verified, err := verifySessionTokenV2(token, secret, now)
			if err != nil {
				t.Fatalf("verifying V2 session token: %v", err)
			}

			if verified.Method != tt.method {
				t.Fatalf(
					"method = %d, want %d",
					verified.Method,
					tt.method,
				)
			}

			if verified.Principal != tt.principal {
				t.Fatalf(
					"principal = %q, want %q",
					verified.Principal,
					tt.principal,
				)
			}

			if verified.ShouldRegen {
				t.Fatal("new V2 token should not require regeneration")
			}
		})
	}
}

func TestAuthTokenV2MethodsProduceDistinctTokens(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()
	principal := "same-principal"

	localToken, err := generateSessionTokenV2(
		authMethodLocal,
		principal,
		secret,
		now,
	)
	if err != nil {
		t.Fatalf("generating local V2 token: %v", err)
	}

	oidcToken, err := generateSessionTokenV2(
		authMethodOIDC,
		principal,
		secret,
		now,
	)
	if err != nil {
		t.Fatalf("generating OIDC V2 token: %v", err)
	}

	if localToken == oidcToken {
		t.Fatal("different authentication methods produced identical tokens")
	}
}

func TestAuthTokenV2RegenerationAndExpiration(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()

	token, err := generateSessionTokenV2(
		authMethodOIDC,
		"test-principal",
		secret,
		now,
	)
	if err != nil {
		t.Fatalf("generating V2 token: %v", err)
	}

	regenTime := now.Add(
		AUTH_TOKEN_VALID_PERIOD - AUTH_TOKEN_REGEN_BEFORE + 2*time.Second,
	)

	verified, err := verifySessionTokenV2(token, secret, regenTime)
	if err != nil {
		t.Fatalf("verifying V2 token during regeneration period: %v", err)
	}
	if !verified.ShouldRegen {
		t.Fatal("V2 token should require regeneration")
	}

	_, err = verifySessionTokenV2(
		token,
		secret,
		now.Add(AUTH_TOKEN_VALID_PERIOD+2*time.Second),
	)
	if err == nil {
		t.Fatal("expected expired V2 token to be rejected")
	}
}

func TestAuthTokenV2RejectsTampering(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()

	token, err := generateSessionTokenV2(
		authMethodOIDC,
		"test-principal",
		secret,
		now,
	)
	if err != nil {
		t.Fatalf("generating V2 token: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding V2 token: %v", err)
	}

	for i := range len(decoded) {
		tampered := append([]byte(nil), decoded...)
		tampered[i]++

		_, err := verifySessionTokenV2(
			base64.StdEncoding.EncodeToString(tampered),
			secret,
			now,
		)
		if err == nil {
			t.Fatalf(
				"expected tampered V2 token at index %d to be rejected",
				i,
			)
		}
	}
}

func TestAuthTokenV2RejectsInvalidInputs(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()

	if _, err := generateSessionTokenV2(
		authMethod(99),
		"test-principal",
		secret,
		now,
	); err == nil {
		t.Fatal("expected invalid authentication method to be rejected")
	}

	if _, err := generateSessionTokenV2(
		authMethodOIDC,
		"test-principal",
		[]byte("short"),
		now,
	); err == nil {
		t.Fatal("expected invalid secret length to be rejected during generation")
	}

	if _, err := generateSessionTokenV2(
		authMethodOIDC,
		"",
		secret,
		now,
	); err == nil {
		t.Fatal("expected empty principal to be rejected")
	}

	if _, err := generateSessionTokenV2(
		authMethodOIDC,
		strings.Repeat("a", AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH+1),
		secret,
		now,
	); err == nil {
		t.Fatal("expected oversized principal to be rejected")
	}

	tests := []struct {
		name   string
		token  string
		secret []byte
	}{
		{
			name:   "invalid base64",
			token:  "not-base64",
			secret: secret,
		},
		{
			name: "invalid token length",
			token: base64.StdEncoding.EncodeToString(
				make([]byte, AUTH_TOKEN_V2_FIXED_DATA_LENGTH),
			),
			secret: secret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := verifySessionTokenV2(
				tt.token,
				tt.secret,
				now,
			); err == nil {
				t.Fatal("expected V2 token verification to fail")
			}
		})
	}

	validToken, err := generateSessionTokenV2(
		authMethodOIDC,
		"test-principal",
		secret,
		now,
	)
	if err != nil {
		t.Fatalf("generating valid V2 token: %v", err)
	}

	if _, err := verifySessionTokenV2(
		validToken,
		[]byte("short"),
		now,
	); err == nil {
		t.Fatal("expected invalid secret length to be rejected during verification")
	}

	decoded, err := base64.StdEncoding.DecodeString(validToken)
	if err != nil {
		t.Fatalf("decoding valid V2 token: %v", err)
	}

	t.Run("signed invalid magic", func(t *testing.T) {
		modified := append([]byte(nil), decoded...)
		modified[0]++

		dataLength := len(modified) - sha256.Size
		signingKey, err := authTokenV2SigningKey(secret)
		if err != nil {
			t.Fatalf("deriving V2 signing key: %v", err)
		}
		h := hmac.New(sha256.New, signingKey)
		h.Write(modified[:dataLength])
		copy(modified[dataLength:], h.Sum(nil))

		if _, err := verifySessionTokenV2(
			base64.StdEncoding.EncodeToString(modified),
			secret,
			now,
		); err == nil {
			t.Fatal("expected signed token with invalid magic to be rejected")
		}
	})

	t.Run("signed invalid version", func(t *testing.T) {
		modified := append([]byte(nil), decoded...)
		modified[AUTH_TOKEN_V2_MAGIC_LENGTH] = 99

		dataLength := len(modified) - sha256.Size
		signingKey, err := authTokenV2SigningKey(secret)
		if err != nil {
			t.Fatalf("deriving V2 signing key: %v", err)
		}
		h := hmac.New(sha256.New, signingKey)
		h.Write(modified[:dataLength])
		copy(modified[dataLength:], h.Sum(nil))

		if _, err := verifySessionTokenV2(
			base64.StdEncoding.EncodeToString(modified),
			secret,
			now,
		); err == nil {
			t.Fatal("expected signed token with invalid version to be rejected")
		}
	})

	t.Run("signed invalid method", func(t *testing.T) {
		modified := append([]byte(nil), decoded...)
		methodOffset := AUTH_TOKEN_V2_MAGIC_LENGTH + AUTH_TOKEN_V2_VERSION_LENGTH
		modified[methodOffset] = 99

		dataLength := len(modified) - sha256.Size
		signingKey, err := authTokenV2SigningKey(secret)
		if err != nil {
			t.Fatalf("deriving V2 signing key: %v", err)
		}
		h := hmac.New(sha256.New, signingKey)
		h.Write(modified[:dataLength])
		copy(modified[dataLength:], h.Sum(nil))

		if _, err := verifySessionTokenV2(
			base64.StdEncoding.EncodeToString(modified),
			secret,
			now,
		); err == nil {
			t.Fatal("expected signed token with invalid method to be rejected")
		}
	})
}

func TestAuthTokenV2CannotVerifyAsV1AtV1TokenLength(t *testing.T) {
	secret := make([]byte, AUTH_SECRET_KEY_LENGTH)
	now := time.Now()

	principalLength := AUTH_TOKEN_DATA_LENGTH - AUTH_TOKEN_V2_FIXED_DATA_LENGTH
	if principalLength <= 0 {
		t.Fatalf("V2 fixed data length %d cannot produce V1-sized token data", AUTH_TOKEN_V2_FIXED_DATA_LENGTH)
	}

	principal := strings.Repeat("a", principalLength)
	token, err := generateSessionTokenV2(authMethodLocal, principal, secret, now)
	if err != nil {
		t.Fatalf("generating V1-sized V2 token: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding V1-sized V2 token: %v", err)
	}

	wantLength := AUTH_TOKEN_DATA_LENGTH + sha256.Size
	if len(decoded) != wantLength {
		t.Fatalf("V2 token length = %d, want V1 token length %d", len(decoded), wantLength)
	}

	if _, _, err := verifySessionToken(token, secret, now); err == nil {
		t.Fatal("expected V2 token to be rejected by V1 verification")
	}

	verified, err := verifySessionTokenV2(token, secret, now)
	if err != nil {
		t.Fatalf("expected V1-sized V2 token to verify as V2: %v", err)
	}
	if verified.Method != authMethodLocal {
		t.Fatalf("verified method = %d, want %d", verified.Method, authMethodLocal)
	}
	if verified.Principal != principal {
		t.Fatalf("verified principal = %q, want %q", verified.Principal, principal)
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

func TestResolveAuthenticatedSessionV3LocalIdentity(t *testing.T) {
	app := newAuthTestApplication(t)

	token, err := generateSessionTokenV3(
		authMethodLocal,
		"test-user",
		"test-user",
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("generating V3 local session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})

	session, authorized := app.resolveAuthenticatedSession(req, time.Now())
	if !authorized {
		t.Fatal("expected V3 local session to resolve")
	}
	if session.Method != authMethodLocal {
		t.Fatalf("method = %d, want local", session.Method)
	}
	if session.Principal != "test-user" {
		t.Fatalf("principal = %q, want test-user", session.Principal)
	}
	if session.DisplayName != "test-user" {
		t.Fatalf("display name = %q, want test-user", session.DisplayName)
	}
	if session.Version != AUTH_TOKEN_V3_VERSION {
		t.Fatalf(
			"version = %d, want %d",
			session.Version,
			AUTH_TOKEN_V3_VERSION,
		)
	}
}

func TestResolveAuthenticatedSessionV2LocalIdentity(t *testing.T) {
	app := newAuthTestApplication(t)

	token, err := generateSessionTokenV2(
		authMethodLocal,
		"test-user",
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("generating V2 local session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})

	session, authorized := app.resolveAuthenticatedSession(req, time.Now())
	if !authorized {
		t.Fatal("expected V2 local session to resolve")
	}
	if session.DisplayName != "test-user" {
		t.Fatalf("display name = %q, want test-user", session.DisplayName)
	}
	if session.Version != AUTH_TOKEN_V2_VERSION {
		t.Fatalf(
			"version = %d, want %d",
			session.Version,
			AUTH_TOKEN_V2_VERSION,
		)
	}
}

func TestIsAuthorizedAcceptsValidV3LocalSession(t *testing.T) {
	app := newAuthTestApplication(t)

	token, err := generateSessionTokenV3(
		authMethodLocal,
		"test-user",
		"test-user",
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("generating V3 local session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})

	rec := httptest.NewRecorder()

	if !app.isAuthorized(rec, req) {
		t.Fatal("expected valid V3 local session to be authorized")
	}

	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf(
			"expected fresh V3 session not to be regenerated, got %d cookies",
			len(cookies),
		)
	}
}

func TestIsAuthorizedRegeneratesAgingV3LocalSession(t *testing.T) {
	app := newAuthTestApplication(t)

	tokenCreatedAt := time.Now().Add(
		-(AUTH_TOKEN_VALID_PERIOD - AUTH_TOKEN_REGEN_BEFORE + time.Hour),
	)

	token, err := generateSessionTokenV3(
		authMethodLocal,
		"test-user",
		"test-user",
		app.authSecretKey,
		tokenCreatedAt,
	)
	if err != nil {
		t.Fatalf("generating aging V3 local session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})

	rec := httptest.NewRecorder()

	if !app.isAuthorized(rec, req) {
		t.Fatal("expected aging V3 local session to remain authorized")
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf(
			"expected one regenerated V3 session cookie, got %d",
			len(cookies),
		)
	}

	verified, err := verifySessionTokenV3(
		cookies[0].Value,
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("verifying regenerated V3 session: %v", err)
	}
	if verified.Method != authMethodLocal {
		t.Fatalf(
			"regenerated method = %d, want %d",
			verified.Method,
			authMethodLocal,
		)
	}
	if verified.Principal != "test-user" {
		t.Fatalf(
			"regenerated principal = %q, want %q",
			verified.Principal,
			"test-user",
		)
	}
	if verified.DisplayName != "test-user" {
		t.Fatalf(
			"regenerated display name = %q, want %q",
			verified.DisplayName,
			"test-user",
		)
	}
	if verified.ShouldRegen {
		t.Fatal("expected regenerated V3 session to be fresh")
	}
}

func TestIsAuthorizedAcceptsValidV2LocalSession(t *testing.T) {
	app := newAuthTestApplication(t)

	token, err := generateSessionTokenV2(
		authMethodLocal,
		"test-user",
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("generating V2 local session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})

	rec := httptest.NewRecorder()

	if !app.isAuthorized(rec, req) {
		t.Fatal("expected valid V2 local session to be authorized")
	}

	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("expected fresh V2 session not to be regenerated, got %d cookies", len(cookies))
	}
}

func TestIsAuthorizedRejectsUnknownV2LocalAndOIDCSessions(t *testing.T) {
	app := newAuthTestApplication(t)

	tests := []struct {
		name      string
		method    authMethod
		principal string
	}{
		{
			name:      "unknown local user",
			method:    authMethodLocal,
			principal: "unknown-user",
		},
		{
			name:      "OIDC before OIDC is configured",
			method:    authMethodOIDC,
			principal: "https://accounts.example.test|subject-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := generateSessionTokenV2(
				tt.method,
				tt.principal,
				app.authSecretKey,
				time.Now(),
			)
			if err != nil {
				t.Fatalf("generating V2 session: %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.AddCookie(&http.Cookie{
				Name:  AUTH_SESSION_COOKIE_NAME,
				Value: token,
			})

			rec := httptest.NewRecorder()

			if app.isAuthorized(rec, req) {
				t.Fatal("expected V2 session to be unauthorized")
			}
		})
	}
}

func TestIsAuthorizedRegeneratesAgingV2LocalSession(t *testing.T) {
	app := newAuthTestApplication(t)

	tokenCreatedAt := time.Now().Add(
		-(AUTH_TOKEN_VALID_PERIOD - AUTH_TOKEN_REGEN_BEFORE + time.Hour),
	)

	token, err := generateSessionTokenV2(
		authMethodLocal,
		"test-user",
		app.authSecretKey,
		tokenCreatedAt,
	)
	if err != nil {
		t.Fatalf("generating aging V2 local session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})

	rec := httptest.NewRecorder()

	if !app.isAuthorized(rec, req) {
		t.Fatal("expected aging V2 local session to remain authorized")
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one regenerated V2 session cookie, got %d", len(cookies))
	}

	verified, err := verifySessionTokenV2(
		cookies[0].Value,
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("verifying regenerated V2 session: %v", err)
	}
	if verified.Method != authMethodLocal {
		t.Fatalf("regenerated method = %d, want %d", verified.Method, authMethodLocal)
	}
	if verified.Principal != "test-user" {
		t.Fatalf("regenerated principal = %q, want %q", verified.Principal, "test-user")
	}
	if verified.ShouldRegen {
		t.Fatal("expected regenerated V2 session to be fresh")
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
	tests := []struct {
		name           string
		proxied        bool
		trustedProxies []string
		remoteAddr     string
		proto          string
		directTLS      bool
		wantSecure     bool
	}{
		{
			name:       "plain HTTP",
			remoteAddr: "192.0.2.10:1234",
		},
		{
			name:       "untrusted forwarded HTTPS",
			remoteAddr: "192.0.2.10:1234",
			proto:      "https",
			wantSecure: false,
		},
		{
			name:       "legacy proxied forwarded HTTPS",
			proxied:    true,
			remoteAddr: "192.0.2.10:1234",
			proto:      "https",
			wantSecure: true,
		},
		{
			name:           "trusted proxy forwarded HTTPS",
			proxied:        true,
			trustedProxies: []string{"192.0.2.0/24"},
			remoteAddr:     "192.0.2.10:1234",
			proto:          "https",
			wantSecure:     true,
		},
		{
			name:           "untrusted peer forwarded HTTPS",
			proxied:        true,
			trustedProxies: []string{"192.0.2.0/24"},
			remoteAddr:     "203.0.113.10:1234",
			proto:          "https",
			wantSecure:     false,
		},
		{
			name:       "direct TLS",
			remoteAddr: "192.0.2.10:1234",
			directTLS:  true,
			wantSecure: true,
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

			scheme := "http"
			if tt.directTLS {
				scheme = "https"
			}
			req := httptest.NewRequest(http.MethodGet, scheme+"://example.test/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}

			rec := httptest.NewRecorder()
			expires := time.Now().Add(time.Hour)
			app.setAuthSessionCookie(rec, req, "test-token", expires)

			cookies := rec.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("expected one cookie, got %d", len(cookies))
			}

			cookie := cookies[0]
			if cookie.Name != AUTH_SESSION_COOKIE_NAME {
				t.Fatalf("cookie name = %q, want %q", cookie.Name, AUTH_SESSION_COOKIE_NAME)
			}
			if cookie.Value != "test-token" {
				t.Fatalf("cookie value = %q, want %q", cookie.Value, "test-token")
			}
			if cookie.Path != "/glance/" {
				t.Fatalf("cookie path = %q, want %q", cookie.Path, "/glance/")
			}
			if cookie.Secure != tt.wantSecure {
				t.Fatalf("cookie Secure = %v, want %v", cookie.Secure, tt.wantSecure)
			}
			if !cookie.HttpOnly {
				t.Fatal("expected cookie to be HttpOnly")
			}
			if cookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("cookie SameSite = %v, want %v", cookie.SameSite, http.SameSiteLaxMode)
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
	req.TLS = &tls.ConnectionState{}
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

	verifiedSession, err := verifySessionTokenV3(
		cookie.Value,
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("successful local login did not issue a valid V3 session: %v", err)
	}
	if verifiedSession.Method != authMethodLocal {
		t.Fatalf(
			"session method = %d, want %d",
			verifiedSession.Method,
			authMethodLocal,
		)
	}
	if verifiedSession.Principal != "test-user" {
		t.Fatalf(
			"session principal = %q, want %q",
			verifiedSession.Principal,
			"test-user",
		)
	}
	if verifiedSession.DisplayName != "test-user" {
		t.Fatalf(
			"session display name = %q, want %q",
			verifiedSession.DisplayName,
			"test-user",
		)
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
