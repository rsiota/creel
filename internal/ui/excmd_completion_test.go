package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestExCompletionShowsAllOnOpen: typing ":" alone lists every command — the
// core discoverability win. Each command contributes exactly one row even if
// several of its aliases would match the empty prefix.
func TestExCompletionShowsAllOnOpen(t *testing.T) {
	var ex exCmd
	ex.Open()
	if len(ex.comp) != len(exCommands()) {
		t.Errorf("after Open, comp has %d items, want all %d commands",
			len(ex.comp), len(exCommands()))
	}
}

func TestExCompletionFiltersByPrefix(t *testing.T) {
	var ex exCmd

	// "g" matches only goto (verbs goto/gt) → one row, still shown.
	ex.input = "g"
	ex.recomputeCompletion()
	if len(ex.comp) != 1 || ex.comp[0].verb != "goto" {
		t.Errorf("prefix %q -> comp=%+v, want [goto]", "g", ex.comp)
	}

	// Fully typing the canonical verb hides the popup.
	ex.input = "goto"
	ex.recomputeCompletion()
	if len(ex.comp) != 0 {
		t.Errorf("exact %q should hide popup, got %+v", "goto", ex.comp)
	}

	// Moving past the verb (a space) hides it too.
	ex.input = "goto "
	ex.recomputeCompletion()
	if len(ex.comp) != 0 {
		t.Errorf("past the verb should hide popup, got %+v", ex.comp)
	}

	// An unknown prefix yields no matches (and no popup).
	ex.input = "zzz"
	ex.recomputeCompletion()
	if len(ex.comp) != 0 {
		t.Errorf("unknown prefix should yield no matches, got %+v", ex.comp)
	}
}

// TestExCompletionTabCompletes: Tab fills the verb from the top match and then
// hides the popup (the verb is now exact).
func TestExCompletionTabCompletes(t *testing.T) {
	m := &Model{}
	m.ex.Open()
	for _, r := range "g" {
		m.handleExKey(runeKey(r))
	}
	if m.ex.input != "g" {
		t.Fatalf("after typing 'g', input=%q", m.ex.input)
	}
	m.handleExKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.ex.input != "goto" {
		t.Errorf("Tab -> input=%q, want goto", m.ex.input)
	}
	if len(m.ex.comp) != 0 {
		t.Errorf("Tab should hide popup after exact match, got %+v", m.ex.comp)
	}
}

func TestExCompletionViewEmpty(t *testing.T) {
	var ex exCmd
	if ex.completionView() != "" {
		t.Error("completionView should be empty when not visible")
	}
	ex.visible = true
	ex.comp = nil
	if ex.completionView() != "" {
		t.Error("completionView should be empty with no candidates")
	}
}

// TestExCompletionViewRendersUsage: the popup contains the canonical command
// name (the blue command column) so the Tab target is readable. The full
// invocation form (with arguments) lives in :help, not the popup.
func TestExCompletionViewRendersUsage(t *testing.T) {
	ex := exCmd{visible: true}
	ex.input = "g"
	ex.recomputeCompletion()
	out := ex.completionView()
	if !strings.Contains(out, ":goto") {
		t.Errorf("completionView missing command name; got %q", out)
	}
	if strings.Contains(out, "<table>") {
		t.Errorf("completionView should not show arg syntax; got %q", out)
	}
}

// TestExCompletionFixedWidth verifies the verb-completion popup renders at a
// constant width regardless of which commands match: every row (including the
// highlight bar) is the same width, so the box doesn't jitter as you type.
func TestExCompletionFixedWidth(t *testing.T) {
	var ex exCmd
	ex.Open() // ":" alone -> every command is a candidate
	out := ex.completionView()
	if out == "" {
		t.Fatal("expected a completion popup for \":\"")
	}
	// Strip the border to measure content rows: drop the first/last line
	// (top/bottom border) and the side border chars from each remaining line.
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("popup too short: %d lines", len(lines))
	}
	body := lines[1 : len(lines)-1]
	wantW := lipgloss.Width(body[0])
	for i, l := range body {
		if w := lipgloss.Width(l); w != wantW {
			t.Errorf("row %d width = %d, want fixed %d", i, w, wantW)
		}
	}

	// Narrow the filter to a few short commands and re-render: the width must
	// not shrink to fit them (the usage column is pinned to the global max).
	var ex2 exCmd
	ex2.Open()
	ex2.input = "co"
	ex2.recomputeCompletion()
	out2 := ex2.completionView()
	if out2 == "" {
		t.Fatal("expected a completion popup for \"co\"")
	}
	lines2 := strings.Split(out2, "\n")
	body2 := lines2[1 : len(lines2)-1]
	if w := lipgloss.Width(body2[0]); w != wantW {
		t.Errorf("narrowed popup width = %d, want fixed %d", w, wantW)
	}
}
