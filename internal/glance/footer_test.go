package glance

import (
	"bytes"
	"strings"
	"testing"
)

const (
	testInstalledForkVersion = "v1.2.3-samcro1967.r004"
	testLatestForkVersion    = "v1.2.3-samcro1967.r005"
	testLatestReleaseURL     = "https://example.invalid/releases/r005"
)

func renderFooterForTest(t *testing.T, app *application) string {
	t.Helper()

	var rendered bytes.Buffer
	err := pageTemplate.ExecuteTemplate(
		&rendered,
		"footer.html",
		templateData{
			App: app,
		},
	)
	if err != nil {
		t.Fatalf("render footer: %v", err)
	}

	return rendered.String()
}

func TestFooterReleaseStatusDev(t *testing.T) {
	app := &application{
		Version: "dev",
	}

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, ">Glance</a>") ||
		!strings.Contains(rendered, "(dev)") {
		t.Fatalf("footer does not contain development version: %q", rendered)
	}

	if strings.Contains(rendered, "Latest") {
		t.Fatalf("development footer contains latest status: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("development footer contains update status: %q", rendered)
	}
}

func TestFooterReleaseStatusDevWithRevision(t *testing.T) {
	app := &application{
		Version:       "dev",
		ShortRevision: "8534072",
	}

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, "(dev 8534072)") {
		t.Fatalf("footer does not contain development revision: %q", rendered)
	}

	if strings.Contains(rendered, "Latest") {
		t.Fatalf("development footer contains latest status: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("development footer contains update status: %q", rendered)
	}
}

func TestFooterReleaseStatusLatest(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}

	app.releaseStatus.set(releaseStatusResult{
		Status:        releaseStatusLatest,
		LatestVersion: testInstalledForkVersion,
		ReleaseURL:    testLatestReleaseURL,
	})

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, testInstalledForkVersion) ||
		!strings.Contains(rendered, `· <span class="color-positive">Latest</span>`) {
		t.Fatalf("footer does not contain latest status: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("latest footer contains update status: %q", rendered)
	}
}

func TestFooterReleaseStatusUpdateAvailable(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}

	app.releaseStatus.set(releaseStatusResult{
		Status:        releaseStatusUpdateAvailable,
		LatestVersion: testLatestForkVersion,
		ReleaseURL:    testLatestReleaseURL,
	})

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, testInstalledForkVersion) {
		t.Fatalf("footer does not contain installed version: %q", rendered)
	}

	want := `· <a class="color-primary" href="` +
		testLatestReleaseURL +
		`" target="_blank" rel="noreferrer">Update available</a>`

	if !strings.Contains(rendered, want) {
		t.Fatalf("footer does not contain linked update status: %q", rendered)
	}

	if strings.Contains(rendered, ">Latest<") {
		t.Fatalf("update footer contains latest status: %q", rendered)
	}
}

func TestFooterReleaseStatusUnknown(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, testInstalledForkVersion) {
		t.Fatalf("footer does not contain installed version: %q", rendered)
	}

	if strings.Contains(rendered, "Latest") {
		t.Fatalf("unknown footer contains latest status: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("unknown footer contains update status: %q", rendered)
	}
}

func TestFooterReleaseStatusCustomFooterUnaffected(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}
	app.Config.Branding.CustomFooter = "Custom footer"

	app.releaseStatus.set(releaseStatusResult{
		Status: releaseStatusUpdateAvailable,
	})

	rendered := renderFooterForTest(t, app)

	if !strings.Contains(rendered, "Custom footer") {
		t.Fatalf("custom footer missing: %q", rendered)
	}

	if strings.Contains(rendered, "Update available") {
		t.Fatalf("custom footer contains release status: %q", rendered)
	}
}

func TestFooterReleaseStatusHiddenFooterUnaffected(t *testing.T) {
	app := &application{
		Version: testInstalledForkVersion,
	}
	app.Config.Branding.HideFooter = true

	rendered := renderFooterForTest(t, app)

	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("hidden footer rendered content: %q", rendered)
	}
}
