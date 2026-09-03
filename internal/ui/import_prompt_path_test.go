package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestImportPromptEnterAcceptsPathCompletion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dump.sql"), []byte("select 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewImportPrompt()
	p.Show("")
	prefix := dir + "/d"
	for _, ch := range prefix {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if !p.pathComp.hasChoices() {
		t.Fatalf("expected path completions for %q", p.input.Value())
	}

	if !p.AcceptPathCompletion() {
		t.Fatal("AcceptPathCompletion should succeed with open dropdown")
	}
	want := filepath.Join(dir, "dump.sql")
	if got := p.input.Value(); got != want {
		t.Fatalf("after accept: value = %q, want %q", got, want)
	}
	if !p.IsVisible() {
		t.Fatal("accepting completion must not hide the import prompt")
	}
	// File accept clears the dropdown so a following Enter can submit.
	if p.pathComp.hasChoices() {
		t.Fatal("after accepting a file, completions should clear")
	}
}

func TestImportPromptEnterSubmitsWhenPathAlreadyComplete(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dump.sql"), []byte("select 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewImportPrompt()
	p.Show("")
	full := filepath.Join(dir, "dump.sql")
	for _, ch := range full {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	// Exact basename still prefixes itself, so the dropdown can reopen —
	// but Accept must yield so Enter submits the import.
	if !p.pathComp.hasChoices() {
		t.Fatalf("expected a self-prefix match for %q", p.input.Value())
	}
	if p.AcceptPathCompletion() {
		t.Fatal("AcceptPathCompletion must be false when the path is already complete")
	}
	if got := p.input.Value(); got != full {
		t.Fatalf("value changed on no-op accept: %q", got)
	}
}

func TestImportPromptShowKeepsTrailingSlash(t *testing.T) {
	p := NewImportPrompt()
	p.Show("~/Downloads/")
	if got := p.Value(); got != "~/Downloads/" {
		t.Fatalf("Show value = %q, want ~/Downloads/", got)
	}
}

func TestImportPromptEnterWithoutCompletionDoesNotAccept(t *testing.T) {
	p := NewImportPrompt()
	p.Show("/no/such/dir/hopefully/")
	if p.AcceptPathCompletion() {
		t.Fatal("AcceptPathCompletion should be false with no choices")
	}
}
