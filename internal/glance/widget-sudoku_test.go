package glance

import (
	"strings"
	"testing"
)

func TestSudokuWidgetDefaultsAndRender(t *testing.T) {
	widget := &sudokuWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize sudoku: %v", err)
	}
	if widget.Title != "Sudoku" {
		t.Fatalf("title=%q, want Sudoku", widget.Title)
	}
	if widget.Difficulty != "easy" {
		t.Fatalf("difficulty=%q, want easy", widget.Difficulty)
	}
	rendered := string(widget.Render())
	for _, expected := range []string{
		`class="sudoku"`,
		`data-difficulty="easy"`,
		`data-sudoku-board`,
		`data-sudoku-restart`,
		`data-sudoku-time`,
		`data-sudoku-number="1"`,
		`value="easy" selected`,
		`value="medium"`,
		`value="hard"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered widget missing %q", expected)
		}
	}
}

func TestSudokuWidgetAcceptsDifficulties(t *testing.T) {
	for _, difficulty := range []string{"easy", "medium", "hard"} {
		widget := &sudokuWidget{Difficulty: difficulty}
		if err := widget.initialize(); err != nil {
			t.Fatalf("difficulty %q: %v", difficulty, err)
		}
	}
}

func TestSudokuWidgetRejectsInvalidDifficulty(t *testing.T) {
	widget := &sudokuWidget{Difficulty: "impossible"}
	if err := widget.initialize(); err == nil {
		t.Fatal("expected invalid difficulty error")
	}
}

func TestSudokuWidgetRenderExpanded(t *testing.T) {
	widget := &sudokuWidget{Difficulty: "hard"}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize sudoku: %v", err)
	}

	rendered := string(widget.RenderExpanded())
	for _, expected := range []string{
		`class="sudoku sudoku-expanded"`,
		`data-difficulty="hard"`,
		`data-sudoku-board`,
		`value="hard" selected`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expanded widget missing %q", expected)
		}
	}
}
