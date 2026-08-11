package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Dragging the editor/results seam should resize the editor panel.
func TestSplitDragResizesEditor(t *testing.T) {
	m := newFocusModel() // 120×40, default editor height 12
	g := m.workspaceGeom()
	if g.EditorHeight != defaultEditorHeight {
		t.Fatalf("precondition: EditorHeight=%d", g.EditorHeight)
	}

	// Press on the results top border (the seam).
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionPress,
		X:      40,
		Y:      g.ResultsTop,
	})
	m = out.(Model)
	if !m.splitDragging {
		t.Fatal("expected splitDragging after press on seam")
	}

	// Drag down: results top follows Y=20 → editor grows to 20.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionMotion,
		X:      40,
		Y:      20,
	})
	m = out.(Model)
	g = m.workspaceGeom()
	if g.EditorHeight != 20 {
		t.Errorf("after drag to Y=20: EditorHeight=%d, want 20", g.EditorHeight)
	}

	// Release ends the drag; height sticks.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{
		Type:   tea.MouseRelease,
		Action: tea.MouseActionRelease,
		X:      40,
		Y:      20,
	})
	m = out.(Model)
	if m.splitDragging {
		t.Error("splitDragging still set after release")
	}
	if m.workspaceGeom().EditorHeight != 20 {
		t.Errorf("height after release=%d, want 20", m.workspaceGeom().EditorHeight)
	}
}

// Pressing the seam while maximized should exit maximize and adopt the
// current visual height, then resize from there.
func TestSplitDragExitsMaximize(t *testing.T) {
	m := newFocusModel()
	m.editorMaximized = true
	m.layoutWorkspace()
	g := m.workspaceGeom()
	maxH := g.EditorHeight
	if maxH <= defaultEditorHeight {
		t.Fatalf("precondition: maximized height %d should exceed default", maxH)
	}

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionPress,
		X:      40,
		Y:      g.ResultsTop,
	})
	m = out.(Model)
	if m.editorMaximized {
		t.Error("expected maximize cleared on split drag")
	}
	if !m.splitDragging {
		t.Fatal("expected splitDragging")
	}
	// Height should still be the former maximized size (no jump on press).
	if m.workspaceGeom().EditorHeight != maxH {
		t.Errorf("EditorHeight=%d, want adopted maximized %d", m.workspaceGeom().EditorHeight, maxH)
	}
}

// A click in the editor body (not the seam) must not start a split drag.
func TestSplitDragIgnoresEditorBody(t *testing.T) {
	m := newFocusModel()
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionPress,
		X:      40,
		Y:      6,
	})
	m = out.(Model)
	if m.splitDragging {
		t.Error("split drag started from editor body click")
	}
	if m.focus != FocusEditor {
		t.Errorf("focus=%v, want FocusEditor", m.focus)
	}
}

// onEditorResultsSplit hits both the editor bottom border and results top.
func TestOnEditorResultsSplit(t *testing.T) {
	m := newFocusModel()
	g := m.workspaceGeom()
	if !m.onEditorResultsSplit(40, g.ResultsTop-1, g) {
		t.Error("expected hit on editor bottom border")
	}
	if !m.onEditorResultsSplit(40, g.ResultsTop, g) {
		t.Error("expected hit on results top border")
	}
	if m.onEditorResultsSplit(40, g.ResultsTop-2, g) {
		t.Error("editor body should not be the seam")
	}
	if m.onEditorResultsSplit(40, g.ResultsTop+1, g) {
		t.Error("results body should not be the seam")
	}
	if m.onEditorResultsSplit(5, g.ResultsTop, g) {
		t.Error("sidebar x should not be the seam")
	}

	m.inspector.Toggle()
	g = m.workspaceGeom()
	if m.onEditorResultsSplit(g.EditorRight, g.ResultsTop, g) {
		t.Error("seam hit should fail at EditorRight (exclusive)")
	}
	if !m.onEditorResultsSplit(g.EditorRight-1, g.ResultsTop, g) {
		t.Error("seam hit should succeed just inside EditorRight")
	}
}

// Dragging the sidebar↔centre seam should resize the sidebar.
func TestSidebarDragResizes(t *testing.T) {
	m := newFocusModel() // 120×40, default sidebar 30
	g := m.workspaceGeom()
	if g.SidebarWidth != defaultSidebarWidth {
		t.Fatalf("precondition: SidebarWidth=%d", g.SidebarWidth)
	}

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionPress,
		X:      g.SidebarWidth,
		Y:      10,
	})
	m = out.(Model)
	if !m.sidebarDragging {
		t.Fatal("expected sidebarDragging after press on seam")
	}

	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionMotion,
		X:      45,
		Y:      10,
	})
	m = out.(Model)
	g = m.workspaceGeom()
	if g.SidebarWidth != 45 {
		t.Errorf("after drag to X=45: SidebarWidth=%d, want 45", g.SidebarWidth)
	}

	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{
		Type:   tea.MouseRelease,
		Action: tea.MouseActionRelease,
		X:      45,
		Y:      10,
	})
	m = out.(Model)
	if m.sidebarDragging {
		t.Error("sidebarDragging still set after release")
	}
	if m.workspaceGeom().SidebarWidth != 45 {
		t.Errorf("width after release=%d, want 45", m.workspaceGeom().SidebarWidth)
	}
}

func TestOnSidebarSplit(t *testing.T) {
	m := newFocusModel()
	g := m.workspaceGeom()
	if !m.onSidebarSplit(g.SidebarWidth-1, 5, g) {
		t.Error("expected hit on sidebar right border")
	}
	if !m.onSidebarSplit(g.SidebarWidth, 5, g) {
		t.Error("expected hit on centre left border")
	}
	if m.onSidebarSplit(g.SidebarWidth-2, 5, g) {
		t.Error("sidebar body should not be the seam")
	}
	if m.onSidebarSplit(g.SidebarWidth+1, 5, g) {
		t.Error("centre body should not be the seam")
	}
	if m.onSidebarSplit(g.SidebarWidth, g.ResultsBottom+1, g) {
		t.Error("below panel area should not be the seam")
	}
}
