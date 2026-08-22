package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestFilterCandidates(t *testing.T) {
	all := []completionItem{
		{text: "SELECT", kind: kindKeyword},
		{text: "SET", kind: kindKeyword},
		{text: "SUM", kind: kindKeyword},
		{text: "users", kind: kindTable},
		{text: "user_settings", kind: kindTable},
		{text: "id", kind: kindColumn},
	}

	// Fuzzy match "se" → SELECT, SET, users, user_settings (subsequence match)
	filtered := filterCandidates(all, "se")
	if len(filtered) != 4 {
		t.Fatalf("expected 4 matches for 'se', got %d: %+v", len(filtered), filtered)
	}

	// Fuzzy match "us" → users, user_settings
	filtered = filterCandidates(all, "us")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 matches for 'us', got %d", len(filtered))
	}

	// Empty partial → all
	filtered = filterCandidates(all, "")
	if len(filtered) != len(all) {
		t.Errorf("expected all %d, got %d", len(all), len(filtered))
	}

	// No match
	filtered = filterCandidates(all, "xyz")
	if len(filtered) != 0 {
		t.Errorf("expected 0 matches for 'xyz', got %d", len(filtered))
	}
}

func TestWordBeforeCursor(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	for _, ch := range "SELECT * FROM us" {
		e.textarea.InsertString(string(ch))
	}

	word, start := e.wordBeforeCursor()
	if word != "us" {
		t.Errorf("expected word 'us', got %q", word)
	}
	if start != 14 {
		t.Errorf("expected start 14, got %d", start)
	}
}

func TestCompletionManualTrigger(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	for _, ch := range "SEL" {
		e.textarea.InsertString(string(ch))
	}

	e.SetCandidates([]completionItem{
		{text: "SELECT", kind: kindKeyword},
		{text: "SET", kind: kindKeyword},
		{text: "users", kind: kindTable},
	})
	e.StartCompletion()

	if !e.CompletionVisible() {
		t.Fatal("expected completion to be visible")
	}
	if len(e.completion.candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(e.completion.candidates))
	}
	if e.completion.candidates[0].text != "SELECT" {
		t.Errorf("expected SELECT, got %s", e.completion.candidates[0].text)
	}
}

func TestCompletionAccept(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	for _, ch := range "SEL" {
		e.textarea.InsertString(string(ch))
	}

	e.SetCandidates([]completionItem{
		{text: "SELECT", kind: kindKeyword},
	})
	e.StartCompletion()
	e.AcceptCompletion()

	if e.CompletionVisible() {
		t.Error("expected completion hidden after accept")
	}
	if e.Value() != "SELECT" {
		t.Errorf("expected 'SELECT', got %q", e.Value())
	}
}

func TestCompletionNavigation(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	e.textarea.InsertString("s")

	e.SetCandidates([]completionItem{
		{text: "SELECT", kind: kindKeyword},
		{text: "SET", kind: kindKeyword},
		{text: "SUM", kind: kindKeyword},
	})
	e.StartCompletion()

	e.MoveCompletion(1)
	if e.completion.selected != 1 {
		t.Errorf("expected selected=1, got %d", e.completion.selected)
	}

	e.MoveCompletion(10)
	// from 1, +10 = 11, 11 % 3 = 2
	if e.completion.selected != 2 {
		t.Errorf("expected selected=2 after wrap, got %d", e.completion.selected)
	}

	e.MoveCompletion(-1)
	if e.completion.selected != 1 {
		t.Errorf("expected selected=1 after -1, got %d", e.completion.selected)
	}
}

func TestCompletionCancel(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	e.textarea.InsertString("SEL")
	e.SetCandidates([]completionItem{
		{text: "SELECT", kind: kindKeyword},
	})
	e.StartCompletion()

	// Esc cancels (via handleInsertMode)
	updated, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	e = updated

	if e.CompletionVisible() {
		t.Error("expected completion hidden after esc")
	}
	if e.VimMode() != VimInsert {
		t.Error("expected still in insert mode")
	}
}

func TestCompletionAcceptReplacesPartialWord(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	for _, ch := range "SELECT * FROM us" {
		e.textarea.InsertString(string(ch))
	}

	e.SetCandidates([]completionItem{
		{text: "users", kind: kindTable},
		{text: "user_settings", kind: kindTable},
	})
	e.StartCompletion()

	// "users" ranks first by fuzzy score; accept at cursor.
	e.AcceptCompletion()

	expected := "SELECT * FROM users"
	if e.Value() != expected {
		t.Errorf("expected %q, got %q", expected, e.Value())
	}
}

func TestAutoTriggerAfterOneChar(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	e.SetCandidates([]completionItem{
		{text: "SELECT", kind: kindKeyword},
		{text: "SET", kind: kindKeyword},
	})

	// Type "s" — should trigger immediately (>= minAutoTriggerChars)
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !e.CompletionVisible() {
		t.Fatal("expected popup visible after 1 char")
	}
	// Both SELECT and SET start with 's'
	if len(e.completion.candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(e.completion.candidates))
	}

	// Type "e" — should narrow to SELECT and SET (both start with "se")
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if len(e.completion.candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(e.completion.candidates))
	}
}

func TestAutoTriggerDismissOnSpace(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	e.SetCandidates([]completionItem{
		{text: "SELECT", kind: kindKeyword},
	})

	// Type "se" to trigger
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !e.CompletionVisible() {
		t.Fatal("expected popup visible after 'se'")
	}

	// Type space — should dismiss
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeySpace})
	if e.CompletionVisible() {
		t.Error("expected popup dismissed after space")
	}
}

func TestAutoTriggerBackspaceRefilter(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()

	e.SetCandidates([]completionItem{
		{text: "SELECT", kind: kindKeyword},
		{text: "SET", kind: kindKeyword},
		{text: "users", kind: kindTable},
	})

	// Type "se" to trigger (3 fuzzy matches: SELECT, SET, users)
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if len(e.completion.candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(e.completion.candidates))
	}

	// Backspace → word is "s" (still >= 1 char) → popup stays but refilters
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if !e.CompletionVisible() {
		t.Error("expected popup still visible with 1-char word")
	}
}

func TestAutoTriggerFromApp(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "orders"}
	m.columnCache = map[string][]db.Column{
		"users":  {{Name: "id"}, {Name: "email"}},
		"orders": {{Name: "id"}, {Name: "total"}},
	}
	m.state = stateWorkspace
	m.focus = FocusEditor
	m.refreshCompletionCandidates()

	// Enter insert mode
	m.editor.vimMode = VimInsert
	m.editor.Focus()

	// Type "u" — should auto-trigger with "users" as a candidate
	m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if !m.editor.CompletionVisible() {
		t.Fatal("expected popup visible after 1 char")
	}

	found := false
	for _, c := range m.editor.completion.candidates {
		if c.text == "users" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'users' in candidates")
	}

	m.editor.SetValue("")
	m.editor.textarea.InsertString("SELECT * FROM users WHERE ")
	m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !m.editor.CompletionVisible() {
		t.Fatal("expected popup visible after WHERE e")
	}
	for _, c := range m.editor.completion.candidates {
		if c.text == "total" {
			t.Fatal("orders.total should not appear after FROM users WHERE")
		}
	}
}

func TestRenderCompletionOmitsTypedPrefix(t *testing.T) {
	applyPalette(defaultPalette)
	c := completion{
		visible: true,
		partial: "us",
		candidates: []completionItem{
			{text: "users", kind: kindTable},
			{text: "user_id", kind: kindColumn},
		},
	}
	out := ansi.Strip(c.renderCompletion())
	if strings.Contains(out, "❯") || strings.Contains(out, "> us") {
		t.Errorf("popup should not echo typed prefix, got:\n%s", out)
	}
	if !strings.Contains(out, "users") {
		t.Errorf("popup missing candidates:\n%s", out)
	}
}
