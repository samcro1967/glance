package glance

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"net/netip"
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

	slugToPage               map[string]*page
	slugToDashboard          map[string]*dashboard
	dashboards               []*dashboard
	defaultDashboard         *dashboard
	widgetByID               map[uint64]widget
	refreshWidgets           []widget
	widgetReloadFingerprints map[widget]widgetReloadFingerprint
	liveUpdates              *liveUpdateBroker
	configDiagnostics        *configRuntimeDiagnostics
	profilingDiagnostics     *profilingRuntimeDiagnostics
	frontendDiagnostics      *frontendRuntimeDiagnostics
	trustedProxyPrefixes     []netip.Prefix
	resourceProxy            *resourceProxy

	RequiresAuth           bool
	authSecretKey          []byte
	usernameHashToUsername map[string]string
	authAttemptsMu         sync.Mutex
	failedAuthAttempts     map[string]*failedAuthAttempt
	oidc                   *oidcRuntime
	authorization          *authorizationPolicy
}

func (a *application) OIDCEnabled() bool {
	return a.oidc != nil
}

func (a *application) OIDCProviderName() string {
	if a.oidc == nil {
		return ""
	}
	return a.Config.Auth.OIDC.providerDisplayName()
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
	return newApplicationWithOIDCRuntime(c, nil)
}

func newApplicationWithOIDCRuntime(c *config, reusableOIDC *oidcRuntime) (*application, error) {
	app := &application{
		Version:             buildVersion,
		ShortRevision:       shortBuildRevision(buildRevision),
		CreatedAt:           time.Now(),
		Config:              *c,
		slugToPage:          make(map[string]*page),
		slugToDashboard:     make(map[string]*dashboard),
		widgetByID:          make(map[uint64]widget),
		liveUpdates:         newLiveUpdateBroker(),
		frontendDiagnostics: newFrontendRuntimeDiagnostics(),
	}
	config := &app.Config

	var err error
	app.resourceProxy, err = newResourceProxy(config.Server.ResourceProxy.AllowedOrigins)
	if err != nil {
		return nil, fmt.Errorf("initializing resource proxy policy: %w", err)
	}

	for _, trustedProxy := range config.Server.TrustedProxies {
		trustedProxy = strings.TrimSpace(trustedProxy)

		if prefix, err := netip.ParsePrefix(trustedProxy); err == nil {
			app.trustedProxyPrefixes = append(app.trustedProxyPrefixes, prefix.Masked())
			continue
		}

		addr, err := netip.ParseAddr(trustedProxy)
		if err != nil {
			return nil, fmt.Errorf("parsing trusted proxy %q: %w", trustedProxy, err)
		}
		app.trustedProxyPrefixes = append(
			app.trustedProxyPrefixes,
			netip.PrefixFrom(addr, addr.BitLen()),
		)
	}

	//
	// Init auth
	//

	authConfigured := len(config.Auth.Users) > 0 || config.Auth.OIDC.configured()
	if authConfigured {
		secretBytes, err := base64.StdEncoding.DecodeString(config.Auth.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("decoding secret-key: %v", err)
		}

		app.RequiresAuth = true
		app.authSecretKey = secretBytes
	}

	if len(config.Auth.Users) > 0 {
		app.usernameHashToUsername = make(map[string]string)
		app.failedAuthAttempts = make(map[string]*failedAuthAttempt)

		for username := range config.Auth.Users {
			user := config.Auth.Users[username]
			usernameHash, err := computeUsernameHash(username, app.authSecretKey)
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
	}

	if config.Auth.OIDC.configured() {
		var err error
		app.oidc, err = newOIDCRuntime(config.Auth.OIDC, reusableOIDC)
		if err != nil {
			return nil, err
		}
	}

	//
	// Init themes
	//

	if !config.Theme.DisablePicker {
		themeKeys := []string{"glance-dark", "glance-light"}
		themeProps := []*themeProperties{
			{
				Key:                      "glance-dark",
				Name:                     "Glance Dark",
				BackgroundColor:          &hslColorField{222, 24, 6},
				PrimaryColor:             &hslColorField{214, 72, 68},
				PositiveColor:            &hslColorField{145, 62, 55},
				WarningColor:             &hslColorField{40, 90, 62},
				NegativeColor:            &hslColorField{0, 78, 64},
				AccentColor:              &hslColorField{258, 88, 68},
				ContrastMultiplier:       1.15,
				TextSaturationMultiplier: 0.7,
				Typography: themeTypographyProperties{
					FontFamily:         "system",
					FontSize:           "medium",
					FontWeight:         "normal",
					TextColor:          &hslColorField{220, 18, 91},
					SecondaryTextColor: &hslColorField{220, 11, 68},
					MutedTextColor:     &hslColorField{220, 9, 52},
					Headings: themeHeadingProperties{
						FontFamily: "system",
						FontWeight: "semibold",
						TextColor:  &hslColorField{220, 20, 95},
					},
				},
				Page: themePageProperties{
					AmbientAccent: "subtle",
				},
				Header: themeHeaderProperties{
					BackgroundColor: &hslColorField{220, 22, 10},
					TextColor:       &hslColorField{220, 20, 90},
					BorderColor:     &hslColorField{220, 16, 18},
					Radius:          "large",
					Shadow:          "medium",
					Blur:            "medium",
				},
				Navigation: themeNavigationProperties{
					TextColor:   &hslColorField{220, 12, 68},
					HoverColor:  &hslColorField{220, 20, 94},
					ActiveColor: &hslColorField{220, 24, 98},
					AccentColor: &hslColorField{258, 88, 68},
					FontWeight:  "medium",
				},
				Widgets: themeSurfaceProperties{
					BackgroundColor: &hslColorField{220, 19, 11},
					BorderColor:     &hslColorField{220, 12, 16},
					Radius:          "large",
					Shadow:          "subtle",
				},
				WidgetHeader: themeWidgetHeaderProperties{
					TextColor:  &hslColorField{220, 18, 93},
					FontWeight: "semibold",
				},
				Cards: themeCardProperties{
					BackgroundColor: &hslColorField{220, 17, 14},
					BorderColor:     &hslColorField{220, 12, 19},
					Radius:          "medium",
					Shadow:          "subtle",
				},
				Controls: themeControlProperties{
					BackgroundColor: &hslColorField{220, 17, 14},
					TextColor:       &hslColorField{220, 20, 92},
					MutedColor:      &hslColorField{220, 10, 56},
					BorderColor:     &hslColorField{220, 14, 22},
					FocusColor:      &hslColorField{258, 88, 68},
					Radius:          "medium",
				},
				Surfaces: themeElevatedSurfaceProperties{
					ElevatedBackgroundColor: &hslColorField{220, 18, 14},
					ElevatedBorderColor:     &hslColorField{220, 14, 22},
					SeparatorColor:          &hslColorField{220, 12, 18},
				},
			},
			{
				Key:                      "glance-light",
				Name:                     "Glance Light",
				Light:                    true,
				BackgroundColor:          &hslColorField{220, 20, 96},
				PrimaryColor:             &hslColorField{230, 100, 30},
				PositiveColor:            &hslColorField{145, 55, 38},
				WarningColor:             &hslColorField{38, 88, 42},
				NegativeColor:            &hslColorField{0, 70, 50},
				AccentColor:              &hslColorField{258, 72, 52},
				ContrastMultiplier:       1.15,
				TextSaturationMultiplier: 0.7,
				Typography: themeTypographyProperties{
					FontFamily:         "system",
					FontSize:           "medium",
					FontWeight:         "normal",
					TextColor:          &hslColorField{220, 24, 15},
					SecondaryTextColor: &hslColorField{220, 12, 38},
					MutedTextColor:     &hslColorField{220, 9, 52},
					Headings: themeHeadingProperties{
						FontFamily: "system",
						FontWeight: "semibold",
						TextColor:  &hslColorField{220, 28, 11},
					},
				},
				Page: themePageProperties{
					AmbientAccent: "subtle",
				},
				Header: themeHeaderProperties{
					BackgroundColor: &hslColorField{220, 24, 99},
					TextColor:       &hslColorField{220, 24, 15},
					BorderColor:     &hslColorField{220, 16, 88},
					Radius:          "large",
					Shadow:          "medium",
					Blur:            "medium",
				},
				Navigation: themeNavigationProperties{
					TextColor:   &hslColorField{220, 12, 38},
					HoverColor:  &hslColorField{220, 24, 15},
					ActiveColor: &hslColorField{220, 28, 10},
					AccentColor: &hslColorField{258, 72, 52},
					FontWeight:  "medium",
				},
				Widgets: themeSurfaceProperties{
					BackgroundColor: &hslColorField{220, 20, 99},
					BorderColor:     &hslColorField{220, 16, 89},
					Radius:          "large",
					Shadow:          "subtle",
				},
				WidgetHeader: themeWidgetHeaderProperties{
					TextColor:  &hslColorField{220, 26, 13},
					FontWeight: "semibold",
				},
				Cards: themeCardProperties{
					BackgroundColor: &hslColorField{220, 22, 96},
					BorderColor:     &hslColorField{220, 16, 88},
					Radius:          "medium",
					Shadow:          "none",
				},
				Controls: themeControlProperties{
					BackgroundColor: &hslColorField{220, 22, 97},
					TextColor:       &hslColorField{220, 24, 15},
					MutedColor:      &hslColorField{220, 9, 48},
					BorderColor:     &hslColorField{220, 16, 86},
					FocusColor:      &hslColorField{258, 72, 52},
					Radius:          "medium",
				},
				Surfaces: themeElevatedSurfaceProperties{
					ElevatedBackgroundColor: &hslColorField{220, 22, 96},
					ElevatedBorderColor:     &hslColorField{220, 16, 88},
					SeparatorColor:          &hslColorField{220, 14, 89},
				},
			},
		}

		builtInThemes, err := newOrderedYAMLMap(themeKeys, themeProps)
		if err != nil {
			return nil, fmt.Errorf("creating built-in themes: %v", err)
		}
		config.Theme.Presets = *builtInThemes.Merge(&config.Theme.Presets)

		for key, properties := range config.Theme.Presets.Items() {
			if properties.Key == "" {
				properties.Key = key
			}
			if properties.Name == "" {
				properties.Name = themeDisplayName(key)
			}
			if err := properties.init(); err != nil {
				return nil, fmt.Errorf("initializing preset theme %s: %v", key, err)
			}
		}
	}

	config.Theme.Key = "default"
	if config.Theme.Name == "" {
		config.Theme.Name = themeDisplayName(config.Theme.Key)
	}
	if err := config.Theme.init(); err != nil {
		return nil, fmt.Errorf("initializing default theme: %v", err)
	}

	//
	// Init pages
	//

	app.slugToPage[""] = &config.Pages[0]

	providers := &widgetProviders{
		assetResolver:    app.StaticAssetPath,
		resourceProxyURL: app.resolveResourceProxyURL,
	}

	footerDynamicWidgets := config.FooterMicroWidgets.dynamicWidgets()
	for _, widget := range footerDynamicWidgets {
		widget.setID(widgetIDCounter.Add(1))
		widget.setProviders(providers)
		app.widgetByID[widget.GetID()] = widget
	}

	for p := range config.Pages {
		page := &config.Pages[p]
		page.PrimaryColumnIndex = -1

		app.slugToPage[page.Slug] = page

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

			if dashboardName != "Default" {
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
				dashboardPages = append(dashboardPages, app.slugToPage[pageSlug])
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

	app.authorization = newAuthorizationPolicy(
		app.Config.Auth.Groups,
		app.Config.Auth.Access,
		app.dashboards,
	)

	refreshSources := make(widgets, 0)
	refreshSources = append(refreshSources, footerDynamicWidgets...)

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
	app.widgetReloadFingerprints, err = captureWidgetReloadFingerprints(app.refreshWidgets)
	if err != nil {
		return nil, fmt.Errorf("capturing widget reload fingerprints: %w", err)
	}

	config.Server.BaseURL = strings.TrimRight(config.Server.BaseURL, "/")
	config.Theme.CustomCSSFile = app.resolveUserDefinedAssetPath(config.Theme.CustomCSSFile)

	for _, preset := range config.Theme.Presets.Items() {
		preset.CustomCSSFile = app.resolveUserDefinedAssetPath(preset.CustomCSSFile)
	}

	for i := range config.Pages {
		config.Pages[i].Theme.CustomCSSFile = app.resolveUserDefinedAssetPath(config.Pages[i].Theme.CustomCSSFile)
	}

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

func (a *application) resolveUserDefinedAssetPath(path string) string {
	if strings.HasPrefix(path, "/assets/") {
		return a.Config.Server.BaseURL + path
	}

	return path
}

type templateRequestData struct {
	Theme               *themeProperties
	ThemeChoices        []*themeProperties
	GlobalCustomCSSFile string
	ThemeCustomCSSFile  string
	PageCustomCSSFile   string
	AuthDisplayName     string
	AuthDescription     string
	LoginMessage        string
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

func themeDisplayName(key string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(key))
	for i := range words {
		if len(words[i]) == 0 {
			continue
		}
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, " ")
}

func (a *application) populateTemplateRequestData(data *templateRequestData, r *http.Request, page *page) {
	theme := &a.Config.Theme.themeProperties

	data.GlobalCustomCSSFile = a.Config.Theme.CustomCSSFile

	if !a.Config.Theme.DisablePicker {
		selectedTheme, err := r.Cookie("theme")
		if err == nil {
			preset, exists := a.Config.Theme.Presets.Get(selectedTheme.Value)
			if exists {
				theme = preset
				data.ThemeCustomCSSFile = preset.CustomCSSFile
			}
		}
	}

	var pageOverride *themeProperties
	if page != nil {
		pageOverride = &page.Theme
		data.PageCustomCSSFile = page.Theme.CustomCSSFile
	}

	resolved, err := resolveTheme(theme, pageOverride)
	if err != nil {
		data.Theme = theme
		return
	}

	data.Theme = resolved

	if a.Config.Theme.DisablePicker {
		return
	}

	choices := make([]*themeProperties, 0, len(a.Config.Theme.Presets.keys))

	for _, preset := range a.Config.Theme.Presets.Items() {
		choice, err := resolveTheme(preset, pageOverride)
		if err != nil {
			return
		}
		choices = append(choices, choice)
	}

	data.ThemeChoices = choices
}

func (a *application) renderPage(
	w http.ResponseWriter,
	r *http.Request,
	session authenticatedSession,
	page *page,
	navigationPages []*page,
	dashboard *dashboard,
	dashboardPath string,
) {
	data := templateData{
		App:             a,
		Page:            page,
		NavigationPages: navigationPages,
		Dashboards: a.authorization.authorizedDashboards(
			session.AuthorizationIdentity,
			a.dashboards,
		),
		Dashboard:     dashboard,
		DashboardPath: dashboardPath,
	}
	a.populateTemplateRequestData(&data.Request, r, page)

	if a.RequiresAuth && session.DisplayName != "" {
		data.Request.AuthDisplayName = session.DisplayName
		switch session.Method {
		case authMethodLocal:
			data.Request.AuthDescription = "Signed in locally"
		case authMethodOIDC:
			data.Request.AuthDescription = "Signed in with " + a.OIDCProviderName()
		}
	}

	var responseBytes bytes.Buffer
	err := pageTemplate.Execute(&responseBytes, data)
	if err != nil {
		writeInternalServerError(w, "Failed to render page", err)
		return
	}

	_, _ = w.Write(responseBytes.Bytes())
}

func (a *application) handlePageRequest(w http.ResponseWriter, r *http.Request) {
	session, authenticated := a.authorizeSession(w, r)
	if !authenticated {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
		return
	}

	if a.defaultDashboard == nil {
		page, exists := a.slugToPage[r.PathValue("page")]
		if !exists {
			a.renderNotFound(
				w,
				r,
				session,
				pagePointers(a.Config.Pages),
				nil,
				"",
			)
			return
		}

		a.renderPage(
			w,
			r,
			session,
			page,
			pagePointers(a.Config.Pages),
			nil,
			"",
		)
		return
	}

	identity := session.AuthorizationIdentity
	pageSlug := r.PathValue("page")

	if !a.authorization.canAccessDashboard(identity, a.defaultDashboard) {
		if pageSlug == "" {
			authorizedDashboards := a.authorization.authorizedDashboards(
				identity,
				a.dashboards,
			)
			if len(authorizedDashboards) > 0 {
				http.Redirect(
					w,
					r,
					a.Config.Server.BaseURL+"/"+authorizedDashboards[0].Slug+"/",
					http.StatusSeeOther,
				)
				return
			}
		}

		a.renderNotFound(w, r, session, nil, nil, "")
		return
	}

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
		a.renderNotFound(
			w,
			r,
			session,
			a.defaultDashboard.Pages,
			a.defaultDashboard,
			"",
		)
		return
	}

	a.renderPage(
		w,
		r,
		session,
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
	session, authenticated := a.authorizeSession(w, r)
	if !authenticated {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
		return
	}

	if !a.authorization.canAccessDashboard(
		session.AuthorizationIdentity,
		dashboard,
	) {
		a.renderNotFound(w, r, session, nil, nil, "")
		return
	}

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
		a.renderNotFound(
			w,
			r,
			session,
			dashboard.Pages,
			dashboard,
			"/"+dashboard.Slug,
		)
		return
	}

	a.renderPage(
		w,
		r,
		session,
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

	session, authenticated := a.authorizeSession(w, r)
	if !authenticated {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "Unauthorized"}`))
		return
	}

	if !a.authorization.canAccessPage(session.AuthorizationIdentity, page) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	pageData := templateData{
		Page: page,
	}

	var err error
	var responseBytes bytes.Buffer

	func() {
		lockWaitStarted := time.Now()
		page.mu.Lock()
		renderDiagnostics.recordPageLockWait(time.Since(lockWaitStarted))
		defer page.mu.Unlock()

		templateStarted := time.Now()
		err = pageContentTemplate.Execute(&responseBytes, pageData)
		renderDiagnostics.recordPageTemplateExecution(time.Since(templateStarted), err)
	}()

	if err != nil {
		writeInternalServerError(w, "Failed to render page content", err)
		return
	}

	_, _ = w.Write(responseBytes.Bytes())
}

func remoteAddressOfRequest(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return strings.Trim(r.RemoteAddr, "[]")
}

func (a *application) requestIsFromTrustedProxy(r *http.Request) bool {
	if !a.Config.Server.Proxied {
		return false
	}

	if len(a.trustedProxyPrefixes) == 0 {
		return true
	}

	remoteAddr, err := netip.ParseAddr(remoteAddressOfRequest(r))
	if err != nil {
		return false
	}

	for _, prefix := range a.trustedProxyPrefixes {
		if prefix.Contains(remoteAddr) {
			return true
		}
	}

	return false
}

func (a *application) requestIsSecure(r *http.Request) bool {
	if r.TLS != nil || a.Config.Server.HTTPS {
		return true
	}

	return a.requestIsFromTrustedProxy(r) &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (a *application) addressOfRequest(r *http.Request) string {
	remoteAddr := remoteAddressOfRequest(r)

	if !a.requestIsFromTrustedProxy(r) {
		return remoteAddr
	}

	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor == "" {
		return remoteAddr
	}

	ips := strings.Split(forwardedFor, ",")
	lastIP := strings.TrimSpace(ips[len(ips)-1])
	if lastIP == "" {
		return remoteAddr
	}

	return lastIP
}

func (a *application) renderNotFound(
	w http.ResponseWriter,
	r *http.Request,
	session authenticatedSession,
	navigationPages []*page,
	dashboard *dashboard,
	dashboardPath string,
) {
	data := templateData{
		App:             a,
		NavigationPages: navigationPages,
		Dashboards: a.authorization.authorizedDashboards(
			session.AuthorizationIdentity,
			a.dashboards,
		),
		Dashboard:     dashboard,
		DashboardPath: dashboardPath,
	}
	a.populateTemplateRequestData(&data.Request, r, nil)

	if a.RequiresAuth && session.DisplayName != "" {
		data.Request.AuthDisplayName = session.DisplayName
		switch session.Method {
		case authMethodLocal:
			data.Request.AuthDescription = "Signed in locally"
		case authMethodOIDC:
			data.Request.AuthDescription = "Signed in with " + a.OIDCProviderName()
		}
	}

	var responseBytes bytes.Buffer
	if err := notFoundTemplate.Execute(&responseBytes, data); err != nil {
		writeInternalServerError(w, "Failed to render not found page", err)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(responseBytes.Bytes())
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
	_, _ = w.Write([]byte(renderWidget(widget)))
}

func (a *application) handleWidgetRequest(w http.ResponseWriter, r *http.Request) {
	// Generic widget subrequests remain intentionally disabled; supported widget content requests use their dedicated handlers.
	w.WriteHeader(http.StatusNotImplemented)
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
					session, authenticated := a.authorizeSession(w, r)
					if !authenticated {
						http.Redirect(
							w,
							r,
							a.Config.Server.BaseURL+"/login",
							http.StatusSeeOther,
						)
						return
					}

					if !a.authorization.canAccessDashboard(
						session.AuthorizationIdentity,
						dashboard,
					) {
						a.renderNotFound(w, r, session, nil, nil, "")
						return
					}

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
	mux.HandleFunc("GET /api/resource-proxy/{resource}", a.handleResourceProxyRequest)
	mux.HandleFunc("GET /api/live-updates", a.handleLiveUpdatesRequest)
	mux.HandleFunc("POST /api/frontend-diagnostics", a.handleFrontendDiagnosticsRequest)
	mux.HandleFunc(
		"POST /api/frontend-diagnostics/performance-snapshot",
		a.handleFrontendPerformanceSnapshotRequest,
	)
	mux.HandleFunc(
		"POST /api/frontend-diagnostics/long-task-capture",
		a.handleFrontendLongTaskCaptureRequest,
	)
	mux.HandleFunc(
		"POST /api/frontend-diagnostics/runtime-state",
		a.handleFrontendRuntimeStateRequest,
	)
	mux.HandleFunc("GET /api/diagnostics", a.handleRuntimeDiagnosticsRequest)
	mux.HandleFunc("GET /api/diagnostics/report", a.handleRuntimeDiagnosticsReportRequest)
	mux.HandleFunc("/api/widgets/{widget}/{path...}", a.handleWidgetRequest)
	mux.HandleFunc("GET /api/healthz", a.handleHealthzRequest)

	if a.RequiresAuth {
		mux.HandleFunc("GET /login", a.handleLoginPageRequest)
		mux.HandleFunc("GET /logout", a.handleLogoutRequest)

		if len(a.Config.Auth.Users) > 0 {
			mux.HandleFunc("POST /api/authenticate", a.handleAuthenticationAttempt)
		}

		if a.oidc != nil {
			mux.HandleFunc("GET /auth/oidc/login", a.handleOIDCLoginRequest)
			mux.HandleFunc("GET /auth/oidc/callback", a.handleOIDCCallbackRequest)
		}
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
		_, _ = w.Write(bundledCSSContents)
	})

	mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", assetCacheControlValue)
		w.Header().Add("Content-Type", "application/json")
		_, _ = w.Write(a.parsedManifest)
	})

	if a.Config.Server.AssetsPath != "" {
		assetsFS := fileServerWithCache(http.Dir(a.Config.Server.AssetsPath), 2*time.Hour)
		mux.Handle("/assets/{path...}", http.StripPrefix("/assets/", assetsFS))
	}

	var handler http.Handler = mux
	if a.Config.Server.FrontendDiagnostics {
		handler = a.frontendDiagnosticHTTPPerformanceHandler(handler)
	}

	return securityHeadersHandler(handler)
}

func securityHeadersHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func frontendDiagnosticProfileHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return mux
}

func (s *processServer) processDiagnosticProfileHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", frontendDiagnosticProfileHandler())
	mux.HandleFunc("POST /debug/frontend-diagnostics/performance-snapshot", s.handleInternalFrontendDiagnosticCommand("performance_snapshot"))
	mux.HandleFunc("POST /debug/frontend-diagnostics/long-task-capture", s.handleInternalFrontendDiagnosticCommand("long_task_capture"))
	mux.HandleFunc("POST /debug/frontend-diagnostics/runtime-state", s.handleInternalFrontendDiagnosticCommand("runtime_state"))
	mux.HandleFunc("GET /debug/frontend-diagnostics/results/{commandID}", s.handleInternalFrontendDiagnosticResults)

	return mux
}

func (s *processServer) handleInternalFrontendDiagnosticResults(w http.ResponseWriter, r *http.Request) {
	app := s.activeApplication.Load()
	if app == nil || !app.Config.Server.FrontendDiagnostics {
		http.NotFound(w, r)
		return
	}

	commandID, err := strconv.ParseUint(r.PathValue("commandID"), 10, 64)
	if err != nil || commandID == 0 {
		http.Error(w, "Invalid command ID", http.StatusBadRequest)
		return
	}

	results := app.frontendDiagnostics.activeResultsForCommand(commandID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		slog.Error(
			"Frontend diagnostic result encoding failed",
			"command_id", commandID,
			"error", err,
		)
	}
}

func (s *processServer) handleInternalFrontendDiagnosticCommand(commandName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app := s.activeApplication.Load()
		if app == nil || !app.Config.Server.FrontendDiagnostics {
			http.NotFound(w, r)
			return
		}

		command, err := app.publishFrontendDiagnosticCommand(commandName)
		if err != nil {
			http.Error(w, "Live updates unavailable", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(
			w,
			`{"id":%d,"command":"%s"}`,
			command.ID,
			command.Command,
		)
	}
}
