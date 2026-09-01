package glance

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	goldmarkextension "github.com/yuin/goldmark/extension"
)

var markdownWidgetTemplate = mustParseTemplate("markdown.html", "widget-base.html")

type markdownWidget struct {
	widgetBase `yaml:",inline"`
	Source     string        `yaml:"source"`
	File       string        `yaml:"file"`
	Content    template.HTML `yaml:"-"`
	markdown   goldmark.Markdown
}

func (widget *markdownWidget) initialize() error {
	widget.withTitle("")

	hasSource := strings.TrimSpace(widget.Source) != ""
	hasFile := strings.TrimSpace(widget.File) != ""

	if hasSource == hasFile {
		return fmt.Errorf("markdown widget must specify exactly one of source or file")
	}

	widget.markdown = goldmark.New(
		goldmark.WithExtensions(goldmarkextension.GFM),
	)

	if hasSource {
		content, err := widget.renderMarkdown([]byte(widget.Source))
		if err != nil {
			return err
		}

		widget.Content = content
		widget.ContentAvailable = true
		return nil
	}

	widget.withCacheDuration(5 * time.Minute)
	return nil
}

func (widget *markdownWidget) update(ctx context.Context) {
	select {
	case <-ctx.Done():
		widget.canContinueUpdateAfterHandlingErr(ctx.Err())
		return
	default:
	}

	body, err := os.ReadFile(widget.File)
	if err != nil {
		widget.canContinueUpdateAfterHandlingErr(fmt.Errorf("reading markdown file: %w", err))
		return
	}

	content, err := widget.renderMarkdown(body)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Content = content
}

func (widget *markdownWidget) renderMarkdown(source []byte) (template.HTML, error) {
	var output bytes.Buffer

	if err := widget.markdown.Convert(source, &output); err != nil {
		return "", fmt.Errorf("rendering markdown: %w", err)
	}

	return template.HTML(output.String()), nil
}

func (widget *markdownWidget) Render() template.HTML {
	return widget.renderTemplate(widget, markdownWidgetTemplate)
}
