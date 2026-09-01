package glance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMarkdownWidgetRequiresExactlyOneSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		file   string
		valid  bool
	}{
		{name: "neither", valid: false},
		{name: "source", source: "# Hello", valid: true},
		{name: "file", file: "/tmp/notes.md", valid: true},
		{name: "both", source: "# Hello", file: "/tmp/notes.md", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			widget := &markdownWidget{Source: test.source, File: test.file}
			err := widget.initialize()
			if test.valid && err != nil {
				t.Fatalf("initialize: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected initialization error")
			}
		})
	}
}

func TestMarkdownWidgetInlineRendering(t *testing.T) {
	widget := &markdownWidget{Source: "## Hello\n\nThis is **Markdown**."}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	rendered := string(widget.Render())
	for _, expected := range []string{"<h2>Hello</h2>", "<strong>Markdown</strong>"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered markdown missing %q: %s", expected, rendered)
		}
	}

	if widget.cacheType != cacheTypeInfinite {
		t.Fatalf("inline cache type = %v, want infinite", widget.cacheType)
	}
}

func TestMarkdownWidgetGFM(t *testing.T) {
	widget := &markdownWidget{Source: "| A | B |\n|---|---|\n| 1 | 2 |\n\n- [x] Done\n\n~~old~~"}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	rendered := string(widget.Render())
	for _, expected := range []string{"<table>", `type="checkbox"`, "<del>old</del>"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("GFM output missing %q: %s", expected, rendered)
		}
	}
}

func TestMarkdownWidgetDoesNotRenderRawHTML(t *testing.T) {
	widget := &markdownWidget{Source: `<script>alert("x")</script><b>unsafe</b>`}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	rendered := string(widget.Render())
	if strings.Contains(rendered, "<script>") || strings.Contains(rendered, "<b>unsafe</b>") {
		t.Fatalf("raw HTML rendered unsafely: %s", rendered)
	}
}

func TestMarkdownWidgetRendersSafeLink(t *testing.T) {
	widget := &markdownWidget{Source: `[OpenAI](https://openai.com)`}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	rendered := string(widget.Render())
	if !strings.Contains(rendered, `href="https://openai.com"`) {
		t.Fatalf("safe HTTPS link not rendered: %s", rendered)
	}
}

func TestMarkdownWidgetDoesNotRenderDangerousLink(t *testing.T) {
	widget := &markdownWidget{Source: `[click](javascript:alert("x"))`}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	rendered := strings.ToLower(string(widget.Render()))
	if strings.Contains(rendered, "href=\"javascript:") {
		t.Fatalf("dangerous link rendered: %s", rendered)
	}
}

func TestMarkdownWidgetFileUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte("# First"), 0o600); err != nil {
		t.Fatal(err)
	}

	widget := &markdownWidget{File: path}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	if widget.cacheType != cacheTypeDuration || widget.cacheDuration != 5*time.Minute {
		t.Fatalf("file cache = %v/%v, want duration/5m", widget.cacheType, widget.cacheDuration)
	}

	widget.update(context.Background())
	if !widget.ContentAvailable {
		t.Fatal("content not marked available after successful file refresh")
	}
	if !strings.Contains(string(widget.Render()), "<h1>First</h1>") {
		t.Fatalf("initial file render failed: %s", widget.Render())
	}

	if err := os.WriteFile(path, []byte("# Second"), 0o600); err != nil {
		t.Fatal(err)
	}
	widget.update(context.Background())
	if !strings.Contains(string(widget.Render()), "<h1>Second</h1>") {
		t.Fatalf("updated file render failed: %s", widget.Render())
	}
}

func TestMarkdownWidgetFileFailurePreservesContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte("# Good"), 0o600); err != nil {
		t.Fatal(err)
	}

	widget := &markdownWidget{File: path}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}
	widget.update(context.Background())
	before := widget.Content

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	widget.update(context.Background())

	if widget.Content != before {
		t.Fatalf("content changed after failed refresh: before=%q after=%q", before, widget.Content)
	}
	if widget.Error == nil {
		t.Fatal("expected refresh error")
	}

	rendered := string(widget.Render())
	if !strings.Contains(rendered, "<h1>Good</h1>") {
		t.Fatalf("last successful content missing after failed refresh: %s", rendered)
	}
	if !strings.Contains(rendered, "notice-icon-major") {
		t.Fatalf("refresh error not surfaced by widget base: %s", rendered)
	}
}

func TestMarkdownWidgetRenderThroughWidgetBase(t *testing.T) {
	widget := &markdownWidget{
		Source: "## Heading\n\n**bold**\n\n<script>alert(1)</script>",
	}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	rendered := string(widget.Render())

	for _, expected := range []string{
		`class="markdown-content"`,
		"<h2>Heading</h2>",
		"<strong>bold</strong>",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget missing %q: %s", expected, rendered)
		}
	}

	if strings.Contains(rendered, "<script>") {
		t.Fatalf("raw HTML survived widget rendering: %s", rendered)
	}
}

func TestMarkdownWidgetFileCustomCache(t *testing.T) {
	widget := &markdownWidget{File: "/tmp/notes.md"}
	widget.CustomCacheDuration = durationField(30 * time.Second)

	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	if widget.cacheType != cacheTypeDuration {
		t.Fatalf("cache type = %v, want duration", widget.cacheType)
	}
	if widget.cacheDuration != 30*time.Second {
		t.Fatalf("cache duration = %v, want 30s", widget.cacheDuration)
	}
}
