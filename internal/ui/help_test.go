package ui

import (
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/config"
)

func TestHelpPanelRender(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	out := h.View()
	if out == "" {
		t.Fatal("help panel rendered empty")
	}
	for _, want := range []string{"Keybindings", "Global", "Sidebar", "Editor", "Results", "Inspector", "? or esc"} {
		if !strings.Contains(out, want) {
			t.Errorf("help panel missing %q", want)
		}
	}
}

func TestHelpPanelToggle(t *testing.T) {
	h := NewHelpPanel()
	if h.IsVisible() {
		t.Fatal("help panel should start hidden")
	}
	h.Show()
	if !h.IsVisible() {
		t.Fatal("help panel should be visible after Show")
	}
	h.Hide()
	if h.IsVisible() {
		t.Fatal("help panel should be hidden after Hide")
	}
	h.Toggle()
	if !h.IsVisible() {
		t.Fatal("help panel should be visible after Toggle")
	}
}

func TestStatusBarRender(t *testing.T) {
	m := NewModel(&config.Config{})
	m.width = 120
	m.height = 40
	out := m.statusBar("test-conn")
	if out == "" {
		t.Fatal("status bar empty")
	}
	if !strings.Contains(out, "?") {
		t.Error("status bar should contain help hint")
	}
}
