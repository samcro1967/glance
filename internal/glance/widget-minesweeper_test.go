package glance

import (
	"strings"
	"testing"
)

func TestMinesweeperWidgetDefaultsAndRender(t *testing.T) {
	widget := &minesweeperWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize minesweeper: %v", err)
	}
	if widget.Title != "Minesweeper" {
		t.Fatalf("title=%q, want Minesweeper", widget.Title)
	}
	if widget.Difficulty != "beginner" {
		t.Fatalf("difficulty=%q, want beginner", widget.Difficulty)
	}
	rendered := string(widget.Render())
	for _, expected := range []string{
		`class="minesweeper"`,
		`data-difficulty="beginner"`,
		`data-minesweeper-board`,
		`data-minesweeper-restart`,
		`data-minesweeper-mines`,
		`data-minesweeper-time`,
		`value="beginner" selected`,
		`value="intermediate"`,
		`value="expert"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget missing %q", expected)
		}
	}
}

func TestMinesweeperWidgetAcceptsDifficulties(t *testing.T) {
	for _, difficulty := range []string{"beginner", "intermediate", "expert"} {
		widget := &minesweeperWidget{Difficulty: difficulty}
		if err := widget.initialize(); err != nil {
			t.Fatalf("difficulty %q: %v", difficulty, err)
		}
	}
}

func TestMinesweeperWidgetRejectsInvalidDifficulty(t *testing.T) {
	widget := &minesweeperWidget{Difficulty: "impossible"}
	if err := widget.initialize(); err == nil {
		t.Fatal("expected invalid difficulty error")
	}
}

func TestMinesweeperWidgetRenderExpanded(t *testing.T) {
	widget := &minesweeperWidget{Difficulty: "expert"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize minesweeper: %v", err)
	}

	rendered := string(widget.RenderExpanded())
	for _, expected := range []string{
		`class="minesweeper minesweeper-expanded"`,
		`data-difficulty="expert"`,
		`data-minesweeper-board`,
		`value="expert" selected`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expanded widget missing %q", expected)
		}
	}
}
