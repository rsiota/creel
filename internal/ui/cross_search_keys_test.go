package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Typing in the cross-table search popup must not be swallowed by the
// workspace "t" new-tab binding (which is active on the sidebar).
func TestCrossSearchTypingDoesNotCreateTab(t *testing.T) {
	m := newWorkspaceModel(t)
	m.focus = FocusConnections
	m.crossSearch.Show()
	before := len(m.resultsTabs)

	m = sendKey(m, runeKey('t'))

	if len(m.resultsTabs) != before {
		t.Fatalf("typing t in cross-search created a tab: %d → %d", before, len(m.resultsTabs))
	}
	if got := m.crossSearch.Query(); got != "t" {
		t.Fatalf("query = %q, want %q", got, "t")
	}
	if !m.crossSearch.IsVisible() {
		t.Fatal("cross-search should stay open while typing")
	}
}

// Long HTML values (with newlines/CRs) must stay on one row and span the
// full inner width of the panel — not a fixed byte cap from search collection.
func TestCrossSearchLongHTMLDoesNotOverflow(t *testing.T) {
	p := NewCrossSearchPanel()
	p.Show()
	p.SetSize(60, 16)
	p.FinishSearch()
	html := "<div class=\"wrap\">\r\n" + strings.Repeat("abcdefghij", 40) + "\r\n</div>"
	p.AddResults([]SearchResult{{
		Table:  "posts",
		Column: "html_content",
		Value:  html,
	}}, 1)

	view := p.View()
	panelW := lipgloss.Width(view)
	innerW := panelW - 2
	foundMatch := false
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > panelW {
			t.Fatalf("line %d width %d > panel %d: %q", i, w, panelW, ansi.Strip(line))
		}
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, "\r") || strings.Contains(line, "\r") {
			t.Fatalf("line %d still contains CR: %q", i, stripped)
		}
		if !strings.Contains(stripped, "posts.html_content") {
			continue
		}
		foundMatch = true
		// Strip border glyphs; remaining content should fill the inner width.
		inner := strings.TrimPrefix(stripped, "│")
		inner = strings.TrimSuffix(inner, "│")
		if lipgloss.Width(inner) != innerW {
			t.Fatalf("match row inner width %d, want %d: %q", lipgloss.Width(inner), innerW, inner)
		}
		if !strings.Contains(inner, "…") {
			t.Fatalf("expected truncated value with ellipsis: %q", inner)
		}
		// Value must extend well past the old 80-byte collection clamp relative
		// to this 60-col panel (inner 58); text before ellipsis should use most
		// of the row after the label.
		trimmed := strings.TrimRight(inner, " ")
		if lipgloss.Width(trimmed) < innerW-1 {
			t.Fatalf("value clamped early: textW=%d innerW=%d %q", lipgloss.Width(trimmed), innerW, trimmed)
		}
	}
	if !foundMatch {
		t.Fatalf("match row not found in view=%q", ansi.Strip(view))
	}
}

// Values longer than the old 80-byte search clamp must still fill a wide panel.
func TestCrossSearchValueUsesFullPanelWidth(t *testing.T) {
	p := NewCrossSearchPanel()
	p.Show()
	p.SetSize(120, 16)
	p.FinishSearch()
	p.AddResults([]SearchResult{{
		Table:  "posts",
		Column: "body",
		Value:  strings.Repeat("x", 300),
	}}, 1)

	view := p.View()
	innerW := lipgloss.Width(view) - 2
	for _, line := range strings.Split(view, "\n") {
		st := ansi.Strip(line)
		if !strings.Contains(st, "posts.body") {
			continue
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(st, "│"), "│")
		trimmed := strings.TrimRight(inner, " ")
		// Must show more than 80 value chars (prefix+label+gap ≈ 14).
		if lipgloss.Width(trimmed) <= 80+14 {
			t.Fatalf("still clamped near 80 chars: textW=%d inner=%q", lipgloss.Width(trimmed), trimmed)
		}
		if lipgloss.Width(trimmed) != innerW {
			t.Fatalf("textW=%d, want full inner %d: %q", lipgloss.Width(trimmed), innerW, trimmed)
		}
		return
	}
	t.Fatal("match row not found")
}
