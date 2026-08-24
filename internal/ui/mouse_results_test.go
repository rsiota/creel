package ui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
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
	g := m.workspaceGeom()
	// Start past the sidebar↔centre seam (SidebarWidth-1 and SidebarWidth are
	// resize handles) and stop before the centre↔right-slot seam so a probe
	// click cannot be swallowed by a split drag.
	// layoutWorkspace/SetSize also clamps cursorCol from -1 → 0, so a drag
	// that only relayouts would otherwise look like a successful column hit.
	for x := g.SidebarWidth + 1; x < g.EditorRight && x < g.SidebarWidth+50; x++ {
		if m.onSidebarSplit(x, y, g) || m.onRightSlotSplit(x, y, g) {
			continue
		}
		wantCol := m.results.ColumnAtX(x - g.SidebarWidth)
		if wantCol < 0 {
			continue
		}
		probe := m
		probe.results.cursorRow = -1
		probe.results.cursorCol = -1
		out, _ := probe.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
		mm := out.(Model)
		if mm.sidebarDragging || mm.splitDragging || mm.rightSlotDragging {
			continue
		}
		if mm.results.cursorCol == wantCol {
			return x, wantCol
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

// TestResultsDoubleClickEntersEditMode verifies that two left-clicks on the
// same data cell within doubleClickInterval enter inline edit mode, while a
// single click does not.
func TestResultsDoubleClickEntersEditMode(t *testing.T) {
	m := newResultsMouseModel()
	m.results.SetEditable("users", []string{"id"})

	yAlice := workspaceLineY(t, m, "alice")
	x, col := findResultsColumnX(t, m, yAlice)
	if col < 0 {
		t.Fatalf("no column under click")
	}

	// First click: moves cursor, must NOT enter edit mode.
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if m.results.IsEditing() {
		t.Fatal("single click should not enter edit mode")
	}

	// Second click on the same cell: enters edit mode.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if !m.results.IsEditing() {
		t.Errorf("double click on a cell should enter inline edit mode")
	}
}

// TestResultsDoubleClickDifferentCellsDoesNotEdit verifies that clicking two
// different cells does not trigger edit mode even within doubleClickInterval.
func TestResultsDoubleClickDifferentCellsDoesNotEdit(t *testing.T) {
	m := newResultsMouseModel()
	m.results.SetEditable("users", []string{"id"})

	yAlice := workspaceLineY(t, m, "alice")
	yBob := workspaceLineY(t, m, "bob")
	x, _ := findResultsColumnX(t, m, yAlice)

	// Click alice then bob: different rows, no edit.
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yBob})
	m = out.(Model)
	if m.results.IsEditing() {
		t.Errorf("clicking two different cells should not enter edit mode")
	}
}

// TestResultsDoubleClickNotEditableDoesNotEdit verifies that double-clicking a
// cell on non-editable results (no primary key) does not enter edit mode.
func TestResultsDoubleClickNotEditableDoesNotEdit(t *testing.T) {
	m := newResultsMouseModel()
	// Not editable: SetEditable is never called.

	yAlice := workspaceLineY(t, m, "alice")
	x, _ := findResultsColumnX(t, m, yAlice)

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if m.results.IsEditing() {
		t.Errorf("double click on non-editable results should not enter edit mode")
	}
}

// TestResultsDoubleClickClosesInspectorAndEdits verifies that double-clicking a
// results cell while the inspector is open closes the inspector and enters
// inline edit mode on that cell, so the gesture isn't a silent no-op.
func TestResultsDoubleClickClosesInspectorAndEdits(t *testing.T) {
	m := newResultsMouseModel()
	m.results.SetEditable("users", []string{"id"})

	// Compute the click target before opening the inspector: the results row
	// Y and column X are unchanged by the inspector toggle (only the panel
	// width changes), and this avoids "alice" matching the inspector's field.
	yAlice := workspaceLineY(t, m, "alice")
	x, col := findResultsColumnX(t, m, yAlice)
	if col < 0 {
		t.Fatalf("no column under click")
	}

	m.inspector.Toggle() // open the inspector
	m.layoutWorkspace()
	if !m.inspector.IsVisible() {
		t.Fatal("inspector should be visible")
	}

	// First click: cursor moves, inspector stays open, no edit.
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if m.results.IsEditing() {
		t.Fatal("single click should not enter edit mode")
	}
	if !m.inspector.IsVisible() {
		t.Fatal("single click should not close the inspector")
	}

	// Second click (double-click): closes the inspector and edits the cell.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if m.inspector.IsVisible() {
		t.Errorf("double click should close the inspector to edit the cell")
	}
	if !m.results.IsEditing() {
		t.Errorf("double click should enter inline edit mode")
	}
}

// TestResultsKeyEditClosesInspectorAndEdits verifies that pressing e/i on the
// results panel while the inspector is open closes it and edits the cell,
// matching mouse double-click behaviour.
func TestResultsKeyEditClosesInspectorAndEdits(t *testing.T) {
	for _, key := range []string{"e", "i"} {
		t.Run(key, func(t *testing.T) {
			m := newResultsMouseModel()
			m.results.SetEditable("users", []string{"id"})
			m.results.SetCursor(0, 1)
			m.inspector.Toggle()
			m.layoutWorkspace()
			m.focus = FocusResults
			if !m.inspector.IsVisible() {
				t.Fatal("inspector should be visible")
			}

			out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			m = out.(Model)
			if m.inspector.IsVisible() {
				t.Errorf("%q should close the inspector", key)
			}
			if !m.results.IsEditing() {
				t.Errorf("%q should enter inline edit mode", key)
			}
		})
	}
}

// TestResultsKeyExpandClosesInspector verifies that E opens the cell popup
// after dismissing the inspector when focus is on results.
func TestResultsKeyExpandClosesInspector(t *testing.T) {
	m := newResultsMouseModel()
	m.results.SetEditable("users", []string{"id"})
	m.results.SetCursor(0, 1)
	m.inspector.Toggle()
	m.layoutWorkspace()
	m.focus = FocusResults

	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	m = out.(Model)
	if m.inspector.IsVisible() {
		t.Error("E should close the inspector")
	}
	if !m.cellEdit.IsVisible() {
		t.Error("E should open the cell-edit popup")
	}
}

// TestResultsKeyEditKeepsInspectorWhenInserting verifies e does not discard
// an in-progress inspector insert.
func TestResultsKeyEditKeepsInspectorWhenInserting(t *testing.T) {
	m := newResultsMouseModel()
	m.results.SetEditable("users", []string{"id"})
	m.results.SetCursor(0, 1)
	m.inspector.Toggle()
	m.inspector.StartInsert()
	m.layoutWorkspace()
	m.focus = FocusResults

	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = out.(Model)
	if !m.inspector.IsVisible() || !m.inspector.IsInserting() {
		t.Error("inspector insert should be preserved")
	}
	if m.results.IsEditing() {
		t.Error("should not enter grid edit while inspector is inserting")
	}
}

// TestResultsDoubleClickKeepsInspectorWhenInserting verifies that a double-click
// on the grid does not close the inspector (or start a grid edit) while the
// inspector is mid-insert, so in-progress insert data isn't discarded.
func TestResultsDoubleClickKeepsInspectorWhenInserting(t *testing.T) {
	m := newResultsMouseModel()
	m.results.SetEditable("users", []string{"id"})

	yAlice := workspaceLineY(t, m, "alice")
	x, _ := findResultsColumnX(t, m, yAlice)

	m.inspector.Toggle()
	m.inspector.StartInsert()
	m.layoutWorkspace()
	if !m.inspector.IsInserting() {
		t.Fatal("inspector should be in insert mode")
	}

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if !m.inspector.IsVisible() {
		t.Errorf("inspector should stay open while inserting")
	}
	if !m.inspector.IsInserting() {
		t.Errorf("insert mode should be preserved")
	}
	if m.results.IsEditing() {
		t.Errorf("should not enter grid edit mode while inspector is inserting")
	}
}

// TestResultsClickAnotherCellCommitsEdit is a regression test: clicking a
// different cell while inline-editing another used to leave edit mode active
// with the original cell's text buffer, so the clicked cell then rendered that
// text in the edit colour. The click now commits the in-flight edit (mirroring
// Enter) onto its own cell and moves the cursor, leaving the clicked cell a
// plain selection.
func TestResultsClickAnotherCellCommitsEdit(t *testing.T) {
	m := newResultsMouseModel()
	m.results.SetEditable("users", []string{"id"})

	yAlice := workspaceLineY(t, m, "alice")
	yBob := workspaceLineY(t, m, "bob")
	x, col := findResultsColumnX(t, m, yAlice)
	if col < 0 {
		t.Fatalf("no column under click")
	}
	editedRow, editedCol := 0, col // alice is data row 0

	// Enter inline edit on alice via a double-click.
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if !m.results.IsEditing() {
		t.Fatal("precondition: double-click should enter edit mode")
	}

	// Simulate typing a new value into the cell being edited.
	m.results.editInput.SetValue("edited-value")

	// Click a different cell (bob's row). This must commit the edit on alice,
	// not carry the edit buffer over to bob.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yBob})
	m = out.(Model)
	if m.results.IsEditing() {
		t.Errorf("clicking another cell should exit edit mode (still editing)")
	}
	if m.results.CursorRow() != 1 {
		t.Errorf("cursor row=%d want 1 (bob)", m.results.CursorRow())
	}
	// The edit is staged on the original cell (alice), not lost or mis-placed.
	if got := m.results.RowValue(editedRow, editedCol); got != "edited-value" {
		t.Errorf("edited cell value=%q want %q (edit should be staged on its own cell)", got, "edited-value")
	}
	// The clicked cell (bob) is a plain selection, not an edit buffer.
	if got := m.results.RowValue(1, col); got == "edited-value" {
		t.Errorf("clicked cell holds the edited value — edit buffer leaked onto the new cell")
	}
}

// TestResultsClickSameCellKeepsEditing verifies that clicking the cell already
// being edited does not drop out of the textinput (only a different cell
// commits), so a stray click inside the editing cell doesn't discard work.
func TestResultsClickSameCellKeepsEditing(t *testing.T) {
	m := newResultsMouseModel()
	m.results.SetEditable("users", []string{"id"})

	yAlice := workspaceLineY(t, m, "alice")
	x, _ := findResultsColumnX(t, m, yAlice)

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if !m.results.IsEditing() {
		t.Fatal("precondition: double-click should enter edit mode")
	}

	// A further click on the same cell keeps the textinput focused.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yAlice})
	m = out.(Model)
	if !m.results.IsEditing() {
		t.Errorf("clicking the cell being edited should keep edit mode active")
	}
}

// TestResultsWheelCoalescing verifies that a rapid flood of wheel events (e.g.
// a Magic Mouse / trackpad momentum swipe) is coalesced into a single scroll
// per tick rather than one Update+render per event. Without coalescing the
// event rate outruns the renderer and the grid keeps "scrolling without
// stopping" after the gesture ends.
func TestResultsWheelCoalescing(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusResults

	// 100 rows in a viewport that fits far fewer, so scrolling is meaningful.
	rows := make([][]string, 100)
	for i := range rows {
		rows[i] = []string{"x", "y"}
	}
	m.results.SetSize(86, 22)
	m.results.SetResult([]string{"id", "name"}, rows, "100 rows")

	// Y=30 sits inside the results panel (resultsTop=12, resultsBottom=38).
	wheel := func(down bool) {
		typ := tea.MouseWheelUp
		if down {
			typ = tea.MouseWheelDown
		}
		upd, _ := m.Update(tea.MouseMsg{Type: typ, X: 50, Y: 30})
		m = upd.(Model)
	}

	// A burst of 20 wheel-downs: nothing scrolls yet (coalesced), but the
	// delta accumulates and exactly one tick is armed.
	for i := 0; i < 20; i++ {
		wheel(true)
	}
	if got := m.results.ScrollRow(); got != 0 {
		t.Errorf("scrollRow = %d after flood, want 0 (events coalesced, not applied per-event)", got)
	}
	if m.wheelAccum != 20 {
		t.Errorf("wheelAccum = %d, want 20", m.wheelAccum)
	}
	if !m.wheelTickPending {
		t.Error("wheelTickPending should be true while a tick is armed")
	}

	// More events while a tick is pending must NOT arm a second tick.
	wheel(true)
	if m.wheelAccum != 21 {
		t.Errorf("wheelAccum = %d, want 21", m.wheelAccum)
	}
	if !m.wheelTickPending {
		t.Error("wheelTickPending should still be true")
	}

	// The tick flushes the whole accumulated delta in one scroll.
	upd, _ := m.Update(wheelTickMsg{})
	m = upd.(Model)
	if got := m.results.ScrollRow(); got != 21 {
		t.Errorf("scrollRow = %d after tick, want 21", got)
	}
	if m.wheelAccum != 0 || m.wheelTickPending {
		t.Errorf("after tick: wheelAccum=%d wheelTickPending=%v, want 0/false", m.wheelAccum, m.wheelTickPending)
	}

	// A delta past the end clamps; a large negative delta clamps back to 0.
	m.wheelAccum = 1 << 20
	upd, _ = m.Update(wheelTickMsg{})
	m = upd.(Model)
	if got, max := m.results.ScrollRow(), m.results.NumRows()-1; got >= max {
		t.Errorf("scrollRow = %d should clamp below %d", got, max)
	}
	m.wheelAccum = -(1 << 20)
	upd, _ = m.Update(wheelTickMsg{})
	m = upd.(Model)
	if got := m.results.ScrollRow(); got != 0 {
		t.Errorf("scrollRow = %d after huge up-scroll, want 0 (clamped)", got)
	}
}

// TestResultsWheelViewCache verifies that a coalesced wheel event marks the
// frame view-cached so bubbletea's per-message View() call returns the prior
// frame instead of rebuilding the whole screen (the cost that lets a violent
// momentum flood outrun the render loop). The tick that applies the scroll
// invalidates the cache so the next View() rebuilds.
func TestResultsWheelViewCache(t *testing.T) {
	m := newResultsMouseModel()
	v0 := m.View() // primes the cache (*viewBuf = v0)

	// A wheel event is view-neutral: it arms the cache and must not change the
	// rendered output.
	upd, _ := m.Update(tea.MouseMsg{Type: tea.MouseWheelDown, X: 50, Y: 30})
	m = upd.(Model)
	if !m.viewCached {
		t.Fatal("coalesced wheel event should mark the view cached (viewCached=true)")
	}
	if m.View() != v0 {
		t.Error("View() changed after a view-neutral wheel event; cache should return the prior frame")
	}

	// Applying the accumulated scroll invalidates the cache → View() rebuilds.
	upd, _ = m.Update(wheelTickMsg{})
	m = upd.(Model)
	if m.viewCached {
		t.Error("wheelTickMsg should invalidate the view cache (viewCached=false)")
	}

	// Any other message (e.g. a key) also invalidates, so the cache can't go stale.
	upd, _ = m.Update(tea.MouseMsg{Type: tea.MouseWheelDown, X: 50, Y: 30})
	m = upd.(Model)
	if !m.viewCached {
		t.Fatal("precondition: wheel should re-arm the cache")
	}
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = upd.(Model)
	if m.viewCached {
		t.Error("a non-wheel message should invalidate the view cache")
	}
}
