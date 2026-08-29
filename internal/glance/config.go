package glance

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"iter"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
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
		Host       string `yaml:"host"`
		Port       uint16 `yaml:"port"`
		Proxied    bool   `yaml:"proxied"`
		AssetsPath string `yaml:"assets-path"`
		BaseURL    string `yaml:"base-url"`
	} `yaml:"server"`

	Auth struct {
		SecretKey string           `yaml:"secret-key"`
		Users     map[string]*user `yaml:"users"`
	} `yaml:"auth"`

	Document struct {
		Head template.HTML `yaml:"head"`
	} `yaml:"document"`

	Theme struct {
		themeProperties `yaml:",inline"`
		CustomCSSFile   string `yaml:"custom-css-file"`

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

	Pages      []page                           `yaml:"pages"`
	Dashboards orderedYAMLMap[string, []string] `yaml:"dashboards"`
}

type user struct {
	Password           string `yaml:"password"`
	PasswordHashString string `yaml:"password-hash"`
	PasswordHash       []byte `yaml:"-"`
}

type page struct {
	Title                  string  `yaml:"name"`
	Slug                   string  `yaml:"slug"`
	Width                  string  `yaml:"width"`
	DesktopNavigationWidth string  `yaml:"desktop-navigation-width"`
	ShowMobileHeader       bool    `yaml:"show-mobile-header"`
	HideDesktopNavigation  bool    `yaml:"hide-desktop-navigation"`
	CenterVertically       bool    `yaml:"center-vertically"`
	HeadWidgets            widgets `yaml:"head-widgets"`
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

type configSemanticSources struct {
	root       int
	server     int
	assetsPath int
	auth       int
	authUsers  int
	users      map[string]int
	dashboards int
	dashboard  map[string]int
	pages      int
	page       []configPageSemanticSources
}

type configPageSemanticSources struct {
	line                   int
	name                   int
	width                  int
	desktopNavigationWidth int
	headWidgets            []configWidgetSemanticSources
	columns                int
	column                 []configColumnSemanticSources
}

type configColumnSemanticSources struct {
	line    int
	size    int
	widgets []configWidgetSemanticSources
}

type configWidgetSemanticSources struct {
	line    int
	widgets []configWidgetSemanticSources
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
	return newConfigFromParsedYAML(&parsedYAMLConfig{Contents: contents})
}

func newConfigFromParsedYAML(parsed *parsedYAMLConfig) (*config, error) {
	contents, err := parseConfigVariables(parsed.Contents)
	if err != nil {
		return nil, err
	}

	config := &config{}
	config.Server.Port = 8080

	err = yaml.Unmarshal(contents, config)
	if err != nil {
		return nil, configDiagnosticFromYAMLError(parsed, err)
	}

	semanticSources, err := parseConfigSemanticSources(contents)
	if err != nil {
		return nil, configDiagnosticFromYAMLError(parsed, err)
	}

	if err = isConfigStateValidWithSources(config, parsed, semanticSources); err != nil {
		return nil, err
	}

	for p := range config.Pages {
		var pageSource configPageSemanticSources
		if semanticSources != nil && p < len(semanticSources.page) {
			pageSource = semanticSources.page[p]
		}

		for w := range config.Pages[p].HeadWidgets {
			candidate := config.Pages[p].HeadWidgets[w]
			if err := candidate.initialize(); err != nil {
				formatted := formatWidgetInitError(err, candidate)
				return nil, widgetInitializationDiagnostic(
					parsed,
					formatted,
					candidate,
					widgetSourceAt(pageSource.headWidgets, w),
				)
			}
		}

		for c := range config.Pages[p].Columns {
			var columnSource configColumnSemanticSources
			if c < len(pageSource.column) {
				columnSource = pageSource.column[c]
			}

			for w := range config.Pages[p].Columns[c].Widgets {
				candidate := config.Pages[p].Columns[c].Widgets[w]
				if err := candidate.initialize(); err != nil {
					formatted := formatWidgetInitError(err, candidate)
					return nil, widgetInitializationDiagnostic(
						parsed,
						formatted,
						candidate,
						widgetSourceAt(columnSource.widgets, w),
					)
				}
			}
		}
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
	var err error

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
			err = fmt.Errorf("parsing variable: %v", localErr)
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
		return nil, err
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
			return "", false, fmt.Errorf("reading secret file: %v", err)
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
			return "", false, fmt.Errorf("readFileFromEnv: reading file from %s: %v", variableName, err)
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

	return semanticConfigDiagnostic(parsed, targetSource.line, err)
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

	// TODO: refactor, flaky
	lastParsed.Includes[mainFileAbsPath] = struct{}{}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating watcher: %w", err)
	}

	updateWatchedFiles := func(previousWatched map[string]struct{}, newWatched map[string]struct{}) {
		for filePath := range previousWatched {
			if _, ok := newWatched[filePath]; !ok {
				watcher.Remove(filePath)
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

	parseAndCompareBeforeCallback := func() {
		currentParsed, err := parseYAMLIncludesWithSources(mainFilePath)
		if err != nil {
			onErr(fmt.Errorf("parsing main file contents for comparison: %w", err))
			return
		}

		// TODO: refactor, flaky
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
	var debounceTimer *time.Timer
	debouncedParseAndCompareBeforeCallback := func() {
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

	onChange(lastParsed)

	return func() error {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}

		return watcher.Close()
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
		users:     make(map[string]int),
		dashboard: make(map[string]int),
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
	}

	if key, auth := yamlMappingValue(root, "auth"); auth != nil {
		sources.auth = key.Line
		if usersKey, users := yamlMappingValue(auth, "users"); users != nil {
			sources.authUsers = usersKey.Line
			if users.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(users.Content); i += 2 {
					keyNode := users.Content[i]
					sources.users[keyNode.Value] = keyNode.Line
				}
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
				if key, value := yamlMappingValue(pageNode, "width"); value != nil {
					pageSource.width = key.Line
				}
				if key, value := yamlMappingValue(pageNode, "desktop-navigation-width"); value != nil {
					pageSource.desktopNavigationWidth = key.Line
				}
				if _, headWidgets := yamlMappingValue(pageNode, "head-widgets"); headWidgets != nil {
					pageSource.headWidgets = parseWidgetSemanticSources(headWidgets)
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

func configPageDescription(page *page, index int) string {
	if page != nil && strings.TrimSpace(page.Title) != "" {
		return fmt.Sprintf("page %q", page.Title)
	}
	return fmt.Sprintf("page %d", index+1)
}

// TODO: Refactor, we currently validate in two different places, this being
// one of them, which doesn't modify the data and only checks for logical errors
// and then again when creating the application which does modify the data and do
// further validation. Would be better if validation was done in a single place.
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

	if len(config.Auth.Users) > 0 && config.Auth.SecretKey == "" {
		line := rootLine
		if sources != nil {
			line = semanticSourceLine(sources.authUsers, sources.auth, rootLine)
		}
		return diagnostic(line, fmt.Errorf("secret-key must be set when users are configured"))
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
		}
	}

	if config.Server.AssetsPath != "" {
		if _, err := os.Stat(config.Server.AssetsPath); os.IsNotExist(err) {
			line := rootLine
			if sources != nil {
				line = semanticSourceLine(sources.assetsPath, sources.server, rootLine)
			}
			return diagnostic(line, fmt.Errorf("assets directory does not exist: %s", config.Server.AssetsPath))
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

			if column.Size != "small" && column.Size != "full" {
				return diagnostic(
					semanticSourceLine(columnSource.size, columnLine),
					fmt.Errorf("column %d of %s: size can only be either small or full", j+1, pageDescription),
				)
			}

			columnSizesCount[page.Columns[j].Size]++
		}

		full := columnSizesCount["full"]

		if full > 2 || full == 0 {
			return diagnostic(
				semanticSourceLine(pageSource.columns, pageLine),
				fmt.Errorf("%s must have either 1 or 2 full width columns", pageDescription),
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
			return fmt.Errorf("orderedMap: decoding key: %v", err)
		}

		if _, ok := om.data[key]; ok {
			return fmt.Errorf("orderedMap: duplicate key %v", key)
		}

		var value V
		if err := valueNode.Decode(&value); err != nil {
			return fmt.Errorf("orderedMap: decoding value: %v", err)
		}

		(*om).keys[i/2] = key
		(*om).data[key] = value
	}

	return nil
}
