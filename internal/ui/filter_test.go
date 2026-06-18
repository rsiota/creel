package ui

import (
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

func TestBuildFilteredQuery_NoFilters(t *testing.T) {
	m := Model{baseQuery: "SELECT * FROM users"}
	got := m.buildFilteredQuery()
	if got != "SELECT * FROM users" {
		t.Errorf("expected base query, got %q", got)
	}
}

func TestBuildFilteredQuery_SingleFilter(t *testing.T) {
	m := Model{
		baseQuery: "SELECT * FROM users",
		filters:   []string{"country = 'UK'"},
	}
	got := m.buildFilteredQuery()
	want := "SELECT * FROM users WHERE country = 'UK'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildFilteredQuery_MultipleFilters(t *testing.T) {
	m := Model{
		baseQuery: "SELECT * FROM users",
		filters:   []string{"country = 'UK'", "active = '1'"},
	}
	got := m.buildFilteredQuery()
	want := "SELECT * FROM users WHERE country = 'UK' AND active = '1'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildFilteredQuery_StripsExistingLimit(t *testing.T) {
	m := Model{
		baseQuery: "SELECT * FROM users LIMIT 100;",
		filters:   []string{"country = 'UK'"},
	}
	got := m.buildFilteredQuery()
	want := "SELECT * FROM users WHERE country = 'UK'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildFilteredQuery_ExistingWhere(t *testing.T) {
	m := Model{
		baseQuery: "SELECT * FROM users WHERE active = 1",
		filters:   []string{"country = 'UK'"},
	}
	got := m.buildFilteredQuery()
	// Filters replace the query entirely; existing WHERE in base is discarded.
	// The original WHERE is not preserved — filters are the new WHERE.
	want := "SELECT * FROM users WHERE country = 'UK'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanFilter_SimpleSelect(t *testing.T) {
	m := Model{baseQuery: "SELECT * FROM users", connection: &db.Connection{}}
	if !m.canFilter() {
		t.Error("simple SELECT should be filterable")
	}
}

func TestCanFilter_Join(t *testing.T) {
	m := Model{baseQuery: "SELECT * FROM users JOIN orders ON users.id = orders.user_id", connection: &db.Connection{}}
	if m.canFilter() {
		t.Error("JOIN query should not be filterable")
	}
}

func TestCanFilter_EmptyBaseQuery(t *testing.T) {
	m := Model{baseQuery: ""}
	if m.canFilter() {
		t.Error("empty base query should not be filterable")
	}
}

func TestClearFilters(t *testing.T) {
	m := &Model{
		baseQuery: "SELECT * FROM users",
		filters:   []string{"country = 'UK'", "active = 1"},
	}
	m.clearFilters()
	if len(m.filters) != 0 {
		t.Error("filters should be empty after clearFilters")
	}
	if m.lastQuery != "SELECT * FROM users" {
		t.Errorf("lastQuery should be base query, got %q", m.lastQuery)
	}
}

func TestStatusBarShowsFilters(t *testing.T) {
	m := NewModel(&config.Config{})
	m.width = 200
	m.height = 40
	m.baseQuery = "SELECT * FROM users"
	m.filters = []string{"country = 'UK'"}
	out := m.statusBar("test")
	if !strings.Contains(out, "country = 'UK'") {
		t.Error("status bar should show active filter")
	}
}

func TestBuildFilteredQuery_WithSort(t *testing.T) {
	m := Model{
		baseQuery: "SELECT * FROM users",
		sortCol:   "name",
		sortDir:   "ASC",
	}
	got := m.buildFilteredQuery()
	want := "SELECT * FROM users ORDER BY name ASC"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildFilteredQuery_FiltersAndSort(t *testing.T) {
	m := Model{
		baseQuery: "SELECT * FROM users",
		filters:   []string{"country = 'UK'"},
		sortCol:   "name",
		sortDir:   "DESC",
	}
	got := m.buildFilteredQuery()
	want := "SELECT * FROM users WHERE country = 'UK' ORDER BY name DESC"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestToggleSort_Cycle(t *testing.T) {
	m := &Model{
		baseQuery:  "SELECT * FROM users",
		connection: &db.Connection{},
		results:    NewResultsTable(),
	}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}}, "")
	m.results.SetCursor(0, 1) // cursor on "name" column

	// First toggle: no sort → ASC
	m.toggleSort()
	if m.sortCol != "name" || m.sortDir != "ASC" {
		t.Errorf("after first toggle: sortCol=%q sortDir=%q, want name/ASC", m.sortCol, m.sortDir)
	}

	// Second toggle: ASC → DESC
	m.toggleSort()
	if m.sortCol != "name" || m.sortDir != "DESC" {
		t.Errorf("after second toggle: sortCol=%q sortDir=%q, want name/DESC", m.sortCol, m.sortDir)
	}

	// Third toggle: DESC → none
	m.toggleSort()
	if m.sortCol != "" || m.sortDir != "" {
		t.Errorf("after third toggle: sortCol=%q sortDir=%q, want empty", m.sortCol, m.sortDir)
	}
}
