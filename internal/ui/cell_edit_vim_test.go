package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCellEditPopupVimEsc(t *testing.T) {
	p := NewCellEditPopup()
	p.Show("hello", 0, 0, "body", false)

	if p.VimMode() != VimInsert {
		t.Fatalf("mode = %v, want insert", p.VimMode())
	}
	handled, close := p.ConsumeEsc()
	if !handled || close {
		t.Fatalf("first esc: handled=%v close=%v, want true/false", handled, close)
	}
	if p.VimMode() != VimNormal {
		t.Fatalf("after esc: mode = %v, want normal", p.VimMode())
	}
	handled, close = p.ConsumeEsc()
	if !handled || !close {
		t.Fatalf("second esc: handled=%v close=%v, want true/true", handled, close)
	}
}

func TestCellEditPopupVimUndo(t *testing.T) {
	p := NewCellEditPopup()
	p.Show("", 0, 0, "body", false)
	p.buf.Focus()

	p, _ = p.Update(runeKey('x'))
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.Value() != "x" {
		t.Fatalf("after insert: %q", p.Value())
	}
	p, _ = p.Update(runeKey('u'))
	if p.Value() != "" {
		t.Fatalf("undo: %q, want empty", p.Value())
	}
}

func TestCellEditPopupReadOnlyEscCloses(t *testing.T) {
	p := NewCellEditPopup()
	p.Show("peek", 0, 0, "body", true)
	handled, close := p.ConsumeEsc()
	if !handled || !close {
		t.Fatalf("read-only esc: handled=%v close=%v", handled, close)
	}
}
