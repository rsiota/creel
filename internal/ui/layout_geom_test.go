package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func newGeomModel(w, h int) Model {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = mm.(Model)
	m.state = stateWorkspace
	return m
}

func TestWorkspaceGeomDefaults(t *testing.T) {
	m := newGeomModel(120, 40)
	g := m.workspaceGeom()
	if g.SidebarWidth != defaultSidebarWidth {
		t.Errorf("SidebarWidth=%d, want %d", g.SidebarWidth, defaultSidebarWidth)
	}
	if g.EditorHeight != defaultEditorHeight {
		t.Errorf("EditorHeight=%d, want %d", g.EditorHeight, defaultEditorHeight)
	}
	if g.ResultsTop != g.EditorHeight {
		t.Errorf("ResultsTop=%d, want EditorHeight=%d", g.ResultsTop, g.EditorHeight)
	}
	if g.EditorRight != m.width {
		t.Errorf("EditorRight=%d, want width=%d (no right slot)", g.EditorRight, m.width)
	}
	// resultsHeight = 40 - 12 - 1 - 0 - 2 = 25
	if g.ResultsHeight != 25 {
		t.Errorf("ResultsHeight=%d, want 25", g.ResultsHeight)
	}
	if g.RightWidth != 120-30-2 {
		t.Errorf("RightWidth=%d, want %d", g.RightWidth, 120-30-2)
	}
}

func TestWorkspaceGeomEditorSplit(t *testing.T) {
	m := newGeomModel(120, 40)
	m.editorSplitH = 20
	g := m.workspaceGeom()
	if g.EditorHeight != 20 {
		t.Errorf("EditorHeight=%d, want 20", g.EditorHeight)
	}
	if g.ResultsHeight != 40-20-1-2 {
		t.Errorf("ResultsHeight=%d, want %d", g.ResultsHeight, 40-20-1-2)
	}
}

func TestWorkspaceGeomClampsEditorSplit(t *testing.T) {
	m := newGeomModel(120, 40)
	m.editorSplitH = 2 // below min
	g := m.workspaceGeom()
	if g.EditorHeight != minEditorHeight {
		t.Errorf("EditorHeight=%d, want min %d", g.EditorHeight, minEditorHeight)
	}

	m.editorSplitH = 1000 // would starve results
	g = m.workspaceGeom()
	maxH := 40 - workspaceStatusH - workspaceBorderOH - minResultsHeight
	if g.EditorHeight != maxH {
		t.Errorf("EditorHeight=%d, want max %d", g.EditorHeight, maxH)
	}
	if g.ResultsHeight < minResultsHeight {
		t.Errorf("ResultsHeight=%d, want >= %d", g.ResultsHeight, minResultsHeight)
	}
}

func TestWorkspaceGeomMaximized(t *testing.T) {
	m := newGeomModel(120, 40)
	m.editorSplitH = 12
	m.editorMaximized = true
	g := m.workspaceGeom()
	// avail - 12 = 40 - 1 - 2 - 12 = 25
	if g.EditorHeight != 25 {
		t.Errorf("maximized EditorHeight=%d, want 25", g.EditorHeight)
	}
	m.editorMaximized = false
	g = m.workspaceGeom()
	if g.EditorHeight != 12 {
		t.Errorf("restored EditorHeight=%d, want stored 12", g.EditorHeight)
	}
}

func TestWorkspaceGeomRightSlot(t *testing.T) {
	m := newGeomModel(120, 40)
	m.inspector.Toggle()
	g := m.workspaceGeom()
	if g.RightSlotW != InspectorWidth {
		t.Errorf("RightSlotW=%d, want %d", g.RightSlotW, InspectorWidth)
	}
	if g.EditorRight != 120-InspectorWidth {
		t.Errorf("EditorRight=%d, want %d", g.EditorRight, 120-InspectorWidth)
	}
	if g.RightWidth != 120-30-2-InspectorWidth {
		t.Errorf("RightWidth=%d, want %d", g.RightWidth, 120-30-2-InspectorWidth)
	}
}

func TestWorkspaceGeomCmdLineShrinksResults(t *testing.T) {
	m := newGeomModel(120, 40)
	base := m.workspaceGeom().ResultsHeight
	m.ex.visible = true
	g := m.workspaceGeom()
	if g.CmdHeight != 1 {
		t.Fatalf("CmdHeight=%d, want 1", g.CmdHeight)
	}
	if g.ResultsHeight != base-1 {
		t.Errorf("ResultsHeight=%d, want %d with cmd line", g.ResultsHeight, base-1)
	}
}

func TestWorkspaceGeomSidebarSplit(t *testing.T) {
	m := newGeomModel(120, 40)
	m.sidebarSplitW = 40
	g := m.workspaceGeom()
	if g.SidebarWidth != 40 {
		t.Errorf("SidebarWidth=%d, want 40", g.SidebarWidth)
	}
	if g.RightWidth != 120-40-2 {
		t.Errorf("RightWidth=%d, want %d", g.RightWidth, 120-40-2)
	}
}

func TestWorkspaceGeomClampsSidebarSplit(t *testing.T) {
	m := newGeomModel(120, 40)
	m.sidebarSplitW = 2
	g := m.workspaceGeom()
	if g.SidebarWidth != minSidebarWidth {
		t.Errorf("SidebarWidth=%d, want min %d", g.SidebarWidth, minSidebarWidth)
	}

	m.sidebarSplitW = 1000
	g = m.workspaceGeom()
	maxW := 120 - workspaceBorderOH - minCenterWidth
	if g.SidebarWidth != maxW {
		t.Errorf("SidebarWidth=%d, want max %d", g.SidebarWidth, maxW)
	}
	if g.RightWidth < minCenterWidth {
		t.Errorf("RightWidth=%d, want >= %d", g.RightWidth, minCenterWidth)
	}
}
