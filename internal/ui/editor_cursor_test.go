package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func init() {
	// Force ANSI output so styling is present in test-rendered views.
	lipgloss.SetColorProfile(termenv.ANSI)
}

// runeKey builds a KeyRunes message for driving vim normal/insert mode.
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestEditorNormalModeNoCharDoublingAtEnd(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.SetValue("select")
	e.Focus()

	// Move to start, then right onto each character. Normal mode must not
	// duplicate the character under the cursor (previously "selectt").
	e, _ = e.Update(runeKey('0')) // cursor on 's'
	e, _ = e.Update(runeKey('l')) // cursor on 'e'
	e, _ = e.Update(runeKey('l')) // cursor on 'l'

	stripped := ansi.Strip(e.View())
	if strings.Contains(stripped, "selectt") || strings.Contains(stripped, "selecct") {
		t.Fatalf("cursor doubled a character: %q", stripped)
	}
}

func TestEditorEmptyShowsCursor(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.Focus()

	// Normal mode: reverse-video cursor on empty buffer.
	view := e.View()
	if !strings.Contains(view, "\x1b[7m") {
		t.Fatalf("empty editor missing normal-mode reverse cursor")
	}

	// Enter insert mode: cursor should switch to underline.
	e, _ = e.Update(runeKey('i'))
	view = e.View()
	if !strings.Contains(view, ";4m") {
		t.Fatalf("empty editor missing insert-mode underline cursor")
	}
}

func TestEditorInsertModeUnderlineCursor(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.SetValue("select")
	e.Focus()

	e, _ = e.Update(runeKey('i'))

	if !strings.Contains(e.View(), ";4m") {
		t.Fatalf("insert mode missing underline cursor")
	}
}

func TestEditorNormalModeReverseCursor(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.SetValue("select")
	e.Focus()

	// Default is normal mode: reverse-video on the char under cursor.
	if !strings.Contains(e.View(), "\x1b[7m") {
		t.Fatalf("normal mode missing reverse cursor")
	}
}

func TestEditorOpenLineShowsCursor(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.SetValue("select")
	e.Focus()

	// 'o' opens a new line below and enters insert mode. The new line is
	// empty, so the cursor (underline on space) must still be visible.
	e, _ = e.Update(runeKey('o'))

	if !strings.Contains(e.View(), ";4m") {
		t.Fatalf("cursor invisible after 'o' on empty line")
	}
}

func TestEditorUndoRedo(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.Focus()

	e, _ = e.Update(runeKey('i'))
	e, _ = e.Update(runeKey('x'))
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if e.Value() != "x" {
		t.Fatalf("after insert: %q", e.Value())
	}
	e, _ = e.Update(runeKey('u'))
	if e.Value() != "" {
		t.Fatalf("undo should restore empty, got %q", e.Value())
	}
	e, _ = e.Update(runeKey('U'))
	if e.Value() != "x" {
		t.Fatalf("redo should restore x, got %q", e.Value())
	}
}

func TestEditorSearch(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.SetValue("select foo from bar where foo > 1")
	e.Focus()
	e, _ = e.Update(runeKey('0')) // start of buffer

	e, _ = e.Update(runeKey('/'))
	if !e.IsSearching() {
		t.Fatal("expected search prompt")
	}
	e, _ = e.Update(runeKey('f'))
	e, _ = e.Update(runeKey('o'))
	e, _ = e.Update(runeKey('o'))
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if e.IsSearching() {
		t.Fatal("enter should close the search prompt")
	}
	_, col := e.cursorLineCol()
	if col != 7 { // "select " is 7 runes; first "foo" starts at 7
		t.Fatalf("first match col=%d, want 7", col)
	}
	e, _ = e.Update(runeKey('n'))
	_, col = e.cursorLineCol()
	if col != 26 { // "select foo from bar where " is 26
		t.Fatalf("n: col=%d, want 26", col)
	}
}

func TestEditorVisualLineYank(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.SetValue("one\ntwo\nthree")
	e.Focus()
	e, _ = e.Update(runeKey('g'))
	e, _ = e.Update(runeKey('g')) // top
	e, _ = e.Update(runeKey('V'))
	if !e.IsVisual() {
		t.Fatal("expected visual line")
	}
	e, _ = e.Update(runeKey('j'))
	e, _ = e.Update(runeKey('y'))
	if e.IsVisual() {
		t.Fatal("y should leave visual mode")
	}
	if e.yank != "one\ntwo" {
		t.Fatalf("yank=%q, want one\\ntwo", e.yank)
	}
}
