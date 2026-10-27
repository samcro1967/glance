package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// redditBrowserConfig is shared by all browser-backed Reddit widgets in a config.
// Credentials use Glance's existing environment/secret expansion.
type redditBrowserConfig struct {
	Endpoint string `yaml:"endpoint"`
	Reddit   struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"reddit"`
}

func configureRedditBrowser(candidate widget, cfg redditBrowserConfig) widget {
	if reddit, ok := candidate.(*redditWidget); ok {
		reddit.browserConfig = cfg
	}

	if container, ok := candidate.(widgetContainer); ok {
		for _, child := range container.childWidgets() {
			configureRedditBrowser(child, cfg)
		}
	}

	return candidate
}

func validateRedditBrowserConfig(w *redditWidget) error {
	if w.AppAuth.enabled || w.Proxy.URL != "" || w.RequestURLTemplate != "" {
		return errors.New("use-playwright cannot be combined with app-auth, proxy, or request-url-template")
	}
	endpoint := strings.TrimSpace(w.browserConfig.Endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("use-playwright requires a valid playwright.endpoint HTTP URL")
	}
	if (w.browserConfig.Reddit.Username == "") != (w.browserConfig.Reddit.Password == "") {
		return errors.New("playwright.reddit.username and playwright.reddit.password must both be configured or both omitted")
	}
	return nil
}

func (w *redditWidget) browserRequestURL() string {
	q := url.Values{}
	if w.Limit > 25 {
		q.Set("limit", strconv.Itoa(w.Limit))
	}
	if w.Search != "" {
		q.Set("q", w.Search+" subreddit:"+w.Subreddit)
		q.Set("sort", w.SortBy)
		return "https://www.reddit.com/search.json?" + q.Encode()
	}
	if w.SortBy == "top" {
		q.Set("t", w.TopPeriod)
	}
	return "https://www.reddit.com/r/" + url.PathEscape(w.Subreddit) + "/" + w.SortBy + ".json?" + q.Encode()
}

type redditBrowserLoginState struct {
	gate     chan struct{}
	failedAt time.Time
	users    int
}

func (state *redditBrowserLoginState) acquire(ctx context.Context) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case state.gate <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-state.gate
		return nil, err
	}
	return func() { <-state.gate }, nil
}

var redditBrowserLogins = struct {
	sync.Mutex
	states map[string]*redditBrowserLoginState
}{
	states: make(map[string]*redditBrowserLoginState),
}

func redditBrowserState(cfg redditBrowserConfig) (*redditBrowserLoginState, func()) {
	key := cfg.Endpoint + "\x00" + cfg.Reddit.Username

	redditBrowserLogins.Lock()
	now := time.Now()
	for existingKey, existing := range redditBrowserLogins.states {
		if existing.users == 0 &&
			(existing.failedAt.IsZero() || now.Sub(existing.failedAt) >= 5*time.Minute) {
			delete(redditBrowserLogins.states, existingKey)
		}
	}

	state := redditBrowserLogins.states[key]
	if state == nil {
		state = &redditBrowserLoginState{gate: make(chan struct{}, 1)}
		redditBrowserLogins.states[key] = state
	}
	state.users++
	redditBrowserLogins.Unlock()

	return state, func() {
		redditBrowserLogins.Lock()
		defer redditBrowserLogins.Unlock()

		state.users--
		if state.users == 0 && state.failedAt.IsZero() {
			delete(redditBrowserLogins.states, key)
		}
	}
}

func redditBrowserResolveEndpoint(ctx context.Context, endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parsing Playwright CDP endpoint: %w", err)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return "", errors.New("Playwright CDP endpoint has no hostname")
	}

	if net.ParseIP(hostname) == nil {
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
		if err != nil {
			return "", fmt.Errorf("resolving Playwright CDP hostname %q: %w", hostname, err)
		}
		if len(addresses) == 0 {
			return "", fmt.Errorf("Playwright CDP hostname %q resolved to no addresses", hostname)
		}
		hostname = addresses[0].IP.String()
	}

	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	parsed.Host = net.JoinHostPort(hostname, port)
	return parsed.String(), nil
}

func redditBrowserWebSocketURL(advertisedURL, relayEndpoint string) (string, error) {
	advertised, err := url.Parse(advertisedURL)
	if err != nil {
		return "", fmt.Errorf("invalid CDP WebSocket URL: %w", err)
	}
	if (advertised.Scheme != "ws" && advertised.Scheme != "wss") ||
		advertised.Hostname() == "" ||
		advertised.User != nil ||
		advertised.RawQuery != "" ||
		advertised.Fragment != "" ||
		!strings.HasPrefix(advertised.Path, "/devtools/browser/") ||
		strings.TrimPrefix(advertised.Path, "/devtools/browser/") == "" {
		return "", errors.New("invalid CDP browser WebSocket endpoint")
	}

	relay, err := url.Parse(relayEndpoint)
	if err != nil || relay.Host == "" ||
		(relay.Scheme != "http" && relay.Scheme != "https") {
		return "", errors.New("invalid CDP relay endpoint")
	}

	advertised.Host = relay.Host
	if relay.Scheme == "https" {
		advertised.Scheme = "wss"
	} else {
		advertised.Scheme = "ws"
	}
	return advertised.String(), nil
}

func redditBrowserConnect(ctx context.Context, endpoint string, timeouts ...durationField) (context.Context, context.CancelFunc, error) {
	endpoint = strings.TrimRight(endpoint, "/")
	// Bound metadata DNS and HTTP together, without attaching this short-lived
	// context to the authenticated browser or its independent stage waits.
	timeout := newProviderHTTPClient(timeouts...).Timeout
	metadataCtx, cancelMetadata := context.WithTimeout(ctx, timeout)
	defer cancelMetadata()
	resolvedEndpoint, err := redditBrowserResolveEndpoint(metadataCtx, endpoint)
	if err != nil {
		return nil, nil, err
	}
	request, err := http.NewRequestWithContext(metadataCtx, http.MethodGet, resolvedEndpoint+"/json/version", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating CDP metadata request: %w", err)
	}
	// Preserve the CDP discovery transport and browser lifecycle; only its HTTP
	// budget is configurable. This is not a browser login/navigation deadline.
	client := &http.Client{Timeout: timeout}
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to Playwright CDP: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("Playwright CDP returned HTTP %d", response.StatusCode)
	}
	var info struct {
		WebSocket string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&info); err != nil {
		return nil, nil, fmt.Errorf("decoding Playwright CDP metadata: %w", err)
	}
	if info.WebSocket == "" {
		return nil, nil, errors.New("Playwright CDP did not return a WebSocket endpoint")
	}
	// Chromium advertises a loopback WebSocket URL; replace its authority with
	// the configured relay, which is reachable from the Glance container.
	webSocketURL, err := redditBrowserWebSocketURL(info.WebSocket, resolvedEndpoint)
	if err != nil {
		return nil, nil, err
	}
	allocator, stopAllocator := remote.NewAllocator(ctx, webSocketURL)
	browser, stopBrowser := chromedp.NewContext(allocator)
	cleanup := func() { stopBrowser(); stopAllocator() }
	if err := chromedp.Do(browser); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("attaching to Playwright browser: %w", err)
	}
	return browser, cleanup, nil
}

// redditBrowserPage owns only a newly created child tab. Never cancel an
// attached existing target: that can close the permanent browser tab.
func redditBrowserPage(browser context.Context, address string) (context.Context, context.CancelFunc, error) {
	page, cancel := chromedp.NewContext(browser)
	if err := chromedp.Do(page, chromedp.Navigate(address)); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("navigating Reddit browser tab: %w", err)
	}
	return page, cancel, nil
}

func redditBrowserEvaluate(ctx context.Context, script string) (string, error) {
	return chromedp.Run(ctx, chromedp.Evaluate[string](script))
}

func redditBrowserAuthenticated(ctx context.Context) (bool, error) {
	// Account-only navigation controls distinguish a logged-in account from
	// a guest session; JSON Listing alone does not prove authentication.
	deadline := time.NewTimer(12 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		state, err := redditBrowserEvaluate(ctx, `(() => {
            const accountMenu = Array.from(document.querySelectorAll("button"))
                .some(el => el.textContent.trim() === "Expand user menu");
            const inbox = Array.from(document.querySelectorAll('a[href="/notifications"]'))
                .some(el => el.textContent.trim() === "Open inbox");
            if (accountMenu && inbox) return "yes";

            const loginLink = document.querySelector(
                'a[href="/login/"], a[href="/login"]'
            );
            if (loginLink) return "no";

            return "pending";
        })()`)
		if err != nil {
			return false, err
		}
		if state == "yes" {
			return true, nil
		}
		if state == "no" {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, errors.New("Reddit login state could not be determined")
		case <-tick.C:
		}
	}
}

func redditBrowserWait(ctx context.Context, max time.Duration, predicate func() (bool, error)) error {
	timer := time.NewTimer(max)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready, err := predicate()
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("Reddit browser operation timed out")
		case <-ticker.C:
		}
	}
}

const redditLoginFindField = `(() => {
    function find(root, name) {
        const field = root.querySelector("input[name=\"" + name + "\"]");
        if (field) return field;
        for (const el of root.querySelectorAll("*")) {
            if (el.shadowRoot) {
                const found = find(el.shadowRoot, name);
                if (found) return found;
            }
        }
        return null;
    }
    const field = find(document, NAME);
    if (!field || !field.getClientRects().length || field.disabled) return false;
    field.focus();
    return true;
})()`

func redditBrowserFill(ctx context.Context, name, value string) error {
	script := strings.ReplaceAll(redditLoginFindField, "NAME", strconv.Quote(name))
	focused, err := chromedp.Run(ctx, chromedp.Evaluate[bool](script))
	if err != nil {
		return fmt.Errorf("focusing Reddit %s field: %w", name, err)
	}
	if !focused {
		return fmt.Errorf("Reddit %s field unavailable", name)
	}
	_, err = chromedp.Call(ctx, input.InsertText, input.InsertTextParams{Text: value})
	if err != nil {
		return fmt.Errorf("entering Reddit %s field: %w", name, err)
	}
	return nil
}

func redditBrowserLogin(ctx context.Context, cfg redditBrowserConfig) error {
	if err := chromedp.Do(ctx, chromedp.Navigate("https://www.reddit.com/login/")); err != nil {
		return err
	}
	err := redditBrowserWait(ctx, 20*time.Second, func() (bool, error) {
		result, err := redditBrowserEvaluate(ctx, `(() => {
            function find(root, name) {
                const f = root.querySelector('input[name="'+name+'"]'); if (f) return f;
                for (const el of root.querySelectorAll('*')) if (el.shadowRoot) { const v=find(el.shadowRoot,name); if(v) return v; }
                return null;
            }
            return !!(find(document,'username') && find(document,'password'));
        })()`)
		return result == "true", err
	})
	if err != nil {
		return fmt.Errorf("Reddit login form unavailable: %w", err)
	}
	if err := redditBrowserFill(ctx, "username", cfg.Reddit.Username); err != nil {
		return err
	}
	if err := redditBrowserFill(ctx, "password", cfg.Reddit.Password); err != nil {
		return err
	}
	err = redditBrowserWait(ctx, 10*time.Second, func() (bool, error) {
		result, err := redditBrowserEvaluate(ctx, `(() => {
            function find(root) {
                for (const b of root.querySelectorAll('button')) if ((b.textContent||'').trim()==='Log In' && b.getClientRects().length && !b.disabled) return true;
                for (const el of root.querySelectorAll('*')) if (el.shadowRoot && find(el.shadowRoot)) return true;
                return false;
            }
            return find(document);
        })()`)
		return result == "true", err
	})
	if err != nil {
		return fmt.Errorf("Reddit login button unavailable: %w", err)
	}
	result, err := redditBrowserEvaluate(ctx, `(() => {
        function click(root) {
            for (const b of root.querySelectorAll('button')) if ((b.textContent||'').trim()==='Log In' && b.getClientRects().length && !b.disabled) { b.click(); return true; }
            for (const el of root.querySelectorAll('*')) if (el.shadowRoot && click(el.shadowRoot)) return true;
            return false;
        }
        return click(document);
    })()`)
	if err != nil {
		return fmt.Errorf("submitting Reddit login: %w", err)
	}
	if result != "true" {
		return &providerAuthenticationError{Message: "Reddit login could not be submitted"}
	}
	// A navigation alone is not proof of authentication. Check the account UI.
	err = redditBrowserWait(ctx, 25*time.Second, func() (bool, error) {
		path, err := redditBrowserEvaluate(ctx, `location.pathname`)
		return err == nil && !strings.HasPrefix(path, "/login"), err
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("waiting for Reddit login completion: %w", err)
	}
	if err := chromedp.Do(ctx, chromedp.Navigate("https://www.reddit.com/")); err != nil {
		return err
	}
	ok, err := redditBrowserAuthenticated(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return &providerAuthenticationError{Message: "Reddit authentication was not confirmed; manual challenge may be required"}
	}
	return nil
}

func redditBrowserEnsureLogin(ctx context.Context, browser context.Context, cfg redditBrowserConfig) error {
	state, releaseState := redditBrowserState(cfg)
	defer releaseState()
	// Serialize authentication across widgets sharing the same browser profile.
	release, err := state.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	page, cancel, err := redditBrowserPage(browser, "https://www.reddit.com/")
	if err != nil {
		return err
	}
	defer cancel()
	loggedIn, err := redditBrowserAuthenticated(page)
	if err != nil {
		return fmt.Errorf("checking Reddit login: %w", err)
	}
	if loggedIn {
		state.failedAt = time.Time{}
		return nil
	}
	if cfg.Reddit.Username == "" || cfg.Reddit.Password == "" {
		return &providerAuthenticationError{Message: "Reddit browser session is not authenticated; sign in manually or configure playwright.reddit.username and playwright.reddit.password"}
	}
	if !state.failedAt.IsZero() && time.Since(state.failedAt) < 5*time.Minute {
		return &providerAuthenticationError{Message: "Reddit login recovery is cooling down after a previous failure"}
	}
	if err := redditBrowserLogin(page, cfg); err != nil {
		var authErr *providerAuthenticationError
		if errors.As(err, &authErr) {
			state.failedAt = time.Now()
		}
		return fmt.Errorf("recovering Reddit login: %w", err)
	}
	state.failedAt = time.Time{}
	return nil
}

func redditBrowserDecodeListing(raw string, listing *subredditResponseJson) bool {
	var envelope struct {
		Kind string `json:"kind"`
		Data struct {
			Children json.RawMessage `json:"children"`
		} `json:"data"`
	}

	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return false
	}
	if envelope.Kind != "Listing" {
		return false
	}

	var children []json.RawMessage
	if err := json.Unmarshal(envelope.Data.Children, &children); err != nil {
		return false
	}
	if children == nil {
		return false
	}

	return json.Unmarshal([]byte(raw), listing) == nil
}

func fetchRedditBrowserListing(ctx context.Context, cfg redditBrowserConfig, address string, timeouts ...durationField) (subredditResponseJson, error) {
	var listing subredditResponseJson
	browser, cleanup, err := redditBrowserConnect(ctx, cfg.Endpoint, timeouts...)
	if err != nil {
		return listing, err
	}
	defer cleanup()
	if err := redditBrowserEnsureLogin(ctx, browser, cfg); err != nil {
		return listing, err
	}
	page, cancel, err := redditBrowserPage(browser, address)
	if err != nil {
		return listing, err
	}
	defer cancel()
	err = redditBrowserWait(page, 15*time.Second, func() (bool, error) {
		raw, err := redditBrowserEvaluate(page, `(document.body?.innerText || "")`)
		if err != nil {
			return false, err
		}
		if redditBrowserDecodeListing(raw, &listing) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return listing, fmt.Errorf("reading Reddit browser listing: %w", err)
	}
	return listing, nil
}
