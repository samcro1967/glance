package glance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseConfigVariablesIgnoresComments(t *testing.T) {
	// Set up an environment variable that should be resolved
	os.Setenv("TEST_API_KEY", "my-secret-value")
	defer os.Unsetenv("TEST_API_KEY")

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "variable in comment is not expanded",
			input:    "api-key: ${TEST_API_KEY} # Use secrets with ${secret:my-token}",
			expected: "api-key: my-secret-value # Use secrets with ${secret:my-token}",
		},
		{
			name:     "variable before comment is expanded",
			input:    "key: ${TEST_API_KEY} # this is a comment",
			expected: "key: my-secret-value # this is a comment",
		},
		{
			name:     "no comment, variable is expanded",
			input:    "key: ${TEST_API_KEY}",
			expected: "key: my-secret-value",
		},
		{
			name:     "hash inside double quotes is not a comment",
			input:    `key: "${TEST_API_KEY} # not a comment ${TEST_API_KEY}"`,
			expected: `key: "my-secret-value # not a comment my-secret-value"`,
		},
		{
			name:     "hash inside single quotes is not a comment",
			input:    `key: '${TEST_API_KEY} # not a comment ${TEST_API_KEY}'`,
			expected: `key: 'my-secret-value # not a comment my-secret-value'`,
		},
		{
			name:     "comment-only line is not expanded",
			input:    "# ${secret:some-secret}",
			expected: "# ${secret:some-secret}",
		},
		{
			name:     "multiple lines with mixed comments",
			input:    "key1: ${TEST_API_KEY}\n# comment with ${secret:x}\nkey2: ${TEST_API_KEY} # ${secret:y}",
			expected: "key1: my-secret-value\n# comment with ${secret:x}\nkey2: my-secret-value # ${secret:y}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseConfigVariables([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseConfigVariables() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(result) != tt.expected {
				t.Errorf("parseConfigVariables()\ngot:  %q\nwant: %q", string(result), tt.expected)
			}
		})
	}
}

func TestFindYAMLCommentStart(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"no comment here", -1},
		{"# full line comment", 0},
		{"key: value # comment", 11},
		{`key: "value # not comment"`, -1},
		{`key: 'value # not comment'`, -1},
		{`key: "quoted" # comment`, 14},
		{`key: "value \"quoted # text\""`, -1},
		{`key: "value \"quoted # text\"" # comment`, 31},
		{`key: 'value ''quoted # text'''`, -1},
		{`key: 'value ''quoted # text''' # comment`, 31},
		{"key: value#no-space-not-comment", -1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := findYAMLCommentStart([]byte(tt.input))
			if got != tt.expected {
				t.Errorf("findYAMLCommentStart(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func writeConfigTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func absConfigTestPath(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func assertConfigSources(t *testing.T, got, want []configSourceLocation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d source entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseYAMLIncludesWithSourcesRootFile(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "glance.yml")
	contents := "first: value\n\nthird: value\n"
	writeConfigTestFile(t, mainPath, contents)

	parsed, err := parseYAMLIncludesWithSources(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(parsed.Contents) != contents {
		t.Fatalf("contents mismatch\ngot:  %q\nwant: %q", parsed.Contents, contents)
	}

	mainAbs := absConfigTestPath(t, mainPath)
	assertConfigSources(t, parsed.Sources, []configSourceLocation{
		{File: mainAbs, Line: 1},
		{File: mainAbs, Line: 2},
		{File: mainAbs, Line: 3},
		{File: mainAbs, Line: 4},
	})
}

func TestParseYAMLIncludesWithSourcesIncludes(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		want       string
		wantSource []struct {
			file string
			line int
		}
		wantIncludes []string
	}{
		{
			name: "direct",
			files: map[string]string{
				"glance.yml": "pages:\n  - $include: page.yml\nfooter: value",
				"page.yml":   "name: Home\nslug: home",
			},
			want: "pages:\n  name: Home\n  slug: home\nfooter: value",
			wantSource: []struct {
				file string
				line int
			}{
				{"glance.yml", 1},
				{"page.yml", 1},
				{"page.yml", 2},
				{"glance.yml", 3},
			},
			wantIncludes: []string{"page.yml"},
		},
		{
			name: "nested",
			files: map[string]string{
				"glance.yml": "pages:\n  - $include: page.yml",
				"page.yml":   "name: Home\nwidgets:\n  - $include: widget.yml",
				"widget.yml": "type: clock\ntitle: Clock",
			},
			want: "pages:\n  name: Home\n  widgets:\n    type: clock\n    title: Clock",
			wantSource: []struct {
				file string
				line int
			}{
				{"glance.yml", 1},
				{"page.yml", 1},
				{"page.yml", 2},
				{"widget.yml", 1},
				{"widget.yml", 2},
			},
			wantIncludes: []string{"page.yml", "widget.yml"},
		},
		{
			name: "multiple",
			files: map[string]string{
				"glance.yml": "$include: first.yml\nmiddle: true\n$include: second.yml",
				"first.yml":  "first: true",
				"second.yml": "second: true",
			},
			want: "first: true\nmiddle: true\nsecond: true",
			wantSource: []struct {
				file string
				line int
			}{
				{"first.yml", 1},
				{"glance.yml", 2},
				{"second.yml", 1},
			},
			wantIncludes: []string{"first.yml", "second.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range tt.files {
				writeConfigTestFile(t, filepath.Join(dir, name), contents)
			}

			parsed, err := parseYAMLIncludesWithSources(filepath.Join(dir, "glance.yml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(parsed.Contents) != tt.want {
				t.Fatalf("expanded contents mismatch\ngot:\n%s\nwant:\n%s", parsed.Contents, tt.want)
			}

			wantSources := make([]configSourceLocation, len(tt.wantSource))
			for i, source := range tt.wantSource {
				wantSources[i] = configSourceLocation{
					File: absConfigTestPath(t, filepath.Join(dir, source.file)),
					Line: source.line,
				}
			}
			assertConfigSources(t, parsed.Sources, wantSources)

			for _, include := range tt.wantIncludes {
				path := absConfigTestPath(t, filepath.Join(dir, include))
				if _, ok := parsed.Includes[path]; !ok {
					t.Errorf("included file %q was not tracked", path)
				}
			}
		})
	}
}

func TestParseYAMLIncludesWithSourcesPreservesBlankAndTrailingLines(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "glance.yml")
	includePath := filepath.Join(dir, "included.yml")
	writeConfigTestFile(t, mainPath, "before: true\n$include: included.yml\nafter: true\n")
	writeConfigTestFile(t, includePath, "first: true\n\nthird: true\n")

	parsed, err := parseYAMLIncludesWithSources(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	want := "before: true\nfirst: true\n\nthird: true\n\nafter: true\n"
	if string(parsed.Contents) != want {
		t.Fatalf("expanded contents mismatch\ngot:  %q\nwant: %q", parsed.Contents, want)
	}

	mainAbs := absConfigTestPath(t, mainPath)
	includeAbs := absConfigTestPath(t, includePath)
	assertConfigSources(t, parsed.Sources, []configSourceLocation{
		{File: mainAbs, Line: 1},
		{File: includeAbs, Line: 1},
		{File: includeAbs, Line: 2},
		{File: includeAbs, Line: 3},
		{File: includeAbs, Line: 4},
		{File: mainAbs, Line: 3},
		{File: mainAbs, Line: 4},
	})
}

func TestParsedYAMLConfigSourceLocation(t *testing.T) {
	parsed := &parsedYAMLConfig{
		Sources: []configSourceLocation{
			{File: "/config/glance.yml", Line: 1},
			{File: "/config/page.yml", Line: 7},
		},
	}

	location, ok := parsed.sourceLocation(2)
	if !ok || location != (configSourceLocation{File: "/config/page.yml", Line: 7}) {
		t.Fatalf("sourceLocation(2) = %+v, %v", location, ok)
	}

	for _, line := range []int{-1, 0, 3, 100} {
		if _, ok := parsed.sourceLocation(line); ok {
			t.Errorf("sourceLocation(%d) returned ok=true", line)
		}
	}

	var nilParsed *parsedYAMLConfig
	if _, ok := nilParsed.sourceLocation(1); ok {
		t.Error("sourceLocation() returned ok=true for nil parsed config")
	}
}

func TestParseYAMLIncludesMissingNestedIncludeReportsParent(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "glance.yml")
	pagePath := filepath.Join(dir, "page.yml")
	writeConfigTestFile(t, mainPath, "pages:\n  - $include: page.yml")
	writeConfigTestFile(t, pagePath, "name: Home\nwidgets:\n  - $include: missing.yml")

	_, err := parseYAMLIncludesWithSources(mainPath)
	if err == nil {
		t.Fatal("expected include error")
	}

	expected := []string{
		"resolving include " + absConfigTestPath(t, mainPath) + ":2",
		"resolving include " + absConfigTestPath(t, pagePath) + ":3",
		"reading " + absConfigTestPath(t, filepath.Join(dir, "missing.yml")),
	}
	for _, part := range expected {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q does not contain %q", err, part)
		}
	}
}

func TestParseYAMLIncludesCompatibilityWrapper(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "glance.yml")
	includePath := filepath.Join(dir, "page.yml")
	writeConfigTestFile(t, mainPath, "pages:\n  - $include: page.yml")
	writeConfigTestFile(t, includePath, "name: Home\nslug: home")

	contents, includes, err := parseYAMLIncludes(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "pages:\n  name: Home\n  slug: home"; string(contents) != want {
		t.Fatalf("contents = %q, want %q", contents, want)
	}
	if path := absConfigTestPath(t, includePath); func() bool { _, ok := includes[path]; return ok }() == false {
		t.Errorf("included file was not tracked")
	}
}

func TestParseYAMLLinePrefix(t *testing.T) {
	tests := []struct {
		message     string
		wantLine    int
		wantMessage string
		wantOK      bool
	}{
		{"yaml: line 17: could not find expected ':'", 17, "could not find expected ':'", true},
		{"line 8: cannot unmarshal value", 8, "cannot unmarshal value", true},
		{"unknown widget type: example", 0, "unknown widget type: example", false},
		{"yaml: line nope: invalid", 0, "yaml: line nope: invalid", false},
		{"yaml: line 0: invalid", 0, "yaml: line 0: invalid", false},
		{"yaml: line 4:", 0, "yaml: line 4:", false},
	}

	for _, tt := range tests {
		line, message, ok := parseYAMLLinePrefix(tt.message)
		if line != tt.wantLine || message != tt.wantMessage || ok != tt.wantOK {
			t.Errorf("parseYAMLLinePrefix(%q) = (%d, %q, %v), want (%d, %q, %v)",
				tt.message, line, message, ok, tt.wantLine, tt.wantMessage, tt.wantOK)
		}
	}
}

func TestNewConfigFromParsedYAMLReportsSource(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantFile string
		wantLine int
	}{
		{
			name: "root",
			files: map[string]string{
				"glance.yml": "pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets:\n          - type: clock\n    broken\n",
			},
			wantFile: "glance.yml",
			wantLine: 7,
		},
		{
			name: "direct include",
			files: map[string]string{
				"glance.yml": "server:\n  port: 8080\n\npages:\n  - $include: page.yml\n",
				"page.yml":   "name: Home\ncolumns:\n  - size: full\n    widgets:\n      - type: clock\nbroken\n",
			},
			wantFile: "page.yml",
			wantLine: 6,
		},
		{
			name: "nested include",
			files: map[string]string{
				"glance.yml": "server:\n  port: 8080\n\npages:\n  - $include: page.yml\n",
				"page.yml":   "name: Home\ncolumns:\n  - $include: column.yml\n",
				"column.yml": "size: full\nwidgets:\n  - type: clock\nbroken\n",
			},
			wantFile: "column.yml",
			wantLine: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range tt.files {
				writeConfigTestFile(t, filepath.Join(dir, name), contents)
			}

			parsed, err := parseYAMLIncludesWithSources(filepath.Join(dir, "glance.yml"))
			if err != nil {
				t.Fatal(err)
			}
			_, err = newConfigFromParsedYAML(parsed)
			if err == nil {
				t.Fatal("expected configuration error")
			}

			var diagnostic *configDiagnostic
			if !errors.As(err, &diagnostic) {
				t.Fatalf("error type = %T, want *configDiagnostic: %v", err, err)
			}
			if want := absConfigTestPath(t, filepath.Join(dir, tt.wantFile)); diagnostic.File != want {
				t.Errorf("diagnostic file = %q, want %q", diagnostic.File, want)
			}
			if diagnostic.Line != tt.wantLine {
				t.Errorf("diagnostic line = %d, want %d", diagnostic.Line, tt.wantLine)
			}
			if diagnostic.Message == "" {
				t.Error("diagnostic message is empty")
			}
		})
	}
}

func TestConfigDiagnosticFromSingleYAMLTypeError(t *testing.T) {
	parsed := &parsedYAMLConfig{
		Sources: []configSourceLocation{
			{File: "/config/glance.yml", Line: 1},
			{File: "/config/widget.yml", Line: 9},
		},
	}
	original := &yaml.TypeError{
		Errors: []string{"line 2: cannot unmarshal !!str `invalid` into uint16"},
	}

	err := configDiagnosticFromYAMLError(parsed, original)
	var diagnostic *configDiagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error type = %T, want *configDiagnostic", err)
	}
	if diagnostic.File != "/config/widget.yml" || diagnostic.Line != 9 {
		t.Errorf("diagnostic location = %s:%d", diagnostic.File, diagnostic.Line)
	}
	if diagnostic.Message != "cannot unmarshal !!str `invalid` into uint16" {
		t.Errorf("diagnostic message = %q", diagnostic.Message)
	}
	if !errors.Is(err, original) {
		t.Error("diagnostic does not unwrap to original yaml.TypeError")
	}
}

func TestConfigDiagnosticFallbacks(t *testing.T) {
	t.Run("unknown source line", func(t *testing.T) {
		parsed := &parsedYAMLConfig{
			Sources: []configSourceLocation{{File: "/config/glance.yml", Line: 1}},
		}
		original := errors.New("yaml: line 20: could not find expected ':'")
		if got := configDiagnosticFromYAMLError(parsed, original); got != original {
			t.Fatalf("error = %v, want original error", got)
		}
	})

	t.Run("multiple type errors", func(t *testing.T) {
		parsed := &parsedYAMLConfig{
			Sources: []configSourceLocation{
				{File: "/config/glance.yml", Line: 1},
				{File: "/config/glance.yml", Line: 2},
			},
		}
		original := &yaml.TypeError{
			Errors: []string{
				"line 1: cannot unmarshal one",
				"line 2: cannot unmarshal two",
			},
		}
		if got := configDiagnosticFromYAMLError(parsed, original); got != original {
			t.Fatalf("error = %v, want original multi-error", got)
		}
	})
}

func TestNewConfigFromParsedYAMLSemanticDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantFile    string
		wantLine    int
		wantMessage string
	}{
		{
			name: "root no pages",
			files: map[string]string{
				"glance.yml": "server:\n  port: 8080\n",
			},
			wantFile:    "glance.yml",
			wantLine:    1,
			wantMessage: "no pages configured",
		},
		{
			name: "direct included page full width requirement",
			files: map[string]string{
				"glance.yml": "pages:\n  $include: pages.yml\n",
				"pages.yml":  "- name: News\n  columns:\n    - size: small\n",
			},
			wantFile:    "pages.yml",
			wantLine:    2,
			wantMessage: `page "News" must have either 1 or 2 full width columns`,
		},
		{
			name: "nested included column size",
			files: map[string]string{
				"glance.yml":  "pages:\n  $include: pages.yml\n",
				"pages.yml":   "- name: News\n  columns:\n    $include: columns.yml\n",
				"columns.yml": "- size: enormous\n",
			},
			wantFile:    "columns.yml",
			wantLine:    1,
			wantMessage: `column 1 of page "News": size can only be either small or full`,
		},
		{
			name: "page width property",
			files: map[string]string{
				"glance.yml": "pages:\n  $include: pages.yml\n",
				"pages.yml":  "- name: News\n  width: enormous\n  columns:\n    - size: full\n",
			},
			wantFile:    "pages.yml",
			wantLine:    2,
			wantMessage: `page "News": width can only be either wide, slim or default`,
		},
		{
			name: "desktop navigation width property",
			files: map[string]string{
				"glance.yml": "pages:\n  $include: pages.yml\n",
				"pages.yml":  "- name: News\n  desktop-navigation-width: enormous\n  columns:\n    - size: full\n",
			},
			wantFile:    "pages.yml",
			wantLine:    2,
			wantMessage: `page "News": desktop-navigation-width can only be either wide, slim or default`,
		},
		{
			name: "dashboard",
			files: map[string]string{
				"glance.yml": "dashboards:\n  Secondary: [home]\npages:\n  - name: Home\n    slug: home\n    columns:\n      - size: full\n",
			},
			wantFile:    "glance.yml",
			wantLine:    1,
			wantMessage: "dashboards configuration requires a Default dashboard",
		},
		{
			name: "included user",
			files: map[string]string{
				"glance.yml": "auth:\n  secret-key: configured\n  users:\n    $include: users.yml\npages:\n  - name: Home\n    columns:\n      - size: full\n",
				"users.yml":  "ab:\n  password: example-password\n",
			},
			wantFile:    "users.yml",
			wantLine:    1,
			wantMessage: "usernames must be at least 3 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range tt.files {
				writeConfigTestFile(t, filepath.Join(dir, name), contents)
			}

			parsed, err := parseYAMLIncludesWithSources(filepath.Join(dir, "glance.yml"))
			if err != nil {
				t.Fatal(err)
			}

			_, err = newConfigFromParsedYAML(parsed)
			if err == nil {
				t.Fatal("expected semantic configuration error")
			}

			var diagnostic *configDiagnostic
			if !errors.As(err, &diagnostic) {
				t.Fatalf("error type = %T, want *configDiagnostic: %v", err, err)
			}

			wantFile := absConfigTestPath(t, filepath.Join(dir, tt.wantFile))
			if diagnostic.File != wantFile {
				t.Errorf("diagnostic file = %q, want %q", diagnostic.File, wantFile)
			}
			if diagnostic.Line != tt.wantLine {
				t.Errorf("diagnostic line = %d, want %d", diagnostic.Line, tt.wantLine)
			}
			if diagnostic.Message != tt.wantMessage {
				t.Errorf("diagnostic message = %q, want %q", diagnostic.Message, tt.wantMessage)
			}
			if diagnostic.cause == nil {
				t.Fatal("diagnostic cause is nil")
			}
			if !errors.Is(err, diagnostic.cause) {
				t.Error("diagnostic does not unwrap to its semantic cause")
			}
		})
	}
}

func TestNewConfigFromParsedYAMLSemanticServerAssetsPathDiagnostic(t *testing.T) {
	dir := t.TempDir()
	missingAssetsPath := filepath.Join(dir, "missing-assets")
	mainPath := filepath.Join(dir, "glance.yml")

	writeConfigTestFile(t, mainPath,
		"server:\n"+
			"  assets-path: "+missingAssetsPath+"\n"+
			"pages:\n"+
			"  - name: Home\n"+
			"    columns:\n"+
			"      - size: full\n",
	)

	parsed, err := parseYAMLIncludesWithSources(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = newConfigFromParsedYAML(parsed)
	if err == nil {
		t.Fatal("expected assets-path configuration error")
	}

	var diagnostic *configDiagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error type = %T, want *configDiagnostic: %v", err, err)
	}
	if diagnostic.File != absConfigTestPath(t, mainPath) {
		t.Errorf("diagnostic file = %q", diagnostic.File)
	}
	if diagnostic.Line != 2 {
		t.Errorf("diagnostic line = %d, want 2", diagnostic.Line)
	}
	if want := "assets directory does not exist: " + missingAssetsPath; diagnostic.Message != want {
		t.Errorf("diagnostic message = %q, want %q", diagnostic.Message, want)
	}
}

func TestIsConfigStateValidCompatibilityWithoutSources(t *testing.T) {
	cfg := &config{}
	err := isConfigStateValid(cfg)
	if err == nil {
		t.Fatal("expected configuration error")
	}

	var diagnostic *configDiagnostic
	if errors.As(err, &diagnostic) {
		t.Fatalf("compatibility validation unexpectedly returned configDiagnostic: %v", err)
	}
	if err.Error() != "no pages configured" {
		t.Errorf("error = %q, want %q", err.Error(), "no pages configured")
	}
}

func TestSemanticConfigDiagnosticFallbackWithoutSource(t *testing.T) {
	original := errors.New("semantic failure")

	tests := []struct {
		name   string
		parsed *parsedYAMLConfig
		line   int
	}{
		{name: "nil parsed", parsed: nil, line: 1},
		{
			name: "unknown generated line",
			parsed: &parsedYAMLConfig{
				Sources: []configSourceLocation{{File: "/config/glance.yml", Line: 1}},
			},
			line: 20,
		},
		{
			name: "zero generated line",
			parsed: &parsedYAMLConfig{
				Sources: []configSourceLocation{{File: "/config/glance.yml", Line: 1}},
			},
			line: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := semanticConfigDiagnostic(tt.parsed, tt.line, original)
			if got != original {
				t.Fatalf("error = %v, want original error", got)
			}
		})
	}
}

func TestNewConfigFromParsedYAMLWidgetInitializationDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantFile    string
		wantLine    int
		wantMessage string
	}{
		{
			name: "included head widget",
			files: map[string]string{
				"glance.yml":  "pages:\n  - name: Home\n    head-widgets:\n      $include: widgets.yml\n    columns:\n      - size: full\n",
				"widgets.yml": "- type: weather\n",
			},
			wantFile:    "widgets.yml",
			wantLine:    1,
			wantMessage: "weather widget: location is required",
		},
		{
			name: "included column widget",
			files: map[string]string{
				"glance.yml":  "pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets:\n          $include: widgets.yml\n",
				"widgets.yml": "- type: weather\n",
			},
			wantFile:    "widgets.yml",
			wantLine:    1,
			wantMessage: "weather widget: location is required",
		},
		{
			name: "nested container child",
			files: map[string]string{
				"glance.yml": "pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets:\n          - type: group\n            widgets:\n              - type: weather\n",
			},
			wantFile:    "glance.yml",
			wantLine:    8,
			wantMessage: "group widget: weather widget: location is required",
		},
		{
			name: "nested included container child",
			files: map[string]string{
				"glance.yml":   "pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets:\n          $include: group.yml\n",
				"group.yml":    "- type: group\n  widgets:\n    $include: children.yml\n",
				"children.yml": "- type: weather\n",
			},
			wantFile:    "children.yml",
			wantLine:    1,
			wantMessage: "group widget: weather widget: location is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range tt.files {
				writeConfigTestFile(t, filepath.Join(dir, name), contents)
			}

			parsed, err := parseYAMLIncludesWithSources(filepath.Join(dir, "glance.yml"))
			if err != nil {
				t.Fatal(err)
			}

			_, err = newConfigFromParsedYAML(parsed)
			if err == nil {
				t.Fatal("expected widget initialization error")
			}

			var diagnostic *configDiagnostic
			if !errors.As(err, &diagnostic) {
				t.Fatalf("error type = %T, want *configDiagnostic: %v", err, err)
			}

			wantFile := absConfigTestPath(t, filepath.Join(dir, tt.wantFile))
			if diagnostic.File != wantFile {
				t.Errorf("diagnostic file = %q, want %q", diagnostic.File, wantFile)
			}
			if diagnostic.Line != tt.wantLine {
				t.Errorf("diagnostic line = %d, want %d", diagnostic.Line, tt.wantLine)
			}
			if diagnostic.Message != tt.wantMessage {
				t.Errorf("diagnostic message = %q, want %q", diagnostic.Message, tt.wantMessage)
			}

			var initErr *widgetInitError
			if !errors.As(err, &initErr) {
				t.Fatalf("diagnostic does not unwrap to *widgetInitError: %v", err)
			}
			if initErr.widget == nil {
				t.Fatal("widget initialization error has no failing widget")
			}
			if initErr.widget.GetType() != "weather" {
				t.Errorf("failing widget type = %q, want %q", initErr.widget.GetType(), "weather")
			}
		})
	}
}

func TestNewConfigFromParsedYAMLCustomAPITemplateDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantFile    string
		wantLine    int
		wantMessage string
	}{
		{
			name: "inline multiline template",
			files: map[string]string{
				"glance.yml": "pages:\n" +
					"  - name: Home\n" +
					"    columns:\n" +
					"      - size: full\n" +
					"        widgets:\n" +
					"          - type: custom-api\n" +
					"            template: |\n" +
					"              first line\n" +
					"              second line\n" +
					"              {{ doesNotExist }}\n",
			},
			wantFile:    "glance.yml",
			wantLine:    10,
			wantMessage: `custom-api widget: parsing template: template: :3: function "doesNotExist" not defined`,
		},
		{
			name: "included template",
			files: map[string]string{
				"glance.yml": "pages:\n" +
					"  - name: Home\n" +
					"    columns:\n" +
					"      - size: full\n" +
					"        widgets:\n" +
					"          - type: custom-api\n" +
					"            template: |\n" +
					"              $include: template.yml\n",
				"template.yml": "first line\n" +
					"second line\n" +
					"{{ doesNotExist }}\n",
			},
			wantFile:    "template.yml",
			wantLine:    3,
			wantMessage: `custom-api widget: parsing template: template: :3: function "doesNotExist" not defined`,
		},
		{
			name: "nested included container template",
			files: map[string]string{
				"glance.yml": "pages:\n" +
					"  - name: Home\n" +
					"    columns:\n" +
					"      - size: full\n" +
					"        widgets:\n" +
					"          $include: group.yml\n",
				"group.yml": "- type: group\n" +
					"  widgets:\n" +
					"    - type: custom-api\n" +
					"      template: |\n" +
					"        $include: template.yml\n",
				"template.yml": "first line\n" +
					"{{ doesNotExist }}\n",
			},
			wantFile:    "template.yml",
			wantLine:    2,
			wantMessage: `group widget: custom-api widget: parsing template: template: :2: function "doesNotExist" not defined`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range tt.files {
				writeConfigTestFile(t, filepath.Join(dir, name), contents)
			}

			parsed, err := parseYAMLIncludesWithSources(filepath.Join(dir, "glance.yml"))
			if err != nil {
				t.Fatal(err)
			}

			_, err = newConfigFromParsedYAML(parsed)
			if err == nil {
				t.Fatal("expected custom API template initialization error")
			}

			var diagnostic *configDiagnostic
			if !errors.As(err, &diagnostic) {
				t.Fatalf("error type = %T, want *configDiagnostic: %v", err, err)
			}

			wantFile := absConfigTestPath(t, filepath.Join(dir, tt.wantFile))
			if diagnostic.File != wantFile {
				t.Errorf("diagnostic file = %q, want %q", diagnostic.File, wantFile)
			}
			if diagnostic.Line != tt.wantLine {
				t.Errorf("diagnostic line = %d, want %d", diagnostic.Line, tt.wantLine)
			}
			if diagnostic.Message != tt.wantMessage {
				t.Errorf("diagnostic message = %q, want %q", diagnostic.Message, tt.wantMessage)
			}

			var initErr *widgetInitError
			if !errors.As(err, &initErr) {
				t.Fatalf("diagnostic does not unwrap to *widgetInitError: %v", err)
			}
			if initErr.widget == nil {
				t.Fatal("widget initialization error has no failing widget")
			}
			if initErr.widget.GetType() != "custom-api" {
				t.Errorf(
					"failing widget type = %q, want %q",
					initErr.widget.GetType(),
					"custom-api",
				)
			}

			var templateErr *customAPITemplateParseError
			if !errors.As(err, &templateErr) {
				t.Fatalf("diagnostic does not unwrap to *customAPITemplateParseError: %v", err)
			}
			if templateErr.line < 1 {
				t.Errorf("template parse error line = %d, want positive line", templateErr.line)
			}
			if templateErr.Unwrap() == nil {
				t.Fatal("template parse error does not preserve underlying cause")
			}
		})
	}
}

func TestWidgetInitializationDiagnosticCustomAPITemplateFallback(t *testing.T) {
	w := &customAPIWidget{}
	w.Type = "custom-api"

	original := errors.New("template parser failure")
	templateErr := &customAPITemplateParseError{
		line:  0,
		cause: original,
	}
	formatted := formatWidgetInitError(templateErr, w)

	parsed := &parsedYAMLConfig{
		Sources: []configSourceLocation{
			{File: "/config/glance.yml", Line: 1},
			{File: "/config/widgets.yml", Line: 7},
		},
	}

	got := widgetInitializationDiagnostic(
		parsed,
		formatted,
		w,
		configWidgetSemanticSources{
			line:     2,
			template: 2,
		},
	)

	var diagnostic *configDiagnostic
	if !errors.As(got, &diagnostic) {
		t.Fatalf("error type = %T, want *configDiagnostic: %v", got, got)
	}
	if diagnostic.File != "/config/widgets.yml" || diagnostic.Line != 7 {
		t.Errorf(
			"diagnostic location = %s:%d, want %s:%d",
			diagnostic.File,
			diagnostic.Line,
			"/config/widgets.yml",
			7,
		)
	}
	if !errors.Is(got, original) {
		t.Error("template diagnostic does not unwrap to original cause")
	}
}

func TestWidgetInitializationDiagnosticFallbackWithoutSource(t *testing.T) {
	w := &weatherWidget{}
	w.Type = "weather"

	original := errors.New("location is required")
	formatted := formatWidgetInitError(original, w)

	got := widgetInitializationDiagnostic(
		nil,
		formatted,
		w,
		configWidgetSemanticSources{},
	)

	if got != formatted {
		t.Fatalf("error = %v, want original formatted widget error", got)
	}
	if got.Error() != "weather widget: location is required" {
		t.Errorf("error = %q, want %q", got.Error(), "weather widget: location is required")
	}
	if !errors.Is(got, original) {
		t.Error("formatted widget error does not unwrap to original cause")
	}
}

func TestNewConfigFromParsedYAMLUnknownWidgetTypeDiagnostic(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "glance.yml")
	widgetPath := filepath.Join(dir, "widgets.yml")

	writeConfigTestFile(t, mainPath,
		"pages:\n"+
			"  - name: Home\n"+
			"    columns:\n"+
			"      - size: full\n"+
			"        widgets:\n"+
			"          $include: widgets.yml\n",
	)
	writeConfigTestFile(t, widgetPath, "- type: does-not-exist\n")

	parsed, err := parseYAMLIncludesWithSources(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = newConfigFromParsedYAML(parsed)
	if err == nil {
		t.Fatal("expected unknown widget type error")
	}

	var diagnostic *configDiagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error type = %T, want *configDiagnostic: %v", err, err)
	}
	if want := absConfigTestPath(t, widgetPath); diagnostic.File != want {
		t.Errorf("diagnostic file = %q, want %q", diagnostic.File, want)
	}
	if diagnostic.Line != 1 {
		t.Errorf("diagnostic line = %d, want 1", diagnostic.Line)
	}
	if diagnostic.Message != "unknown widget type: does-not-exist" {
		t.Errorf(
			"diagnostic message = %q, want %q",
			diagnostic.Message,
			"unknown widget type: does-not-exist",
		)
	}
}
