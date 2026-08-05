package ui

import (
	"strings"
	"testing"
)

func TestExCopyInsert(t *testing.T) {
	t.Run("no rows", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("copyinsert")
		if !strings.Contains(m.schemaMsg, "no rows to copy") {
			t.Errorf(":copyinsert -> %q", m.schemaMsg)
		}
	})
	t.Run("copies rows", func(t *testing.T) {
		// Rows present -> CopyAsInsert generates INSERTs (falling back to table
		// name "table" with no source). :copyinsert should return the
		// copy-feedback cmd.
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"a", "b"}, [][]string{{"1", "2"}}, "")
		cmd := m.runExCommand("copyinsert")
		if cmd == nil {
			t.Fatalf(":copyinsert -> %q", m.schemaMsg)
		}
		if !strings.Contains(m.exportMsg, "copied") {
			t.Errorf("exportMsg = %q", m.exportMsg)
		}
	})
}

func TestExCopyRow(t *testing.T) {
	t.Run("no rows", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("copyrow")
		if !strings.Contains(m.schemaMsg, "nothing to copy") {
			t.Errorf(":copyrow with no rows -> %q", m.schemaMsg)
		}
	})
	t.Run("copies cursor row as tsv by default", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"a", "b"}, [][]string{{"1", "2"}}, "")
		cmd := m.runExCommand("copyrow")
		if cmd == nil {
			t.Fatalf(":copyrow -> %q", m.schemaMsg)
		}
		if !strings.Contains(m.exportMsg, "copied 1 row as tsv") {
			t.Errorf("exportMsg = %q", m.exportMsg)
		}
	})
	t.Run("honours format argument", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"a", "b"}, [][]string{{"1", "2"}}, "")
		m.runExCommand("copyrow csv")
		if !strings.Contains(m.exportMsg, "as csv") {
			t.Errorf(":copyrow csv -> %q", m.exportMsg)
		}
	})
	t.Run("rejects unknown format", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"a"}, [][]string{{"1"}}, "")
		m.runExCommand("copyrow xml")
		if !strings.Contains(m.schemaMsg, "format must be one of") {
			t.Errorf(":copyrow xml -> %q", m.schemaMsg)
		}
	})
}

func TestExRegex(t *testing.T) {
	t.Run("no rows", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("regex foo")
		if !strings.Contains(m.schemaMsg, "no rows to search") {
			t.Errorf(":regex -> %q", m.schemaMsg)
		}
	})
	t.Run("needs a pattern", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"a"}, [][]string{{"x"}}, "")
		m.runExCommand("regex")
		if !strings.Contains(m.schemaMsg, "needs a pattern") {
			t.Errorf(":regex bare -> %q", m.schemaMsg)
		}
	})
	t.Run("applies and reports matches", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"a", "b"}, [][]string{{"alice", "x"}, {"bob", "y"}}, "")
		m.runExCommand("regex al")
		if !strings.Contains(m.searchMsg, "1 match") {
			t.Errorf(":regex al -> searchMsg %q", m.searchMsg)
		}
		if m.lastSearch != "al" {
			t.Errorf("lastSearch = %q, want al", m.lastSearch)
		}
	})
	t.Run("no matches", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"a"}, [][]string{{"alice"}}, "")
		m.runExCommand("regex zzz")
		if !strings.Contains(m.searchMsg, "no matches") {
			t.Errorf(":regex zzz -> searchMsg %q", m.searchMsg)
		}
	})
	t.Run("pattern with spaces", func(t *testing.T) {
		// regex that matches "a b" — args are rejoined so spaces survive.
		m := &Model{results: NewResultsTable()}
		m.results.SetResult([]string{"a"}, [][]string{{"a b"}}, "")
		m.runExCommand("regex a b")
		if !strings.Contains(m.searchMsg, "1 match") {
			t.Errorf(":regex 'a b' -> searchMsg %q", m.searchMsg)
		}
	})
}

func TestExHideShowColumns(t *testing.T) {
	resultsWithCols := func() ResultsTable {
		r := NewResultsTable()
		r.SetResult([]string{"id", "name", "email"}, [][]string{{"1", "a", "b"}}, "")
		return r
	}
	t.Run("hidecolumn no columns", func(t *testing.T) {
		m := &Model{results: NewResultsTable()}
		m.runExCommand("hidecolumn")
		if !strings.Contains(m.schemaMsg, "no columns to hide") {
			t.Errorf(":hidecolumn -> %q", m.schemaMsg)
		}
	})
	t.Run("hidecolumn by name", func(t *testing.T) {
		m := &Model{results: resultsWithCols()}
		m.runExCommand("hidecolumn email")
		if m.schemaMsg != "" {
			t.Errorf(":hidecolumn email -> %q", m.schemaMsg)
		}
		if !m.results.IsColumnHidden(2) {
			t.Error("email column should be hidden")
		}
	})
	t.Run("hidecolumn bad name", func(t *testing.T) {
		m := &Model{results: resultsWithCols()}
		m.runExCommand("hidecolumn nope")
		if !strings.Contains(m.schemaMsg, "no such column") {
			t.Errorf(":hidecolumn nope -> %q", m.schemaMsg)
		}
	})
	t.Run("hidecolumn defaults to cursor col", func(t *testing.T) {
		m := &Model{results: resultsWithCols()}
		m.results.SetCursor(0, 1) // name
		m.runExCommand("hidecolumn")
		if !m.results.IsColumnHidden(1) {
			t.Error("cursor column (name) should be hidden")
		}
	})
	t.Run("showcolumns reveals all", func(t *testing.T) {
		r := resultsWithCols()
		r.HideColumn(0)
		r.HideColumn(2)
		m := &Model{results: r}
		m.runExCommand("showcolumns")
		if m.results.HiddenCount() != 0 {
			t.Errorf("expected no hidden columns, got %d", m.results.HiddenCount())
		}
	})
}
