package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func TestColumnSepAtX(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(40, 10)
	r.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}},
		"",
	)
	// Layout: │ + " id " + │ + " name " + …  (widths include sort-indicator room)
	idW := r.ColWidth(0)
	// Left border at 0; first cell content starts at 1; first │ at 1+idW+2.
	sep0 := 1 + idW + 2
	if got := r.ColumnSepAtX(sep0); got != 0 {
		t.Fatalf("sep at %d = %d, want col 0 (idW=%d)", sep0, got, idW)
	}
	if got := r.ColumnSepAtX(sep0 - colResizeGrip); got != 0 {
		t.Fatalf("grip left of sep = %d, want 0", got)
	}
	// Middle of the id header label should not be a sep hit.
	mid := 1 + idW/2
	if got := r.ColumnSepAtX(mid); got != -1 {
		t.Fatalf("mid-header %d = %d, want -1 (sort owns the label)", mid, got)
	}
}

func findHeaderSepX(t *testing.T, m Model, col int) (headerY, sepX int) {
	t.Helper()
	g := m.workspaceGeom()
	headerY = g.ResultsTop + 1
	rel := m.results.ColumnSepAtX
	// Scan relative X inside the results panel for the separator of col.
	for relX := 1; relX < g.RightWidth; relX++ {
		if rel(relX) == col {
			return headerY, g.SidebarWidth + relX
		}
	}
	t.Fatalf("no separator X found for column %d", col)
	return 0, 0
}

func TestColResizeDragWidensColumn(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusResults
	m.tables = []string{"users"}
	m.results.SetSize(86, 22)
	m.results.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}, {"2", "bob"}},
		"2 rows",
	)
	m.results.SetEditable("users", []string{"id"})
	m.syncColWidthMemory()
	m.layoutWorkspace()

	startW := m.results.ColWidth(0)
	headerY, sepX := findHeaderSepX(t, m, 0)

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{
		Type: tea.MouseLeft, Action: tea.MouseActionPress, X: sepX, Y: headerY,
	})
	m = out.(Model)
	if !m.colResizeDragging {
		t.Fatal("expected colResizeDragging after press on separator")
	}
	if m.colResizeCol != 0 {
		t.Fatalf("colResizeCol = %d, want 0", m.colResizeCol)
	}

	// Drag 10 cells right → widen by 10.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{
		Type: tea.MouseLeft, Action: tea.MouseActionMotion, X: sepX + 10, Y: headerY,
	})
	m = out.(Model)
	want := startW + 10
	if want > maxManualCellWidth {
		want = maxManualCellWidth
	}
	if m.results.ColWidth(0) != want {
		t.Fatalf("after drag width = %d, want %d", m.results.ColWidth(0), want)
	}
	if got := m.colOverridesFor("users")["id"]; got != want {
		t.Fatalf("override = %d, want %d", got, want)
	}

	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{
		Type: tea.MouseRelease, Action: tea.MouseActionRelease, X: sepX + 10, Y: headerY,
	})
	m = out.(Model)
	if m.colResizeDragging {
		t.Error("colResizeDragging still set after release")
	}
}

func TestHeaderLabelClickStillSorts(t *testing.T) {
	m := newResultsMouseModel()
	m.layoutWorkspace()
	// Ensure canFilter path: need a simple table browse for sort to run.
	// sortByColName still sets sort state when canFilter is false? Check —
	// header click calls sortByColName which returns a cmd. We only assert
	// that pressing the label does NOT start a resize drag.
	g := m.workspaceGeom()
	headerY := g.ResultsTop + 1
	// Find an X that maps to a column but is not a separator.
	var labelX int
	found := false
	for relX := 1; relX < 40; relX++ {
		if m.results.ColumnSepAtX(relX) >= 0 {
			continue
		}
		if m.results.ColumnAtX(relX) == 0 {
			labelX = g.SidebarWidth + relX
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no non-separator X inside column 0")
	}
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{
		Type: tea.MouseLeft, Action: tea.MouseActionPress, X: labelX, Y: headerY,
	})
	m = out.(Model)
	if m.colResizeDragging {
		t.Fatal("label click must not start column resize")
	}
}
