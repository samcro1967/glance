package glance

import (
	"strings"
	"testing"
)

func TestMarkdownRendererGFM(t *testing.T) {
	markdown := newMarkdownRenderer()
	content, err := renderMarkdown(
		markdown,
		[]byte("| A | B |\n|---|---|\n| 1 | 2 |\n\n- [x] Done\n\n~~old~~"),
	)
	if err != nil {
		t.Fatal(err)
	}

	rendered := string(content)
	for _, expected := range []string{"<table>", `type="checkbox"`, "<del>old</del>"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("GFM output missing %q: %s", expected, rendered)
		}
	}
}

func TestMarkdownRendererDoesNotRenderRawHTML(t *testing.T) {
	markdown := newMarkdownRenderer()
	content, err := renderMarkdown(
		markdown,
		[]byte(`<script>alert("x")</script><b>unsafe</b>`),
	)
	if err != nil {
		t.Fatal(err)
	}

	rendered := string(content)
	if strings.Contains(rendered, "<script>") || strings.Contains(rendered, "<b>unsafe</b>") {
		t.Fatalf("raw HTML rendered unsafely: %s", rendered)
	}
}

func TestMarkdownRendererRendersSafeLink(t *testing.T) {
	markdown := newMarkdownRenderer()
	content, err := renderMarkdown(markdown, []byte(`[OpenAI](https://openai.com)`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), `href="https://openai.com"`) {
		t.Fatalf("safe HTTPS link not rendered: %s", content)
	}
}

func TestMarkdownRendererDoesNotRenderDangerousLink(t *testing.T) {
	markdown := newMarkdownRenderer()
	content, err := renderMarkdown(markdown, []byte(`[click](javascript:alert("x"))`))
	if err != nil {
		t.Fatal(err)
	}

	rendered := strings.ToLower(string(content))
	if strings.Contains(rendered, `href="javascript:`) {
		t.Fatalf("dangerous link rendered: %s", rendered)
	}
}

func TestDocumentationMarkdownRendererAddsHeadingIDs(t *testing.T) {
	rendered, err := renderMarkdown(
		newDocumentationMarkdownRenderer(),
		[]byte("# Configuration\n\n## Shared Properties"),
	)
	if err != nil {
		t.Fatal(err)
	}

	body := string(rendered)
	for _, expected := range []string{`id="configuration"`, `id="shared-properties"`} {
		if !strings.Contains(body, expected) {
			t.Errorf("rendered documentation missing %q: %s", expected, body)
		}
	}
}
