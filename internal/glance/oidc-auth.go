package glance

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	OIDC_FLOW_COOKIE_NAME       = "oidc_flow"
	OIDC_FLOW_VALID_PERIOD      = 10 * time.Minute
	OIDC_FLOW_RANDOM_BYTES      = 32
	OIDC_FLOW_ENCRYPTION_DOMAIN = "glance-oidc-flow-v1"
	OIDC_CALLBACK_PATH          = "/auth/oidc/callback"
)

type oidcRuntime struct {
	issuer      string
	provider    *oidc.Provider
	verifier    *oidc.IDTokenVerifier
	oauthConfig *oauth2.Config
}

type oidcFlow struct {
	State        string `json:"state"`
	Nonce        string `json:"nonce"`
	PKCEVerifier string `json:"pkce_verifier"`
	RedirectURL  string `json:"redirect_url"`
	Expires      int64  `json:"expires"`
}

func newOIDCRuntime(config oidcConfig, reusable *oidcRuntime) (*oidcRuntime, error) {
	var provider *oidc.Provider

	// Discovery belongs to the application generation, but the provider can
	// safely be reused when a reload retains the same issuer. This preserves
	// the provider discovery state and JWKS cache while allowing client
	// credentials and other generation-specific configuration to be rebuilt.
	if reusable != nil &&
		reusable.provider != nil &&
		reusable.issuer == config.Issuer {
		provider = reusable.provider
	}

	if provider == nil {
		client := newHTTPClient(0, false)

		ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
		defer cancel()

		ctx = oidc.ClientContext(ctx, client)

		var err error
		provider, err = oidc.NewProvider(ctx, config.Issuer)
		if err != nil {
			return nil, fmt.Errorf(
				"discovering OIDC provider %q: %w",
				config.Issuer,
				err,
			)
		}
	}

	return &oidcRuntime{
		issuer:   config.Issuer,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{
			ClientID: config.ClientID,
		}),
		oauthConfig: &oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes: []string{
				oidc.ScopeOpenID,
				"email",
			},
		},
	}, nil
}

func makeOIDCRandomValue() (string, error) {
	value := make([]byte, OIDC_FLOW_RANDOM_BYTES)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generating OIDC random value: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(value), nil
}

func oidcPKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func oidcFlowEncryptionKey(secret []byte) ([]byte, error) {
	if len(secret) != AUTH_SECRET_KEY_LENGTH {
		return nil, fmt.Errorf(
			"secret key length is not %d bytes",
			AUTH_SECRET_KEY_LENGTH,
		)
	}

	h := hmac.New(sha256.New, secret[:AUTH_TOKEN_SECRET_LENGTH])
	h.Write([]byte(OIDC_FLOW_ENCRYPTION_DOMAIN))
	return h.Sum(nil), nil
}

func encryptOIDCFlow(flow oidcFlow, secret []byte) (string, error) {
	plaintext, err := json.Marshal(flow)
	if err != nil {
		return "", fmt.Errorf("encoding OIDC flow: %w", err)
	}

	key, err := oidcFlowEncryptionKey(secret)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating OIDC flow cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating OIDC flow AEAD: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating OIDC flow encryption nonce: %w", err)
	}

	sealed := aead.Seal(nil, nonce, plaintext, nil)
	payload := append(nonce, sealed...)

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decryptOIDCFlow(encoded string, secret []byte, now time.Time) (oidcFlow, error) {
	var flow oidcFlow

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return flow, fmt.Errorf("decoding OIDC flow: %w", err)
	}

	key, err := oidcFlowEncryptionKey(secret)
	if err != nil {
		return flow, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return flow, fmt.Errorf("creating OIDC flow cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return flow, fmt.Errorf("creating OIDC flow AEAD: %w", err)
	}

	if len(payload) < aead.NonceSize()+aead.Overhead() {
		return flow, fmt.Errorf("OIDC flow payload is too short")
	}

	nonce := payload[:aead.NonceSize()]
	ciphertext := payload[aead.NonceSize():]

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return flow, fmt.Errorf("decrypting OIDC flow: %w", err)
	}

	if err := json.Unmarshal(plaintext, &flow); err != nil {
		return flow, fmt.Errorf("decoding OIDC flow data: %w", err)
	}

	if flow.State == "" ||
		flow.Nonce == "" ||
		flow.PKCEVerifier == "" ||
		flow.RedirectURL == "" {
		return oidcFlow{}, fmt.Errorf("OIDC flow data is incomplete")
	}

	if flow.Expires == 0 || now.Unix() > flow.Expires {
		return oidcFlow{}, fmt.Errorf("OIDC flow has expired")
	}

	return flow, nil
}

func (a *application) oidcRedirectURL(r *http.Request) string {
	if a.Config.Auth.OIDC.RedirectURL != "" {
		return a.Config.Auth.OIDC.RedirectURL
	}

	scheme := "http"
	if a.requestIsSecure(r) {
		scheme = "https"
	}

	return scheme + "://" + r.Host + a.Config.Server.BaseURL + OIDC_CALLBACK_PATH
}

func (a *application) setOIDCFlowCookie(
	w http.ResponseWriter,
	r *http.Request,
	value string,
	expires time.Time,
) {
	http.SetCookie(w, &http.Cookie{
		Name:     OIDC_FLOW_COOKIE_NAME,
		Value:    value,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		Secure:   a.requestIsSecure(r),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     a.Config.Server.BaseURL + OIDC_CALLBACK_PATH,
	})
}

func (a *application) clearOIDCFlowCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     OIDC_FLOW_COOKIE_NAME,
		Value:    "",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		Secure:   a.requestIsSecure(r),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     a.Config.Server.BaseURL + OIDC_CALLBACK_PATH,
	})
}

func (a *application) handleOIDCLoginRequest(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.NotFound(w, r)
		return
	}

	if a.isAuthorized(w, r) {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/", http.StatusSeeOther)
		return
	}

	state, err := makeOIDCRandomValue()
	if err != nil {
		writeInternalServerError(w, "Failed to start OIDC authentication", err)
		return
	}

	nonce, err := makeOIDCRandomValue()
	if err != nil {
		writeInternalServerError(w, "Failed to start OIDC authentication", err)
		return
	}

	pkceVerifier, err := makeOIDCRandomValue()
	if err != nil {
		writeInternalServerError(w, "Failed to start OIDC authentication", err)
		return
	}

	now := time.Now()
	redirectURL := a.oidcRedirectURL(r)

	flow := oidcFlow{
		State:        state,
		Nonce:        nonce,
		PKCEVerifier: pkceVerifier,
		RedirectURL:  redirectURL,
		Expires:      now.Add(OIDC_FLOW_VALID_PERIOD).Unix(),
	}

	flowCookie, err := encryptOIDCFlow(flow, a.authSecretKey)
	if err != nil {
		writeInternalServerError(w, "Failed to start OIDC authentication", err)
		return
	}

	a.setOIDCFlowCookie(
		w,
		r,
		flowCookie,
		now.Add(OIDC_FLOW_VALID_PERIOD),
	)

	oauthConfig := *a.oidc.oauthConfig
	oauthConfig.RedirectURL = redirectURL

	authorizationURL := oauthConfig.AuthCodeURL(
		state,
		oidc.Nonce(nonce),
		oauth2.SetAuthURLParam("code_challenge", oidcPKCEChallenge(pkceVerifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	http.Redirect(w, r, authorizationURL, http.StatusSeeOther)
}

func (a *application) handleOIDCCallbackRequest(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.NotFound(w, r)
		return
	}

	flowCookie, err := r.Cookie(OIDC_FLOW_COOKIE_NAME)
	if err != nil || flowCookie.Value == "" {
		http.Error(w, "OIDC authentication flow is missing or expired", http.StatusBadRequest)
		return
	}

	// The flow material contains the PKCE verifier and must not remain in the
	// browser after the callback is processed, regardless of callback outcome.
	a.clearOIDCFlowCookie(w, r)

	flow, err := decryptOIDCFlow(flowCookie.Value, a.authSecretKey, time.Now())
	if err != nil {
		slog.Warn("OIDC callback rejected", "stage", "flow_validation", "reason", "invalid_or_expired_flow")
		http.Error(w, "OIDC authentication flow is invalid or expired", http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("state") != flow.State {
		slog.Warn("OIDC callback rejected", "stage", "state_validation")
		http.Error(w, "OIDC authentication state is invalid", http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("error") != "" {
		slog.Warn(
			"OIDC provider rejected authentication",
			"stage", "authorization",
			"reason", "provider_rejected",
		)
		http.Error(w, "OIDC authentication was rejected", http.StatusUnauthorized)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		slog.Warn("OIDC callback rejected", "stage", "authorization_code")
		http.Error(w, "OIDC authorization code is missing", http.StatusBadRequest)
		return
	}

	oauthConfig := *a.oidc.oauthConfig
	oauthConfig.RedirectURL = flow.RedirectURL

	client := newHTTPClient(0, false)
	ctx, cancel := context.WithTimeout(r.Context(), client.Timeout)
	defer cancel()
	ctx = oidc.ClientContext(ctx, client)

	token, err := oauthConfig.Exchange(
		ctx,
		code,
		oauth2.SetAuthURLParam("code_verifier", flow.PKCEVerifier),
	)
	if err != nil {
		slog.Warn("OIDC callback rejected", "stage", "token_exchange")
		http.Error(w, "OIDC authentication failed", http.StatusUnauthorized)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		slog.Warn("OIDC callback rejected", "stage", "id_token_presence")
		http.Error(w, "OIDC identity token is missing", http.StatusUnauthorized)
		return
	}

	idToken, err := a.oidc.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		slog.Warn("OIDC callback rejected", "stage", "id_token_verification")
		http.Error(w, "OIDC identity token is invalid", http.StatusUnauthorized)
		return
	}

	if idToken.Nonce != flow.Nonce {
		slog.Warn("OIDC callback rejected", "stage", "nonce_validation")
		http.Error(w, "OIDC identity token nonce is invalid", http.StatusUnauthorized)
		return
	}

	if idToken.Subject == "" {
		slog.Warn("OIDC callback rejected", "stage", "subject_validation")
		http.Error(w, "OIDC identity is invalid", http.StatusUnauthorized)
		return
	}

	var identityClaims struct {
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}

	displayName := ""
	claimsValid := idToken.Claims(&identityClaims) == nil
	if claimsValid {
		displayName = strings.TrimSpace(identityClaims.Email)
		if displayName == "" {
			displayName = strings.TrimSpace(identityClaims.PreferredUsername)
		}
		if displayName == "" {
			displayName = strings.TrimSpace(identityClaims.Name)
		}

		if len([]byte(displayName)) > AUTH_TOKEN_V3_MAX_DISPLAY_LENGTH {
			displayName = ""
		}
	}

	if len(a.Config.Auth.OIDC.AllowedUsers) > 0 {
		if !claimsValid || strings.TrimSpace(identityClaims.Email) == "" {
			slog.Warn("OIDC callback rejected", "stage", "authorization", "reason", "email_claim_missing")
			http.Redirect(w, r, a.Config.Server.BaseURL+"/login?reason=oidc_not_authorized", http.StatusSeeOther)
			return
		}
		if !identityClaims.EmailVerified {
			slog.Warn("OIDC callback rejected", "stage", "authorization", "reason", "email_not_verified")
			http.Redirect(w, r, a.Config.Server.BaseURL+"/login?reason=oidc_not_authorized", http.StatusSeeOther)
			return
		}

		email := strings.ToLower(strings.TrimSpace(identityClaims.Email))
		allowed := false
		for _, allowedUser := range a.Config.Auth.OIDC.AllowedUsers {
			if email == strings.ToLower(strings.TrimSpace(allowedUser)) {
				allowed = true
				break
			}
		}
		if !allowed {
			slog.Warn("OIDC callback rejected", "stage", "authorization", "reason", "user_not_allowed")
			http.Redirect(w, r, a.Config.Server.BaseURL+"/login?reason=oidc_not_authorized", http.StatusSeeOther)
			return
		}
	}

	principal, err := encodeOIDCPrincipal(a.oidc.issuer, idToken.Subject)
	if err != nil {
		writeInternalServerError(w, "Failed to create OIDC session", err)
		return
	}

	now := time.Now()
	sessionToken, err := generateSessionTokenV3(
		authMethodOIDC,
		principal,
		displayName,
		a.authSecretKey,
		now,
	)
	if err != nil {
		writeInternalServerError(w, "Failed to create OIDC session", err)
		return
	}

	a.setAuthSessionCookie(
		w,
		r,
		sessionToken,
		now.Add(AUTH_TOKEN_VALID_PERIOD),
	)

	http.Redirect(w, r, a.Config.Server.BaseURL+"/", http.StatusSeeOther)
}
