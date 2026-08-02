package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

// detectResultMetadata loads table context (foreign keys, editability) for a query.
func (m *Model) detectResultMetadata(query string) {
	m.results.ClearForeignKeys()
	m.detectEditability(query)

	table := parseSimpleSelectTable(query)
	if table == "" || m.connection == nil {
		return
	}

	found := false
	for _, t := range m.tables {
		if strings.EqualFold(t, table) {
			found = true
			break
		}
	}
	if !found {
		return
	}

	fks, err := m.connection.DB().ForeignKeys(table)
	if err != nil {
		return
	}
	m.results.SetForeignKeys(table, fks)
}

// canFilter reports whether the current results support quick-filtering
// (i.e. the base query is a simple single-table SELECT).
func (m Model) canFilter() bool {
	if m.connection == nil || m.baseQuery == "" {
		return false
	}
	return parseSimpleSelectTable(m.baseQuery) != ""
}

// buildFilteredQuery reconstructs the query from the known table name with
// all active filters and sort applied. Since canFilter() guarantees a simple
// SELECT * FROM <table>, we rebuild from scratch to avoid issues with existing
// LIMIT/ORDER BY clauses in the original query.
func (m Model) buildFilteredQuery() string {
	table := parseSimpleSelectTable(m.baseQuery)
	q := fmt.Sprintf("SELECT * FROM %s", table)
	if len(m.filters) > 0 {
		q += " WHERE " + strings.Join(m.filters, " AND ")
	}
	if m.sortCol != "" {
		q += fmt.Sprintf(" ORDER BY %s %s", m.sortCol, m.sortDir)
	}
	return q
}

// applyFilteredQuery rebuilds lastQuery from active filters/sort and mirrors it
// into the editor when results-driven actions change the effective SQL.
func (m *Model) applyFilteredQuery() {
	m.lastQuery = m.buildFilteredQuery()
	m.syncEditorQuery()
}

// syncEditorQuery updates the editor to reflect lastQuery. Skipped while the
// editor is focused so an in-progress draft is not overwritten.
func (m *Model) syncEditorQuery() {
	if m.focus == FocusEditor || m.lastQuery == "" {
		return
	}
	q := strings.TrimRight(strings.TrimSpace(m.lastQuery), ";")
	m.editor.SetValue(q + ";")
}

// openColumnPicker opens the column-visibility overlay, seeded with the
// current results columns and their hidden state.
func (m *Model) openColumnPicker() {
	hidden := make(map[string]bool)
	for _, name := range m.results.HiddenColumnNames() {
		hidden[name] = true
	}
	m.columnPicker.Show(m.results.columns, hidden)
}

// applyColumnVisibility commits the picker's selection to the results table
// and closes the overlay.
func (m *Model) applyColumnVisibility() tea.Cmd {
	hidden := m.columnPicker.HiddenColumns()
	m.columnPicker.Hide()
	m.results.SetHiddenColumns(hidden)
	return nil
}

// openFilterPicker opens the value picker for the current column,
// fetching distinct values from the database asynchronously.
func (m *Model) openFilterPicker() tea.Cmd {
	if !m.canFilter() || m.results.NumRows() == 0 {
		return nil
	}
	colName := m.results.ColumnName(m.results.CursorCol())
	if colName == "" {
		return nil
	}

	table := parseSimpleSelectTable(m.baseQuery)
	m.filterPicker.Show(colName)

	conn := m.connection
	return func() tea.Msg {
		result, err := conn.DB().Execute(fmt.Sprintf("SELECT DISTINCT %s FROM %s", colName, table))
		if err != nil {
			return filterValuesMsg{column: colName}
		}
		values := make([]string, 0, len(result.Rows))
		for _, row := range result.Rows {
			if len(row) > 0 {
				values = append(values, row[0])
			}
		}
		return filterValuesMsg{column: colName, values: values}
	}
}

// applyFilterPickerSelection takes the selected values from the picker
// and applies them as a filter (IN clause or IS NULL), then re-executes.
func (m *Model) applyFilterPickerSelection() tea.Cmd {
	colName := m.filterPicker.Column()
	selected := m.filterPicker.SelectedValues()
	m.filterPicker.Hide()

	// Remove any existing equality/IN filter on this column.
	if idx, _, found := findEqualityFilter(m.filters, colName); found {
		m.filters = append(m.filters[:idx], m.filters[idx+1:]...)
	}

	if len(selected) > 0 {
		escaped := make([]string, len(selected))
		for i, v := range selected {
			escaped[i] = strings.ReplaceAll(v, "'", "''")
		}
		m.filters = append(m.filters, buildInClause(colName, escaped))
	}

	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// clearFilters removes all active filters and re-executes the base query.
func (m *Model) clearFilters() tea.Cmd {
	if len(m.filters) == 0 {
		return nil
	}
	m.filters = nil
	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// undoFilter removes the last-added filter and re-executes the query.
func (m *Model) undoFilter() tea.Cmd {
	if len(m.filters) == 0 {
		return nil
	}
	m.filters = m.filters[:len(m.filters)-1]
	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// backendSearchDebounce is the delay before the backend search query fires.
const backendSearchDebounce = 0 * time.Millisecond

// scheduleBackendSearch arms a debounce timer; the query executes when it fires.
func (m *Model) scheduleBackendSearch() tea.Cmd {
	if m.backendSearchTimer != nil {
		m.backendSearchTimer.Stop()
	}
	input := m.backendSearchInput
	m.backendSearchTimer = time.AfterFunc(backendSearchDebounce, func() {})
	return tea.Tick(backendSearchDebounce, func(time.Time) tea.Msg {
		return backendSearchTickMsg{input: input}
	})
}

// buildBackendSearchQuery constructs a LIKE-based query across all text columns
// of the current table.
func (m Model) buildBackendSearchQuery(term string) string {
	table := parseSimpleSelectTable(m.baseQuery)
	if table == "" || term == "" {
		return m.baseQuery
	}
	cols, ok := m.columnCache[table]
	if !ok {
		resultCols := m.results.columns
		cols = make([]db.Column, len(resultCols))
		for i, c := range resultCols {
			cols[i] = db.Column{Name: c}
		}
	}
	escaped := strings.ReplaceAll(term, "'", "''")
	var clauses []string
	for _, c := range cols {
		clauses = append(clauses, fmt.Sprintf("%s LIKE '%%%s%%'", c.Name, escaped))
	}
	q := fmt.Sprintf("SELECT * FROM %s WHERE %s", table, strings.Join(clauses, " OR "))
	if m.sortCol != "" {
		q += fmt.Sprintf(" ORDER BY %s %s", m.sortCol, m.sortDir)
	}
	return q
}

// runBackendSearch replaces lastQuery with the backend search query and runs it.
func (m *Model) runBackendSearch(term string) tea.Cmd {
	if term == "" {
		m.lastQuery = m.buildFilteredQuery()
		m.syncEditorQuery()
		m.page = 0
		return m.runPageQuery()
	}
	q := m.buildBackendSearchQuery(term)
	m.lastQuery = q
	m.syncEditorQuery()
	m.page = 0
	return m.runPageQuery()
}

// commitBackendSearch exits search input mode, keeping results.
func (m *Model) commitBackendSearch() {
	m.backendSearching = false
	m.backendSearchInput = ""
	if m.backendSearchTimer != nil {
		m.backendSearchTimer.Stop()
		m.backendSearchTimer = nil
	}
}

// cancelBackendSearch exits search input mode and restores the original query.
func (m *Model) cancelBackendSearch() {
	m.backendSearching = false
	m.backendSearchInput = ""
	if m.backendSearchTimer != nil {
		m.backendSearchTimer.Stop()
		m.backendSearchTimer = nil
	}
	m.lastQuery = m.buildFilteredQuery()
	m.syncEditorQuery()
	m.page = 0
}

// none → ASC → DESC → none.
func (m *Model) toggleSort() tea.Cmd {
	return m.sortByColName(m.results.ColumnName(m.results.CursorCol()))
}

// sortByColName cycles the sort state for the given column: none → ASC → DESC → none.
func (m *Model) sortByColName(colName string) tea.Cmd {
	if !m.canFilter() || m.results.NumRows() == 0 || colName == "" {
		return nil
	}
	switch {
	case m.sortCol == "":
		m.sortCol = colName
		m.sortDir = "ASC"
	case m.sortCol == colName && m.sortDir == "ASC":
		m.sortDir = "DESC"
	default:
		m.sortCol = ""
		m.sortDir = ""
	}
	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// preserveCursorCol sets up cursor restoration so the column position
// survives the query re-execution (row resets to 0 since data order changes).
func (m *Model) preserveCursorCol() {
	m.restoreCursor = true
	m.restoreCursorRow = 0
	m.restoreCursorCol = m.results.CursorCol()
}

// findEqualityFilter looks for an existing `=` or `IN` filter on colName.
// Returns the filter index, the set of values, and whether one was found.
func findEqualityFilter(filters []string, colName string) (int, []string, bool) {
	for i, f := range filters {
		eqPrefix := colName + " = '"
		if strings.HasPrefix(f, eqPrefix) && strings.HasSuffix(f, "'") {
			return i, []string{f[len(eqPrefix) : len(f)-1]}, true
		}
		inPrefix := colName + " IN ("
		if strings.HasPrefix(f, inPrefix) && strings.HasSuffix(f, ")") {
			inner := f[len(inPrefix) : len(f)-1]
			parts := strings.Split(inner, ", ")
			vals := make([]string, 0, len(parts))
			for _, p := range parts {
				vals = append(vals, strings.Trim(p, "'"))
			}
			return i, vals, true
		}
	}
	return -1, nil, false
}

// buildInClause builds a filter expression for a set of values.
// Single value → col = 'val', multiple → col IN ('v1', 'v2', ...).
func buildInClause(colName string, values []string) string {
	if len(values) == 1 {
		return fmt.Sprintf("%s = '%s'", colName, values[0])
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("'%s'", v)
	}
	return fmt.Sprintf("%s IN (%s)", colName, strings.Join(quoted, ", "))
}

// isNumericType reports whether a database column type should be treated as
// numeric (and therefore left unquoted in generated WHERE fragments).
func isNumericType(dbType string) bool {
	if dbType == "" {
		return false
	}
	t := strings.ToLower(dbType)
	// Strip common "(n)" or "(n,m)" suffixes, e.g. "decimal(10,2)".
	if i := strings.IndexByte(t, '('); i > 0 {
		t = t[:i]
	}
	switch t {
	case "int", "integer", "tinyint", "smallint", "mediumint", "bigint",
		"int unsigned", "bigint unsigned", "tinyint unsigned", "smallint unsigned", "mediumint unsigned",
		"unsigned int", "unsigned bigint", "unsigned tinyint", "unsigned smallint", "unsigned mediumint",
		"unsigned", "unsigned big int",
		"real", "double", "float", "decimal", "numeric":
		return true
	}
	return false
}

// formatFilterValue quotes a literal for use in a WHERE fragment, leaving
// numeric values bare so they don't get string-coerced on MySQL.
func formatFilterValue(value, dbType string) string {
	if isNumericType(dbType) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// buildQuickFilter constructs a single-column WHERE fragment for a cell value.
// negate=true builds a NULL-safe "not equal" form so that pressing `!` on a
// nullable column does not silently drop NULL rows (the plain `!=` operator
// treats NULL comparisons as unknown). A NULL cell value maps to IS NULL /
// IS NOT NULL instead of an equality comparison.
func buildQuickFilter(colName, value, dbType string, negate bool) string {
	if value == "NULL" {
		if negate {
			return fmt.Sprintf("%s IS NOT NULL", colName)
		}
		return fmt.Sprintf("%s IS NULL", colName)
	}
	v := formatFilterValue(value, dbType)
	if negate {
		return fmt.Sprintf("(%s != %s OR %s IS NULL)", colName, v, colName)
	}
	return fmt.Sprintf("%s = %s", colName, v)
}

// removeColumnFilters drops every filter fragment that belongs to colName.
// It recognizes all generated shapes: `col = ...`, `col IN (...)`,
// `col IS [NOT] NULL`, and the NULL-safe negate form `(col != ... OR ...)`,
// so quick filters and the value picker interoperate cleanly.
func removeColumnFilters(filters []string, colName string) []string {
	prefix := colName + " "
	groupPrefix := "(" + colName + " "
	out := make([]string, 0, len(filters))
	for _, f := range filters {
		if strings.HasPrefix(f, prefix) || strings.HasPrefix(f, groupPrefix) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// quickFilterCell builds a single-column filter from the cursor cell value
// and applies it, replacing any existing filter on that column. negate=false
// keeps rows matching the value (`*`); negate=true hides them (`!`).
func (m *Model) quickFilterCell(negate bool) tea.Cmd {
	if !m.canFilter() || m.results.NumRows() == 0 {
		return nil
	}
	col := m.results.CursorCol()
	colName := m.results.ColumnName(col)
	if colName == "" {
		return nil
	}
	value := m.results.CursorCellValue()
	if value == "" {
		return nil
	}
	dbType := m.results.ColumnType(col)

	m.filters = removeColumnFilters(m.filters, colName)
	m.filters = append(m.filters, buildQuickFilter(colName, value, dbType, negate))

	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// filterByMarks builds a WHERE pk IN (...) clause from the marked rows and
// applies it as a filter, then clears the marks. Marks are consumed because
// the resulting filter now represents them in the active result set.
func (m *Model) filterByMarks() tea.Cmd {
	if !m.canFilter() || !m.results.IsEditable() {
		return nil
	}
	tuples := m.results.MarkedPKs()
	if len(tuples) == 0 {
		return nil
	}
	pkNames := m.results.PKColumns()
	pkTypes := m.results.PKTypes()

	clause := buildPKInClause(pkNames, pkTypes, tuples)

	// Replace any existing filter on the PK column(s) so pressing F twice
	// doesn't stack redundant clauses.
	for _, pk := range pkNames {
		m.filters = removeColumnFilters(m.filters, pk)
	}
	// For composite PKs the clause starts with "(pk1, pk2) IN ..."; remove
	// any stale composite clause on the same leading column.
	if len(pkNames) > 1 {
		m.filters = removeColumnFilters(m.filters, "("+pkNames[0]+" ")
	}
	m.filters = append(m.filters, clause)
	m.results.ClearMarks()

	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// buildPKInClause constructs a type-correct IN filter from PK tuples.
// Single PK:  pk IN (v1, v2, ...)
// Composite:  (pk1, pk2) IN ((v1a, v2a), (v1b, v2b))
func buildPKInClause(pkNames, pkTypes []string, tuples [][]string) string {
	if len(pkNames) == 1 {
		parts := make([]string, len(tuples))
		for i, t := range tuples {
			v := ""
			if len(t) > 0 {
				v = t[0]
			}
			parts[i] = formatFilterValue(v, pkTypes[0])
		}
		return fmt.Sprintf("%s IN (%s)", pkNames[0], strings.Join(parts, ", "))
	}
	rows := make([]string, len(tuples))
	for i, t := range tuples {
		vals := make([]string, len(pkNames))
		for j := range pkNames {
			v := ""
			if j < len(t) {
				v = t[j]
			}
			vals[j] = formatFilterValue(v, firstOr(pkTypes, j, ""))
		}
		rows[i] = "(" + strings.Join(vals, ", ") + ")"
	}
	return fmt.Sprintf("(%s) IN (%s)", strings.Join(pkNames, ", "), strings.Join(rows, ", "))
}

// firstOr returns types[i] if in range, else fallback.
func firstOr(types []string, i int, fallback string) string {
	if i >= 0 && i < len(types) {
		return types[i]
	}
	return fallback
}

// pluralIf returns suffix (e.g. "es") when cond is true, "" otherwise.
// Used for simple pluralization like "1 match" vs "2 matches".
func pluralIf(cond bool, suffix string) string {
	if cond {
		return suffix
	}
	return ""
}

// compactFilter shortens a raw WHERE fragment for display in the status bar.
// IN (...) lists collapse to "col ∈ (n)" and NULL-safe negates collapse to
// "col ≠ v", so a handful of filters fit on one line.
func compactFilter(f string) string {
	// NULL-safe negate: (col != v OR col IS NULL) → col ≠ v
	if strings.HasPrefix(f, "(") && strings.HasSuffix(f, ")") {
		inner := f[1 : len(f)-1]
		if orIdx := strings.Index(inner, " OR "); orIdx >= 0 {
			left := inner[:orIdx]
			if eq := strings.Index(left, " != "); eq >= 0 {
				return left[:eq] + " ≠" + left[eq+3:]
			}
		}
	}
	// IN (...) → col ∈ (n)
	if i := strings.Index(f, " IN ("); i >= 0 && strings.HasSuffix(f, ")") {
		inner := f[i+len(" IN (") : len(f)-1]
		prefix := f[:i]
		var count int
		if strings.HasPrefix(prefix, "(") {
			// Composite PK: (pk1,pk2) IN ((..),(..)) — count tuples, not values.
			count = strings.Count(inner, "), (") + 1
		} else {
			count = strings.Count(inner, ",") + 1
		}
		return prefix + fmt.Sprintf(" ∈ (%d)", count)
	}
	// IS NULL / IS NOT NULL / equality → keep as-is (short enough).
	return f
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeString(s []string, v string) []string {
	for i, x := range s {
		if x == v {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}
