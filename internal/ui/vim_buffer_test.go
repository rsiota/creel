package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func TestEditorVimModeOnStatusBar(t *testing.T) {
	m := NewModel(&config.Config{})
	m.state = stateWorkspace
	m.focus = FocusEditor
	bar := stripANSIForTest(m.statusBar("test"))
	if !strings.Contains(bar, "NORMAL") {
		t.Fatalf("status bar missing NORMAL: %q", bar)
	}
	m.editor, _ = m.editor.Update(runeKey('i'))
	bar = stripANSIForTest(m.statusBar("test"))
	if !strings.Contains(bar, "INSERT") {
		t.Fatalf("status bar missing INSERT: %q", bar)
	}
}

func TestSharedYankRegister(t *testing.T) {
	var reg string
	editor := NewQueryEditor()
	editor.BindYank(&reg)
	popup := NewCellEditPopup()
	popup.BindYank(&reg)

	editor.SetValue("alpha")
	editor, _ = editor.Update(runeKey('y'))
	if reg != "alpha" {
		t.Fatalf("shared reg = %q, want alpha", reg)
	}

	popup.Show("", 0, 0, "body", false)
	popup, _ = popup.Update(tea.KeyMsg{Type: tea.KeyEsc})
	popup, _ = popup.Update(runeKey('p'))
	if popup.Value() != "alpha" {
		t.Fatalf("paste = %q, want alpha", popup.Value())
	}
}

func TestVimBufferYankMotions(t *testing.T) {
	b := NewVimBuffer(VimBufferConfig{InitialMode: VimNormal})
	b.setValueRaw("one two three")
	b.restoreCursor(0, 4)

	b, _ = b.Update(runeKey('y'))
	b, _ = b.Update(runeKey('w'))
	if got := b.Yank(); got != "two" {
		t.Fatalf("yw = %q, want two", got)
	}

	b.setValueRaw("one two")
	b.restoreCursor(0, 4)
	b, _ = b.Update(runeKey('y'))
	b, _ = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'$'}})
	if got := b.Yank(); got != "two" {
		t.Fatalf("y$ = %q, want two", got)
	}
}

func TestCellEditPopupReadOnlySearch(t *testing.T) {
	p := NewCellEditPopup()
	p.Show("alpha\nbeta\ngamma", 0, 0, "body", true)
	p.SetMaxSize(60, 2)

	p, _ = p.Update(runeKey('/'))
	handled, close := p.ConsumeEsc()
	if !handled || close {
		t.Fatalf("esc during search prompt: handled=%v close=%v", handled, close)
	}

	p, _ = p.Update(runeKey('/'))
	p, _ = p.Update(runeKey('b'))
	p, _ = p.Update(runeKey('e'))
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.buf.Line() != 1 {
		t.Fatalf("cursor line = %d, want 1 (beta)", p.buf.Line())
	}
	view := p.View()
	if !strings.Contains(view, "beta") {
		t.Fatalf("search view missing beta:\n%s", view)
	}

	handled, close = p.ConsumeEsc()
	if !handled || !close {
		t.Fatalf("esc after search should close: handled=%v close=%v", handled, close)
	}
}

func stripANSIForTest(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '\x1b' {
			in = true
			continue
		}
		if in {
			if r == 'm' {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
