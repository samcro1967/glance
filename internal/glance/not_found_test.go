package glance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestThemedNotFoundPage(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets: []
  - name: News
    slug: news
    columns:
      - size: full
        widgets: []
`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	app.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	body := rec.Body.String()

	for _, want := range []string{
		"<title>Page not found</title>",
		">404<",
		">Page not found<",
		"Available pages",
		`href="/home"`,
		`href="/news"`,
		">Home<",
		">News<",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response does not contain %q", want)
		}
	}
}

func TestThemedNotFoundPageRespectsBaseURL(t *testing.T) {
	app := newGlanceTestApplication(t, `
server:
  base-url: /glance

pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets: []
  - name: News
    slug: news
    columns:
      - size: full
        widgets: []
`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.SetPathValue("page", "missing")
	app.handlePageRequest(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	body := rec.Body.String()

	for _, want := range []string{
		`href="/glance/home"`,
		`href="/glance/news"`,
		`/glance/static/`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response does not contain base-url value %q", want)
		}
	}
}

func TestNamedDashboardThemedNotFoundUsesDashboardPages(t *testing.T) {
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

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/personal/missing", nil)
	req.SetPathValue("page", "missing")

	app.handleDashboardPageRequest(personal, rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	body := rec.Body.String()

	for _, want := range []string{
		`href="/personal/page2"`,
		`href="/personal/shared"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response does not contain dashboard page link %q", want)
		}
	}

	if strings.Contains(body, `href="/personal/home"`) {
		t.Error("response contains page not assigned to named dashboard")
	}
}

func TestNamedDashboardThemedNotFoundRespectsBaseURL(t *testing.T) {
	app := newDashboardTestApplication(t, dashboardTestYAML(`
server:
  base-url: /glance

dashboards:
  Default:
    - home
    - page2

  Personal:
    - page2
    - shared
`))

	personal := app.slugToDashboard["personal"]

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/personal/missing", nil)
	req.SetPathValue("page", "missing")

	app.handleDashboardPageRequest(personal, rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	body := rec.Body.String()

	for _, want := range []string{
		`href="/glance/personal/page2"`,
		`href="/glance/personal/shared"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response does not contain base-url dashboard link %q", want)
		}
	}
}

func TestPageContentNotFoundRemainsPlainResponse(t *testing.T) {
	app := newGlanceTestApplication(t, `
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets: []
`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/pages/missing/content/",
		nil,
	)
	app.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	if body := rec.Body.String(); body != "" {
		t.Fatalf("API 404 body = %q, want empty response", body)
	}
}
