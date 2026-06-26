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
