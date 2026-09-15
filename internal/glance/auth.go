package glance

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	mathrand "math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const AUTH_SESSION_COOKIE_NAME = "session_token"
const AUTH_RATE_LIMIT_WINDOW = 5 * time.Minute
const AUTH_RATE_LIMIT_MAX_ATTEMPTS = 5

const AUTH_TOKEN_SECRET_LENGTH = 32
const AUTH_USERNAME_HASH_LENGTH = 32
const AUTH_SECRET_KEY_LENGTH = AUTH_TOKEN_SECRET_LENGTH + AUTH_USERNAME_HASH_LENGTH
const AUTH_TIMESTAMP_LENGTH = 4 // uint32
const AUTH_TOKEN_DATA_LENGTH = AUTH_USERNAME_HASH_LENGTH + AUTH_TIMESTAMP_LENGTH

const AUTH_TOKEN_V2_MAGIC = "GATK"
const AUTH_TOKEN_V2_MAGIC_LENGTH = len(AUTH_TOKEN_V2_MAGIC)
const AUTH_TOKEN_V2_SIGNING_DOMAIN = "glance-session-v2"
const AUTH_TOKEN_V2_VERSION byte = 2
const AUTH_TOKEN_V2_VERSION_LENGTH = 1
const AUTH_TOKEN_V2_METHOD_LENGTH = 1
const AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH = 2 // uint16
const AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH = 2048
const AUTH_TOKEN_V2_FIXED_DATA_LENGTH = AUTH_TOKEN_V2_MAGIC_LENGTH +
	AUTH_TOKEN_V2_VERSION_LENGTH +
	AUTH_TOKEN_V2_METHOD_LENGTH +
	AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH +
	AUTH_TIMESTAMP_LENGTH

const AUTH_TOKEN_V3_SIGNING_DOMAIN = "glance-session-v3"
const AUTH_TOKEN_V3_VERSION byte = 3
const AUTH_TOKEN_V3_DISPLAY_LENGTH_LENGTH = 2 // uint16
const AUTH_TOKEN_V3_MAX_DISPLAY_LENGTH = 256
const AUTH_TOKEN_V3_FIXED_DATA_LENGTH = AUTH_TOKEN_V2_MAGIC_LENGTH +
	AUTH_TOKEN_V2_VERSION_LENGTH +
	AUTH_TOKEN_V2_METHOD_LENGTH +
	AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH +
	AUTH_TOKEN_V3_DISPLAY_LENGTH_LENGTH +
	AUTH_TIMESTAMP_LENGTH

type authMethod byte

const (
	authMethodLocal authMethod = 1
	authMethodOIDC  authMethod = 2
)

type verifiedSessionTokenV2 struct {
	Method      authMethod
	Principal   string
	ShouldRegen bool
}

type verifiedSessionTokenV3 struct {
	Method      authMethod
	Principal   string
	DisplayName string
	ShouldRegen bool
}

type oidcPrincipal struct {
	Issuer  string
	Subject string
}

func encodeOIDCPrincipal(issuer string, subject string) (string, error) {
	issuerBytes := []byte(issuer)
	subjectBytes := []byte(subject)

	if len(issuerBytes) == 0 {
		return "", fmt.Errorf("OIDC issuer is empty")
	}
	if len(subjectBytes) == 0 {
		return "", fmt.Errorf("OIDC subject is empty")
	}
	if len(issuerBytes) > int(^uint16(0)) {
		return "", fmt.Errorf("OIDC issuer is too long")
	}

	data := make([]byte, 2+len(issuerBytes)+len(subjectBytes))
	binary.BigEndian.PutUint16(data[:2], uint16(len(issuerBytes)))
	copy(data[2:], issuerBytes)
	copy(data[2+len(issuerBytes):], subjectBytes)

	encoded := base64.RawURLEncoding.EncodeToString(data)
	if len([]byte(encoded)) > AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH {
		return "", fmt.Errorf("OIDC principal length exceeds %d bytes", AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH)
	}

	return encoded, nil
}

func decodeOIDCPrincipal(encoded string) (oidcPrincipal, error) {
	var principal oidcPrincipal

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return principal, fmt.Errorf("decoding OIDC principal: %w", err)
	}
	if len(data) < 4 {
		return principal, fmt.Errorf("OIDC principal is too short")
	}

	issuerLength := int(binary.BigEndian.Uint16(data[:2]))
	if issuerLength == 0 {
		return principal, fmt.Errorf("OIDC issuer is empty")
	}
	if len(data) <= 2+issuerLength {
		return principal, fmt.Errorf("OIDC principal length is invalid")
	}

	principal.Issuer = string(data[2 : 2+issuerLength])
	principal.Subject = string(data[2+issuerLength:])

	if principal.Subject == "" {
		return oidcPrincipal{}, fmt.Errorf("OIDC subject is empty")
	}

	return principal, nil
}

// How long the token will be valid for
const AUTH_TOKEN_VALID_PERIOD = 14 * 24 * time.Hour // 14 days
// How long the token has left before it should be regenerated
const AUTH_TOKEN_REGEN_BEFORE = 7 * 24 * time.Hour // 7 days

var loginPageTemplate = mustParseTemplate("login.html", "document.html", "footer.html")

type doWhenUnauthorized int

const (
	redirectToLogin doWhenUnauthorized = iota
	showUnauthorizedJSON
)

type failedAuthAttempt struct {
	attempts int
	first    time.Time
}

func generateSessionToken(username string, secret []byte, now time.Time) (string, error) {
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return "", fmt.Errorf("secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}

	usernameHash, err := computeUsernameHash(username, secret)
	if err != nil {
		return "", err
	}

	data := make([]byte, AUTH_TOKEN_DATA_LENGTH)
	copy(data, usernameHash)
	expires := now.Add(AUTH_TOKEN_VALID_PERIOD).Unix()
	binary.LittleEndian.PutUint32(data[AUTH_USERNAME_HASH_LENGTH:], uint32(expires))

	h := hmac.New(sha256.New, secret[0:AUTH_TOKEN_SECRET_LENGTH])
	h.Write(data)

	signature := h.Sum(nil)
	encodedToken := base64.StdEncoding.EncodeToString(append(data, signature...))
	// encodedToken ends up being (hashed username + expiration timestamp + signature) encoded as base64

	return encodedToken, nil
}

func computeUsernameHash(username string, secret []byte) ([]byte, error) {
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return nil, fmt.Errorf("secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}

	h := hmac.New(sha256.New, secret[AUTH_TOKEN_SECRET_LENGTH:])
	h.Write([]byte(username))

	return h.Sum(nil), nil
}

func authTokenV2SigningKey(secret []byte) ([]byte, error) {
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return nil, fmt.Errorf("secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}

	h := hmac.New(sha256.New, secret[:AUTH_TOKEN_SECRET_LENGTH])
	h.Write([]byte(AUTH_TOKEN_V2_SIGNING_DOMAIN))

	return h.Sum(nil), nil
}

func generateSessionTokenV2(method authMethod, principal string, secret []byte, now time.Time) (string, error) {
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return "", fmt.Errorf("secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}
	if method != authMethodLocal && method != authMethodOIDC {
		return "", fmt.Errorf("authentication method is invalid")
	}

	principalBytes := []byte(principal)
	if len(principalBytes) == 0 {
		return "", fmt.Errorf("principal is empty")
	}
	if len(principalBytes) > AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH {
		return "", fmt.Errorf("principal length exceeds %d bytes", AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH)
	}

	dataLength := AUTH_TOKEN_V2_FIXED_DATA_LENGTH + len(principalBytes)
	data := make([]byte, dataLength)

	offset := 0
	copy(data[offset:], AUTH_TOKEN_V2_MAGIC)
	offset += AUTH_TOKEN_V2_MAGIC_LENGTH

	data[offset] = AUTH_TOKEN_V2_VERSION
	offset += AUTH_TOKEN_V2_VERSION_LENGTH

	data[offset] = byte(method)
	offset += AUTH_TOKEN_V2_METHOD_LENGTH

	binary.LittleEndian.PutUint16(
		data[offset:offset+AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH],
		uint16(len(principalBytes)),
	)
	offset += AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH

	principalOffset := offset
	copy(data[principalOffset:], principalBytes)

	timestampOffset := principalOffset + len(principalBytes)
	expires := now.Add(AUTH_TOKEN_VALID_PERIOD).Unix()
	binary.LittleEndian.PutUint32(
		data[timestampOffset:timestampOffset+AUTH_TIMESTAMP_LENGTH],
		uint32(expires),
	)

	signingKey, err := authTokenV2SigningKey(secret)
	if err != nil {
		return "", err
	}

	h := hmac.New(sha256.New, signingKey)
	h.Write(data)

	return base64.StdEncoding.EncodeToString(append(data, h.Sum(nil)...)), nil
}

func authTokenV3SigningKey(secret []byte) ([]byte, error) {
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return nil, fmt.Errorf("secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}

	h := hmac.New(sha256.New, secret[:AUTH_TOKEN_SECRET_LENGTH])
	h.Write([]byte(AUTH_TOKEN_V3_SIGNING_DOMAIN))

	return h.Sum(nil), nil
}

func generateSessionTokenV3(
	method authMethod,
	principal string,
	displayName string,
	secret []byte,
	now time.Time,
) (string, error) {
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return "", fmt.Errorf("secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}
	if method != authMethodLocal && method != authMethodOIDC {
		return "", fmt.Errorf("authentication method is invalid")
	}

	principalBytes := []byte(principal)
	displayNameBytes := []byte(displayName)

	if len(principalBytes) == 0 {
		return "", fmt.Errorf("principal is empty")
	}
	if len(principalBytes) > AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH {
		return "", fmt.Errorf(
			"principal length exceeds %d bytes",
			AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH,
		)
	}
	if len(displayNameBytes) > AUTH_TOKEN_V3_MAX_DISPLAY_LENGTH {
		return "", fmt.Errorf(
			"display name length exceeds %d bytes",
			AUTH_TOKEN_V3_MAX_DISPLAY_LENGTH,
		)
	}

	dataLength := AUTH_TOKEN_V3_FIXED_DATA_LENGTH +
		len(principalBytes) +
		len(displayNameBytes)
	data := make([]byte, dataLength)

	offset := 0
	copy(data[offset:], AUTH_TOKEN_V2_MAGIC)
	offset += AUTH_TOKEN_V2_MAGIC_LENGTH

	data[offset] = AUTH_TOKEN_V3_VERSION
	offset += AUTH_TOKEN_V2_VERSION_LENGTH

	data[offset] = byte(method)
	offset += AUTH_TOKEN_V2_METHOD_LENGTH

	binary.LittleEndian.PutUint16(
		data[offset:offset+AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH],
		uint16(len(principalBytes)),
	)
	offset += AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH

	binary.LittleEndian.PutUint16(
		data[offset:offset+AUTH_TOKEN_V3_DISPLAY_LENGTH_LENGTH],
		uint16(len(displayNameBytes)),
	)
	offset += AUTH_TOKEN_V3_DISPLAY_LENGTH_LENGTH

	copy(data[offset:], principalBytes)
	offset += len(principalBytes)

	copy(data[offset:], displayNameBytes)
	offset += len(displayNameBytes)

	binary.LittleEndian.PutUint32(
		data[offset:offset+AUTH_TIMESTAMP_LENGTH],
		uint32(now.Add(AUTH_TOKEN_VALID_PERIOD).Unix()),
	)

	signingKey, err := authTokenV3SigningKey(secret)
	if err != nil {
		return "", err
	}

	h := hmac.New(sha256.New, signingKey)
	h.Write(data)

	return base64.StdEncoding.EncodeToString(
		append(data, h.Sum(nil)...),
	), nil
}

func verifySessionTokenV3(
	token string,
	secret []byte,
	now time.Time,
) (verifiedSessionTokenV3, error) {
	var verified verifiedSessionTokenV3

	tokenBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return verified, err
	}
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return verified, fmt.Errorf(
			"secret key length is not %d bytes",
			AUTH_SECRET_KEY_LENGTH,
		)
	}

	minimumTokenLength := AUTH_TOKEN_V3_FIXED_DATA_LENGTH + 1 + sha256.Size
	if len(tokenBytes) < minimumTokenLength {
		return verified, fmt.Errorf("token length is invalid")
	}

	offset := 0

	if string(tokenBytes[offset:offset+AUTH_TOKEN_V2_MAGIC_LENGTH]) != AUTH_TOKEN_V2_MAGIC {
		return verified, fmt.Errorf("token magic is invalid")
	}
	offset += AUTH_TOKEN_V2_MAGIC_LENGTH

	if tokenBytes[offset] != AUTH_TOKEN_V3_VERSION {
		return verified, fmt.Errorf("token version is invalid")
	}
	offset += AUTH_TOKEN_V2_VERSION_LENGTH

	method := authMethod(tokenBytes[offset])
	if method != authMethodLocal && method != authMethodOIDC {
		return verified, fmt.Errorf("authentication method is invalid")
	}
	offset += AUTH_TOKEN_V2_METHOD_LENGTH

	principalLength := int(binary.LittleEndian.Uint16(
		tokenBytes[offset : offset+AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH],
	))
	offset += AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH

	displayNameLength := int(binary.LittleEndian.Uint16(
		tokenBytes[offset : offset+AUTH_TOKEN_V3_DISPLAY_LENGTH_LENGTH],
	))
	offset += AUTH_TOKEN_V3_DISPLAY_LENGTH_LENGTH

	if principalLength == 0 ||
		principalLength > AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH {
		return verified, fmt.Errorf("principal length is invalid")
	}
	if displayNameLength > AUTH_TOKEN_V3_MAX_DISPLAY_LENGTH {
		return verified, fmt.Errorf("display name length is invalid")
	}

	dataLength := AUTH_TOKEN_V3_FIXED_DATA_LENGTH +
		principalLength +
		displayNameLength

	if len(tokenBytes) != dataLength+sha256.Size {
		return verified, fmt.Errorf("token length is invalid")
	}

	data := tokenBytes[:dataLength]
	providedSignature := tokenBytes[dataLength:]

	signingKey, err := authTokenV3SigningKey(secret)
	if err != nil {
		return verified, err
	}

	h := hmac.New(sha256.New, signingKey)
	h.Write(data)

	if !hmac.Equal(h.Sum(nil), providedSignature) {
		return verified, fmt.Errorf("signature does not match")
	}

	principalOffset := offset
	displayNameOffset := principalOffset + principalLength
	timestampOffset := displayNameOffset + displayNameLength

	expiresTimestamp := int64(binary.LittleEndian.Uint32(
		data[timestampOffset : timestampOffset+AUTH_TIMESTAMP_LENGTH],
	))
	if now.Unix() > expiresTimestamp {
		return verified, fmt.Errorf("token has expired")
	}

	verified.Method = method
	verified.Principal = string(data[principalOffset:displayNameOffset])
	verified.DisplayName = string(data[displayNameOffset:timestampOffset])
	verified.ShouldRegen = time.Unix(expiresTimestamp, 0).
		Add(-AUTH_TOKEN_REGEN_BEFORE).
		Before(now)

	return verified, nil
}

func verifySessionTokenV2(token string, secret []byte, now time.Time) (verifiedSessionTokenV2, error) {
	var verified verifiedSessionTokenV2

	tokenBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return verified, err
	}
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return verified, fmt.Errorf("secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}

	minimumTokenLength := AUTH_TOKEN_V2_FIXED_DATA_LENGTH + 1 + sha256.Size
	if len(tokenBytes) < minimumTokenLength {
		return verified, fmt.Errorf("token length is invalid")
	}

	offset := 0
	if string(tokenBytes[offset:offset+AUTH_TOKEN_V2_MAGIC_LENGTH]) != AUTH_TOKEN_V2_MAGIC {
		return verified, fmt.Errorf("token magic is invalid")
	}
	offset += AUTH_TOKEN_V2_MAGIC_LENGTH

	if tokenBytes[offset] != AUTH_TOKEN_V2_VERSION {
		return verified, fmt.Errorf("token version is invalid")
	}
	offset += AUTH_TOKEN_V2_VERSION_LENGTH

	method := authMethod(tokenBytes[offset])
	if method != authMethodLocal && method != authMethodOIDC {
		return verified, fmt.Errorf("authentication method is invalid")
	}
	offset += AUTH_TOKEN_V2_METHOD_LENGTH

	principalLength := int(binary.LittleEndian.Uint16(
		tokenBytes[offset : offset+AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH],
	))
	offset += AUTH_TOKEN_V2_PRINCIPAL_LENGTH_LENGTH
	if principalLength == 0 {
		return verified, fmt.Errorf("principal is empty")
	}
	if principalLength > AUTH_TOKEN_V2_MAX_PRINCIPAL_LENGTH {
		return verified, fmt.Errorf("principal length is invalid")
	}

	dataLength := AUTH_TOKEN_V2_FIXED_DATA_LENGTH + principalLength
	if len(tokenBytes) != dataLength+sha256.Size {
		return verified, fmt.Errorf("token length is invalid")
	}

	data := tokenBytes[:dataLength]
	providedSignature := tokenBytes[dataLength:]

	signingKey, err := authTokenV2SigningKey(secret)
	if err != nil {
		return verified, err
	}

	h := hmac.New(sha256.New, signingKey)
	h.Write(data)
	if !hmac.Equal(h.Sum(nil), providedSignature) {
		return verified, fmt.Errorf("signature does not match")
	}

	principalOffset := offset
	timestampOffset := principalOffset + principalLength
	expiresTimestamp := int64(binary.LittleEndian.Uint32(
		data[timestampOffset : timestampOffset+AUTH_TIMESTAMP_LENGTH],
	))
	if now.Unix() > expiresTimestamp {
		return verified, fmt.Errorf("token has expired")
	}

	verified.Method = method
	verified.Principal = string(data[principalOffset:timestampOffset])
	verified.ShouldRegen = time.Unix(expiresTimestamp, 0).
		Add(-AUTH_TOKEN_REGEN_BEFORE).
		Before(now)

	return verified, nil
}

func verifySessionToken(token string, secretBytes []byte, now time.Time) ([]byte, bool, error) {
	tokenBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, false, err
	}

	if len(tokenBytes) != AUTH_TOKEN_DATA_LENGTH+32 {
		return nil, false, fmt.Errorf("token length is invalid")
	}

	if len(secretBytes) != AUTH_SECRET_KEY_LENGTH {
		return nil, false, fmt.Errorf("secret key length is not %d bytes", AUTH_SECRET_KEY_LENGTH)
	}

	usernameHashBytes := tokenBytes[0:AUTH_USERNAME_HASH_LENGTH]
	timestampBytes := tokenBytes[AUTH_USERNAME_HASH_LENGTH : AUTH_USERNAME_HASH_LENGTH+AUTH_TIMESTAMP_LENGTH]
	providedSignatureBytes := tokenBytes[AUTH_TOKEN_DATA_LENGTH:]

	h := hmac.New(sha256.New, secretBytes[0:32])
	h.Write(tokenBytes[0:AUTH_TOKEN_DATA_LENGTH])
	expectedSignatureBytes := h.Sum(nil)

	if !hmac.Equal(expectedSignatureBytes, providedSignatureBytes) {
		return nil, false, fmt.Errorf("signature does not match")
	}

	expiresTimestamp := int64(binary.LittleEndian.Uint32(timestampBytes))
	if now.Unix() > expiresTimestamp {
		return nil, false, fmt.Errorf("token has expired")
	}

	return usernameHashBytes,
		// True if the token should be regenerated
		time.Unix(expiresTimestamp, 0).Add(-AUTH_TOKEN_REGEN_BEFORE).Before(now),
		nil
}

func makeAuthSecretKey(length int) (string, error) {
	key := make([]byte, length)
	_, err := rand.Read(key)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func (a *application) handleAuthenticationAttempt(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	waitOnFailure := 1*time.Second - time.Duration(mathrand.IntN(500))*time.Millisecond

	ip := a.addressOfRequest(r)

	a.authAttemptsMu.Lock()
	exceededRateLimit, retryAfter := func() (bool, int) {
		attempt, exists := a.failedAuthAttempts[ip]
		if !exists {
			a.failedAuthAttempts[ip] = &failedAuthAttempt{
				attempts: 1,
				first:    time.Now(),
			}

			return false, 0
		}

		elapsed := time.Since(attempt.first)
		if elapsed < AUTH_RATE_LIMIT_WINDOW && attempt.attempts >= AUTH_RATE_LIMIT_MAX_ATTEMPTS {
			return true, max(1, int(AUTH_RATE_LIMIT_WINDOW.Seconds()-elapsed.Seconds()))
		}

		attempt.attempts++
		return false, 0
	}()

	if exceededRateLimit {
		a.authAttemptsMu.Unlock()
		time.Sleep(waitOnFailure)
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		w.WriteHeader(http.StatusTooManyRequests)
		return
	} else {
		// Clean up old failed attempts
		for ipOfAttempt := range a.failedAuthAttempts {
			if time.Since(a.failedAuthAttempts[ipOfAttempt].first) > AUTH_RATE_LIMIT_WINDOW {
				delete(a.failedAuthAttempts, ipOfAttempt)
			}
		}
		a.authAttemptsMu.Unlock()
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512*1024))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	err = json.Unmarshal(body, &creds)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logAuthFailure := func() {
		slog.Warn(
			"Failed login attempt",
			"client_ip", ip,
		)
	}

	if len(creds.Username) == 0 || len(creds.Password) == 0 {
		time.Sleep(waitOnFailure)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if len(creds.Username) > 50 || len(creds.Password) > 100 {
		logAuthFailure()
		time.Sleep(waitOnFailure)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	u, exists := a.Config.Auth.Users[creds.Username]
	if !exists {
		logAuthFailure()
		time.Sleep(waitOnFailure)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(creds.Password)); err != nil {
		logAuthFailure()
		time.Sleep(waitOnFailure)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	token, err := generateSessionTokenV3(
		authMethodLocal,
		creds.Username,
		creds.Username,
		a.authSecretKey,
		time.Now(),
	)
	if err != nil {
		slog.Error("Could not compute session token during login attempt", "error", err)
		time.Sleep(waitOnFailure)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	a.setAuthSessionCookie(w, r, token, time.Now().Add(AUTH_TOKEN_VALID_PERIOD))

	a.authAttemptsMu.Lock()
	delete(a.failedAuthAttempts, ip)
	a.authAttemptsMu.Unlock()

	w.WriteHeader(http.StatusOK)
}

type authenticatedSession struct {
	Method           authMethod
	Principal        string
	DisplayName      string
	ShouldRegenerate bool
	Version          byte
}

func (a *application) resolveAuthenticatedSession(
	r *http.Request,
	now time.Time,
) (authenticatedSession, bool) {
	var session authenticatedSession

	if !a.RequiresAuth {
		return session, true
	}

	token, err := r.Cookie(AUTH_SESSION_COOKIE_NAME)
	if err != nil || token.Value == "" {
		return session, false
	}

	if verified, err := verifySessionTokenV3(
		token.Value,
		a.authSecretKey,
		now,
	); err == nil {
		switch verified.Method {
		case authMethodLocal:
			if _, exists := a.Config.Auth.Users[verified.Principal]; !exists {
				return session, false
			}

		case authMethodOIDC:
			if a.oidc == nil {
				return session, false
			}

			principal, err := decodeOIDCPrincipal(verified.Principal)
			if err != nil || principal.Issuer != a.oidc.issuer {
				return session, false
			}

		default:
			return session, false
		}

		return authenticatedSession{
			Method:           verified.Method,
			Principal:        verified.Principal,
			DisplayName:      verified.DisplayName,
			ShouldRegenerate: verified.ShouldRegen,
			Version:          AUTH_TOKEN_V3_VERSION,
		}, true
	}

	if verified, err := verifySessionTokenV2(
		token.Value,
		a.authSecretKey,
		now,
	); err == nil {
		switch verified.Method {
		case authMethodLocal:
			if _, exists := a.Config.Auth.Users[verified.Principal]; !exists {
				return session, false
			}

		case authMethodOIDC:
			if a.oidc == nil {
				return session, false
			}

			principal, err := decodeOIDCPrincipal(verified.Principal)
			if err != nil || principal.Issuer != a.oidc.issuer {
				return session, false
			}

		default:
			return session, false
		}

		displayName := ""
		if verified.Method == authMethodLocal {
			displayName = verified.Principal
		}

		return authenticatedSession{
			Method:           verified.Method,
			Principal:        verified.Principal,
			DisplayName:      displayName,
			ShouldRegenerate: verified.ShouldRegen,
			Version:          AUTH_TOKEN_V2_VERSION,
		}, true
	}

	usernameHash, shouldRegenerate, err := verifySessionToken(
		token.Value,
		a.authSecretKey,
		now,
	)
	if err != nil {
		return session, false
	}

	username, exists := a.usernameHashToUsername[string(usernameHash)]
	if !exists {
		return session, false
	}

	if _, exists := a.Config.Auth.Users[username]; !exists {
		return session, false
	}

	return authenticatedSession{
		Method:           authMethodLocal,
		Principal:        username,
		DisplayName:      username,
		ShouldRegenerate: shouldRegenerate,
		Version:          1,
	}, true
}

func (a *application) authorizeSession(w http.ResponseWriter, r *http.Request) (authenticatedSession, bool) {
	if !a.RequiresAuth {
		return authenticatedSession{}, true
	}

	now := time.Now()

	session, authorized := a.resolveAuthenticatedSession(r, now)
	if !authorized {
		return authenticatedSession{}, false
	}

	if !session.ShouldRegenerate {
		return session, true
	}

	var newToken string
	var err error

	switch session.Version {
	case AUTH_TOKEN_V3_VERSION:
		newToken, err = generateSessionTokenV3(
			session.Method,
			session.Principal,
			session.DisplayName,
			a.authSecretKey,
			now,
		)

	case AUTH_TOKEN_V2_VERSION:
		newToken, err = generateSessionTokenV2(
			session.Method,
			session.Principal,
			a.authSecretKey,
			now,
		)

	default:
		if session.Method != authMethodLocal {
			return authenticatedSession{}, false
		}

		newToken, err = generateSessionToken(
			session.Principal,
			a.authSecretKey,
			now,
		)
	}

	if err != nil {
		slog.Error(
			"Could not compute session token during regeneration",
			"version",
			session.Version,
			"error",
			err,
		)
		return authenticatedSession{}, false
	}

	a.setAuthSessionCookie(
		w,
		r,
		newToken,
		now.Add(AUTH_TOKEN_VALID_PERIOD),
	)

	session.ShouldRegenerate = false
	return session, true
}

func (a *application) isAuthorized(w http.ResponseWriter, r *http.Request) bool {
	_, authorized := a.authorizeSession(w, r)
	return authorized
}

// Handles sending the appropriate response for an unauthorized request and returns true if the request was unauthorized
func (a *application) handleUnauthorizedResponse(w http.ResponseWriter, r *http.Request, fallback doWhenUnauthorized) bool {
	if a.isAuthorized(w, r) {
		return false
	}

	switch fallback {
	case redirectToLogin:
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
	case showUnauthorizedJSON:
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "Unauthorized"}`))
	}

	return true
}

// Maybe this should be a POST request instead?
func (a *application) handleLogoutRequest(w http.ResponseWriter, r *http.Request) {
	a.setAuthSessionCookie(w, r, "", time.Now().Add(-1*time.Hour))
	http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
}

func (a *application) setAuthSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     AUTH_SESSION_COOKIE_NAME,
		Value:    token,
		Expires:  expires,
		Secure:   a.requestIsSecure(r),
		Path:     a.Config.Server.BaseURL + "/",
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})
}

func (a *application) handleLoginPageRequest(w http.ResponseWriter, r *http.Request) {
	if a.isAuthorized(w, r) {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/", http.StatusSeeOther)
		return
	}

	data := &templateData{
		App: a,
	}
	a.populateTemplateRequestData(&data.Request, r, nil)

	if r.URL.Query().Get("reason") == "oidc_not_authorized" {
		data.Request.LoginMessage = "This account is not authorized to access Glance."
	}

	var responseBytes bytes.Buffer
	err := loginPageTemplate.Execute(&responseBytes, data)
	if err != nil {
		writeInternalServerError(w, "Failed to render login page", err)
		return
	}

	_, _ = w.Write(responseBytes.Bytes())
}
