package glance

import (
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"time"
)

var extensionWidgetTemplate = mustParseTemplate("extension.html", "widget-base.html")

const extensionWidgetDefaultTitle = "Extension"

type extensionWidget struct {
	widgetBase          `yaml:",inline"`
	URL                 string               `yaml:"url"`
	FallbackContentType string               `yaml:"fallback-content-type"`
	Parameters          queryParametersField `yaml:"parameters"`
	Headers             map[string]string    `yaml:"headers"`
	Timeout             durationField        `yaml:"timeout"`
	AllowInsecure       bool                 `yaml:"allow-insecure"`
	BasicAuth           struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"basic-auth"`
	AllowHtml  bool          `yaml:"allow-potentially-dangerous-html"`
	Extension  extension     `yaml:"-"`
	cachedHTML template.HTML `yaml:"-"`
}

func (widget *extensionWidget) initialize() error {
	widget.withTitle(extensionWidgetDefaultTitle).withCacheDuration(time.Minute * 30)

	if widget.URL == "" {
		return errors.New("URL is required")
	}

	if _, err := url.Parse(widget.URL); err != nil {
		return fmt.Errorf("parsing URL: %v", err)
	}

	return nil
}

func (widget *extensionWidget) update(ctx context.Context) {
	extension, err := fetchExtension(ctx, extensionRequestOptions{
		URL:                 widget.URL,
		FallbackContentType: widget.FallbackContentType,
		Parameters:          widget.Parameters,
		Headers:             widget.Headers,
		Timeout:             widget.Timeout,
		AllowInsecure:       widget.AllowInsecure,
		BasicAuthUsername:   widget.BasicAuth.Username,
		BasicAuthPassword:   widget.BasicAuth.Password,
		AllowHtml:           widget.AllowHtml,
	})

	widget.canContinueUpdateAfterHandlingErr(err)

	widget.Extension = extension

	if widget.Title == extensionWidgetDefaultTitle && extension.Title != "" {
		widget.Title = extension.Title
	}

	if widget.TitleURL == "" && extension.TitleURL != "" {
		widget.TitleURL = extension.TitleURL
	}

	widget.cachedHTML = widget.renderTemplate(widget, extensionWidgetTemplate)
}

func (widget *extensionWidget) Render() template.HTML {
	return widget.cachedHTML
}

type extensionType int

const (
	extensionContentHTML extensionType = iota
	extensionContentUnknown
)

var extensionStringToType = map[string]extensionType{
	"html": extensionContentHTML,
}

const (
	extensionHeaderTitle            = "Widget-Title"
	extensionHeaderTitleURL         = "Widget-Title-URL"
	extensionHeaderContentType      = "Widget-Content-Type"
	extensionHeaderContentFrameless = "Widget-Content-Frameless"
)

type extensionRequestOptions struct {
	URL                 string               `yaml:"url"`
	FallbackContentType string               `yaml:"fallback-content-type"`
	Parameters          queryParametersField `yaml:"parameters"`
	Headers             map[string]string    `yaml:"headers"`
	Timeout             durationField        `yaml:"timeout"`
	AllowInsecure       bool                 `yaml:"allow-insecure"`
	AllowHtml           bool                 `yaml:"allow-potentially-dangerous-html"`
	BasicAuthUsername   string
	BasicAuthPassword   string
}

type extension struct {
	Title     string
	TitleURL  string
	Content   template.HTML
	Frameless bool
}

func convertExtensionContent(options extensionRequestOptions, content []byte, contentType extensionType) template.HTML {
	switch contentType {
	case extensionContentHTML:
		if options.AllowHtml {
			return template.HTML(content)
		}

		fallthrough
	default:
		return template.HTML("<pre>" + html.EscapeString(string(content)) + "</pre>")
	}
}

func fetchExtension(ctx context.Context, options extensionRequestOptions) (extension, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, options.URL, nil)
	if err != nil {
		return extension{}, fmt.Errorf("%w: creating extension request: %w", errNoContent, err)
	}

	if len(options.Parameters) > 0 {
		request.URL.RawQuery = options.Parameters.toQueryString()
	}

	for key, value := range options.Headers {
		request.Header.Add(key, value)
	}

	if options.BasicAuthUsername != "" || options.BasicAuthPassword != "" {
		request.SetBasicAuth(options.BasicAuthUsername, options.BasicAuthPassword)
	}

	baseClient := ternary(options.AllowInsecure, defaultInsecureHTTPClient, defaultHTTPClient)
	client := *baseClient
	if options.Timeout > 0 {
		client.Timeout = time.Duration(options.Timeout)
	}

	response, err := client.Do(request)
	if err != nil {
		return extension{}, fmt.Errorf(
			"%w: extension request failed: %w",
			errNoContent,
			safeHTTPTransportError(err),
		)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return extension{}, fmt.Errorf("%w: could not read body: %w", errNoContent, err)
	}

	extension := extension{}

	if response.Header.Get(extensionHeaderTitle) == "" {
		extension.Title = "Extension"
	} else {
		extension.Title = response.Header.Get(extensionHeaderTitle)
	}

	if response.Header.Get(extensionHeaderTitleURL) != "" {
		extension.TitleURL = response.Header.Get(extensionHeaderTitleURL)
	}

	contentType, ok := extensionStringToType[response.Header.Get(extensionHeaderContentType)]

	if !ok {
		contentType, ok = extensionStringToType[options.FallbackContentType]

		if !ok {
			contentType = extensionContentUnknown
		}
	}

	if stringToBool(response.Header.Get(extensionHeaderContentFrameless)) {
		extension.Frameless = true
	}

	extension.Content = convertExtensionContent(options, body, contentType)

	return extension, nil
}

func (widget *extensionWidget) setDefaultHeaders(value map[string]string) {
	widget.Headers = mergeStringMaps(value, widget.Headers)
}

func (widget *extensionWidget) setDefaultTimeout(value durationField) {
	widget.Timeout = value
}

func (widget *extensionWidget) setDefaultAllowInsecure(value bool) {
	widget.AllowInsecure = value
}

func (widget *extensionWidget) setDefaultBasicAuth(value basicAuthDefaults) {
	if widget.configuredFields["basic-auth"] {
		return
	}

	widget.BasicAuth.Username = value.Username
	widget.BasicAuth.Password = value.Password
}
