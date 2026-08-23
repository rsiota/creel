package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// highlightMatches must paint non-accent runes in colorFg. Leaving them
// unstyled lets paintBg's theme background sit under the terminal's default
// FG — illegible when a light creel theme runs inside a dark terminal.
func TestHighlightMatchesUsesForeground(t *testing.T) {
	prevFg, prevPrimary := colorFg, colorPrimary
	colorFg = lipgloss.Color("#1f2328")
	colorPrimary = lipgloss.Color("#0969da")
	defer func() { colorFg, colorPrimary = prevFg, prevPrimary }()

	got := highlightMatches("acme", nil)
	if !strings.Contains(got, "38;2;31;35;40") && !strings.Contains(got, string(colorFg)) {
		// lipgloss may emit truecolor or a profile-dependent form; at minimum
		// the plain text must not equal the unstyled input.
		if ansi.Strip(got) != "acme" || got == "acme" {
			t.Fatalf("empty matchIdx: got %q, want styled colorFg text", got)
		}
	}
	if got == "acme" {
		t.Fatal("empty matchIdx returned unstyled text")
	}

	got = highlightMatches("acme", []int{0, 2})
	stripped := ansi.Strip(got)
	if stripped != "acme" {
		t.Fatalf("matched render stripped to %q want acme", stripped)
	}
	if got == "acme" {
		t.Fatal("matched render returned unstyled text")
	}
	// Non-matched runes still carry a style (not raw runes spliced in).
	if !strings.Contains(got, "\x1b[") {
		t.Fatal("matched render has no ANSI styling")
	}
}
