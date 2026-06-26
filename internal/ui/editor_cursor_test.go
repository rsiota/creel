package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

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

func TestEditorInsertModeBarCursor(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.SetValue("select")
	e.Focus()

	// Enter insert mode; the editor should render a bar cursor glyph.
	e, _ = e.Update(runeKey('i'))

	if !strings.Contains(e.View(), "▏") {
		t.Fatalf("insert mode missing bar cursor")
	}
}

func TestEditorOpenLineShowsCursor(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	e.SetValue("select")
	e.Focus()

	// 'o' opens a new line below and enters insert mode. The new line is
	// empty, so the cursor must still be visible (as a bar).
	e, _ = e.Update(runeKey('o'))

	if !strings.Contains(e.View(), "▏") {
		t.Fatalf("cursor invisible after 'o' on empty line")
	}
}
