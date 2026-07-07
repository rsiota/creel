package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/db"
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
		m.sidebarFiltering = true
		m.sidebarFilter = ""
		return nil
	}

	cmd := m.editor.Focus()
	m.loadTables()
	m.layoutWorkspace()
	m.sidebarFiltering = true
	m.sidebarFilter = ""

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
	m.sidebarFiltering = true
	m.sidebarFilter = ""

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
