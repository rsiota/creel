package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResizePaneMovesSeamBothWays(t *testing.T) {
	m := newFocusModel() // FocusResults, 120×40
	m.layoutWorkspace()
	before := m.workspaceGeom()

	// Results + alt+k → border moves up (editor shrinks).
	m = m.resizePane("alt+k")
	after := m.workspaceGeom()
	if after.EditorHeight >= before.EditorHeight {
		t.Fatalf("results alt+k: EditorHeight %d → %d, want smaller", before.EditorHeight, after.EditorHeight)
	}
	if after.ResultsHeight <= before.ResultsHeight {
		t.Fatalf("results alt+k: ResultsHeight %d → %d, want larger", before.ResultsHeight, after.ResultsHeight)
	}

	// Results + alt+j → border moves down (editor grows), same as sidebar h/l.
	before = m.workspaceGeom()
	m = m.resizePane("alt+j")
	after = m.workspaceGeom()
	if after.EditorHeight <= before.EditorHeight {
		t.Fatalf("results alt+j: EditorHeight %d → %d, want larger", before.EditorHeight, after.EditorHeight)
	}

	// Editor + alt+k → border moves up (same seam, either focus).
	m.focus = FocusEditor
	before = m.workspaceGeom()
	m = m.resizePane("alt+k")
	after = m.workspaceGeom()
	if after.EditorHeight >= before.EditorHeight {
		t.Fatalf("editor alt+k: EditorHeight %d → %d, want smaller", before.EditorHeight, after.EditorHeight)
	}

	// Sidebar + alt+l → grow sidebar.
	m.focus = FocusConnections
	before = m.workspaceGeom()
	m = m.resizePane("alt+l")
	after = m.workspaceGeom()
	if after.SidebarWidth <= before.SidebarWidth {
		t.Fatalf("sidebar alt+l: SidebarWidth %d → %d, want larger", before.SidebarWidth, after.SidebarWidth)
	}
}

func TestResizePaneRightSlot(t *testing.T) {
	m := newFocusModel()
	m.inspector.Toggle()
	m.focus = FocusInspector
	m.layoutWorkspace()
	before := m.workspaceGeom()
	if before.RightSlotW == 0 {
		t.Fatal("precondition: inspector should open a right slot")
	}

	m = m.resizePane("alt+h")
	after := m.workspaceGeom()
	if after.RightSlotW <= before.RightSlotW {
		t.Fatalf("inspector alt+h: RightSlotW %d → %d, want larger", before.RightSlotW, after.RightSlotW)
	}

	// Centre + alt+l steals from the right slot.
	m.focus = FocusEditor
	before = m.workspaceGeom()
	m = m.resizePane("alt+l")
	after = m.workspaceGeom()
	if after.RightSlotW >= before.RightSlotW {
		t.Fatalf("editor alt+l: RightSlotW %d → %d, want smaller", before.RightSlotW, after.RightSlotW)
	}
}

func TestResizePaneKeyDispatch(t *testing.T) {
	m := newFocusModel()
	m.focus = FocusEditor
	m.layoutWorkspace()
	before := m.workspaceGeom().EditorHeight

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}, Alt: true}
	if msg.String() != "alt+j" {
		t.Fatalf("precondition: KeyMsg String=%q, want alt+j", msg.String())
	}
	out, _ := m.Update(msg)
	m2 := out.(Model)
	if m2.workspaceGeom().EditorHeight <= before {
		t.Fatalf("Update(alt+j): EditorHeight %d → %d, want larger", before, m2.workspaceGeom().EditorHeight)
	}
}

func TestResizePaneCtrlAltAlias(t *testing.T) {
	m := newFocusModel()
	m.focus = FocusResults
	m.layoutWorkspace()
	before := m.workspaceGeom().EditorHeight

	// Bubble Tea encodes ctrl+alt+j as Type=KeyCtrlJ with Alt set.
	msg := tea.KeyMsg{Type: tea.KeyCtrlJ, Alt: true}
	if msg.String() != "alt+ctrl+j" {
		t.Fatalf("precondition: KeyMsg String=%q, want alt+ctrl+j", msg.String())
	}
	out, _ := m.Update(msg)
	m2 := out.(Model)
	if m2.workspaceGeom().EditorHeight <= before {
		t.Fatalf("Update(alt+ctrl+j): EditorHeight %d → %d, want larger", before, m2.workspaceGeom().EditorHeight)
	}
}

func TestResizePaneExitsMaximize(t *testing.T) {
	m := newFocusModel()
	m.editorMaximized = true
	m.focus = FocusEditor
	m.layoutWorkspace()
	maxH := m.workspaceGeom().EditorHeight

	m = m.resizePane("alt+j")
	if m.editorMaximized {
		t.Fatal("alt+j should clear editor maximize")
	}
	// Seeded to maximized height then grown; stays at max clamp if already full.
	got := m.workspaceGeom().EditorHeight
	if got < maxH {
		t.Fatalf("EditorHeight=%d after resize from maximized %d", got, maxH)
	}
}
