package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

// execSchemaDDL runs a pending schema statement asynchronously.
func (m *Model) execSchemaDDL(table, query string, action db.SchemaAction, newTable string) tea.Cmd {
	if m.connection == nil || table == "" || query == "" {
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		_, err := conn.DB().Exec(query)
		return schemaResultMsg{table: table, newTable: newTable, action: action, err: err}
	}
}

// confirmDestructive reports whether destructive-action confirmation dialogs
// are enabled. It defaults to true (the safe choice) when
// settings.confirm_destructive is unset; set confirm_destructive: false in
// config to skip the prompts and run each destructive action immediately
// instead. Every destructive trigger site (drop table/database, truncate,
// delete rows, discard edits, drop column, clear history/bookmarks) consults
// this before staging its confirmation.
func (m *Model) confirmDestructive() bool {
	if m.settings.ConfirmDestructive == nil {
		return true
	}
	return *m.settings.ConfirmDestructive
}

// execDropTable builds and runs DROP TABLE asynchronously. Shared by the
// typed drop-table confirmation flow and the confirm-skipped fast path so the
// confirmed action is identical whether or not the prompt runs.
func (m *Model) execDropTable(table string) tea.Cmd {
	if m.connection == nil || table == "" {
		return nil
	}
	sql, err := db.BuildDropTableSQL(m.connection.Config().Driver, table)
	if err != nil {
		m.schemaMsg = fmt.Sprintf("drop table failed: %v", err)
		return nil
	}
	return m.execSchemaDDL(table, sql, db.SchemaDropTable, "")
}

func (m *Model) setSchemaConfirm(table, sql string, action db.SchemaAction) {
	m.schemaConfirmTable = table
	m.schemaConfirmSQL = sql
	m.schemaConfirmAction = action
}

// execDropDatabase runs DROP DATABASE asynchronously and returns a
// dropDBResultMsg. After the drop, the caller is responsible for refreshing the
// database picker (and reconnecting if the current database was dropped).
func (m *Model) execDropDatabase(name string) tea.Cmd {
	if m.connection == nil || name == "" {
		return nil
	}
	driver := m.connection.Config().Driver
	sql, err := db.BuildDropDatabaseSQL(driver, name)
	if err != nil {
		return func() tea.Msg {
			return dropDBResultMsg{database: name, err: err}
		}
	}
	conn := m.connection
	return func() tea.Msg {
		_, err := conn.DB().Exec(sql)
		return dropDBResultMsg{database: name, err: err}
	}
}

// execCreateDatabase runs CREATE DATABASE asynchronously and returns a
// createDBResultMsg. The caller is responsible for switching to the new
// database after a successful creation.
func (m *Model) execCreateDatabase(name string) tea.Cmd {
	if m.connection == nil || name == "" {
		return nil
	}
	driver := m.connection.Config().Driver
	sql, err := db.BuildCreateDatabaseSQL(driver, name)
	if err != nil {
		return func() tea.Msg {
			return createDBResultMsg{database: name, err: err}
		}
	}
	conn := m.connection
	return func() tea.Msg {
		_, err := conn.DB().Exec(sql)
		return createDBResultMsg{database: name, err: err}
	}
}

func (m *Model) clearSchemaConfirm() {
	m.schemaConfirmSQL = ""
	m.schemaConfirmTable = ""
	m.schemaConfirmAction = ""
}

func schemaChangeMessage(action db.SchemaAction, table string) string {
	switch action {
	case db.SchemaAddColumn:
		return fmt.Sprintf("added column to %s", table)
	case db.SchemaRenameTable:
		return fmt.Sprintf("renamed table %s", table)
	case db.SchemaRenameColumn:
		return fmt.Sprintf("renamed column on %s", table)
	case db.SchemaModifyType:
		return fmt.Sprintf("changed column type on %s", table)
	case db.SchemaModifyNullable:
		return fmt.Sprintf("changed nullable on %s", table)
	case db.SchemaModifyDefault:
		return fmt.Sprintf("changed column default on %s", table)
	case db.SchemaDropColumn:
		return fmt.Sprintf("dropped column from %s", table)
	case db.SchemaDropTable:
		return fmt.Sprintf("dropped table %s", table)
	default:
		return fmt.Sprintf("updated schema on %s", table)
	}
}

// openAddColumnForm opens the add-column overlay for the selected sidebar table.
func (m *Model) openAddColumnForm() tea.Cmd {
	table := m.sidebarSelectedTable()
	if table == "" && m.schemaEditor.IsVisible() {
		table = m.schemaEditor.Table()
	}
	return m.openAddColumnFormForTable(table)
}

// openAddColumnFormForTable opens the add-column overlay for a specific table.
func (m *Model) openAddColumnFormForTable(table string) tea.Cmd {
	if m.connection == nil || table == "" {
		return nil
	}
	cols, err := m.connection.DB().TableSchema(table)
	if err != nil {
		m.connError = err.Error()
		return nil
	}
	existing := make([]string, len(cols))
	for i, c := range cols {
		existing[i] = c.Name
	}
	m.addColumnForm.Show(table, m.connection.Config().Driver, existing)
	m.clearSchemaConfirm()
	return m.addColumnForm.Focus()
}

// openTableRenameForm opens the rename overlay for a sidebar table.
func (m *Model) openTableRenameForm(table string) tea.Cmd {
	if m.connection == nil || table == "" {
		return nil
	}
	m.tableRenameForm.Show(table, m.connection.Config().Driver, m.tables)
	m.clearSchemaConfirm()
	return m.tableRenameForm.Focus()
}

// openCellEditPopup opens the expanded cell editor for a results cell. It is
// used from both the grid (row/col from the results cursor) and the inspector
// (col from the inspector's selected field). The edited value is staged into
// the same dirtyCells pipeline used by the inline editor.
func (m *Model) openCellEditPopup(row, col int) tea.Cmd {
	if row < 0 || row >= m.results.NumRows() || col < 0 || col >= m.results.NumCols() {
		return nil
	}
	colName := m.results.ColumnName(col)
	val := m.results.RowValue(row, col)
	if val == "NULL" {
		val = ""
	}
	m.cellEdit.Show(val, row, col, colName)
	return m.cellEdit.Focus()
}

// openCreateTableForm opens the inline table designer.
func (m *Model) openCreateTableForm() tea.Cmd {
	if m.connection == nil {
		return nil
	}
	m.tableDesigner.Show(m.connection.Config().Driver, m.tables)
	m.clearSchemaConfirm()
	return m.tableDesigner.Focus()
}

// applyTableRename updates sidebar and workspace state after a successful rename.
func (m *Model) applyTableRename(oldName, newName string) {
	if cols, ok := m.expanded[oldName]; ok {
		delete(m.expanded, oldName)
		m.expanded[newName] = cols
	}
	if cols, ok := m.columnCache[oldName]; ok {
		delete(m.columnCache, oldName)
		m.columnCache[newName] = cols
	}

	m.loadTables()
	m.syncSidebarCursorToTable(newName)

	if m.schemaEditor.IsVisible() && m.schemaEditor.Table() == oldName {
		// Reload under new name by closing + reopening; the editor stores the
		// table name at Show time.
		m.schemaEditor.Hide()
	}

	m.results.RenameTableReferences(oldName, newName)

	if parseSimpleSelectTable(m.lastQuery) == oldName {
		m.lastQuery = replaceSimpleSelectTable(m.lastQuery, oldName, newName)
	}
	if parseSimpleSelectTable(m.baseQuery) == oldName {
		m.baseQuery = replaceSimpleSelectTable(m.baseQuery, oldName, newName)
	}
	if m.resultsShowTable(oldName) {
		m.editor.SetValue(m.lastQuery)
	}
}

// openSchemaPanel opens the inline schema editor for the selected sidebar table.
func (m *Model) openSchemaPanel() tea.Cmd {
	if m.connection == nil {
		return nil
	}
	table := m.sidebarSelectedTable()
	if table == "" {
		return nil
	}
	cols, err := m.connection.DB().TableColumnInfo(table)
	if err != nil {
		m.connError = err.Error()
		return nil
	}
	m.schemaEditor.Show(table, m.connection.Config().Driver, cols)
	// Async-load the read-only structure tabs (indexes, FKs, triggers, view).
	return m.loadStructureMetadata(table)
}

// dropCurrentColumn runs the existing drop-column confirmation flow for the
// cursor row in the schema editor.
func (m *Model) dropCurrentColumn() tea.Cmd {
	col, ok := m.schemaEditor.PendingDropColumn()
	if !ok || m.connection == nil {
		return nil
	}
	if msg := GuardColumnAction(db.SchemaDropColumn, col); msg != "" {
		m.schemaEditor.SetNotice(msg)
		return nil
	}
	driver := m.connection.Config().Driver
	sql, err := db.BuildDropColumnSQL(driver, m.schemaEditor.Table(), col.Name, col)
	if err != nil {
		m.schemaEditor.SetNotice(err.Error())
		return nil
	}
	if m.confirmDestructive() {
		m.setSchemaConfirm(m.schemaEditor.Table(), sql, db.SchemaDropColumn)
		return nil
	}
	return m.execSchemaDDL(m.schemaEditor.Table(), sql, db.SchemaDropColumn, "")
}

// reloadSchemaPanel refreshes column metadata when the editor is open.
func (m *Model) reloadSchemaPanel(table string) {
	if !m.schemaEditor.IsVisible() || m.schemaEditor.Table() != table || m.connection == nil {
		return
	}
	cols, err := m.connection.DB().TableColumnInfo(table)
	if err != nil {
		m.schemaEditor.SetNotice(err.Error())
		return
	}
	m.schemaEditor.SetColumns(cols)
	m.schemaEditor.SetNotice("")
}

// refreshTableSchemaSync updates cached sidebar schema for one table.
func (m *Model) refreshTableSchemaSync(table string) Model {
	if m.connection == nil || table == "" {
		return *m
	}
	cols, err := m.connection.DB().TableSchema(table)
	if err != nil {
		return *m
	}
	if _, ok := m.expanded[table]; ok {
		m.expanded[table] = cols
	}
	m.refreshCompletionCandidates()
	return *m
}

// resultsShowTable reports whether the results panel is showing data from table.
func (m Model) resultsShowTable(table string) bool {
	if table == "" {
		return false
	}
	if m.results.SourceTable() == table {
		return true
	}
	if parseSimpleSelectTable(m.lastQuery) == table {
		return true
	}
	return parseSimpleSelectTable(m.baseQuery) == table
}

// startInsert opens inspector new-record mode for the current editable table.
func (m *Model) startInsert() {
	if !m.results.IsEditable() || m.results.HasDirtyCells() || m.inspector.IsInserting() {
		return
	}
	if !m.inspector.IsVisible() {
		m.inspector.visible = true
	}
	m.inspector.StartInsert()
	m.focus = FocusInspector
	m.applyFocus()
}
