package ui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
)

var ansiStrip = regexp.MustCompile("\x1b\\[[0-9;]*m")

// workspaceLineY renders the workspace (ANSI-stripped) and returns the 0-based
// screen-Y of the first line containing needle. It lets the mouse test derive
// real row positions instead of hardcoding magic offsets.
func workspaceLineY(t *testing.T, m Model, needle string) int {
	t.Helper()
	out := ansiStrip.ReplaceAllString(m.viewWorkspace(), "")
	for i, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	t.Fatalf("needle %q not found in workspace", needle)
	return -1
}

func newResultsMouseModel() Model {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusResults
	m.results.SetSize(86, 22)
	m.results.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}, {"2", "bob"}, {"3", "carol"}},
		"3 rows",
	)
	return m
}

// findResultsColumnX scans X values to locate one that maps to a valid results
// column when clicked at the given Y. The cursor is reset to -1 first so a
// no-op click (default cursorCol 0) is not mistaken for a hit. Returns the X
// and the column index.
func findResultsColumnX(t *testing.T, m Model, y int) (int, int) {
	t.Helper()
	for x := 30; x < 70; x++ {
		probe := m
		probe.results.cursorRow = -1
		probe.results.cursorCol = -1
		out, _ := probe.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
		mm := out.(Model)
		if mm.results.cursorCol >= 0 {
			return x, mm.results.cursorCol
		}
	}
	t.Fatalf("no valid results column X found for Y=%d", y)
	return -1, -1
}

func TestResultsMouseClickSelectsClickedRow(t *testing.T) {
	m := newResultsMouseModel()

	// Locate the actual screen rows of the data values.
	yAlice := workspaceLineY(t, m, "alice")
	yBob := workspaceLineY(t, m, "bob")
	if yBob != yAlice+1 {
		t.Fatalf("expected bob directly below alice; got alice@%d bob@%d", yAlice, yBob)
	}

	x, col := findResultsColumnX(t, m, yAlice)
	if col < 0 {
		t.Fatalf("no column under click")
	}

	// Clicking the "alice" row must select data row 0, not the row below it.
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	clicked := out.(Model)
	if clicked.results.cursorRow != 0 {
		t.Errorf("click on row 0 (alice, Y=%d): cursorRow=%d, want 0 (off-by-one: selects the cell below)", yAlice, clicked.results.cursorRow)
	}

	// Clicking the "bob" row must select data row 1.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yBob})
	clicked = out.(Model)
	if clicked.results.cursorRow != 1 {
		t.Errorf("click on row 1 (bob, Y=%d): cursorRow=%d, want 1", yBob, clicked.results.cursorRow)
	}

	// Clicking a row below (carol) selects row 2.
	yCarol := workspaceLineY(t, m, "carol")
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yCarol})
	clicked = out.(Model)
	if clicked.results.cursorRow != 2 {
		t.Errorf("click on row 2 (carol, Y=%d): cursorRow=%d, want 2", yCarol, clicked.results.cursorRow)
	}

	// Clicking the header separator (one row below the header) must NOT move
	// the cursor onto a data row.
	yHeader := workspaceLineY(t, m, " id ")
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yHeader + 1})
	clicked = out.(Model)
	if clicked.results.cursorRow != 0 && clicked.results.cursorRow != -1 {
		t.Errorf("click on header separator (Y=%d): cursorRow=%d, want none", yHeader+1, clicked.results.cursorRow)
	}
}
