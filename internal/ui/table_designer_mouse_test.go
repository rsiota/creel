package ui

import (
	"testing"

	"github.com/ruben/gsql/internal/db"
)

// showMouseDesigner builds a designer sized for the tests below.
func showMouseDesigner(t *testing.T) *TableDesigner {
	t.Helper()
	d := NewTableDesigner()
	d.SetSize(80, 24)
	d.Show(db.DriverMySQL, nil)
	return &d
}

// Clicking a data cell moves the (row, col) cursor and leaves grid focus.
func TestTableDesignerClickMovesCursor(t *testing.T) {
	d := showMouseDesigner(t)
	// Starts on the name field; a grid click must hand focus to the grid.
	if !d.focusName {
		t.Fatal("designer should start focused on the name field")
	}
	top := d.gridTopRow()

	// Grid: name(0), blank(1), top border(2), header(3), separator(4), data at 5.
	row := top + 1
	d.Click(1, 5+1) // second visible row, Name column
	if d.focusName {
		t.Error("grid click should blur the name field")
	}
	if d.cursorRow != row {
		t.Errorf("cursorRow=%d, want %d", d.cursorRow, row)
	}
	if d.cursorCol != 0 {
		t.Errorf("cursorCol=%d, want 0 (Name)", d.cursorCol)
	}

	// Type column sits after Name (colWidths[0]+2 + leading + separator).
	typeX := 1 + d.colWidths[0] + 2 + 1
	d.Click(typeX, 5+1)
	if d.cursorCol != 1 {
		t.Errorf("cursorCol=%d, want 1 (Type)", d.cursorCol)
	}
}

// A double-click on a cell starts an inline edit; a single click does not.
func TestTableDesignerDoubleClickEdits(t *testing.T) {
	d := showMouseDesigner(t)
	d.Click(1, 5+0) // first data row, Name cell
	if d.editing {
		t.Fatal("single click should not start editing")
	}
	d.Click(1, 5+0)
	if !d.editing {
		t.Fatal("double-click should start inline edit")
	}
}

// Two clicks on different cells do not count as a double-click.
func TestTableDesignerDoubleClickDifferentCells(t *testing.T) {
	d := showMouseDesigner(t)
	d.Click(1, 5+0)
	d.Click(1, 5+1) // different row
	if d.editing {
		t.Fatal("clicks on different cells must not start editing")
	}
}

// Clicking the table-name line focuses the name field (even from the grid).
func TestTableDesignerClickFocusesName(t *testing.T) {
	d := showMouseDesigner(t)
	// Move focus to the grid first.
	d.Click(1, 5+0)
	if d.focusName {
		t.Fatal("expected grid focus after grid click")
	}
	// Click the name line (y==0) → name field regains focus.
	d.Click(1, 0)
	if !d.focusName {
		t.Error("name-line click should focus the name field")
	}
}

// Clicking another cell while an inline edit is open cancels the edit and
// relocates the cursor (the edit input must not linger on the wrong cell).
func TestTableDesignerClickCancelsInProgressEdit(t *testing.T) {
	d := showMouseDesigner(t)
	// Enter edit mode on the first row, Name cell.
	d.Click(1, 5+0)
	d.Click(1, 5+0)
	if !d.editing {
		t.Fatal("expected edit mode after double-click")
	}
	// Click a different cell (second row).
	d.Click(1, 5+1)
	if d.editing {
		t.Error("clicking another cell should cancel the in-progress edit")
	}
	if d.cursorRow != d.gridTopRow()+1 {
		t.Errorf("cursorRow=%d, want %d", d.cursorRow, d.gridTopRow()+1)
	}
}

// Wheel scrolls by moving the cursor (viewport is cursor-centered).
func TestTableDesignerWheel(t *testing.T) {
	d := showMouseDesigner(t)
	// Drop into the grid so wheel targets it (it blurs the name field).
	d.Click(1, 5+0)
	start := d.cursorRow
	d.Wheel(1)
	if d.cursorRow != start+1 {
		t.Errorf("wheel down: cursorRow=%d, want %d", d.cursorRow, start+1)
	}
	d.Wheel(-1)
	if d.cursorRow != start {
		t.Errorf("wheel up: cursorRow=%d, want %d", d.cursorRow, start)
	}
}

// gridColumnAtX resolves each column; clicks past the grid resolve to -1.
func TestTableDesignerGridColumnAtX(t *testing.T) {
	d := showMouseDesigner(t)
	// Leading border at X=0; Name content starts at X=1.
	if got := d.gridColumnAtX(1); got != 0 {
		t.Errorf("x=1: col=%d, want 0 (Name)", got)
	}
	// Far right → -1.
	if got := d.gridColumnAtX(500); got != -1 {
		t.Errorf("x=500: col=%d, want -1", got)
	}
}
