package ui

import (
	"testing"

	"github.com/ruben/gsql/internal/db"
)

func TestExportPicker_ShowAllMarked(t *testing.T) {
	p := NewExportPicker()
	p.Show([]string{"users", "orders", "items"})

	if !p.IsVisible() {
		t.Fatal("picker should be visible after Show")
	}
	if p.MarkedCount() != 3 {
		t.Fatalf("expected 3 marked, got %d", p.MarkedCount())
	}
	tables := p.SelectedTables()
	if len(tables) != 3 || tables[0] != "users" || tables[2] != "items" {
		t.Fatalf("unexpected tables: %v", tables)
	}
}

func TestExportPicker_Toggle(t *testing.T) {
	p := NewExportPicker()
	p.Show([]string{"a", "b", "c"})

	// Cursor starts at 0 — toggle it off.
	p.ToggleSelected()
	if p.MarkedCount() != 2 {
		t.Fatalf("expected 2 marked after toggle, got %d", p.MarkedCount())
	}

	// Toggle back on.
	p.ToggleSelected()
	if p.MarkedCount() != 3 {
		t.Fatalf("expected 3 marked after re-toggle, got %d", p.MarkedCount())
	}
}

func TestExportPicker_SelectAllNone(t *testing.T) {
	p := NewExportPicker()
	p.Show([]string{"a", "b"})

	p.SelectNone()
	if p.MarkedCount() != 0 {
		t.Fatalf("expected 0 after none, got %d", p.MarkedCount())
	}
	if len(p.SelectedTables()) != 0 {
		t.Fatalf("expected empty selection, got %v", p.SelectedTables())
	}

	p.SelectAll()
	if p.MarkedCount() != 2 {
		t.Fatalf("expected 2 after all, got %d", p.MarkedCount())
	}
}

func TestExportPicker_Hide(t *testing.T) {
	p := NewExportPicker()
	p.Show([]string{"a"})
	p.Hide()

	if p.IsVisible() {
		t.Fatal("picker should be hidden after Hide")
	}
	if p.MarkedCount() != 0 {
		t.Fatal("picker should have no items after Hide")
	}
}

func TestExportPicker_Format(t *testing.T) {
	p := NewExportPicker()
	if p.CurrentFormat() != db.FormatSQL {
		t.Fatalf("expected default format SQL, got %s", p.CurrentFormat())
	}
}

func TestExportPicker_CursorNavigation(t *testing.T) {
	p := NewExportPicker()
	p.Show([]string{"a", "b", "c"})

	p.CursorDown()
	if p.cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", p.cursor)
	}
	p.CursorDown()
	p.CursorDown() // past the end — should clamp
	if p.cursor != 2 {
		t.Fatalf("expected cursor clamped at 2, got %d", p.cursor)
	}
	p.CursorUp()
	if p.cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", p.cursor)
	}
	p.CursorUp()
	p.CursorUp() // past the start — should clamp
	if p.cursor != 0 {
		t.Fatalf("expected cursor clamped at 0, got %d", p.cursor)
	}
}
