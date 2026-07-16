package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
