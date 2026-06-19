package ui

import (
	"fmt"
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
	p.Show("country")
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
	p.Show("country")
	p.SetValues([]string{"UK", "US", "FR"}, map[string]bool{"UK": true, "FR": true})

	vals := p.SelectedValues()
	if len(vals) != 2 {
		t.Errorf("expected 2 pre-selected, got %v", vals)
	}
}

func TestFilterPicker_SelectAllNone(t *testing.T) {
	p := NewFilterPicker()
	p.Show("country")
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
	p.Show("name")
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

	m.filterPicker.Show("country")
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

	m.filterPicker.Show("country")
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

	m.filterPicker.Show("country")
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

func TestBuildQuickFilter_EqualsString(t *testing.T) {
	got := buildQuickFilter("country", "UK", "TEXT", false)
	want := "country = 'UK'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildQuickFilter_EqualsNumeric(t *testing.T) {
	got := buildQuickFilter("age", "30", "INTEGER", false)
	want := "age = 30"
	if got != want {
		t.Errorf("numeric should be unquoted: got %q, want %q", got, want)
	}
}

func TestBuildQuickFilter_NegateStringNullSafe(t *testing.T) {
	got := buildQuickFilter("country", "UK", "TEXT", true)
	want := "(country != 'UK' OR country IS NULL)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildQuickFilter_NegateNumeric(t *testing.T) {
	got := buildQuickFilter("age", "30", "INT", true)
	want := "(age != 30 OR age IS NULL)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildQuickFilter_NullValue(t *testing.T) {
	if got := buildQuickFilter("email", "NULL", "TEXT", false); got != "email IS NULL" {
		t.Errorf("null equals: got %q", got)
	}
	if got := buildQuickFilter("email", "NULL", "TEXT", true); got != "email IS NOT NULL" {
		t.Errorf("null negate: got %q", got)
	}
}

func TestBuildQuickFilter_EscapesQuotes(t *testing.T) {
	got := buildQuickFilter("name", "O'Brien", "VARCHAR", false)
	want := "name = 'O''Brien'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsNumericType(t *testing.T) {
	numerics := []string{"INTEGER", "int", "BIGINT", "TINYINT", "REAL", "FLOAT", "DECIMAL(10,2)", "decimal", "numeric"}
	for _, ty := range numerics {
		if !isNumericType(ty) {
			t.Errorf("expected %q to be numeric", ty)
		}
	}
	nonNumerics := []string{"TEXT", "VARCHAR(255)", "", "BLOB", "DATE", "TIMESTAMP"}
	for _, ty := range nonNumerics {
		if isNumericType(ty) {
			t.Errorf("expected %q to NOT be numeric", ty)
		}
	}
}

func TestRemoveColumnFilters(t *testing.T) {
	filters := []string{
		"country = 'UK'",
		"age = 30",
		"(status != 'active' OR status IS NULL)",
		"country IS NULL",
		"email IN ('a@x.com', 'b@x.com')",
	}
	got := removeColumnFilters(filters, "country")
	// Both country filters (equality + IS NULL) removed; others intact.
	want := []string{"age = 30", "(status != 'active' OR status IS NULL)", "email IN ('a@x.com', 'b@x.com')"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("idx %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRemoveColumnFilters_WordBoundary(t *testing.T) {
	// Removing "age" must not touch "age_group".
	filters := []string{"age = 30", "age_group = 'admin'"}
	got := removeColumnFilters(filters, "age")
	if len(got) != 1 || got[0] != "age_group = 'admin'" {
		t.Errorf("word boundary violated: got %v", got)
	}
}

func TestCompactFilter_InClause(t *testing.T) {
	got := compactFilter("country IN ('UK', 'US', 'FR')")
	want := "country ∈ (3)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCompactFilter_Negate(t *testing.T) {
	got := compactFilter("(status != 'active' OR status IS NULL)")
	want := "status ≠ 'active'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCompactFilter_Plain(t *testing.T) {
	cases := map[string]string{
		"country = 'UK'":      "country = 'UK'",
		"age = 30":            "age = 30",
		"email IS NULL":       "email IS NULL",
		"email IS NOT NULL":   "email IS NOT NULL",
	}
	for in, want := range cases {
		if got := compactFilter(in); got != want {
			t.Errorf("compactFilter(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuickFilterCell_ReplacesExisting(t *testing.T) {
	m := &Model{
		baseQuery:  "SELECT * FROM users",
		connection: &db.Connection{},
		results:    NewResultsTable(),
	}
	m.results.SetResult([]string{"id", "country"}, [][]string{{"1", "UK"}, {"2", "US"}}, "")
	m.results.SetColumnTypes(map[string]string{"id": "INTEGER", "country": "TEXT"})
	m.results.CursorRight() // move cursor to country column
	m.filters = []string{"country = 'US'", "age = 30"}

	m.quickFilterCell(false)

	if len(m.filters) != 2 {
		t.Fatalf("expected 2 filters (country replaced + age kept), got %d: %v", len(m.filters), m.filters)
	}
	if m.filters[0] != "age = 30" {
		t.Errorf("age filter should be preserved at 0, got %q", m.filters[0])
	}
	if m.filters[1] != "country = 'UK'" {
		t.Errorf("country should be replaced with UK, got %q", m.filters[1])
	}
}

func TestRowMarks_ToggleAndCount(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}, {"2", "bob"}}, "")
	r.SetEditable("users", []string{"id"})
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "name": "TEXT"})

	if r.MarkCount() != 0 {
		t.Fatalf("expected 0 marks, got %d", r.MarkCount())
	}

	// Mark row 0.
	r.ToggleMark()
	if r.MarkCount() != 1 {
		t.Errorf("expected 1 mark, got %d", r.MarkCount())
	}
	if !r.IsMarkedRow(0) {
		t.Error("row 0 should be marked")
	}
	if r.IsMarkedRow(1) {
		t.Error("row 1 should not be marked")
	}

	// Unmark row 0.
	r.ToggleMark()
	if r.MarkCount() != 0 {
		t.Errorf("expected 0 marks after untoggle, got %d", r.MarkCount())
	}
}

func TestRowMarks_MoveCursorAndMarkMultiple(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}, {"2", "b"}, {"3", "c"}}, "")
	r.SetEditable("users", []string{"id"})

	r.ToggleMark()       // cursor on row 0 (id=1)
	r.CursorDown()
	r.ToggleMark()       // cursor on row 1 (id=2)
	r.CursorDown()
	r.ToggleMark()       // cursor on row 2 (id=3)

	if r.MarkCount() != 3 {
		t.Fatalf("expected 3 marks, got %d", r.MarkCount())
	}
	tuples := r.MarkedPKs()
	if len(tuples) != 3 {
		t.Fatalf("expected 3 tuples, got %d", len(tuples))
	}
}

func TestRowMarks_SurviveRequerySameTable(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}, {"2", "b"}}, "")
	r.SetEditable("users", []string{"id"})
	r.ToggleMark() // mark id=1

	// Simulate a filtered re-query: new rows but same table/PK.
	r.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}, {"9", "z"}}, "")
	// Editability is re-established after the re-query.
	r.SetEditable("users", []string{"id"})

	if r.MarkCount() != 1 {
		t.Errorf("mark should survive same-table requery, got %d", r.MarkCount())
	}
	if !r.IsMarkedRow(0) {
		t.Error("row with id=1 should still be marked after requery")
	}
}

func TestRowMarks_InvalidateOnTableChange(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}}, "")
	r.SetEditable("users", []string{"id"})
	r.ToggleMark()
	if r.MarkCount() != 1 {
		t.Fatalf("expected 1 mark, got %d", r.MarkCount())
	}

	// Switch to a different table.
	r.SetResult([]string{"oid", "label"}, [][]string{{"1", "x"}}, "")
	r.SetEditable("orders", []string{"oid"})

	if r.MarkCount() != 0 {
		t.Errorf("marks should invalidate on table change, got %d", r.MarkCount())
	}
	if r.IsMarkedRow(0) {
		t.Error("no rows should be marked after table change")
	}
}

func TestRowMarks_NoOpWhenNotEditable(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}}, "")
	// Not editable: no PK.
	r.ToggleMark()
	if r.MarkCount() != 0 {
		t.Errorf("should not mark non-editable table, got %d", r.MarkCount())
	}
}

func TestBuildPKInClause_Single(t *testing.T) {
	got := buildPKInClause([]string{"id"}, []string{"INTEGER"}, [][]string{{"1"}, {"2"}, {"3"}})
	want := "id IN (1, 2, 3)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPKInClause_SingleStringType(t *testing.T) {
	got := buildPKInClause([]string{"code"}, []string{"VARCHAR"}, [][]string{{"US"}, {"UK"}})
	want := "code IN ('US', 'UK')"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPKInClause_Composite(t *testing.T) {
	got := buildPKInClause([]string{"a", "b"}, []string{"INTEGER", "TEXT"}, [][]string{{"1", "x"}, {"2", "y"}})
	want := "(a, b) IN ((1, 'x'), (2, 'y'))"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterByMarks_BuildsAndConsumes(t *testing.T) {
	m := &Model{
		baseQuery:  "SELECT * FROM users",
		connection: &db.Connection{},
		results:    NewResultsTable(),
	}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}, {"2", "b"}, {"3", "c"}}, "")
	m.results.SetColumnTypes(map[string]string{"id": "INTEGER", "name": "TEXT"})
	m.results.SetEditable("users", []string{"id"})

	// Mark rows 0 and 2 (ids 1 and 3).
	m.results.ToggleMark()
	m.results.CursorDown()
	m.results.CursorDown()
	m.results.ToggleMark()

	cmd := m.filterByMarks()
	if cmd == nil {
		t.Fatal("expected a command from filterByMarks")
	}
	if len(m.filters) != 1 {
		t.Fatalf("expected 1 filter, got %d: %v", len(m.filters), m.filters)
	}
	if m.filters[0] != "id IN (1, 3)" {
		t.Errorf("expected 'id IN (1, 3)', got %q", m.filters[0])
	}
	if m.results.MarkCount() != 0 {
		t.Errorf("marks should be cleared after F, got %d", m.results.MarkCount())
	}
}

func TestFilterByMarks_CompositePK(t *testing.T) {
	m := &Model{
		baseQuery:  "SELECT * FROM orders",
		connection: &db.Connection{},
		results:    NewResultsTable(),
	}
	m.results.SetResult([]string{"a", "b"}, [][]string{{"1", "x"}, {"2", "y"}}, "")
	m.results.SetColumnTypes(map[string]string{"a": "INTEGER", "b": "TEXT"})
	m.results.SetEditable("orders", []string{"a", "b"})

	m.results.ToggleMark()
	m.results.CursorDown()
	m.results.ToggleMark()

	m.filterByMarks()
	if len(m.filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(m.filters))
	}
	want := "(a, b) IN ((1, 'x'), (2, 'y'))"
	if m.filters[0] != want {
		t.Errorf("got %q, want %q", m.filters[0], want)
	}
}

func TestCompactFilter_CompositePKIn(t *testing.T) {
	got := compactFilter("(a, b) IN ((1, 'x'), (2, 'y'), (3, 'z'))")
	want := "(a, b) ∈ (3)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestComputeClientStats_Numeric(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "age"}, [][]string{
		{"1", "30"},
		{"2", "40"},
		{"3", "NULL"},
		{"4", "50"},
	}, "")
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "age": "INTEGER"})

	row := computeClientStats(r, 1, true)
	stats := formatColumnStats(row, true)
	// count 3, distinct 3, min 30, max 50, sum 120, avg 40
	if !strings.Contains(stats, "count 3") {
		t.Errorf("expected count 3, got: %s", stats)
	}
	if !strings.Contains(stats, "min 30") {
		t.Errorf("expected min 30, got: %s", stats)
	}
	if !strings.Contains(stats, "max 50") {
		t.Errorf("expected max 50, got: %s", stats)
	}
	if !strings.Contains(stats, "sum 120") {
		t.Errorf("expected sum 120, got: %s", stats)
	}
	if !strings.Contains(stats, "avg 40") {
		t.Errorf("expected avg 40, got: %s", stats)
	}
}

func TestComputeClientStats_NumericSkipsNulls(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "age"}, [][]string{
		{"1", "NULL"},
		{"2", "NULL"},
	}, "")
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "age": "INTEGER"})

	row := computeClientStats(r, 1, true)
	stats := formatColumnStats(row, true)
	if !strings.Contains(stats, "count 0") {
		t.Errorf("expected count 0 (all NULL), got: %s", stats)
	}
}

func TestComputeClientStats_TextColumn(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "name"}, [][]string{
		{"1", "alice"},
		{"2", "bob"},
		{"3", "alice"},
	}, "")
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "name": "TEXT"})

	row := computeClientStats(r, 1, false)
	stats := formatColumnStats(row, false)
	// count 3, distinct 2 (alice + bob), min alice, max bob
	if !strings.Contains(stats, "count 3") {
		t.Errorf("expected count 3, got: %s", stats)
	}
	if !strings.Contains(stats, "distinct 2") {
		t.Errorf("expected distinct 2, got: %s", stats)
	}
	if !strings.Contains(stats, "min alice") {
		t.Errorf("expected min alice, got: %s", stats)
	}
	if !strings.Contains(stats, "max bob") {
		t.Errorf("expected max bob, got: %s", stats)
	}
}

func TestFormatColumnStats_Numeric(t *testing.T) {
	row := []string{"10", "5", "1.5", "9.9", "42.5", "4.25"}
	got := formatColumnStats(row, true)
	if !strings.Contains(got, "count 10") || !strings.Contains(got, "avg 4.25") {
		t.Errorf("unexpected numeric format: %s", got)
	}
}

func TestFormatColumnStats_Text(t *testing.T) {
	row := []string{"10", "3", "aaa", "zzz"}
	got := formatColumnStats(row, false)
	if !strings.Contains(got, "count 10") || !strings.Contains(got, "min aaa") {
		t.Errorf("unexpected text format: %s", got)
	}
}

func TestSerializeCSV_Basic(t *testing.T) {
	cols := []string{"id", "name", "email"}
	rows := [][]string{{"1", "alice", "alice@test.com"}, {"2", "bob", "bob@test.com"}}
	got, err := serializeCSV(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	want := "id,name,email\n1,alice,alice@test.com\n2,bob,bob@test.com\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSerializeCSV_NullBecomesEmpty(t *testing.T) {
	cols := []string{"id", "name"}
	rows := [][]string{{"1", "NULL"}, {"2", "bob"}}
	got, err := serializeCSV(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "1,\n") {
		t.Errorf("NULL should become empty field, got: %q", got)
	}
}

func TestSerializeCSV_QuotingCommas(t *testing.T) {
	cols := []string{"desc"}
	rows := [][]string{{"value, with comma"}}
	got, err := serializeCSV(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\"value, with comma\"") {
		t.Errorf("comma values should be quoted, got: %q", got)
	}
}

func TestSerializeCSV_QuotingEmbeddedQuotes(t *testing.T) {
	cols := []string{"desc"}
	rows := [][]string{{"O'Brien"}}
	got, err := serializeCSV(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	// No commas — should not be quoted. Single quotes are not special in CSV.
	want := "desc\nO'Brien\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSerializeCSV_EmptyRows(t *testing.T) {
	cols := []string{"id"}
	got, err := serializeCSV(cols, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "id\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExportFilename(t *testing.T) {
	got := exportFilename("users", "20260619_120000")
	want := "gsql_users_20260619_120000.csv"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExportFilename_EmptyTable(t *testing.T) {
	got := exportFilename("", "20260619")
	if got != "gsql_query_20260619.csv" {
		t.Errorf("got %q", got)
	}
}

func TestExportStatusMessage_Success(t *testing.T) {
	got := exportStatusMessage("/Users/x/Downloads/f.csv", 42, nil)
	if !strings.Contains(got, "exported 42 rows") || !strings.Contains(got, "/Users/x/Downloads/f.csv") {
		t.Errorf("unexpected success message: %q", got)
	}
}

func TestExportStatusMessage_Error(t *testing.T) {
	got := exportStatusMessage("", 0, fmt.Errorf("disk full"))
	if !strings.Contains(got, "export failed") || !strings.Contains(got, "disk full") {
		t.Errorf("unexpected error message: %q", got)
	}
}
