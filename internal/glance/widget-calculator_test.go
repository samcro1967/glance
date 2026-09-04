package glance

import (
	"strings"
	"testing"
)

func TestCalculatorWidgetInitializeAndRender(t *testing.T) {
	widget := &calculatorWidget{}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize calculator: %v", err)
	}

	if widget.Title != "Calculator" {
		t.Fatalf("title=%q, want %q", widget.Title, "Calculator")
	}

	rendered := string(widget.Render())

	for _, expected := range []string{
		`class="calculator"`,
		`data-calculator-expression`,
		`data-calculator-result`,
		`data-calculator-action="percent"`,
		`data-calculator-action="clear-entry"`,
		`data-calculator-action="clear"`,
		`data-calculator-action="backspace"`,
		`data-calculator-action="reciprocal"`,
		`data-calculator-action="square"`,
		`data-calculator-action="square-root"`,
		`data-calculator-action="sign"`,
		`data-calculator-action="decimal"`,
		`data-calculator-action="equals"`,
		`data-calculator-operator="+"`,
		`data-calculator-operator="-"`,
		`data-calculator-operator="*"`,
		`data-calculator-operator="/"`,
		`data-calculator-operator="^"`,
		`data-calculator-operator="root"`,
		`data-calculator-paren="("`,
		`data-calculator-paren=")"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget missing %q", expected)
		}
	}
}

func TestCalculatorWidgetRendersAllDigits(t *testing.T) {
	widget := &calculatorWidget{}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize calculator: %v", err)
	}

	rendered := string(widget.Render())

	for digit := '0'; digit <= '9'; digit++ {
		expected := `data-calculator-digit="` + string(digit) + `"`
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget missing digit %q", string(digit))
		}
	}
}

func TestCalculatorWidgetRendersExpectedButtonCount(t *testing.T) {
	widget := &calculatorWidget{}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize calculator: %v", err)
	}

	rendered := string(widget.Render())

	if got := strings.Count(rendered, "<button "); got != 28 {
		t.Fatalf("button count=%d, want 28", got)
	}
}
