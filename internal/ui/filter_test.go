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

func TestUndoFilter(t *testing.T) {
	m := &Model{
		baseQuery: "SELECT * FROM users",
		filters:   []string{"country = 'UK'", "active = '1'"},
	}
	m.undoFilter()
	if len(m.filters) != 1 {
		t.Fatalf("expected 1 filter after undo, got %d", len(m.filters))
	}
	if m.filters[0] != "country = 'UK'" {
		t.Errorf("expected last filter to remain, got %q", m.filters[0])
	}
}

func TestUndoFilter_Empty(t *testing.T) {
	m := &Model{
		baseQuery: "SELECT * FROM users",
		filters:   nil,
	}
	cmd := m.undoFilter()
	if cmd != nil {
		t.Error("undoFilter with no filters should return nil")
	}
}

func TestUndoFilter_AllStripped(t *testing.T) {
	m := &Model{
		baseQuery: "SELECT * FROM users",
		filters:   []string{"country = 'UK'"},
	}
	m.undoFilter()
	if len(m.filters) != 0 {
		t.Errorf("expected 0 filters, got %d", len(m.filters))
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

func TestFindEqualityFilter_Equals(t *testing.T) {
	filters := []string{"name = 'alice'", "age = '30'"}
	idx, vals, found := findEqualityFilter(filters, "name")
	if !found || idx != 0 || len(vals) != 1 || vals[0] != "alice" {
		t.Errorf("expected to find name=alice at idx 0, got idx=%d vals=%v found=%v", idx, vals, found)
	}
}

func TestFindEqualityFilter_InClause(t *testing.T) {
	filters := []string{"country IN ('UK', 'US')"}
	idx, vals, found := findEqualityFilter(filters, "country")
	if !found || idx != 0 {
		t.Fatalf("expected to find country filter, got idx=%d found=%v", idx, found)
	}
	if len(vals) != 2 || vals[0] != "UK" || vals[1] != "US" {
		t.Errorf("expected [UK US], got %v", vals)
	}
}

func TestFindEqualityFilter_NotFound(t *testing.T) {
	filters := []string{"name = 'alice'"}
	_, _, found := findEqualityFilter(filters, "country")
	if found {
		t.Error("should not find filter for country")
	}
}

func TestBuildInClause_Single(t *testing.T) {
	got := buildInClause("country", []string{"UK"})
	want := "country = 'UK'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildInClause_Multiple(t *testing.T) {
	got := buildInClause("country", []string{"UK", "US"})
	want := "country IN ('UK', 'US')"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterPicker_ToggleSelected(t *testing.T) {
	p := NewFilterPicker()
	p.Show("country", "")
	p.SetValues([]string{"UK", "US", "FR"}, nil)

	p.ToggleSelected() // toggle cursor on UK (first item)
	if vals := p.SelectedValues(); len(vals) != 1 || vals[0] != "UK" {
		t.Errorf("expected [UK] selected, got %v", vals)
	}

	p.ToggleSelected() // toggle off
	if vals := p.SelectedValues(); len(vals) != 0 {
		t.Errorf("expected no selections, got %v", vals)
	}
}

func TestFilterPicker_PreSelected(t *testing.T) {
	p := NewFilterPicker()
	p.Show("country", "")
	p.SetValues([]string{"UK", "US", "FR"}, map[string]bool{"UK": true, "FR": true})

	vals := p.SelectedValues()
	if len(vals) != 2 {
		t.Errorf("expected 2 pre-selected, got %v", vals)
	}
}

func TestFilterPicker_SelectAllNone(t *testing.T) {
	p := NewFilterPicker()
	p.Show("country", "")
	p.SetValues([]string{"UK", "US", "FR"}, nil)

	p.SelectAll()
	if len(p.SelectedValues()) != 3 {
		t.Error("SelectAll should select all 3")
	}

	p.SelectNone()
	if len(p.SelectedValues()) != 0 {
		t.Error("SelectNone should clear all")
	}
}

func TestFilterPicker_FuzzyFilter(t *testing.T) {
	p := NewFilterPicker()
	p.Show("name", "")
	p.SetValues([]string{"alice", "bob", "charlie", "andrew"}, nil)

	p.FilterAddChar("a")
	filtered := p.filteredValues()
	// "a" matches alice, andrew, charlie
	if len(filtered) != 3 {
		t.Errorf("expected 3 matches for 'a', got %d: %v", len(filtered), filtered)
	}
}

func TestApplyFilterPickerSelection(t *testing.T) {
	m := &Model{
		baseQuery:  "SELECT * FROM users",
		connection: &db.Connection{},
		results:    NewResultsTable(),
	}
	m.results.SetResult([]string{"id", "country"}, [][]string{{"1", "UK"}}, "")

	m.filterPicker.Show("country", "")
	m.filterPicker.SetValues([]string{"UK", "US", "FR"}, nil)
	m.filterPicker.ToggleSelected() // select UK
	m.filterPicker.CursorDown()
	m.filterPicker.ToggleSelected() // select US

	m.applyFilterPickerSelection()

	if len(m.filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(m.filters))
	}
	if m.filters[0] != "country IN ('UK', 'US')" {
		t.Errorf("expected IN clause, got %q", m.filters[0])
	}
}

func TestApplyFilterPickerSelection_Single(t *testing.T) {
	m := &Model{
		baseQuery:  "SELECT * FROM users",
		connection: &db.Connection{},
		results:    NewResultsTable(),
	}
	m.results.SetResult([]string{"id", "country"}, [][]string{{"1", "UK"}}, "")

	m.filterPicker.Show("country", "")
	m.filterPicker.SetValues([]string{"UK", "US"}, nil)
	m.filterPicker.ToggleSelected() // select UK

	m.applyFilterPickerSelection()

	if len(m.filters) != 1 || m.filters[0] != "country = 'UK'" {
		t.Errorf("expected single equality, got %v", m.filters)
	}
}

func TestApplyFilterPickerSelection_Replaces(t *testing.T) {
	m := &Model{
		baseQuery:  "SELECT * FROM users",
		connection: &db.Connection{},
		results:    NewResultsTable(),
		filters:    []string{"country = 'FR'", "active = '1'"},
	}
	m.results.SetResult([]string{"id", "country"}, [][]string{{"1", "UK"}}, "")

	m.filterPicker.Show("country", "")
	m.filterPicker.SetValues([]string{"UK", "US"}, nil)
	m.filterPicker.ToggleSelected() // select UK

	m.applyFilterPickerSelection()

	// Should have replaced country filter, kept active filter
	if len(m.filters) != 2 {
		t.Fatalf("expected 2 filters, got %d: %v", len(m.filters), m.filters)
	}
	if m.filters[0] != "active = '1'" {
		t.Errorf("expected active filter preserved at 0, got %q", m.filters[0])
	}
	if m.filters[1] != "country = 'UK'" {
		t.Errorf("expected new country filter at 1, got %q", m.filters[1])
	}
}
