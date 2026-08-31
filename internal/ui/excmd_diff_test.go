package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func TestExDiffNeedsTwoTabs(t *testing.T) {
	m := NewModel(&config.Config{})
	m.runExCommand("diff")
	if !strings.Contains(m.schemaMsg, "two tabs") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExDiffComparesTabs(t *testing.T) {
	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	// Tab 0 already exists; add tab 1 and put results on both.
	m.addTab("B", "SELECT 1")
	m.saveTabState()

	// Fill tab 0 (active after addTab is the new tab — switch back).
	// addTab activates the new tab; set results on both via tab structs.
	if len(m.resultsTabs) < 2 {
		t.Fatal("need 2 tabs")
	}
	left := m.resultsTabs[0]
	right := m.resultsTabs[1]
	left.Results.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}, {"2", "bob"}}, "")
	left.Results.SetEditable("users", []string{"id"})
	right.Results.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}, {"2", "bobby"}}, "")
	right.Results.SetEditable("users", []string{"id"})

	m.activeTabID = right.ID
	m.restoreTabState()
	m.runExCommand("diff 1 2")
	if !m.diffPanel.IsVisible() {
		t.Fatalf("diff panel not shown; schemaMsg=%q", m.schemaMsg)
	}
	if m.diffPanel.diff.Changed != 1 || m.diffPanel.diff.Same != 1 {
		t.Fatalf("diff counts: %+v", m.diffPanel.diff)
	}
	if m.diffPanel.diff.Mode != "pk" {
		t.Fatalf("mode = %q, want pk", m.diffPanel.diff.Mode)
	}
}
