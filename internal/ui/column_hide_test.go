package ui

import (
	"testing"
)

func newHiddenTestTable() ResultsTable {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name", "email", "age", "city"},
		[][]string{
			{"1", "alice", "alice@test.com", "30", "NYC"},
			{"2", "bob", "bob@test.com", "25", "LA"},
		},
		"2 rows",
	)
	return r
}

func TestHideColumnRemovesFromVisibleRange(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})

	// Initially all 5 columns are visible.
	if got := len(r.visibleColRange()); got != 5 {
		t.Fatalf("expected 5 visible cols, got %d", got)
	}

	if !r.HideColumn(2) { // hide "email"
		t.Fatal("HideColumn(2) returned false")
	}

	vis := r.visibleColRange()
	if len(vis) != 4 {
		t.Fatalf("expected 4 visible cols after hide, got %d", len(vis))
	}
	for _, c := range vis {
		if c == 2 {
			t.Error("hidden column index 2 still in visible range")
		}
	}
	if !r.IsColumnHidden(2) {
		t.Error("IsColumnHidden(2) should be true")
	}
	if r.HiddenCount() != 1 {
		t.Errorf("HiddenCount = %d, want 1", r.HiddenCount())
	}
}

func TestHideColumnRefusesLastVisible(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})

	// Hide 4 of 5 columns.
	r.HideColumn(1)
	r.HideColumn(2)
	r.HideColumn(3)
	r.HideColumn(4)

	if r.VisibleColumnCount() != 1 {
		t.Fatalf("expected 1 visible col, got %d", r.VisibleColumnCount())
	}
	// Hiding the last one must be a no-op.
	if r.HideColumn(0) {
		t.Error("HideColumn on last visible column should return false")
	}
	if r.VisibleColumnCount() != 1 {
		t.Errorf("expected still 1 visible col, got %d", r.VisibleColumnCount())
	}
}

func TestCursorSkipsHiddenColumns(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})
	r.SetCursor(0, 0)

	// Hide "name" (index 1). Cursor is at 0; moving right must skip to 2.
	r.HideColumn(1)
	r.CursorRight()
	if r.CursorCol() != 2 {
		t.Errorf("after hide col 1, CursorRight: expected col 2, got %d", r.CursorCol())
	}
	// Moving left from 2 should skip back to 0.
	r.CursorLeft()
	if r.CursorCol() != 0 {
		t.Errorf("CursorLeft: expected col 0, got %d", r.CursorCol())
	}
}

func TestCursorMovedOffNewlyHiddenColumn(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})
	r.SetCursor(0, 2) // cursor on "email"

	r.HideColumn(2) // hide the column the cursor is on
	// clampCursor should have moved the cursor to a visible column.
	if r.IsColumnHidden(r.CursorCol()) {
		t.Errorf("cursor landed on hidden column %d", r.CursorCol())
	}
}

func TestShowAllColumnsRestores(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})
	r.HideColumn(1)
	r.HideColumn(3)

	if r.HiddenCount() != 2 {
		t.Fatalf("expected 2 hidden, got %d", r.HiddenCount())
	}
	r.ShowAllColumns()
	if r.HiddenCount() != 0 {
		t.Errorf("after ShowAllColumns, HiddenCount = %d, want 0", r.HiddenCount())
	}
	if len(r.visibleColRange()) != 5 {
		t.Errorf("expected all 5 visible again, got %d", len(r.visibleColRange()))
	}
}

func TestSetHiddenColumnsBatch(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})

	r.SetHiddenColumns([]string{"name", "age", "nonexistent"})
	if r.HiddenCount() != 2 {
		t.Errorf("expected 2 hidden (nonexistent ignored), got %d", r.HiddenCount())
	}
	if !r.IsColumnHiddenByName("name") {
		t.Error("name should be hidden")
	}
}

func TestSetHiddenColumnsRejectsHidingAll(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})

	// Try to hide every column — must be rejected.
	r.SetHiddenColumns([]string{"id", "name", "email", "age", "city"})
	if r.HiddenCount() != 0 {
		t.Errorf("hiding all columns must be rejected; HiddenCount = %d", r.HiddenCount())
	}
}

func TestHiddenColumnsSurviveSameTableRequery(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})
	r.HideColumn(1)

	// Re-query the same table: same sourceTable, hidden set must persist.
	r.SetResult(
		[]string{"id", "name", "email", "age", "city"},
		[][]string{{"1", "alice", "x", "30", "NYC"}},
		"1 row",
	)
	if r.HiddenCount() != 1 {
		t.Errorf("hidden cols should survive same-table requery; got %d", r.HiddenCount())
	}
}

// IsColumnHiddenByName is a small test helper.
func (r ResultsTable) IsColumnHiddenByName(name string) bool {
	for i, c := range r.columns {
		if c == name {
			return r.IsColumnHidden(i)
		}
	}
	return false
}

func TestColumnPickerToggleAndApply(t *testing.T) {
	r := newHiddenTestTable()
	r.SetEditable("users", []string{"id"})

	var p ColumnPicker
	p.Show([]string{"id", "name", "email", "age", "city"}, nil)
	if p.VisibleCount() != 5 {
		t.Fatalf("expected 5 visible initially, got %d", p.VisibleCount())
	}

	// Toggle the cursor item (id) off. Cursor starts at 0 after Show.
	p.ToggleSelected()
	if p.VisibleCount() != 4 {
		t.Errorf("after toggle, expected 4 visible, got %d", p.VisibleCount())
	}
	hidden := p.HiddenColumns()
	if len(hidden) != 1 || hidden[0] != "id" {
		t.Errorf("unexpected hidden set: %v", hidden)
	}

	// Applying to the table should hide exactly that column.
	r.SetHiddenColumns(hidden)
	if !r.IsColumnHiddenByName("id") {
		t.Error("id should be hidden after apply")
	}
}

func TestColumnPickerRefusesLastVisible(t *testing.T) {
	var p ColumnPicker
	p.Show([]string{"a", "b", "c"}, nil)
	// Cursor stays on the toggled item, so navigate then toggle.
	p.CursorDown() // cursor -> 1 (b)
	p.ToggleSelected()
	p.CursorDown() // cursor -> 2 (c)
	p.ToggleSelected()
	if p.VisibleCount() != 1 {
		t.Fatalf("expected 1 visible, got %d", p.VisibleCount())
	}
	// Only 'a' is visible; navigate to it and toggle (must be refused).
	p.CursorUp() // cursor -> 1 (b)
	p.CursorUp() // cursor -> 0 (a)
	p.ToggleSelected()
	if p.VisibleCount() != 1 {
		t.Errorf("refused toggle should keep 1 visible, got %d", p.VisibleCount())
	}
}

func TestColumnPickerSelectAllAndNone(t *testing.T) {
	var p ColumnPicker
	p.Show([]string{"a", "b"}, map[string]bool{"a": true})
	if p.VisibleCount() != 1 {
		t.Fatalf("expected 1 visible initially, got %d", p.VisibleCount())
	}
	p.SelectAll()
	if p.VisibleCount() != 2 {
		t.Errorf("SelectAll: expected 2, got %d", p.VisibleCount())
	}
	p.SelectNone()
	if p.VisibleCount() != 1 {
		t.Errorf("SelectNone must keep >=1 visible, got %d", p.VisibleCount())
	}
}
