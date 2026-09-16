package glance

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newDashboardTestConfig(t *testing.T, yaml string) *config {
	t.Helper()

	c, err := newConfigFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("newConfigFromYAML() error = %v", err)
	}

	return c
}

func newDashboardTestApplication(t *testing.T, yaml string) *application {
	t.Helper()

	c := newDashboardTestConfig(t, yaml)

	app, err := newApplication(c)
	if err != nil {
		t.Fatalf("newApplication() error = %v", err)
	}

	return app
}

func dashboardTestYAML(dashboards string) string {
	return `
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets: []

  - name: Page 2
    slug: page2
    columns:
      - size: full
        widgets: []

  - name: Page 3
    slug: page3
    columns:
      - size: full
        widgets: []

  - name: Shared
    slug: shared
    columns:
      - size: full
        widgets: []
` + dashboards
}

func TestNamedDashboardsAreOptional(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(""))

	if app.defaultDashboard != nil {
		t.Fatal("defaultDashboard should be nil when dashboards are not configured")
	}

	if len(app.slugToDashboard) != 0 {
		t.Fatalf("slugToDashboard has %d entries, want 0", len(app.slugToDashboard))
	}

	if app.slugToPage[""] != app.slugToPage["home"] {
		t.Fatal("legacy home route should point to the first configured page")
	}
}

func TestNamedDashboardsRequireDefault(t *testing.T) {
	yaml := dashboardTestYAML(`
dashboards:
  Personal:
    - home
    - page2
`)

	_, err := newConfigFromYAML([]byte(yaml))
	if err == nil {
		t.Fatal("expected dashboards without Default to be rejected")
	}

	if !strings.Contains(err.Error(), "Default") {
		t.Fatalf("error = %q, want error mentioning Default", err)
	}
}

func TestNamedDashboardCannotBeEmpty(t *testing.T) {
	yaml := dashboardTestYAML(`
dashboards:
  Default:
    - home

  Personal: []
`)

	_, err := newConfigFromYAML([]byte(yaml))
	if err == nil {
		t.Fatal("expected empty dashboard to be rejected")
	}
}

func TestNamedDashboardCannotReferenceUnknownPage(t *testing.T) {
	yaml := dashboardTestYAML(`
dashboards:
  Default:
    - home
    - missing
`)

	_, err := newConfigFromYAML([]byte(yaml))
	if err == nil {
		t.Fatal("expected unknown dashboard page reference to be rejected")
	}

	if !strings.Contains(err.Error(), "unknown page slug") {
		t.Fatalf("error = %q, want unknown page slug error", err)
	}
}

func TestNamedDashboardCannotContainDuplicatePage(t *testing.T) {
	yaml := dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2
    - page2
`)

	_, err := newConfigFromYAML([]byte(yaml))
	if err == nil {
		t.Fatal("expected duplicate page reference to be rejected")
	}
}

func TestNamedDashboardSlugsMustBeUnique(t *testing.T) {
	yaml := dashboardTestYAML(`
dashboards:
  Default:
    - home

  Page Two:
    - page2

  Page-Two:
    - page3
`)

	_, err := newConfigFromYAML([]byte(yaml))
	if err == nil {
		t.Fatal("expected duplicate generated dashboard slug to be rejected")
	}

	if !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("error = %q, want duplicate dashboard slug error", err)
	}
}

func TestNamedDashboardReservedSlugIsRejected(t *testing.T) {
	yaml := dashboardTestYAML(`
dashboards:
  Default:
    - home

  API:
    - page2
`)

	_, err := newConfigFromYAML([]byte(yaml))
	if err == nil {
		t.Fatal("expected reserved dashboard slug to be rejected")
	}

	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("error = %q, want reserved dashboard slug error", err)
	}
}

func TestNamedDashboardConstruction(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - page2
    - home
    - shared

  Personal:
    - home
    - page2
    - shared

  Family:
    - home
    - page3
    - shared
`))

	if app.defaultDashboard == nil {
		t.Fatal("defaultDashboard is nil")
	}

	if app.defaultDashboard.Slug != "" {
		t.Fatalf("default dashboard slug = %q, want empty string", app.defaultDashboard.Slug)
	}

	if len(app.defaultDashboard.Pages) != 3 {
		t.Fatalf("default dashboard has %d pages, want 3", len(app.defaultDashboard.Pages))
	}

	if app.defaultDashboard.Pages[0].Slug != "page2" {
		t.Fatalf(
			"default dashboard first page = %q, want page2",
			app.defaultDashboard.Pages[0].Slug,
		)
	}

	personal, exists := app.slugToDashboard["personal"]
	if !exists {
		t.Fatal("personal dashboard was not created")
	}

	if len(personal.Pages) != 3 {
		t.Fatalf("personal dashboard has %d pages, want 3", len(personal.Pages))
	}

	if personal.Pages[0] != app.slugToPage["home"] {
		t.Fatal("dashboard should reference the canonical Home page")
	}

	if personal.Pages[1] != app.slugToPage["page2"] {
		t.Fatal("dashboard should reference the canonical Page 2 page")
	}

	family, exists := app.slugToDashboard["family"]
	if !exists {
		t.Fatal("family dashboard was not created")
	}

	if family.Pages[2] != app.slugToPage["shared"] {
		t.Fatal("shared page should reference the canonical Shared page")
	}

	if personal.Pages[2] != family.Pages[2] {
		t.Fatal("shared page should be the same canonical page in both dashboards")
	}
}

func TestNamedDashboardPageSlugCollisionIsIgnored(t *testing.T) {
	yaml := dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2
    - page3

  Page2:
    - shared
    - page3

  Personal:
    - home
    - page3
`)

	c := newDashboardTestConfig(t, yaml)

	var logOutput bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logOutput)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
	})

	app, err := newApplication(c)
	if err != nil {
		t.Fatalf("newApplication() error = %v", err)
	}

	if app.defaultDashboard == nil {
		t.Fatal("Default dashboard should still be created")
	}

	if _, exists := app.slugToDashboard["page2"]; exists {
		t.Fatal("dashboard whose slug conflicts with page slug should be ignored")
	}

	if _, exists := app.slugToDashboard["personal"]; !exists {
		t.Fatal("non-conflicting dashboard should still be created")
	}

	if app.slugToPage["page2"] == nil {
		t.Fatal("canonical page should remain available")
	}

	warning := logOutput.String()

	if !strings.Contains(warning, "WARN Ignoring dashboard because its slug conflicts with a page slug") {
		t.Fatalf(
			"log output = %q, want dashboard slug collision warning",
			warning,
		)
	}

	if !strings.Contains(warning, "dashboard=Page2") {
		t.Fatalf(
			"log output = %q, want dashboard field",
			warning,
		)
	}

	if !strings.Contains(warning, "slug=page2") {
		t.Fatalf(
			"log output = %q, want slug field",
			warning,
		)
	}
}

func TestDefaultDashboardHomeUsesFirstAssignedPage(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - page2
    - home
    - shared
`))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	app.handlePageRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()

	if !strings.Contains(body, "Page 2") {
		t.Fatal("GET / did not render the first page assigned to Default")
	}
}

func TestDefaultDashboardRejectsUnassignedPage(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2

  Personal:
    - home
    - page3
`))

	req := httptest.NewRequest(http.MethodGet, "/page3", nil)
	req.SetPathValue("page", "page3")
	rec := httptest.NewRecorder()

	app.handlePageRequest(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf(
			"GET /page3 status = %d, want %d",
			rec.Code,
			http.StatusNotFound,
		)
	}
}

func TestNamedDashboardHomeUsesFirstAssignedPage(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2

  Personal:
    - page2
    - shared
`))

	personal := app.slugToDashboard["personal"]

	req := httptest.NewRequest(http.MethodGet, "/personal/", nil)
	rec := httptest.NewRecorder()

	app.handleDashboardPageRequest(personal, rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"GET /personal/ status = %d, want %d",
			rec.Code,
			http.StatusOK,
		)
	}

	body := rec.Body.String()

	if !strings.Contains(body, "Page 2") {
		t.Fatal("named dashboard home did not render its first assigned page")
	}
}

func TestNamedDashboardPageMembership(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2
    - page3

  Personal:
    - home
    - page2
`))

	personal := app.slugToDashboard["personal"]

	tests := []struct {
		name       string
		page       string
		wantStatus int
	}{
		{
			name:       "assigned page",
			page:       "page2",
			wantStatus: http.StatusOK,
		},
		{
			name:       "unassigned page",
			page:       "page3",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unknown page",
			page:       "missing",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodGet,
				"/personal/"+tt.page,
				nil,
			)
			req.SetPathValue("page", tt.page)

			rec := httptest.NewRecorder()

			app.handleDashboardPageRequest(personal, rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d",
					rec.Code,
					tt.wantStatus,
				)
			}
		})
	}
}

func TestNamedDashboardNavigation(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2
    - page3
    - shared

  Personal:
    - home
    - page2
    - shared
`))

	personal := app.slugToDashboard["personal"]

	req := httptest.NewRequest(http.MethodGet, "/personal/page2", nil)
	req.SetPathValue("page", "page2")
	rec := httptest.NewRecorder()

	app.handleDashboardPageRequest(personal, rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()

	expectedLinks := []string{
		`href="/personal/home"`,
		`href="/personal/page2"`,
		`href="/personal/shared"`,
	}

	for _, link := range expectedLinks {
		if !strings.Contains(body, link) {
			t.Errorf("response does not contain dashboard navigation link %s", link)
		}
	}

	if strings.Contains(body, `href="/personal/page3"`) {
		t.Error("response contains navigation for page not assigned to dashboard")
	}
}

func TestNamedDashboardNavigationWithBaseURL(t *testing.T) {
	yaml := dashboardTestYAML(`
server:
  base-url: /glance

dashboards:
  Default:
    - home
    - page2

  Personal:
    - home
    - page2
`)

	app := newDashboardTestApplication(t, yaml)
	personal := app.slugToDashboard["personal"]

	req := httptest.NewRequest(http.MethodGet, "/personal/page2", nil)
	req.SetPathValue("page", "page2")
	rec := httptest.NewRecorder()

	app.handleDashboardPageRequest(personal, rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()

	expectedLinks := []string{
		`href="/glance/personal/home"`,
		`href="/glance/personal/page2"`,
	}

	for _, link := range expectedLinks {
		if !strings.Contains(body, link) {
			t.Errorf("response does not contain base-url navigation link %s", link)
		}
	}
}

func TestNamedDashboardWithoutTrailingSlashRedirects(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2

  Personal:
    - home
    - page2
`))

	handler := app.router()

	req := httptest.NewRequest(http.MethodGet, "/personal", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf(
			"GET /personal status = %d, want %d",
			rec.Code,
			http.StatusMovedPermanently,
		)
	}

	if location := rec.Header().Get("Location"); location != "/personal/" {
		t.Fatalf(
			"GET /personal Location = %q, want %q",
			location,
			"/personal/",
		)
	}
}

func TestNamedDashboardWithoutTrailingSlashRedirectsWithBaseURL(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
server:
  base-url: /glance

dashboards:
  Default:
    - home
    - page2

  Personal:
    - home
    - page2
`))

	handler := app.router()

	req := httptest.NewRequest(http.MethodGet, "/personal", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf(
			"GET /personal status = %d, want %d",
			rec.Code,
			http.StatusMovedPermanently,
		)
	}

	if location := rec.Header().Get("Location"); location != "/glance/personal/" {
		t.Fatalf(
			"GET /personal Location = %q, want %q",
			location,
			"/glance/personal/",
		)
	}
}

func TestDashboardPageSlugCollisionPreservesPageRoute(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2
    - page3

  Page2:
    - shared
    - page3

  Personal:
    - home
    - page3
`))

	handler := app.router()

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantContent string
	}{
		{
			name:        "canonical page remains available",
			path:        "/page2",
			wantStatus:  http.StatusOK,
			wantContent: "Page 2",
		},
		{
			name:       "conflicting dashboard home is unavailable",
			path:       "/page2/",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "conflicting dashboard page is unavailable",
			path:       "/page2/shared",
			wantStatus: http.StatusNotFound,
		},
		{
			name:        "other dashboard remains available",
			path:        "/personal/",
			wantStatus:  http.StatusOK,
			wantContent: "Home",
		},
		{
			name:        "other dashboard page remains available",
			path:        "/personal/page3",
			wantStatus:  http.StatusOK,
			wantContent: "Page 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"GET %s status = %d, want %d",
					tt.path,
					rec.Code,
					tt.wantStatus,
				)
			}

			if tt.wantContent != "" && !strings.Contains(rec.Body.String(), tt.wantContent) {
				t.Fatalf(
					"GET %s response does not contain %q",
					tt.path,
					tt.wantContent,
				)
			}
		})
	}
}

func TestDashboardRouter(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2

  Personal:
    - home
    - page2

  Family:
    - shared
    - page3
`))

	handler := app.router()

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantContent string
	}{
		{
			name:        "default home",
			path:        "/",
			wantStatus:  http.StatusOK,
			wantContent: "Home",
		},
		{
			name:        "default page",
			path:        "/page2",
			wantStatus:  http.StatusOK,
			wantContent: "Page 2",
		},
		{
			name:        "named dashboard home",
			path:        "/personal/",
			wantStatus:  http.StatusOK,
			wantContent: "Home",
		},
		{
			name:        "named dashboard page",
			path:        "/personal/page2",
			wantStatus:  http.StatusOK,
			wantContent: "Page 2",
		},
		{
			name:        "second named dashboard home",
			path:        "/family/",
			wantStatus:  http.StatusOK,
			wantContent: "Shared",
		},
		{
			name:        "second named dashboard page",
			path:        "/family/page3",
			wantStatus:  http.StatusOK,
			wantContent: "Page 3",
		},
		{
			name:       "page not assigned to named dashboard",
			path:       "/personal/page3",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unknown dashboard",
			path:       "/missing/",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"GET %s status = %d, want %d",
					tt.path,
					rec.Code,
					tt.wantStatus,
				)
			}

			if tt.wantContent != "" && !strings.Contains(rec.Body.String(), tt.wantContent) {
				t.Fatalf(
					"GET %s response does not contain %q",
					tt.path,
					tt.wantContent,
				)
			}
		})
	}
}

func TestDashboardRouterWithAssetsPath(t *testing.T) {
	assetsDir := t.TempDir()
	assetPath := filepath.Join(assetsDir, "dashboard-test.txt")
	assetContents := "dashboard assets route"

	if err := os.WriteFile(assetPath, []byte(assetContents), 0o644); err != nil {
		t.Fatalf("writing test asset: %v", err)
	}

	yaml := dashboardTestYAML(`
server:
  assets-path: ` + assetsDir + `

dashboards:
  Default:
    - home
    - page2

  Personal:
    - home
    - page2
`)

	app := newDashboardTestApplication(t, yaml)

	var handler http.Handler
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("router() panicked with dashboards and assets-path: %v", recovered)
			}
		}()

		handler = app.router()
	}()

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantContent string
	}{
		{
			name:        "default dashboard remains available",
			path:        "/",
			wantStatus:  http.StatusOK,
			wantContent: "Home",
		},
		{
			name:        "named dashboard remains available",
			path:        "/personal/",
			wantStatus:  http.StatusOK,
			wantContent: "Home",
		},
		{
			name:        "named dashboard page remains available",
			path:        "/personal/page2",
			wantStatus:  http.StatusOK,
			wantContent: "Page 2",
		},
		{
			name:        "custom asset remains available",
			path:        "/assets/dashboard-test.txt",
			wantStatus:  http.StatusOK,
			wantContent: assetContents,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"GET %s status = %d, want %d",
					tt.path,
					rec.Code,
					tt.wantStatus,
				)
			}

			if !strings.Contains(rec.Body.String(), tt.wantContent) {
				t.Fatalf(
					"GET %s response does not contain %q",
					tt.path,
					tt.wantContent,
				)
			}
		})
	}
}

func TestLegacyRouterWithoutDashboards(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(""))
	handler := app.router()

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantContent string
	}{
		{
			name:        "legacy home",
			path:        "/",
			wantStatus:  http.StatusOK,
			wantContent: "Home",
		},
		{
			name:        "legacy page",
			path:        "/page2",
			wantStatus:  http.StatusOK,
			wantContent: "Page 2",
		},
		{
			name:       "dashboard-style route remains unavailable",
			path:       "/personal/page2",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"GET %s status = %d, want %d",
					tt.path,
					rec.Code,
					tt.wantStatus,
				)
			}

			if tt.wantContent != "" && !strings.Contains(rec.Body.String(), tt.wantContent) {
				t.Fatalf(
					"GET %s response does not contain %q",
					tt.path,
					tt.wantContent,
				)
			}
		})
	}
}

func dashboardAuthorizationTestYAML(t *testing.T) string {
	t.Helper()

	secret, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatalf("generating auth secret: %v", err)
	}

	return dashboardTestYAML(fmt.Sprintf(`
auth:
  secret-key: %s
  users:
    mark:
      password-hash: unused
    kellie:
      password-hash: unused

  groups:
    family:
      users:
        - mark
        - kellie

    admins:
      users:
        - mark

  access:
    dashboards:
      Default:
        groups:
          - family

      Personal:
        groups:
          - admins

dashboards:
  Default:
    - home
    - page2
    - shared

  Personal:
    - page3
    - shared
`, secret))
}

func addDashboardAuthorizationSession(
	t *testing.T,
	app *application,
	req *http.Request,
	username string,
) {
	t.Helper()

	token, err := generateSessionTokenV4(
		authMethodLocal,
		username,
		username,
		username,
		app.authSecretKey,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("generateSessionTokenV4() error = %v", err)
	}

	req.AddCookie(&http.Cookie{
		Name:  AUTH_SESSION_COOKIE_NAME,
		Value: token,
	})
}

func TestDashboardAuthorizationRoutes(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardAuthorizationTestYAML(t))
	router := app.router()

	tests := []struct {
		name         string
		username     string
		path         string
		wantStatus   int
		wantLocation string
	}{
		{
			name:       "family user accesses default page",
			username:   "kellie",
			path:       "/page2",
			wantStatus: http.StatusOK,
		},
		{
			name:       "family user denied personal dashboard",
			username:   "kellie",
			path:       "/personal/page3",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "admin accesses personal dashboard",
			username:   "mark",
			path:       "/personal/page3",
			wantStatus: http.StatusOK,
		},
		{
			name:         "authorized no slash dashboard redirects",
			username:     "mark",
			path:         "/personal",
			wantStatus:   http.StatusMovedPermanently,
			wantLocation: "/personal/",
		},
		{
			name:       "unauthorized no slash dashboard does not redirect",
			username:   "kellie",
			path:       "/personal",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "named dashboard page does not become default alias",
			username:   "mark",
			path:       "/page3",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			addDashboardAuthorizationSession(t, app, req, tt.username)

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body=%q",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}
			if tt.wantLocation != "" &&
				rec.Header().Get("Location") != tt.wantLocation {
				t.Fatalf(
					"Location = %q, want %q",
					rec.Header().Get("Location"),
					tt.wantLocation,
				)
			}
		})
	}
}

func TestDashboardAuthorizationFiltersDashboardNavigation(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardAuthorizationTestYAML(t))
	router := app.router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	addDashboardAuthorizationSession(t, app, req, "kellie")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d; body=%q",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	body := rec.Body.String()
	if !strings.Contains(body, ">Default<") {
		t.Fatal("authorized Default dashboard missing from navigation")
	}
	if strings.Contains(body, ">Personal<") ||
		strings.Contains(body, `href="/personal/"`) {
		t.Fatal("unauthorized Personal dashboard exposed in navigation")
	}
}

func TestDashboardAuthorizationPageContentUsesAnyContainingDashboard(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardAuthorizationTestYAML(t))
	router := app.router()

	tests := []struct {
		name       string
		username   string
		page       string
		wantStatus int
	}{
		{
			name:       "shared page granted through default",
			username:   "kellie",
			page:       "shared",
			wantStatus: http.StatusOK,
		},
		{
			name:       "personal only page denied",
			username:   "kellie",
			page:       "page3",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "personal only page granted to admin",
			username:   "mark",
			page:       "page3",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(
				http.MethodGet,
				"/api/pages/"+tt.page+"/content/",
				nil,
			)
			addDashboardAuthorizationSession(t, app, req, tt.username)
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body=%q",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}
		})
	}
}

func TestDashboardAuthorizationUnauthenticatedBehavior(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardAuthorizationTestYAML(t))
	router := app.router()

	t.Run("HTML redirects to login", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/personal/page3", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
		}
		if got := rec.Header().Get("Location"); got != "/login" {
			t.Fatalf("Location = %q, want %q", got, "/login")
		}
	})

	t.Run("page content returns unauthorized", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			"/api/pages/page3/content/",
			nil,
		)
		router.ServeHTTP(rec, req)

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
}

func TestDashboardAuthorizationRootFallsBackToFirstAuthorizedDashboard(
	t *testing.T,
) {
	secret, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatalf("generating auth secret: %v", err)
	}

	yaml := dashboardTestYAML(fmt.Sprintf(`
auth:
  secret-key: %s
  users:
    mark:
      password-hash: unused

  access:
    dashboards:
      Personal:
        users:
          - mark

dashboards:
  Default:
    - home

  Personal:
    - page3
`, secret))
	app := newDashboardTestApplication(t, yaml)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	addDashboardAuthorizationSession(t, app, req, "mark")
	app.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/personal/" {
		t.Fatalf("Location = %q, want %q", got, "/personal/")
	}
}

func TestDashboardAuthorizationRootNotFoundWhenNoDashboardAuthorized(
	t *testing.T,
) {
	secret, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatalf("generating auth secret: %v", err)
	}

	yaml := dashboardTestYAML(fmt.Sprintf(`
auth:
  secret-key: %s
  users:
    kellie:
      password-hash: unused

  access:
    dashboards:
      Personal:
        users:
          - mark

dashboards:
  Default:
    - home

  Personal:
    - page3
`, secret))
	app := newDashboardTestApplication(t, yaml)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	addDashboardAuthorizationSession(t, app, req, "kellie")
	app.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf(
			"status = %d, want %d; body=%q",
			rec.Code,
			http.StatusNotFound,
			rec.Body.String(),
		)
	}
	if strings.Contains(rec.Body.String(), "Personal") {
		t.Fatal("unauthorized dashboard exposed in not-found response")
	}
}
