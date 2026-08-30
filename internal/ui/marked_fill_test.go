package ui

import (
	"strings"
	"testing"

	"github.com/atotto/clipboard"
)

func TestMarkedFillFromCursor(t *testing.T) {
	m := newResultsWorkspaceModel()
	_ = clipboard.WriteAll("")

	// Mark both rows, then fill name from cursor (alice on row 0).
	m = press(m, keyRunes(' ')) // mark row 0
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes(' ')) // mark row 1
	m = press(m, keyRunes('k')) // back to alice
	m = press(m, keyRunes('l')) // name column
	m = press(m, keyRunes('p'))

	if m.results.DirtyCellCount() != 1 {
		t.Fatalf("dirty count = %d, want 1 (bob→alice)", m.results.DirtyCellCount())
	}
	if got := m.results.RowValue(1, 1); got != "alice" {
		t.Errorf("row 1 name = %q, want alice", got)
	}
	if m.results.MarkCount() != 2 {
		t.Errorf("marks should remain after fill, got %d", m.results.MarkCount())
	}
	if !strings.Contains(m.schemaMsg, "filled 1") {
		t.Errorf("schemaMsg = %q, want filled 1…", m.schemaMsg)
	}
}

func TestMarkedFillRefusesPK(t *testing.T) {
	m := newResultsWorkspaceModel()
	_ = clipboard.WriteAll("")

	m = press(m, keyRunes(' '))
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes(' '))
	m = press(m, keyRunes('p')) // still on id column

	if m.results.DirtyCellCount() != 0 {
		t.Errorf("PK fill should stage nothing, dirty=%d", m.results.DirtyCellCount())
	}
	if m.schemaMsg != "cannot fill primary key column" {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestMarkedFillFromClipboard(t *testing.T) {
	m := newResultsWorkspaceModel()
	if err := clipboard.WriteAll("filled"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}

	m = press(m, keyRunes(' '))
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes(' '))
	m = press(m, keyRunes('l')) // name
	m = press(m, keyRunes('p'))

	if m.results.DirtyCellCount() != 2 {
		t.Fatalf("dirty count = %d, want 2", m.results.DirtyCellCount())
	}
	if got := m.results.RowValue(0, 1); got != "filled" {
		t.Errorf("row 0 name = %q, want filled", got)
	}
	if got := m.results.RowValue(1, 1); got != "filled" {
		t.Errorf("row 1 name = %q, want filled", got)
	}
}

func TestPasteWithoutMarksDoesNotFill(t *testing.T) {
	m := newResultsWorkspaceModel()
	_ = clipboard.WriteAll("")

	m = press(m, keyRunes('l'))
	m = press(m, keyRunes('p'))

	if strings.Contains(m.schemaMsg, "filled") {
		t.Errorf("no-mark p should not fill: schemaMsg=%q", m.schemaMsg)
	}
	if m.exportMsg != "clipboard is empty" {
		t.Errorf("expected paste path empty-clipboard msg, got %q", m.exportMsg)
	}
}
