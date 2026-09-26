package glance

import (
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"strings"
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

	titleConfigured    bool `yaml:"-"`
	titleURLConfigured bool `yaml:"-"`
}

func (widget *extensionWidget) initialize() error {
	widget.titleConfigured = widget.Title != ""
	widget.titleURLConfigured = widget.TitleURL != ""
	widget.withTitle(extensionWidgetDefaultTitle).withCacheDuration(time.Minute * 30)

	if widget.URL == "" {
		return errors.New("URL is required")
	}

	if _, err := validateExtensionEndpointURL(widget.URL); err != nil {
		return err
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

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Extension = extension

	if !widget.titleConfigured {
		widget.Title = extensionWidgetDefaultTitle
		if extension.Title != "" {
			widget.Title = extension.Title
		}
	}

	if !widget.titleURLConfigured {
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
	extensionContentPresentationV1
	extensionContentUnknown
)

var extensionStringToType = map[string]extensionType{
	"html":            extensionContentHTML,
	"presentation-v1": extensionContentPresentationV1,
}

const (
	extensionHeaderTitle            = "Widget-Title"
	extensionHeaderTitleURL         = "Widget-Title-URL"
	extensionHeaderContentType      = "Widget-Content-Type"
	extensionHeaderContentFrameless = "Widget-Content-Frameless"
)

var errExtensionCrossOriginRedirectWithCredentials = errors.New("extension cross-origin redirect with credentials is not allowed")

func validateExtensionEndpointURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("URL must include a host")
	}
	if parsed.User != nil {
		return nil, errors.New("URL must not include userinfo; use basic-auth instead")
	}
	if parsed.Fragment != "" {
		return nil, errors.New("URL must not include a fragment")
	}

	return parsed, nil
}

func validateExtensionTitleURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing Widget-Title-URL: %w", err)
	}

	if parsed.User != nil {
		return "", errors.New("Widget-Title-URL must not include userinfo")
	}
	if parsed.IsAbs() {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", errors.New("Widget-Title-URL must use http or https")
		}
		if parsed.Host == "" {
			return "", errors.New("Widget-Title-URL must include a host")
		}
		return rawURL, nil
	}
	if parsed.Host != "" {
		return "", errors.New("Widget-Title-URL must not be protocol-relative")
	}

	return rawURL, nil
}

func extensionURLPort(parsed *url.URL) string {
	if port := parsed.Port(); port != "" {
		return port
	}
	if parsed.Scheme == "http" {
		return "80"
	}
	if parsed.Scheme == "https" {
		return "443"
	}
	return ""
}

func extensionURLsShareOrigin(first, second *url.URL) bool {
	return strings.EqualFold(first.Scheme, second.Scheme) &&
		strings.EqualFold(first.Hostname(), second.Hostname()) &&
		extensionURLPort(first) == extensionURLPort(second)
}

func extensionRequestHasCredentials(options extensionRequestOptions) bool {
	return options.BasicAuthUsername != "" ||
		options.BasicAuthPassword != "" ||
		len(options.Headers) > 0
}

func configureExtensionRedirectPolicy(client *http.Client, original *url.URL, options extensionRequestOptions) {
	if !extensionRequestHasCredentials(options) {
		return
	}

	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("extension stopped after 10 redirects")
		}
		if !extensionURLsShareOrigin(original, request.URL) {
			return errExtensionCrossOriginRedirectWithCredentials
		}
		return nil
	}
}

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
	converted, _ := convertExtensionContentValidated(options, content, contentType)
	return converted
}

func convertExtensionContentValidated(options extensionRequestOptions, content []byte, contentType extensionType) (template.HTML, error) {
	switch contentType {
	case extensionContentHTML:
		if options.AllowHtml {
			return template.HTML(content), nil
		}
	case extensionContentPresentationV1:
		presentation, err := renderExtensionPresentation(content)
		if err != nil {
			return "", fmt.Errorf("invalid presentation-v1 content: %w", err)
		}
		return presentation, nil
	}

	return template.HTML("<pre>" + html.EscapeString(string(content)) + "</pre>"), nil
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

	client := newHTTPClient(options.Timeout, options.AllowInsecure)
	configureExtensionRedirectPolicy(client, request.URL, options)

	response, err := client.Do(request)
	if err != nil {
		return extension{}, fmt.Errorf(
			"%w: extension request failed: %w",
			errNoContent,
			safeHTTPTransportError(err),
		)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := readDefaultHTTPResponseBody(response.Body)
	if err != nil {
		return extension{}, fmt.Errorf("%w: could not read body: %w", errNoContent, err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return extension{}, fmt.Errorf(
			"%w: extension request failed: %w",
			errNoContent,
			unexpectedHTTPStatusError(response),
		)
	}

	result := extension{}

	if response.Header.Get(extensionHeaderTitle) == "" {
		result.Title = "Extension"
	} else {
		result.Title = response.Header.Get(extensionHeaderTitle)
	}

	if response.Header.Get(extensionHeaderTitleURL) != "" {
		titleURL, err := validateExtensionTitleURL(response.Header.Get(extensionHeaderTitleURL))
		if err != nil {
			return extension{}, fmt.Errorf("%w: invalid %s header: %w", errNoContent, extensionHeaderTitleURL, err)
		}
		result.TitleURL = titleURL
	}

	contentType, ok := extensionStringToType[response.Header.Get(extensionHeaderContentType)]

	if !ok {
		contentType, ok = extensionStringToType[options.FallbackContentType]

		if !ok {
			contentType = extensionContentUnknown
		}
	}

	if stringToBool(response.Header.Get(extensionHeaderContentFrameless)) {
		result.Frameless = true
	}

	content, err := convertExtensionContentValidated(options, body, contentType)
	if err != nil {
		return extension{}, fmt.Errorf("%w: %w", errNoContent, err)
	}
	result.Content = content

	return result, nil
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
