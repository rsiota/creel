package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

// Each newly-sectioned context resolves to its registry section and yields
// descriptions for its keys.
func TestNewHintSections(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.state = stateWorkspace

	check := func(section string, samples map[string]string) {
		t.Helper()
		if got := m.hintSection(); got != section {
			t.Errorf("section = %q, want %q", got, section)
		}
		for key, want := range samples {
			if got := m.hintDescription(key); got != want {
				t.Errorf("%s: hintDescription(%q) = %q, want %q", section, key, got, want)
			}
		}
	}

	m.help.Show()
	check("Help", map[string]string{
		"tab": "switch page", "/": "search", "n": "next / prev match", "?": "close",
	})
	m.help.Hide()

	m.crossSearch.Show()
	check("Cross-Table Search", map[string]string{
		"enter": "search / open result", "esc": "close",
	})
	m.crossSearch.Hide()

	m.explainPanel.visible = true // avoid building a db.Result just to flip visibility
	check("Explain Panel", map[string]string{
		"j": "scroll", "G": "top / bottom", "ctrl+d": "page down / up", "esc": "close",
	})
	m.explainPanel.visible = false

	m.diffPanel.visible = true
	check("Diff Panel", map[string]string{
		"j": "scroll", "a": "toggle changes-only / all rows", "esc": "close",
	})
	m.diffPanel.visible = false

	m.lookupPanel.visible = true
	check("Lookup Panel", map[string]string{
		"j": "move", "G": "top / bottom", "ctrl+u": "page down / up", "esc": "close",
	})
	m.lookupPanel.visible = false

	m.focus = FocusTabBar
	check("Tab Bar", map[string]string{
		"h": "switch tab", "l": "switch tab", "t": "new tab", "enter": "focus editor",
	})

	// Connections list (pre-workspace screen).
	m.state = stateConnections
	m.focus = FocusConnections
	check("Connections", map[string]string{
		"enter": "connect", "n": "new connection", "e": "edit connection",
		"d": "delete connection", "/": "filter connections",
	})
}
