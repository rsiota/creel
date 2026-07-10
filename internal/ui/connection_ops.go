package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/db"
	"github.com/ruben/gsql/internal/secrets"
)

// connectToDB establishes a connection to the selected database.
func (m *Model) connectToDB() tea.Cmd {
	name := m.connList.SelectedName()
	driver := m.connList.SelectedDriver()
	connCfg := m.config.GetConnection(name)
	if connCfg == nil {
		m.connError = fmt.Sprintf("connection '%s' not found", name)
		return nil
	}

	dbCfg := db.ConnectionConfig{
		Name:     connCfg.Name,
		Driver:   db.Driver(driver),
		Database: connCfg.Database,
		Host:     connCfg.Host,
		Port:     connCfg.Port,
		Username: connCfg.Username,
		Password: connCfg.Password,

		SSHHost:       connCfg.SSHHost,
		SSHPort:       connCfg.SSHPort,
		SSHUser:       connCfg.SSHUser,
		SSHPassword:   connCfg.SSHPassword,
		SSHKeyPath:    connCfg.SSHKeyPath,
		SSHPassphrase: connCfg.SSHPassphrase,
	}

	// Resolve any keychain references ("secret://...") into plaintext for the
	// DB driver. A plaintext value passes through unchanged.
	resolved, err := resolveConnSecrets(dbCfg)
	if err != nil {
		m.connError = err.Error()
		return nil
	}
	dbCfg = resolved

	conn, err := db.New(dbCfg)
	if err != nil {
		m.connError = err.Error()
		return nil
	}

	if err := conn.Connect(); err != nil {
		m.connError = err.Error()
		return nil
	}

	m.connection = conn
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.columnCache = make(map[string][]db.Column)

	// MySQL/Postgres: always show the database picker (no history of last selection).
	if dbCfg.Driver == db.DriverMySQL || dbCfg.Driver == db.DriverPostgres {
		dbs, err := conn.DB().Databases()
		if err != nil {
			m.connError = err.Error()
			return nil
		}
		m.dbPicker.Show(dbs, true)
		m.layoutWorkspace()
		return nil
	}

	cmd := m.editor.Focus()
	m.loadTables()
	m.layoutWorkspace()
	m.applyFocus()

	return tea.Batch(cmd, m.prefetchSchemas(), m.fetchTableRowCounts())
}

// selectDatabase switches to the chosen database, reloads tables/schemas, and
// clears stale results. Called from the database picker.
func (m *Model) selectDatabase(name string) tea.Cmd {
	if m.connection == nil || name == "" {
		return nil
	}
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
	m.refreshCompletionCandidates()
}

// prefetchSchemas asynchronously fetches column schemas for all tables.
func (m Model) prefetchSchemas() tea.Cmd {
	d := m.connection.DB()
	tables := m.tables
	return func() tea.Msg {
		schemas := make(map[string][]db.Column)
		for _, t := range tables {
			cols, err := d.TableSchema(t)
			if err == nil {
				schemas[t] = cols
			}
		}
		return schemasLoadedMsg{schemas: schemas}
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
