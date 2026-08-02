package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

// newDesignerMouseModel builds a workspace model with the table designer open,
// sized the same as the results mouse tests.
func newDesignerMouseModel() Model {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.tableDesigner.Show(db.DriverMySQL, nil)
	return m
}

// Clicking a designer grid cell through the real workspace router lands the
// cursor on that (row, col) and hands focus to the grid (blurring the name
// field). Uses the rendered workspace to derive real screen coordinates, so a
// coordinate-mapping drift fails here rather than only at runtime.
func TestTableDesignerMouseRoutingAppLevel(t *testing.T) {
	m := newDesignerMouseModel()
	if !m.tableDesigner.focusName {
		t.Fatal("designer should start focused on the name field")
	}

	// Screen Y of the first data row (its first cell reads "id").
	idY := workspaceLineY(t, m, "id")

	// Scan X to find one that lands on the Name column (col 0) of row 0.
	var hitX int
	found := false
	for x := 30; x < 60; x++ {
		probe := m
		out, _ := probe.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: idY})
		mm := out.(Model)
		if !mm.tableDesigner.focusName && mm.tableDesigner.cursorRow == 0 && mm.tableDesigner.cursorCol == 0 {
			hitX = x
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no X clicked at Y=%d selected row 0 col 0", idY)
	}

	// Apply the click for real and check state.
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: hitX, Y: idY})
	m = out.(Model)
	if m.tableDesigner.focusName {
		t.Error("grid click should blur the name field")
	}
	if m.tableDesigner.cursorRow != 0 || m.tableDesigner.cursorCol != 0 {
		t.Errorf("cursor=(%d,%d), want (0,0)", m.tableDesigner.cursorRow, m.tableDesigner.cursorCol)
	}

	// The blank second row is one line below the "id" row; clicking it selects
	// row 1. (Avoid searching for "(empty)" — it also appears in row 0's Default
	// cell.)
	emptyY := idY + 1
	found = false
	var hitX2 int
	for x := 30; x < 60; x++ {
		probe := m
		o, _ := probe.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: emptyY})
		mm := o.(Model)
		if mm.tableDesigner.cursorRow == 1 {
			hitX2 = x
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no X clicked at the blank row selected row 1")
	}
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: hitX2, Y: emptyY})
	m = out.(Model)
	if m.tableDesigner.cursorRow != 1 {
		t.Errorf("cursorRow=%d, want 1", m.tableDesigner.cursorRow)
	}
}

// Wheel events scroll the designer grid through the workspace router.
func TestTableDesignerWheelRouting(t *testing.T) {
	m := newDesignerMouseModel()
	idY := workspaceLineY(t, m, "id")
	// Click into the grid first so the cursor is on row 0.
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 32, Y: idY})
	m = out.(Model)
	if m.tableDesigner.cursorRow != 0 {
		t.Fatalf("setup: cursorRow=%d, want 0", m.tableDesigner.cursorRow)
	}
	// Wheel down moves the cursor to row 1.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseWheelDown, X: 40, Y: idY})
	m = out.(Model)
	if m.tableDesigner.cursorRow != 1 {
		t.Errorf("after wheel down: cursorRow=%d, want 1", m.tableDesigner.cursorRow)
	}
}
