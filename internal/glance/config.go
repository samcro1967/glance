package glance

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"iter"
	"log/slog"
	"maps"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

const CONFIG_INCLUDE_RECURSION_DEPTH_LIMIT = 20

const (
	configVarTypeEnv         = "env"
	configVarTypeSecret      = "secret"
	configVarTypeFileFromEnv = "readFileFromEnv"
)

type config struct {
	Server struct {
		Host                string   `yaml:"host"`
		Port                uint16   `yaml:"port"`
		HTTPS               bool     `yaml:"https"`
		Proxied             bool     `yaml:"proxied"`
		TrustedProxies      []string `yaml:"trusted-proxies"`
		AssetsPath          string   `yaml:"assets-path"`
		BaseURL             string   `yaml:"base-url"`
		FrontendDiagnostics bool     `yaml:"frontend-diagnostics"`
		PersonalState       struct {
			Enabled bool   `yaml:"enabled"`
			Path    string `yaml:"path"`
		} `yaml:"personal-state"`
		ResourceProxy struct {
			AllowedOrigins []string `yaml:"allowed-origins"`
		} `yaml:"resource-proxy"`
	} `yaml:"server"`

	Auth struct {
		SecretKey string                     `yaml:"secret-key"`
		Users     map[string]*user           `yaml:"users"`
		OIDC      oidcConfig                 `yaml:"oidc"`
		Groups    map[string]authGroupConfig `yaml:"groups"`
		Access    authAccessConfig           `yaml:"access"`
	} `yaml:"auth"`

	Analytics analyticsConfig `yaml:"analytics"`

	Document struct {
		Head template.HTML `yaml:"head"`
	} `yaml:"document"`

	Theme struct {
		themeProperties `yaml:",inline"`

		DisablePicker bool                                     `yaml:"disable-picker"`
		Presets       orderedYAMLMap[string, *themeProperties] `yaml:"presets"`
	} `yaml:"theme"`

	Branding struct {
		HideFooter         bool          `yaml:"hide-footer"`
		CustomFooter       template.HTML `yaml:"custom-footer"`
		LogoText           string        `yaml:"logo-text"`
		LogoURL            string        `yaml:"logo-url"`
		FaviconURL         string        `yaml:"favicon-url"`
		FaviconType        string        `yaml:"-"`
		AppName            string        `yaml:"app-name"`
		AppIconURL         string        `yaml:"app-icon-url"`
		AppBackgroundColor string        `yaml:"app-background-color"`
	} `yaml:"branding"`

	FooterMicroWidgets footerMicroWidgets               `yaml:"footer-micro-widgets"`
	WidgetDefaults     widgetDefaultsConfig             `yaml:"widget-defaults"`
	Pages              []page                           `yaml:"pages"`
	Dashboards         orderedYAMLMap[string, []string] `yaml:"dashboards"`

	// recoverableErrors contains widget-local and micro-widget-local configuration
	// failures that do not prevent the rest of the application from being constructed. The
	// serve path logs them, while config:validate still treats them as invalid
	// configuration so administrators do not lose validation coverage.
	recoverableErrors []error `yaml:"-"`
}

type user struct {
	Password           string `yaml:"password"`
	PasswordHashString string `yaml:"password-hash"`
	PasswordHash       []byte `yaml:"-"`
}

type authGroupConfig struct {
	Users []string `yaml:"users"`
}

type authDashboardAccessConfig struct {
	Users  []string `yaml:"users"`
	Groups []string `yaml:"groups"`
}

type authAccessConfig struct {
	Dashboards map[string]authDashboardAccessConfig `yaml:"dashboards"`
}

type oidcConfig struct {
	Issuer       string   `yaml:"issuer"`
	ClientID     string   `yaml:"client-id"`
	ClientSecret string   `yaml:"client-secret"`
	RedirectURL  string   `yaml:"redirect-url"`
	ProviderName string   `yaml:"provider-name"`
	AllowedUsers []string `yaml:"allowed-users"`
}

func (c oidcConfig) configured() bool {
	return c.Issuer != "" ||
		c.ClientID != "" ||
		c.ClientSecret != "" ||
		c.RedirectURL != "" ||
		len(c.AllowedUsers) > 0
}

func (c oidcConfig) providerDisplayName() string {
	if c.ProviderName != "" {
		return c.ProviderName
	}
	return "SSO"
}

type page struct {
	Title                  string          `yaml:"name"`
	Icon                   customIconField `yaml:"icon"`
	Slug                   string          `yaml:"slug"`
	Width                  string          `yaml:"width"`
	DesktopNavigationWidth string          `yaml:"desktop-navigation-width"`
	ShowMobileHeader       bool            `yaml:"show-mobile-header"`
	HideDesktopNavigation  bool            `yaml:"hide-desktop-navigation"`
	CenterVertically       bool            `yaml:"center-vertically"`
	Theme                  themeProperties `yaml:"theme"`
	HeadWidgets            widgets         `yaml:"head-widgets"`
	BottomWidgets          widgets         `yaml:"bottom-widgets"`
	Columns                []struct {
		Size    string  `yaml:"size"`
		Widgets widgets `yaml:"widgets"`
	} `yaml:"columns"`
	PrimaryColumnIndex int8       `yaml:"-"`
	mu                 sync.Mutex `yaml:"-"`
}

type configSourceLocation struct {
	File string
	Line int
}

type parsedYAMLConfig struct {
	Contents []byte
	Includes map[string]struct{}
	Sources  []configSourceLocation
}

type configDiagnostic struct {
	File    string
	Line    int
	Message string
	cause   error
}

// configWatcherReloadError marks a watcher callback failure that occurred
// while constructing a candidate configuration after a filesystem change.
// The currently active application remains valid, but runtime diagnostics
// should record the failed reload attempt rather than treating it as an
// unrelated watcher transport error.
type configWatcherReloadError struct {
	cause error
}

func (e *configWatcherReloadError) Error() string {
	if e == nil || e.cause == nil {
		return "configuration reload failed"
	}
	return fmt.Sprintf("parsing changed configuration: %v", e.cause)
}

func (e *configWatcherReloadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

type configSemanticSources struct {
	analytics            int
	analyticsProvider    int
	analyticsEndpoint    int
	root                 int
	server               int
	personalState        int
	personalStatePath    int
	assetsPath           int
	auth                 int
	authSecret           int
	authUsers            int
	users                map[string]int
	authGroups           int
	authGroup            map[string]int
	authAccess           int
	authAccessDashboards int
	authAccessDashboard  map[string]int
	authOIDC             int
	authOIDCIssuer       int
	authOIDCClientID     int
	authOIDCSecret       int
	authOIDCRedirect     int
	dashboards           int
	dashboard            map[string]int
	widgetDefaults       int
	widgetDefaultsGlobal int
	widgetDefaultType    map[string]int
	theme                int
	themePresets         int
	themePreset          map[string]int
	footerMicroLeft      []int
	footerMicroRight     []int
	pages                int
	page                 []configPageSemanticSources
}

type configPageSemanticSources struct {
	line                   int
	name                   int
	slug                   int
	width                  int
	desktopNavigationWidth int
	theme                  int
	headWidgets            []configWidgetSemanticSources
	bottomWidgets          []configWidgetSemanticSources
	columns                int
	column                 []configColumnSemanticSources
}

type configColumnSemanticSources struct {
	line    int
	size    int
	widgets []configWidgetSemanticSources
}

type configWidgetSemanticSources struct {
	line     int
	template int
	widgets  []configWidgetSemanticSources
}

type widgetInitError struct {
	message string
	widget  widget
	cause   error
}

func (e *widgetInitError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *widgetInitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (d *configDiagnostic) Error() string {
	if d == nil {
		return ""
	}

	if d.File != "" && d.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", d.File, d.Line, d.Message)
	}
	if d.File != "" {
		return fmt.Sprintf("%s: %s", d.File, d.Message)
	}
	return d.Message
}

func (d *configDiagnostic) Unwrap() error {
	if d == nil {
		return nil
	}
	return d.cause
}

func (parsed *parsedYAMLConfig) sourceLocation(generatedLine int) (configSourceLocation, bool) {
	if parsed == nil || generatedLine < 1 || generatedLine > len(parsed.Sources) {
		return configSourceLocation{}, false
	}

	return parsed.Sources[generatedLine-1], true
}

func parseYAMLLinePrefix(message string) (int, string, bool) {
	for _, prefix := range []string{"yaml: line ", "line "} {
		if !strings.HasPrefix(message, prefix) {
			continue
		}

		remainder := strings.TrimPrefix(message, prefix)
		colon := strings.IndexByte(remainder, ':')
		if colon <= 0 {
			return 0, message, false
		}

		line, err := strconv.Atoi(remainder[:colon])
		if err != nil || line < 1 {
			return 0, message, false
		}

		detail := strings.TrimSpace(remainder[colon+1:])
		if detail == "" {
			return 0, message, false
		}

		return line, detail, true
	}

	return 0, message, false
}

func configDiagnosticFromYAMLError(parsed *parsedYAMLConfig, err error) error {
	if err == nil {
		return nil
	}

	if typeErr, ok := err.(*yaml.TypeError); ok && len(typeErr.Errors) == 1 {
		if generatedLine, message, ok := parseYAMLLinePrefix(typeErr.Errors[0]); ok {
			if source, found := parsed.sourceLocation(generatedLine); found {
				return &configDiagnostic{
					File:    source.File,
					Line:    source.Line,
					Message: message,
					cause:   err,
				}
			}
		}
	}

	if generatedLine, message, ok := parseYAMLLinePrefix(err.Error()); ok {
		if source, found := parsed.sourceLocation(generatedLine); found {
			return &configDiagnostic{
				File:    source.File,
				Line:    source.Line,
				Message: message,
				cause:   err,
			}
		}
	}

	return err
}

func newConfigFromYAML(contents []byte) (*config, error) {
	config, err := newConfigFromParsedYAML(&parsedYAMLConfig{Contents: contents})
	if err != nil {
		return nil, err
	}

	// Keep this test/helper entrypoint strict. Runtime loading uses
	// newConfigFromParsedYAML directly so it can preserve valid widgets while
	// surfacing widget-local failures as recoverable configuration issues.
	if len(config.recoverableErrors) > 0 {
		return nil, config.recoverableErrors[0]
	}

	return config, nil
}

func normalizeAndValidateCompiledConfigWithSources(
	config *config,
	parsed *parsedYAMLConfig,
	sources *configSemanticSources,
) error {
	diagnostic := func(line int, err error) error {
		return semanticConfigDiagnostic(parsed, line, err)
	}

	rootLine := 0
	if sources != nil {
		rootLine = sources.root
	}
	if len(config.Auth.Users) > 0 || config.Auth.OIDC.configured() {
		secretLine := rootLine
		if sources != nil {
			secretLine = semanticSourceLine(sources.authSecret, sources.auth, rootLine)
		}

		secretBytes, err := base64.StdEncoding.DecodeString(config.Auth.SecretKey)
		if err != nil {
			return diagnostic(secretLine, fmt.Errorf("decoding secret-key: %w", err))
		}

		if len(secretBytes) != AUTH_SECRET_KEY_LENGTH {
			return diagnostic(secretLine, fmt.Errorf("secret-key must be exactly %d bytes", AUTH_SECRET_KEY_LENGTH))
		}
	}

	pageSlugs := make(map[string]struct{}, len(config.Pages))

	for p := range config.Pages {
		page := &config.Pages[p]

		if page.Slug == "" {
			page.Slug = titleToSlug(page.Title)
		}

		if slices.Contains(reservedPageSlugs, page.Slug) {
			line := rootLine
			if sources != nil && p < len(sources.page) {
				line = semanticSourceLine(sources.page[p].slug, sources.page[p].line, sources.pages, rootLine)
			}
			return diagnostic(line, fmt.Errorf("page slug %q is reserved", page.Slug))
		}

		pageSlugs[page.Slug] = struct{}{}

		if page.Width == "default" {
			page.Width = ""
		}

		if page.DesktopNavigationWidth == "" || page.DesktopNavigationWidth == "default" {
			page.DesktopNavigationWidth = page.Width
		}
	}

	if len(config.Dashboards.keys) == 0 {
		return nil
	}

	acceptedDashboardSlugs := make(map[string]struct{})

	for dashboardName, referencedPageSlugs := range config.Dashboards.Items() {
		dashboardSlug := titleToSlug(dashboardName)
		dashboardLine := rootLine
		if sources != nil {
			dashboardLine = semanticSourceLine(sources.dashboard[dashboardName], sources.dashboards, rootLine)
		}

		if dashboardSlug == "" {
			return diagnostic(dashboardLine, fmt.Errorf("dashboard %q has an invalid slug", dashboardName))
		}

		if dashboardName != "Default" && slices.Contains(reservedDashboardSlugs, dashboardSlug) {
			return diagnostic(dashboardLine, fmt.Errorf("dashboard slug %q is reserved", dashboardSlug))
		}

		if dashboardName != "Default" {
			if _, exists := acceptedDashboardSlugs[dashboardSlug]; exists {
				return diagnostic(dashboardLine, fmt.Errorf("dashboard slug %q is duplicated", dashboardSlug))
			}

			// Preserve the existing runtime behavior: a dashboard whose slug
			// conflicts with a page is ignored rather than rejected, and
			// therefore does not participate in duplicate-dashboard checks.
			if _, exists := pageSlugs[dashboardSlug]; exists {
				continue
			}

			acceptedDashboardSlugs[dashboardSlug] = struct{}{}
		}

		for _, pageSlug := range referencedPageSlugs {
			if _, exists := pageSlugs[pageSlug]; !exists {
				return diagnostic(
					dashboardLine,
					fmt.Errorf(
						"dashboard %q references unknown page slug %q",
						dashboardName,
						pageSlug,
					),
				)
			}
		}
	}

	return nil
}

func newConfigFromParsedYAML(parsed *parsedYAMLConfig) (*config, error) {
	contents, err := parseConfigVariablesWithSources(parsed.Contents, parsed)
	if err != nil {
		return nil, err
	}

	config := &config{}
	config.Server.Port = 8080

	err = yaml.Unmarshal(contents, config)
	if err != nil {
		return nil, configDiagnosticFromYAMLError(parsed, err)
	}

	if err = captureThemeConfiguredFields(config, contents); err != nil {
		return nil, configDiagnosticFromYAMLError(parsed, err)
	}

	semanticSources, err := parseConfigSemanticSources(contents)
	if err != nil {
		return nil, configDiagnosticFromYAMLError(parsed, err)
	}

	recoverConfiguredThemeErrors(config, parsed, semanticSources)
	recoverWidgetDefaultErrors(config, parsed, semanticSources)
	recoverAnalyticsError(config, parsed, semanticSources)
	recoverAssetsPathError(config, parsed, semanticSources)

	if err = isConfigStateValidWithSources(config, parsed, semanticSources); err != nil {
		return nil, err
	}

	defaultsLogSummary := widgetDefaultsLogSummary{}

	config.FooterMicroWidgets.Left = initializeConfiguredMicroWidgets(
		config.FooterMicroWidgets.Left,
		semanticSources.footerMicroLeft,
		config.WidgetDefaults,
	)
	config.FooterMicroWidgets.Right = initializeConfiguredMicroWidgets(
		config.FooterMicroWidgets.Right,
		semanticSources.footerMicroRight,
		config.WidgetDefaults,
	)
	resolveInvalidMicroWidgetDiagnostics(
		parsed,
		config.FooterMicroWidgets.Left,
		&config.recoverableErrors,
	)
	resolveInvalidMicroWidgetDiagnostics(
		parsed,
		config.FooterMicroWidgets.Right,
		&config.recoverableErrors,
	)

	for p := range config.Pages {
		for w := range config.Pages[p].HeadWidgets {
			candidate := config.Pages[p].HeadWidgets[w]
			defaultsSummary, initialized := initializeConfiguredWidget(candidate, config.WidgetDefaults)
			config.Pages[p].HeadWidgets[w] = initialized
			defaultsLogSummary.add(defaultsSummary)
		}

		for w := range config.Pages[p].BottomWidgets {
			candidate := config.Pages[p].BottomWidgets[w]
			defaultsSummary, initialized := initializeConfiguredWidget(candidate, config.WidgetDefaults)
			config.Pages[p].BottomWidgets[w] = initialized
			defaultsLogSummary.add(defaultsSummary)
		}

		for c := range config.Pages[p].Columns {
			for w := range config.Pages[p].Columns[c].Widgets {
				candidate := config.Pages[p].Columns[c].Widgets[w]
				defaultsSummary, initialized := initializeConfiguredWidget(candidate, config.WidgetDefaults)
				config.Pages[p].Columns[c].Widgets[w] = initialized
				defaultsLogSummary.add(defaultsSummary)
			}
		}
	}

	for p := range config.Pages {
		var pageSource configPageSemanticSources
		if p < len(semanticSources.page) {
			pageSource = semanticSources.page[p]
		}

		resolveInvalidWidgetDiagnostics(
			parsed,
			config.Pages[p].HeadWidgets,
			pageSource.headWidgets,
			&config.recoverableErrors,
		)
		resolveInvalidWidgetDiagnostics(
			parsed,
			config.Pages[p].BottomWidgets,
			pageSource.bottomWidgets,
			&config.recoverableErrors,
		)
		for c := range config.Pages[p].Columns {
			var columnSource configColumnSemanticSources
			if c < len(pageSource.column) {
				columnSource = pageSource.column[c]
			}
			resolveInvalidWidgetDiagnostics(
				parsed,
				config.Pages[p].Columns[c].Widgets,
				columnSource.widgets,
				&config.recoverableErrors,
			)
		}
	}

	logWidgetDefaultsConfigured(config.WidgetDefaults, defaultsLogSummary)

	if err := normalizeAndValidateCompiledConfigWithSources(config, parsed, semanticSources); err != nil {
		return nil, err
	}

	return config, nil
}

var envVariableNamePattern = regexp.MustCompile(`^[A-Z0-9_]+$`)
var configVariablePattern = regexp.MustCompile(`(^|.)\$\{(?:([a-zA-Z]+):)?([a-zA-Z0-9_-]+)\}`)

// Parses variables defined in the config such as:
// ${API_KEY}                                      - gets replaced with the value of the API_KEY environment variable
// \${API_KEY}                                                 - escaped, gets used as is without the \ in the config
// ${secret:api_key}                           - value gets loaded from /run/secrets/api_key
// ${readFileFromEnv:PATH_TO_SECRET}    - value gets loaded from the file path specified in the environment variable PATH_TO_SECRET
//
// findYAMLCommentStart returns the index of the YAML comment start (# preceded
// by whitespace or at the start of a line) while respecting quoted strings.
// Returns -1 if there is no comment on the line.
func findYAMLCommentStart(line []byte) int {
	inSingle := false
	inDouble := false

	for i := 0; i < len(line); i++ {
		switch {
		case inSingle:
			if line[i] == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
				} else {
					inSingle = false
				}
			}
		case inDouble:
			if line[i] == '\\' {
				if i+1 < len(line) {
					i++
				}
			} else if line[i] == '"' {
				inDouble = false
			}
		default:
			switch line[i] {
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case '#':
				if i == 0 || line[i-1] == ' ' || line[i-1] == '\t' {
					return i
				}
			}
		}
	}

	return -1
}

func parseConfigVariables(contents []byte) ([]byte, error) {
	return parseConfigVariablesWithSources(contents, nil)
}

func parseConfigVariablesWithSources(contents []byte, parsed *parsedYAMLConfig) ([]byte, error) {
	var err error
	generatedLine := 0
	errorLine := 0

	replaceFunc := func(match []byte) []byte {
		if err != nil {
			return nil
		}

		groups := configVariablePattern.FindSubmatch(match)
		if len(groups) != 4 {
			// we can't handle this match, this shouldn't happen unless the number of groups
			// in the regex has been changed without updating the below code
			return match
		}

		prefix := string(groups[1])
		if prefix == `\` {
			if len(match) >= 2 {
				return match[1:]
			} else {
				return nil
			}
		}

		typeAsString, variableName := string(groups[2]), string(groups[3])
		variableType := ternary(typeAsString == "", configVarTypeEnv, typeAsString)

		parsedValue, returnOriginal, localErr := parseConfigVariableOfType(variableType, variableName)
		if localErr != nil {
			err = fmt.Errorf("parsing variable: %w", localErr)
			errorLine = generatedLine
			return nil
		}

		if returnOriginal {
			return match
		}

		return []byte(prefix + parsedValue)
	}

	// Process line by line so we can skip YAML comments, which should not
	// have their variables expanded (fixes #948).
	lines := bytes.Split(contents, []byte("\n"))
	for i, line := range lines {
		generatedLine = i + 1
		commentIdx := findYAMLCommentStart(line)
		if commentIdx >= 0 {
			// Only apply variable substitution to the part before the comment
			lines[i] = append(
				configVariablePattern.ReplaceAllFunc(line[:commentIdx], replaceFunc),
				line[commentIdx:]...,
			)
		} else {
			lines[i] = configVariablePattern.ReplaceAllFunc(line, replaceFunc)
		}
	}

	if err != nil {
		return nil, semanticConfigDiagnostic(parsed, errorLine, err)
	}

	return bytes.Join(lines, []byte("\n")), nil
}

// When the bool return value is true, it indicates that the caller should use the original value
func parseConfigVariableOfType(variableType, variableName string) (string, bool, error) {
	switch variableType {
	case configVarTypeEnv:
		if !envVariableNamePattern.MatchString(variableName) {
			return "", true, nil
		}

		v, found := os.LookupEnv(variableName)
		if !found {
			return "", false, fmt.Errorf("environment variable %s not found", variableName)
		}

		return v, false, nil
	case configVarTypeSecret:
		secretPath := filepath.Join("/run/secrets", variableName)
		secret, err := os.ReadFile(secretPath)
		if err != nil {
			return "", false, fmt.Errorf("reading secret file: %w", err)
		}

		return strings.TrimSpace(string(secret)), false, nil
	case configVarTypeFileFromEnv:
		if !envVariableNamePattern.MatchString(variableName) {
			return "", true, nil
		}

		filePath, found := os.LookupEnv(variableName)
		if !found {
			return "", false, fmt.Errorf("readFileFromEnv: environment variable %s not found", variableName)
		}

		if !filepath.IsAbs(filePath) {
			return "", false, fmt.Errorf("readFileFromEnv: file path %s is not absolute", filePath)
		}

		fileContents, err := os.ReadFile(filePath)
		if err != nil {
			return "", false, fmt.Errorf("readFileFromEnv: reading file from %s: %w", variableName, err)
		}

		return strings.TrimSpace(string(fileContents)), false, nil
	default:
		return "", true, nil
	}
}

func formatWidgetInitError(err error, w widget) error {
	failedWidget := w
	var nested *widgetInitError
	if errors.As(err, &nested) && nested.widget != nil {
		failedWidget = nested.widget
	}

	return &widgetInitError{
		message: fmt.Sprintf("%s widget: %v", w.GetType(), err),
		widget:  failedWidget,
		cause:   err,
	}
}

func formatMicroWidgetInitError(err error, w widget) error {
	failedWidget := w
	var nested *widgetInitError
	if errors.As(err, &nested) && nested.widget != nil {
		failedWidget = nested.widget
	}

	return &widgetInitError{
		message: fmt.Sprintf("%s micro-widget: %v", w.GetType(), err),
		widget:  failedWidget,
		cause:   err,
	}
}

func widgetSourceAt(sources []configWidgetSemanticSources, index int) configWidgetSemanticSources {
	if index < 0 || index >= len(sources) {
		return configWidgetSemanticSources{}
	}
	return sources[index]
}

func findWidgetSemanticSource(
	candidate widget,
	source configWidgetSemanticSources,
	target widget,
) (configWidgetSemanticSources, bool) {
	if candidate == nil || target == nil {
		return configWidgetSemanticSources{}, false
	}
	if candidate == target {
		return source, true
	}

	container, ok := candidate.(widgetContainer)
	if !ok {
		return configWidgetSemanticSources{}, false
	}

	children := container.childWidgets()
	for i := range children {
		if found, ok := findWidgetSemanticSource(
			children[i],
			widgetSourceAt(source.widgets, i),
			target,
		); ok {
			return found, true
		}
	}

	return configWidgetSemanticSources{}, false
}

func appendRecoverableConfigError(config *config, parsed *parsedYAMLConfig, generatedLine int, err error) {
	if err == nil {
		return
	}
	config.recoverableErrors = append(
		config.recoverableErrors,
		semanticConfigDiagnostic(parsed, generatedLine, err),
	)
}

func recoverConfiguredThemeErrors(config *config, parsed *parsedYAMLConfig, sources *configSemanticSources) {
	rootLine := 0
	if sources != nil {
		rootLine = sources.root
	}

	if err := config.Theme.themeProperties.validate("theme"); err != nil {
		line := rootLine
		if sources != nil {
			line = semanticSourceLine(sources.theme, rootLine)
		}
		appendRecoverableConfigError(config, parsed, line, err)
		config.Theme.themeProperties = themeProperties{}
	}

	validKeys := make([]string, 0, len(config.Theme.Presets.keys))
	for _, key := range config.Theme.Presets.keys {
		properties, exists := config.Theme.Presets.data[key]
		if !exists || properties == nil {
			continue
		}
		if err := properties.validate("theme.presets." + key); err != nil {
			line := rootLine
			if sources != nil {
				line = semanticSourceLine(sources.themePreset[key], sources.themePresets, sources.theme, rootLine)
			}
			appendRecoverableConfigError(config, parsed, line, err)
			delete(config.Theme.Presets.data, key)
			continue
		}
		validKeys = append(validKeys, key)
	}
	config.Theme.Presets.keys = validKeys

	for i := range config.Pages {
		if err := config.Pages[i].Theme.validate(fmt.Sprintf("pages[%d].theme", i)); err != nil {
			line := rootLine
			if sources != nil && i < len(sources.page) {
				line = semanticSourceLine(sources.page[i].theme, sources.page[i].line, sources.pages, rootLine)
			}
			appendRecoverableConfigError(config, parsed, line, err)
			config.Pages[i].Theme = themeProperties{}
		}
	}
}

func recoverWidgetDefaultErrors(config *config, parsed *parsedYAMLConfig, sources *configSemanticSources) {
	rootLine := 0
	if sources != nil {
		rootLine = sources.root
	}

	globalOnly := widgetDefaultsConfig{Global: config.WidgetDefaults.Global}
	if err := validateWidgetDefaults(globalOnly); err != nil {
		line := rootLine
		if sources != nil {
			line = semanticSourceLine(sources.widgetDefaultsGlobal, sources.widgetDefaults, rootLine)
		}
		appendRecoverableConfigError(config, parsed, line, err)
		config.WidgetDefaults.Global = widgetDefaultValues{}
	}

	for widgetType, values := range config.WidgetDefaults.Types {
		typeOnly := widgetDefaultsConfig{Types: map[string]widgetDefaultValues{widgetType: values}}
		if err := validateWidgetDefaults(typeOnly); err != nil {
			line := rootLine
			if sources != nil {
				line = semanticSourceLine(sources.widgetDefaultType[widgetType], sources.widgetDefaults, rootLine)
			}
			appendRecoverableConfigError(config, parsed, line, err)
			delete(config.WidgetDefaults.Types, widgetType)
		}
	}
}

func recoverAnalyticsError(config *config, parsed *parsedYAMLConfig, sources *configSemanticSources) {
	if !config.Analytics.configured() {
		return
	}

	rootLine := 0
	analyticsLine := 0
	providerLine := 0
	endpointLine := 0
	if sources != nil {
		rootLine = sources.root
		analyticsLine = semanticSourceLine(sources.analytics, rootLine)
		providerLine = semanticSourceLine(sources.analyticsProvider, analyticsLine, rootLine)
		endpointLine = semanticSourceLine(sources.analyticsEndpoint, analyticsLine, rootLine)
	}

	provider := strings.ToLower(strings.TrimSpace(config.Analytics.Provider))
	var line int
	var err error
	switch {
	case provider == "":
		line = providerLine
		err = errors.New("analytics provider must be set")
	case provider != analyticsProviderGoatCounter:
		line = providerLine
		err = fmt.Errorf("unsupported analytics provider %q", config.Analytics.Provider)
	case strings.TrimSpace(config.Analytics.Endpoint) == "":
		line = endpointLine
		err = errors.New("analytics endpoint must be set")
	default:
		var normalized string
		normalized, err = normalizeAnalyticsEndpoint(config.Analytics.Endpoint)
		if err == nil {
			config.Analytics.Provider = provider
			config.Analytics.Endpoint = normalized
			return
		}
		line = endpointLine
		err = fmt.Errorf("analytics endpoint %w", err)
	}

	appendRecoverableConfigError(config, parsed, line, err)
	config.Analytics = analyticsConfig{}
}

func recoverAssetsPathError(config *config, parsed *parsedYAMLConfig, sources *configSemanticSources) {
	if config.Server.AssetsPath == "" {
		return
	}

	info, err := os.Stat(config.Server.AssetsPath)
	if err == nil && info.IsDir() {
		return
	}

	line := 0
	if sources != nil {
		line = semanticSourceLine(sources.assetsPath, sources.server, sources.root)
	}

	if err != nil {
		appendRecoverableConfigError(
			config,
			parsed,
			line,
			fmt.Errorf("assets directory is unavailable: %s: %w", config.Server.AssetsPath, err),
		)
	} else {
		appendRecoverableConfigError(
			config,
			parsed,
			line,
			fmt.Errorf("assets path is not a directory: %s", config.Server.AssetsPath),
		)
	}
	config.Server.AssetsPath = ""
}

func initializeConfiguredMicroWidgets(items microWidgets, sourceLines []int, defaults widgetDefaultsConfig) microWidgets {
	for i, candidate := range items {
		if _, invalid := candidate.(*invalidConfiguredMicroWidget); invalid {
			continue
		}
		micro := candidate.(microWidget)
		descriptor := microWidgetRegistry[candidate.GetType()]
		if descriptor.applyWidgetDefaults {
			if _, err := applyWidgetDefaultsTree(candidate, defaults); err != nil {
				line := 0
				if i < len(sourceLines) {
					line = sourceLines[i]
				}
				items[i] = newInvalidMicroWidget(candidate, candidate.GetType(), micro.GetPosition(), line, err)
				continue
			}
		}
		if err := candidate.initialize(); err != nil {
			line := 0
			if i < len(sourceLines) {
				line = sourceLines[i]
			}
			items[i] = newInvalidMicroWidget(candidate, candidate.GetType(), micro.GetPosition(), line, formatMicroWidgetInitError(err, candidate))
		}
	}
	return items
}

func resolveInvalidMicroWidgetDiagnostics(
	parsed *parsedYAMLConfig,
	items microWidgets,
	issues *[]error,
) {
	for _, candidate := range items {
		invalid, ok := candidate.(*invalidConfiguredMicroWidget)
		if !ok || invalid.ConfigError == nil {
			continue
		}

		diagnostic := semanticConfigDiagnostic(parsed, invalid.configLine, invalid.ConfigError)
		invalid.configError = diagnostic
		invalid.ConfigError = diagnostic
		invalid.Error = diagnostic
		*issues = append(*issues, diagnostic)
	}
}

func initializeConfiguredWidget(
	candidate widget,
	defaults widgetDefaultsConfig,
) (widgetDefaultsLogSummary, widget) {
	if _, invalid := candidate.(*invalidConfiguredWidget); invalid {
		return widgetDefaultsLogSummary{}, candidate
	}

	defaultsSummary, err := applyWidgetDefaultsTree(candidate, defaults)
	if err != nil {
		generatedLine := 0
		if base, ok := widgetBaseOf(candidate); ok {
			generatedLine = base.configLine
		}
		return widgetDefaultsLogSummary{}, newInvalidConfiguredWidget(
			candidate,
			candidate.GetType(),
			generatedLine,
			err,
		)
	}

	if err := candidate.initialize(); err != nil {
		formatted := formatWidgetInitError(err, candidate)
		generatedLine := 0
		if base, ok := widgetBaseOf(candidate); ok {
			generatedLine = base.configLine
		}
		return widgetDefaultsLogSummary{}, newInvalidConfiguredWidget(
			candidate,
			candidate.GetType(),
			generatedLine,
			formatted,
		)
	}

	return defaultsSummary, candidate
}

func resolveInvalidWidgetDiagnostics(
	parsed *parsedYAMLConfig,
	widgetList widgets,
	sources []configWidgetSemanticSources,
	issues *[]error,
) {
	for i := range widgetList {
		candidate := widgetList[i]
		source := widgetSourceAt(sources, i)

		if invalid, ok := candidate.(*invalidConfiguredWidget); ok {
			generatedLine := source.line
			if generatedLine == 0 {
				generatedLine = invalid.configLine
			}

			var templateErr *customAPITemplateParseError
			if errors.As(invalid.configError, &templateErr) &&
				templateErr.line > 0 &&
				source.template > 0 {
				generatedLine = source.template + templateErr.line
			}

			diagnostic := semanticConfigDiagnostic(parsed, generatedLine, invalid.configError)
			invalid.Error = diagnostic
			*issues = append(*issues, diagnostic)
			continue
		}

		container, ok := candidate.(widgetContainer)
		if !ok {
			continue
		}

		resolveInvalidWidgetDiagnostics(
			parsed,
			container.childWidgets(),
			source.widgets,
			issues,
		)
	}
}

func widgetInitializationDiagnostic(
	parsed *parsedYAMLConfig,
	err error,
	root widget,
	source configWidgetSemanticSources,
) error {
	if err == nil {
		return nil
	}

	target := root
	var initErr *widgetInitError
	if errors.As(err, &initErr) && initErr.widget != nil {
		target = initErr.widget
	}

	targetSource, found := findWidgetSemanticSource(root, source, target)
	if !found {
		return err
	}

	generatedLine := targetSource.line

	var templateErr *customAPITemplateParseError
	if errors.As(err, &templateErr) &&
		templateErr.line > 0 &&
		targetSource.template > 0 {
		generatedLine = targetSource.template + templateErr.line
	}

	return semanticConfigDiagnostic(parsed, generatedLine, err)
}

var configIncludePattern = regexp.MustCompile(`(?m)^([ \t]*)(?:-[ \t]*)?(?:!|\$)include:[ \t]*(.+)$`)

func parseYAMLIncludes(mainFilePath string) ([]byte, map[string]struct{}, error) {
	parsed, err := parseYAMLIncludesWithSources(mainFilePath)
	if err != nil {
		return nil, nil, err
	}

	return parsed.Contents, parsed.Includes, nil
}

func parseYAMLIncludesWithSources(mainFilePath string) (*parsedYAMLConfig, error) {
	return recursiveParseYAMLIncludesWithSources(mainFilePath, nil, 0)
}

func recursiveParseYAMLIncludes(mainFilePath string, includes map[string]struct{}, depth int) ([]byte, map[string]struct{}, error) {
	parsed, err := recursiveParseYAMLIncludesWithSources(mainFilePath, includes, depth)
	if err != nil {
		return nil, nil, err
	}

	return parsed.Contents, parsed.Includes, nil
}

func recursiveParseYAMLIncludesWithSources(mainFilePath string, includes map[string]struct{}, depth int) (*parsedYAMLConfig, error) {
	if depth > CONFIG_INCLUDE_RECURSION_DEPTH_LIMIT {
		return nil, fmt.Errorf("recursion depth limit of %d reached", CONFIG_INCLUDE_RECURSION_DEPTH_LIMIT)
	}

	mainFileContents, err := os.ReadFile(mainFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", mainFilePath, err)
	}

	mainFileAbsPath, err := filepath.Abs(mainFilePath)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path of %s: %w", mainFilePath, err)
	}
	mainFileDir := filepath.Dir(mainFileAbsPath)

	if includes == nil {
		includes = make(map[string]struct{})
	}

	lines := bytes.Split(mainFileContents, []byte("\n"))
	expandedLines := make([][]byte, 0, len(lines))
	sources := make([]configSourceLocation, 0, len(lines))

	for lineIndex, line := range lines {
		matches := configIncludePattern.FindSubmatch(line)
		if len(matches) == 0 {
			expandedLines = append(expandedLines, line)
			sources = append(sources, configSourceLocation{
				File: mainFileAbsPath,
				Line: lineIndex + 1,
			})
			continue
		}

		if len(matches) != 3 || !bytes.Equal(matches[0], line) {
			return nil, fmt.Errorf("invalid include match in %s at line %d", mainFileAbsPath, lineIndex+1)
		}

		indent := string(matches[1])
		includeFilePath := strings.TrimSpace(string(matches[2]))
		if !filepath.IsAbs(includeFilePath) {
			includeFilePath = filepath.Join(mainFileDir, includeFilePath)
		}

		includeFileAbsPath, err := filepath.Abs(includeFilePath)
		if err != nil {
			return nil, fmt.Errorf(
				"resolving include %s:%d: getting absolute path of %s: %w",
				mainFileAbsPath, lineIndex+1, includeFilePath, err,
			)
		}

		includes[includeFileAbsPath] = struct{}{}

		included, err := recursiveParseYAMLIncludesWithSources(includeFileAbsPath, includes, depth+1)
		if err != nil {
			return nil, fmt.Errorf(
				"resolving include %s:%d: %w",
				mainFileAbsPath, lineIndex+1, err,
			)
		}

		includedLines := bytes.Split(included.Contents, []byte("\n"))
		if len(includedLines) != len(included.Sources) {
			return nil, fmt.Errorf(
				"resolving include %s:%d: source map contains %d entries for %d generated lines",
				mainFileAbsPath, lineIndex+1, len(included.Sources), len(includedLines),
			)
		}

		for i := range includedLines {
			expandedLines = append(expandedLines, []byte(indent+string(includedLines[i])))
			sources = append(sources, included.Sources[i])
		}
	}

	return &parsedYAMLConfig{
		Contents: bytes.Join(expandedLines, []byte("\n")),
		Includes: includes,
		Sources:  sources,
	}, nil
}

func configFilesWatcherWithSources(
	mainFilePath string,
	lastParsed *parsedYAMLConfig,
	onChange func(newParsed *parsedYAMLConfig),
	onErr func(error),
) (func() error, error) {
	mainFileAbsPath, err := filepath.Abs(mainFilePath)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path of main file: %w", err)
	}

	lastParsed.Includes[mainFileAbsPath] = struct{}{}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating watcher: %w", err)
	}

	updateWatchedFiles := func(previousWatched map[string]struct{}, newWatched map[string]struct{}) {
		for filePath := range previousWatched {
			if _, ok := newWatched[filePath]; !ok {
				if err := watcher.Remove(filePath); err != nil {
					slog.Warn("Could not remove configuration file from watcher", "path", filePath, "error", err)
				}
			}
		}

		for filePath := range newWatched {
			if _, ok := previousWatched[filePath]; !ok {
				if err := watcher.Add(filePath); err != nil {
					slog.Warn(
						"Could not add configuration file to watcher",
						"path", filePath,
						"error", err,
					)
				}
			}
		}
	}

	updateWatchedFiles(nil, lastParsed.Includes)

	// needed for lastParsed because it gets updated in multiple goroutines
	mu := sync.Mutex{}

	callbackMu := sync.Mutex{}

	parseAndCompareBeforeCallback := func() {
		callbackMu.Lock()
		defer callbackMu.Unlock()

		currentParsed, err := parseYAMLIncludesWithSources(mainFilePath)
		if err != nil {
			onErr(&configWatcherReloadError{cause: err})
			return
		}

		currentParsed.Includes[mainFileAbsPath] = struct{}{}

		mu.Lock()
		defer mu.Unlock()

		if !maps.Equal(currentParsed.Includes, lastParsed.Includes) {
			updateWatchedFiles(lastParsed.Includes, currentParsed.Includes)
		}

		if !bytes.Equal(lastParsed.Contents, currentParsed.Contents) {
			lastParsed = currentParsed
			onChange(currentParsed)
			return
		}

		lastParsed.Includes = currentParsed.Includes
	}

	const debounceDuration = 500 * time.Millisecond
	var debounceMu sync.Mutex
	var debounceTimer *time.Timer
	watcherStopped := false

	debouncedParseAndCompareBeforeCallback := func() {
		debounceMu.Lock()
		defer debounceMu.Unlock()

		if watcherStopped {
			return
		}

		if debounceTimer != nil {
			debounceTimer.Stop()
			debounceTimer.Reset(debounceDuration)
		} else {
			debounceTimer = time.AfterFunc(debounceDuration, parseAndCompareBeforeCallback)
		}
	}

	deleteLastInclude := func(filePath string) {
		mu.Lock()
		defer mu.Unlock()
		fileAbsPath, _ := filepath.Abs(filePath)
		delete(lastParsed.Includes, fileAbsPath)
	}

	go func() {
		for {
			select {
			case event, isOpen := <-watcher.Events:
				if !isOpen {
					return
				}
				if event.Has(fsnotify.Write) {
					debouncedParseAndCompareBeforeCallback()
				} else if event.Has(fsnotify.Rename) {
					// on linux the file will no longer be watched after a rename, on windows
					// it will continue to be watched with the new name but we have no access to
					// the new name in this event in order to stop watching it manually and match the
					// behavior in linux, may lead to weird unintended behaviors on windows as we're
					// only handling renames from linux's perspective
					// see https://github.com/fsnotify/fsnotify/issues/255

					// remove the old file from our manually tracked includes, calling
					// debouncedParseAndCompareBeforeCallback will re-add it if it's still
					// required after it triggers
					deleteLastInclude(event.Name)

					// wait for file to maybe get created again
					// see https://github.com/glanceapp/glance/pull/358
					for range 10 {
						if _, err := os.Stat(event.Name); err == nil {
							break
						}
						time.Sleep(200 * time.Millisecond)
					}

					debouncedParseAndCompareBeforeCallback()
				} else if event.Has(fsnotify.Remove) {
					deleteLastInclude(event.Name)
					debouncedParseAndCompareBeforeCallback()
				}
			case err, isOpen := <-watcher.Errors:
				if !isOpen {
					return
				}
				onErr(fmt.Errorf("watcher error: %w", err))
			}
		}
	}()

	return func() error {
		debounceMu.Lock()
		watcherStopped = true
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceMu.Unlock()

		watcherErr := watcher.Close()

		// Wait for any in-progress configuration callback to finish before returning.
		callbackMu.Lock()
		callbackMu.Unlock() //nolint:staticcheck // SA2001: lock/unlock is an intentional synchronization barrier.

		return watcherErr
	}, nil
}

func configFilesWatcher(
	mainFilePath string,
	lastContents []byte,
	lastIncludes map[string]struct{},
	onChange func(newContents []byte),
	onErr func(error),
) (func() error, error) {
	return configFilesWatcherWithSources(
		mainFilePath,
		&parsedYAMLConfig{
			Contents: lastContents,
			Includes: lastIncludes,
		},
		func(newParsed *parsedYAMLConfig) {
			onChange(newParsed.Contents)
		},
		onErr,
	)
}

func yamlMappingValue(node *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Value == key {
			return keyNode, node.Content[i+1]
		}
	}

	return nil, nil
}

func parseWidgetSemanticSources(node *yaml.Node) []configWidgetSemanticSources {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}

	sources := make([]configWidgetSemanticSources, 0, len(node.Content))
	for _, widgetNode := range node.Content {
		source := configWidgetSemanticSources{line: widgetNode.Line}

		if _, templateNode := yamlMappingValue(widgetNode, "template"); templateNode != nil &&
			templateNode.Kind == yaml.ScalarNode &&
			templateNode.Style == yaml.LiteralStyle {
			source.template = templateNode.Line
		}

		if _, children := yamlMappingValue(widgetNode, "widgets"); children != nil {
			source.widgets = parseWidgetSemanticSources(children)
		}

		sources = append(sources, source)
	}

	return sources
}

func parseConfigSemanticSources(contents []byte) (*configSemanticSources, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return nil, err
	}

	sources := &configSemanticSources{
		users:               make(map[string]int),
		authGroup:           make(map[string]int),
		widgetDefaultType:   make(map[string]int),
		themePreset:         make(map[string]int),
		authAccessDashboard: make(map[string]int),
		dashboard:           make(map[string]int),
	}

	if len(document.Content) == 0 {
		return sources, nil
	}

	root := document.Content[0]
	sources.root = root.Line

	if key, server := yamlMappingValue(root, "server"); server != nil {
		sources.server = key.Line
		if key, value := yamlMappingValue(server, "assets-path"); value != nil {
			sources.assetsPath = key.Line
		}
		if key, personalState := yamlMappingValue(server, "personal-state"); personalState != nil {
			sources.personalState = key.Line
			if key, value := yamlMappingValue(personalState, "path"); value != nil {
				sources.personalStatePath = key.Line
			}
		}
	}

	if key, analytics := yamlMappingValue(root, "analytics"); analytics != nil {
		sources.analytics = key.Line
		if key, value := yamlMappingValue(analytics, "provider"); value != nil {
			sources.analyticsProvider = key.Line
		}
		if key, value := yamlMappingValue(analytics, "endpoint"); value != nil {
			sources.analyticsEndpoint = key.Line
		}
	}

	if key, auth := yamlMappingValue(root, "auth"); auth != nil {
		sources.auth = key.Line
		if key, value := yamlMappingValue(auth, "secret-key"); value != nil {
			sources.authSecret = key.Line
		}
		if usersKey, users := yamlMappingValue(auth, "users"); users != nil {
			sources.authUsers = usersKey.Line
			if users.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(users.Content); i += 2 {
					keyNode := users.Content[i]
					sources.users[keyNode.Value] = keyNode.Line
				}
			}
		}
		if groupsKey, groups := yamlMappingValue(auth, "groups"); groups != nil {
			sources.authGroups = groupsKey.Line
			if groups.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(groups.Content); i += 2 {
					keyNode := groups.Content[i]
					sources.authGroup[keyNode.Value] = keyNode.Line
				}
			}
		}
		if accessKey, access := yamlMappingValue(auth, "access"); access != nil {
			sources.authAccess = accessKey.Line
			if dashboardsKey, dashboards := yamlMappingValue(access, "dashboards"); dashboards != nil {
				sources.authAccessDashboards = dashboardsKey.Line
				if dashboards.Kind == yaml.MappingNode {
					for i := 0; i+1 < len(dashboards.Content); i += 2 {
						keyNode := dashboards.Content[i]
						sources.authAccessDashboard[keyNode.Value] = keyNode.Line
					}
				}
			}
		}
		if oidcKey, oidc := yamlMappingValue(auth, "oidc"); oidc != nil {
			sources.authOIDC = oidcKey.Line
			if key, value := yamlMappingValue(oidc, "issuer"); value != nil {
				sources.authOIDCIssuer = key.Line
			}
			if key, value := yamlMappingValue(oidc, "client-id"); value != nil {
				sources.authOIDCClientID = key.Line
			}
			if key, value := yamlMappingValue(oidc, "client-secret"); value != nil {
				sources.authOIDCSecret = key.Line
			}
			if key, value := yamlMappingValue(oidc, "redirect-url"); value != nil {
				sources.authOIDCRedirect = key.Line
			}
		}
	}

	if key, widgetDefaults := yamlMappingValue(root, "widget-defaults"); widgetDefaults != nil {
		sources.widgetDefaults = key.Line
		if globalKey, global := yamlMappingValue(widgetDefaults, "global"); global != nil {
			sources.widgetDefaultsGlobal = globalKey.Line
		}
		if _, types := yamlMappingValue(widgetDefaults, "types"); types != nil && types.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(types.Content); i += 2 {
				keyNode := types.Content[i]
				sources.widgetDefaultType[keyNode.Value] = keyNode.Line
			}
		}
	}

	if key, theme := yamlMappingValue(root, "theme"); theme != nil {
		sources.theme = key.Line
		if presetsKey, presets := yamlMappingValue(theme, "presets"); presets != nil {
			sources.themePresets = presetsKey.Line
			if presets.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(presets.Content); i += 2 {
					keyNode := presets.Content[i]
					sources.themePreset[keyNode.Value] = keyNode.Line
				}
			}
		}
	}

	if _, footer := yamlMappingValue(root, "footer-micro-widgets"); footer != nil {
		if _, left := yamlMappingValue(footer, "left"); left != nil && left.Kind == yaml.SequenceNode {
			sources.footerMicroLeft = make([]int, 0, len(left.Content))
			for _, item := range left.Content {
				sources.footerMicroLeft = append(sources.footerMicroLeft, item.Line)
			}
		}
		if _, right := yamlMappingValue(footer, "right"); right != nil && right.Kind == yaml.SequenceNode {
			sources.footerMicroRight = make([]int, 0, len(right.Content))
			for _, item := range right.Content {
				sources.footerMicroRight = append(sources.footerMicroRight, item.Line)
			}
		}
	}

	if key, dashboards := yamlMappingValue(root, "dashboards"); dashboards != nil {
		sources.dashboards = key.Line
		if dashboards.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(dashboards.Content); i += 2 {
				keyNode := dashboards.Content[i]
				sources.dashboard[keyNode.Value] = keyNode.Line
			}
		}
	}

	if key, pages := yamlMappingValue(root, "pages"); pages != nil {
		sources.pages = key.Line
		if pages.Kind == yaml.SequenceNode {
			sources.page = make([]configPageSemanticSources, 0, len(pages.Content))
			for _, pageNode := range pages.Content {
				pageSource := configPageSemanticSources{line: pageNode.Line}

				if key, value := yamlMappingValue(pageNode, "name"); value != nil {
					pageSource.name = key.Line
				}
				if key, value := yamlMappingValue(pageNode, "slug"); value != nil {
					pageSource.slug = key.Line
				}
				if key, value := yamlMappingValue(pageNode, "width"); value != nil {
					pageSource.width = key.Line
				}
				if key, value := yamlMappingValue(pageNode, "desktop-navigation-width"); value != nil {
					pageSource.desktopNavigationWidth = key.Line
				}
				if key, pageTheme := yamlMappingValue(pageNode, "theme"); pageTheme != nil {
					pageSource.theme = key.Line
				}
				if _, headWidgets := yamlMappingValue(pageNode, "head-widgets"); headWidgets != nil {
					pageSource.headWidgets = parseWidgetSemanticSources(headWidgets)
				}
				if _, bottomWidgets := yamlMappingValue(pageNode, "bottom-widgets"); bottomWidgets != nil {
					pageSource.bottomWidgets = parseWidgetSemanticSources(bottomWidgets)
				}
				if columnsKey, columns := yamlMappingValue(pageNode, "columns"); columns != nil {
					pageSource.columns = columnsKey.Line
					if columns.Kind == yaml.SequenceNode {
						pageSource.column = make([]configColumnSemanticSources, 0, len(columns.Content))
						for _, columnNode := range columns.Content {
							columnSource := configColumnSemanticSources{line: columnNode.Line}
							if key, value := yamlMappingValue(columnNode, "size"); value != nil {
								columnSource.size = key.Line
							}
							if _, widgets := yamlMappingValue(columnNode, "widgets"); widgets != nil {
								columnSource.widgets = parseWidgetSemanticSources(widgets)
							}
							pageSource.column = append(pageSource.column, columnSource)
						}
					}
				}

				sources.page = append(sources.page, pageSource)
			}
		}
	}

	return sources, nil
}

func semanticConfigDiagnostic(
	parsed *parsedYAMLConfig,
	generatedLine int,
	err error,
) error {
	if err == nil || generatedLine < 1 {
		return err
	}

	source, found := parsed.sourceLocation(generatedLine)
	if !found {
		return err
	}

	return &configDiagnostic{
		File:    source.File,
		Line:    source.Line,
		Message: err.Error(),
		cause:   err,
	}
}

func semanticSourceLine(lines ...int) int {
	for _, line := range lines {
		if line > 0 {
			return line
		}
	}
	return 0
}

func validateStatusBarPlacement(widgetList widgets, widgetSources []configWidgetSemanticSources, allowDirect bool) (int, error) {
	for i := range widgetList {
		candidate := widgetList[i]
		source := widgetSourceAt(widgetSources, i)

		if candidate.GetType() == "status-bar" && !allowDirect {
			return source.line, errors.New("status-bar widget can only be used directly in head-widgets or bottom-widgets")
		}

		container, ok := candidate.(widgetContainer)
		if !ok {
			continue
		}

		if line, err := validateStatusBarPlacement(container.childWidgets(), source.widgets, false); err != nil {
			return semanticSourceLine(line, source.line), err
		}
	}

	return 0, nil
}

func configPageDescription(page *page, index int) string {
	if page != nil && strings.TrimSpace(page.Title) != "" {
		return fmt.Sprintf("page %q", page.Title)
	}
	return fmt.Sprintf("page %d", index+1)
}

func validateAuthorizationConfig(
	config *config,
	parsed *parsedYAMLConfig,
	sources *configSemanticSources,
) error {
	if len(config.Auth.Access.Dashboards) == 0 {
		return nil
	}

	diagnostic := func(line int, err error) error {
		return semanticConfigDiagnostic(parsed, line, err)
	}

	rootLine := 0
	authLine := 0
	accessLine := 0
	accessDashboardsLine := 0
	if sources != nil {
		rootLine = sources.root
		authLine = semanticSourceLine(sources.auth, rootLine)
		accessLine = semanticSourceLine(sources.authAccess, authLine)
		accessDashboardsLine = semanticSourceLine(sources.authAccessDashboards, accessLine)
	}

	authConfigured := len(config.Auth.Users) > 0 || config.Auth.OIDC.configured()
	if !authConfigured {
		return diagnostic(
			accessLine,
			errors.New("auth access requires local users or OIDC authentication to be configured"),
		)
	}

	if len(config.Dashboards.keys) == 0 {
		return diagnostic(
			accessDashboardsLine,
			errors.New("auth access dashboards requires dashboards configuration"),
		)
	}

	for groupName, group := range config.Auth.Groups {
		line := authLine
		if sources != nil {
			line = semanticSourceLine(sources.authGroup[groupName], sources.authGroups, authLine)
		}

		if strings.TrimSpace(groupName) == "" {
			return diagnostic(line, errors.New("auth group has no name"))
		}
		if len(group.Users) == 0 {
			return diagnostic(line, fmt.Errorf("auth group %q has no users", groupName))
		}

		seenUsers := make(map[string]struct{}, len(group.Users))
		for _, identity := range group.Users {
			if strings.TrimSpace(identity) == "" {
				return diagnostic(line, fmt.Errorf("auth group %q contains an empty user", groupName))
			}
			if _, exists := seenUsers[identity]; exists {
				return diagnostic(
					line,
					fmt.Errorf("auth group %q contains duplicate user %q", groupName, identity),
				)
			}
			seenUsers[identity] = struct{}{}
		}
	}

	for dashboardName, rule := range config.Auth.Access.Dashboards {
		line := accessDashboardsLine
		if sources != nil {
			line = semanticSourceLine(
				sources.authAccessDashboard[dashboardName],
				sources.authAccessDashboards,
				sources.authAccess,
				sources.auth,
				rootLine,
			)
		}

		if _, exists := config.Dashboards.Get(dashboardName); !exists {
			return diagnostic(
				line,
				fmt.Errorf("auth access references unknown dashboard %q", dashboardName),
			)
		}

		if len(rule.Users) == 0 && len(rule.Groups) == 0 {
			return diagnostic(
				line,
				fmt.Errorf("auth access dashboard %q has no users or groups", dashboardName),
			)
		}

		seenUsers := make(map[string]struct{}, len(rule.Users))
		for _, identity := range rule.Users {
			if strings.TrimSpace(identity) == "" {
				return diagnostic(
					line,
					fmt.Errorf("auth access dashboard %q contains an empty user", dashboardName),
				)
			}
			if _, exists := seenUsers[identity]; exists {
				return diagnostic(
					line,
					fmt.Errorf(
						"auth access dashboard %q contains duplicate user %q",
						dashboardName,
						identity,
					),
				)
			}
			seenUsers[identity] = struct{}{}
		}

		seenGroups := make(map[string]struct{}, len(rule.Groups))
		for _, groupName := range rule.Groups {
			if strings.TrimSpace(groupName) == "" {
				return diagnostic(
					line,
					fmt.Errorf("auth access dashboard %q contains an empty group", dashboardName),
				)
			}
			if _, exists := seenGroups[groupName]; exists {
				return diagnostic(
					line,
					fmt.Errorf(
						"auth access dashboard %q contains duplicate group %q",
						dashboardName,
						groupName,
					),
				)
			}
			if _, exists := config.Auth.Groups[groupName]; !exists {
				return diagnostic(
					line,
					fmt.Errorf(
						"auth access dashboard %q references unknown group %q",
						dashboardName,
						groupName,
					),
				)
			}
			seenGroups[groupName] = struct{}{}
		}
	}

	return nil
}

func isConfigStateValid(config *config) error {
	return isConfigStateValidWithSources(config, nil, nil)
}

func isConfigStateValidWithSources(
	config *config,
	parsed *parsedYAMLConfig,
	sources *configSemanticSources,
) error {
	diagnostic := func(line int, err error) error {
		return semanticConfigDiagnostic(parsed, line, err)
	}

	rootLine := 0
	if sources != nil {
		rootLine = sources.root
	}

	if len(config.Pages) == 0 {
		line := rootLine
		if sources != nil {
			line = semanticSourceLine(sources.pages, rootLine)
		}
		return diagnostic(line, fmt.Errorf("no pages configured"))
	}

	if len(config.Dashboards.keys) > 0 {
		if _, exists := config.Dashboards.Get("Default"); !exists {
			line := rootLine
			if sources != nil {
				line = semanticSourceLine(sources.dashboards, rootLine)
			}
			return diagnostic(line, fmt.Errorf("dashboards configuration requires a Default dashboard"))
		}

		for dashboardName, pageSlugs := range config.Dashboards.Items() {
			line := rootLine
			if sources != nil {
				line = semanticSourceLine(sources.dashboard[dashboardName], sources.dashboards, rootLine)
			}

			if strings.TrimSpace(dashboardName) == "" {
				return diagnostic(line, fmt.Errorf("dashboard has no name"))
			}

			if len(pageSlugs) == 0 {
				return diagnostic(line, fmt.Errorf("dashboard %q has no pages", dashboardName))
			}

			seenPageSlugs := make(map[string]struct{}, len(pageSlugs))
			for _, pageSlug := range pageSlugs {
				if strings.TrimSpace(pageSlug) == "" {
					return diagnostic(line, fmt.Errorf("dashboard %q contains an empty page slug", dashboardName))
				}

				if _, exists := seenPageSlugs[pageSlug]; exists {
					return diagnostic(line, fmt.Errorf("dashboard %q contains duplicate page slug %q", dashboardName, pageSlug))
				}

				seenPageSlugs[pageSlug] = struct{}{}
			}
		}
	}

	if len(config.Server.ResourceProxy.AllowedOrigins) > 0 {
		seenOrigins := make(map[string]struct{}, len(config.Server.ResourceProxy.AllowedOrigins))
		for _, configuredOrigin := range config.Server.ResourceProxy.AllowedOrigins {
			normalized, err := normalizeResourceProxyOrigin(configuredOrigin)
			if err != nil {
				if strings.TrimSpace(configuredOrigin) == "" {
					return diagnostic(rootLine, fmt.Errorf("server resource-proxy allowed-origins contains an empty origin"))
				}
				return diagnostic(rootLine, fmt.Errorf("server resource-proxy origin %w", err))
			}
			if _, exists := seenOrigins[normalized]; exists {
				return diagnostic(rootLine, fmt.Errorf("server resource-proxy allowed-origins contains duplicate origin %q", configuredOrigin))
			}
			seenOrigins[normalized] = struct{}{}
		}
	}

	if len(config.Server.TrustedProxies) > 0 {
		if !config.Server.Proxied {
			return diagnostic(rootLine, fmt.Errorf("server trusted-proxies requires proxied to be enabled"))
		}

		for _, trustedProxy := range config.Server.TrustedProxies {
			trustedProxy = strings.TrimSpace(trustedProxy)
			if trustedProxy == "" {
				return diagnostic(rootLine, fmt.Errorf("server trusted-proxies contains an empty address"))
			}

			if strings.Contains(trustedProxy, "/") {
				if _, err := netip.ParsePrefix(trustedProxy); err != nil {
					return diagnostic(rootLine, fmt.Errorf("invalid trusted proxy %q: %w", trustedProxy, err))
				}
				continue
			}

			if _, err := netip.ParseAddr(trustedProxy); err != nil {
				return diagnostic(rootLine, fmt.Errorf("invalid trusted proxy %q: %w", trustedProxy, err))
			}
		}
	}

	authConfigured := len(config.Auth.Users) > 0 || config.Auth.OIDC.configured()
	if authConfigured && config.Auth.SecretKey == "" {
		line := rootLine
		if sources != nil {
			line = semanticSourceLine(sources.auth, rootLine)
		}
		return diagnostic(line, fmt.Errorf("secret-key must be set when authentication is configured"))
	}

	if config.Auth.OIDC.configured() {
		oidcLine := rootLine
		if sources != nil {
			oidcLine = semanticSourceLine(sources.authOIDC, sources.auth, rootLine)
		}

		if config.Auth.OIDC.Issuer == "" {
			return diagnostic(oidcLine, fmt.Errorf("OIDC issuer must be set"))
		}
		if config.Auth.OIDC.ClientID == "" {
			line := oidcLine
			if sources != nil {
				line = semanticSourceLine(sources.authOIDCClientID, sources.authOIDC, sources.auth, rootLine)
			}
			return diagnostic(line, fmt.Errorf("OIDC client-id must be set"))
		}
		if config.Auth.OIDC.ClientSecret == "" {
			line := oidcLine
			if sources != nil {
				line = semanticSourceLine(sources.authOIDCSecret, sources.authOIDC, sources.auth, rootLine)
			}
			return diagnostic(line, fmt.Errorf("OIDC client-secret must be set"))
		}

		issuerURL, err := url.Parse(config.Auth.OIDC.Issuer)
		if err != nil || issuerURL.Scheme != "https" || issuerURL.Host == "" {
			line := oidcLine
			if sources != nil {
				line = semanticSourceLine(sources.authOIDCIssuer, sources.authOIDC, sources.auth, rootLine)
			}
			return diagnostic(line, fmt.Errorf("OIDC issuer must be an absolute HTTPS URL"))
		}

		if config.Auth.OIDC.RedirectURL != "" {
			redirectURL, err := url.Parse(config.Auth.OIDC.RedirectURL)
			if err != nil || redirectURL.Scheme != "https" || redirectURL.Host == "" {
				line := oidcLine
				if sources != nil {
					line = semanticSourceLine(sources.authOIDCRedirect, sources.authOIDC, sources.auth, rootLine)
				}
				return diagnostic(line, fmt.Errorf("OIDC redirect-url must be an absolute HTTPS URL"))
			}
		}

		allowedUsers := make(map[string]struct{}, len(config.Auth.OIDC.AllowedUsers))
		for _, allowedUser := range config.Auth.OIDC.AllowedUsers {
			normalized := strings.ToLower(strings.TrimSpace(allowedUser))
			if normalized == "" {
				return diagnostic(oidcLine, fmt.Errorf("OIDC allowed-users entries must not be empty"))
			}
			if _, exists := allowedUsers[normalized]; exists {
				return diagnostic(oidcLine, fmt.Errorf("OIDC allowed-users contains duplicate user %q", allowedUser))
			}
			allowedUsers[normalized] = struct{}{}
		}
	}

	for username := range config.Auth.Users {
		line := rootLine
		if sources != nil {
			line = semanticSourceLine(sources.users[username], sources.authUsers, sources.auth, rootLine)
		}

		if username == "" {
			return diagnostic(line, fmt.Errorf("user has no name"))
		}

		if len(username) < 3 {
			return diagnostic(line, errors.New("usernames must be at least 3 characters"))
		}

		user := config.Auth.Users[username]

		if user.Password == "" {
			if user.PasswordHashString == "" {
				return diagnostic(line, fmt.Errorf("user %s must have a password or a password-hash set", username))
			}
		} else if len(user.Password) < 6 {
			return diagnostic(line, fmt.Errorf("the password for %s must be at least 6 characters", username))
		} else if len([]byte(user.Password)) > 72 {
			return diagnostic(line, fmt.Errorf("the password for %s must be at most 72 bytes", username))
		}
	}

	if err := validateAuthorizationConfig(config, parsed, sources); err != nil {
		return err
	}

	if config.Server.PersonalState.Enabled {
		line := rootLine
		pathLine := rootLine
		if sources != nil {
			line = semanticSourceLine(sources.personalState, sources.server, rootLine)
			pathLine = semanticSourceLine(sources.personalStatePath, line)
		}
		if len(config.Auth.Users) == 0 && !config.Auth.OIDC.configured() {
			return diagnostic(line, errors.New("server personal-state requires authentication"))
		}
		if strings.TrimSpace(config.Server.PersonalState.Path) == "" {
			return diagnostic(pathLine, errors.New("server personal-state path is required when enabled"))
		}
		if !filepath.IsAbs(config.Server.PersonalState.Path) {
			return diagnostic(pathLine, errors.New("server personal-state path must be absolute"))
		}
	}

	for i := range config.Pages {
		page := &config.Pages[i]
		pageDescription := configPageDescription(page, i)

		var pageSource configPageSemanticSources
		if sources != nil && i < len(sources.page) {
			pageSource = sources.page[i]
		}
		pagesLine := rootLine
		if sources != nil {
			pagesLine = semanticSourceLine(sources.pages, rootLine)
		}
		pageLine := semanticSourceLine(pageSource.line, pagesLine)

		if line, err := validateStatusBarPlacement(page.HeadWidgets, pageSource.headWidgets, true); err != nil {
			return diagnostic(semanticSourceLine(line, pageLine), err)
		}

		if line, err := validateStatusBarPlacement(page.BottomWidgets, pageSource.bottomWidgets, true); err != nil {
			return diagnostic(semanticSourceLine(line, pageLine), err)
		}

		if page.Title == "" {
			return diagnostic(
				semanticSourceLine(pageSource.name, pageLine),
				fmt.Errorf("page %d has no name", i+1),
			)
		}

		if page.Width != "" && (page.Width != "wide" && page.Width != "slim" && page.Width != "default") {
			return diagnostic(
				semanticSourceLine(pageSource.width, pageLine),
				fmt.Errorf("%s: width can only be either wide, slim or default", pageDescription),
			)
		}

		if page.DesktopNavigationWidth != "" {
			if page.DesktopNavigationWidth != "wide" && page.DesktopNavigationWidth != "slim" && page.DesktopNavigationWidth != "default" {
				return diagnostic(
					semanticSourceLine(pageSource.desktopNavigationWidth, pageLine),
					fmt.Errorf("%s: desktop-navigation-width can only be either wide, slim or default", pageDescription),
				)
			}
		}

		if len(page.Columns) == 0 {
			return diagnostic(
				semanticSourceLine(pageSource.columns, pageLine),
				fmt.Errorf("%s has no columns", pageDescription),
			)
		}

		if page.Width == "slim" {
			if len(page.Columns) > 2 {
				return diagnostic(
					semanticSourceLine(pageSource.columns, pageLine),
					fmt.Errorf("%s is slim and cannot have more than 2 columns", pageDescription),
				)
			}
		} else {
			if len(page.Columns) > 3 {
				return diagnostic(
					semanticSourceLine(pageSource.columns, pageLine),
					fmt.Errorf("%s has more than 3 columns", pageDescription),
				)
			}
		}

		columnSizesCount := make(map[string]int)

		for j := range page.Columns {
			column := &page.Columns[j]

			var columnSource configColumnSemanticSources
			if j < len(pageSource.column) {
				columnSource = pageSource.column[j]
			}
			columnLine := semanticSourceLine(columnSource.line, pageSource.columns, pageLine)

			if line, err := validateStatusBarPlacement(column.Widgets, columnSource.widgets, false); err != nil {
				return diagnostic(semanticSourceLine(line, columnLine), err)
			}

			if column.Size != "small" && column.Size != "medium" && column.Size != "full" {
				return diagnostic(
					semanticSourceLine(columnSource.size, columnLine),
					fmt.Errorf("column %d of %s: size can only be either small, medium or full", j+1, pageDescription),
				)
			}

			columnSizesCount[column.Size]++
		}

		full := columnSizesCount["full"]
		medium := columnSizesCount["medium"]

		if medium == 0 {
			if full > 2 || full == 0 {
				return diagnostic(
					semanticSourceLine(pageSource.columns, pageLine),
					fmt.Errorf("%s must have either 1 or 2 full width columns", pageDescription),
				)
			}
			continue
		}

		threeMediumLayout := len(page.Columns) == 3 && medium == 3
		mediumFullLayout := len(page.Columns) == 2 && medium == 1 && full == 1

		if !threeMediumLayout && !mediumFullLayout {
			return diagnostic(
				semanticSourceLine(pageSource.columns, pageLine),
				fmt.Errorf("%s has an invalid column layout", pageDescription),
			)
		}

	}

	return nil
}

// Read-only way to store ordered maps from a YAML structure
type orderedYAMLMap[K comparable, V any] struct {
	keys []K
	data map[K]V
}

func newOrderedYAMLMap[K comparable, V any](keys []K, values []V) (*orderedYAMLMap[K, V], error) {
	if len(keys) != len(values) {
		return nil, fmt.Errorf("keys and values must have the same length")
	}

	om := &orderedYAMLMap[K, V]{
		keys: make([]K, len(keys)),
		data: make(map[K]V, len(keys)),
	}

	copy(om.keys, keys)

	for i := range keys {
		om.data[keys[i]] = values[i]
	}

	return om, nil
}

func (om *orderedYAMLMap[K, V]) Items() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, key := range om.keys {
			value, ok := om.data[key]
			if !ok {
				continue
			}
			if !yield(key, value) {
				return
			}
		}
	}
}

func (om *orderedYAMLMap[K, V]) Get(key K) (V, bool) {
	value, ok := om.data[key]
	return value, ok
}

func (self *orderedYAMLMap[K, V]) Merge(other *orderedYAMLMap[K, V]) *orderedYAMLMap[K, V] {
	merged := &orderedYAMLMap[K, V]{
		keys: make([]K, 0, len(self.keys)+len(other.keys)),
		data: make(map[K]V, len(self.data)+len(other.data)),
	}

	merged.keys = append(merged.keys, self.keys...)
	maps.Copy(merged.data, self.data)

	for _, key := range other.keys {
		if _, exists := self.data[key]; !exists {
			merged.keys = append(merged.keys, key)
		}
	}
	maps.Copy(merged.data, other.data)

	return merged
}

func (om *orderedYAMLMap[K, V]) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("orderedMap: expected mapping node, got %d", node.Kind)
	}

	if len(node.Content)%2 != 0 {
		return fmt.Errorf("orderedMap: expected even number of content items, got %d", len(node.Content))
	}

	om.keys = make([]K, len(node.Content)/2)
	om.data = make(map[K]V, len(node.Content)/2)

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		var key K
		if err := keyNode.Decode(&key); err != nil {
			return fmt.Errorf("orderedMap: decoding key: %w", err)
		}

		if _, ok := om.data[key]; ok {
			return fmt.Errorf("orderedMap: duplicate key %v", key)
		}

		var value V
		if err := valueNode.Decode(&value); err != nil {
			return fmt.Errorf("orderedMap: decoding value: %w", err)
		}

		(*om).keys[i/2] = key
		(*om).data[key] = value
	}

	return nil
}
