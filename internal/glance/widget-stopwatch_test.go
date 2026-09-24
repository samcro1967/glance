package glance

import (
	"strings"
	"testing"
)

func TestStopwatchWidgetInitializeAndRender(t *testing.T) {
	widget := &stopwatchWidget{}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize stopwatch: %v", err)
	}

	if widget.Title != "Stopwatch" {
		t.Fatalf("title=%q, want %q", widget.Title, "Stopwatch")
	}

	rendered := string(widget.Render())

	for _, expected := range []string{
		`class="stopwatch"`,
		`data-stopwatch-start-on-open="false"`,
		`data-stopwatch-display`,
		`data-stopwatch-toggle`,
		`data-stopwatch-reset`,
		`data-stopwatch-lap`,
		`data-stopwatch-laps`,
		`aria-label="Start stopwatch"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget missing %q", expected)
		}
	}
}

func TestStopwatchWidgetRendersStartOnOpen(t *testing.T) {
	widget := &stopwatchWidget{StartOnOpen: true}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize stopwatch: %v", err)
	}

	if rendered := string(widget.Render()); !strings.Contains(rendered, `data-stopwatch-start-on-open="true"`) {
		t.Fatal("rendered widget does not enable start-on-open")
	}
}
