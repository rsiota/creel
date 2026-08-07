package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The popup should show an underline insert cursor (matching the rest of the
// app), never the bubbles textarea's default reverse-video block. The textarea
// cursor alternates with blink state, so force it visible (Blink=false ⇒ the
// reverse marker is emitted) to make the assertion deterministic.
func TestCellEditPopupUnderlineCursorPlainText(t *testing.T) {
	p := NewCellEditPopup()
	p.Show("hello world", 0, 0, "name", false)
	p.ta.Focus()
	p.ta.Cursor.Blink = false // cursor "on" ⇒ ta.View() emits the reverse marker

	raw := p.View()
	if strings.Contains(raw, "\x1b[7m") {
		t.Errorf("plain-text popup still renders a reverse cursor")
	}
	if !hasUnderline(raw) {
		t.Errorf("plain-text popup missing underline insert cursor")
	}
}

// JSON mode renders the cursor itself (independent of blink state), so it must
// use the underline style directly.
func TestCellEditPopupUnderlineCursorJSON(t *testing.T) {
	p := NewCellEditPopup()
	p.Show(`{"a": 1}`, 0, 0, "data", false)
	if !p.jsonMode {
		t.Fatal("expected JSON mode for an object value")
	}
	p.ta.Focus()

	raw := p.View()
	if strings.Contains(raw, "\x1b[7m") {
		t.Errorf("JSON popup still renders a reverse cursor")
	}
	if !hasUnderline(raw) {
		t.Errorf("JSON popup missing underline insert cursor")
	}
}

// sgrPrefix must yield exactly the style's opening SGR (no content, no reset).
func TestSGRPrefix(t *testing.T) {
	got := sgrPrefix(lipgloss.NewStyle().Foreground(colorFg).Underline(true))
	if !strings.HasPrefix(got, "\x1b[") || !strings.HasSuffix(got, "m") {
		t.Errorf("sgrPrefix not a single SGR sequence: %q", got)
	}
	if strings.Contains(got, "\x00") {
		t.Errorf("sgrPrefix leaked the NUL marker: %q", got)
	}
}

// TestCellEditPopupReadOnlyView verifies the view-only popup renders the value
// statically with a "(read-only)" marker, preserves real newlines (the grid
// flattens them), and pages through content that's taller than the viewport.
func TestCellEditPopupReadOnlyView(t *testing.T) {
	p := NewCellEditPopup()
	// Five lines in a viewport three tall → scrollable by two lines.
	p.Show("alpha\nbeta\ngamma\ndelta\nepsilon", 0, 0, "body", true)
	p.SetMaxSize(60, 3)
	if !p.IsReadOnly() {
		t.Fatal("popup should report read-only after Show(..., true)")
	}

	view := p.View()
	if !strings.Contains(view, "(read-only)") {
		t.Errorf("read-only popup missing the (read-only) marker")
	}
	if !strings.Contains(view, "alpha") || !strings.Contains(view, "beta") {
		t.Errorf("read-only popup should render the first window of lines")
	}
	if strings.Contains(view, "epsilon") {
		t.Errorf("fifth line should be scrolled out of the initial window")
	}

	// Jump to the end: the top line scrolls out and the last line appears.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	view = p.View()
	if strings.Contains(view, "alpha") {
		t.Errorf("after scrolling to the end, the first line should be out of view")
	}
	if !strings.Contains(view, "epsilon") {
		t.Errorf("after scrolling to the end, the last line should be visible")
	}

	// Jump back home.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !strings.Contains(p.View(), "alpha") {
		t.Errorf("after scrolling home, the first line should be visible again")
	}
}
