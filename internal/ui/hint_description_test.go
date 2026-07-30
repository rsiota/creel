package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
)

// Pressing a hint key stages its registry description, which renders next to
// the hint line on the status bar.
func TestHintDescriptionShownOnPress(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusResults

	// "j" is a Results hint ("move cursor"); it should stage the description.
	mm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = mm2.(Model)
	if m.hintDesc != "move cursor" {
		t.Fatalf("hintDesc = %q, want %q", m.hintDesc, "move cursor")
	}

	bar := stripANSI(m.statusBar(""))
	if !strings.Contains(bar, "move cursor") {
		t.Errorf("status bar missing description after press:\n%s", bar)
	}
}

// The description disappears once it has aged past hintDescDuration at render.
func TestHintDescriptionExpires(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusResults

	mm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = mm2.(Model)
	m.hintDescAt = m.hintDescAt.Add(-(hintDescDuration + 1)) // force expiry

	bar := stripANSI(m.statusBar(""))
	if strings.Contains(bar, "move cursor") {
		t.Errorf("expired description still shown:\n%s", bar)
	}
}

// A key that matches a hint but whose context has no registry section (e.g. the
// confirm-dialog "y"/"n") stages no description, but still flashes the key.
func TestHintDescriptionNoneForInlineContext(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.dropTableConfirm = "tbl" // inline "enter"/"esc" context, no section

	// "enter" matches the hint list but hintSection() == "" → no description.
	if got := m.hintSection(); got != "" {
		t.Fatalf("hintSection = %q, want empty for inline context", got)
	}
	mm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	m = mm2.(Model)
	if m.hintDesc != "" {
		t.Errorf("hintDesc = %q, want empty (no registry section)", m.hintDesc)
	}
	if m.hintFlash != "enter" {
		t.Errorf("hintFlash = %q, want enter (key still flashes)", m.hintFlash)
	}
}

// A non-matching key clears any staged description.
func TestHintDescriptionClearedOnUnrelatedKey(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusResults

	mm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = mm2.(Model)
	if m.hintDesc == "" {
		t.Fatal("expected description staged after j")
	}
	// Press a key not in the Results hint list (e.g. "z").
	mm3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = mm3.(Model)
	if m.hintDesc != "" {
		t.Errorf("hintDesc = %q, want cleared after non-matching key", m.hintDesc)
	}
}

// hintDescription resolves a hint key to its section binding's description.
func TestHintDescriptionLookup(t *testing.T) {
	m := NewModel(&config.Config{})
	m.state = stateWorkspace
	m.focus = FocusResults
	cases := map[string]string{
		"j":      "move cursor",
		"ctrl+s": "save edits",
		"V":      "visual mode (select range)",
	}
	for key, want := range cases {
		if got := m.hintDescription(key); got != want {
			t.Errorf("hintDescription(%q) = %q, want %q", key, got, want)
		}
	}
}
