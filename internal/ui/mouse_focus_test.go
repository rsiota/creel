package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/creel/internal/config"
)

// newFocusModel builds a workspace model whose initial focus is on the results
// panel, so we can verify that clicking another panel moves focus away from it.
func newFocusModel() Model {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusResults
	return m
}

// Clicking the sidebar should focus (highlight) it, not leave focus on results.
func TestMouseClickSidebarFocuses(t *testing.T) {
	m := newFocusModel()
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 5, Y: 5})
	if got := out.(Model).focus; got != FocusConnections {
		t.Errorf("click sidebar: focus=%v, want FocusConnections", got)
	}
}

// Clicking anywhere in the editor panel should focus it.
func TestMouseClickEditorFocuses(t *testing.T) {
	m := newFocusModel()
	// Editor panel content area sits between the tab bar (Y=1) and the
	// results panel (Y=12).
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 40, Y: 6})
	if got := out.(Model).focus; got != FocusEditor {
		t.Errorf("click editor: focus=%v, want FocusEditor", got)
	}
}

// Clicking the tab bar should focus it.
func TestMouseClickTabBarFocuses(t *testing.T) {
	m := newFocusModel()
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 32, Y: 1})
	if got := out.(Model).focus; got != FocusTabBar {
		t.Errorf("click tab bar: focus=%v, want FocusTabBar", got)
	}
}

// Clicking the results panel — even empty space with no result loaded — should
// focus it.
func TestMouseClickResultsFocuses(t *testing.T) {
	m := newFocusModel()
	m.focus = FocusEditor
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 40, Y: 20})
	if got := out.(Model).focus; got != FocusResults {
		t.Errorf("click results: focus=%v, want FocusResults", got)
	}
}

// Clicking the inspector column should focus it.
func TestMouseClickInspectorFocuses(t *testing.T) {
	m := newFocusModel()
	m.inspector.Toggle()
	m.layoutWorkspace()
	if !m.inspector.IsVisible() {
		t.Fatalf("inspector did not become visible")
	}
	// Inspector occupies the rightmost InspectorWidth columns.
	x := m.width - 10
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: 10})
	if got := out.(Model).focus; got != FocusInspector {
		t.Errorf("click inspector: focus=%v, want FocusInspector", got)
	}
}
