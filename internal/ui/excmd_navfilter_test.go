package ui

import (
	"strings"
	"testing"
)

func TestExFollowBack(t *testing.T) {
	t.Run("follow no results", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("follow")
		if !strings.Contains(m.schemaMsg, "no results") {
			t.Errorf(":follow -> %q", m.schemaMsg)
		}
	})
	t.Run("follow no FK at cursor", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}}, "")
		m.runExCommand("follow")
		if !strings.Contains(m.schemaMsg, "no foreign key") {
			t.Errorf(":follow -> %q", m.schemaMsg)
		}
	})
	t.Run("back empty stack", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("back")
		if !strings.Contains(m.schemaMsg, "nowhere to go back") {
			t.Errorf(":back -> %q", m.schemaMsg)
		}
	})
}

func TestExKeepHide(t *testing.T) {
	t.Run("keep nothing to filter", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("keep")
		if !strings.Contains(m.schemaMsg, "no rows to filter") {
			t.Errorf(":keep -> %q", m.schemaMsg)
		}
	})
	t.Run("hide nothing to filter", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("hide")
		if !strings.Contains(m.schemaMsg, "no rows to filter") {
			t.Errorf(":hide -> %q", m.schemaMsg)
		}
	})
}

func TestExUndoUnfilter(t *testing.T) {
	t.Run("undo no filters", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("undo")
		if !strings.Contains(m.schemaMsg, "no filters to undo") {
			t.Errorf(":undo -> %q", m.schemaMsg)
		}
	})
	t.Run("unfilter no filters", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("unfilter")
		if !strings.Contains(m.schemaMsg, "no filters to clear") {
			t.Errorf(":unfilter -> %q", m.schemaMsg)
		}
	})
	t.Run("undo delegates and pops one filter", func(t *testing.T) {
		m := &Model{results: NewResultsTable(), editor: NewQueryEditor(), filters: []string{"a = 1", "b = 2"}}
		m.runExCommand("undo")
		if m.schemaMsg != "" {
			t.Errorf(":undo should delegate, got msg %q", m.schemaMsg)
		}
		if len(m.filters) != 1 || m.filters[0] != "a = 1" {
			t.Errorf("undo should pop the last filter, got %v", m.filters)
		}
	})
	t.Run("unfilter delegates and clears all", func(t *testing.T) {
		m := &Model{results: NewResultsTable(), editor: NewQueryEditor(), filters: []string{"a = 1", "b = 2"}}
		m.runExCommand("unfilter")
		if m.schemaMsg != "" {
			t.Errorf(":unfilter should delegate, got msg %q", m.schemaMsg)
		}
		if len(m.filters) != 0 {
			t.Errorf("unfilter should clear all filters, got %v", m.filters)
		}
	})
}
