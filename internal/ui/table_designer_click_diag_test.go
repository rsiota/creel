package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/creel/internal/config"
	"github.com/ruben/creel/internal/db"
)

// Regression test for stale size: SetSize is applied during View (a
// value-receiver), so the model that handles mouse events used to carry a tiny
// stale height/colWidths. That made gridTopRow() ≈ cursorRow once the cursor
// moved, so clicking a row above the cursor (or a non-first column) landed on
// the wrong cell — only the first-clicked cell ever stayed highlighted.
//
// This drives a sequence through the real workspace router and asserts each
// click selects the cell under the cursor, including clicking back up and
// across columns after the cursor has moved.
func TestTableDesignerClickMappingNotStale(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.tableDesigner.Show(db.DriverMySQL, nil)

	idY := workspaceLineY(t, m, "id") // screen Y of row 0
	nameX := 31 + 1                   // sidebarWidth(30) + border(1) + into Name col
	typeX := nameX + m.tableDesigner.colWidths[0] + 2 + 1

	click := func(x, y int) Model {
		out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
		m = out.(Model)
		return m
	}

	// Select row 0 Name, then row 1 Name.
	click(nameX, idY)
	if got := (m.tableDesigner.cursorRow); got != 0 {
		t.Fatalf("click row0/Name: cursorRow=%d want 0", got)
	}
	click(nameX, idY+1)
	if m.tableDesigner.cursorRow != 1 {
		t.Fatalf("click row1/Name: cursorRow=%d want 1", m.tableDesigner.cursorRow)
	}

	// Now click row 0's Type column — cursor is on row 1, so a stale
	// gridTopRow would map this to the wrong row. Must land on (0,1).
	click(typeX, idY)
	if m.tableDesigner.cursorRow != 0 || m.tableDesigner.cursorCol != 1 {
		t.Errorf("click row0/Type: cursor=(%d,%d) want (0,1)",
			m.tableDesigner.cursorRow, m.tableDesigner.cursorCol)
	}

	// Clicking the same cell again (double-click) enters edit; clicking away
	// cancels and relocates.
	click(typeX, idY)
	if !m.tableDesigner.editing {
		t.Error("double-click on row0/Type should start editing")
	}
	click(nameX, idY+1)
	if m.tableDesigner.editing {
		t.Error("clicking another cell should cancel the in-progress edit")
	}
	if m.tableDesigner.cursorRow != 1 || m.tableDesigner.cursorCol != 0 {
		t.Errorf("after edit-cancel click: cursor=(%d,%d) want (1,0)",
			m.tableDesigner.cursorRow, m.tableDesigner.cursorCol)
	}
}
