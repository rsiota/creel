package ui

import (
	"testing"
)

func TestZenEnterShowsResultsOnly(t *testing.T) {
	m := workspaceModel(t)
	m.sidebarVisible = true
	m.editorVisible = true
	m.inspector.Show()

	if err := m.enterZen(); err != "" {
		t.Fatalf("enterZen: %s", err)
	}
	if !m.zenActive {
		t.Fatal("zen should be active")
	}
	g := m.workspaceGeom()
	if g.SidebarWidth != 0 || g.EditorHeight != 0 {
		t.Fatalf("zen layout should hide sidebar and editor: sidebar=%d editor=%d",
			g.SidebarWidth, g.EditorHeight)
	}
	if m.inspector.IsVisible() || m.assistant.IsVisible() {
		t.Fatal("zen should hide right-slot panels")
	}
	if m.focus != FocusResults {
		t.Errorf("focus = %v, want results", m.focus)
	}
}

func TestZenExitRestoresLayout(t *testing.T) {
	m := workspaceModel(t)
	m.sidebarVisible = false
	m.editorVisible = true
	m.inspector.Show()
	m.focus = FocusInspector

	if err := m.enterZen(); err != "" {
		t.Fatalf("enterZen: %s", err)
	}
	m.exitZen()

	if m.zenActive {
		t.Fatal("zen should be inactive")
	}
	if m.sidebarVisible {
		t.Error("sidebar should restore hidden")
	}
	if !m.editorVisible {
		t.Error("editor should restore visible")
	}
	if !m.inspector.IsVisible() {
		t.Error("inspector should restore open")
	}
	if m.focus != FocusInspector {
		t.Errorf("focus = %v, want inspector", m.focus)
	}
}

func TestZenToggleAndOff(t *testing.T) {
	m := workspaceModel(t)
	if msg := m.toggleZen(); msg != "" {
		t.Fatalf("toggleZen: %s", msg)
	}
	if !m.zenActive {
		t.Fatal("first toggle should enter zen")
	}
	m.toggleZen()
	if m.zenActive {
		t.Fatal("second toggle should exit zen")
	}

	m.toggleZen()
	m.exZen([]string{"off"})
	if m.zenActive {
		t.Fatal(":zen off should exit zen")
	}
}

func TestZenRestoresChart(t *testing.T) {
	m := workspaceModel(t)
	m.chartPanel.ShowBar("bar · a", []chartBar{{label: "x", value: 1, n: 1}}, 0, barAggCount)
	if err := m.enterZen(); err != "" {
		t.Fatalf("enterZen: %s", err)
	}
	if m.chartPanel.IsVisible() {
		t.Fatal("chart should hide in zen")
	}
	m.exitZen()
	if !m.chartPanel.IsVisible() {
		t.Fatal("chart should restore after zen")
	}
}

func TestZenBlockedWhileEditing(t *testing.T) {
	m := workspaceModel(t)
	m.results.editing = true
	if msg := m.enterZen(); msg == "" {
		t.Fatal("enterZen should fail while editing")
	}
}

func TestClearZenStateOnManualToggle(t *testing.T) {
	m := workspaceModel(t)
	m.inspector.Show()
	if err := m.enterZen(); err != "" {
		t.Fatalf("enterZen: %s", err)
	}
	m.toggleSidebar()
	if m.zenActive {
		t.Fatal("manual toggle should clear zen without restore")
	}
	if m.inspector.IsVisible() {
		t.Error("inspector should stay hidden after sidebar toggle from zen")
	}
}
