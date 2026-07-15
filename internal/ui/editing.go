package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/db"
)

func (m *Model) pushQueryStack() {
	m.queryStack = append(m.queryStack, queryStackEntry{
		query:     m.lastQuery,
		page:      m.page,
		cursorRow: m.results.CursorRow(),
		cursorCol: m.results.CursorCol(),
	})
}

func (m *Model) followForeignKey() tea.Cmd {
	fk, ok := m.results.ForeignKeyAtCursor()
	if !ok {
		return nil
	}
	val := m.results.CursorCellValue()
	if val == "" || val == "NULL" {
		return nil
	}

	m.pushQueryStack()
	query := buildForeignKeyQuery(fk.RefTable, fk.RefColumn, val)
	m.editor.SetValue(query)
	m.lastQuery = query
	m.baseQuery = ""
	m.filters = nil
	m.sortCol = ""
	m.sortDir = ""
	m.page = 0
	m.results.SetSearchMatcher(nil)
	m.lastSearch = ""
	return m.runPageQuery()
}

func (m *Model) goBackQuery() tea.Cmd {
	if len(m.queryStack) == 0 {
		return nil
	}
	entry := m.queryStack[len(m.queryStack)-1]
	m.queryStack = m.queryStack[:len(m.queryStack)-1]
	m.editor.SetValue(entry.query)
	m.lastQuery = entry.query
	m.baseQuery = ""
	m.filters = nil
	m.sortCol = ""
	m.sortDir = ""
	m.page = entry.page
	m.results.SetSearchMatcher(nil)
	m.lastSearch = ""
	m.restoreCursor = true
	m.restoreCursorRow = entry.cursorRow
	m.restoreCursorCol = entry.cursorCol
	return m.runPageQuery()
}

// detectEditability checks if the query is a simple "SELECT * FROM <table>"
// and, if so, sets up the results table for inline editing by fetching the
// primary keys. Non-pointer because it only touches Model fields directly.
func (m *Model) detectEditability(query string) {
	m.results.ClearEditable()

	// Must be a SELECT from a single table with no JOIN/GROUP BY.
	table := parseSimpleSelectTable(query)
	if table == "" || m.connection == nil {
		return
	}

	// Verify the table exists.
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

	pkCols, err := m.connection.DB().PrimaryKeys(table)
	if err != nil {
		return
	}

	// Row updates and deletes need PK columns in the result set. Inserts only
	// need table metadata, so tables without a primary key stay insertable.
	if len(pkCols) > 0 {
		colSet := make(map[string]bool)
		for _, c := range m.results.columns {
			colSet[strings.ToLower(c)] = true
		}
		for _, pk := range pkCols {
			if !colSet[strings.ToLower(pk)] {
				return
			}
		}
	}

	m.results.SetEditable(table, pkCols)
	if cols, err := m.connection.DB().TableColumnInfo(table); err == nil {
		m.results.SetTableColumns(cols)
	}
}

// parseSimpleSelectTable extracts the table name from a query of the form
// "SELECT ... FROM <table>" (no JOIN, no subquery). Returns "" if the query
// is not a simple single-table SELECT.
func parseSimpleSelectTable(query string) string {
	upper := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(upper, "SELECT") {
		return ""
	}

	// Find "FROM " as a word boundary.
	fromIdx := strings.Index(upper, " FROM ")
	if fromIdx == -1 {
		return ""
	}

	afterFrom := strings.TrimSpace(query[fromIdx:]) // "FROM <table> ..."
	if !strings.EqualFold(afterFrom[:4], "FROM") {
		return ""
	}
	rest := strings.TrimSpace(afterFrom[4:])

	// Reject if there's a JOIN, subquery, or GROUP BY clause.
	restUpper := strings.ToUpper(rest)
	for _, banned := range []string{" JOIN ", " GROUP BY ", " HAVING ", " UNION ", "(", ","} {
		if strings.Contains(restUpper, banned) {
			return ""
		}
	}

	// Extract the table name, handling quoted identifiers.
	rest = strings.TrimSpace(rest)
	if len(rest) > 0 && (rest[0] == '"' || rest[0] == '`' || rest[0] == '[') {
		// Quoted identifier — find closing quote/bracket.
		closeCh := rest[0]
		if rest[0] == '[' {
			closeCh = ']'
		}
		endIdx := strings.IndexByte(rest[1:], closeCh)
		if endIdx == -1 {
			return ""
		}
		return rest[1 : 1+endIdx]
	}

	// Unquoted: the table name is the first token.
	for _, terminator := range []string{" ", ";", "\n", "\t"} {
		if idx := strings.Index(rest, terminator); idx != -1 {
			rest = rest[:idx]
		}
	}
	if rest == "" {
		return ""
	}
	return rest
}

// replaceSimpleSelectTable rewrites the table name in a simple SELECT ... FROM query.
func replaceSimpleSelectTable(query, oldName, newName string) string {
	if parseSimpleSelectTable(query) != oldName {
		return query
	}
	upper := strings.ToUpper(query)
	fromIdx := strings.Index(upper, " FROM ")
	if fromIdx == -1 {
		return query
	}
	afterFromStart := fromIdx + len(" FROM ")
	rest := query[afterFromStart:]
	restTrim := strings.TrimSpace(rest)

	if len(restTrim) > 0 && (restTrim[0] == '"' || restTrim[0] == '`' || restTrim[0] == '[') {
		closeCh := restTrim[0]
		if restTrim[0] == '[' {
			closeCh = ']'
		}
		endIdx := strings.IndexByte(restTrim[1:], closeCh)
		if endIdx == -1 {
			return query
		}
		suffix := restTrim[1+endIdx+1:]
		newIdent := string(restTrim[0]) + newName + string(closeCh)
		return query[:afterFromStart] + newIdent + suffix
	}

	tableEnd := len(restTrim)
	for _, term := range []string{" ", ";", "\n", "\t"} {
		if idx := strings.Index(restTrim, term); idx != -1 && idx < tableEnd {
			tableEnd = idx
		}
	}
	return query[:afterFromStart] + newName + restTrim[tableEnd:]
}

// startResultsCellEdit enters inline edit mode on the cell under the results
// cursor, opening the expanded cell-edit popup instead when the value is
// wider than its column. It mirrors the "e"/"i" keyboard binding and is a
// no-op when the results are not editable, have no primary key, or when the
// inspector panel is open (where edits happen in the inspector instead).
func (m *Model) startResultsCellEdit() tea.Cmd {
	if !m.results.IsEditable() || !m.results.HasPrimaryKey() {
		return nil
	}
	if m.inspector.IsVisible() {
		return nil
	}
	if m.results.IsCellTruncated(m.results.CursorRow(), m.results.CursorCol()) {
		return m.openCellEditPopup(m.results.CursorRow(), m.results.CursorCol())
	}
	m.results.StartEdit()
	return nil
}

// startInspectorFieldEdit begins editing the inspector field at the current
// field cursor, opening the expanded cell-edit popup instead when the value
// is wider than the field box. It mirrors the inspector "e"/"i" keyboard
// binding. Returns nil when the inspector is not visible.
func (m *Model) startInspectorFieldEdit() tea.Cmd {
	if !m.inspector.IsVisible() {
		return nil
	}
	col := m.inspector.selectedColumn(m.results)
	if !m.inspector.IsInserting() && m.inspector.IsFieldTruncated(m.results) {
		return m.openCellEditPopup(m.results.CursorRow(), col)
	}
	m.inspector.StartFieldEdit(m.results)
	return nil
}

// saveEdits writes all pending dirty cells to the database using parameterized
// UPDATE queries, wrapped in a single transaction so the batch is atomic.
func (m *Model) saveEdits() tea.Cmd {
	if !m.results.HasDirtyCells() || m.connection == nil {
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}

	conn := m.connection
	table := m.results.SourceTable()
	pkCols := m.results.PKColumns()
	colNames := make([]string, len(m.results.columns))
	copy(colNames, m.results.columns)
	edits := m.results.DirtyCells()

	// Pre-resolve PK column indices and values for each dirty row.
	type rowData struct {
		edit    CellEdit
		pkVals  []string
		colName string
	}
	var pending []rowData

	for _, edit := range edits {
		colName := ""
		if edit.Col >= 0 && edit.Col < len(colNames) {
			colName = colNames[edit.Col]
		}
		if colName == "" {
			continue
		}

		var pkVals []string
		for _, pk := range pkCols {
			for i, cn := range colNames {
				if strings.EqualFold(cn, pk) {
					pkVals = append(pkVals, m.results.rows[edit.Row][i])
					break
				}
			}
		}
		if len(pkVals) != len(pkCols) {
			continue
		}

		pending = append(pending, rowData{
			edit:    edit,
			pkVals:  pkVals,
			colName: colName,
		})
	}

	return func() tea.Msg {
		driver := conn.Config().Driver
		tx, err := conn.DB().Begin()
		if err != nil {
			return saveResultMsg{saved: 0, err: err}
		}
		saved := 0
		for _, p := range pending {
			// Build: UPDATE <table> SET <col> = ? WHERE <pk1> = ? AND <pk2> = ?
			var b strings.Builder
			phIdx := 0
			ph := func() string {
				phIdx++
				if driver == db.DriverPostgres {
					return fmt.Sprintf("$%d", phIdx)
				}
				return "?"
			}
			fmt.Fprintf(&b, "UPDATE %s SET %s = %s", table, p.colName, ph())
			for i, pk := range pkCols {
				if i == 0 {
					b.WriteString(" WHERE ")
				} else {
					b.WriteString(" AND ")
				}
				fmt.Fprintf(&b, "%s = %s", pk, ph())
			}

			var setArg interface{} = p.edit.NewValue
			if p.edit.NewValue == "NULL" {
				setArg = nil
			}
			args := []interface{}{setArg}
			for _, v := range p.pkVals {
				args = append(args, v)
			}

			if _, err := tx.Exec(b.String(), args...); err != nil {
				_ = tx.Rollback()
				return saveResultMsg{saved: 0, err: err}
			}
			saved++
		}
		if err := tx.Commit(); err != nil {
			return saveResultMsg{saved: 0, err: err}
		}
		return saveResultMsg{saved: saved}
	}
}

// saveChanges saves pending inserts or cell edits.
func (m *Model) saveChanges() tea.Cmd {
	if m.inspector.IsEditing() {
		col, val, ok := m.inspector.CommitFieldEdit()
		if ok && !m.inspector.IsInserting() {
			m.results.SetDirtyCell(m.results.CursorRow(), col, val)
		}
	}
	if m.inspector.IsInserting() {
		return m.saveInsert()
	}
	return m.saveEdits()
}

// saveInsert writes a new row from inspector insert mode.
func (m *Model) saveInsert() tea.Cmd {
	if !m.inspector.IsInserting() || m.connection == nil || !m.results.IsEditable() {
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}

	conn := m.connection
	table := m.results.SourceTable()
	columns := m.results.TableColumns()
	values := insertValuesByName(m.results, m.inspector.InsertValues())

	driver := conn.Config().Driver
	query, args, err := buildInsertQuery(driver, table, columns, values)
	if err != nil {
		return func() tea.Msg {
			return insertResultMsg{err: err}
		}
	}

	return func() tea.Msg {
		_, err := conn.DB().Exec(query, args...)
		return insertResultMsg{err: err}
	}
}

// cloneRows duplicates marked rows (or the cursor row) by building and
// executing INSERT statements. Auto-increment PK values are stripped so
// the database assigns new IDs. All clones run in a single transaction;
// if any fails, all changes are rolled back.
func (m *Model) cloneRows() tea.Cmd {
	if !m.results.IsEditable() || m.results.NumRows() == 0 || m.connection == nil {
		return nil
	}

	table, columns, rows := m.results.CloneRowsData()
	if table == "" || len(rows) == 0 {
		return nil
	}

	conn := m.connection
	type pending struct {
		query string
		args  []interface{}
	}
	var batch []pending
	for _, row := range rows {
		q, args, err := buildInsertQuery(conn.Config().Driver, table, columns, row.Values)
		if err != nil {
			return func() tea.Msg {
				return cloneResultMsg{table: table, err: err}
			}
		}
		batch = append(batch, pending{query: q, args: args})
	}

	return func() tea.Msg {
		tx, err := conn.DB().Begin()
		if err != nil {
			return cloneResultMsg{table: table, err: err}
		}
		cloned := 0
		for _, p := range batch {
			if _, err := tx.Exec(p.query, p.args...); err != nil {
				_ = tx.Rollback()
				return cloneResultMsg{table: table, count: 0, err: err}
			}
			cloned++
		}
		if err := tx.Commit(); err != nil {
			return cloneResultMsg{table: table, count: 0, err: err}
		}
		return cloneResultMsg{table: table, count: cloned}
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// execTruncate removes all rows from a table asynchronously.
func (m *Model) execTruncate(table string) tea.Cmd {
	if m.connection == nil || table == "" {
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	conn := m.connection
	query := buildTruncateQuery(conn.Config().Driver, table)
	return func() tea.Msg {
		_, err := conn.DB().Exec(query)
		return truncateResultMsg{table: table, err: err}
	}
}

// execDeleteRows removes specific rows by PK asynchronously.
func (m *Model) execDeleteRows(table, query string, count int) tea.Cmd {
	if m.connection == nil || table == "" || query == "" {
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		res, err := conn.DB().Exec(query)
		if err != nil {
			return deleteRowsResultMsg{table: table, err: err}
		}
		deleted := int(res.RowsAffected)
		if deleted == 0 {
			deleted = count // fallback if driver reports 0
		}
		return deleteRowsResultMsg{table: table, count: deleted, err: nil}
	}
}

// startDeleteRows prepares a row deletion confirmation. If rows are marked,
// those are targeted; otherwise the cursor row is targeted. The built DELETE
// query and metadata are stored in confirmation fields for the modal handler.
// When confirm_destructive is disabled the DELETE runs immediately and the
// returned command drives it; otherwise the fields are staged and nil is
// returned.
func (m *Model) startDeleteRows() tea.Cmd {
	if !m.results.IsEditable() || m.results.NumRows() == 0 {
		return nil
	}
	pkNames := m.results.PKColumns()
	if len(pkNames) == 0 {
		return nil
	}
	table := m.results.SourceTable()
	pkTypes := m.results.PKTypes()

	var count int
	var query string
	if m.results.MarkCount() > 0 {
		tuples := m.results.MarkedPKs()
		count = len(tuples)
		query = buildDeleteQuery(table, pkNames, pkTypes, tuples)
	} else {
		tuple := m.results.CursorPKTuple()
		if tuple == nil {
			return nil
		}
		count = 1
		query = buildDeleteQuery(table, pkNames, pkTypes, [][]string{tuple})
	}

	if m.confirmDestructive() {
		m.deleteRowsConfirmTable = table
		m.deleteRowsConfirmQuery = query
		m.deleteRowsConfirmCount = count
		return nil
	}
	return m.execDeleteRows(table, query, count)
}

// commitVisualMarks marks every row in the visual selection range.
func (m *Model) commitVisualMarks() {
	if !m.results.visualActive {
		return
	}
	lo, hi := m.results.VisualRange()
	for row := lo; row <= hi; row++ {
		m.results.cursorRow = row
		m.results.ToggleMark()
	}
	// Restore cursor to the end of the range so the user sees where they left off.
	if m.results.visualAnchor > lo {
		m.results.cursorRow = lo
	} else {
		m.results.cursorRow = hi
	}
	m.results.ensureCursorVisible()
}
