package glance

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	"github.com/yuin/goldmark"
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

	widget.markdown = newMarkdownRenderer()

	if hasSource {
		content, err := renderMarkdown(widget.markdown, []byte(widget.Source))
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

	content, err := renderMarkdown(widget.markdown, body)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Content = content
}

func (widget *markdownWidget) Render() template.HTML {
	return widget.renderTemplate(widget, markdownWidgetTemplate)
}
