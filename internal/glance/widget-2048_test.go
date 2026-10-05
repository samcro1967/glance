package glance

import (
	"strings"
	"testing"
)

func Test2048WidgetDefaultsAndRender(t *testing.T) {
	widget := &game2048Widget{}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize 2048: %v", err)
	}
	if widget.Title != "2048" {
		t.Fatalf("title=%q, want 2048", widget.Title)
	}
	rendered := string(widget.Render())
	for _, expected := range []string{`class="game-2048"`, `data-2048-board`, `data-2048-score`, `data-2048-restart`, `data-2048-continue`, `data-2048-try-again`} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget missing %q", expected)
		}
	}
}

func Test2048WidgetRenderExpanded(t *testing.T) {
	widget := &game2048Widget{}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize 2048: %v", err)
	}
	rendered := string(widget.RenderExpanded())
	for _, expected := range []string{`class="game-2048 game-2048-expanded"`, `data-2048-board`, `data-2048-score`} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expanded widget missing %q", expected)
		}
	}
}
