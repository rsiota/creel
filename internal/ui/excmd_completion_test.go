package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

// TestExCompletionViewRendersUsage: the popup contains the canonical usage
// text (the blue command column) so the Tab target is readable.
func TestExCompletionViewRendersUsage(t *testing.T) {
	ex := exCmd{visible: true}
	ex.input = "g"
	ex.recomputeCompletion()
	out := ex.completionView()
	if !strings.Contains(out, ":goto <table>") {
		t.Errorf("completionView missing usage text; got %q", out)
	}
}
