package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestHelpListsExCommands pins that the Commands tab of the "?" overlay folds
// the ":" command registry into its output (title + usages), so the help sheet
// is the single place to discover both keybindings and commands.
func TestHelpListsExCommands(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.page = helpPageCommands // switch to the Commands tab
	h.SetSize(140, 50)
	out := stripAnsi(h.View())
	if !strings.Contains(out, "Commands") {
		t.Error("help overlay should include a Commands section")
	}
	for _, want := range []string{":goto <table>", ":export", ":refs", ":begin"} {
		if !strings.Contains(out, want) {
			t.Errorf("help overlay should list %q", want)
		}
	}
}

// TestHelpTabSwitchAndScroll exercises the overlay's navigation: tab switches
// pages (the Keys page shows the "Keybindings" header; the Commands page shows
// usages like ":goto <table>" that the Keys page never has), and scrolling the
// Keys page moves the viewport.
func TestHelpTabSwitchAndScroll(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)

	keys := stripAnsi(h.View())
	if !strings.Contains(keys, "Keybindings") {
		t.Error("Keys page should show the Keybindings header")
	}
	// The Keys page lists bindings, not command usages; ":goto <table>" is a
	// Commands-only usage form. (The "=" binding's description mentions
	// ":goto" but never with a " <table>" argument.)
	if strings.Contains(keys, ":goto <table>") {
		t.Error("command usages should not appear on the Keys page")
	}

	// Tab over to Commands.
	if !h.HandleKey(tea.KeyMsg{Type: tea.KeyTab}) {
		t.Fatal("tab should be consumed to switch help pages")
	}
	if !strings.Contains(stripAnsi(h.View()), ":goto <table>") {
		t.Error("Commands page should list command usages")
	}

	// Back to Keys; confirm scrolling moves the viewport (Global scrolls off).
	h.page = helpPageKeys
	h.keysOff = 0
	if !strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should be visible at the top of the Keys page")
	}
	for i := 0; i < 3; i++ {
		h.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	if strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should have scrolled off the top after paging down")
	}
}

// TestHelpMouseScroll confirms the mouse-wheel scroll path (ScrollBy) moves the
// viewport in both directions and is clamped at the ends.
func TestHelpMouseScroll(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	if !strings.Contains(stripAnsi(h.View()), "Global") {
		t.Fatal("Global should be visible at the top initially")
	}
	h.ScrollBy(1000) // scroll far down
	if strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should scroll off after scrolling down")
	}
	h.ScrollBy(-1000) // back to the top
	if !strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should reappear after scrolling back to the top")
	}
}

// TestHelpCloseKeysDismisses confirms that close keys (esc/?/q) are NOT consumed
// by HandleKey (the caller hides the overlay), while nav keys are consumed.
func TestHelpCloseKeysDismisses(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	for _, k := range []string{"esc", "?", "q"} {
		km := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if k == "esc" {
			km = tea.KeyMsg{Type: tea.KeyEsc}
		}
		if h.HandleKey(km) {
			t.Errorf("close key %q should not be consumed (caller hides)", k)
		}
	}
	// An unmapped key is also not consumed → dismisses (old "any key" feel).
	if h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}) {
		t.Error("unmapped key should not be consumed")
	}
	// Nav keys are consumed.
	if !h.HandleKey(tea.KeyMsg{Type: tea.KeyTab}) {
		t.Error("tab should be consumed")
	}
}

// TestHelpConstantHeightAcrossTabs guards against the panel resizing when the
// page changes (which made the top border jump up/down on tab switch). Both
// pages must render the same number of lines.
func TestHelpConstantHeightAcrossTabs(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	h.page = helpPageKeys
	keysH := lipgloss.Height(h.View())
	h.page = helpPageCommands
	cmdsH := lipgloss.Height(h.View())
	if keysH != cmdsH {
		t.Errorf("help panel height differs across tabs: keys=%d commands=%d (border would jump)",
			keysH, cmdsH)
	}
}

// TestHelpHintsWhenVisible confirms the status bar surfaces help-specific
// keybindings while the overlay is open (instead of the workspace hints).
func TestHelpHintsWhenVisible(t *testing.T) {
	m := Model{help: NewHelpPanel()}
	m.help.Show()
	hints := m.hintList()
	joined := strings.Join(hints, " ")
	if !strings.Contains(joined, "tab") || !strings.Contains(joined, "?") {
		t.Errorf("help-visible hints should mention tab and ?: got %v", hints)
	}
}

// TestHelpTabClickSwitchesPage covers the mouse-click tab hit boxes: a click
// on each tab switches to it, and clicks off the tab row (or off any tab) do
// nothing.
func TestHelpTabClickSwitchesPage(t *testing.T) {
	m := Model{help: NewHelpPanel()}
	m.help.Show()
	m.width = 120
	pl := helpPanelLeft(m.width)

	for _, want := range []int{helpPageKeys, helpPageCommands} {
		// Find an x coordinate that lands on tab `want`.
		x := -1
		for xx := pl; xx < pl+30; xx++ {
			if helpTabAt(pl, xx) == want {
				x = xx
				break
			}
		}
		if x < 0 {
			t.Fatalf("no click x found for tab %d", want)
		}
		mm, _ := m.handleHelpMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: helpTabRow})
		if got := mm.(Model).help.page; got != want {
			t.Errorf("click on tab %d (x=%d) set page=%d", want, x, got)
		}
	}

	// A click off the tab row leaves the page unchanged.
	m.help.page = helpPageKeys
	mm, _ := m.handleHelpMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 5, Y: 0})
	if mm.(Model).help.page != helpPageKeys {
		t.Error("click off the tab row should not switch pages")
	}
	// A click on the tab row but past the tabs also does nothing.
	mm, _ = m.handleHelpMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 200, Y: helpTabRow})
	if mm.(Model).help.page != helpPageKeys {
		t.Error("click past the tabs should not switch pages")
	}
}
