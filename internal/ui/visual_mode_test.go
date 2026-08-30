package ui

import (
	"strings"
	"testing"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

func TestVisualModeRangeDown(t *testing.T) {
	m := newResultsWorkspaceModel()

	// Enter visual mode at row 0.
	m = press(m, keyRunes('V'))
	if !m.results.IsVisualMode() {
		t.Fatal("expected visual mode active after V")
	}
	lo, hi := m.results.VisualRange()
	if lo != 0 || hi != 0 {
		t.Fatalf("initial range = [%d,%d], want [0,0]", lo, hi)
	}

	// Move down twice — range should extend (test data has 2 rows, so
	// the cursor stops at row 1).
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes('j'))
	lo, hi = m.results.VisualRange()
	if lo != 0 || hi != 1 {
		t.Fatalf("after jj range = [%d,%d], want [0,1]", lo, hi)
	}
	if m.results.VisualRangeSize() != 2 {
		t.Errorf("range size = %d, want 2", m.results.VisualRangeSize())
	}
}

func TestVisualModeRangeUp(t *testing.T) {
	m := newResultsWorkspaceModel()
	// Move to row 1 first.
	m = press(m, keyRunes('j'))

	// Enter visual mode and move up.
	m = press(m, keyRunes('V'))
	m = press(m, keyRunes('k'))
	lo, hi := m.results.VisualRange()
	if lo != 0 || hi != 1 {
		t.Fatalf("range = [%d,%d], want [0,1]", lo, hi)
	}
}

func TestVisualModeCommitMarks(t *testing.T) {
	m := newResultsWorkspaceModel()

	// Enter visual mode, move down 1, commit.
	m = press(m, keyRunes('V'))
	m = press(m, keyRunes('j'))
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.results.IsVisualMode() {
		t.Error("visual mode should exit after enter")
	}
	if m.results.MarkCount() != 2 {
		t.Fatalf("expected 2 marks after commit, got %d", m.results.MarkCount())
	}
}

func TestVisualModeEscapeCancel(t *testing.T) {
	m := newResultsWorkspaceModel()

	m = press(m, keyRunes('V'))
	m = press(m, keyRunes('j'))

	// Escape without committing.
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.results.IsVisualMode() {
		t.Error("visual mode should exit after esc")
	}
	if m.results.MarkCount() != 0 {
		t.Errorf("expected 0 marks after cancel, got %d", m.results.MarkCount())
	}
}

func TestVisualModeVToggleExit(t *testing.T) {
	m := newResultsWorkspaceModel()

	m = press(m, keyRunes('V'))
	if !m.results.IsVisualMode() {
		t.Fatal("expected visual mode after first V")
	}
	m = press(m, keyRunes('V'))
	if m.results.IsVisualMode() {
		t.Error("visual mode should exit after second V")
	}
}

func TestVisualModeNotEditable(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.results.ClearEditable()

	m = press(m, keyRunes('V'))
	if m.results.IsVisualMode() {
		t.Error("should not enter visual mode on non-editable results")
	}
}

func TestVisualModeOnlyJKEscEnterV(t *testing.T) {
	m := newResultsWorkspaceModel()

	m = press(m, keyRunes('V'))
	// Press a key that's not handled by visual mode — should be swallowed.
	m = press(m, keyRunes('x'))
	if !m.results.IsVisualMode() {
		t.Error("visual mode should remain active after unhandled key")
	}
	// The unhandled key should NOT have triggered export.
	if m.exportMsg != "" {
		t.Error("x should be swallowed in visual mode, not trigger export")
	}
}

func TestVisualModeGotoTopBottom(t *testing.T) {
	m := newResultsWorkspaceModel()

	m = press(m, keyRunes('V'))
	// G goes to bottom.
	m = press(m, keyRunes('G'))
	lo, hi := m.results.VisualRange()
	if hi != 1 { // only 2 rows in test data
		t.Fatalf("after G, hi = %d, want 1", hi)
	}
	if lo != 0 {
		t.Fatalf("after G, lo = %d, want 0", lo)
	}

	// g goes to top.
	m = press(m, keyRunes('g'))
	lo, hi = m.results.VisualRange()
	if lo != 0 || hi != 0 {
		t.Fatalf("after g, range = [%d,%d], want [0,0]", lo, hi)
	}
}

func TestVisualModeAddToExistingMarks(t *testing.T) {
	m := newResultsWorkspaceModel()

	// Mark row 0 with space.
	m = press(m, keyRunes(' '))
	if m.results.MarkCount() != 1 {
		t.Fatalf("expected 1 mark, got %d", m.results.MarkCount())
	}

	// Enter visual mode at row 1, select row 1, commit.
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes('V'))
	m = press(m, keyRunes('j'))
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // enter — commits row 1

	if m.results.MarkCount() != 2 {
		t.Errorf("expected 2 marks total (1 space + 1 visual), got %d", m.results.MarkCount())
	}
}

func TestVisualModeClearedOnTableSwitch(t *testing.T) {
	m := newResultsWorkspaceModel()

	m = press(m, keyRunes('V'))
	if !m.results.IsVisualMode() {
		t.Fatal("expected visual mode active")
	}

	// Simulate table switch via SetEditable (called on new query).
	m.results.SetEditable("other_table", []string{"id"})
	if m.results.IsVisualMode() {
		t.Error("visual mode should be cleared after SetEditable (table switch)")
	}
}

func TestVisualModeFillFromAnchor(t *testing.T) {
	m := newResultsWorkspaceModel()
	_ = clipboard.WriteAll("") // prefer anchor when clipboard empty
	m.yank = ""

	// Move to name column (col 1).
	m = press(m, keyRunes('l'))
	m = press(m, keyRunes('V'))
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes('p'))

	if m.results.IsVisualMode() {
		t.Error("visual mode should exit after fill")
	}
	if m.results.DirtyCellCount() != 1 {
		t.Fatalf("dirty count = %d, want 1 (bob→alice)", m.results.DirtyCellCount())
	}
	if got := m.results.RowValue(1, 1); got != "alice" {
		t.Errorf("row 1 name = %q, want alice", got)
	}
	if !strings.Contains(m.schemaMsg, "filled 1") {
		t.Errorf("schemaMsg = %q, want filled 1…", m.schemaMsg)
	}
}

func TestVisualModeFillRefusesPK(t *testing.T) {
	m := newResultsWorkspaceModel()
	_ = clipboard.WriteAll("")
	m.yank = ""

	// Stay on id (PK) column.
	m = press(m, keyRunes('V'))
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes('p'))

	if m.results.DirtyCellCount() != 0 {
		t.Errorf("PK fill should stage nothing, dirty=%d", m.results.DirtyCellCount())
	}
	if m.schemaMsg != "cannot fill primary key column" {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestVisualModeFillFromClipboard(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.yank = ""
	if err := clipboard.WriteAll("filled"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}

	m = press(m, keyRunes('l')) // name
	m = press(m, keyRunes('V'))
	m = press(m, keyRunes('j'))
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
