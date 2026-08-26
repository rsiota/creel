package ui

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

// copyCursorCell writes the cell under the cursor to the clipboard and starts
// the flash/confirmation feedback. Shared by the yy chord and :copy.
func (m *Model) copyCursorCell() tea.Cmd {
	if !m.results.HasResult() || m.results.NumRows() == 0 {
		m.schemaMsg = "nothing to copy"
		return nil
	}
	if err := clipboard.WriteAll(m.results.CursorCellValue()); err != nil {
		m.schemaMsg = "clipboard: " + err.Error()
		return nil
	}
	m.results.StartCopyFeedback()
	return copyFeedbackCmd()
}

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

// explorerExpand (→) opens the selected node: if collapsed, expand it (loading
// children lazily if needed); if already expanded, move the cursor to its first
// child. Synthetic nodes are inert.
func (m *Model) explorerExpand() tea.Cmd {
	node := m.explorer.selectedNode()
	if node == nil || node.synthetic {
		return nil
	}
	if node.expanded {
		m.explorer.cursorToFirstChild(node)
		return nil
	}
	if node.depth >= maxExplorerDepth {
		node.err = "max depth reached"
		return nil
	}
	node.expanded = true
	if node.children == nil {
		node.loading = true
		return m.loadExplorerChildren(node)
	}
	return nil
}

// explorerCollapse (←) folds the selected node; if already collapsed, moves the
// cursor to its parent so ← doubles as "go up a level".
func (m *Model) explorerCollapse() {
	node := m.explorer.selectedNode()
	if node == nil {
		return
	}
	if node.expanded {
		node.expanded = false
		return
	}
	if node.parent != nil {
		m.explorer.cursorToNode(node.parent)
	}
}

// explorerActivate (Enter) opens the selected node's data in the grid: a row
// node navigates to that exact row; an edge node opens its full related set.
// The current query is pushed onto the stack so ←/g b returns, and the tree
// re-roots to the new focused row once the grid query completes
// (queryExecutedMsg → loadExplorer). Inert for synthetic nodes and rows with
// no drillable identity.
func (m *Model) explorerActivate() tea.Cmd {
	node := m.explorer.selectedNode()
	if node == nil || node.synthetic || node.drillQuery == "" {
		return nil
	}
	m.pushQueryStack()
	m.applyExplorerDrill(node.drillQuery)
	return m.runPageQuery()
}

// explorerOpenInTab (t) runs the selected node's drill query in a new results
// tab so the parent grid stays on the previous tab. Enter still re-roots here.
func (m *Model) explorerOpenInTab() tea.Cmd {
	node := m.explorer.selectedNode()
	if node == nil || node.synthetic || node.drillQuery == "" {
		return nil
	}
	m.addTab(generateTabTitle(node.drillQuery), node.drillQuery)
	m.applyExplorerDrill(node.drillQuery)
	return m.runPageQuery()
}

func (m *Model) applyExplorerDrill(query string) {
	m.editor.SetValue(query)
	m.lastQuery = query
	m.baseQuery = ""
	m.filters = nil
	m.sortCol = ""
	m.sortDir = ""
	m.page = 0
	m.results.SetSearchMatcher(nil)
	m.lastSearch = ""
	m.explorer.markLoading()
}

// explorerInsertRelated (A) starts an insert into an inbound relationship's
// child table with the FK column prefilled from the parent row. Select an
// inbound edge (e.g. orders under users) and press A — the inspector opens
// insert mode for the child table while the grid stays on the parent.
func (m *Model) explorerInsertRelated() tea.Cmd {
	if m.isReadOnly() {
		m.schemaMsg = "read-only — cannot insert"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	node := m.explorer.selectedNode()
	if node == nil {
		return nil
	}
	// Allow A on an inbound edge, or on a synthetic "(no rows)" child of one.
	edge := node
	if !node.isEdge() {
		if node.parent != nil && node.parent.isEdge() && node.synthetic {
			edge = node.parent
		} else {
			m.schemaMsg = "select an inbound relationship (child table) to insert related"
			return nil
		}
	}
	if edge.edge.dir != relInbound {
		m.schemaMsg = "insert related works on inbound edges (child tables)"
		return nil
	}
	if edge.filterVal == "" || edge.filterVal == "NULL" {
		m.schemaMsg = "cannot insert related: parent key is NULL"
		return nil
	}

	child := edge.edge.targetTable
	target, err := m.newInsertTarget(child)
	if err != nil {
		m.schemaMsg = fmt.Sprintf("cannot insert related: %v", err)
		return nil
	}
	m.insertTarget = target
	m.startInsertWithValues(map[string]string{edge.edge.targetColumn: edge.filterVal})
	if !m.inspector.IsInserting() {
		m.insertTarget = nil
		m.schemaMsg = "cannot insert related"
		return nil
	}
	m.schemaMsg = fmt.Sprintf("insert related into %s — %s prefilled", child, edge.edge.targetColumn)
	return nil
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
	if m.explorer.IsVisible() {
		m.explorer.markLoading()
	}
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

// prepareResultsEdit dismisses the inspector when it is safe so the results
// panel can own a cell edit (keyboard e/i/E or mouse double-click). Returns
// false when the inspector is mid-edit or mid-insert and must not be closed.
func (m *Model) prepareResultsEdit() bool {
	if !m.inspector.IsVisible() {
		return true
	}
	if m.inspector.IsEditing() || m.inspector.IsInserting() {
		return false
	}
	m.inspector.Hide()
	if m.focus == FocusInspector {
		m.focus = FocusResults
	}
	m.layoutWorkspace()
	m.applyFocus()
	return true
}

// startResultsCellEdit enters inline edit mode on the cell under the results
// cursor, opening the expanded cell-edit popup instead when the value is
// wider than its column. It mirrors the "e"/"i" keyboard binding. When the
// inspector is open (and not mid-edit/insert), it is closed first so the grid
// can own the edit — matching mouse double-click behaviour.
func (m *Model) startResultsCellEdit() tea.Cmd {
	if !m.results.IsEditable() || !m.results.HasPrimaryKey() {
		return nil
	}
	if !m.prepareResultsEdit() {
		return nil
	}
	row, col := m.results.CursorRow(), m.results.CursorCol()
	// Binary cells open the view-only summary (with :saveblob hint) instead
	// of the text editor — editing would corrupt the value.
	if m.results.IsBlobCell(row, col) {
		return m.openCellEditPopup(row, col)
	}
	if m.results.IsCellTruncated(row, col) {
		return m.openCellEditPopup(row, col)
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
	src := m.inspectorResults()
	col := m.inspector.selectedColumn(src)
	if !m.inspector.IsInserting() && m.results.IsBlobCell(m.results.CursorRow(), col) {
		return m.openCellEditPopup(m.results.CursorRow(), col)
	}
	if !m.inspector.IsInserting() && m.inspector.IsFieldTruncated(src) {
		return m.openCellEditPopup(m.results.CursorRow(), col)
	}
	m.inspector.StartFieldEdit(src)
	return nil
}

// editValueSQLArg maps a staged cell value to the driver argument for UPDATE.
// The "NULL" sentinel and empty strings on datetime columns become SQL NULL;
// other empty strings remain empty strings.
func editValueSQLArg(val, colType string) interface{} {
	if val == "NULL" || (val == "" && db.IsDateTimeType(colType)) {
		return nil
	}
	return val
}

// exSetNull stages SQL NULL for the cursor cell, or for a named column on the
// cursor row (:setnull [column]). Does not flush to the database — use :w.
func (m *Model) exSetNull(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only — cannot edit"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	if !m.results.IsEditable() || !m.results.HasPrimaryKey() {
		m.schemaMsg = "results not editable"
		return nil
	}
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no rows to edit"
		return nil
	}
	if len(args) > 1 {
		m.schemaMsg = "usage: :setnull [column]"
		return nil
	}

	row := m.results.CursorRow()
	col := m.results.CursorCol()
	if len(args) == 1 {
		col = m.resultColumnIndex(args[0])
		if col < 0 {
			m.schemaMsg = fmt.Sprintf("no such column: %s", args[0])
			return nil
		}
	}

	colName := m.results.ColumnName(col)
	if colName == "" {
		m.schemaMsg = "invalid column"
		return nil
	}
	if m.results.isPKColumn(colName) {
		m.schemaMsg = "cannot set primary key to NULL"
		return nil
	}
	if m.results.IsBlobCell(row, col) {
		m.schemaMsg = "binary cell — use :saveblob to export"
		return nil
	}
	if m.results.RowValue(row, col) == "NULL" {
		m.schemaMsg = fmt.Sprintf("%s already NULL", colName)
		return nil
	}

	if m.results.IsEditing() {
		m.results.CancelEdit()
	}
	if m.inspector.IsEditing() {
		m.inspector.CancelEdit()
	}

	m.results.SetDirtyCell(row, col, "NULL")
	m.schemaMsg = fmt.Sprintf("staged NULL on %s — :w to save", colName)
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
		colType string
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
			colType: m.results.columnType(edit.Col),
		})
	}

	return func() tea.Msg {
		driver := conn.Config().Driver
		tx, err := conn.DB().Begin(db.IsolationDefault)
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

			setArg := editValueSQLArg(p.edit.NewValue, p.colType)
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
	src := m.inspectorResults()
	if !m.inspector.IsInserting() || m.connection == nil || !src.IsEditable() {
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}

	conn := m.connection
	table := src.SourceTable()
	columns := src.TableColumns()
	values := insertValuesByName(src, m.inspector.InsertValues())

	driver := conn.Config().Driver
	query, args, err := buildInsertQuery(driver, table, columns, values, nil)
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

// discardResultsEdits discards staged cell edits in the results panel,
// staging the y/enter confirmation when confirm_destructive is on (unless force
// is set). Reports whether there was anything to discard (it discarded, or
// staged a confirm). Shared by the results D key and :discard so the two
// surfaces can't drift — the confirm gating lives here, not in each caller.
func (m *Model) discardResultsEdits(force bool) bool {
	if !m.results.IsEditable() || !m.results.HasDirtyCells() {
		return false
	}
	if !force && m.confirmDestructive() {
		m.discardConfirm = true
		return true
	}
	m.results.DiscardEdits()
	return true
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
		q, args, err := buildInsertQuery(conn.Config().Driver, table, columns, row.Values, row.Blobs)
		if err != nil {
			return func() tea.Msg {
				return cloneResultMsg{table: table, err: err}
			}
		}
		batch = append(batch, pending{query: q, args: args})
	}

	return func() tea.Msg {
		tx, err := conn.DB().Begin(db.IsolationDefault)
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

// copyRowsAsInsert copies the current result rows as INSERT statements to the
// clipboard, shared by the Y key and :copyinsert. Returns the copy-feedback
// cmd when something was copied, nil otherwise (no rows, or nothing generated).
func (m *Model) copyRowsAsInsert() tea.Cmd {
	if m.results.NumRows() == 0 {
		return nil
	}
	sql, count := m.results.CopyAsInsert()
	if count == 0 {
		return nil
	}
	_ = clipboard.WriteAll(sql)
	m.results.StartCopyFeedback()
	if count >= copyAsInsertMaxRows {
		m.exportMsg = fmt.Sprintf("copied %d rows as INSERT (cap %d)", count, copyAsInsertMaxRows)
	} else {
		m.exportMsg = fmt.Sprintf("copied %d rows as INSERT", count)
	}
	return copyFeedbackCmd()
}

// copyRowsDelimited copies the marked rows (or the cursor row when none are
// marked) to the clipboard in a delimited format (default tsv), shared by
// :copyrow. Returns the copy-feedback cmd when something was copied, nil
// otherwise. Fills the gap between :copy (one cell) and copyRowsAsInsert
// (rows as SQL) — the "paste this row into Sheets/Slack" case.
func (m *Model) copyRowsDelimited(format exportFormat) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "nothing to copy"
		return nil
	}
	content, count := m.results.CopyAsDelimited(format)
	if count == 0 {
		m.schemaMsg = "nothing to copy"
		return nil
	}
	_ = clipboard.WriteAll(content)
	m.results.StartCopyFeedback()
	m.exportMsg = fmt.Sprintf("copied %d row%s as %s", count, plural(count), string(format))
	return copyFeedbackCmd()
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
