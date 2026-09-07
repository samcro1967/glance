package glance

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestCustomAPISproutTemplateFuncsMatchAllowlist(t *testing.T) {
	funcs := customAPISproutTemplateFuncs()

	if len(funcs) != len(customAPISproutAllowedFunctions) {
		t.Fatalf("got %d Sprout functions, want %d", len(funcs), len(customAPISproutAllowedFunctions))
	}

	for rawName := range customAPISproutAllowedFunctions {
		name := "sprout" + strings.ToUpper(rawName[:1]) + rawName[1:]
		if _, exists := funcs[name]; !exists {
			t.Errorf("allowlisted Sprout function %q is not exposed as %q", rawName, name)
		}
	}

	for name := range funcs {
		if !strings.HasPrefix(name, "sprout") {
			t.Errorf("Sprout function %q does not use the sprout prefix", name)
		}
	}
}

func TestCustomAPISproutTemplateFuncsExcludeUnapprovedFunctions(t *testing.T) {
	funcs := customAPISproutTemplateFuncs()

	unapproved := []string{
		"sproutHello",
		"sproutShuffle",
		"sproutAddf",
		"sproutAdd1f",
		"sproutSubf",
		"sproutMustAppend",
		"sproutMustMerge",
		"sproutMustToJson",
		"sproutFromJson",
		"sproutFromYaml",
		"sproutToJson",
		"sproutToYaml",
		"sproutExpandEnv",
		"sproutReadFile",
		"sproutRandInt",
		"sproutUUIDv4",
		"sproutSHA256Sum",
	}

	for _, name := range unapproved {
		if _, exists := funcs[name]; exists {
			t.Errorf("unapproved Sprout function %q is exposed", name)
		}
	}
}

func TestCustomAPISproutTemplateFuncsExecute(t *testing.T) {
	const source = `{{ sproutToUpper "glance" }}|{{ sproutAdd 2 3 }}|{{ sproutDefault "fallback" "" }}|{{ sproutRegexMatch "^g.*e$" "glance" }}`

	tmpl, err := template.New("sprout").Funcs(customAPISproutTemplateFuncs()).Parse(source)
	if err != nil {
		t.Fatalf("parse Sprout template: %v", err)
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, nil); err != nil {
		t.Fatalf("execute Sprout template: %v", err)
	}

	if got, want := output.String(), "GLANCE|5|fallback|true"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCustomAPISproutTemplateFuncsPropagateErrors(t *testing.T) {
	const source = `{{ sproutDiv 10 0 }}`

	tmpl, err := template.New("sprout").Funcs(customAPISproutTemplateFuncs()).Parse(source)
	if err != nil {
		t.Fatalf("parse Sprout template: %v", err)
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, nil); err == nil {
		t.Fatal("expected Sprout division by zero to return a template execution error")
	}
}

func TestCustomAPITemplateFuncsIncludeSproutWithoutChangingLegacyHelpers(t *testing.T) {
	for _, name := range []string{
		"sproutRound",
		"sproutList",
		"sproutDict",
		"sproutRegexMatch",
		"sproutSemverCompare",
	} {
		if _, exists := customAPITemplateFuncs[name]; !exists {
			t.Errorf("Custom API function map does not expose %q", name)
		}
	}

	legacyDiv := customAPITemplateFuncs["div"].(func(any, any) any)
	if got := legacyDiv(10, 0); got != 0 {
		t.Fatalf("legacy div(10, 0) = %v, want 0", got)
	}

	legacyConcat := customAPITemplateFuncs["concat"].(func(...string) string)
	if got := legacyConcat("glance", "-", "custom-api"); got != "glance-custom-api" {
		t.Fatalf("legacy concat result = %q, want %q", got, "glance-custom-api")
	}
}

func TestCustomAPITemplateFuncsExecuteLegacyAndSproutHelpers(t *testing.T) {
	const source = `{{ add 2 3 }}|{{ concat "glance" "-" "api" }}|{{ sproutAdd 2 3 }}|{{ sproutList "a" "b" | sproutLast }}`

	tmpl, err := template.New("custom-api").Funcs(customAPITemplateFuncs).Parse(source)
	if err != nil {
		t.Fatalf("parse Custom API template: %v", err)
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, nil); err != nil {
		t.Fatalf("execute Custom API template: %v", err)
	}

	if got, want := output.String(), "5|glance-api|5|b"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCustomAPITemplateFuncsKeepSproutErrorsSeparateFromLegacyBehavior(t *testing.T) {
	legacyTemplate, err := template.New("legacy").Funcs(customAPITemplateFuncs).Parse(`{{ div 10 0 }}`)
	if err != nil {
		t.Fatalf("parse legacy division template: %v", err)
	}

	var legacyOutput bytes.Buffer
	if err := legacyTemplate.Execute(&legacyOutput, nil); err != nil {
		t.Fatalf("execute legacy division template: %v", err)
	}
	if got, want := legacyOutput.String(), "0"; got != want {
		t.Fatalf("legacy division result = %q, want %q", got, want)
	}

	sproutTemplate, err := template.New("sprout").Funcs(customAPITemplateFuncs).Parse(`{{ sproutDiv 10 0 }}`)
	if err != nil {
		t.Fatalf("parse Sprout division template: %v", err)
	}

	var sproutOutput bytes.Buffer
	if err := sproutTemplate.Execute(&sproutOutput, nil); err == nil {
		t.Fatal("expected sproutDiv division by zero to return a template execution error")
	}
}

func TestCustomAPISproutSliceHelpersWorkWithJSONArrays(t *testing.T) {
	data := &customAPITemplateData{
		customAPIResponseData: &customAPIResponseData{
			JSON: decoratedGJSONResult{gjson.Parse(`{
				"items": [
					{"name": "alpha", "score": 10},
					{"name": "bravo", "score": 20},
					{"name": "charlie", "score": 30}
				]
			}`)},
		},
	}

	const source = `{{ (.JSON.Array "items" | sproutFirst).String "name" }}|{{ (.JSON.Array "items" | sproutLast).String "name" }}|{{ range (.JSON.Array "items" | sproutSlice 1 3) }}{{ .String "name" }},{{ end }}|{{ range (.JSON.Array "items" | sproutReverse) }}{{ .String "name" }},{{ end }}`

	tmpl, err := template.New("json-slices").Funcs(customAPITemplateFuncs).Parse(source)
	if err != nil {
		t.Fatalf("parse JSON slice template: %v", err)
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		t.Fatalf("execute JSON slice template: %v", err)
	}

	if got, want := output.String(), "alpha|charlie|bravo,charlie,|charlie,bravo,alpha,"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
