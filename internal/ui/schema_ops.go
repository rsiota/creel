package ui

import (
	"fmt"
	"strings"

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
	// Binary cells: always view-only, with a short summary and a hint to
	// export via :saveblob. Editing binary as text would corrupt the value.
	if data, ok := m.results.BlobData(row, col); ok {
		val := fmt.Sprintf("Binary value (%s)\n\nUse :saveblob <file> to write these bytes to disk.",
			db.FormatByteSize(len(data)))
		m.cellEdit.Show(val, row, col, colName, true)
		return m.cellEdit.Focus()
	}
	val := m.results.RawRowValue(row, col)
	if val == "NULL" {
		val = ""
	}
	// Open view-only when the results can't be written back: read-only mode
	// (global --readonly or a read-only connection), custom queries, or
	// PK-less views. Otherwise E is an editor for the cell.
	readOnly := !m.results.IsEditable() || !m.results.HasPrimaryKey() || m.isReadOnly()
	m.cellEdit.Show(val, row, col, colName, readOnly)
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
	m.renameColWidthTable(oldName, newName)

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
	m.startInsertWithValues(nil)
}

// startInsertWithValues opens inspector insert mode, optionally prefilling
// fields by column name (used by explorer "insert related"). The explorer
// yields the right slot so the inspector is visible, then comes back after
// save or cancel (restoreExplorerAfterInsert).
func (m *Model) startInsertWithValues(byName map[string]string) {
	if !m.results.IsEditable() || m.results.HasDirtyCells() || m.inspector.IsInserting() {
		return
	}
	if m.explorer.IsVisible() {
		m.restoreExplorerAfterInsert = true
		m.explorer.Hide()
	}
	if !m.inspector.IsVisible() {
		m.inspector.visible = true
	}
	m.inspector.StartInsert()
	if len(byName) > 0 {
		vals := make(map[int]string, len(byName))
		for i := 0; i < m.results.NumCols(); i++ {
			name := m.results.ColumnName(i)
			for k, v := range byName {
				if strings.EqualFold(name, k) {
					vals[i] = v
					break
				}
			}
		}
		m.inspector.SetInsertValues(vals)
	}
	m.focus = FocusInspector
	m.applyFocus()
}

// restoreExplorerPanel puts the docked explorer back in the right slot after
// an insert that hid it. The caller decides whether to reload the tree.
func (m *Model) restoreExplorerPanel() {
	m.restoreExplorerAfterInsert = false
	m.inspector.Hide()
	m.assistant.Hide()
	m.explorer.ShowDocked()
	m.focus = FocusExplorer
	m.layoutWorkspace()
	m.applyFocus()
}

// maybeRestoreExplorerAfterInsert restores the explorer after insert cancel.
func (m *Model) maybeRestoreExplorerAfterInsert() tea.Cmd {
	if !m.restoreExplorerAfterInsert {
		return nil
	}
	m.restoreExplorerPanel()
	m.explorer.markLoading()
	return m.loadExplorer()
}
