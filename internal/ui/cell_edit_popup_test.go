package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The popup should show an underline insert cursor (matching the rest of the
// app), never the bubbles textarea's default reverse-video block. The textarea
// cursor alternates with blink state, so force it visible (Blink=false ⇒ the
// reverse marker is emitted) to make the assertion deterministic.
func TestCellEditPopupUnderlineCursorPlainText(t *testing.T) {
	p := NewCellEditPopup()
	p.Show("hello world", 0, 0, "name")
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
	p.Show(`{"a": 1}`, 0, 0, "data")
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
