package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/db"
)

// exTruncate empties a table (:truncate [table]), defaulting to the current
// table. Shares execTruncate with the sidebar T key. Stages the enter/esc
// confirm dialog when confirm_destructive is on, unless forced (:truncate!).
func (m *Model) exTruncate(name string, force bool) tea.Cmd {
	table := m.resolveDDLTableArg(name)
	if table == "" {
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: truncate disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	if !force && m.confirmDestructive() {
		m.truncateConfirm = table
		return nil
	}
	return m.execTruncate(table)
}

// exDrop drops a table (:drop [table]), defaulting to the current table.
// Shares execDropTable with the sidebar D key. Stages the typed-name confirm
// when confirm_destructive is on, unless forced (:drop!).
func (m *Model) exDrop(name string, force bool) tea.Cmd {
	table := m.resolveDDLTableArg(name)
	if table == "" {
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: drop disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	if !force && m.confirmDestructive() {
		m.dropTableConfirm = table
		m.dropTableInput = ""
		return nil
	}
	return m.execDropTable(table)
}

// exRename renames a table (:rename [old] [new]). With no args (or one), opens
// the rename form for the current/named table (same as sidebar r). With two
// args, renames non-interactively via BuildRenameTableSQL.
func (m *Model) exRename(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: rename disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	switch len(args) {
	case 0:
		table := m.resolveDDLTableArg("")
		if table == "" {
			return nil
		}
		return m.openTableRenameForm(table)
	case 1:
		table := m.resolveDDLTableArg(args[0])
		if table == "" {
			return nil
		}
		return m.openTableRenameForm(table)
	default:
		old := m.resolveTableName(args[0])
		if old == "" {
			m.schemaMsg = fmt.Sprintf("no such table: %s", args[0])
			return nil
		}
		newName := args[1]
		sql, err := db.BuildRenameTableSQL(m.connection.Config().Driver, old, newName, m.tables)
		if err != nil {
			m.schemaMsg = err.Error()
			return nil
		}
		return m.execSchemaDDL(old, sql, db.SchemaRenameTable, newName)
	}
}

// exCreate opens the inline table designer (:create), sharing openCreateTableForm
// with the sidebar N key.
func (m *Model) exCreate() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: create table disabled"
		return nil
	}
	return m.openCreateTableForm()
}

// exAddColumn adds a column to a table (:addcolumn ...), sharing
// openAddColumnFormForTable + execSchemaDDL(SchemaAddColumn) with the sidebar a
// key. With zero or one argument it opens the form for the current/named table
// (like :rename); with three or more — <table> <name> <type> [nullable]
// [default] — it runs ALTER TABLE ADD COLUMN directly. SQL types containing
// spaces (e.g. Postgres "double precision") should use the form, since the
// type is one shell field here.
func (m *Model) exAddColumn(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: add column disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	// 0-1 args: open the form for the current / named table (default-to-current,
	// preferring the sidebar cursor like the a key).
	if len(args) <= 1 {
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		table := m.resolveDDLTableArg(name)
		if table == "" {
			return nil
		}
		return m.openAddColumnFormForTable(table)
	}
	// Two args is ambiguous (table + name, no type) — ask for the type.
	if len(args) < 3 {
		m.schemaMsg = "usage: :addcolumn <table> <name> <type> [nullable] [default]"
		return nil
	}
	// Direct: <table> <name> <type> [nullable] [default].
	table := m.resolveDDLTableArg(args[0])
	if table == "" {
		return nil
	}
	col := db.ColumnDef{
		Name: args[1],
		Type: args[2],
	}
	if len(args) >= 4 {
		nullable, errMsg := parseNullable(args[3])
		if errMsg != "" {
			m.schemaMsg = errMsg
			return nil
		}
		col.NotNull = !nullable
	}
	if len(args) >= 5 && strings.TrimSpace(args[4]) != "" {
		col.HasDefault = true
		col.Default = args[4]
	}
	cols, err := m.connection.DB().TableSchema(table)
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	existing := make([]string, len(cols))
	for i, c := range cols {
		existing[i] = c.Name
	}
	sql, err := db.BuildAddColumnSQL(m.connection.Config().Driver, table, col, existing)
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	return m.execSchemaDDL(table, sql, db.SchemaAddColumn, "")
}

// resolveNameInList picks an entry by EqualFold exact match, then substring.
// Returns "" if nothing matches.
func resolveNameInList(query string, names []string) string {
	if query == "" {
		return ""
	}
	for _, n := range names {
		if strings.EqualFold(n, query) {
			return n
		}
	}
	needle := strings.ToLower(query)
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), needle) {
			return n
		}
	}
	return ""
}

// resolveConnectionName resolves a user-typed connection name against config.
func (m *Model) resolveConnectionName(query string) string {
	if m.config == nil {
		return ""
	}
	names := make([]string, 0, len(m.config.Connections))
	for _, c := range m.config.Connections {
		names = append(names, c.Name)
	}
	return resolveNameInList(query, names)
}

// exConnections opens the connection list (:connections), sharing
// showConnectionList with ctrl+t.
func (m *Model) exConnections() tea.Cmd {
	return m.showConnectionList()
}

// exConnect switches to a named connection (:connect [name] / :c). With no
// argument it opens the connection list like :connections.
func (m *Model) exConnect(args []string) tea.Cmd {
	if len(args) == 0 {
		return m.showConnectionList()
	}
	if m.config == nil || len(m.config.Connections) == 0 {
		m.schemaMsg = "no connections configured"
		return nil
	}
	resolved := m.resolveConnectionName(args[0])
	if resolved == "" {
		m.schemaMsg = fmt.Sprintf("no such connection: %s", args[0])
		return nil
	}
	m.connError = ""
	cmd := m.connectByName(resolved)
	if m.connError != "" {
		m.schemaMsg = m.connError
		m.connError = ""
		return nil
	}
	if !m.dbPicker.IsVisible() {
		m.schemaMsg = "connected: " + resolved
	}
	return cmd
}

// exReconnect rebuilds the active MySQL/Postgres connection (and SSH tunnel)
// in place so a dropped session does not kick the user back to the picker.
func (m *Model) exReconnect() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if !m.needsKeepAlive() {
		m.schemaMsg = "reconnect is for MySQL/Postgres connections"
		return nil
	}
	if m.reconnecting {
		m.schemaMsg = "already reconnecting…"
		return nil
	}
	m.reconnectRetry = false
	return m.reconnectInPlace()
}

// exDB lists or switches databases (:db / :use [database]). Bare opens the
// picker (ctrl+b); with an argument switches directly. SQLite gets an explicit
// message rather than a silent no-op.
func (m *Model) exDB(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	driver := m.connection.Config().Driver
	if driver != db.DriverMySQL && driver != db.DriverPostgres {
		m.schemaMsg = "switching databases is not supported for " + string(driver)
		return nil
	}
	if len(args) == 0 {
		return m.openDatabasePicker(false)
	}
	dbs, err := m.connection.DB().Databases()
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	name := resolveNameInList(args[0], dbs)
	if name == "" {
		m.schemaMsg = fmt.Sprintf("no such database: %s", args[0])
		return nil
	}
	m.connError = ""
	cmd := m.selectDatabase(name)
	if m.connError != "" {
		m.schemaMsg = m.connError
		m.connError = ""
		return nil
	}
	m.schemaMsg = "database: " + name
	return cmd
}

// exSchema lists or switches schemas (:schema [name]). MySQL equates schema
// with database and delegates to :db. Postgres lists/switches search_path.
// SQLite is unsupported.
func (m *Model) exSchema(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	switch m.connection.Config().Driver {
	case db.DriverMySQL:
		return m.exDB(args)
	case db.DriverSQLite:
		m.schemaMsg = "schemas are not supported for sqlite"
		return nil
	case db.DriverPostgres:
		schemas, err := m.connection.Schemas()
		if err != nil {
			m.schemaMsg = err.Error()
			return nil
		}
		if len(args) == 0 {
			if len(schemas) == 0 {
				m.schemaMsg = "no schemas"
				return nil
			}
			cur := m.connection.Config().Schema
			parts := make([]string, 0, len(schemas))
			for _, s := range schemas {
				if cur != "" && strings.EqualFold(s, cur) {
					parts = append(parts, "["+s+"]")
				} else {
					parts = append(parts, s)
				}
			}
			m.schemaMsg = strings.Join(parts, "  ")
			return nil
		}
		name := resolveNameInList(args[0], schemas)
		if name == "" {
			m.schemaMsg = fmt.Sprintf("no such schema: %s", args[0])
			return nil
		}
		m.connError = ""
		cmd := m.selectSchema(name)
		if m.connError != "" {
			m.schemaMsg = m.connError
			m.connError = ""
			return nil
		}
		m.schemaMsg = "schema: " + name
		return cmd
	default:
		m.schemaMsg = "schemas are not supported for " + string(m.connection.Config().Driver)
		return nil
	}
}

// exCreateDatabase creates a database (:createdb <name>), sharing
// execCreateDatabase with the db-picker N key. MySQL/Postgres only; SQLite is
// unsupported (a single file is the database). DDL, so blocked in read-only
// mode and while a transaction is open (it would implicitly commit on
// MySQL/Postgres).
func (m *Model) exCreateDatabase(name string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	driver := m.connection.Config().Driver
	if driver != db.DriverMySQL && driver != db.DriverPostgres {
		m.schemaMsg = "create database is not supported for " + string(driver)
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: create database disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		m.schemaMsg = ":createdb needs a database name"
		return nil
	}
	return m.execCreateDatabase(name)
}

// exDropDatabase drops a database (:dropdb[!] [name]), sharing execDropDatabase
// with the db-picker D key. Defaults to the current database when no name is
// given. Stages the typed-name confirmation when confirm_destructive is on,
// unless forced (:dropdb!). MySQL/Postgres only.
func (m *Model) exDropDatabase(name string, force bool) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	driver := m.connection.Config().Driver
	if driver != db.DriverMySQL && driver != db.DriverPostgres {
		m.schemaMsg = "drop database is not supported for " + string(driver)
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: drop database disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = m.connection.Config().Database
	}
	if name == "" {
		m.schemaMsg = ":dropdb needs a database name"
		return nil
	}
	if !force && m.confirmDestructive() {
		m.dropDBConfirm = name
		m.dropDBInput = ""
		return nil
	}
	return m.execDropDatabase(name)
}

// maxRecentTables caps the in-memory MRU list backing :recent.
const maxRecentTables = 20

// touchRecentTable records name at the front of the MRU list (deduped).
func (m *Model) touchRecentTable(name string) {
	if name == "" {
		return
	}
	out := make([]string, 0, len(m.recentTables)+1)
	out = append(out, name)
	for _, t := range m.recentTables {
		if !strings.EqualFold(t, name) {
			out = append(out, t)
		}
	}
	if len(out) > maxRecentTables {
		out = out[:maxRecentTables]
	}
	m.recentTables = out
}

// openTable browses a table via SELECT * FROM — shared by :goto, sidebar
// enter, mouse open, and :recent <n>. Records the table in the MRU list.
// name may be bare ("users") or qualified ("analytics.events").
func (m *Model) openTable(name string) tea.Cmd {
	schema, table := splitTableRef(name)
	return m.openTableRef(schema, table)
}

// openTableRef browses schema.table (schema empty = active-schema bare name).
// Foreign schemas are quoted so the query does not depend on search_path.
func (m *Model) openTableRef(schema, table string) tea.Cmd {
	if table == "" {
		return nil
	}
	label := table
	from := table
	if schema != "" {
		label = schema + "." + table
		if m.connection != nil {
			from = quoteTableRef(m.connection.Config().Driver, schema, table)
		} else {
			from = label
		}
	}
	m.touchRecentTable(label)
	m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s;", from))
	return m.executeQuery()
}

// splitTableRef splits "schema.table" into parts. Bare names return ("", name).
func splitTableRef(name string) (schema, table string) {
	return db.SplitTableRef(name)
}

// quoteTableRef returns a driver-quoted schema.table (or bare table when schema
// is empty) for use in SELECT ... FROM.
func quoteTableRef(driver db.Driver, schema, table string) string {
	return db.QuoteTableRef(driver, schema, table)
}

// liveRecentTables returns recent table names that still exist in m.tables,
// preserving MRU order.
func (m Model) liveRecentTables() []string {
	if len(m.recentTables) == 0 {
		return nil
	}
	alive := make(map[string]string, len(m.tables)) // lower → canonical
	for _, t := range m.tables {
		alive[strings.ToLower(t)] = t
	}
	var out []string
	for _, t := range m.recentTables {
		if canon, ok := alive[strings.ToLower(t)]; ok {
			out = append(out, canon)
			continue
		}
		// Qualified schema.table: keep if still in the cross-schema cache.
		schema, table := splitTableRef(t)
		if schema == "" || table == "" {
			continue
		}
		for _, cand := range m.schemaTableCache[schema] {
			if strings.EqualFold(cand, table) {
				out = append(out, schema+"."+cand)
				break
			}
		}
	}
	return out
}

// exGoto opens a table by name (:goto users or :goto analytics.events): exact
// (case-insensitive) match first, then a substring fallback, then runs
// SELECT * FROM <table> (qualified when the match is outside the active schema).
func (m *Model) exGoto(name string) tea.Cmd {
	wantSchema, wantTable := splitTableRef(name)
	items := m.sidebarItems()
	target := -1
	for i, it := range items {
		if !it.isTableRow() {
			continue
		}
		if wantSchema != "" {
			if strings.EqualFold(it.schema, wantSchema) && strings.EqualFold(it.text, wantTable) {
				target = i
				break
			}
			continue
		}
		if strings.EqualFold(it.text, wantTable) {
			target = i
			break
		}
	}
	if target < 0 && wantSchema == "" {
		needle := strings.ToLower(wantTable)
		for i, it := range items {
			if it.isTableRow() && strings.Contains(strings.ToLower(it.text), needle) {
				target = i
				break
			}
		}
	}
	if target >= 0 {
		it := items[target]
		m.sidebarCursor = target
		m.sidebarViewAnchored = false
		if it.schema != "" && it.schema != m.currentSchemaName() {
			return m.openTableRef(it.schema, it.text)
		}
		return m.openTable(it.text)
	}
	// Qualified name may be in a collapsed schema section — still open from cache.
	if wantSchema != "" {
		for _, t := range m.schemaTableCache[wantSchema] {
			if strings.EqualFold(t, wantTable) {
				m.ensureSchemaSectionExpanded(wantSchema)
				m.syncSidebarCursorToQualified(wantSchema, t)
				return m.openTableRef(wantSchema, t)
			}
		}
		m.schemaMsg = fmt.Sprintf("no such table: %s", name)
		return nil
	}
	m.schemaMsg = fmt.Sprintf("no such table: %s", name)
	return nil
}

// exBegin starts a manual transaction (:begin [isolation]). While it is
// active, statements run from the editor execute on the tx — so SELECTs see
// the tx's own uncommitted writes — and :commit / :rollback finish it. Cell
// edits / inserts / deletes / DDL are refused for the duration (they use their
// own autocommit path and would commit outside the tx). Refused while
// read-only, while a query is in flight, or when a transaction is already open.
//
// Isolation is optional: `:begin`, `:begin serializable`, `:begin repeatable
// read`, `:begin read committed`, `:begin read uncommitted` (also hyphenated
// or short forms: rr, rc, s, ru). SQLite is effectively always serializable.
//
// begin/commit/rollback run synchronously: they're rare, explicit actions
// that transfer no row data, and doing them inline (rather than as a goroutine
// command) keeps the tx lifecycle single-threaded. The queryRunning guard
// ensures no in-flight query goroutine is touching the tx when they run.
func (m *Model) exBegin(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: transactions disabled"
		return nil
	}
	if m.tx != nil {
		m.schemaMsg = "transaction already in progress — use :commit or :rollback"
		return nil
	}
	if m.queryRunning {
		m.schemaMsg = "wait for the running query to finish"
		return nil
	}
	level, err := db.ParseIsolation(strings.Join(args, " "))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	tx, err := m.connection.DB().Begin(level)
	if err != nil {
		m.schemaMsg = "begin failed: " + err.Error()
		return nil
	}
	m.tx = tx
	m.txIsolation = level
	if level == db.IsolationDefault {
		m.schemaMsg = "transaction started — :commit or :rollback to finish"
	} else {
		m.schemaMsg = fmt.Sprintf("transaction started (%s) — :commit or :rollback to finish", level)
	}
	return nil
}

// exCommit commits the active manual transaction (:commit). The displayed
// results are left as-is; re-run a SELECT (or ctrl+r) to see the committed
// state. We deliberately do NOT auto-re-run the statement under the cursor —
// after a write that would re-execute it outside the tx (a double apply).
func (m *Model) exCommit() tea.Cmd {
	if m.tx == nil {
		m.schemaMsg = "no transaction in progress"
		return nil
	}
	if m.queryRunning {
		m.schemaMsg = "wait for the running query to finish"
		return nil
	}
	if err := m.tx.Commit(); err != nil {
		m.schemaMsg = "commit failed: " + err.Error()
		m.tx = nil // don't reuse a (likely) dead tx
		m.txIsolation = db.IsolationDefault
		return nil
	}
	m.tx = nil
	m.txIsolation = db.IsolationDefault
	m.schemaMsg = "transaction committed"
	return nil
}

// exRollback discards the active manual transaction (:rollback).
func (m *Model) exRollback() tea.Cmd {
	if m.tx == nil {
		m.schemaMsg = "no transaction in progress"
		return nil
	}
	if m.queryRunning {
		m.schemaMsg = "wait for the running query to finish"
		return nil
	}
	if err := m.tx.Rollback(); err != nil {
		m.schemaMsg = "rollback failed: " + err.Error()
		m.tx = nil
		m.txIsolation = db.IsolationDefault
		return nil
	}
	m.tx = nil
	m.txIsolation = db.IsolationDefault
	m.schemaMsg = "transaction rolled back"
	return nil
}

// txnBlocksWrite reports whether a manual transaction is active and, if so,
// sets a transient status-bar message explaining why the write was refused.
// Cell edits, inserts, deletes, and DDL each use their own autocommit path; if
// allowed during a transaction they'd commit outside it, surprising the user
// (and on MySQL/PG, DDL would implicitly commit the whole tx). Blocking them
// keeps transaction semantics honest.
func (m *Model) txnBlocksWrite() bool {
	if m.tx != nil {
		m.schemaMsg = "transaction active — :commit or :rollback before editing"
		return true
	}
	return false
}
