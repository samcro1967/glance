package glance

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveBrowserImageURLTrustBoundary(t *testing.T) {
	proxyResolver := func(raw string) (string, error) {
		if raw == "http://allowed.example/image.png" {
			return "/api/resource-proxy/opaque", nil
		}
		return raw, nil
	}

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "https is rendered directly", raw: "https://example.com/image.png", want: "https://example.com/image.png"},
		{name: "local relative is rendered directly", raw: "/assets/image.png", want: "/assets/image.png"},
		{name: "allowlisted http is proxied", raw: "http://allowed.example/image.png", want: "/api/resource-proxy/opaque"},
		{name: "non allowlisted http is omitted", raw: "http://other.example/image.png", want: ""},
		{name: "protocol relative is omitted", raw: "//example.com/image.png", want: ""},
		{name: "credentialed https is omitted", raw: "https://user:secret@example.com/image.png", want: ""},
		{name: "unsupported scheme is omitted", raw: "data:image/png;base64,AAAA", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBrowserImageURL(tt.raw, proxyResolver)
			if err != nil {
				t.Fatalf("resolveBrowserImageURL(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("resolveBrowserImageURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestResolveBrowserImageURLPropagatesProxyRegistrationFailure(t *testing.T) {
	wantErr := errors.New("registration failed")
	_, err := resolveBrowserImageURL(
		"http://allowed.example/image.png",
		func(string) (string, error) { return "", wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestConfiguredIconOmitsNonAllowlistedHTTPImage(t *testing.T) {
	icon := newCustomIconField("http://other.example/icon.png")
	icon.resolveResourceProxy(func(raw string) (string, error) { return raw, nil })
	if got := string(icon.RenderURL()); got != "" {
		t.Fatalf("RenderURL = %q, want empty", got)
	}
}

func TestLogoutRouteRequiresPOST(t *testing.T) {
	app := newAuthTestApplication(t)
	handler := app.router()

	getRequest := httptest.NewRequest(http.MethodGet, "/logout", nil)
	getRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /logout status = %d, want %d", getRecorder.Code, http.StatusMethodNotAllowed)
	}
	if allow := getRecorder.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("GET /logout Allow = %q, want %q", allow, http.MethodPost)
	}

	postRequest := httptest.NewRequest(http.MethodPost, "/logout", nil)
	postRecorder := httptest.NewRecorder()
	handler.ServeHTTP(postRecorder, postRequest)
	if postRecorder.Code != http.StatusSeeOther {
		t.Fatalf("POST /logout status = %d, want %d", postRecorder.Code, http.StatusSeeOther)
	}
	if location := postRecorder.Header().Get("Location"); location != "/login" {
		t.Fatalf("POST /logout Location = %q, want %q", location, "/login")
	}
}
