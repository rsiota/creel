package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// hasUnderline reports whether s carries an underline SGR attribute. The plain
// colorFg style emits "\x1b[96m" (no "4"); the cursor style emits something
// like "\x1b[4;96;4m", which contains ";4m".
func hasUnderline(s string) bool {
	return strings.Contains(s, "\x1b[4m") || strings.Contains(s, ";4m")
}

// renderEditInput must fill exactly `width` cells regardless of value length or
// cursor position. The old bar-glyph implementation rendered width-1 in insert
// mode (the bar consumed a column), which shifted text by one position on a
// mode switch; this guards against that regression.
func TestRenderEditInputFillsExactWidth(t *testing.T) {
	cases := []struct {
		name  string
		value string
		width int
	}{
		{"short", "hi", 10},
		{"exact", "0123456789", 10},
		{"overflow", "abcdefghijklmnopqrstuvwxyz", 10},
		{"empty", "", 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ti := textinput.New()
			ti.SetValue(c.value) // cursor at end
			out := renderEditInput(ti, c.width, colorFg)
			if w := lipgloss.Width(out); w != c.width {
				t.Errorf("%s: width=%d want %d (rendered=%q)", c.name, w, c.width, out)
			}
			if strings.Contains(formStripANSI(out), "▏") {
				t.Errorf("%s: bar glyph present in output", c.name)
			}
		})
	}
}

// The cursor overlays the character at the position with an underline (a space
// at the end), and the text stays fully visible and width-stable as it moves.
func TestRenderEditInputUnderlineCursor(t *testing.T) {
	ti := textinput.New()
	ti.Focus()
	ti.SetValue("hello") // cursor at end (position 5) → underline on trailing space

	out := renderEditInput(ti, 10, colorFg)
	if !hasUnderline(out) {
		t.Errorf("cursor at end missing underline: %q", out)
	}
	if w := lipgloss.Width(out); w != 10 {
		t.Errorf("width at end=%d want 10", w)
	}

	// Move the cursor left twice → position 3, overlays the second 'l'.
	ti, _ = ti.Update(tea.KeyMsg{Type: tea.KeyLeft})
	ti, _ = ti.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if pos := ti.Position(); pos != 3 {
		t.Fatalf("cursor position=%d want 3", pos)
	}

	out = renderEditInput(ti, 10, colorFg)
	if !hasUnderline(out) {
		t.Errorf("mid-string cursor missing underline: %q", out)
	}
	if w := lipgloss.Width(out); w != 10 {
		t.Errorf("mid-string width=%d want 10", w)
	}
	// The whole value is still visible (the cursor overlays, not replaces).
	if plain := formStripANSI(out); !strings.HasPrefix(plain, "hello") {
		t.Errorf("mid-string text not fully visible: %q", plain)
	}
}
