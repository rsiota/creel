package ui

import (
	"strings"
	"testing"

	"github.com/atotto/clipboard"
)

func TestMarkedFillFromCursor(t *testing.T) {
	m := newResultsWorkspaceModel()
	_ = clipboard.WriteAll("")
	m.yank = ""

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
	m.yank = ""

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
	m.yank = ""
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
	m.yank = ""

	m = press(m, keyRunes('l'))
	m = press(m, keyRunes('p'))

	if strings.Contains(m.schemaMsg, "filled") {
		t.Errorf("no-mark p should not fill: schemaMsg=%q", m.schemaMsg)
	}
	if m.exportMsg != "clipboard is empty" {
		t.Errorf("expected paste path empty-clipboard msg, got %q", m.exportMsg)
	}
}

// yy → mark → move onto a marked row → p must still use the yanked cell, even
// when the OS clipboard is empty (fallback used to use the cursor cell and
// become a no-op).
func TestMarkedFillFromYankIgnoresCursor(t *testing.T) {
	m := newResultsWorkspaceModel()
	_ = clipboard.WriteAll("")

	m = press(m, keyRunes('l')) // name: alice
	m = press(m, keyRunes('y'))
	m = press(m, keyRunes('y'))
	if m.yank != "alice" {
		t.Fatalf("yank = %q, want alice", m.yank)
	}
	_ = clipboard.WriteAll("") // simulate flaky/empty OS pasteboard after yy

	m = press(m, keyRunes('j')) // bob
	m = press(m, keyRunes(' ')) // mark bob only
	m = press(m, keyRunes('p')) // cursor still on bob

	if got := m.results.RowValue(1, 1); got != "alice" {
		t.Errorf("row 1 name = %q, want alice from yank", got)
	}
	if m.results.DirtyCellCount() != 1 {
		t.Fatalf("dirty count = %d, want 1", m.results.DirtyCellCount())
	}
	if !strings.Contains(m.schemaMsg, "filled 1") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

// Stale OS clipboard must not override a fresher yy yank for marked fill.
func TestMarkedFillPrefersYankOverClipboard(t *testing.T) {
	m := newResultsWorkspaceModel()
	if err := clipboard.WriteAll("stale"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}

	m = press(m, keyRunes('l'))
	m = press(m, keyRunes('y'))
	m = press(m, keyRunes('y')) // yank alice (also writes clipboard)
	m.yank = "alice"
	_ = clipboard.WriteAll("stale") // overwrite OS board after yy

	m = press(m, keyRunes('j'))
	m = press(m, keyRunes(' '))
	m = press(m, keyRunes('p'))

	if got := m.results.RowValue(1, 1); got != "alice" {
		t.Errorf("row 1 name = %q, want alice (yank over stale clipboard)", got)
	}
}

// p used to no-op silently whenever the inspector panel was open, even with
// results focus — a common layout while editing.
func TestMarkedFillWorksWithInspectorOpen(t *testing.T) {
	m := newResultsWorkspaceModel()
	_ = clipboard.WriteAll("")
	m.inspector.Show()

	m = press(m, keyRunes('l'))
	m = press(m, keyRunes('y'))
	m = press(m, keyRunes('y'))
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes(' '))
	m = press(m, keyRunes('p'))

	if got := m.results.RowValue(1, 1); got != "alice" {
		t.Errorf("row 1 name = %q, want alice (inspector open)", got)
	}
	if !strings.Contains(m.schemaMsg, "filled 1") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}
