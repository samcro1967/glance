package glance

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	c, err := newConfigFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("newConfigFromYAML() unexpected error = %v", err)
	}

	_, err = newApplication(c)
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

	c, err := newConfigFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("newConfigFromYAML() unexpected error = %v", err)
	}

	_, err = newApplication(c)
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

	c, err := newConfigFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("newConfigFromYAML() unexpected error = %v", err)
	}

	_, err = newApplication(c)
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

func TestPageSlugCanMatchDashboardSlug(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
dashboards:
  Default:
    - home
    - page2

  Page2:
    - shared
    - page3
`))

	// /page2 is the Page 2 page in the Default dashboard.
	defaultReq := httptest.NewRequest(http.MethodGet, "/page2", nil)
	defaultReq.SetPathValue("page", "page2")
	defaultRec := httptest.NewRecorder()

	app.handlePageRequest(defaultRec, defaultReq)

	if defaultRec.Code != http.StatusOK {
		t.Fatalf(
			"GET /page2 status = %d, want %d",
			defaultRec.Code,
			http.StatusOK,
		)
	}

	if !strings.Contains(defaultRec.Body.String(), "Page 2") {
		t.Fatal("GET /page2 did not render the Default Page 2 page")
	}

	// /page2/ is the home of the Page2 named dashboard.
	page2Dashboard := app.slugToDashboard["page2"]
	dashboardReq := httptest.NewRequest(http.MethodGet, "/page2/", nil)
	dashboardRec := httptest.NewRecorder()

	app.handleDashboardPageRequest(page2Dashboard, dashboardRec, dashboardReq)

	if dashboardRec.Code != http.StatusOK {
		t.Fatalf(
			"GET /page2/ status = %d, want %d",
			dashboardRec.Code,
			http.StatusOK,
		)
	}

	if !strings.Contains(dashboardRec.Body.String(), "Shared") {
		t.Fatal("GET /page2/ did not render the named dashboard home")
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

  Page2:
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
			name:        "same slug dashboard home",
			path:        "/page2/",
			wantStatus:  http.StatusOK,
			wantContent: "Shared",
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
