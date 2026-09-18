package glance

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

func normalizeResourceProxyOrigin(configuredOrigin string) (string, error) {
	origin := strings.TrimSpace(configuredOrigin)
	if origin == "" {
		return "", fmt.Errorf("origin is empty")
	}

	parsed, err := url.Parse(origin)
	if err != nil ||
		!strings.EqualFold(parsed.Scheme, "http") ||
		parsed.Host == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("must be an absolute HTTP origin without userinfo, path, query, or fragment")
	}

	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", fmt.Errorf("must contain a host")
	}

	port := parsed.Port()
	if port == "" {
		port = "80"
	}

	return "http://" + net.JoinHostPort(hostname, port), nil
}

const resourceProxyIDRandomBytes = 32

const resourceProxyResponseBodyLimit int64 = 10 * 1024 * 1024

var errResourceProxyRedirectNotAllowed = errors.New("resource proxy redirect not allowed")
var errResourceProxyUnsupportedContentType = errors.New("resource proxy unsupported content type")

type resourceProxyResponse struct {
	ContentType string
	Body        []byte
}

func newResourceProxyHTTPClient(policy *resourceProxyPolicy) *http.Client {
	transport := defaultHTTPTransport.Clone()
	transport.Proxy = nil

	client := &http.Client{
		Transport: observeHTTPTransport(transport),
		Timeout:   defaultClientTimeout,
	}

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("resource proxy stopped after 10 redirects")
		}

		if len(via) == 0 || !policy.allowsURL(req.URL) {
			return errResourceProxyRedirectNotAllowed
		}

		originalOrigin, err := resourceProxyURLOrigin(via[0].URL)
		if err != nil {
			return errResourceProxyRedirectNotAllowed
		}

		redirectOrigin, err := resourceProxyURLOrigin(req.URL)
		if err != nil || redirectOrigin != originalOrigin {
			return errResourceProxyRedirectNotAllowed
		}

		return nil
	}

	return client
}

func resourceProxyContentTypeAllowed(contentType string) bool {
	switch contentType {
	case "image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/avif",
		"image/x-icon",
		"image/vnd.microsoft.icon":
		return true
	default:
		return false
	}
}

func (p *resourceProxy) fetch(ctx context.Context, id string) (*resourceProxyResponse, error) {
	if p == nil || p.policy == nil || p.client == nil {
		return nil, errors.New("resource proxy is unavailable")
	}

	rawURL, exists := p.lookup(id)
	if !exists {
		return nil, errors.New("resource proxy resource not found")
	}

	resourceURL, err := url.Parse(rawURL)
	if err != nil || !p.policy.allowsURL(resourceURL) {
		return nil, errors.New("resource proxy destination is not allowed")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, resourceURL.String(), nil)
	if err != nil {
		return nil, errors.New("creating resource proxy request")
	}

	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf(
			"fetching resource proxy response: %w",
			safeHTTPTransportError(err),
		)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"resource proxy upstream returned HTTP status %d",
			response.StatusCode,
		)
	}

	contentType := response.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errResourceProxyUnsupportedContentType
	}
	mediaType = strings.ToLower(mediaType)
	if !resourceProxyContentTypeAllowed(mediaType) {
		return nil, errResourceProxyUnsupportedContentType
	}

	body, err := readHTTPResponseBody(response.Body, resourceProxyResponseBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("reading resource proxy response: %w", err)
	}

	return &resourceProxyResponse{
		ContentType: mediaType,
		Body:        body,
	}, nil
}

func (a *application) resolveResourceProxyURL(rawURL string) (string, error) {
	if a == nil || a.resourceProxy == nil || a.resourceProxy.policy == nil {
		return rawURL, nil
	}

	resourceURL, err := url.Parse(rawURL)
	if err != nil || !a.resourceProxy.policy.allowsURL(resourceURL) {
		return rawURL, nil
	}

	resourceID, err := a.resourceProxy.register(rawURL)
	if err != nil {
		return "", errors.New("registering resource proxy URL")
	}

	return a.Config.Server.BaseURL + "/api/resource-proxy/" + resourceID, nil
}

func (a *application) handleResourceProxyRequest(w http.ResponseWriter, r *http.Request) {
	if _, authenticated := a.authorizeSession(w, r); !authenticated {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	resourceID := r.PathValue("resource")
	response, err := a.resourceProxy.fetch(r.Context(), resourceID)
	if err != nil {
		if _, exists := a.resourceProxy.lookup(resourceID); !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		slog.Warn("Resource proxy request failed", "resource_id", resourceID, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", response.ContentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response.Body)
}

type resourceProxyPolicy struct {
	allowedOrigins map[string]struct{}
}

type resourceProxy struct {
	policy *resourceProxyPolicy
	client *http.Client

	mu    sync.RWMutex
	byID  map[string]string
	byURL map[string]string
}

func newResourceProxy(configuredOrigins []string) (*resourceProxy, error) {
	policy, err := newResourceProxyPolicy(configuredOrigins)
	if err != nil {
		return nil, err
	}

	return &resourceProxy{
		policy: policy,
		client: newResourceProxyHTTPClient(policy),
		byID:   make(map[string]string),
		byURL:  make(map[string]string),
	}, nil
}

func makeResourceProxyID() (string, error) {
	value := make([]byte, resourceProxyIDRandomBytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generating resource proxy ID: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (p *resourceProxy) allowsURL(resourceURL *url.URL) bool {
	return p != nil && p.policy != nil && p.policy.allowsURL(resourceURL)
}

func (p *resourceProxy) register(rawURL string) (string, error) {
	if p == nil || p.policy == nil {
		return "", fmt.Errorf("resource proxy is unavailable")
	}

	resourceURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing resource URL: %w", err)
	}
	if !p.policy.allowsURL(resourceURL) {
		return "", fmt.Errorf("resource URL origin is not allowed")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if id, exists := p.byURL[rawURL]; exists {
		return id, nil
	}

	for {
		id, err := makeResourceProxyID()
		if err != nil {
			return "", err
		}
		if _, exists := p.byID[id]; exists {
			continue
		}

		p.byID[id] = rawURL
		p.byURL[rawURL] = id
		return id, nil
	}
}

func (p *resourceProxy) lookup(id string) (string, bool) {
	if p == nil {
		return "", false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	rawURL, exists := p.byID[id]
	return rawURL, exists
}

func newResourceProxyPolicy(configuredOrigins []string) (*resourceProxyPolicy, error) {
	policy := &resourceProxyPolicy{
		allowedOrigins: make(map[string]struct{}, len(configuredOrigins)),
	}

	for _, configuredOrigin := range configuredOrigins {
		normalized, err := normalizeResourceProxyOrigin(configuredOrigin)
		if err != nil {
			return nil, fmt.Errorf("normalizing resource proxy origin: %w", err)
		}
		policy.allowedOrigins[normalized] = struct{}{}
	}

	return policy, nil
}

func resourceProxyURLOrigin(resourceURL *url.URL) (string, error) {
	if resourceURL == nil ||
		!strings.EqualFold(resourceURL.Scheme, "http") ||
		resourceURL.Host == "" ||
		resourceURL.User != nil {
		return "", fmt.Errorf("resource URL must be an absolute HTTP URL without userinfo")
	}

	hostname := strings.ToLower(resourceURL.Hostname())
	if hostname == "" {
		return "", fmt.Errorf("resource URL must contain a host")
	}

	port := resourceURL.Port()
	if port == "" {
		port = "80"
	}

	return "http://" + net.JoinHostPort(hostname, port), nil
}

func (p *resourceProxyPolicy) allowsURL(resourceURL *url.URL) bool {
	if p == nil {
		return false
	}

	origin, err := resourceProxyURLOrigin(resourceURL)
	if err != nil {
		return false
	}

	_, allowed := p.allowedOrigins[origin]
	return allowed
}
