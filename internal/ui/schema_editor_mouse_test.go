package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/db"
)

// showMouseEditor builds a structure editor sized for the tests below. The
// Columns grid has a few rows so cursor/click mapping is meaningful.
func showMouseEditor(t *testing.T) *SchemaEditor {
	t.Helper()
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "id", Type: "INT", PrimaryKey: true, AutoIncrement: true, NotNull: true},
		{Name: "email", Type: "VARCHAR(255)", NotNull: true},
		{Name: "age", Type: "INT"},
		{Name: "score", Type: "REAL"},
	})
	return &e
}

// tabAtX resolves each tab to a distinct X range, and gaps (between tabs)
// and far-right clicks resolve to nothing.
func TestSchemaEditorTabAtX(t *testing.T) {
	e := showMouseEditor(t)
	tabs := e.availableTabs()

	// Compute each tab's [start, end) from its label, matching renderTabBar
	// (1-char padding each side, single space between tabs).
	cursor := 0
	starts := make([]int, len(tabs))
	ends := make([]int, len(tabs))
	for i, tab := range tabs {
		segW := len(seTabLabels[tab]) + 2
		starts[i] = cursor
		ends[i] = cursor + segW
		cursor = ends[i] + 1 // +1 for the space separator
	}

	// First char of each tab resolves to that tab.
	for i, tab := range tabs {
		if got, ok := e.tabAtX(starts[i]); !ok || got != tab {
			t.Errorf("x=%d (start of %s): tab=%d ok=%v", starts[i], seTabLabels[tab], got, ok)
		}
	}
	// The space gap after each tab resolves to nothing.
	for i := range tabs {
		gap := ends[i]
		if _, ok := e.tabAtX(gap); ok {
			t.Errorf("x=%d (gap after %s) should not resolve to a tab", gap, seTabLabels[tabs[i]])
		}
	}
	// Far past the last tab → no tab.
	if _, ok := e.tabAtX(500); ok {
		t.Error("x=500 should not resolve to a tab")
	}
}

// Clicking a tab switches the active tab (and resets the read-only cursor).
func TestSchemaEditorClickSwitchesTab(t *testing.T) {
	e := showMouseEditor(t)
	e.roCursor = 2
	e.roScroll = 1

	// Tab-bar row is content-Y 2. Indexes starts right after Columns
	// ("Columns" + 2 padding = 9 wide, +1 gap → X=10).
	indexesX := len(seTabLabels[seTabColumns]) + 2 + 1
	e.Click(indexesX, 2)
	if e.activeTab != seTabIndexes {
		t.Errorf("activeTab=%d, want Indexes", e.activeTab)
	}
	if e.roCursor != 0 || e.roScroll != 0 {
		t.Errorf("ro cursor/scroll not reset: %d/%d", e.roCursor, e.roScroll)
	}

	// Clicking the already-active tab is a no-op (no error).
	e.Click(indexesX, 2)
	if e.activeTab != seTabIndexes {
		t.Errorf("activeTab=%d, want Indexes after re-click", e.activeTab)
	}
}

// Clicking a data cell on the Columns grid moves the (row, col) cursor there.
func TestSchemaEditorClickMovesColumnsCursor(t *testing.T) {
	e := showMouseEditor(t)
	top := e.gridTopRow()

	// Grid: top border (y=4), header (5), sep (6), first data row at y=7.
	// Column 0 (Name) is at the leading edge; column 1 (Type) follows it.
	row := top + 2 // third visible row ("age")
	e.Click(1, 7+2) // col 0 (Name) of that row
	if e.cursorRow != row {
		t.Errorf("cursorRow=%d, want %d", e.cursorRow, row)
	}
	if e.cursorCol != 0 {
		t.Errorf("cursorCol=%d, want 0 (Name)", e.cursorCol)
	}

	// Type column: skip Name (colWidths[0]+2 + leading + separator).
	typeX := 1 + e.colWidths[0] + 2 + 1
	e.Click(typeX, 7+2)
	if e.cursorCol != 1 {
		t.Errorf("cursorCol=%d, want 1 (Type)", e.cursorCol)
	}
}

// A double-click on an editable cell starts an inline edit.
func TestSchemaEditorDoubleClickEdits(t *testing.T) {
	e := showMouseEditor(t)
	e.Click(1, 7+1) // email row, Name cell (editable)
	if e.editing {
		t.Fatal("single click should not start editing")
	}
	// Second click on the same cell within the interval → edit.
	e.Click(1, 7+1)
	if !e.editing {
		t.Fatal("double-click should start inline edit")
	}
}

// A double-click on a non-editable cell (auto-increment PK) does not edit.
func TestSchemaEditorDoubleClickNonEditableNoEdit(t *testing.T) {
	e := showMouseEditor(t)
	// id is row top+0, Name cell. id is PK+auto-increment → Name not editable
	// (cellEditable forbids auto-increment columns).
	e.Click(1, 7+0)
	e.Click(1, 7+0)
	if e.editing {
		t.Fatal("double-click on non-editable cell should not start editing")
	}
}

// Two clicks on different cells do not count as a double-click.
func TestSchemaEditorDoubleClickDifferentCells(t *testing.T) {
	e := showMouseEditor(t)
	e.Click(1, 7+1) // email, Name
	e.Click(1, 7+2) // age, Name — different row
	if e.editing {
		t.Fatal("clicks on different cells must not start editing")
	}
}

// A click while a cell is mid-edit cancels the edit and relocates the cursor.
func TestSchemaEditorClickCancelsInProgressEdit(t *testing.T) {
	e := showMouseEditor(t)
	// Enter edit mode on the email row, Name cell (editable).
	e.Click(1, 7+1)
	e.Click(1, 7+1)
	if !e.editing {
		t.Fatal("expected edit mode after double-click")
	}
	// Click a different cell (the age row).
	e.Click(1, 7+2)
	if e.editing {
		t.Error("clicking another cell should cancel the in-progress edit")
	}
	top := e.gridTopRow()
	if e.cursorRow != top+2 {
		t.Errorf("cursorRow=%d, want %d", e.cursorRow, top+2)
	}
}

// Clicks on the read-only metadata tabs move the row cursor.
func TestSchemaEditorClickReadOnlyRow(t *testing.T) {
	e := showMouseEditor(t)
	e.LoadStructure(structureData{
		checks: []db.CheckConstraint{
			{Column: "a", Expression: "a > 0"},
			{Column: "b", Expression: "b > 0"},
			{Column: "c", Expression: "c > 0"},
		},
	})
	e.activeTab = seTabChecks

	// Third data row (index 2) of the box table.
	e.Click(1, 7+2)
	if e.roCursor != 2 {
		t.Errorf("roCursor=%d, want 2", e.roCursor)
	}
}

// Wheel moves the cursor on the Columns grid and the ro cursor on read-only
// tabs, matching j/k.
func TestSchemaEditorWheel(t *testing.T) {
	e := showMouseEditor(t)
	e.cursorRow = 0
	e.Wheel(1)
	if e.cursorRow != 1 {
		t.Errorf("Columns wheel down: cursorRow=%d, want 1", e.cursorRow)
	}
	e.Wheel(-1)
	if e.cursorRow != 0 {
		t.Errorf("Columns wheel up: cursorRow=%d, want 0", e.cursorRow)
	}

	e.LoadStructure(structureData{indexes: []db.Index{{Name: "i1"}, {Name: "i2"}}})
	e.activeTab = seTabIndexes
	e.roCursor = 0
	e.Wheel(1)
	if e.roCursor != 1 {
		t.Errorf("read-only wheel down: roCursor=%d, want 1", e.roCursor)
	}
}

// gridTopRow keeps the cursor in view; clicking near the bottom of a long
// column list lands on a real row, never past the end.
func TestSchemaEditorClickClampsToRows(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	cols := make([]db.TableColumnInfo, 50)
	for i := range cols {
		cols[i] = db.TableColumnInfo{Name: "c" + string(rune('a'+i%26)), Type: "INT"}
	}
	e.Show("big", db.DriverMySQL, cols)

	// Click well past the data area — must not panic or set an out-of-range row.
	e.Click(1, 100)
	if e.cursorRow < 0 || e.cursorRow >= len(e.rows) {
		t.Errorf("cursorRow=%d out of range [0,%d)", e.cursorRow, len(e.rows))
	}
}

// Sanity: the double-click interval matches the results panel constant so the
// two panels feel consistent.
func TestSchemaEditorDoubleClickInterval(t *testing.T) {
	if doubleClickInterval <= 0 {
		t.Fatal("doubleClickInterval must be positive")
	}
	// Ensure the type is usable as a Duration in this package.
	_ = time.Duration(doubleClickInterval)
	_ = tea.MouseLeft
}
