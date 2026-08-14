package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMoveFocusNoDeadEnds locks in the three-column panel navigation:
// Sidebar ↔ Editor ↔ right-slot, with the editor as the centre hub. The key
// property: a left→right and right→left traverse never dead-ends at the editor
// — the prior graph stranded you there (the tab bar had no ctrl+l, and the
// right slot returned to results rather than the editor).
func TestMoveFocusNoDeadEnds(t *testing.T) {
	m := newFocusModel()
	m.inspector.Toggle() // open the right-hand slot
	if !m.inspector.IsVisible() {
		t.Fatal("inspector did not become visible")
	}

	// Forward: Sidebar →(l)→ Editor →(l)→ Inspector.
	m.focus = FocusConnections
	m = m.moveFocus("ctrl+l")
	if m.focus != FocusEditor {
		t.Fatalf("sidebar→l: focus=%v, want FocusEditor", m.focus)
	}
	m = m.moveFocus("ctrl+l")
	if m.focus != FocusInspector {
		t.Fatalf("editor→l: focus=%v, want FocusInspector", m.focus)
	}

	// Reverse: Inspector →(h)→ Editor →(h)→ Sidebar.
	m = m.moveFocus("ctrl+h")
	if m.focus != FocusEditor {
		t.Fatalf("inspector→h: focus=%v, want FocusEditor", m.focus)
	}
	m = m.moveFocus("ctrl+h")
	if m.focus != FocusConnections {
		t.Fatalf("editor→h: focus=%v, want FocusConnections", m.focus)
	}

	// The tab bar must not dead-end going right (the original bug): it should
	// reach the inspector directly.
	m.focus = FocusTabBar
	m = m.moveFocus("ctrl+l")
	if m.focus != FocusInspector {
		t.Fatalf("tabbar→l: focus=%v, want FocusInspector", m.focus)
	}

	// And ctrl+l on the editor with no right slot open is a no-op (not a crash,
	// not a stray jump).
	m2 := newFocusModel() // inspector closed
	m2.focus = FocusEditor
	m2 = m2.moveFocus("ctrl+l")
	if m2.focus != FocusEditor {
		t.Fatalf("editor→l with no right slot: focus=%v, want FocusEditor", m2.focus)
	}
}

// TestMoveFocusCentreStack covers ctrl+j/ctrl/k within the centre column.
func TestMoveFocusCentreStack(t *testing.T) {
	m := newFocusModel()
	m.focus = FocusTabBar
	m = m.moveFocus("ctrl+j")
	if m.focus != FocusEditor {
		t.Fatalf("tabbar→j: focus=%v, want FocusEditor", m.focus)
	}
	m = m.moveFocus("ctrl+j")
	if m.focus != FocusResults {
		t.Fatalf("editor→j: focus=%v, want FocusResults", m.focus)
	}
	m = m.moveFocus("ctrl+k")
	if m.focus != FocusEditor {
		t.Fatalf("results→k: focus=%v, want FocusEditor", m.focus)
	}
	m = m.moveFocus("ctrl+k")
	if m.focus != FocusTabBar {
		t.Fatalf("editor→k: focus=%v, want FocusTabBar", m.focus)
	}
}

func TestCycleFocusSkipsTabBar(t *testing.T) {
	m := newFocusModel()
	m.focus = FocusConnections
	m = m.cycleFocus()
	if m.focus != FocusEditor {
		t.Fatalf("tab from sidebar: focus=%v, want FocusEditor", m.focus)
	}
	m = m.cycleFocus()
	if m.focus != FocusResults {
		t.Fatalf("tab from editor: focus=%v, want FocusResults", m.focus)
	}
	m = m.cycleFocusBack()
	if m.focus != FocusEditor {
		t.Fatalf("shift-tab from results: focus=%v, want FocusEditor", m.focus)
	}
	m = m.cycleFocusBack()
	if m.focus != FocusConnections {
		t.Fatalf("shift-tab from editor: focus=%v, want FocusConnections", m.focus)
	}

	// Tab still leaves the tab bar if it was focused by click / ctrl+k.
	m.focus = FocusTabBar
	m = m.cycleFocus()
	if m.focus != FocusEditor {
		t.Fatalf("tab from tab bar: focus=%v, want FocusEditor", m.focus)
	}
}

func TestSidebarLFocusesResults(t *testing.T) {
	m := newFocusModel()
	m.focus = FocusConnections
	got, _ := m.updateWorkspace(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if got.(Model).focus != FocusResults {
		t.Fatalf("sidebar l: focus=%v, want FocusResults", got.(Model).focus)
	}
}
