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
}

func TestImportPromptEnterWithoutCompletionDoesNotAccept(t *testing.T) {
	p := NewImportPrompt()
	p.Show("/no/such/dir/hopefully/")
	if p.AcceptPathCompletion() {
		t.Fatal("AcceptPathCompletion should be false with no choices")
	}
}
