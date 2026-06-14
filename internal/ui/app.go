package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
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
	history      HistoryPanel
	sidebarCursor int
	expanded     map[string][]db.Column

	// Fuzzy table search
	sidebarFilter    string
	sidebarFiltering bool

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
		connList:     NewConnectionList(),
		history:      NewHistoryPanel(),
		historyStore: history.NewStore(historyDir()),
		expanded:     make(map[string][]db.Column),
		pageSize:     defaultPageSize,
	}
	m.loadConnections()
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
	m.focus = FocusEditor

	cmd := m.editor.Focus()
	m.loadTables()
	m.layoutWorkspace()

	return cmd
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
	for _, banned := range []string{" JOIN ", " WHERE ", " GROUP BY ", " HAVING ", " UNION ", "(", ","} {
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
	case "esc":
		if m.connList.list.FilterState() == list.Filtering {
			break
		}
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.connList, cmd = m.connList.Update(msg)
	return m, cmd
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
	case "ctrl+enter", "ctrl+j", "f5":
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
	case "tab":
		// Don't cycle focus while editing a cell.
		if m.results.IsEditing() {
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
		if m.results.IsEditing() {
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
		m.lastQuery = ""
		m.page = 0
		m.pageMsg = ""
		m.expanded = make(map[string][]db.Column)
		m.sidebarFiltering = false
		m.sidebarFilter = ""
		m.editor.CancelCompletion()
		m.loadConnections()
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
				m.results.CursorUp()
				return m, nil
			case "down", "j":
				m.results.CursorDown()
				return m, nil
			case "left", "h":
				m.results.CursorLeft()
				return m, nil
			case "right", "l":
				m.results.CursorRight()
				return m, nil
			case "enter", "e", "i":
				m.results.StartEdit()
				return m, nil
			case "ctrl+s":
				return m, m.saveEdits()
			}
			m.results, cmd = m.results.Update(msg)
			return m, cmd
		}

		// Non-editable results: scroll navigation.
		switch msg.String() {
		case "up", "k":
			m.results.ScrollUp()
			return m, nil
		case "down", "j":
			m.results.ScrollDown()
			return m, nil
		case "left", "h":
			m.results.ScrollLeft()
			return m, nil
		case "right", "l":
			m.results.ScrollRight()
			return m, nil
		}
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
			m = m.scrollSidebar(-1)
			return m, nil
		case "down", "j":
			m = m.scrollSidebar(1)
			return m, nil
		case "enter", " ":
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
				return m, m.editor.Focus()
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
	}
	return m, cmd
}

// refreshCompletionCandidates rebuilds the editor's candidate list from
// keywords, tables, and expanded columns.
func (m *Model) refreshCompletionCandidates() {
	var candidates []completionItem

	for _, kw := range sqlKeywords {
		candidates = append(candidates, completionItem{text: kw, kind: kindKeyword})
	}
	for _, t := range m.tables {
		candidates = append(candidates, completionItem{text: t, kind: kindTable})
	}

	seen := make(map[string]bool)
	for _, cols := range m.expanded {
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
	accent := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
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
	if m.focus > FocusResults {
		m.focus = FocusConnections
	}
	m.applyFocus()
	return m
}

func (m Model) cycleFocusBack() Model {
	m.focus--
	if m.focus < FocusConnections {
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

func (m Model) updateLayout() Model {
	if m.width == 0 || m.height == 0 {
		return m
	}

	if m.state == stateConnections {
		m.connList.SetSize(m.width, m.height)
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
	statusHeight := 1
	borderOverhead := 2
	editorHeight := 8

	resultsHeight := m.height - editorHeight - statusHeight - (borderOverhead * 2)
	if resultsHeight < 3 {
		resultsHeight = 3
	}

	// Sidebar spans the same height as editor + results combined.
	sidebarContentHeight := m.height - statusHeight - borderOverhead
	if sidebarContentHeight < 3 {
		sidebarContentHeight = 3
	}

	editorContentHeight := editorHeight - borderOverhead
	if editorContentHeight < 1 {
		editorContentHeight = 1
	}

	m.connList.SetSize(sidebarWidth-borderOverhead, sidebarContentHeight)
	m.editor.SetSize(m.width-sidebarWidth-borderOverhead, editorContentHeight)
	m.results.SetSize(m.width-sidebarWidth-borderOverhead, resultsHeight)
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
	if m.connError != "" {
		return errorStyle.Render(m.connError)
	}

	header := titleStyle.Render("gsql") + mutedStyle.Render("  — a fast SQL TUI")
	body := m.connList.View()
	footer := helpStyle.Render("enter: connect  n: new  e: edit  d: delete  esc: quit  /: filter")

	return lipgloss.JoinVertical(lipgloss.Left,
		appStyle.Render(header),
		appStyle.Render(body),
		footer,
	)
}

func (m Model) viewAddConnection() string {
	return appStyle.Render(
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2).
			Render(m.connForm.View()),
	)
}

func (m Model) viewWorkspace() string {
	sidebarWidth := 30
	statusHeight := 1
	borderOverhead := 2
	editorHeight := 8
	resultsHeight := m.height - editorHeight - statusHeight - (borderOverhead * 2)
	if resultsHeight < 3 {
		resultsHeight = 3
	}

	// Build right column first so we can measure its actual rendered height.
	editorTitle := titleStyle.Render("Query")
	modeIndicator := mutedStyle.Render(fmt.Sprintf("[%s]", m.editor.VimModeStr()))
	editorContent := lipgloss.JoinVertical(lipgloss.Left,
		editorTitle+"  "+modeIndicator,
		m.editor.View(),
	)
	editorPanel := lipgloss.NewStyle().
		Width(m.width - sidebarWidth - borderOverhead).
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
		Width(m.width - sidebarWidth - borderOverhead).
		Height(resultsHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.borderForFocus(FocusResults)).
		Render(resultsContent)

	rightPanel := lipgloss.JoinVertical(lipgloss.Left,
		editorPanel,
		resultsPanel,
	)

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
			prefix := "     "
			if isCursor {
				prefix = "→    "
			}
			colName := lipgloss.NewStyle().Foreground(colorFg).Render(item.text)
			colType := mutedStyle.Render(item.colType)
			line = prefix + colName + " " + colType
		} else {
			prefix := "  "
			style := normalStyle
			expandIcon := "▸"
			if _, ok := m.expanded[item.text]; ok {
				expandIcon = "▾"
			}
			if isCursor {
				prefix = "→ "
				style = selectedStyle
			}
			tableName := item.text
			if m.sidebarFiltering {
				tableName = highlightMatches(item.text, item.matchIdx)
			}
			line = prefix + expandIcon + " " + tableName
			line = style.Render(line)
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
				mutedStyle.Render("ctrl+t: switch  ctrl+y: history  ctrl+q: quit"),
			),
		)

	workspace := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel)

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

func (m Model) connectionInfo(name string) string {
	if m.connection == nil {
		return mutedStyle.Render("not connected")
	}
	return successStyle.Render("● " + name)
}

func (m Model) focusInfo() string {
	switch m.focus {
	case FocusConnections:
		return mutedStyle.Render("focus: tables")
	case FocusEditor:
		return mutedStyle.Render("focus: editor")
	case FocusResults:
		return mutedStyle.Render("focus: results")
	default:
		return ""
	}
}

func (m Model) contextHelp() string {
	switch m.focus {
	case FocusConnections:
		if m.sidebarFiltering {
			return mutedStyle.Render("type to filter  enter: select  esc: cancel")
		}
		return mutedStyle.Render("enter: expand  s: select  d: describe  /: find  j/k: scroll")
	case FocusResults:
		if m.results.IsEditing() {
			return mutedStyle.Render("enter: commit  esc: cancel")
		}
		if m.results.IsEditable() {
			pg := ""
			if m.pageMsg != "" {
				pg = "  " + m.pageMsg
			}
			return mutedStyle.Render("h/j/k/l: move  enter: edit  ctrl+s: save" + pg)
		}
		pg := ""
		if m.pageMsg != "" {
			pg = "  " + m.pageMsg
		}
		return mutedStyle.Render("j/k: rows  h/l: cols  ctrl+d/ctrl+u: page" + pg)
	default:
		if m.editor.CompletionVisible() {
			return mutedStyle.Render("tab/enter: accept  ctrl+p/n: select  esc: cancel")
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
