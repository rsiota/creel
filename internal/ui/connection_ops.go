package ui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/demo"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
	"github.com/rsiota/creel/internal/secrets"
)

// rollbackTxn rolls back and clears any active manual transaction. It is a
// no-op when none is open. Called at every connection lifecycle boundary
// (switch, disconnect, database change) so an open transaction is never
// orphaned on a closed or replaced connection.
func (m *Model) rollbackTxn() {
	if m.tx != nil {
		_ = m.tx.Rollback()
		m.tx = nil
		m.txIsolation = db.IsolationDefault
	}
}

// connectToDB establishes a connection to the selected database.
func (m *Model) connectToDB() tea.Cmd {
	if m.connList.SelectedIsDemo() {
		return m.openDemoDatabase()
	}
	return m.connectByName(m.connList.SelectedName())
}

// openDemoDatabase materializes (if needed) and opens the bundled sample
// SQLite database. Used from the empty connection-list invitation.
func (m *Model) openDemoDatabase() tea.Cmd {
	path, err := demo.ResolvePath(historyDir())
	if err != nil {
		m.connError = err.Error()
		return nil
	}
	return m.connectWithConfig(db.ConnectionConfig{
		Driver:   db.DriverSQLite,
		Database: path,
		// Name left empty → connectWithConfig uses the file basename for
		// session keying; omitted from the saved-connection MRU.
	})
}

// connectByName opens the named connection from config. On success it replaces
// any existing connection (closing it only after the new one is live), resets
// workspace query state, and for MySQL/Postgres opens the database picker.
// Failures set connError and leave the previous connection untouched.
func (m *Model) connectByName(name string) tea.Cmd {
	connCfg := m.config.GetConnection(name)
	if connCfg == nil {
		m.connError = fmt.Sprintf("connection '%s' not found", name)
		return nil
	}

	dbCfg := connConfigToDB(*connCfg, m.forceReadOnly)

	// Resolve any keychain references ("secret://...") into plaintext for the
	// DB driver. A plaintext value passes through unchanged.
	resolved, err := resolveConnSecrets(dbCfg)
	if err != nil {
		m.connError = err.Error()
		return nil
	}
	return m.connectWithConfig(resolved)
}

// connectWithConfig opens dbCfg, swaps it in as the active connection, and
// enters the workspace. Shared by connectByName (saved connections) and the
// creel -database / -c startup path (ad-hoc or named). Failures set
// connError and leave the previous connection untouched.
func (m *Model) connectWithConfig(dbCfg db.ConnectionConfig) tea.Cmd {
	if dbCfg.Driver == db.DriverSQLite && dbCfg.Database != "" {
		if expanded, err := expandTilde(filepath.Clean(dbCfg.Database)); err == nil {
			dbCfg.Database = expanded
		}
	}
	// Session restore is keyed by connection name; ad-hoc -database launches
	// have none, so fall back to the file/database basename.
	if dbCfg.Name == "" {
		dbCfg.Name = filepath.Base(dbCfg.Database)
	}

	conn, err := db.New(dbCfg)
	if err != nil {
		m.connError = db.FormatConnectError(dbCfg.Driver, err)
		return nil
	}

	if err := conn.Connect(); err != nil {
		m.connError = db.FormatConnectError(dbCfg.Driver, err)
		return nil
	}

	// Swap only after the new connection is live so a failed :connect leaves
	// the previous session intact.
	m.rollbackTxn()
	if m.connection != nil {
		m.saveSession() // capture the previous connection's workspace first
		m.connection.Close()
		m.connection = nil
	}
	m.resetWorkspaceForNewConnection()
	m.connection = conn
	m.connError = ""
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.columnCache = make(map[string][]db.Column)

	// Remember saved connections in the picker MRU. Ad-hoc opens (-database,
	// demo invitation) have no matching config entry and are skipped.
	if m.recentStore != nil && dbCfg.Name != "" && m.config != nil {
		if m.config.GetConnection(dbCfg.Name) != nil {
			_ = m.recentStore.Touch(dbCfg.Name)
		}
	}

	// MySQL/Postgres: always show the database picker (no history of last selection).
	if dbCfg.Driver == db.DriverMySQL || dbCfg.Driver == db.DriverPostgres {
		dbs, err := conn.DB().Databases()
		if err != nil {
			m.connError = err.Error()
			return nil
		}
		m.dbPicker.Show(dbs, true)
		m.layoutWorkspace()
		return m.scheduleKeepAlive()
	}

	m.loadTables()
	m.restoreSession() // reopen tabs/editor buffers from the last visit
	cmd := m.editor.Focus()
	m.layoutWorkspace()
	m.applyFocus()

	return tea.Batch(cmd, m.prefetchSchemas(), m.fetchTableRowCounts(), m.scheduleKeepAlive())
}

// resetWorkspaceForNewConnection clears query/results/tab state after a
// successful connection switch so stale data from the previous session cannot
// linger. Shared by connectByName and showConnectionList.
func (m *Model) resetWorkspaceForNewConnection() {
	m.results.Clear()
	m.results.ClearEditable()
	m.results.SetSearchMatcher(nil)
	m.resultsTabs = []*ResultsTab{NewResultsTab(0, "New Query")}
	m.activeTabID = 0
	m.nextTabID = 1
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	m.inspector.Hide()
	m.dbPicker.Hide()
	m.columnPicker.Hide()
	m.discardConfirm = false
	m.truncateConfirm = ""
	m.deleteRowsConfirmTable = ""
	m.deleteRowsConfirmQuery = ""
	m.deleteRowsConfirmCount = 0
	m.addColumnForm.Hide()
	m.tableRenameForm.Hide()
	m.schemaEditor.Hide()
	m.clearSchemaConfirm()
	m.schemaMsg = ""
	m.lastQuery = ""
	m.clearQueryFailure()
	m.baseQuery = ""
	m.filters = nil
	m.sortCol = ""
	m.sortDir = ""
	m.page = 0
	m.pageMsg = ""
	m.statsMsg = ""
	m.exportMsg = ""
	m.searchMsg = ""
	m.queryStack = nil
	m.expanded = make(map[string][]db.Column)
	m.columnCache = nil
	m.views = nil
	m.recentTables = nil
	m.sidebarFiltering = false
	m.sidebarFilter = ""
	m.editor.CancelCompletion()
	m.colWidthMem = nil
	m.erdPosMem = nil
}

// showConnectionList disconnects (if needed) and returns to the connection
// picker. Shared by ctrl+t, :connections, and bare :connect.
func (m *Model) showConnectionList() tea.Cmd {
	m.saveSession()
	m.stopKeepAlive()
	if m.connection != nil {
		m.rollbackTxn()
		m.connection.Close()
		m.connection = nil
	}
	m.resetWorkspaceForNewConnection()
	m.state = stateConnections
	m.focus = FocusConnections
	m.connError = ""
	m.loadConnections()
	if len(m.config.Connections) > 0 {
		m.connList.StartFilter()
		m.selectRecentConnection() // StartFilter resets cursor; re-apply MRU
	}
	return nil
}

// selectSchema switches the active Postgres schema (search_path), reloading
// tables like selectDatabase. No-op helpers for other drivers live on the
// driver UseSchema implementations.
func (m *Model) selectSchema(name string) tea.Cmd {
	if m.connection == nil || name == "" {
		return nil
	}
	m.rollbackTxn()
	if err := m.connection.UseSchema(name); err != nil {
		m.connError = err.Error()
		return nil
	}
	m.connError = ""
	m.dbPicker.Hide()

	m.expanded = make(map[string][]db.Column)
	m.columnCache = make(map[string][]db.Column)
	m.results.Clear()
	m.results.ClearEditable()
	m.inspector.Hide()
	m.tables = nil
	m.lastQuery = ""
	m.clearQueryFailure()
	m.page = 0
	m.pageMsg = ""
	m.statsMsg = ""
	m.exportMsg = ""
	m.searchMsg = ""
	m.results.SetSearchMatcher(nil)
	m.queryStack = nil
	m.sidebarCursor = 0

	cmd := m.editor.Focus()
	m.loadTables()
	m.layoutWorkspace()
	m.applyFocus()
	return tea.Batch(cmd, m.prefetchSchemas(), m.fetchTableRowCounts())
}

// connConfigToDB maps a stored connection config to the db package's
// ConnectionConfig, merging the global force-read-only flag. Centralized so
// connect and test-connection build identical driver configs.
func connConfigToDB(cfg config.ConnectionConfig, forceReadOnly bool) db.ConnectionConfig {
	return db.ConnectionConfig{
		Name:     cfg.Name,
		Driver:   db.Driver(cfg.Driver),
		Database: cfg.Database,
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
		SSLMode:  cfg.SSLMode,
		Socket:   cfg.Socket,

		SSHHost:       cfg.SSHHost,
		SSHPort:       cfg.SSHPort,
		SSHUser:       cfg.SSHUser,
		SSHPassword:   cfg.SSHPassword,
		SSHKeyPath:    cfg.SSHKeyPath,
		SSHPassphrase: cfg.SSHPassphrase,

		ReadOnly: cfg.ReadOnly || forceReadOnly,
	}
}

// testConnection validates the form and, on success, opens the database in a
// background goroutine to surface the real connection error (auth failure,
// unreachable host, bad database name, …) without saving. The result arrives
// as a connTestResultMsg.
func (m *Model) testConnection() tea.Cmd {
	connCfg, errMsg := m.connForm.EnterPressed()
	if errMsg != "" {
		m.connForm.SetError(errMsg)
		return nil
	}

	// Preserve fields the form does not expose (ssh_passphrase) when editing,
	// so the test exercises the saved config rather than a stripped copy.
	if m.connForm.mode == formModeEdit {
		if existing := m.config.GetConnection(m.connForm.editName); existing != nil {
			connCfg.SSHPassphrase = existing.SSHPassphrase
		}
	}

	dbCfg := connConfigToDB(connCfg, m.forceReadOnly)
	resolved, err := resolveConnSecrets(dbCfg)
	if err != nil {
		m.connForm.SetError(err.Error())
		return nil
	}
	dbCfg = resolved

	m.connForm.SetTesting(true)
	driver := dbCfg.Driver
	return func() tea.Msg {
		conn, err := db.New(dbCfg)
		if err == nil {
			if cerr := conn.Connect(); cerr != nil {
				err = cerr
			}
			// The test connection is never kept; close it regardless of result.
			conn.Close()
		}
		return connTestResultMsg{driver: driver, err: err}
	}
}

// selectDatabase switches to the chosen database, reloads tables/schemas, and
// clears stale results. Called from the database picker.
func (m *Model) selectDatabase(name string) tea.Cmd {
	if m.connection == nil || name == "" {
		return nil
	}
	// UseDatabase may re-open the connection, orphaning an active transaction.
	m.rollbackTxn()
	m.saveSession() // capture the previous database's workspace first
	if err := m.connection.UseDatabase(name); err != nil {
		m.connError = err.Error()
		return nil
	}
	m.connError = ""
	m.dbPicker.Hide()

	// Reset workspace state for the new database.
	m.expanded = make(map[string][]db.Column)
	m.columnCache = make(map[string][]db.Column)
	m.results.Clear()
	m.results.ClearEditable()
	m.inspector.Hide()
	m.tables = nil
	m.lastQuery = ""
	m.clearQueryFailure()
	m.page = 0
	m.pageMsg = ""
	m.statsMsg = ""
	m.exportMsg = ""
	m.searchMsg = ""
	m.results.SetSearchMatcher(nil)
	m.queryStack = nil
	m.sidebarCursor = 0

	m.loadTables()
	m.restoreSession() // reopen tabs/editor buffers from the last visit
	cmd := m.editor.Focus()
	m.layoutWorkspace()
	m.applyFocus()
	return tea.Batch(cmd, m.prefetchSchemas(), m.fetchTableRowCounts())
}

// openDatabasePicker fetches available databases and shows the picker overlay.
func (m *Model) openDatabasePicker(mustChoose bool) tea.Cmd {
	if m.connection == nil {
		return nil
	}
	dbs, err := m.connection.DB().Databases()
	if err != nil {
		m.connError = err.Error()
		return nil
	}
	m.dbPicker.Show(dbs, mustChoose)
	return nil
}

func (m *Model) loadTables() {
	if m.connection == nil {
		return
	}
	tables, err := m.connection.DB().Tables()
	if err != nil {
		m.connError = err.Error()
		return
	}
	m.tables = tables
	// Cache the view-name set so the sidebar can badge views vs base tables.
	// A Views() failure (rare; driver/privilege issue) just leaves no badges.
	m.views = nil
	if views, err := m.connection.DB().Views(); err == nil {
		m.views = make(map[string]bool, len(views))
		for _, v := range views {
			m.views[v] = true
		}
	}
	m.refreshCompletionCandidates()
}

// prefetchSchemas asynchronously fetches column schemas for all tables,
// along with each table's primary keys and foreign keys. All three feed the
// AI schema context (so an AI turn builds its prompt from memory instead of
// re-running metadata queries) and the autocomplete candidate list.
func (m Model) prefetchSchemas() tea.Cmd {
	d := m.connection.DB()
	tables := m.tables
	return func() tea.Msg {
		schemas := make(map[string][]db.Column, len(tables))
		pks := make(map[string][]string, len(tables))
		fks := make(map[string][]db.ForeignKey, len(tables))
		for _, t := range tables {
			if cols, err := d.TableSchema(t); err == nil {
				schemas[t] = cols
			}
			if pk, err := d.PrimaryKeys(t); err == nil {
				pks[t] = pk
			}
			if fk, err := d.ForeignKeys(t); err == nil {
				fks[t] = fk
			}
		}
		return schemasLoadedMsg{schemas: schemas, pks: pks, fks: fks}
	}
}

// fetchTableRowCounts fetches approximate row counts for all tables
// asynchronously, so the sidebar can display them without blocking.
func (m Model) fetchTableRowCounts() tea.Cmd {
	if m.connection == nil || len(m.tables) == 0 {
		return nil
	}
	d := m.connection.DB()
	return func() tea.Msg {
		counts, err := d.TableRowCounts()
		if err != nil {
			return tableRowCountsMsg{counts: nil}
		}
		return tableRowCountsMsg{counts: counts}
	}
}

// openStructurePanel opens the Structure overlay for the selected table and
// begins loading its metadata asynchronously. Each catalog query is fetched
// independently so one failure (e.g. missing privilege) degrades gracefully
// instead of blanking the whole panel.
// loadStructureMetadata fetches the read-only structure metadata (PK, FKs,
// indexes, triggers, view definition) for a table asynchronously. It is issued
// by openSchemaPanel so the structure tabs populate without blocking the TUI;
// the result is routed to the schema editor by the structureLoadedMsg handler.
func (m *Model) loadStructureMetadata(table string) tea.Cmd {
	if table == "" || m.connection == nil {
		return nil
	}
	dbDriver := m.connection.DB()
	return func() tea.Msg {
		data := structureData{}

		if pk, err := dbDriver.PrimaryKeys(table); err == nil {
			data.pk = pk
		}
		if fks, err := dbDriver.ForeignKeys(table); err == nil {
			data.fks = fks
		}
		if idxs, err := dbDriver.Indexes(table); err == nil {
			data.indexes = idxs
		} else {
			data.indexErr = err.Error()
		}
		if triggers, err := dbDriver.Triggers(table); err == nil {
			data.triggers = triggers
		} else {
			data.triggerErr = err.Error()
		}
		if checks, err := dbDriver.CheckConstraints(table); err == nil {
			data.checks = checks
		} else {
			data.checkErr = err.Error()
		}
		if vd, err := dbDriver.ViewDefinition(table); err == nil {
			data.viewDef = vd
		} else {
			data.viewErr = err.Error()
		}

		return structureLoadedMsg{table: table, data: data}
	}
}

// resolveConnSecrets resolves any "secret://" references on the connection
// config into plaintext for the DB driver. Plaintext values pass through
// unchanged. The first reference that fails to resolve aborts with its error.
func resolveConnSecrets(cfg db.ConnectionConfig) (db.ConnectionConfig, error) {
	var err error
	if cfg.Password, err = secrets.Resolve(cfg.Password); err != nil {
		return cfg, err
	}
	if cfg.SSHPassword, err = secrets.Resolve(cfg.SSHPassword); err != nil {
		return cfg, err
	}
	if cfg.SSHPassphrase, err = secrets.Resolve(cfg.SSHPassphrase); err != nil {
		return cfg, err
	}
	return cfg, nil
}
