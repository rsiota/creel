package ui

import (
	"strings"
	"testing"

	"github.com/atotto/clipboard"
)

func TestExRunNothing(t *testing.T) {
	m := &Model{editor: NewQueryEditor()}
	m.runExCommand("run")
	if !strings.Contains(m.schemaMsg, "nothing to run") {
		t.Errorf(":run empty editor -> %q", m.schemaMsg)
	}
}

func TestExRunAliasR(t *testing.T) {
	m := &Model{editor: NewQueryEditor()}
	m.editor.SetValue("SELECT 1;")
	// Without a connection executeQuery still builds a command (async);
	// we only assert the empty-guard is skipped and a cmd is returned.
	cmd := m.runExCommand("r")
	if cmd == nil {
		t.Error(":r with a statement should return an execute command")
	}
	if m.lastQuery != "SELECT 1;" && m.lastQuery != "SELECT 1" {
		// StatementAtCursor may or may not keep the semicolon depending on
		// editor parsing; either way lastQuery must be set.
		if m.lastQuery == "" {
			t.Error(":r should set lastQuery before returning the cmd")
		}
	}
}

func TestExDescribeAliasD(t *testing.T) {
	m := &Model{}
	m.runExCommand("d users")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":d should resolve to describe, got %q", m.schemaMsg)
	}
}

func TestExQuitAll(t *testing.T) {
	t.Run("quits with multiple tabs", func(t *testing.T) {
		m := &Model{
			results:     NewResultsTable(),
			resultsTabs: []*ResultsTab{{ID: 1}, {ID: 2}},
			activeTabID: 1,
		}
		cmd := m.runExCommand("qa")
		if !m.quitting {
			t.Error(":qa should set quitting")
		}
		if cmd == nil {
			t.Error(":qa should return tea.Quit")
		}
	})
	t.Run("dirty blocks", func(t *testing.T) {
		m := &Model{
			results:     NewResultsTable(),
			resultsTabs: []*ResultsTab{{ID: 1, Results: NewResultsTable()}, {ID: 2}},
			activeTabID: 1,
		}
		m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
		m.results.SetDirtyCell(0, 0, "x")
		cmd := m.runExCommand("qa")
		if !strings.Contains(m.schemaMsg, "unsaved") {
			t.Errorf("dirty :qa -> %q", m.schemaMsg)
		}
		if m.quitting || cmd != nil {
			t.Error("dirty :qa should not quit")
		}
	})
	t.Run("force overrides dirty", func(t *testing.T) {
		m := &Model{
			results:     NewResultsTable(),
			resultsTabs: []*ResultsTab{{ID: 1}, {ID: 2}},
			activeTabID: 1,
		}
		m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
		m.results.SetDirtyCell(0, 0, "x")
		cmd := m.runExCommand("qa!")
		if !m.quitting || cmd == nil {
			t.Error(":qa! should quit despite dirty cells")
		}
	})
	t.Run("dirty on inactive tab blocks", func(t *testing.T) {
		dirty := NewResultsTable()
		dirty.SetResult([]string{"id"}, [][]string{{"1"}}, "")
		dirty.SetDirtyCell(0, 0, "x")
		m := &Model{
			results:     NewResultsTable(),
			resultsTabs: []*ResultsTab{{ID: 1, Results: dirty}, {ID: 2}},
			activeTabID: 2,
		}
		cmd := m.runExCommand("qa")
		if !strings.Contains(m.schemaMsg, "unsaved") {
			t.Errorf("inactive dirty :qa -> %q", m.schemaMsg)
		}
		if m.quitting || cmd != nil {
			t.Error("inactive dirty :qa should not quit")
		}
	})
}

func TestExTabCommands(t *testing.T) {
	t.Run("tabnew", func(t *testing.T) {
		m := &Model{
			editor:      NewQueryEditor(),
			results:     NewResultsTable(),
			resultsTabs: []*ResultsTab{NewResultsTab(1, "first")},
			activeTabID: 1,
			nextTabID:   2,
			width:       120,
			height:      40,
		}
		m.editor.SetValue("SELECT 1;")
		m.runExCommand("tabnew")
		if len(m.resultsTabs) != 2 {
			t.Fatalf(":tabnew -> %d tabs, want 2", len(m.resultsTabs))
		}
		if m.activeTabID != 2 {
			t.Errorf("active tab = %d, want 2", m.activeTabID)
		}
	})
	t.Run("tabclose last refused", func(t *testing.T) {
		m := &Model{resultsTabs: []*ResultsTab{{ID: 1}}, results: NewResultsTable()}
		m.runExCommand("tabclose")
		if !strings.Contains(m.schemaMsg, "last tab") {
			t.Errorf(":tabclose last -> %q", m.schemaMsg)
		}
		if len(m.resultsTabs) != 1 {
			t.Errorf("last tab should remain, got %d", len(m.resultsTabs))
		}
	})
	t.Run("tabclose closes", func(t *testing.T) {
		m := &Model{
			editor:      NewQueryEditor(),
			results:     NewResultsTable(),
			resultsTabs: []*ResultsTab{NewResultsTab(1, "a"), NewResultsTab(2, "b")},
			activeTabID: 2,
			width:       120,
			height:      40,
		}
		m.runExCommand("tabclose")
		if len(m.resultsTabs) != 1 {
			t.Errorf(":tabclose -> %d tabs, want 1", len(m.resultsTabs))
		}
		if m.activeTabID != 1 {
			t.Errorf("active after close = %d, want 1", m.activeTabID)
		}
	})
	t.Run("tabnext and tabprev", func(t *testing.T) {
		m := &Model{
			editor:      NewQueryEditor(),
			results:     NewResultsTable(),
			resultsTabs: []*ResultsTab{NewResultsTab(1, "a"), NewResultsTab(2, "b"), NewResultsTab(3, "c")},
			activeTabID: 1,
			width:       120,
			height:      40,
		}
		m.runExCommand("tabnext")
		if m.activeTabID != 2 {
			t.Errorf(":tabnext -> %d, want 2", m.activeTabID)
		}
		m.runExCommand("tabp")
		if m.activeTabID != 1 {
			t.Errorf(":tabp -> %d, want 1", m.activeTabID)
		}
	})
	t.Run("tabs lists", func(t *testing.T) {
		m := &Model{
			resultsTabs: []*ResultsTab{
				{ID: 1, Title: "users"},
				{ID: 2, Title: "orders"},
			},
			activeTabID: 2,
		}
		m.runExCommand("tabs")
		if !strings.Contains(m.schemaMsg, "1:users") || !strings.Contains(m.schemaMsg, "[2:orders]") {
			t.Errorf(":tabs -> %q", m.schemaMsg)
		}
	})
}

func TestExCopy(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("copy")
		if !strings.Contains(m.schemaMsg, "nothing to copy") {
			t.Errorf(":copy empty -> %q", m.schemaMsg)
		}
	})
	t.Run("copies cell", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}}, "")
		m.results.SetCursor(0, 1)
		cmd := m.runExCommand("copy")
		if cmd == nil {
			t.Error(":copy should return feedback cmd")
		}
		got, err := clipboard.ReadAll()
		if err != nil {
			t.Skipf("clipboard unavailable: %v", err)
		}
		if got != "alice" {
			t.Errorf("clipboard = %q, want alice", got)
		}
	})
}
