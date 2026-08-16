package ui

import (
	"strings"
	"testing"
)

func TestCopyRowKeybindingYR(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.exportMsg = ""
	m.schemaMsg = ""

	// y r — same path as :copyrow (TSV).
	m = press(m, keyRunes('y'))
	if !m.resultsPendingY {
		t.Fatal("expected pending-Y after first y")
	}
	m = press(m, keyRunes('r'))
	if m.resultsPendingY {
		t.Fatal("pending-Y should clear after y r")
	}
	if !strings.Contains(m.exportMsg, "copied") || !strings.Contains(m.exportMsg, "tsv") {
		t.Errorf("exportMsg = %q, want copied … as tsv", m.exportMsg)
	}
}

func TestCopyRowKeybindingDoesNotStealGR(t *testing.T) {
	m := newResultsWorkspaceModel()
	// g r must still open the explorer, not copy rows.
	m = press(m, keyRunes('g'))
	if !m.resultsPendingG {
		t.Fatal("expected pending-G after g")
	}
	m = press(m, keyRunes('r'))
	if m.resultsPendingG {
		t.Fatal("pending-G should clear after g r")
	}
	if m.exportMsg != "" && strings.Contains(m.exportMsg, "tsv") {
		t.Errorf("g r must not copy rows: exportMsg=%q", m.exportMsg)
	}
}

func TestCopyCellYYStillWorks(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.exportMsg = ""
	m = press(m, keyRunes('y'))
	m = press(m, keyRunes('y'))
	if m.resultsPendingY {
		t.Fatal("pending-Y should clear after yy")
	}
	if strings.Contains(m.exportMsg, "tsv") {
		t.Errorf("yy must not take the copyrow path: exportMsg=%q", m.exportMsg)
	}
}
