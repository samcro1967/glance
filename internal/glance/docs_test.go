package glance

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestCleanDocsDocumentPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"configuration", "configuration.md", true},
		{"widgets/monitor", "widgets/monitor.md", true},
		{"widgets/monitor.md", "widgets/monitor.md", true},
		{"../configuration", "", false},
		{"widgets/../configuration", "", false},
		{"/configuration", "", false},
		{`widgets\\monitor`, "", false},
		{"images/example.png", "", false},
	}

	for _, test := range tests {
		got, ok := cleanDocsDocumentPath(test.input)
		if got != test.want || ok != test.ok {
			t.Errorf("cleanDocsDocumentPath(%q) = (%q, %v), want (%q, %v)",
				test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestReadDocsDocument(t *testing.T) {
	source, filePath, err := readDocsDocument("configuration")
	if err != nil {
		t.Fatal(err)
	}
	if filePath != "configuration.md" {
		t.Fatalf("file path = %q, want configuration.md", filePath)
	}
	if len(source) == 0 {
		t.Fatal("embedded configuration documentation is empty")
	}

	nested, nestedPath, err := readDocsDocument(
		"examples/custom-api/monitoring/integrations/argus/README",
	)
	if err != nil {
		t.Fatal(err)
	}
	if nestedPath != "examples/custom-api/monitoring/integrations/argus/README.md" {
		t.Fatalf("nested file path = %q, want embedded integration README", nestedPath)
	}
	if len(nested) == 0 {
		t.Fatal("embedded nested documentation is empty")
	}

	oldContributing := docsContributing
	docsContributing = []byte("# Contributing")
	t.Cleanup(func() {
		docsContributing = oldContributing
	})

	contributing, contributingPath, err := readDocsDocument("contributing")
	if err != nil {
		t.Fatal(err)
	}
	if contributingPath != "CONTRIBUTING.md" {
		t.Fatalf("file path = %q, want CONTRIBUTING.md", contributingPath)
	}
	if string(contributing) != "# Contributing" {
		t.Fatalf("contributing source = %q, want embedded root document", contributing)
	}

	_, _, err = readDocsDocument("../configuration")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("traversal error = %v, want fs.ErrNotExist", err)
	}
}

func TestDocsURLForTarget(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
		image   bool
		want    string
	}{
		{"parent", "widgets/monitor.md", "../widgets.md", false, "/glance/docs/widgets"},
		{"sibling", "widgets/monitor.md", "html.md", false, "/glance/docs/widgets/html"},
		{"anchor", "widgets/monitor.md", "../widgets.md#shared-properties", false, "/glance/docs/widgets#shared-properties"},
		{"same page anchor", "widgets/monitor.md", "#properties", false, "#properties"},
		{"readme to docs", "README.md", "docs/configuration.md", false, "/glance/docs/configuration"},
		{"readme to docs anchor", "README.md", "docs/fork.md#development-and-ci-validation", false, "/glance/docs/fork#development-and-ci-validation"},
		{"readme image", "README.md", "docs/images/pages/overview.png", true, "/glance/docs/assets/images/pages/overview.png"},
		{"readme contributing", "README.md", "CONTRIBUTING.md", false, "/glance/docs/contributing"},
		{"contributing to docs", "CONTRIBUTING.md", "docs/fork.md", false, "/glance/docs/fork"},
		{"contributing to widget guide", "CONTRIBUTING.md", "docs/adding-a-widget.md", false, "/glance/docs/adding-a-widget"},
		{"docs to contributing", "adding-a-widget.md", "../CONTRIBUTING.md", false, "/glance/docs/contributing"},
		{"nested to readme", "examples/custom-api/monitoring/integrations/argus/README.md", "../../../../../../README.md", false, "/glance/docs"},
		{"nested to configuration", "examples/custom-api/monitoring/integrations/argus/README.md", "../../../../../configuration.md", false, "/glance/docs/configuration"},
		{"nested to parent readme", "examples/custom-api/monitoring/integrations/argus/README.md", "../../README.md", false, "/glance/docs/examples/custom-api/monitoring/README"},
		{"nested markdown asset", "examples/custom-api/monitoring/README.md", "template.yml", false, "/glance/docs/assets/examples/custom-api/monitoring/template.yml"},
		{"nested image", "examples/custom-api/monitoring/integrations/argus/README.md", "../../../../../images/examples/custom-api/monitoring/argus.png", true, "/glance/docs/assets/images/examples/custom-api/monitoring/argus.png"},
		{"docs to readme", "configuration.md", "../README.md", false, "/glance/docs"},
		{"external", "widgets/monitor.md", "https://example.com/docs", false, "https://example.com/docs"},
		{"image", "widgets/markdown.md", "../images/widgets/markdown.png", true, "/glance/docs/assets/images/widgets/markdown.png"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := docsURLForTarget(test.current, test.target, "/glance", test.image)
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderDocsMarkdown(t *testing.T) {
	source := []byte("# Example\n\n## Properties\n\n[Widgets](../widgets.md#shared-properties)\n\n![Example](../images/widgets/markdown.png)\n")

	rendered, err := renderDocsMarkdown(
		newDocumentationMarkdownRenderer(),
		source,
		"widgets/markdown.md",
		"/glance",
	)
	if err != nil {
		t.Fatal(err)
	}

	body := string(rendered)
	for _, expected := range []string{
		`id="example"`,
		`id="properties"`,
		`href="/glance/docs/widgets#shared-properties"`,
		`src="/glance/docs/assets/images/widgets/markdown.png"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("rendered documentation missing %q: %s", expected, body)
		}
	}
}

func TestRenderTrustedReadmeRewritesRawHTMLDestinations(t *testing.T) {
	source := []byte(`<p align="center"><img src="docs/logo.png"></p>
<a href="#installation">Install</a>
<a href="docs/configuration.md#configuring-glance">Configuration</a>
<a href="docs/widgets.md">Widgets</a>`)

	rendered, err := renderDocsMarkdown(
		newTrustedDocumentationMarkdownRenderer(),
		source,
		"README.md",
		"/glance",
	)
	if err != nil {
		t.Fatal(err)
	}

	body := string(rendered)
	for _, expected := range []string{
		`src="/glance/docs/assets/logo.png"`,
		`href="#installation"`,
		`href="/glance/docs#configuration"`,
		`href="/glance/docs/widgets"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("trusted README missing rewritten destination %q: %s", expected, body)
		}
	}

	if strings.Contains(body, `href="docs/configuration.md`) {
		t.Fatalf("trusted README retained raw Markdown destination: %s", body)
	}
}

func TestRenderDocsMarkdownKeepsSafetyDefaults(t *testing.T) {
	source := []byte("<script>alert(1)</script>\n\n[bad](javascript:alert(1))")

	rendered, err := renderDocsMarkdown(
		newDocumentationMarkdownRenderer(),
		source,
		"configuration.md",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	body := strings.ToLower(string(rendered))
	if strings.Contains(body, "<script>") {
		t.Fatalf("raw HTML rendered unexpectedly: %s", body)
	}
	if strings.Contains(body, `href="javascript:`) {
		t.Fatalf("dangerous link rendered unexpectedly: %s", body)
	}
}
