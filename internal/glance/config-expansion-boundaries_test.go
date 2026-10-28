package glance

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestMultilineExpansionPreservesOriginalPropertyLine(t *testing.T) {
	t.Setenv("TEST_MULTILINE_VALUE", "first\n    second")
	path := filepath.Join(t.TempDir(), "glance.yml")
	source := `branding:
  custom-footer: |
    ${TEST_MULTILINE_VALUE}
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: clock
            ignored-probe: true
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseYAMLIncludesWithSources(path)
	if err != nil {
		t.Fatal(err)
	}
	originalLines := len(parsed.Sources)
	c, err := newConfigFromParsedYAML(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.propertyDiagnostics) != 1 || c.propertyDiagnostics[0].Line != 10 || c.propertyDiagnostics[0].File != path {
		t.Fatalf("wrong source: %+v", c.propertyDiagnostics)
	}
	if len(parsed.Sources) != originalLines {
		t.Fatal("compilation mutated watcher source mapping")
	}
}

func TestDurationOverflowBoundaries(t *testing.T) {
	for unit, multiplier := range map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour} {
		maximum := int64(1<<63-1) / int64(multiplier)
		for _, overflow := range []bool{false, true} {
			value := maximum
			if overflow {
				value++
			}
			var d durationField
			err := yaml.Unmarshal([]byte(fmt.Sprintf("%d%s", value, unit)), &d)
			if (err != nil) != overflow {
				t.Fatalf("%d%s error=%v", value, unit, err)
			}
		}
	}
}
