package ui

import (
	"testing"

	"github.com/ruben/gsql/internal/config"
)

func TestSyncEditorQuery(t *testing.T) {
	m := NewModel(&config.Config{})
	m.lastQuery = "SELECT * FROM users WHERE id = 1"
	m.focus = FocusResults

	m.syncEditorQuery()
	if got := m.editor.Value(); got != "SELECT * FROM users WHERE id = 1;" {
		t.Fatalf("editor = %q, want filtered query with semicolon", got)
	}

	m.editor.SetValue("SELECT * FROM orders;")
	m.focus = FocusEditor
	m.syncEditorQuery()
	if got := m.editor.Value(); got != "SELECT * FROM orders;" {
		t.Fatalf("editor = %q, want draft preserved while editor focused", got)
	}
}

func TestClearFiltersSyncsEditor(t *testing.T) {
	m := &Model{
		baseQuery: "SELECT * FROM users",
		filters:   []string{"name = 'alice'"},
		focus:     FocusResults,
	}
	m.editor = NewQueryEditor()
	m.editor.SetValue("SELECT id FROM users LIMIT 10;")

	m.clearFilters()
	if got := m.editor.Value(); got != "SELECT * FROM users;" {
		t.Fatalf("editor = %q, want base query after clearing filters", got)
	}
	if len(m.filters) != 0 {
		t.Fatalf("expected filters cleared, got %v", m.filters)
	}
}

func TestApplyFilteredQueryIncludesSort(t *testing.T) {
	m := &Model{
		baseQuery: "SELECT * FROM users",
		filters:   []string{"name = 'alice'"},
		sortCol:   "id",
		sortDir:   "DESC",
		focus:     FocusResults,
	}
	m.editor = NewQueryEditor()

	m.applyFilteredQuery()
	want := "SELECT * FROM users WHERE name = 'alice' ORDER BY id DESC;"
	if got := m.editor.Value(); got != want {
		t.Fatalf("editor = %q, want %q", got, want)
	}
}
