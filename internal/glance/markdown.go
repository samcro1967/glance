package glance

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/yuin/goldmark"
	goldmarkextension "github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

// newMarkdownRenderer returns the shared Markdown renderer used by Glance.
//
// GitHub-Flavored Markdown is enabled so all consumers use the same Markdown
// dialect. Goldmark's default safe rendering behavior is intentionally kept:
// raw HTML is not rendered and dangerous link destinations are not emitted.
func newMarkdownRenderer() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(goldmarkextension.GFM),
	)
}

// newDocumentationMarkdownRenderer keeps the shared Markdown dialect and
// safety defaults while adding stable heading IDs for documentation anchors.
func newDocumentationMarkdownRenderer() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(goldmarkextension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
}

// newTrustedDocumentationMarkdownRenderer renders trusted documentation that

// is compiled into the Glance binary. Raw HTML is allowed here because this

// renderer is used only for the compile-time embedded repository README.

func newTrustedDocumentationMarkdownRenderer() goldmark.Markdown {

	return goldmark.New(

		goldmark.WithExtensions(goldmarkextension.GFM),

		goldmark.WithParserOptions(parser.WithAutoHeadingID()),

		goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
	)

}

// renderMarkdown converts Markdown source into HTML using the supplied
// renderer.
//
// The returned value is template.HTML because Goldmark, rather than Go's
// template package, owns Markdown-to-HTML escaping and safety decisions.
// Conversion failures are returned to the caller so each consumer can apply
// its own lifecycle and error-handling policy.
func renderMarkdown(markdown goldmark.Markdown, source []byte) (template.HTML, error) {
	var output bytes.Buffer

	if err := markdown.Convert(source, &output); err != nil {
		return "", fmt.Errorf("rendering markdown: %w", err)
	}

	return template.HTML(output.String()), nil
}
