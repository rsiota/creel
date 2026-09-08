package ui

import (
	"testing"
)

// Typing in the cross-table search popup must not be swallowed by the
// workspace "t" new-tab binding (which is active on the sidebar).
func TestCrossSearchTypingDoesNotCreateTab(t *testing.T) {
	m := newWorkspaceModel(t)
	m.focus = FocusConnections
	m.crossSearch.Show()
	before := len(m.resultsTabs)

	m = sendKey(m, runeKey('t'))

	if len(m.resultsTabs) != before {
		t.Fatalf("typing t in cross-search created a tab: %d → %d", before, len(m.resultsTabs))
	}
	if got := m.crossSearch.Query(); got != "t" {
		t.Fatalf("query = %q, want %q", got, "t")
	}
	if !m.crossSearch.IsVisible() {
		t.Fatal("cross-search should stay open while typing")
	}
}
