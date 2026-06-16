package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
	"github.com/ruben/gsql/internal/history"
)

// Focus represents which panel currently has keyboard focus.
type Focus int

const (
	FocusConnections Focus = iota
	FocusEditor
	FocusResults
	FocusInspector
)

// state represents the current screen the app is showing.
type state int

const (
	stateConnections state = iota
	stateWorkspace
	stateAddConnection
)

// executeResultMsg carries the result of an async query execution.
type executeResultMsg struct {
	result db.Result
	err    error
}

// saveResultMsg carries the result of an async inline edit save.
type saveResultMsg struct {
	saved int
	err   error
}

// queryExecutedMsg is sent when a query finishes executing.
type queryExecutedMsg struct {
	query    string
	result   db.Result
	err      error
	page     int
	pageSize int
}

// schemasLoadedMsg carries prefetched table schemas for autocomplete.
type schemasLoadedMsg struct {
	schemas map[string][]db.Column
}

// Model is the top-level application model for the Bubble Tea architecture.
type Model struct {
	state    state
	focus    Focus
	width    int
	height   int
	quitting bool

	connList     ConnectionList
	connForm     ConnectionForm
	editor       QueryEditor
	results      ResultsTable
	inspector    Inspector
	history      HistoryPanel
	dbPicker     DatabasePicker
	sidebarCursor int
	expanded     map[string][]db.Column
	columnCache  map[string][]db.Column

	// Fuzzy table search
	sidebarFilter    string
	sidebarFiltering bool

	// Pending vim operator for sidebar (e.g. 'g' waiting for second 'g')
	sidebarPendingG bool
	resultsPendingG bool

	// Discard confirmation dialog
	discardConfirm bool

	config       *config.Config
	connection   *db.Connection
	historyStore *history.Store
	connError    string
	tables       []string

	// Pagination
	page     int
	pageSize int
	lastQuery string
	pageMsg  string
}

const defaultPageSize = 200

// NewModel creates a new top-level application model.
func NewModel(cfg *config.Config) Model {
	m := Model{
		state:        stateConnections,
		focus:        FocusConnections,
		config:       cfg,
		editor:       NewQueryEditor(),
		results:      NewResultsTable(),
		inspector:    NewInspector(),
		connList:     NewConnectionList(),
		history:      NewHistoryPanel(),
		dbPicker:     NewDatabasePicker(),
		historyStore: history.NewStore(historyDir()),
		expanded:     make(map[string][]db.Column),
		pageSize:     defaultPageSize,
	}
	m.loadConnections()
	if len(m.config.Connections) > 0 {
		m.connList.StartFilter()
	}
	return m
}

func (m *Model) loadConnections() {
	var entries []ConnectionEntry
	for _, conn := range m.config.Connections {
		detail := conn.Database
		if conn.Driver == "mysql" {
			detail = fmt.Sprintf("%s@%s:%d/%s", conn.Username, conn.Host, conn.Port, conn.Database)
		}
		if conn.SSHHost != "" {
			sshUser := conn.SSHUser
			if sshUser == "" {
				sshUser = "ssh"
			}
			detail = fmt.Sprintf("%s via %s@%s", detail, sshUser, conn.SSHHost)
		}
		entries = append(entries, ConnectionEntry{
			Name:   conn.Name,
			Driver: conn.Driver,
			Detail: detail,
		})
	}
	m.connList.SetItems(entries)
}

// Init initializes the application.
func (m Model) Init() tea.Cmd {
	return nil
}

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

	// MySQL: always show the database picker (no history of last selection).
	if dbCfg.Driver == db.DriverMySQL {
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

	return tea.Batch(cmd, m.prefetchSchemas())
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
	m.sidebarCursor = 0
	m.sidebarFiltering = false
	m.sidebarFilter = ""

	cmd := m.editor.Focus()
	m.loadTables()
	m.layoutWorkspace()
	m.applyFocus()
	return tea.Batch(cmd, m.prefetchSchemas())
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

// executeQuery runs the current query asynchronously with pagination.
func (m *Model) executeQuery() tea.Cmd {
	query := m.editor.FormatQuery()
	if query == "" {
		return nil
	}

	m.lastQuery = query
	m.page = 0
	return m.runPageQuery()
}

// nextPage advances to the next page of results.
func (m *Model) nextPage() tea.Cmd {
	if m.lastQuery == "" {
		return nil
	}
	m.page++
	return m.runPageQuery()
}

// prevPage goes back to the previous page of results.
func (m *Model) prevPage() tea.Cmd {
	if m.lastQuery == "" || m.page == 0 {
		return nil
	}
	m.page--
	return m.runPageQuery()
}

// runPageQuery wraps the original query with LIMIT/OFFSET and executes it.
// Non-SELECT statements (DESCRIBE, SHOW, EXPLAIN, etc.) are executed directly
// without pagination wrapping, since they can't be used as subqueries.
func (m *Model) runPageQuery() tea.Cmd {
	offset := m.page * m.pageSize
	query := strings.TrimRight(m.lastQuery, ";")

	conn := m.connection
	page := m.page
	pageSize := m.pageSize

	// Only wrap SELECT queries; everything else runs as-is.
	if isSelectQuery(query) {
		pagedQuery := fmt.Sprintf("SELECT * FROM (%s) AS _gsql_page LIMIT %d OFFSET %d",
			query, pageSize+1, offset)
		return func() tea.Msg {
			result, err := conn.DB().Execute(pagedQuery)
			return queryExecutedMsg{
				query:    m.lastQuery,
				result:   result,
				err:      err,
				page:     page,
				pageSize: pageSize,
			}
		}
	}

	return func() tea.Msg {
		result, err := conn.DB().Execute(query)
		return queryExecutedMsg{
			query:    m.lastQuery,
			result:   result,
			err:      err,
			page:     page,
			pageSize: pageSize,
		}
	}
}

// isSelectQuery returns true if the query is a SELECT (or WITH ... SELECT)
// statement that can safely be wrapped in a subquery for pagination.
func isSelectQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
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
	if err != nil || len(pkCols) == 0 {
		return
	}

	// Verify PK columns are present in the result set.
	colSet := make(map[string]bool)
	for _, c := range m.results.columns {
		colSet[strings.ToLower(c)] = true
	}
	for _, pk := range pkCols {
		if !colSet[strings.ToLower(pk)] {
			return
		}
	}

	m.results.SetEditable(table, pkCols)
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

// saveEdits writes all pending dirty cells to the database using parameterized
// UPDATE queries. Each dirty cell generates one UPDATE statement.
func (m *Model) saveEdits() tea.Cmd {
	if !m.results.HasDirtyCells() || m.connection == nil {
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
		edit     CellEdit
		pkVals   []string
		colName  string
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
		saved := 0
		for _, p := range pending {
			// Build: UPDATE <table> SET <col> = ? WHERE <pk1> = ? AND <pk2> = ?
			var b strings.Builder
			fmt.Fprintf(&b, "UPDATE %s SET %s = ?", table, p.colName)
			for i, pk := range pkCols {
				if i == 0 {
					b.WriteString(" WHERE ")
				} else {
					b.WriteString(" AND ")
				}
				fmt.Fprintf(&b, "%s = ?", pk)
			}

			args := []interface{}{p.edit.NewValue}
			for _, v := range p.pkVals {
				args = append(args, v)
			}

			_, err := conn.DB().Exec(b.String(), args...)
			if err != nil {
				return saveResultMsg{saved: saved, err: err}
			}
			saved++
		}
		return saveResultMsg{saved: saved}
	}
}

// Update handles all application messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.updateLayout()
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+q"))):
			m.quitting = true
			return m, tea.Quit
		}

		if m.state == stateConnections {
			return m.updateConnections(msg)
		}
		if m.state == stateAddConnection {
			return m.updateAddConnection(msg)
		}
		return m.updateWorkspace(msg)

	case queryExecutedMsg:
		m.layoutWorkspace()
		// Record to history
		if m.connection != nil && m.historyStore != nil {
			m.historyStore.Record(m.connection.Config().Name, msg.query, msg.err == nil)
		}
		if msg.err != nil {
			m.results.SetError(msg.err.Error())
		} else {
			cols := make([]string, len(msg.result.Columns))
			for i, c := range msg.result.Columns {
				cols[i] = c.Name
			}

			// Check for "has next page" — we fetched pageSize+1 rows
			rows := msg.result.Rows
			hasNext := false
			if len(rows) > msg.pageSize {
				hasNext = true
				rows = rows[:msg.pageSize]
			}

			m.results.SetResult(cols, rows, msg.result.Message)
			colTypes := make(map[string]string)
			for _, c := range msg.result.Columns {
				colTypes[c.Name] = c.Type
			}
			m.results.SetColumnTypes(colTypes)
			m.page = msg.page

			// Build pagination status message
			pgInfo := ""
			if msg.page > 0 || hasNext {
				offset := msg.page * msg.pageSize
				pgInfo = fmt.Sprintf("page %d (rows %d-%d)", msg.page+1, offset+1, offset+len(rows))
				if hasNext {
					pgInfo += " · more available"
				}
			}
			m.pageMsg = pgInfo

			// Enable inline editing if this is a simple SELECT from a single table.
			m.detectEditability(msg.query)
			m.inspector.Reset()
		}
		// Switch focus to results after a query completes.
		if m.focus != FocusInspector {
			m.focus = FocusResults
			m.applyFocus()
		}
		return m, nil

	case saveResultMsg:
		if msg.err != nil {
			m.results.SetSaveError(msg.err.Error())
		} else {
			// Apply dirty values to the underlying rows so the display stays consistent.
			for _, edit := range m.results.DirtyCells() {
				if edit.Row >= 0 && edit.Row < len(m.results.rows) &&
					edit.Col >= 0 && edit.Col < len(m.results.rows[edit.Row]) {
					m.results.rows[edit.Row][edit.Col] = edit.NewValue
				}
			}
			m.results.ConfirmSaved()
		}
		return m, nil

	case schemasLoadedMsg:
		m.columnCache = msg.schemas
		m.refreshCompletionCandidates()
		return m, nil
	}

	if m.state == stateWorkspace {
		var cmd tea.Cmd
		switch m.focus {
		case FocusEditor:
			m.editor, cmd = m.editor.Update(msg)
		case FocusResults:
			m.results, cmd = m.results.Update(msg)
		}
		return m, cmd
	}

	if m.state == stateAddConnection {
		var cmd tea.Cmd
		m.connForm, cmd = m.connForm.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updateConnections(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Filter mode intercepts all keys.
	if m.connList.IsFiltering() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.connList.CancelFilter()
			return m, nil
		case "enter":
			// Connect using the current filtered selection, then commit.
			cmd := m.connectToDB()
			m.connList.CommitFilter()
			return m, cmd
		case "backspace":
			m.connList.FilterBackspace()
			return m, nil
		case "up", "k":
			m.connList.MoveCursor(-1)
			return m, nil
		case "down", "j":
			m.connList.MoveCursor(1)
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.connList.FilterAddChar(msg.String())
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "enter":
		return m, m.connectToDB()
	case "n":
		m.state = stateAddConnection
		m.connForm = NewConnectionForm()
		cmd := m.connForm.Focus()
		return m, cmd
	case "e":
		return m.openEditForm()
	case "d":
		return m.deleteSelectedConnection()
	case "/":
		m.connList.StartFilter()
		return m, nil
	case "esc":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		m.connList.MoveCursor(-1)
		return m, nil
	case "down", "j":
		m.connList.MoveCursor(1)
		return m, nil
	case "G":
		items := m.connList.visibleItems()
		m.connList.SetCursor(len(items) - 1)
		return m, nil
	case "g":
		m.connList.SetCursor(0)
		return m, nil
	}

	return m, nil
}

// addDefaultSQLiteConnection creates a quick local SQLite connection for convenience.
func (m Model) addDefaultSQLiteConnection() (tea.Model, tea.Cmd) {
	// For now just demonstrate; full add-connection UI is a future slice.
	return m, nil
}

func (m Model) openEditForm() (tea.Model, tea.Cmd) {
	name := m.connList.SelectedName()
	if name == "" {
		return m, nil
	}
	existing := m.config.GetConnection(name)
	if existing == nil {
		return m, nil
	}
	m.state = stateAddConnection
	m.connForm = NewConnectionFormEdit(*existing)
	cmd := m.connForm.Focus()
	return m, cmd
}

func (m Model) deleteSelectedConnection() (tea.Model, tea.Cmd) {
	name := m.connList.SelectedName()
	if name == "" {
		return m, nil
	}
	m.config.RemoveConnection(name)
	if err := m.config.Save(); err != nil {
		m.connError = err.Error()
		return m, nil
	}
	m.connError = ""
	m.loadConnections()
	return m, nil
}

func (m Model) updateAddConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateConnections
		m.connError = ""
		return m, nil
	case "enter":
		connCfg, errMsg := m.connForm.EnterPressed()
		if errMsg != "" {
			m.connForm.SetError(errMsg)
			return m, nil
		}

		if m.connForm.mode == formModeEdit {
			m.config.RemoveConnection(m.connForm.editing)
		}

		m.config.AddConnection(connCfg)
		if err := m.config.Save(); err != nil {
			m.connForm.SetError(err.Error())
			return m, nil
		}

		m.state = stateConnections
		m.connError = ""
		m.loadConnections()
		return m, nil
	}

	var cmd tea.Cmd
	m.connForm, cmd = m.connForm.Update(msg)
	return m, cmd
}

func (m Model) updateWorkspace(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ensure panels are correctly sized (handles state transitions where
	// WindowSizeMsg hasn't re-fired).
	m.layoutWorkspace()

	var cmd tea.Cmd

	// Database picker is modal — intercept all keys when visible.
	if m.dbPicker.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.dbPicker.MustChoose() {
				// No database selected yet — return to connection list.
				m.connection.Close()
				m.connection = nil
				m.dbPicker.Hide()
				m.state = stateConnections
				m.focus = FocusConnections
				m.loadConnections()
			} else {
				m.dbPicker.Hide()
			}
			return m, nil
		case "enter":
			name := m.dbPicker.SelectedDatabase()
			m.dbPicker.Hide()
			return m, m.selectDatabase(name)
		case "up", "k":
			m.dbPicker.CursorUp()
			return m, nil
		case "down", "j":
			m.dbPicker.CursorDown()
			return m, nil
		case "backspace":
			m.dbPicker.FilterBackspace()
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.dbPicker.FilterAddChar(msg.String())
			return m, nil
		}
		return m, nil
	}

	// Discard confirmation dialog is modal — intercept all keys.
	if m.discardConfirm {
		switch msg.String() {
		case "y", "Y", "enter":
			m.results.DiscardEdits()
			m.discardConfirm = false
			return m, nil
		case "n", "N", "esc", "ctrl+c":
			m.discardConfirm = false
			return m, nil
		}
		return m, nil
	}

	// Global workspace keys
	switch msg.String() {
	case "ctrl+y":
		if m.connection != nil {
			if m.history.IsVisible() {
				m.history.Toggle()
			} else {
				entries, err := m.historyStore.Get(m.connection.Config().Name)
				if err == nil {
					m.history.SetEntries(entries)
				}
				m.history.Toggle()
			}
		}
		return m, nil
	case "ctrl+enter", "f5":
		return m, m.executeQuery()
	case "ctrl+d":
		// Vim-style page navigation — only when not in the editor
		// (Ctrl+D/U scroll within the editor in vim normal mode).
		// Also block when editing a cell or have unsaved edits.
		if m.focus != FocusEditor {
			if m.results.IsEditing() || m.results.HasDirtyCells() {
				return m, nil
			}
			return m, m.nextPage()
		}
	case "ctrl+u":
		if m.focus != FocusEditor {
			if m.results.IsEditing() || m.results.HasDirtyCells() {
				return m, nil
			}
			return m, m.prevPage()
		}
	case "ctrl+r":
		m.editor.Reset()
		return m, nil
	case "ctrl+b":
		// Browse databases (MySQL only).
		if m.connection != nil && m.connection.Config().Driver == db.DriverMySQL {
			return m, m.openDatabasePicker(false)
		}
		return m, nil
	case "ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l":
		// Directional panel navigation — not while editing or in insert mode.
		if m.results.IsEditing() || m.inspector.IsEditing() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.VimMode() == VimInsert {
			break // let it fall through to the editor
		}
		m = m.moveFocus(msg.String())
		return m, nil
	case "ctrl+o":
		m.inspector.Toggle()
		if m.inspector.IsVisible() {
			m.focus = FocusInspector
		} else if m.focus == FocusInspector {
			m.focus = FocusResults
		}
		m.layoutWorkspace()
		m.applyFocus()
		return m, nil
	case "tab":
		// Don't cycle focus while editing a cell or inspector field.
		if m.results.IsEditing() || m.inspector.IsEditing() {
			return m, nil
		}
		// Tab accepts completion when popup is visible.
		if m.focus == FocusEditor && m.editor.CompletionVisible() {
			m.editor.AcceptCompletion()
			return m, nil
		}
		m = m.cycleFocus()
		return m, nil
	case "shift+tab":
		if m.results.IsEditing() || m.inspector.IsEditing() {
			return m, nil
		}
		m = m.cycleFocusBack()
		return m, nil
	case "ctrl+t":
		// Return to connection screen
		if m.connection != nil {
			m.connection.Close()
			m.connection = nil
		}
		m.state = stateConnections
		m.focus = FocusConnections
		m.results.Clear()
		m.results.ClearEditable()
		m.inspector.Hide()
		m.dbPicker.Hide()
		m.discardConfirm = false
		m.lastQuery = ""
		m.page = 0
		m.pageMsg = ""
		m.expanded = make(map[string][]db.Column)
		m.columnCache = nil
		m.sidebarFiltering = false
		m.sidebarFilter = ""
		m.editor.CancelCompletion()
		m.loadConnections()
		if len(m.config.Connections) > 0 {
			m.connList.StartFilter()
		}
		return m, nil
	case "esc":
		// Exit sidebar fuzzy filter mode.
		if m.focus == FocusConnections && m.sidebarFiltering {
			m.sidebarFiltering = false
			m.sidebarFilter = ""
			m.sidebarCursor = 0
			return m, nil
		}
		// In insert mode, esc goes to the editor for vim mode switching.
		// In normal mode, esc is a no-op (or could blur the editor).
		if m.focus == FocusEditor && m.editor.VimMode() == VimInsert {
			m.editor, cmd = m.editor.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// History panel takes over navigation when visible
	if m.history.IsVisible() {
		switch msg.String() {
		case "j", "down":
			m.history.CursorDown()
			return m, nil
		case "k", "up":
			m.history.CursorUp()
			return m, nil
		case "enter":
			q := m.history.SelectedQuery()
			if q != "" {
				m.editor.SetValue(q)
				m.focus = FocusEditor
				m.applyFocus()
			}
			m.history.Toggle()
			return m, m.editor.Focus()
		case "esc":
			m.history.Toggle()
			return m, nil
		}
	}

	// Dispatch to focused panel
	switch m.focus {
	case FocusEditor:
		// Handle ctrl+arrow for result scrolling while in editor
		switch msg.String() {
		case "ctrl+up":
			m.results.ScrollUp()
			return m, nil
		case "ctrl+down":
			m.results.ScrollDown()
			return m, nil
		case "ctrl+left":
			m.results.ScrollLeft()
			return m, nil
		case "ctrl+right":
			m.results.ScrollRight()
			return m, nil
		case "ctrl+n":
			if m.editor.VimMode() == VimInsert && !m.editor.CompletionVisible() {
				m.editor.StartCompletion()
				return m, nil
			}
		}
		m.editor, cmd = m.editor.Update(msg)
	case FocusResults:
		// If currently editing a cell, intercept keys first.
		if m.results.IsEditing() {
			switch msg.String() {
			case "enter":
				m.results.CommitEdit()
				return m, nil
			case "esc":
				m.results.CancelEdit()
				return m, nil
			case "ctrl+c":
				m.results.CancelEdit()
				return m, nil
			}
			// All other keys go to the textinput.
			m.results, cmd = m.results.Update(msg)
			return m, cmd
		}

		// Editable results: cell cursor navigation.
		if m.results.IsEditable() {
			switch msg.String() {
			case "up", "k":
				m.resultsPendingG = false
				m.results.CursorUp()
				return m, nil
			case "down", "j":
				m.resultsPendingG = false
				m.results.CursorDown()
				return m, nil
			case "left", "h", "b":
				m.resultsPendingG = false
				m.results.CursorLeft()
				return m, nil
			case "right", "l", "w":
				m.resultsPendingG = false
				m.results.CursorRight()
				return m, nil
			case "G":
				m.resultsPendingG = false
				m.results.cursorRow = len(m.results.rows)
				m.results.clampCursor()
				m.results.ensureCursorVisible()
				return m, nil
			case "g":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.results.cursorRow = 0
					m.results.ensureCursorVisible()
					return m, nil
				}
				m.resultsPendingG = true
				return m, nil
			case "enter", "e", "i":
				m.resultsPendingG = false
				if m.inspector.IsVisible() {
					return m, nil
				}
				m.results.StartEdit()
				return m, nil
			case "ctrl+s":
				m.resultsPendingG = false
				return m, m.saveEdits()
			case "D":
				m.resultsPendingG = false
				if m.results.HasDirtyCells() {
					m.discardConfirm = true
				}
				return m, nil
			}
			m.resultsPendingG = false
			m.results, cmd = m.results.Update(msg)
			return m, cmd
		}

		// Non-editable results: scroll navigation.
		switch msg.String() {
		case "up", "k":
			m.resultsPendingG = false
			m.results.ScrollUp()
			return m, nil
		case "down", "j":
			m.resultsPendingG = false
			m.results.ScrollDown()
			return m, nil
		case "left", "h", "b":
			m.resultsPendingG = false
			m.results.ScrollLeft()
			return m, nil
		case "right", "l", "w":
			m.resultsPendingG = false
			m.results.ScrollRight()
			return m, nil
		case "G":
			m.resultsPendingG = false
			m.results.ScrollBottom()
			return m, nil
		case "g":
			if m.resultsPendingG {
				m.resultsPendingG = false
				m.results.ScrollTop()
				return m, nil
			}
			m.resultsPendingG = true
			return m, nil
		}
		m.resultsPendingG = false
		m.results, cmd = m.results.Update(msg)
	case FocusConnections:
		// Fuzzy filter mode intercepts all keys.
		if m.sidebarFiltering {
			switch msg.String() {
			case "esc":
				m.sidebarFiltering = false
				m.sidebarFilter = ""
				m.sidebarCursor = 0
				return m, nil
			case "enter":
				// Select the highlighted match: find it in the full list.
				if item := m.currentSidebarItem(); item != nil {
					for i, t := range m.tables {
						if t == item.text {
							m.sidebarCursor = i
							break
						}
					}
				}
				m.sidebarFiltering = false
				m.sidebarFilter = ""
				return m, nil
			case "backspace":
				if len(m.sidebarFilter) > 0 {
					m.sidebarFilter = m.sidebarFilter[:len(m.sidebarFilter)-1]
				}
				m.sidebarCursor = 0
				return m, nil
			case "up", "k":
				m = m.scrollSidebar(-1)
				return m, nil
			case "down", "j":
				m = m.scrollSidebar(1)
				return m, nil
			case "ctrl+c":
				m.sidebarFiltering = false
				m.sidebarFilter = ""
				m.sidebarCursor = 0
				return m, nil
			}
			// Printable characters extend the filter.
			if msg.Type == tea.KeyRunes {
				m.sidebarFilter += msg.String()
				m.sidebarCursor = 0
				return m, nil
			}
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			m.sidebarPendingG = false
			m = m.scrollSidebar(-1)
			return m, nil
		case "down", "j":
			m.sidebarPendingG = false
			m = m.scrollSidebar(1)
			return m, nil
		case "G":
			m.sidebarPendingG = false
			items := m.sidebarItems()
			if len(items) > 0 {
				m.sidebarCursor = len(items) - 1
			}
			return m, nil
		case "g":
			if m.sidebarPendingG {
				m.sidebarPendingG = false
				m.sidebarCursor = 0
				return m, nil
			}
			m.sidebarPendingG = true
			return m, nil
		case "enter", " ":
			m.sidebarPendingG = false
			m.toggleExpand()
			return m, nil
		case "/":
			m.sidebarFiltering = true
			m.sidebarFilter = ""
			m.sidebarCursor = 0
			return m, nil
		case "s":
			item := m.currentSidebarItem()
			if item != nil && !item.isColumn {
				m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s LIMIT 100;", item.text))
				m.focus = FocusEditor
				m.applyFocus()
				return m, tea.Batch(m.editor.Focus(), m.executeQuery())
			}
		case "d":
			item := m.currentSidebarItem()
			if item != nil && !item.isColumn {
				tableName := item.text
				if m.connection != nil && m.connection.Config().Driver == db.DriverSQLite {
					m.editor.SetValue(fmt.Sprintf("SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('%s');", tableName))
				} else {
					m.editor.SetValue(fmt.Sprintf("DESCRIBE %s;", tableName))
				}
				m.focus = FocusEditor
				m.applyFocus()
				return m, m.executeQuery()
			}
		}
		m.connList, cmd = m.connList.Update(msg)
	case FocusInspector:
		if m.inspector.IsEditing() {
			switch msg.String() {
			case "enter":
				col, val, ok := m.inspector.CommitFieldEdit()
				if ok {
					m.results.SetDirtyCell(m.results.CursorRow(), col, val)
				}
				return m, nil
			case "esc", "ctrl+c":
				m.inspector.CancelEdit()
				return m, nil
			}
			m.inspector, cmd = m.inspector.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "up", "k":
			m.inspector.pendingG = false
			m.inspector.CursorUp()
			return m, nil
		case "down", "j":
			m.inspector.pendingG = false
			m.inspector.CursorDown(m.results.NumCols())
			return m, nil
		case "G":
			m.inspector.pendingG = false
			m.inspector.CursorBottom(m.results.NumCols())
			return m, nil
		case "g":
			if m.inspector.pendingG {
				m.inspector.pendingG = false
				m.inspector.CursorTop()
				return m, nil
			}
			m.inspector.pendingG = true
			return m, nil
		case "enter", "e", "i":
			m.inspector.pendingG = false
			m.inspector.StartFieldEdit(m.results)
			return m, nil
		case "ctrl+s":
			m.inspector.pendingG = false
			return m, m.saveEdits()
		case "D":
			m.inspector.pendingG = false
			if m.results.HasDirtyCells() {
				m.discardConfirm = true
			}
			return m, nil
		}
		return m, nil
	}
	return m, cmd
}

// refreshCompletionCandidates rebuilds the editor's candidate list from
// keywords, tables, and cached column schemas.
func (m *Model) refreshCompletionCandidates() {
	var candidates []completionItem

	for _, kw := range sqlKeywords {
		candidates = append(candidates, completionItem{text: kw, kind: kindKeyword})
	}
	for _, t := range m.tables {
		candidates = append(candidates, completionItem{text: t, kind: kindTable})
	}

	seen := make(map[string]bool)
	for _, cols := range m.columnCache {
		for _, c := range cols {
			if !seen[c.Name] {
				candidates = append(candidates, completionItem{text: c.Name, kind: kindColumn})
				seen[c.Name] = true
			}
		}
	}

	m.editor.SetCandidates(candidates)
}

// sidebarItem is a flat entry in the sidebar (table or column).
type sidebarItem struct {
	text     string
	isColumn bool
	colType  string
	matchIdx []int // rune indices of fuzzy-matched chars (for highlighting)
}

// sidebarItems builds the flat list of tables + expanded columns.
func (m Model) sidebarItems() []sidebarItem {
	if m.sidebarFiltering {
		return m.filteredTables()
	}
	var items []sidebarItem
	for _, t := range m.tables {
		items = append(items, sidebarItem{text: t})
		if cols, ok := m.expanded[t]; ok {
			for _, c := range cols {
				items = append(items, sidebarItem{text: c.Name, isColumn: true, colType: c.Type})
			}
		}
	}
	return items
}

// filteredTables returns tables matching the fuzzy filter, best match first.
func (m Model) filteredTables() []sidebarItem {
	type scored struct {
		item  sidebarItem
		score int
	}
	var results []scored
	for _, t := range m.tables {
		idx, score := fuzzyMatch(m.sidebarFilter, t)
		if idx != nil || m.sidebarFilter == "" {
			results = append(results, scored{sidebarItem{text: t, matchIdx: idx}, score})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score < results[j].score
		}
		return results[i].item.text < results[j].item.text
	})
	items := make([]sidebarItem, len(results))
	for i, r := range results {
		items[i] = r.item
	}
	return items
}

// fuzzyMatch performs case-insensitive subsequence matching.
// Returns matched rune indices (nil if no match) and a score (lower = better).
func fuzzyMatch(query, s string) ([]int, int) {
	if query == "" {
		return nil, 0
	}
	q := []rune(strings.ToLower(query))
	target := []rune(strings.ToLower(s))

	var indices []int
	qi := 0
	for si := 0; si < len(target) && qi < len(q); si++ {
		if target[si] == q[qi] {
			indices = append(indices, si)
			qi++
		}
	}
	if qi < len(q) {
		return nil, 0
	}

	// Score: lower = better. Penalize gaps, reward consecutive/boundary matches.
	score := len(target) // mild preference for shorter names
	for i := 1; i < len(indices); i++ {
		gap := indices[i] - indices[i-1] - 1
		score += gap * 3
		if indices[i] == indices[i-1]+1 {
			score -= 5
		}
	}
	for _, idx := range indices {
		if idx == 0 || target[idx-1] == '_' || target[idx-1] == '.' {
			score -= 2
		}
	}
	return indices, score
}

// highlightMatches renders text with matched characters in the accent color.
func highlightMatches(text string, matchIdx []int) string {
	if len(matchIdx) == 0 {
		return text
	}
	matchSet := make(map[int]bool, len(matchIdx))
	for _, i := range matchIdx {
		matchSet[i] = true
	}
	accent := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	var b strings.Builder
	for i, r := range []rune(text) {
		if matchSet[i] {
			b.WriteString(accent.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// scrollSidebar moves the cursor through the flat sidebar item list.
func (m Model) scrollSidebar(delta int) Model {
	items := m.sidebarItems()
	if len(items) == 0 {
		m.sidebarCursor = 0
		return m
	}
	m.sidebarCursor += delta
	if m.sidebarCursor < 0 {
		m.sidebarCursor = 0
	}
	if m.sidebarCursor > len(items)-1 {
		m.sidebarCursor = len(items) - 1
	}
	return m
}

// currentSidebarItem returns the item under the cursor.
func (m Model) currentSidebarItem() *sidebarItem {
	items := m.sidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return nil
	}
	return &items[m.sidebarCursor]
}

// toggleExpand loads or clears the schema for the selected table.
func (m *Model) toggleExpand() {
	item := m.currentSidebarItem()
	if item == nil || item.isColumn {
		return
	}
	table := item.text
	if _, ok := m.expanded[table]; ok {
		delete(m.expanded, table)
		m.refreshCompletionCandidates()
		return
	}
	cols, err := m.connection.DB().TableSchema(table)
	if err != nil {
		m.connError = err.Error()
		return
	}
	m.expanded[table] = cols
	m.refreshCompletionCandidates()
}

func (m Model) cycleFocus() Model {
	m.focus++
	if m.focus > FocusInspector {
		m.focus = FocusConnections
	}
	if m.focus == FocusInspector && !m.inspector.IsVisible() {
		m.focus = FocusConnections
	}
	m.applyFocus()
	return m
}

func (m Model) cycleFocusBack() Model {
	m.focus--
	if m.focus < FocusConnections {
		m.focus = FocusInspector
	}
	if m.focus == FocusInspector && !m.inspector.IsVisible() {
		m.focus = FocusResults
	}
	m.applyFocus()
	return m
}

func (m Model) applyFocus() Model {
	m.editor.Blur()
	switch m.focus {
	case FocusEditor:
		m.editor.Focus()
	}
	return m
}

// moveFocus navigates between panels using vim-style directions.
func (m Model) moveFocus(direction string) Model {
	switch m.focus {
	case FocusConnections:
		if direction == "ctrl+l" {
			m.focus = FocusEditor
		}
	case FocusEditor:
		switch direction {
		case "ctrl+h":
			m.focus = FocusConnections
		case "ctrl+j":
			m.focus = FocusResults
		case "ctrl+l":
			if m.inspector.IsVisible() {
				m.focus = FocusInspector
			}
		}
	case FocusResults:
		switch direction {
		case "ctrl+h":
			m.focus = FocusConnections
		case "ctrl+k":
			m.focus = FocusEditor
		case "ctrl+l":
			if m.inspector.IsVisible() {
				m.focus = FocusInspector
			}
		}
	case FocusInspector:
		if direction == "ctrl+h" {
			m.focus = FocusResults
		}
	}
	m.applyFocus()
	return m
}

func (m Model) updateLayout() Model {
	if m.width == 0 || m.height == 0 {
		return m
	}

	if m.state == stateConnections {
		return m
	}

	if m.state == stateAddConnection {
		m.connForm.SetSize(m.width, m.height)
		return m
	}

	m.layoutWorkspace()
	return m
}

// layoutWorkspace sizes the workspace panels. Uses pointer receiver so it
// works correctly when called from both value and pointer receiver methods.
func (m *Model) layoutWorkspace() {
	sidebarWidth := 30
	inspectorWidth := InspectorWidth
	statusHeight := 1
	borderOverhead := 2
	editorHeight := 8

	inspectorVisible := m.inspector.IsVisible()

	rightWidth := m.width - sidebarWidth - borderOverhead
	if inspectorVisible {
		rightWidth -= inspectorWidth
	}

	resultsHeight := m.height - editorHeight - statusHeight - (borderOverhead * 2)
	if resultsHeight < 3 {
		resultsHeight = 3
	}

	// Sidebar and inspector span the same height as editor + results combined.
	sideContentHeight := m.height - statusHeight - borderOverhead
	if sideContentHeight < 3 {
		sideContentHeight = 3
	}

	editorContentHeight := editorHeight - borderOverhead
	if editorContentHeight < 1 {
		editorContentHeight = 1
	}

	m.connList.SetSize(sidebarWidth-borderOverhead, sideContentHeight)
	m.editor.SetSize(rightWidth, editorContentHeight)
	m.results.SetSize(rightWidth, resultsHeight)

	if inspectorVisible {
		viewHeight := editorHeight + resultsHeight
		m.inspector.SetSize(inspectorWidth-borderOverhead, viewHeight)
	}
}

// View renders the entire application.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return "Loading..."
	}

	if m.state == stateAddConnection {
		return m.viewAddConnection()
	}

	if m.state == stateConnections {
		return m.viewConnections()
	}

	return m.viewWorkspace()
}

func (m Model) viewConnections() string {
	footer := keybinds("enter", "connect", "n", "new", "e", "edit", "d", "delete", "/", "filter", "esc", "quit")

	popupW, popupH := popupDim()
	borderOverhead := 2
	padH, padW := 2, 4 // Padding(1, 2) → 2 rows, 4 cols

	panelW := popupW - borderOverhead
	panelH := popupH - borderOverhead

	connTitle := titleStyle.Render("Connections")
	listH := panelH - 3 - padH // title + scroll info + footer + padding
	m.connList.SetSize(panelW-padW, listH)

	// Pin the list to a fixed height so ScrollInfo sits at the bottom.
	listStyled := lipgloss.NewStyle().
		Height(listH).
		Render(m.connList.View())

	connPanel := lipgloss.NewStyle().
		Width(panelW).
		Height(panelH).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		Render(
			lipgloss.JoinVertical(lipgloss.Left,
				connTitle,
				listStyled,
				m.connList.ScrollInfo(),
				footer,
			),
		)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		connPanel,
		lipgloss.WithWhitespaceChars(" "),
	)
}

func (m Model) viewAddConnection() string {
	popupW, _ := popupDim()
	borderOverhead := 2
	padding := 4 // padding(1,2) = 2 left + 2 right
	innerW := popupW - borderOverhead - padding
	m.connForm.SetMaxWidth(innerW)

	formPanel := lipgloss.NewStyle().
		Width(popupW - borderOverhead).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		Render(m.connForm.View())

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		formPanel,
		lipgloss.WithWhitespaceChars(" "),
	)
}

func (m Model) viewWorkspace() string {
	sidebarWidth := 30
	inspectorWidth := InspectorWidth
	statusHeight := 1
	borderOverhead := 2
	editorHeight := 8
	resultsHeight := m.height - editorHeight - statusHeight - (borderOverhead * 2)
	if resultsHeight < 3 {
		resultsHeight = 3
	}

	rightWidth := m.width - sidebarWidth - borderOverhead
	if m.inspector.IsVisible() {
		rightWidth -= inspectorWidth
	}

	// Build right column first so we can measure its actual rendered height.
	editorTitle := titleStyle.Render("Query")
	modeIndicator := mutedStyle.Render(fmt.Sprintf("[%s]", m.editor.VimModeStr()))
	editorContent := lipgloss.JoinVertical(lipgloss.Left,
		editorTitle+"  "+modeIndicator,
		m.editor.View(),
	)
	editorPanel := lipgloss.NewStyle().
		Width(rightWidth).
		Height(editorHeight - borderOverhead).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.borderForFocus(FocusEditor)).
		Render(editorContent)

	resultsTitle := titleStyle.Render("Results")
	resultsContent := lipgloss.JoinVertical(lipgloss.Left,
		resultsTitle,
		m.results.View(),
	)
	resultsPanel := lipgloss.NewStyle().
		Width(rightWidth).
		Height(resultsHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.borderForFocus(FocusResults)).
		Render(resultsContent)

	rightPanel := lipgloss.JoinVertical(lipgloss.Left,
		editorPanel,
		resultsPanel,
	)

	// Build inspector panel if visible.
	var inspectorPanel string
	if m.inspector.IsVisible() {
		inspectorContentHeight := lipgloss.Height(rightPanel) - borderOverhead
		if inspectorContentHeight < 3 {
			inspectorContentHeight = 3
		}
		m.inspector.SetSize(inspectorWidth-borderOverhead, inspectorContentHeight)
		inspectorContent := lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Inspector"),
			m.inspector.View(m.results),
		)
		inspectorPanel = lipgloss.NewStyle().
			Width(inspectorWidth - borderOverhead).
			Height(inspectorContentHeight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.borderForFocus(FocusInspector)).
			Render(inspectorContent)
	}

	// Sidebar content height = right panel height minus sidebar's own borders.
	sidebarContentHeight := lipgloss.Height(rightPanel) - borderOverhead
	if sidebarContentHeight < 3 {
		sidebarContentHeight = 3
	}

	sidebarTitle := titleStyle.Render("Tables")

	// Reserve 2 lines: title at top, bottom bar (search/scroll info) at bottom.
	tableAreaHeight := sidebarContentHeight - 2
	if tableAreaHeight < 1 {
		tableAreaHeight = 1
	}
	maxVisible := tableAreaHeight

	items := m.sidebarItems()

	// Scroll window centered on cursor.
	half := maxVisible / 2
	start := m.sidebarCursor - half
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(items) {
		end = len(items)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	sidebarContentWidth := sidebarWidth - borderOverhead

	tableList := strings.Builder{}
	for i := start; i < end; i++ {
		item := items[i]
		isCursor := m.focus == FocusConnections && i == m.sidebarCursor

		var line string
		if item.isColumn {
			indent := "   "
			colStyle := lipgloss.NewStyle().Foreground(colorLabel)
			if isCursor {
				colStyle = lipgloss.NewStyle().Foreground(colorBg).Background(colorPrimary).Bold(true)
			}
			colName := colStyle.Render(item.text)
			colType := mutedStyle.Render(item.colType)
			line = indent + colName + " " + colType
		} else {
			style := normalStyle
			expandIcon := "▸"
			if _, ok := m.expanded[item.text]; ok {
				expandIcon = "▾"
			}
			if isCursor && !m.sidebarFiltering {
				style = selectedStyle
			}
			tableName := item.text
			if m.sidebarFiltering {
				tableName = highlightMatches(item.text, item.matchIdx)
			}
			line = expandIcon + " " + tableName
			if isCursor && m.sidebarFiltering {
				line = lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(line)
			} else {
				line = style.Render(line)
			}
		}

		// Truncate to sidebar width — strip ANSI codes for measurement,
		// then truncate the rendered string.
		line = truncateSidebarLine(line, sidebarContentWidth)
		tableList.WriteString(line)
		tableList.WriteString("\n")
	}
	if len(items) == 0 {
		if m.sidebarFiltering {
			tableList.WriteString(mutedStyle.Render("  (no matches)"))
		} else {
			tableList.WriteString(mutedStyle.Render("  (no tables)"))
		}
	}

	scrollInfo := ""
	if m.sidebarFiltering {
		prompt := lipgloss.NewStyle().Foreground(colorPrimary).Render("/"+m.sidebarFilter) +
			lipgloss.NewStyle().Foreground(colorAccent).Render("▏")
		scrollInfo = prompt
	} else if len(items) > maxVisible {
		scrollInfo = mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", start+1, end, len(items)))
	}

	tableListStyled := lipgloss.NewStyle().
		Height(tableAreaHeight).
		Render(strings.TrimRight(tableList.String(), "\n"))

	sidebar := lipgloss.NewStyle().
		Width(sidebarWidth - borderOverhead).
		Height(sidebarContentHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.borderForFocus(FocusConnections)).
		Render(
			lipgloss.JoinVertical(lipgloss.Left, sidebarTitle, tableListStyled, scrollInfo),
		)

	connName := ""
	if m.connection != nil {
		connName = m.connection.Config().Name
	}

	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(statusHeight).
		Foreground(colorMuted).
		Render(
			fmt.Sprintf(" %s  │  %s  │  %s  │  %s",
				m.connectionInfo(connName),
				m.focusInfo(),
				m.contextHelp(),
				keybinds("ctrl+t", "switch", "ctrl+b", "database", "ctrl+y", "history", "ctrl+o", "inspector", "ctrl+hjkl", "focus", "ctrl+q", "quit"),
			),
		)

	var workspace string
	if m.inspector.IsVisible() {
		workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel, inspectorPanel)
	} else {
		workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, workspace, statusBar)

	// Overlay history panel if visible
	if m.history.IsVisible() {
		m.history.SetSize(m.width/2, m.height-4)
		histPanel := m.history.View()
		view = lipgloss.Place(m.width, m.height-1,
			lipgloss.Center, lipgloss.Center,
			histPanel,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	// Overlay database picker if visible
	if m.dbPicker.IsVisible() {
		pw, ph := popupDim()
		m.dbPicker.SetSize(pw, ph)
		pickerPanel := m.dbPicker.View()
		view = lipgloss.Place(m.width, m.height-1,
			lipgloss.Center, lipgloss.Center,
			pickerPanel,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	// Overlay discard confirmation dialog if visible
	if m.discardConfirm {
		dialog := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 3).
			Width(46).
			Align(lipgloss.Center).
			Render(
				lipgloss.JoinVertical(lipgloss.Center,
					lipgloss.NewStyle().Foreground(colorPrimary).Render("Discard all unsaved changes?"),
					"",
					lipgloss.NewStyle().Foreground(colorLabel).Render("y") + mutedStyle.Render(" confirm    ") +
						lipgloss.NewStyle().Foreground(colorLabel).Render("n") + mutedStyle.Render(" cancel"),
				),
			)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay completion popup if visible
	if m.editor.CompletionVisible() {
		cursorLine, cursorCol := m.editor.CursorScreenPos()
		popup := m.editor.CompletionView()
		popupX := sidebarWidth + 2 + cursorCol
		popupY := 2 + cursorLine + 1
		view = placeOverlay(view, popup, popupX, popupY)
	}

	return view
}

// popupDim returns the fixed popup dimensions matching the connection form.
func popupDim() (w, h int) {
	return 71, 19
}

// keybind renders a single keybinding: the key in colorLabel, the description
// in colorMuted, separated by a space (no colon).
func keybind(key, desc string) string {
	return lipgloss.NewStyle().Foreground(colorLabel).Render(key) + " " +
		mutedStyle.Render(desc)
}

// keybinds renders multiple keybinding pairs joined by double spaces.
// Arguments are alternating key/description strings.
func keybinds(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, keybind(pairs[i], pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

func (m Model) connectionInfo(name string) string {
	if m.connection == nil {
		return mutedStyle.Render("not connected")
	}
	s := successStyle.Render("● " + name)
	if m.connection.Config().Driver == db.DriverMySQL && m.connection.Config().Database != "" {
		s += lipgloss.NewStyle().Foreground(colorLabel).Render("  ⟁ " + m.connection.Config().Database)
	}
	return s
}

func (m Model) focusInfo() string {
	switch m.focus {
	case FocusConnections:
		return keybind("focus", "tables")
	case FocusEditor:
		return keybind("focus", "editor")
	case FocusResults:
		return keybind("focus", "results")
	case FocusInspector:
		return keybind("focus", "inspector")
	default:
		return ""
	}
}

func (m Model) contextHelp() string {
	if m.dbPicker.IsVisible() {
		return keybinds("type", "to filter", "j/k", "navigate", "enter", "select", "esc", "cancel")
	}
	switch m.focus {
	case FocusConnections:
		if m.sidebarFiltering {
			return keybinds("type", "to filter", "enter", "select", "esc", "cancel")
		}
		return keybinds("enter", "expand", "s", "select", "d", "describe", "/", "find", "j/k", "scroll")
	case FocusResults:
		if m.results.IsEditing() {
			return keybinds("enter", "commit", "esc", "cancel")
		}
		inspHint := ""
		if m.inspector.IsVisible() {
			inspHint = "  " + keybind("ctrl+o", "close inspector")
		}
		if m.results.IsEditable() {
			pg := ""
			if m.pageMsg != "" {
				pg = "  " + m.pageMsg
			}
			discardHint := ""
			if m.results.HasDirtyCells() {
				discardHint = "  " + keybind("D", "discard")
			}
			return keybinds("h/j/k/l", "move", "enter", "edit", "ctrl+s", "save") + discardHint + pg + inspHint
		}
		pg := ""
		if m.pageMsg != "" {
			pg = "  " + m.pageMsg
		}
		return keybinds("j/k", "rows", "h/l", "cols", "ctrl+d/ctrl+u", "page") + pg + inspHint
	case FocusInspector:
		if m.inspector.IsEditing() {
			return keybinds("enter", "commit", "esc", "cancel")
		}
		if m.results.IsEditable() {
			discardHint := ""
			if m.results.HasDirtyCells() {
				discardHint = "  " + keybind("D", "discard")
			}
			return keybinds("j/k", "fields", "enter", "edit", "ctrl+s", "save", "ctrl+o", "close") + discardHint
		}
		return keybinds("j/k", "fields", "ctrl+o", "close")
	default:
		if m.editor.CompletionVisible() {
			return keybinds("tab/enter", "accept", "ctrl+p/n", "select", "esc", "cancel")
		}
		return m.editor.HelpText()
	}
}

func (m Model) borderForFocus(f Focus) lipgloss.Color {
	if m.focus == f {
		return colorPrimary
	}
	return colorBorder
}

// truncateSidebarLine truncates a rendered (possibly ANSI-styled) string
// to fit within maxVisible visible characters, appending "…" if truncated.
func truncateSidebarLine(line string, maxVisible int) string {
	// Measure visible width by counting non-ANSI characters.
	visible := lipgloss.Width(line)
	if visible <= maxVisible {
		return line
	}

	// lipgloss doesn't have a truncate-with-ansi helper we can use directly,
	// so use its MaxWidth style which handles ANSI-aware truncation.
	return lipgloss.NewStyle().MaxWidth(maxVisible).Render(line)
}

// Run starts the application.
func Run(cfg *config.Config) {
	m := NewModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}

// historyDir returns the directory for storing query history files.
func historyDir() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "gsql")
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "gsql")
}
