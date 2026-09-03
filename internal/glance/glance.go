package glance

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	pageTemplate        = mustParseTemplate("page.html", "document.html", "footer.html")
	notFoundTemplate    = mustParseTemplate("not-found.html", "document.html", "footer.html")
	pageContentTemplate = mustParseTemplate("page-content.html")
	manifestTemplate    = mustParseTemplate("manifest.json")
)

const STATIC_ASSETS_CACHE_DURATION = 24 * time.Hour

var reservedPageSlugs = []string{"login", "logout"}

var reservedDashboardSlugs = []string{
	"api",
	"assets",
	"login",
	"logout",
	"manifest.json",
	"static",
}

type dashboard struct {
	Name  string
	Slug  string
	Pages []*page
}

type application struct {
	Version       string
	ShortRevision string
	CreatedAt     time.Time
	Config        config

	releaseStatus  releaseStatusCache
	parsedManifest []byte

	slugToPage       map[string]*page
	slugToDashboard  map[string]*dashboard
	dashboards       []*dashboard
	defaultDashboard *dashboard
	widgetByID       map[uint64]widget
	refreshWidgets   []widget
	liveUpdates      *liveUpdateBroker

	RequiresAuth           bool
	authSecretKey          []byte
	usernameHashToUsername map[string]string
	authAttemptsMu         sync.Mutex
	failedAuthAttempts     map[string]*failedAuthAttempt
}

func shortBuildRevision(revision string) string {
	if len(revision) <= 7 {
		return revision
	}

	return revision[:7]
}

func (a *application) shouldCheckForkReleaseStatus() bool {
	return a.Version != "dev" &&
		!a.Config.Branding.HideFooter &&
		a.Config.Branding.CustomFooter == ""
}

func (a *application) ReleaseStatus() releaseStatusResult {
	return a.releaseStatus.get()
}

func collectRefreshWidgets(source widgets) []widget {
	seen := make(map[uint64]struct{})
	collected := make([]widget, 0)

	var collect func(widget)
	collect = func(candidate widget) {
		if container, ok := candidate.(widgetContainer); ok {
			for _, child := range container.childWidgets() {
				collect(child)
			}
			return
		}

		id := candidate.GetID()
		if _, exists := seen[id]; exists {
			return
		}

		seen[id] = struct{}{}
		collected = append(collected, candidate)
	}

	for _, candidate := range source {
		collect(candidate)
	}

	return collected
}

func newApplication(c *config) (*application, error) {
	app := &application{
		Version:         buildVersion,
		ShortRevision:   shortBuildRevision(buildRevision),
		CreatedAt:       time.Now(),
		Config:          *c,
		slugToPage:      make(map[string]*page),
		slugToDashboard: make(map[string]*dashboard),
		widgetByID:      make(map[uint64]widget),
		liveUpdates:     newLiveUpdateBroker(),
	}
	config := &app.Config

	//
	// Init auth
	//

	if len(config.Auth.Users) > 0 {
		secretBytes, err := base64.StdEncoding.DecodeString(config.Auth.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("decoding secret-key: %v", err)
		}

		if len(secretBytes) != AUTH_SECRET_KEY_LENGTH {
			return nil, fmt.Errorf("secret-key must be exactly %d bytes", AUTH_SECRET_KEY_LENGTH)
		}

		app.usernameHashToUsername = make(map[string]string)
		app.failedAuthAttempts = make(map[string]*failedAuthAttempt)
		app.RequiresAuth = true

		for username := range config.Auth.Users {
			user := config.Auth.Users[username]
			usernameHash, err := computeUsernameHash(username, secretBytes)
			if err != nil {
				return nil, fmt.Errorf("computing username hash for user %s: %v", username, err)
			}
			app.usernameHashToUsername[string(usernameHash)] = username

			if user.PasswordHashString != "" {
				user.PasswordHash = []byte(user.PasswordHashString)
				user.PasswordHashString = ""
			} else {
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
				if err != nil {
					return nil, fmt.Errorf("hashing password for user %s: %v", username, err)
				}

				user.Password = ""
				user.PasswordHash = hashedPassword
			}
		}

		app.authSecretKey = secretBytes
	}

	//
	// Init themes
	//

	if !config.Theme.DisablePicker {
		themeKeys := make([]string, 0, 2)
		themeProps := make([]*themeProperties, 0, 2)

		defaultDarkTheme, ok := config.Theme.Presets.Get("default-dark")
		if ok && (!config.Theme.SameAs(defaultDarkTheme) || !config.Theme.SameAs(&themeProperties{})) {
			themeKeys = append(themeKeys, "default-dark")
			themeProps = append(themeProps, &themeProperties{})
		}

		themeKeys = append(themeKeys, "default-light")
		themeProps = append(themeProps, &themeProperties{
			Light:                    true,
			BackgroundColor:          &hslColorField{240, 13, 95},
			PrimaryColor:             &hslColorField{230, 100, 30},
			NegativeColor:            &hslColorField{0, 70, 50},
			ContrastMultiplier:       1.3,
			TextSaturationMultiplier: 0.5,
		})

		themePresets, err := newOrderedYAMLMap(themeKeys, themeProps)
		if err != nil {
			return nil, fmt.Errorf("creating theme presets: %v", err)
		}
		config.Theme.Presets = *themePresets.Merge(&config.Theme.Presets)

		for key, properties := range config.Theme.Presets.Items() {
			properties.Key = key
			if err := properties.init(); err != nil {
				return nil, fmt.Errorf("initializing preset theme %s: %v", key, err)
			}
		}
	}

	config.Theme.Key = "default"
	if err := config.Theme.init(); err != nil {
		return nil, fmt.Errorf("initializing default theme: %v", err)
	}

	//
	// Init pages
	//

	app.slugToPage[""] = &config.Pages[0]

	providers := &widgetProviders{
		assetResolver: app.StaticAssetPath,
	}

	for p := range config.Pages {
		page := &config.Pages[p]
		page.PrimaryColumnIndex = -1

		if page.Slug == "" {
			page.Slug = titleToSlug(page.Title)
		}

		if slices.Contains(reservedPageSlugs, page.Slug) {
			return nil, fmt.Errorf("page slug \"%s\" is reserved", page.Slug)
		}

		app.slugToPage[page.Slug] = page

		if page.Width == "default" {
			page.Width = ""
		}

		if page.DesktopNavigationWidth == "" || page.DesktopNavigationWidth == "default" {
			page.DesktopNavigationWidth = page.Width
		}

		for i := range page.HeadWidgets {
			widget := page.HeadWidgets[i]
			app.widgetByID[widget.GetID()] = widget
			widget.setProviders(providers)
		}

		for i := range page.BottomWidgets {
			widget := page.BottomWidgets[i]
			app.widgetByID[widget.GetID()] = widget
			widget.setProviders(providers)
		}

		for c := range page.Columns {
			column := &page.Columns[c]

			if page.PrimaryColumnIndex == -1 && column.Size == "full" {
				page.PrimaryColumnIndex = int8(c)
			}

			for w := range column.Widgets {
				widget := column.Widgets[w]
				app.widgetByID[widget.GetID()] = widget
				widget.setProviders(providers)
			}
		}

		if page.PrimaryColumnIndex == -1 {
			for c := range page.Columns {
				if page.Columns[c].Size == "medium" {
					page.PrimaryColumnIndex = int8(c)
					break
				}
			}
		}
	}

	if len(config.Dashboards.keys) > 0 {
		for dashboardName, pageSlugs := range config.Dashboards.Items() {
			dashboardSlug := titleToSlug(dashboardName)
			if dashboardSlug == "" {
				return nil, fmt.Errorf("dashboard %q has an invalid slug", dashboardName)
			}

			if dashboardName != "Default" && slices.Contains(reservedDashboardSlugs, dashboardSlug) {
				return nil, fmt.Errorf("dashboard slug %q is reserved", dashboardSlug)
			}

			if dashboardName != "Default" {
				if _, exists := app.slugToDashboard[dashboardSlug]; exists {
					return nil, fmt.Errorf("dashboard slug %q is duplicated", dashboardSlug)
				}

				if _, exists := app.slugToPage[dashboardSlug]; exists {
					slog.Warn(
						"Ignoring dashboard because its slug conflicts with a page slug",
						"dashboard", dashboardName,
						"slug", dashboardSlug,
					)
					continue
				}
			}

			dashboardPages := make([]*page, 0, len(pageSlugs))
			for _, pageSlug := range pageSlugs {
				page, exists := app.slugToPage[pageSlug]
				if !exists {
					return nil, fmt.Errorf("dashboard %q references unknown page slug %q", dashboardName, pageSlug)
				}

				dashboardPages = append(dashboardPages, page)
			}

			dashboard := &dashboard{
				Name:  dashboardName,
				Slug:  dashboardSlug,
				Pages: dashboardPages,
			}

			app.dashboards = append(app.dashboards, dashboard)

			if dashboardName == "Default" {
				dashboard.Slug = ""
				app.defaultDashboard = dashboard
				continue
			}

			app.slugToDashboard[dashboard.Slug] = dashboard
		}
	}

	refreshSources := make(widgets, 0)
	for p := range config.Pages {
		page := &config.Pages[p]
		refreshSources = append(refreshSources, page.HeadWidgets...)

		for c := range page.Columns {
			refreshSources = append(refreshSources, page.Columns[c].Widgets...)
		}

		refreshSources = append(refreshSources, page.BottomWidgets...)
	}
	app.refreshWidgets = collectRefreshWidgets(refreshSources)
	for _, widget := range app.refreshWidgets {
		app.widgetByID[widget.GetID()] = widget
	}

	config.Server.BaseURL = strings.TrimRight(config.Server.BaseURL, "/")
	config.Theme.CustomCSSFile = app.resolveUserDefinedAssetPath(config.Theme.CustomCSSFile)
	config.Branding.LogoURL = app.resolveUserDefinedAssetPath(config.Branding.LogoURL)

	config.Branding.FaviconURL = ternary(
		config.Branding.FaviconURL == "",
		app.StaticAssetPath("favicon.svg"),
		app.resolveUserDefinedAssetPath(config.Branding.FaviconURL),
	)

	config.Branding.FaviconType = ternary(
		strings.HasSuffix(config.Branding.FaviconURL, ".svg"),
		"image/svg+xml",
		"image/png",
	)

	if config.Branding.AppName == "" {
		config.Branding.AppName = "Glance"
	}

	if config.Branding.AppIconURL == "" {
		config.Branding.AppIconURL = app.StaticAssetPath("app-icon.png")
	}

	if config.Branding.AppBackgroundColor == "" {
		config.Branding.AppBackgroundColor = config.Theme.BackgroundColorAsHex
	}

	manifest, err := executeTemplateToString(manifestTemplate, templateData{App: app})
	if err != nil {
		return nil, fmt.Errorf("parsing manifest.json: %v", err)
	}
	app.parsedManifest = []byte(manifest)

	return app, nil
}

func (p *page) updateOutdatedWidgets() {
	now := time.Now()

	var wg sync.WaitGroup
	context := context.Background()

	refreshWidget := func(widget widget) {
		defer wg.Done()
		refreshWidgetIfNeeded(context, widget, &now)
	}

	for w := range p.HeadWidgets {
		wg.Add(1)
		go refreshWidget(p.HeadWidgets[w])
	}

	for c := range p.Columns {
		for w := range p.Columns[c].Widgets {
			wg.Add(1)
			go refreshWidget(p.Columns[c].Widgets[w])
		}
	}

	for w := range p.BottomWidgets {
		wg.Add(1)
		go refreshWidget(p.BottomWidgets[w])
	}

	wg.Wait()
}

func (a *application) resolveUserDefinedAssetPath(path string) string {
	if strings.HasPrefix(path, "/assets/") {
		return a.Config.Server.BaseURL + path
	}

	return path
}

type templateRequestData struct {
	Theme *themeProperties
}

type templateData struct {
	App             *application
	Page            *page
	NavigationPages []*page
	Dashboards      []*dashboard
	Dashboard       *dashboard
	DashboardPath   string
	Request         templateRequestData
}

func (a *application) populateTemplateRequestData(data *templateRequestData, r *http.Request) {
	theme := &a.Config.Theme.themeProperties

	if !a.Config.Theme.DisablePicker {
		selectedTheme, err := r.Cookie("theme")
		if err == nil {
			preset, exists := a.Config.Theme.Presets.Get(selectedTheme.Value)
			if exists {
				theme = preset
			}
		}
	}

	data.Theme = theme
}

func (a *application) renderPage(
	w http.ResponseWriter,
	r *http.Request,
	page *page,
	navigationPages []*page,
	dashboard *dashboard,
	dashboardPath string,
) {
	if a.handleUnauthorizedResponse(w, r, redirectToLogin) {
		return
	}

	data := templateData{
		App:             a,
		Page:            page,
		NavigationPages: navigationPages,
		Dashboards:      a.dashboards,
		Dashboard:       dashboard,
		DashboardPath:   dashboardPath,
	}
	a.populateTemplateRequestData(&data.Request, r)

	var responseBytes bytes.Buffer
	err := pageTemplate.Execute(&responseBytes, data)
	if err != nil {
		writeInternalServerError(w, "Failed to render page", err)
		return
	}

	w.Write(responseBytes.Bytes())
}

func (a *application) handlePageRequest(w http.ResponseWriter, r *http.Request) {
	if a.defaultDashboard == nil {
		page, exists := a.slugToPage[r.PathValue("page")]
		if !exists {
			a.handleNotFound(w, r, pagePointers(a.Config.Pages), nil, "")
			return
		}

		a.renderPage(
			w,
			r,
			page,
			pagePointers(a.Config.Pages),
			nil,
			"",
		)
		return
	}

	pageSlug := r.PathValue("page")
	var page *page

	if pageSlug == "" {
		page = a.defaultDashboard.Pages[0]
	} else {
		for _, candidate := range a.defaultDashboard.Pages {
			if candidate.Slug == pageSlug {
				page = candidate
				break
			}
		}
	}

	if page == nil {
		a.handleNotFound(w, r, a.defaultDashboard.Pages, a.defaultDashboard, "")
		return
	}

	a.renderPage(
		w,
		r,
		page,
		a.defaultDashboard.Pages,
		a.defaultDashboard,
		"",
	)
}

func (a *application) handleDashboardPageRequest(
	dashboard *dashboard,
	w http.ResponseWriter,
	r *http.Request,
) {
	pageSlug := r.PathValue("page")
	var page *page

	if pageSlug == "" {
		page = dashboard.Pages[0]
	} else {
		for _, candidate := range dashboard.Pages {
			if candidate.Slug == pageSlug {
				page = candidate
				break
			}
		}
	}

	if page == nil {
		a.handleNotFound(w, r, dashboard.Pages, dashboard, "/"+dashboard.Slug)
		return
	}

	a.renderPage(
		w,
		r,
		page,
		dashboard.Pages,
		dashboard,
		"/"+dashboard.Slug,
	)
}

func pagePointers(pages []page) []*page {
	result := make([]*page, len(pages))
	for i := range pages {
		result[i] = &pages[i]
	}
	return result
}

func (a *application) handlePageContentRequest(w http.ResponseWriter, r *http.Request) {
	page, exists := a.slugToPage[r.PathValue("page")]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	pageData := templateData{
		Page: page,
	}

	var err error
	var responseBytes bytes.Buffer

	func() {
		page.mu.Lock()
		defer page.mu.Unlock()

		page.updateOutdatedWidgets()
		err = pageContentTemplate.Execute(&responseBytes, pageData)
	}()

	if err != nil {
		writeInternalServerError(w, "Failed to render page content", err)
		return
	}

	w.Write(responseBytes.Bytes())
}

func (a *application) addressOfRequest(r *http.Request) string {
	remoteAddrWithoutPort := func() string {
		for i := len(r.RemoteAddr) - 1; i >= 0; i-- {
			if r.RemoteAddr[i] == ':' {
				return r.RemoteAddr[:i]
			}
		}

		return r.RemoteAddr
	}

	if !a.Config.Server.Proxied {
		return remoteAddrWithoutPort()
	}

	// This should probably be configurable or look for multiple headers, not just this one
	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor == "" {
		return remoteAddrWithoutPort()
	}

	ips := strings.Split(forwardedFor, ",")
	if len(ips) == 0 {
		return remoteAddrWithoutPort()
	}

	// Use the last (rightmost) IP in X-Forwarded-For, as this is the
	// one added by the trusted reverse proxy. The leftmost values can
	// be spoofed by the client and must not be trusted for rate limiting
	// or other security-sensitive operations.
	lastIP := strings.TrimSpace(ips[len(ips)-1])
	if lastIP == "" {
		return remoteAddrWithoutPort()
	}

	return lastIP
}

func (a *application) handleNotFound(
	w http.ResponseWriter,
	r *http.Request,
	navigationPages []*page,
	dashboard *dashboard,
	dashboardPath string,
) {
	if a.handleUnauthorizedResponse(w, r, redirectToLogin) {
		return
	}

	data := templateData{
		App:             a,
		NavigationPages: navigationPages,
		Dashboards:      a.dashboards,
		Dashboard:       dashboard,
		DashboardPath:   dashboardPath,
	}
	a.populateTemplateRequestData(&data.Request, r)

	var responseBytes bytes.Buffer
	if err := notFoundTemplate.Execute(&responseBytes, data); err != nil {
		writeInternalServerError(w, "Failed to render not found page", err)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	w.Write(responseBytes.Bytes())
}

func (a *application) handleWidgetContentRequest(w http.ResponseWriter, r *http.Request) {
	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	widgetID, err := strconv.ParseUint(r.PathValue("widget"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	widget, exists := a.widgetByID[widgetID]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(renderWidget(widget)))
}

func (a *application) handleWidgetRequest(w http.ResponseWriter, r *http.Request) {
	// TODO: this requires a rework of the widget update logic so that rather
	// than locking the entire page we lock individual widgets
	w.WriteHeader(http.StatusNotImplemented)

	// widgetValue := r.PathValue("widget")

	// widgetID, err := strconv.ParseUint(widgetValue, 10, 64)
	// if err != nil {
	// 	a.handleNotFound(w, r)
	// 	return
	// }

	// widget, exists := a.widgetByID[widgetID]

	// if !exists {
	// 	a.handleNotFound(w, r)
	// 	return
	// }

	// widget.handleRequest(w, r)
}

func (a *application) StaticAssetPath(asset string) string {
	return a.Config.Server.BaseURL + "/static/" + staticFSHash + "/" + asset
}

func (a *application) VersionedAssetPath(asset string) string {
	return a.Config.Server.BaseURL + "/" + asset +
		"?v=" + strconv.FormatInt(a.CreatedAt.Unix(), 10)
}

func (a *application) router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", a.handlePageRequest)
	mux.HandleFunc("GET /{page}", a.handlePageRequest)

	if a.defaultDashboard != nil {
		for dashboardSlug, dashboard := range a.slugToDashboard {
			mux.HandleFunc(
				fmt.Sprintf("GET /%s", dashboardSlug),
				func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(
						w,
						r,
						a.Config.Server.BaseURL+"/"+dashboardSlug+"/",
						http.StatusMovedPermanently,
					)
				},
			)

			mux.HandleFunc(
				fmt.Sprintf("GET /%s/{$}", dashboardSlug),
				func(w http.ResponseWriter, r *http.Request) {
					a.handleDashboardPageRequest(dashboard, w, r)
				},
			)

			mux.HandleFunc(
				fmt.Sprintf("GET /%s/{page}", dashboardSlug),
				func(w http.ResponseWriter, r *http.Request) {
					a.handleDashboardPageRequest(dashboard, w, r)
				},
			)
		}
	}

	mux.HandleFunc("GET /api/pages/{page}/content/{$}", a.handlePageContentRequest)

	if !a.Config.Theme.DisablePicker {
		mux.HandleFunc("POST /api/set-theme/{key}", a.handleThemeChangeRequest)
	}

	mux.HandleFunc("GET /api/widgets/{widget}/content/{$}", a.handleWidgetContentRequest)
	mux.HandleFunc("GET /api/live-updates", a.handleLiveUpdatesRequest)
	mux.HandleFunc("POST /api/frontend-diagnostics", a.handleFrontendDiagnosticsRequest)
	mux.HandleFunc("/api/widgets/{widget}/{path...}", a.handleWidgetRequest)
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if a.RequiresAuth {
		mux.HandleFunc("GET /login", a.handleLoginPageRequest)
		mux.HandleFunc("GET /logout", a.handleLogoutRequest)
		mux.HandleFunc("POST /api/authenticate", a.handleAuthenticationAttempt)
	}

	mux.Handle(
		fmt.Sprintf("GET /static/%s/{path...}", staticFSHash),
		http.StripPrefix(
			"/static/"+staticFSHash,
			fileServerWithCache(http.FS(staticFS), STATIC_ASSETS_CACHE_DURATION),
		),
	)

	assetCacheControlValue := fmt.Sprintf(
		"public, max-age=%d",
		int(STATIC_ASSETS_CACHE_DURATION.Seconds()),
	)

	mux.HandleFunc(fmt.Sprintf("GET /static/%s/css/bundle.css", staticFSHash), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", assetCacheControlValue)
		w.Header().Add("Content-Type", "text/css; charset=utf-8")
		w.Write(bundledCSSContents)
	})

	mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", assetCacheControlValue)
		w.Header().Add("Content-Type", "application/json")
		w.Write(a.parsedManifest)
	})

	if a.Config.Server.AssetsPath != "" {
		assetsFS := fileServerWithCache(http.Dir(a.Config.Server.AssetsPath), 2*time.Hour)
		mux.Handle("/assets/{path...}", http.StripPrefix("/assets/", assetsFS))
	}

	return mux
}

func (a *application) server() (func() error, func() error) {
	var absAssetsPath string
	if a.Config.Server.AssetsPath != "" {
		absAssetsPath, _ = filepath.Abs(a.Config.Server.AssetsPath)
	}

	server := http.Server{
		Addr:    fmt.Sprintf("%s:%d", a.Config.Server.Host, a.Config.Server.Port),
		Handler: a.router(),
	}

	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	var schedulerWG sync.WaitGroup

	start := func() error {
		slog.Info(
			"Server starting",
			"host", a.Config.Server.Host,
			"port", a.Config.Server.Port,
			"base_url", a.Config.Server.BaseURL,
			"assets_path", absAssetsPath,
		)

		schedulerWG.Add(1)
		go func() {
			defer schedulerWG.Done()

			runWidgetRefreshScheduler(
				schedulerCtx,
				a.refreshWidgets,
				widgetRefreshScanInterval,
				widgetRefreshConcurrency,
				a.liveUpdates,
			)
		}()

		if a.shouldCheckForkReleaseStatus() {
			schedulerWG.Add(1)
			go func() {
				defer schedulerWG.Done()

				runForkReleaseStatusChecker(
					schedulerCtx,
					a.Version,
					&a.releaseStatus,
				)
			}()
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.liveUpdates.close()
			stopScheduler()
			schedulerWG.Wait()
			return err
		}

		slog.Info("Server stopped")

		return nil
	}

	stop := func() error {
		slog.Info("Server stopping")
		a.liveUpdates.close()

		stopScheduler()

		serverErr := server.Close()

		schedulerWG.Wait()

		return serverErr
	}

	return start, stop
}
