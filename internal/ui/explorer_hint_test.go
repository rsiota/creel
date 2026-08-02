package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

// The docked relationship explorer (g r) resolves to its own registry section
// so pressed keys get descriptions.
func TestExplorerHintSectionAndDescription(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusExplorer
	m.explorer.ShowDocked()

	if got := m.hintSection(); got != "Relationship Explorer" {
		t.Fatalf("hintSection = %q, want Relationship Explorer", got)
	}
	cases := map[string]string{
		"j":      "move",
		"h":      "collapse / expand",
		"enter":  "re-root grid on node",
		"r":      "retarget / refresh",
		"esc":    "close",
		"G":      "top / bottom",
		"ctrl+d": "page down / up",
	}
	for key, want := range cases {
		if got := m.hintDescription(key); got != want {
			t.Errorf("hintDescription(%q) = %q, want %q", key, got, want)
		}
	}
}

// Pressing an explorer key stages the description on the status bar.
func TestExplorerDescriptionShownOnPress(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusExplorer
	m.explorer.ShowDocked()

	mm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = mm2.(Model)
	if m.hintDesc != "move" {
		t.Fatalf("hintDesc = %q, want %q", m.hintDesc, "move")
	}
	bar := stripANSI(m.statusBar(""))
	if !strings.Contains(bar, "move") {
		t.Errorf("status bar missing description after press:\n%s", bar)
	}
}
