package ui

import (
	"testing"

	"github.com/rsiota/creel/internal/config"
)

func workspaceModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(&config.Config{})
	m.state = stateWorkspace
	m.width = 120
	m.height = 40
	m.layoutWorkspace()
	return m
}

func TestToggleSidebarHidesAndShows(t *testing.T) {
	m := workspaceModel(t)
	m.sidebarSplitW = 36
	m.layoutWorkspace()
	if !m.sidebarVisible {
		t.Fatal("sidebar should start visible")
	}
	g0 := m.workspaceGeom()
	if g0.SidebarWidth <= 0 {
		t.Fatalf("SidebarWidth=%d, want >0", g0.SidebarWidth)
	}

	m.toggleSidebar()
	if m.sidebarVisible {
		t.Fatal("sidebar should be hidden")
	}
	if g := m.workspaceGeom(); g.SidebarWidth != 0 {
		t.Errorf("hidden SidebarWidth=%d, want 0", g.SidebarWidth)
	}
	if m.sidebarSplitW <= 0 {
		t.Error("split width should be preserved while hidden")
	}

	m.toggleSidebar()
	if !m.sidebarVisible {
		t.Fatal("sidebar should be visible again")
	}
	if g := m.workspaceGeom(); g.SidebarWidth <= 0 {
		t.Errorf("restored SidebarWidth=%d, want >0", g.SidebarWidth)
	}
}

func TestToggleEditorExpandsResults(t *testing.T) {
	m := workspaceModel(t)
	m.editorMaximized = false
	m.editorSplitH = 12
	m.layoutWorkspace()
	before := m.workspaceGeom()

	m.toggleEditor()
	if m.editorVisible {
		t.Fatal("editor should be hidden")
	}
	if m.editorMaximized {
		t.Fatal("maximize should clear when editor is hidden")
	}
	after := m.workspaceGeom()
	if after.EditorHeight != 0 {
		t.Errorf("EditorHeight=%d, want 0 when hidden", after.EditorHeight)
	}
	if after.ResultsHeight <= before.ResultsHeight {
		t.Errorf("ResultsHeight=%d should grow from %d", after.ResultsHeight, before.ResultsHeight)
	}
}

func TestToggleSidebarMovesFocusOffSidebar(t *testing.T) {
	m := workspaceModel(t)
	m.focus = FocusConnections
	m.toggleSidebar()
	if m.focus == FocusConnections {
		t.Error("focus should leave hidden sidebar")
	}
}

func TestToggleEditorMovesFocusOffEditor(t *testing.T) {
	m := workspaceModel(t)
	m.focus = FocusEditor
	m.toggleEditor()
	if m.focus == FocusEditor {
		t.Error("focus should leave hidden editor")
	}
	if m.focus != FocusResults {
		t.Errorf("focus = %v, want results", m.focus)
	}
}

func TestSessionLayoutPanelVisibilityRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	conn := newNamedSQLiteConn(t, "panels")

	m := workspaceModel(t)
	m.connection = conn
	m.sidebarVisible = false
	m.editorVisible = false
	m.saveSession()

	m2 := NewModel(&config.Config{})
	m2.connection = conn
	m2.restoreSession()
	if m2.sidebarVisible {
		t.Error("sidebar should restore hidden")
	}
	if m2.editorVisible {
		t.Error("editor should restore hidden")
	}
}
