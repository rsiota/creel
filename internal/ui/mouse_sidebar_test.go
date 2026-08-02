package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/creel/internal/config"
)

func newSidebarMouseModel(numTables, cursor int) Model {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusConnections
	tables := make([]string, numTables)
	for i := range tables {
		tables[i] = fmt.Sprintf("t%02d", i)
	}
	m.tables = tables
	m.sidebarCursor = cursor
	return m
}

func TestSidebarMouseClickScrolledWindow(t *testing.T) {
	// 40 tables, cursor deep in the list → the visible window is scrolled
	// (start > 0). The stale-sidebarScroll bug made these clicks select a
	// table higher up the list.
	m := newSidebarMouseModel(40, 30)

	for _, want := range []int{12, 20, 30, 38} { // top, mid, cursor area, near bottom
		name := fmt.Sprintf("t%02d", want)
		y := workspaceLineY(t, m, name)
		out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 5, Y: y})
		got := out.(Model).sidebarCursor
		if got != want {
			t.Errorf("click on %s (Y=%d): cursor=%d, want %d (selects the wrong table — scrolled-window mouse bug)", name, y, got, want)
		}
	}
}

func TestSidebarMouseClickTopWindow(t *testing.T) {
	// Cursor at the top → window not scrolled (start=0). Should keep working.
	m := newSidebarMouseModel(40, 0)

	for _, want := range []int{0, 5, 15} {
		name := fmt.Sprintf("t%02d", want)
		y := workspaceLineY(t, m, name)
		out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 5, Y: y})
		got := out.(Model).sidebarCursor
		if got != want {
			t.Errorf("click on %s (Y=%d): cursor=%d, want %d", name, y, got, want)
		}
	}
}

func TestSidebarClickDoesNotRecenter(t *testing.T) {
	// Clicking a table must not scroll it to the middle of the sidebar — the
	// view is anchored so the clicked table stays exactly where it was.
	m := newSidebarMouseModel(40, 0)

	target := "t30"
	yBefore := workspaceLineY(t, m, target)

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 5, Y: yBefore})
	clicked := out.(Model)
	if clicked.sidebarCursor != 30 {
		t.Fatalf("cursor=%d, want 30", clicked.sidebarCursor)
	}

	yAfter := workspaceLineY(t, clicked, target)
	if yAfter != yBefore {
		t.Errorf("clicked table moved after selection: Y %d → %d (should stay put; the view must be anchored, not re-centered)", yBefore, yAfter)
	}
}

func TestSidebarKeyboardNavClearsAnchor(t *testing.T) {
	// After a click anchors the view, keyboard navigation should resume
	// centering (the anchor is cleared), so the selected table moves toward
	// the middle.
	m := newSidebarMouseModel(40, 0)
	yBefore := workspaceLineY(t, m, "t30")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 5, Y: yBefore})
	clicked := out.(Model)

	// j moves the cursor to t31 and clears the anchor → the view recenters.
	scrolled := clicked.scrollSidebar(1)
	if scrolled.sidebarCursor != 31 {
		t.Fatalf("cursor=%d, want 31", scrolled.sidebarCursor)
	}
	yAfter := workspaceLineY(t, scrolled, "t31")
	if yAfter == yBefore+1 {
		t.Errorf("view still anchored after keyboard nav: t31 at Y=%d (same as t30+1); expected re-centering", yAfter)
	}
}
