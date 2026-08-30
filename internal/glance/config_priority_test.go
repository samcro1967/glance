package glance

import (
	"errors"
	"testing"
)

func TestPriorityConfigDiagnosticNilSafety(t *testing.T) {
	var diagnostic *configDiagnostic
	if got := diagnostic.Error(); got != "" {
		t.Fatalf("nil Error = %q, want empty string", got)
	}
	if diagnostic.Unwrap() != nil {
		t.Fatal("nil Unwrap should be nil")
	}
}

func TestPriorityWidgetInitErrorNilSafety(t *testing.T) {
	var initErr *widgetInitError
	if got := initErr.Error(); got != "" {
		t.Fatalf("nil Error = %q, want empty string", got)
	}
	if initErr.Unwrap() != nil {
		t.Fatal("nil Unwrap should be nil")
	}
}

func TestPriorityOrderedYAMLMapRejectsMismatchedLengths(t *testing.T) {
	_, err := newOrderedYAMLMap([]string{"one", "two"}, []int{1})
	if err == nil {
		t.Fatal("expected mismatched length error")
	}
}

func TestPriorityParseConfigVariableUnknownTypeIsIgnored(t *testing.T) {
	value, skip, err := parseConfigVariableOfType("definitely-unknown", "NAME")
	if err != nil {
		t.Fatalf("unexpected error = %v", err)
	}
	if !skip || value != "" {
		t.Fatalf("value=%q skip=%v, want empty value and skip=true", value, skip)
	}
}

func TestPriorityConfigDiagnosticPreservesCause(t *testing.T) {
	cause := errors.New("priority cause")
	diagnostic := &configDiagnostic{Message: "priority message", cause: cause}
	if !errors.Is(diagnostic, cause) {
		t.Fatalf("diagnostic does not unwrap cause: %v", diagnostic)
	}
	if diagnostic.Error() != "priority message" {
		t.Fatalf("Error = %q", diagnostic.Error())
	}
}
