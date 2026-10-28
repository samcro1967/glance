package glance

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

var errHTTPCredentialRedirect = errors.New("cross-origin redirect with request credentials is not allowed")

// Standard clients do not recognize provider-specific API-token headers.
// Treat non-public headers and replayable bodies as private, rather than trying
// to enumerate every provider's credential header. Public GET redirects remain
// supported, and same-origin redirects retain the configured authentication.
func checkHTTPRequestRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	for _, previous := range via {
		candidate := previous
		// Go adds Referer on redirects when the original request has none.
		// Earlier requests are checked separately for URL credentials; this
		// generated header must not turn a public redirect into a private one.
		// Explicitly configured Referer headers remain protected.
		if via[0].Header.Get("Referer") == "" && previous.Header.Get("Referer") != "" {
			candidate = previous.Clone(previous.Context())
			candidate.Header.Del("Referer")
		}
		if httpRequestHasCredentials(candidate) && !httpURLsShareOrigin(previous.URL, request.URL) {
			return errHTTPCredentialRedirect
		}
	}
	return nil
}

func httpURLsShareOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		if strings.EqualFold(u.Scheme, "http") {
			return "80"
		}
		return ""
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func httpRequestHasCredentials(request *http.Request) bool {
	if request.URL.User != nil || (request.Body != nil && request.Body != http.NoBody) {
		return true
	}
	for key := range request.URL.Query() {
		switch strings.ToLower(key) {
		case "token", "access_token", "api_key", "api-key", "apikey", "key", "password", "passwd", "auth", "sid", "t":
			return true
		}
	}
	for key := range request.Header {
		switch strings.ToLower(key) {
		case "accept", "accept-encoding", "accept-language", "user-agent", "content-type", "content-length", "cache-control", "if-none-match", "if-modified-since":
		default:
			return true
		}
	}
	return false
}
