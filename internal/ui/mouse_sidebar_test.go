package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
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
