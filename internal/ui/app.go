package ui

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/atotto/clipboard"
	"github.com/ruben/gsql/internal/bookmarks"
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

// insertResultMsg carries the result of an async insert save.
type insertResultMsg struct {
	err error
}

// truncateResultMsg carries the result of an async table truncate.
type truncateResultMsg struct {
	table string
	err   error
}

// deleteRowsResultMsg carries the result of an async row deletion.
type deleteRowsResultMsg struct {
	table string
	count int
	err   error
}

// schemaResultMsg carries the result of an async schema change (DDL).
type schemaResultMsg struct {
	table    string
	newTable string
	action   db.SchemaAction
	err      error
}

// dropDBResultMsg carries the result of a DROP DATABASE operation.
type dropDBResultMsg struct {
	database string
	err      error
}

// createDBResultMsg carries the result of a CREATE DATABASE operation.
type createDBResultMsg struct {
	database string
	err      error
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

// copyFlashTickMsg advances the cell flash animation after a clipboard copy.
type copyFlashTickMsg struct{}

// copyCopiedClearMsg clears the clipboard confirmation status message.
type copyCopiedClearMsg struct{}

// filterValuesMsg carries the distinct values fetched for the filter picker.
type filterValuesMsg struct {
	column string
	values []string
}

// statsMsg carries column statistics fetched from the database.
type statsMsg struct {
	column string
	stats  string
}

// countMsg carries the total row count for the current table.
type countMsg struct {
	total int
	err   error
}

// exportDoneMsg carries the result of an async CSV export.
type exportDoneMsg struct {
	path  string
	count int
	err   error
}

// exportDumpMsg carries the result of an async SQL dump export.
type exportDumpMsg struct {
	path   string
	tables int
	err    error
}

// exportProgressMsg carries incremental progress during a table-by-table dump.
// The open file handle flows through the message stream so the model stays free
// of file-state. Each message represents one table written; the Update handler
// chains the next command until the final table, then writes the footer.
type exportProgressMsg struct {
	file   *os.File
	bw     *bufio.Writer
	path   string
	index  int // zero-based index of the table just written
	total  int
	tables []string
	name   string // name of the table just written
	err    error
}

// importProgressMsg carries a live progress update during an SQL import.
type importProgressMsg struct {
	filename string
	read     int64
	total    int64
}

// importDoneMsg carries the result of a completed SQL import.
type importDoneMsg struct {
	result   db.ImportResult
	filename string
	err      error
}

// flashTickMsg is emitted by a timer to auto-expire transient status-bar
// messages. It carries the generation counter from when it was armed; the
// handler only clears the flash if no newer message has arrived (i.e. the
// generation still matches).
type flashTickMsg struct{ gen uint64 }

// flashExpiry is how long a transient status-bar message stays visible before
// auto-clearing.
const flashExpiry = 5 * time.Second

// hintFlashDuration is how long a hint group stays highlighted white.
const hintFlashDuration = 300 * time.Millisecond

// queryStackEntry stores navigation state for returning after following a FK.
type queryStackEntry struct {
	query     string
	page      int
	cursorRow int
	cursorCol int
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
	bookmarks    BookmarkPanel
	dbPicker     DatabasePicker
	help         HelpPanel
	filterPicker FilterPicker
	columnPicker ColumnPicker
	exportPicker ExportPicker
	importPrompt ImportPrompt
	addColumnForm   AddColumnForm
	tableRenameForm TableRenameForm
	tableDesigner   TableDesigner
	schemaEditor    SchemaEditor
	cellEdit        CellEditPopup
	palette         palette
	sidebarCursor int
	expanded     map[string][]db.Column
	columnCache  map[string][]db.Column

	// Fuzzy table search
	sidebarFilter    string
	sidebarFiltering bool

	// Pending vim operator for sidebar (e.g. 'g' waiting for second 'g')
	sidebarPendingG bool
	resultsPendingG bool
	resultsPendingY bool
	resultsPendingD bool // dd double-tap state for row deletion

	// Discard confirmation dialog
	discardConfirm bool

	// Truncate confirmation dialog (non-empty table name while pending).
	truncateConfirm string

	// Drop-table confirmation dialog (non-empty table name while pending).
	// Requires the user to type the table name exactly to proceed.
	dropTableConfirm string
	dropTableInput   string

	// Drop-database typed confirmation (triggered from the database picker).
	dropDBConfirm string
	dropDBInput   string

	// Create-database name input (triggered from the database picker).
	createDBActive bool
	createDBInput   string
	createDBErr     string

	// Row deletion confirmation dialog (non-empty table name while pending).
	deleteRowsConfirmTable string
	deleteRowsConfirmQuery string
	deleteRowsConfirmCount int

	// Schema DDL confirmation (add/edit column, etc.).
	schemaConfirmSQL    string
	schemaConfirmTable  string
	schemaConfirmAction db.SchemaAction

	// Clear-history confirmation dialog.
	clearHistoryConfirm bool

	// Clear-bookmarks confirmation dialog.
	clearBookmarksConfirm bool

	config        *config.Config
	connection    *db.Connection
	historyStore  *history.Store
	bookmarkStore *bookmarks.Store
	connError    string
	tables       []string

	// Pagination
	page     int
	pageSize int
	lastQuery string
	pageMsg  string
	totalRows    int  // total rows in the current table (0 = unknown)
	totalRowsSet bool // whether totalRows has been fetched for this query
	statsMsg string // transient column statistics display
	exportMsg   string // transient CSV export result display
	searchMsg   string // transient regex search result display
	truncateMsg   string // transient truncate result display
	deleteRowsMsg string // transient row deletion result display
	schemaMsg   string // transient schema change result display
	bookmarkMsg string // transient bookmark result display

	// flashGen tracks the current "generation" of the transient status message.
	// Each time a new flash is set, the wrapper increments this and arms a
	// flashTickMsg. When the tick fires it only clears the flash if the
	// generation still matches, preventing it from wiping a newer message.
	flashGen uint64

	// Quick filters (cell-based, server-side WHERE injection)
	baseQuery string   // original query without filters
	filters   []string // active filter expressions, AND-joined

	// Quick sort (single-column, server-side ORDER BY)
	sortCol string // column name, "" = no sort
	sortDir string // "ASC" or "DESC"

	// Foreign-key navigation stack (gb to go back).
	queryStack        []queryStackEntry
	restoreCursor     bool
	restoreCursorRow  int
	restoreCursorCol  int

	// Column jump (: to fuzzy-match and move the column cursor).
	columnJumping bool
	columnJump    string

	// Client-side regex search (g/ to search, n/N to jump between matches).
	searching   bool
	searchQuery string
	lastSearch  string

	// hintFlash is the individual key currently highlighted white on the status bar.
	hintFlash   string
	hintFlashAt time.Time
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
		bookmarks:    NewBookmarkPanel(),
		dbPicker:     NewDatabasePicker(),
		help:         NewHelpPanel(),
		filterPicker: NewFilterPicker(),
		columnPicker: NewColumnPicker(),
		exportPicker: NewExportPicker(),
		importPrompt: NewImportPrompt(),
		addColumnForm:   NewAddColumnForm(),
		tableRenameForm: NewTableRenameForm(),
		tableDesigner:   NewTableDesigner(),
		schemaEditor:    NewSchemaEditor(),
		cellEdit:        NewCellEditPopup(),
		historyStore:  history.NewStore(historyDir()),
		bookmarkStore: bookmarks.NewStore(historyDir()),
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
			detail = conn.Host
			if conn.Port != 0 && conn.Port != 3306 {
				detail = fmt.Sprintf("%s:%d", detail, conn.Port)
			}
		}
		if conn.SSHHost != "" {
			detail = fmt.Sprintf("%s via %s", detail, conn.SSHHost)
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
	m.statsMsg = ""
	m.exportMsg = ""
	m.searchMsg = ""
	m.results.SetSearchMatcher(nil)
	m.queryStack = nil
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

// executeQuery runs the query under the cursor asynchronously with pagination.
// When the editor contains multiple statements, only the one under the cursor
// is executed.
func (m *Model) executeQuery() tea.Cmd {
	query := m.editor.StatementAtCursor()
	if query == "" {
		return nil
	}

	m.lastQuery = query
	m.baseQuery = query
	m.filters = nil
	m.sortCol = ""
	m.sortDir = ""
	m.page = 0
	m.queryStack = nil
	m.totalRows = 0
	m.totalRowsSet = false
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

// buildPageMsg constructs the pagination status string using the current
// page position, row count, and total rows (if known).
func (m Model) buildPageMsg(page, pageSize, rowCount int, hasNext bool) string {
	offset := page * pageSize
	if m.totalRowsSet {
		if hasNext {
			return fmt.Sprintf("page %d (rows %d-%d of %s)", page+1, offset+1, offset+rowCount, formatCount(m.totalRows))
		}
		if page > 0 {
			return fmt.Sprintf("page %d (rows %d-%d of %s)", page+1, offset+1, offset+rowCount, formatCount(m.totalRows))
		}
		return fmt.Sprintf("%s rows", formatCount(m.totalRows))
	}
	if page > 0 || hasNext {
		pgInfo := fmt.Sprintf("page %d (rows %d-%d)", page+1, offset+1, offset+rowCount)
		if hasNext {
			pgInfo += " · more available"
		}
		return pgInfo
	}
	return ""
}

// formatCount formats an integer with thousands separators.
func formatCount(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + formatCount(-n)
	}
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}
	return string(result)
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

// detectResultMetadata loads table context (foreign keys, editability) for a query.
func (m *Model) detectResultMetadata(query string) {
	m.results.ClearForeignKeys()
	m.detectEditability(query)

	table := parseSimpleSelectTable(query)
	if table == "" || m.connection == nil {
		return
	}

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

	fks, err := m.connection.DB().ForeignKeys(table)
	if err != nil {
		return
	}
	m.results.SetForeignKeys(table, fks)
}

// canFilter reports whether the current results support quick-filtering
// (i.e. the base query is a simple single-table SELECT).
func (m Model) canFilter() bool {
	if m.connection == nil || m.baseQuery == "" {
		return false
	}
	return parseSimpleSelectTable(m.baseQuery) != ""
}

// buildFilteredQuery reconstructs the query from the known table name with
// all active filters and sort applied. Since canFilter() guarantees a simple
// SELECT * FROM <table>, we rebuild from scratch to avoid issues with existing
// LIMIT/ORDER BY clauses in the original query.
func (m Model) buildFilteredQuery() string {
	table := parseSimpleSelectTable(m.baseQuery)
	q := fmt.Sprintf("SELECT * FROM %s", table)
	if len(m.filters) > 0 {
		q += " WHERE " + strings.Join(m.filters, " AND ")
	}
	if m.sortCol != "" {
		q += fmt.Sprintf(" ORDER BY %s %s", m.sortCol, m.sortDir)
	}
	return q
}

// applyFilteredQuery rebuilds lastQuery from active filters/sort and mirrors it
// into the editor when results-driven actions change the effective SQL.
func (m *Model) applyFilteredQuery() {
	m.lastQuery = m.buildFilteredQuery()
	m.syncEditorQuery()
}

// syncEditorQuery updates the editor to reflect lastQuery. Skipped while the
// editor is focused so an in-progress draft is not overwritten.
func (m *Model) syncEditorQuery() {
	if m.focus == FocusEditor || m.lastQuery == "" {
		return
	}
	q := strings.TrimRight(strings.TrimSpace(m.lastQuery), ";")
	m.editor.SetValue(q + ";")
}

// openColumnPicker opens the column-visibility overlay, seeded with the
// current results columns and their hidden state.
func (m *Model) openColumnPicker() {
	hidden := make(map[string]bool)
	for _, name := range m.results.HiddenColumnNames() {
		hidden[name] = true
	}
	m.columnPicker.Show(m.results.columns, hidden)
}

// applyColumnVisibility commits the picker's selection to the results table
// and closes the overlay.
func (m *Model) applyColumnVisibility() tea.Cmd {
	hidden := m.columnPicker.HiddenColumns()
	m.columnPicker.Hide()
	m.results.SetHiddenColumns(hidden)
	return nil
}

// openFilterPicker opens the value picker for the current column,
// fetching distinct values from the database asynchronously.
func (m *Model) openFilterPicker() tea.Cmd {
	if !m.canFilter() || m.results.NumRows() == 0 {
		return nil
	}
	colName := m.results.ColumnName(m.results.CursorCol())
	if colName == "" {
		return nil
	}

	table := parseSimpleSelectTable(m.baseQuery)
	m.filterPicker.Show(colName)

	conn := m.connection
	return func() tea.Msg {
		result, err := conn.DB().Execute(fmt.Sprintf("SELECT DISTINCT %s FROM %s", colName, table))
		if err != nil {
			return filterValuesMsg{column: colName}
		}
		values := make([]string, 0, len(result.Rows))
		for _, row := range result.Rows {
			if len(row) > 0 {
				values = append(values, row[0])
			}
		}
		return filterValuesMsg{column: colName, values: values}
	}
}

// applyFilterPickerSelection takes the selected values from the picker
// and applies them as a filter (IN clause or IS NULL), then re-executes.
func (m *Model) applyFilterPickerSelection() tea.Cmd {
	colName := m.filterPicker.Column()
	selected := m.filterPicker.SelectedValues()
	m.filterPicker.Hide()

	// Remove any existing equality/IN filter on this column.
	if idx, _, found := findEqualityFilter(m.filters, colName); found {
		m.filters = append(m.filters[:idx], m.filters[idx+1:]...)
	}

	if len(selected) > 0 {
		escaped := make([]string, len(selected))
		for i, v := range selected {
			escaped[i] = strings.ReplaceAll(v, "'", "''")
		}
		m.filters = append(m.filters, buildInClause(colName, escaped))
	}

	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// clearFilters removes all active filters and re-executes the base query.
func (m *Model) clearFilters() tea.Cmd {
	if len(m.filters) == 0 {
		return nil
	}
	m.filters = nil
	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// undoFilter removes the last-added filter and re-executes the query.
func (m *Model) undoFilter() tea.Cmd {
	if len(m.filters) == 0 {
		return nil
	}
	m.filters = m.filters[:len(m.filters)-1]
	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// fetchColumnStats computes statistics for the cursor column and displays
// them as a transient status-bar message. For simple SELECT * FROM <table>
// queries, stats are computed server-side across the full filtered result
// set. For arbitrary queries, stats are computed client-side on the current
// page.
func (m *Model) fetchColumnStats() tea.Cmd {
	if m.results.NumRows() == 0 || m.connection == nil {
		return nil
	}
	col := m.results.CursorCol()
	colName := m.results.ColumnName(col)
	if colName == "" {
		return nil
	}
	dbType := m.results.ColumnType(col)
	numeric := isNumericType(dbType)

	// Server-side stats on the full filtered result set.
	if m.canFilter() {
		table := parseSimpleSelectTable(m.baseQuery)
		var aggregate string
		if numeric {
			aggregate = fmt.Sprintf(
				"SELECT COUNT(%s), COUNT(DISTINCT %s), MIN(%s), MAX(%s), SUM(%s), AVG(%s) FROM %s",
				colName, colName, colName, colName, colName, colName, table)
		} else {
			aggregate = fmt.Sprintf(
				"SELECT COUNT(%s), COUNT(DISTINCT %s), MIN(%s), MAX(%s) FROM %s",
				colName, colName, colName, colName, table)
		}
		if len(m.filters) > 0 {
			aggregate += " WHERE " + strings.Join(m.filters, " AND ")
		}

		conn := m.connection
		return func() tea.Msg {
			result, err := conn.DB().Execute(aggregate)
			if err != nil || len(result.Rows) == 0 {
				return statsMsg{column: colName, stats: "stats error"}
			}
			row := result.Rows[0]
			stats := formatColumnStats(row, numeric)
			return statsMsg{column: colName, stats: stats}
		}
	}

	// Client-side fallback: stats on the current page.
	row := computeClientStats(m.results, col, numeric)
	stats := formatColumnStats(row, numeric)
	m.statsMsg = fmt.Sprintf("%s: %s  (page only)", colName, stats)
	return nil
}

// fetchTotalRows runs an async COUNT(*) on the current table and returns a
// countMsg. Returns nil if no table can be resolved.
func (m *Model) fetchTotalRows() tea.Cmd {
	if m.connection == nil || !m.canFilter() {
		return nil
	}
	table := parseSimpleSelectTable(m.baseQuery)
	if table == "" {
		return nil
	}
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if len(m.filters) > 0 {
		countQuery += " WHERE " + strings.Join(m.filters, " AND ")
	}
	conn := m.connection
	return func() tea.Msg {
		result, err := conn.DB().Execute(countQuery)
		if err != nil || len(result.Rows) == 0 {
			return countMsg{total: 0, err: fmt.Errorf("count failed")}
		}
		n, _ := strconv.Atoi(result.Rows[0][0])
		return countMsg{total: n}
	}
}

// computeClientStats iterates the visible rows for a column and returns a
// stats row in the same layout as the server-side query: count, distinct,
// min, max, [sum, avg].
func computeClientStats(r ResultsTable, col int, numeric bool) []string {
	count := 0
	seen := make(map[string]bool)
	hasMin, hasMax := false, false
	minVal, maxVal := 0.0, 0.0
	var sum float64
	for rowIdx := 0; rowIdx < r.NumRows(); rowIdx++ {
		val := r.RowValue(rowIdx, col)
		if val == "" || val == "NULL" {
			continue
		}
		count++
		seen[val] = true
		if numeric {
			n, ok := parseFloat(val)
			if ok {
				sum += n
				if !hasMin || n < minVal {
					minVal = n
					hasMin = true
				}
				if !hasMax || n > maxVal {
					maxVal = n
					hasMax = true
				}
			}
		} else {
			if !hasMin || val < fmt.Sprintf("%v", minVal) {
				minVal = 0
				hasMin = true
			}
		}
	}
	distinct := fmt.Sprintf("%d", len(seen))
	row := []string{fmt.Sprintf("%d", count), distinct}
	if numeric {
		if hasMin {
			row = append(row, fmt.Sprintf("%g", minVal))
		} else {
			row = append(row, "NULL")
		}
		if hasMax {
			row = append(row, fmt.Sprintf("%g", maxVal))
		} else {
			row = append(row, "NULL")
		}
		row = append(row, fmt.Sprintf("%g", sum))
		if count > 0 {
			row = append(row, fmt.Sprintf("%g", sum/float64(count)))
		} else {
			row = append(row, "NULL")
		}
	} else {
		if mi, ma := minString(seen), maxString(seen); mi != "" || ma != "" {
			row = append(row, mi, ma)
		} else {
			row = append(row, "NULL", "NULL")
		}
	}
	return row
}

// formatColumnStats renders a stats row as a compact status-bar string.
func formatColumnStats(row []string, numeric bool) string {
	get := func(i int) string {
		if i < len(row) {
			return row[i]
		}
		return "?"
	}
	if numeric {
		return fmt.Sprintf("count %s · distinct %s · min %s · max %s · sum %s · avg %s",
			get(0), get(1), get(2), get(3), get(4), get(5))
	}
	return fmt.Sprintf("count %s · distinct %s · min %s · max %s",
		get(0), get(1), get(2), get(3))
}

// parseFloat parses a numeric cell value (tolerating integers and decimals).
func parseFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err == nil
}

// minString / maxString find the lexicographically smallest/largest key.
func minString(m map[string]bool) string {
	first := true
	var result string
	for k := range m {
		if first || k < result {
			result = k
			first = false
		}
	}
	return result
}

func maxString(m map[string]bool) string {
	first := true
	var result string
	for k := range m {
		if first || k > result {
			result = k
			first = false
		}
	}
	return result
}

// exportToCSV writes result rows to a CSV file in ~/Downloads. If rows are
// marked, it re-queries the full set of marked rows by primary key; otherwise
// it exports the current page. Returns a command that delivers exportDoneMsg.
func (m *Model) exportToCSV() tea.Cmd {
	if m.results.NumRows() == 0 {
		m.exportMsg = "nothing to export"
		return nil
	}

	cols := m.results.columns
	table := m.results.SourceTable()

	// Marked rows: re-query for complete data (may span multiple pages).
	if m.results.IsEditable() && m.results.MarkCount() > 0 {
		tuples := m.results.MarkedPKs()
		pkNames := m.results.PKColumns()
		pkTypes := m.results.PKTypes()
		clause := buildPKInClause(pkNames, pkTypes, tuples)

		conn := m.connection
		query := fmt.Sprintf("SELECT * FROM %s WHERE %s", table, clause)
		timestamp := time.Now().Format("20060102_150405")
		filename := exportFilename(table, timestamp)

		return func() tea.Msg {
			result, err := conn.DB().Execute(query)
			if err != nil {
				return exportDoneMsg{err: err}
			}
			exportCols := make([]string, len(result.Columns))
			for i, c := range result.Columns {
				exportCols[i] = c.Name
			}
			path, count, err := writeCSV(exportCols, result.Rows, filename)
			return exportDoneMsg{path: path, count: count, err: err}
		}
	}

	// No marks: export current page in memory.
	timestamp := time.Now().Format("20060102_150405")
	filename := exportFilename(table, timestamp)
	rows := m.results.rows
	path, count, err := writeCSV(cols, rows, filename)
	m.exportMsg = exportStatusMessage(path, count, err)
	return nil
}

// exportFilename builds a safe filename for a CSV export.
func exportFilename(table, timestamp string) string {
	name := table
	if name == "" {
		name = "query"
	}
	return fmt.Sprintf("gsql_%s_%s.csv", name, timestamp)
}

// exportStatusMessage renders the result of an export for the status bar.
func exportStatusMessage(path string, count int, err error) string {
	if err != nil {
		return fmt.Sprintf("export failed: %v", err)
	}
	return fmt.Sprintf("exported %d rows → %s", count, path)
}

// writeCSV serializes columns and rows to a CSV file in ~/Downloads and
// returns the absolute path and row count.
func writeCSV(cols []string, rows [][]string, filename string) (string, int, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", 0, err
	}
	dir = filepath.Join(dir, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(cols); err != nil {
		return path, 0, err
	}
	for _, row := range rows {
		// Normalize NULL display values to empty fields in CSV.
		out := make([]string, len(row))
		for i, v := range row {
			if v == "NULL" {
				out[i] = ""
			} else {
				out[i] = v
			}
		}
		if err := w.Write(out); err != nil {
			return path, 0, err
		}
	}
	w.Flush()
	return path, len(rows), w.Error()
}

// serializeCSV renders columns and rows as CSV to a string, for testing.
func serializeCSV(cols []string, rows [][]string) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(cols); err != nil {
		return "", err
	}
	for _, row := range rows {
		out := make([]string, len(row))
		for i, v := range row {
			if v == "NULL" {
				out[i] = ""
			} else {
				out[i] = v
			}
		}
		if err := w.Write(out); err != nil {
			return "", err
		}
	}
	w.Flush()
	return buf.String(), nil
}

// execExportDump starts a table-by-table SQL dump of the selected tables,
// writing to ~/Downloads with a timestamped filename. It writes the header and
// first table, then chains per-table commands via exportProgressMsg so the
// status bar can show live progress.
func (m *Model) execExportDump(tables []string) tea.Cmd {
	if m.connection == nil || len(tables) == 0 {
		return nil
	}
	conn := m.connection
	driver := conn.Config().Driver
	realDBName := conn.Config().Database
	fileLabel := filepath.Base(realDBName)
	if fileLabel == "" {
		fileLabel = "database"
	}
	timestamp := time.Now().Format("2006-01-02")
	ext := string(m.exportPicker.CurrentFormat())
	filename := fmt.Sprintf("%s_%s.%s", fileLabel, timestamp, ext)
	total := len(tables)
	database := conn.DB()

	return func() tea.Msg {
		dir, err := os.UserHomeDir()
		if err != nil {
			return exportProgressMsg{err: err, total: total}
		}
		dir = filepath.Join(dir, "Downloads")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return exportProgressMsg{err: err, total: total}
		}
		path := filepath.Join(dir, filename)
		f, err := os.Create(path)
		if err != nil {
			return exportProgressMsg{err: err, total: total}
		}
		bw := bufio.NewWriter(f)
		if err := db.DumpHeader(bw, driver, realDBName, total); err != nil {
			f.Close()
			return exportProgressMsg{err: err, path: path, total: total}
		}
		if err := db.DumpTable(bw, database, driver, tables[0]); err != nil {
			f.Close()
			return exportProgressMsg{err: err, path: path, total: total}
		}
		bw.Flush()
		return exportProgressMsg{
			file:   f,
			bw:     bw,
			path:   path,
			index:  0,
			total:  total,
			tables: tables,
			name:   tables[0],
		}
	}
}

// dumpTableCmd returns a command that writes a single table to an open dump
// file and reports progress via exportProgressMsg.
func dumpTableCmd(f *os.File, bw *bufio.Writer, database db.DB, driver db.Driver, table string, index, total int, tables []string, path string) tea.Cmd {
	return func() tea.Msg {
		if err := db.DumpTable(bw, database, driver, table); err != nil {
			f.Close()
			return exportProgressMsg{err: err, path: path, index: index, total: total, tables: tables, name: table}
		}
		bw.Flush()
		return exportProgressMsg{
			file:   f,
			bw:     bw,
			path:   path,
			index:  index,
			total:  total,
			tables: tables,
			name:   table,
		}
	}
}

// dumpFooterCmd returns a command that writes the dump footer, flushes, and
// closes the file, reporting completion via exportDumpMsg.
func dumpFooterCmd(f *os.File, bw *bufio.Writer, driver db.Driver, total int, path string) tea.Cmd {
	return func() tea.Msg {
		if err := db.DumpFooter(bw, driver); err != nil {
			f.Close()
			return exportDumpMsg{path: path, err: err}
		}
		bw.Flush()
		f.Close()
		return exportDumpMsg{path: path, tables: total}
	}
}

// execImportSQL runs an async SQL import from the given file path, reporting
// execImportSQL starts an async SQL import from the given file path. It runs
// ImportSQL in a goroutine that streams byte-progress over a channel; a polling
// command turns each update into an importProgressMsg for the status bar, and
// the final result is delivered as importDoneMsg.
func (m *Model) execImportSQL(rawPath string) tea.Cmd {
	if m.connection == nil {
		return nil
	}
	database := m.connection.DB()
	filename := filepath.Base(rawPath)

	// progress is a buffered channel: ImportSQL writes progress updates, and
	// waitForImportProgress reads them. A buffer of 1 lets the first update
	// land without blocking if the receiver hasn't been scheduled yet.
	progress := make(chan importProgressMsg, 1)
	done := make(chan importDoneMsg, 1)

	// Run the import in a goroutine so the TUI stays responsive.
	go func() {
		f, err := os.Open(rawPath)
		if err != nil {
			done <- importDoneMsg{filename: filename, err: err}
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			done <- importDoneMsg{filename: filename, err: err}
			return
		}
		totalSize := stat.Size()

		result, err := db.ImportSQL(f, database, totalSize, func(read int64, total int64) {
			progress <- importProgressMsg{filename: filename, read: read, total: total}
		})
		if err != nil {
			done <- importDoneMsg{filename: filename, err: err}
			return
		}
		done <- importDoneMsg{result: result, filename: filename}
	}()

	// Start polling for progress / completion.
	return waitForImportProgress(progress, done)
}

// waitForImportProgress is a tea.Cmd that blocks until either a progress update
// or the final result arrives from the import goroutine. It returns the
// appropriate message and, for progress updates, re-issues itself to continue
// polling.
func waitForImportProgress(progress <-chan importProgressMsg, done <-chan importDoneMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-done:
			return msg
		case msg := <-progress:
			return importProgressWrapper{msg: msg, progress: progress, done: done}
		}
	}
}

// importProgressWrapper carries a progress update together with the channels
// needed to re-issue the polling command.
type importProgressWrapper struct {
	msg      importProgressMsg
	progress <-chan importProgressMsg
	done     <-chan importDoneMsg
}

// none → ASC → DESC → none.
func (m *Model) toggleSort() tea.Cmd {
	if !m.canFilter() || m.results.NumRows() == 0 {
		return nil
	}
	colName := m.results.ColumnName(m.results.CursorCol())
	if colName == "" {
		return nil
	}
	switch {
	case m.sortCol == "":
		m.sortCol = colName
		m.sortDir = "ASC"
	case m.sortCol == colName && m.sortDir == "ASC":
		m.sortDir = "DESC"
	default:
		m.sortCol = ""
		m.sortDir = ""
	}
	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// preserveCursorCol sets up cursor restoration so the column position
// survives the query re-execution (row resets to 0 since data order changes).
func (m *Model) preserveCursorCol() {
	m.restoreCursor = true
	m.restoreCursorRow = 0
	m.restoreCursorCol = m.results.CursorCol()
}

// findEqualityFilter looks for an existing `=` or `IN` filter on colName.
// Returns the filter index, the set of values, and whether one was found.
func findEqualityFilter(filters []string, colName string) (int, []string, bool) {
	for i, f := range filters {
		eqPrefix := colName + " = '"
		if strings.HasPrefix(f, eqPrefix) && strings.HasSuffix(f, "'") {
			return i, []string{f[len(eqPrefix) : len(f)-1]}, true
		}
		inPrefix := colName + " IN ("
		if strings.HasPrefix(f, inPrefix) && strings.HasSuffix(f, ")") {
			inner := f[len(inPrefix) : len(f)-1]
			parts := strings.Split(inner, ", ")
			vals := make([]string, 0, len(parts))
			for _, p := range parts {
				vals = append(vals, strings.Trim(p, "'"))
			}
			return i, vals, true
		}
	}
	return -1, nil, false
}

// buildInClause builds a filter expression for a set of values.
// Single value → col = 'val', multiple → col IN ('v1', 'v2', ...).
func buildInClause(colName string, values []string) string {
	if len(values) == 1 {
		return fmt.Sprintf("%s = '%s'", colName, values[0])
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("'%s'", v)
	}
	return fmt.Sprintf("%s IN (%s)", colName, strings.Join(quoted, ", "))
}

// isNumericType reports whether a database column type should be treated as
// numeric (and therefore left unquoted in generated WHERE fragments).
func isNumericType(dbType string) bool {
	if dbType == "" {
		return false
	}
	t := strings.ToLower(dbType)
	// Strip common "(n)" or "(n,m)" suffixes, e.g. "decimal(10,2)".
	if i := strings.IndexByte(t, '('); i > 0 {
		t = t[:i]
	}
	switch t {
	case "int", "integer", "tinyint", "smallint", "mediumint", "bigint",
		"unsigned", "int unsigned", "unsigned big int",
		"real", "double", "float", "decimal", "numeric":
		return true
	}
	return false
}

// formatFilterValue quotes a literal for use in a WHERE fragment, leaving
// numeric values bare so they don't get string-coerced on MySQL.
func formatFilterValue(value, dbType string) string {
	if isNumericType(dbType) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// buildQuickFilter constructs a single-column WHERE fragment for a cell value.
// negate=true builds a NULL-safe "not equal" form so that pressing `!` on a
// nullable column does not silently drop NULL rows (the plain `!=` operator
// treats NULL comparisons as unknown). A NULL cell value maps to IS NULL /
// IS NOT NULL instead of an equality comparison.
func buildQuickFilter(colName, value, dbType string, negate bool) string {
	if value == "NULL" {
		if negate {
			return fmt.Sprintf("%s IS NOT NULL", colName)
		}
		return fmt.Sprintf("%s IS NULL", colName)
	}
	v := formatFilterValue(value, dbType)
	if negate {
		return fmt.Sprintf("(%s != %s OR %s IS NULL)", colName, v, colName)
	}
	return fmt.Sprintf("%s = %s", colName, v)
}

// removeColumnFilters drops every filter fragment that belongs to colName.
// It recognizes all generated shapes: `col = ...`, `col IN (...)`,
// `col IS [NOT] NULL`, and the NULL-safe negate form `(col != ... OR ...)`,
// so quick filters and the value picker interoperate cleanly.
func removeColumnFilters(filters []string, colName string) []string {
	prefix := colName + " "
	groupPrefix := "(" + colName + " "
	out := make([]string, 0, len(filters))
	for _, f := range filters {
		if strings.HasPrefix(f, prefix) || strings.HasPrefix(f, groupPrefix) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// quickFilterCell builds a single-column filter from the cursor cell value
// and applies it, replacing any existing filter on that column. negate=false
// keeps rows matching the value (`*`); negate=true hides them (`!`).
func (m *Model) quickFilterCell(negate bool) tea.Cmd {
	if !m.canFilter() || m.results.NumRows() == 0 {
		return nil
	}
	col := m.results.CursorCol()
	colName := m.results.ColumnName(col)
	if colName == "" {
		return nil
	}
	value := m.results.CursorCellValue()
	if value == "" {
		return nil
	}
	dbType := m.results.ColumnType(col)

	m.filters = removeColumnFilters(m.filters, colName)
	m.filters = append(m.filters, buildQuickFilter(colName, value, dbType, negate))

	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// filterByMarks builds a WHERE pk IN (...) clause from the marked rows and
// applies it as a filter, then clears the marks. Marks are consumed because
// the resulting filter now represents them in the active result set.
func (m *Model) filterByMarks() tea.Cmd {
	if !m.canFilter() || !m.results.IsEditable() {
		return nil
	}
	tuples := m.results.MarkedPKs()
	if len(tuples) == 0 {
		return nil
	}
	pkNames := m.results.PKColumns()
	pkTypes := m.results.PKTypes()

	clause := buildPKInClause(pkNames, pkTypes, tuples)

	// Replace any existing filter on the PK column(s) so pressing F twice
	// doesn't stack redundant clauses.
	for _, pk := range pkNames {
		m.filters = removeColumnFilters(m.filters, pk)
	}
	// For composite PKs the clause starts with "(pk1, pk2) IN ..."; remove
	// any stale composite clause on the same leading column.
	if len(pkNames) > 1 {
		m.filters = removeColumnFilters(m.filters, "("+pkNames[0]+" ")
	}
	m.filters = append(m.filters, clause)
	m.results.ClearMarks()

	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

// buildPKInClause constructs a type-correct IN filter from PK tuples.
// Single PK:  pk IN (v1, v2, ...)
// Composite:  (pk1, pk2) IN ((v1a, v2a), (v1b, v2b))
func buildPKInClause(pkNames, pkTypes []string, tuples [][]string) string {
	if len(pkNames) == 1 {
		parts := make([]string, len(tuples))
		for i, t := range tuples {
			v := ""
			if len(t) > 0 {
				v = t[0]
			}
			parts[i] = formatFilterValue(v, pkTypes[0])
		}
		return fmt.Sprintf("%s IN (%s)", pkNames[0], strings.Join(parts, ", "))
	}
	rows := make([]string, len(tuples))
	for i, t := range tuples {
		vals := make([]string, len(pkNames))
		for j := range pkNames {
			v := ""
			if j < len(t) {
				v = t[j]
			}
			vals[j] = formatFilterValue(v, firstOr(pkTypes, j, ""))
		}
		rows[i] = "(" + strings.Join(vals, ", ") + ")"
	}
	return fmt.Sprintf("(%s) IN (%s)", strings.Join(pkNames, ", "), strings.Join(rows, ", "))
}

// firstOr returns types[i] if in range, else fallback.
func firstOr(types []string, i int, fallback string) string {
	if i >= 0 && i < len(types) {
		return types[i]
	}
	return fallback
}

// pluralIf returns suffix (e.g. "es") when cond is true, "" otherwise.
// Used for simple pluralization like "1 match" vs "2 matches".
func pluralIf(cond bool, suffix string) string {
	if cond {
		return suffix
	}
	return ""
}

// compactFilter shortens a raw WHERE fragment for display in the status bar.
// IN (...) lists collapse to "col ∈ (n)" and NULL-safe negates collapse to
// "col ≠ v", so a handful of filters fit on one line.
func compactFilter(f string) string {
	// NULL-safe negate: (col != v OR col IS NULL) → col ≠ v
	if strings.HasPrefix(f, "(") && strings.HasSuffix(f, ")") {
		inner := f[1 : len(f)-1]
		if orIdx := strings.Index(inner, " OR "); orIdx >= 0 {
			left := inner[:orIdx]
			if eq := strings.Index(left, " != "); eq >= 0 {
				return left[:eq] + " ≠" + left[eq+3:]
			}
		}
	}
	// IN (...) → col ∈ (n)
	if i := strings.Index(f, " IN ("); i >= 0 && strings.HasSuffix(f, ")") {
		inner := f[i+len(" IN (") : len(f)-1]
		prefix := f[:i]
		var count int
		if strings.HasPrefix(prefix, "(") {
			// Composite PK: (pk1,pk2) IN ((..),(..)) — count tuples, not values.
			count = strings.Count(inner, "), (") + 1
		} else {
			count = strings.Count(inner, ",") + 1
		}
		return prefix + fmt.Sprintf(" ∈ (%d)", count)
	}
	// IS NULL / IS NOT NULL / equality → keep as-is (short enough).
	return f
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeString(s []string, v string) []string {
	for i, x := range s {
		if x == v {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
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

	conn := m.connection
	table := m.results.SourceTable()
	columns := m.results.TableColumns()
	values := insertValuesByName(m.results, m.inspector.InsertValues())

	query, args, err := buildInsertQuery(table, columns, values)
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

// execTruncate removes all rows from a table asynchronously.
func (m *Model) execTruncate(table string) tea.Cmd {
	if m.connection == nil || table == "" {
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
func (m *Model) startDeleteRows() {
	if !m.results.IsEditable() || m.results.NumRows() == 0 {
		return
	}
	pkNames := m.results.PKColumns()
	if len(pkNames) == 0 {
		return
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
			return
		}
		count = 1
		query = buildDeleteQuery(table, pkNames, pkTypes, [][]string{tuple})
	}

	m.deleteRowsConfirmTable = table
	m.deleteRowsConfirmQuery = query
	m.deleteRowsConfirmCount = count
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

// execSchemaDDL runs a pending schema statement asynchronously.
func (m *Model) execSchemaDDL(table, query string, action db.SchemaAction, newTable string) tea.Cmd {
	if m.connection == nil || table == "" || query == "" {
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		_, err := conn.DB().Exec(query)
		return schemaResultMsg{table: table, newTable: newTable, action: action, err: err}
	}
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
func (m *Model) openSchemaPanel() {
	if m.connection == nil {
		return
	}
	table := m.sidebarSelectedTable()
	if table == "" {
		return
	}
	cols, err := m.connection.DB().TableColumnInfo(table)
	if err != nil {
		m.connError = err.Error()
		return
	}
	m.schemaEditor.Show(table, m.connection.Config().Driver, cols)
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
	m.setSchemaConfirm(m.schemaEditor.Table(), sql, db.SchemaDropColumn)
	return nil
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

// Update is the top-level Bubble Tea update handler. It wraps the real
// handler (update) with transient-flash auto-expiry: after processing any
// message, if a flash field changed, a timer is armed to clear the status bar
// after flashExpiry unless a newer message supersedes it.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prev := m.flashSnapshot()
	model, cmd := m.update(msg)
	m = model.(Model)
	if m.flashChanged(prev) && m.anyFlashActive() {
		m.flashGen++
		gen := m.flashGen
		tick := tea.Tick(flashExpiry, func(time.Time) tea.Msg {
			return flashTickMsg{gen: gen}
		})
		if cmd != nil {
			return m, tea.Batch(cmd, tick)
		}
		return m, tick
	}
	return m, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

		// Flash the matching hint group on the status bar (set before
		// dispatch so the value survives the value-receiver copy).
		if matched := matchHint(m.hintList(), msg.String()); matched != "" {
			m.hintFlash = matched
			m.hintFlashAt = time.Now()
		} else {
			m.hintFlash = ""
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
		var cmd tea.Cmd
		if msg.err != nil {
			m.results.SetError(msg.err.Error())
			if m.restoreCursor {
				m.restoreCursor = false
			}
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
			m.pageMsg = m.buildPageMsg(msg.page, msg.pageSize, len(rows), hasNext)

			// Fire a background COUNT(*) on the first page of a table browse.
			if msg.page == 0 {
				cmd = m.fetchTotalRows()
			}

			// Enable inline editing and foreign-key navigation for simple table SELECTs.
			m.detectResultMetadata(msg.query)
			m.inspector.Reset()
			if m.restoreCursor {
				m.results.SetCursor(m.restoreCursorRow, m.restoreCursorCol)
				m.restoreCursor = false
			}
		}
		return m, cmd

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

	case insertResultMsg:
		if msg.err != nil {
			m.results.SetSaveError(msg.err.Error())
		} else {
			m.inspector.CancelInsert()
			m.results.ConfirmSaved()
			return m, m.runPageQuery()
		}
		return m, nil

	case truncateResultMsg:
		if msg.err != nil {
			m.truncateMsg = fmt.Sprintf("truncate failed: %v", msg.err)
		} else {
			m.truncateMsg = fmt.Sprintf("truncated %s", msg.table)
			if m.resultsShowTable(msg.table) {
				if m.results.HasDirtyCells() {
					m.results.DiscardEdits()
				}
				return m, m.runPageQuery()
			}
		}
		return m, nil

	case deleteRowsResultMsg:
		if msg.err != nil {
			m.deleteRowsMsg = fmt.Sprintf("delete failed: %v", msg.err)
		} else {
			m.deleteRowsMsg = fmt.Sprintf("deleted %d row%s from %s", msg.count, pluralIf(msg.count != 1, "s"), msg.table)
			m.results.ClearMarks()
			if m.resultsShowTable(msg.table) {
				if m.results.HasDirtyCells() {
					m.results.DiscardEdits()
				}
				return m, m.runPageQuery()
			}
		}
		return m, nil

	case schemaResultMsg:
		if msg.err != nil {
			m.schemaMsg = fmt.Sprintf("schema change failed: %v", msg.err)
			m.clearSchemaConfirm()
			switch msg.action {
			case db.SchemaAddColumn:
				m.addColumnForm.SetError(msg.err.Error())
			case db.SchemaRenameTable:
				m.tableRenameForm.SetError(msg.err.Error())
			case db.SchemaCreateTable:
				m.tableDesigner.SetError(msg.err.Error())
			case db.SchemaRenameColumn, db.SchemaModifyType, db.SchemaModifyNullable, db.SchemaModifyDefault, db.SchemaDropColumn:
				m.schemaEditor.SetError(msg.err.Error())
			}
		} else {
			if msg.action == db.SchemaRenameTable {
				m.schemaMsg = fmt.Sprintf("renamed %s to %s", msg.table, msg.newTable)
				m.applyTableRename(msg.table, msg.newTable)
			} else if msg.action == db.SchemaCreateTable {
				m.schemaMsg = fmt.Sprintf("created table %s", msg.table)
				m.loadTables()
				m.syncSidebarCursorToTable(msg.table)
			} else if msg.action == db.SchemaDropTable {
				m.schemaMsg = fmt.Sprintf("dropped table %s", msg.table)
				delete(m.expanded, msg.table)
				delete(m.columnCache, msg.table)
				m.loadTables()
				// Clamp the sidebar cursor into the (now shorter) list.
				items := m.sidebarItems()
				if len(items) == 0 {
					m.sidebarCursor = 0
				} else if m.sidebarCursor >= len(items) {
					m.sidebarCursor = len(items) - 1
				}
				if m.schemaEditor.IsVisible() && m.schemaEditor.Table() == msg.table {
					m.schemaEditor.Hide()
				}
				// If the results panel was showing the dropped table, clear it so
				// the stale query isn't re-run (which would error).
				if m.resultsShowTable(msg.table) {
					m.results.Clear()
					m.lastQuery = ""
					m.baseQuery = ""
					m.filters = nil
					m.editor.SetValue("")
				}
			} else {
				m.schemaMsg = schemaChangeMessage(msg.action, msg.table)
				m = m.refreshTableSchemaSync(msg.table)
				m.reloadSchemaPanel(msg.table)
			}
			m.clearSchemaConfirm()
			m.addColumnForm.Hide()
			m.tableRenameForm.Hide()
			m.tableDesigner.Hide()
			if msg.action == db.SchemaRenameTable && m.resultsShowTable(msg.newTable) {
				return m, tea.Batch(m.prefetchSchemas(), m.runPageQuery())
			}
			if m.resultsShowTable(msg.table) {
				return m, tea.Batch(m.prefetchSchemas(), m.runPageQuery())
			}
			return m, m.prefetchSchemas()
		}
		return m, nil

	case schemasLoadedMsg:
		m.columnCache = msg.schemas
		m.refreshCompletionCandidates()
		return m, nil

	case copyFlashTickMsg:
		if m.results.AdvanceCopyFlash() {
			return m, copyFlashTickCmd()
		}
		return m, nil

	case copyCopiedClearMsg:
		m.results.ClearCopiedMessage()
		return m, nil

	case filterValuesMsg:
		if m.filterPicker.IsVisible() && m.filterPicker.Column() == msg.column {
			// Pre-select values that are already in an existing filter.
			preSelected := make(map[string]bool)
			if _, vals, found := findEqualityFilter(m.filters, msg.column); found {
				for _, v := range vals {
					preSelected[v] = true
				}
			}
			m.filterPicker.SetValues(msg.values, preSelected)
		}
		return m, nil

	case statsMsg:
		m.statsMsg = fmt.Sprintf("%s: %s", msg.column, msg.stats)
		return m, nil

	case countMsg:
		if msg.err == nil {
			m.totalRows = msg.total
			m.totalRowsSet = true
			// Rebuild pageMsg now that the total is known.
			rowCount := m.results.NumRows()
			hasNext := rowCount > m.pageSize
			if hasNext {
				rowCount = m.pageSize
			}
			m.pageMsg = m.buildPageMsg(m.page, m.pageSize, rowCount, hasNext)
		}
		return m, nil

	case exportDoneMsg:
		m.exportMsg = exportStatusMessage(msg.path, msg.count, msg.err)
		return m, nil

	case exportProgressMsg:
		if msg.err != nil {
			if msg.file != nil {
				msg.file.Close()
			}
			m.exportMsg = fmt.Sprintf("export failed: %v", msg.err)
			return m, nil
		}
		percent := (msg.index + 1) * 100 / msg.total
		m.exportMsg = fmt.Sprintf("Exporting %d/%d: %s (%d%%)", msg.index+1, msg.total, msg.name, percent)
		if msg.index+1 < msg.total {
			next := msg.index + 1
			return m, dumpTableCmd(msg.file, msg.bw, m.connection.DB(), m.connection.Config().Driver,
				msg.tables[next], next, msg.total, msg.tables, msg.path)
		}
		return m, dumpFooterCmd(msg.file, msg.bw, m.connection.Config().Driver, msg.total, msg.path)

	case exportDumpMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("export failed: %v", msg.err)
		} else {
			m.exportMsg = fmt.Sprintf("dumped %d table%s → %s", msg.tables, pluralIf(msg.tables != 1, "s"), msg.path)
		}
		return m, nil

	case importProgressWrapper:
		percent := 0
		if msg.msg.total > 0 {
			percent = int(msg.msg.read * 100 / msg.msg.total)
		}
		m.exportMsg = fmt.Sprintf("Importing %s… %d%%", msg.msg.filename, percent)
		return m, waitForImportProgress(msg.progress, msg.done)

	case importDoneMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("import failed: %v", msg.err)
		} else {
			m.exportMsg = msg.result.Summary(msg.filename)
			m.loadTables()
		}
		return m, nil

	case flashTickMsg:
		// Only clear if no newer flash has arrived since this tick was armed.
		if msg.gen == m.flashGen {
			m.clearFlash()
		}
		return m, nil

	case dropDBResultMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("drop database failed: %v", msg.err)
			return m, nil
		}
		m.exportMsg = fmt.Sprintf("dropped database %s", msg.database)
		// If the dropped database was the current one, reconnect without a
		// default database so the picker forces a new selection.
		wasCurrent := m.connection != nil &&
			m.connection.Config().Database == msg.database
		if wasCurrent {
			if err := m.connection.UseDatabase(""); err != nil {
				m.connError = err.Error()
			}
		}
		return m, m.openDatabasePicker(wasCurrent)

	case createDBResultMsg:
		if msg.err != nil {
			m.exportMsg = fmt.Sprintf("create database failed: %v", msg.err)
			return m, nil
		}
		m.exportMsg = fmt.Sprintf("created database %s", msg.database)
		// Switch to the newly created database.
		m.dbPicker.Hide()
		return m, m.selectDatabase(msg.database)
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
	// Help overlay is modal — dismiss on any key.
	if m.help.IsVisible() {
		m.help.Hide()
		return m, nil
	}

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
	case "?":
		m.help.Show()
		return m, nil
	case "n":
		m.state = stateAddConnection
		m.connForm = NewConnectionForm()
		cmd := m.connForm.Focus()
		return m, cmd
	case "e":
		return m.openEditForm()
	case "d":
		return m.deleteSelectedConnection()
	case "/", "i":
		m.connList.StartFilter()
		return m, nil
	case "esc", "q":
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

	// Clear transient status-bar messages on any key press.
	m.clearFlash()

	// Help overlay is modal — dismiss on any key.
	if m.help.IsVisible() {
		m.help.Hide()
		return m, nil
	}
	// Drop-database typed confirmation — intercepts all keys when active.
	if m.dropDBConfirm != "" {
		switch msg.String() {
		case "enter":
			if m.dropDBInput == m.dropDBConfirm {
				dbName := m.dropDBConfirm
				m.dropDBConfirm = ""
				m.dropDBInput = ""
				return m, m.execDropDatabase(dbName)
			}
			return m, nil
		case "esc", "ctrl+c":
			m.dropDBConfirm = ""
			m.dropDBInput = ""
			return m, nil
		case "backspace":
			if len(m.dropDBInput) > 0 {
				m.dropDBInput = m.dropDBInput[:len(m.dropDBInput)-1]
			}
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.dropDBInput += msg.String()
			return m, nil
		}
		return m, nil
	}

	// Create-database name input — intercepts all keys when active.
	if m.createDBActive {
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.createDBInput)
			if name == "" {
				return m, nil
			}
			m.createDBInput = ""
			m.createDBActive = false
			return m, m.execCreateDatabase(name)
		case "esc", "ctrl+c":
			m.createDBActive = false
			m.createDBInput = ""
			m.createDBErr = ""
			return m, nil
		case "backspace":
			if len(m.createDBInput) > 0 {
				m.createDBInput = m.createDBInput[:len(m.createDBInput)-1]
			}
			m.createDBErr = ""
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.createDBInput += msg.String()
			m.createDBErr = ""
			return m, nil
		}
		return m, nil
	}

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
		case "D":
			name := m.dbPicker.SelectedDatabase()
			if name != "" {
				m.dropDBConfirm = name
				m.dropDBInput = ""
			}
			return m, nil
		case "N":
			m.createDBActive = true
			m.createDBInput = ""
			m.createDBErr = ""
			return m, nil
		case "up":
			m.dbPicker.CursorUp()
			return m, nil
		case "down":
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

	// Filter picker is modal — intercept all keys when visible.
	if m.filterPicker.IsVisible() {
		// ctrl+a / ctrl+n work in any state (empty or filtered) and never
		// collide with search typing since they're KeyCtrl, not KeyRunes.
		switch msg.String() {
		case "ctrl+a":
			m.filterPicker.SelectAll()
			return m, nil
		case "ctrl+n":
			m.filterPicker.SelectNone()
			return m, nil
		}
		// Navigation is arrow-keys only so every letter (including j/k) can
		// be typed into the filter at any time.
		switch msg.String() {
		case "esc", "ctrl+c":
			m.filterPicker.Hide()
			return m, nil
		case "enter":
			return m, m.applyFilterPickerSelection()
		case " ":
			m.filterPicker.ToggleSelected()
			return m, nil
		case "up":
			m.filterPicker.CursorUp()
			return m, nil
		case "down":
			m.filterPicker.CursorDown()
			return m, nil
		case "backspace":
			m.filterPicker.FilterBackspace()
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.filterPicker.FilterAddChar(msg.String())
			return m, nil
		}
		return m, nil
	}

	// Column visibility picker is modal — intercept all keys when visible.
	if m.columnPicker.IsVisible() {
		// ctrl+a / ctrl+n work in any state (empty or filtered).
		switch msg.String() {
		case "ctrl+a":
			m.columnPicker.SelectAll()
			return m, nil
		case "ctrl+n":
			m.columnPicker.SelectNone()
			return m, nil
		}
		// Navigation is arrow-keys only so every letter (including j/k) can
		// be typed into the filter at any time.
		switch msg.String() {
		case "esc", "ctrl+c":
			m.columnPicker.Hide()
			return m, nil
		case "enter":
			return m, m.applyColumnVisibility()
		case " ":
			m.columnPicker.ToggleSelected()
			return m, nil
		case "up":
			m.columnPicker.CursorUp()
			return m, nil
		case "down":
			m.columnPicker.CursorDown()
			return m, nil
		case "backspace":
			m.columnPicker.FilterBackspace()
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.columnPicker.FilterAddChar(msg.String())
			return m, nil
		}
		return m, nil
	}

	// Import prompt is modal — intercept all keys.
	if m.importPrompt.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.importPrompt.Hide()
			return m, nil
		case "enter":
			path, err := m.importPrompt.ExpandPath()
			if err != nil {
				m.exportMsg = fmt.Sprintf("import failed: %v", err)
				return m, nil
			}
			m.importPrompt.Hide()
			return m, m.execImportSQL(path)
		}
		var cmd tea.Cmd
		m.importPrompt, cmd = m.importPrompt.Update(msg)
		return m, cmd
	}

	// Export picker is modal — intercept all keys.
	if m.exportPicker.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.exportPicker.Hide()
			return m, nil
		case "enter":
			tables := m.exportPicker.SelectedTables()
			if len(tables) == 0 {
				return m, nil
			}
			m.exportPicker.Hide()
			return m, m.execExportDump(tables)
		case " ":
			m.exportPicker.ToggleSelected()
			return m, nil
		case "a":
			m.exportPicker.SelectAll()
			return m, nil
		case "n":
			m.exportPicker.SelectNone()
			return m, nil
		case "up", "k":
			m.exportPicker.CursorUp()
			return m, nil
		case "down", "j":
			m.exportPicker.CursorDown()
			return m, nil
		}
		return m, nil
	}

	// Drop-table typed-name confirmation is modal — intercept all keys.
	if m.dropTableConfirm != "" {
		switch msg.String() {
		case "enter":
			if m.dropTableInput == m.dropTableConfirm {
				table := m.dropTableConfirm
				m.dropTableConfirm = ""
				m.dropTableInput = ""
				sql, err := db.BuildDropTableSQL(m.connection.Config().Driver, table)
				if err != nil {
					m.schemaMsg = fmt.Sprintf("drop table failed: %v", err)
					return m, nil
				}
				return m, m.execSchemaDDL(table, sql, db.SchemaDropTable, "")
			}
			return m, nil
		case "esc", "ctrl+c":
			m.dropTableConfirm = ""
			m.dropTableInput = ""
			return m, nil
		case "backspace":
			if len(m.dropTableInput) > 0 {
				m.dropTableInput = m.dropTableInput[:len(m.dropTableInput)-1]
			}
			return m, nil
		}
		if msg.Type == tea.KeyRunes {
			m.dropTableInput += msg.String()
			return m, nil
		}
		return m, nil
	}

	// Destructive schema DDL confirmation (drop column only).
	if m.schemaConfirmSQL != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			table := m.schemaConfirmTable
			query := m.schemaConfirmSQL
			action := m.schemaConfirmAction
			return m, m.execSchemaDDL(table, query, action, "")
		case "n", "N", "esc", "ctrl+c":
			m.clearSchemaConfirm()
			return m, nil
		}
		return m, nil
	}

	// Add-column form is modal.
	if m.addColumnForm.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.addColumnForm.Hide()
			m.clearSchemaConfirm()
			return m, nil
		case "enter", "ctrl+s":
			sql, errMsg := m.addColumnForm.Submit()
			if errMsg != "" {
				m.addColumnForm.SetError(errMsg)
				return m, nil
			}
			table := m.addColumnForm.Table()
			m.addColumnForm.SetError("")
			return m, m.execSchemaDDL(table, sql, db.SchemaAddColumn, "")
		}
		var cmd tea.Cmd
		m.addColumnForm, cmd = m.addColumnForm.Update(msg)
		return m, cmd
	}

	// Table rename form is modal.
	if m.tableRenameForm.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.tableRenameForm.Hide()
			m.clearSchemaConfirm()
			return m, nil
		case "enter", "ctrl+s":
			sql, errMsg := m.tableRenameForm.Submit()
			if errMsg != "" {
				m.tableRenameForm.SetError(errMsg)
				return m, nil
			}
			oldTable := m.tableRenameForm.Table()
			newTable := m.tableRenameForm.NewName()
			m.tableRenameForm.SetError("")
			return m, m.execSchemaDDL(oldTable, sql, db.SchemaRenameTable, newTable)
		}
		var cmd tea.Cmd
		m.tableRenameForm, cmd = m.tableRenameForm.Update(msg)
		return m, cmd
	}

	// Table designer takes over the workspace.
	if m.tableDesigner.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.tableDesigner.IsEditing() {
				m.tableDesigner, _ = m.tableDesigner.Update(msg)
				return m, nil
			}
			m.tableDesigner.Hide()
			m.clearSchemaConfirm()
			return m, nil
		case "enter", "ctrl+s":
			if m.tableDesigner.IsEditing() {
				// From edit mode, enter commits the cell; submit is
				// triggered only when not editing.
				break
			}
			sql, errMsg := m.tableDesigner.Submit()
			if errMsg != "" {
				m.tableDesigner.SetError(errMsg)
				return m, nil
			}
			table := m.tableDesigner.TableName()
			m.tableDesigner.SetError("")
			return m, m.execSchemaDDL(table, sql, db.SchemaCreateTable, "")
		}
		var cmd tea.Cmd
		m.tableDesigner, cmd = m.tableDesigner.Update(msg)
		return m, cmd
	}

	// Schema editor takes over the workspace.
	if m.schemaEditor.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.schemaEditor.IsEditing() {
				m.schemaEditor, _ = m.schemaEditor.Update(msg)
				return m, nil
			}
			m.schemaEditor.Hide()
			return m, nil
		case "enter":
			if m.schemaEditor.IsEditing() {
				m.schemaEditor, _ = m.schemaEditor.Update(msg)
				// For existing rows, fire per-cell DDL immediately on commit.
				// For new rows, the user fills in cells one by one and
				// presses enter again (not editing) to submit ADD COLUMN.
				if !m.schemaEditor.IsNewRow() {
					sql, action, errMsg := m.schemaEditor.PendingEditDDL()
					if errMsg != "" {
						m.schemaEditor.SetError(errMsg)
						return m, nil
					}
					if sql != "" {
						m.schemaEditor.SetError("")
						return m, m.execSchemaDDL(m.schemaEditor.Table(), sql, action, "")
					}
				}
				return m, nil
			}
			// Not editing: fire pending DDL (ADD COLUMN for new rows).
			sql, action, errMsg := m.schemaEditor.PendingEditDDL()
			if errMsg != "" {
				m.schemaEditor.SetError(errMsg)
				return m, nil
			}
			if sql == "" {
				return m, nil
			}
			m.schemaEditor.SetError("")
			return m, m.execSchemaDDL(m.schemaEditor.Table(), sql, action, "")
		case "d":
			// dd to drop column — only existing rows go through the confirm
			// flow. New rows are removed locally by the editor's Update.
			if m.schemaEditor.pendingD && !m.schemaEditor.IsNewRow() {
				m.schemaEditor.pendingD = false
				return m, m.dropCurrentColumn()
			}
		}
		var cmd tea.Cmd
		m.schemaEditor, cmd = m.schemaEditor.Update(msg)
		return m, cmd
	}

	// Discard / truncate / delete-rows confirmation dialogs are modal — intercept all keys.
	if m.discardConfirm || m.deleteRowsConfirmTable != "" || m.clearHistoryConfirm || m.clearBookmarksConfirm {
		switch msg.String() {
		case "y", "Y", "enter":
			if m.discardConfirm {
				m.results.DiscardEdits()
				m.discardConfirm = false
				return m, nil
			}
			if m.deleteRowsConfirmTable != "" {
				table := m.deleteRowsConfirmTable
				query := m.deleteRowsConfirmQuery
				count := m.deleteRowsConfirmCount
				m.deleteRowsConfirmTable = ""
				m.deleteRowsConfirmQuery = ""
				m.deleteRowsConfirmCount = 0
				return m, m.execDeleteRows(table, query, count)
			}
			if m.clearHistoryConfirm {
				m.clearHistoryConfirm = false
				if m.connection != nil && m.historyStore != nil {
					m.historyStore.Clear(m.connection.Config().Name)
				}
				m.history.SetEntries(nil)
				m.history.Toggle()
				return m, nil
			}
			if m.clearBookmarksConfirm {
				m.clearBookmarksConfirm = false
				if m.connection != nil && m.bookmarkStore != nil {
					m.bookmarkStore.Clear(m.connection.Config().Name)
				}
				m.bookmarks.SetEntries(nil)
				m.bookmarks.Toggle()
				return m, nil
			}
		case "n", "N", "esc", "ctrl+c":
			m.discardConfirm = false
			m.deleteRowsConfirmTable = ""
			m.deleteRowsConfirmQuery = ""
			m.deleteRowsConfirmCount = 0
			m.clearHistoryConfirm = false
			m.clearBookmarksConfirm = false
			return m, nil
		}
		return m, nil
	}

	// Truncate confirmation is modal — uses enter/esc (not y/n).
	if m.truncateConfirm != "" {
		switch msg.String() {
		case "enter":
			table := m.truncateConfirm
			m.truncateConfirm = ""
			return m, m.execTruncate(table)
		case "esc", "ctrl+c":
			m.truncateConfirm = ""
			return m, nil
		}
		return m, nil
	}

	// Cell-edit popup is modal — ctrl+s stages the value into dirtyCells (same
	// as pressing enter on the inline editor) and closes the popup; esc cancels.
	// All other keys go to the textarea.
	if m.cellEdit.IsVisible() {
		switch msg.String() {
		case "ctrl+s":
			m.results.SetDirtyCell(m.cellEdit.Row(), m.cellEdit.Col(), m.cellEdit.Value())
			m.cellEdit.Hide()
			return m, nil
		case "esc", "ctrl+c":
			m.cellEdit.Hide()
			return m, nil
		}
		var cmd tea.Cmd
		m.cellEdit, cmd = m.cellEdit.Update(msg)
		return m, cmd
	}

	// Command palette is modal — intercept all keys when visible.
	if m.palette.visible {
		var cmd tea.Cmd
		m.palette, cmd = m.palette.Update(msg)
		return m, cmd
	}

	// Global workspace keys
	switch msg.String() {
	case "?":
		m.help.Show()
		return m, nil
	case "ctrl+p":
		// Command palette — but not while the editor is in insert mode,
		// where ctrl+p navigates the completion popup.
		if m.focus == FocusEditor && m.editor.VimMode() == VimInsert {
			break
		}
		m.palette.Open()
		return m, nil
	case "q":
		// Quit — but only when no text-input / editing context is active,
		// otherwise 'q' must remain available for typing.
		if m.results.IsEditing() ||
			m.inspector.IsEditing() || m.inspector.IsInserting() || m.inspector.IsFiltering() ||
			m.sidebarFiltering ||
			(m.focus == FocusEditor && m.editor.VimMode() == VimInsert) {
			break
		}
		m.quitting = true
		return m, tea.Quit
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
	case "ctrl+e":
		return m, m.executeQuery()
	case "\\":
		if m.focus == FocusEditor && m.editor.VimMode() == VimNormal && !m.editor.CompletionVisible() {
			return m, m.executeQuery()
		}
	case "ctrl+d":
		// Vim-style page navigation — only when not in the editor
		// (Ctrl+D/U scroll within the editor in vim normal mode).
		// Also block when editing a cell or have unsaved edits.
		if m.focus != FocusEditor {
			if m.results.IsEditing() || m.results.HasDirtyCells() || m.inspector.IsInserting() {
				return m, nil
			}
			return m, m.nextPage()
		}
	case "ctrl+u":
		if m.focus != FocusEditor {
			if m.results.IsEditing() || m.results.HasDirtyCells() || m.inspector.IsInserting() {
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
	case "ctrl+g":
		// Toggle bookmarks panel.
		if m.connection != nil {
			if m.bookmarks.IsVisible() {
				m.bookmarks.Toggle()
			} else {
				entries, err := m.bookmarkStore.Get(m.connection.Config().Name)
				if err == nil {
					m.bookmarks.SetEntries(entries)
				}
				m.bookmarks.Toggle()
			}
		}
		return m, nil
	case "B":
		// Bookmark the current editor query.
		if m.focus == FocusEditor && m.editor.VimMode() == VimInsert {
			break
		}
		q := m.editor.FormatQuery()
		if q != "" && m.connection != nil && m.bookmarkStore != nil {
			if err := m.bookmarkStore.Add(m.connection.Config().Name, q); err == nil {
				m.bookmarkMsg = "bookmarked"
			} else {
				m.bookmarkMsg = "already bookmarked"
			}
		}
		return m, nil
	case "ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l":
		// Directional panel navigation — not while editing or in insert mode.
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
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
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
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
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
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
		m.results.SetSearchMatcher(nil)
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
		m.sidebarFiltering = false
		m.sidebarFilter = ""
		m.editor.CancelCompletion()
		m.loadConnections()
		if len(m.config.Connections) > 0 {
			m.connList.StartFilter()
		}
		return m, nil
	case "esc":
		// Close bookmarks panel if visible.
		if m.bookmarks.IsVisible() {
			m.bookmarks.Toggle()
			return m, nil
		}
		// Close history panel if visible.
		if m.history.IsVisible() {
			m.history.Toggle()
			return m, nil
		}
		// If in visual mode, let the focused panel handle esc.
		if m.results.IsVisualMode() {
			break
		}
		// If in search mode, let the focused panel handle esc.
		if m.searching {
			break
		}
		// Clear committed search highlighting (vim :nohl).
		if m.lastSearch != "" {
			m.lastSearch = ""
			m.results.SetSearchMatcher(nil)
			m.searchMsg = ""
			return m, nil
		}
		// If actively editing a cell or inspector field, let the focused
		// panel handle esc (to cancel the edit) instead of swallowing it.
		if m.results.IsEditing() || m.inspector.IsEditing() {
			break
		}
		// Exit sidebar fuzzy filter mode.
		if m.focus == FocusConnections && m.sidebarFiltering {
			m.sidebarFiltering = false
			m.sidebarFilter = ""
			m.sidebarCursor = 0
			return m, nil
		}
		// Exit inspector filter mode.
		if m.focus == FocusInspector && m.inspector.IsFiltering() {
			m.inspector.CancelFilter()
			m.inspector.cursorField = 0
			return m, nil
		}
		// Cancel new-record mode.
		if m.inspector.IsInserting() && !m.inspector.IsEditing() {
			m.inspector.CancelInsert()
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
		case "D":
			m.clearHistoryConfirm = true
			return m, nil
		case "b":
			// Promote the selected history entry to bookmarks.
			q := m.history.SelectedQuery()
			if q != "" && m.connection != nil && m.bookmarkStore != nil {
				m.bookmarkStore.Add(m.connection.Config().Name, q)
			}
			m.history.Toggle()
			return m, nil
		}
	}

	// Bookmarks panel takes over navigation when visible
	if m.bookmarks.IsVisible() {
		switch msg.String() {
		case "j", "down":
			m.bookmarks.CursorDown()
			return m, nil
		case "k", "up":
			m.bookmarks.CursorUp()
			return m, nil
		case "enter":
			q := m.bookmarks.SelectedQuery()
			if q != "" {
				m.editor.SetValue(q)
				m.focus = FocusEditor
				m.applyFocus()
			}
			m.bookmarks.Toggle()
			return m, m.editor.Focus()
		case "esc":
			m.bookmarks.Toggle()
			return m, nil
		case "d":
			// Delete the bookmark under the cursor.
			if m.connection != nil && m.bookmarkStore != nil && m.bookmarks.CursorIndex() >= 0 {
				idx := len(m.bookmarks.entries) - 1 - m.bookmarks.CursorIndex()
				m.bookmarkStore.RemoveAt(m.connection.Config().Name, idx)
				entries, _ := m.bookmarkStore.Get(m.connection.Config().Name)
				m.bookmarks.SetEntries(entries)
			}
			return m, nil
		case "D":
			m.clearBookmarksConfirm = true
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
		// Clear dd pending state on any non-'d' key.
		if msg.String() != "d" {
			m.resultsPendingD = false
		}
		// Column jump mode intercepts all keys.
		if m.columnJumping {
			switch msg.String() {
			case "esc":
				m.columnJumping = false
				m.columnJump = ""
				return m, nil
			case "enter":
				if idx := bestColumnMatch(m.results.columns, m.columnJump); idx >= 0 {
					m.results.SetCursor(m.results.CursorRow(), idx)
				}
				m.columnJumping = false
				m.columnJump = ""
				return m, nil
			case "backspace":
				if len(m.columnJump) > 0 {
					m.columnJump = m.columnJump[:len(m.columnJump)-1]
				}
				return m, nil
			case "ctrl+c":
				m.columnJumping = false
				m.columnJump = ""
				return m, nil
			}
			if msg.Type == tea.KeyRunes {
				m.columnJump += msg.String()
				return m, nil
			}
			return m, nil
		}
		// Search mode (g/) intercepts all keys.
		if m.searching {
			updateSearchMatcher := func() {
				if m.searchQuery == "" {
					m.results.SetSearchMatcher(nil)
				} else {
					m.results.SetSearchMatcher(compileSearchPattern(m.searchQuery))
				}
			}
			switch msg.String() {
			case "esc":
				m.searching = false
				m.searchQuery = ""
				m.results.SetSearchMatcher(nil)
				return m, nil
			case "enter":
				m.lastSearch = m.searchQuery
				m.searching = false
				query := m.searchQuery
				m.searchQuery = ""
				if query != "" {
					match := compileSearchPattern(query)
					m.results.SetSearchMatcher(match)
					if row, col := findNextMatch(m.results, match, true); row >= 0 {
						m.results.SetCursor(row, col)
						n := countMatches(m.results, match)
						m.searchMsg = fmt.Sprintf("%d match%s (this page)", n, pluralIf(n != 1, "es"))
					} else {
						m.searchMsg = "no matches on this page"
					}
				}
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					updateSearchMatcher()
				}
				return m, nil
			case "ctrl+c":
				m.searching = false
				m.searchQuery = ""
				m.results.SetSearchMatcher(nil)
				return m, nil
			}
			if msg.Type == tea.KeyRunes {
				m.searchQuery += msg.String()
				updateSearchMatcher()
				return m, nil
			}
			return m, nil
		}
		// Visual mode intercepts movement and commit/cancel keys.
		if m.results.IsVisualMode() {
			switch msg.String() {
			case "esc", "V":
				m.results.ClearVisualMode()
				return m, nil
			case "enter":
				m.commitVisualMarks()
				m.results.ClearVisualMode()
				return m, nil
			case "j", "down":
				m.results.CursorDown()
				return m, nil
			case "k", "up":
				m.results.CursorUp()
				return m, nil
			case "g":
				m.results.CursorTop()
				return m, nil
			case "G":
				m.results.CursorBottom()
				return m, nil
			case "ctrl+c":
				m.results.ClearVisualMode()
				return m, nil
			}
			return m, nil
		}
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

		// Insert row must work on empty editable tables too.
		if msg.String() == "A" {
			if m.results.IsEditable() && !m.results.HasDirtyCells() && !m.inspector.IsInserting() {
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.startInsert()
				return m, nil
			}
		}

		// Cell cursor navigation.
		if m.results.NumRows() > 0 {
			switch msg.String() {
			case "up", "k":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorUp()
				return m, nil
			case "down", "j":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorDown()
				return m, nil
			case "left", "h":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorLeft()
				return m, nil
			case "b":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					return m, m.goBackQuery()
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorLeft()
				return m, nil
			case "d":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					m.resultsPendingD = false
					return m, m.followForeignKey()
				}
				if !m.results.IsEditable() || m.results.NumRows() == 0 {
					m.resultsPendingD = false
					return m, nil
				}
				if m.resultsPendingD {
					m.resultsPendingD = false
					m.startDeleteRows()
					return m, nil
				}
				m.resultsPendingD = true
				return m, nil
			case "right", "l", "w":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorRight()
				return m, nil
			case "G":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorBottom()
				return m, nil
			case "g":
				m.resultsPendingY = false
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.results.CursorTop()
					return m, nil
				}
				m.resultsPendingG = true
				return m, nil
			case "y":
				m.resultsPendingG = false
				if m.resultsPendingY {
					m.resultsPendingY = false
					_ = clipboard.WriteAll(m.results.CursorCellValue())
					m.results.StartCopyFeedback()
					return m, copyFeedbackCmd()
				}
				m.resultsPendingY = true
				return m, nil
			case "s":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					return m, m.fetchColumnStats()
				}
			case "e", "i":
				if !m.results.IsEditable() || !m.results.HasPrimaryKey() {
					break
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.inspector.IsVisible() {
					return m, nil
				}
				if m.results.IsCellTruncated(m.results.CursorRow(), m.results.CursorCol()) {
					return m, m.openCellEditPopup(m.results.CursorRow(), m.results.CursorCol())
				}
				m.results.StartEdit()
				return m, nil
			case "E":
				if !m.results.IsEditable() || !m.results.HasPrimaryKey() || m.inspector.IsVisible() {
					break
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.openCellEditPopup(m.results.CursorRow(), m.results.CursorCol())
			case "n":
				if m.lastSearch != "" {
					m.resultsPendingG = false
					m.resultsPendingY = false
					match := compileSearchPattern(m.lastSearch)
					if row, col := findNextMatch(m.results, match, false); row >= 0 {
						m.results.SetCursor(row, col)
					}
					return m, nil
				}
			case "N":
				if m.lastSearch != "" {
					m.resultsPendingG = false
					m.resultsPendingY = false
					match := compileSearchPattern(m.lastSearch)
					if row, col := findPrevMatch(m.results, match); row >= 0 {
						m.results.SetCursor(row, col)
					}
					return m, nil
				}
			case "ctrl+s":
				if !m.results.IsEditable() && !m.inspector.IsInserting() {
					break
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.saveChanges()
			case "D":
				if !m.results.IsEditable() {
					break
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.HasDirtyCells() {
					m.discardConfirm = true
				}
				return m, nil
			case "/":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					if m.results.NumRows() > 0 {
						m.searching = true
						m.searchQuery = ""
					}
					return m, nil
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.canFilter() {
					return m, m.openFilterPicker()
				}
			case "c":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if len(m.filters) > 0 {
					return m, m.clearFilters()
				}
			case "C":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.MarkCount() > 0 {
					m.results.ClearMarks()
				}
				return m, nil
			case "u":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if len(m.filters) > 0 {
					return m, m.undoFilter()
				}
			case "*":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.quickFilterCell(false)
			case "!":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.quickFilterCell(true)
			case " ":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.IsEditable() && m.results.NumRows() > 0 {
					m.results.ToggleMark()
				}
				return m, nil
			case "F":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.filterByMarks()
			case "o":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.canFilter() {
					return m, m.toggleSort()
				}
			case "x":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.exportToCSV()
			case ":":
				if m.results.NumCols() > 0 {
					m.columnJumping = true
					m.columnJump = ""
				}
				return m, nil
			case "H":
				if m.resultsPendingG {
					// g H — show all columns.
					m.resultsPendingG = false
					m.resultsPendingY = false
					m.results.ShowAllColumns()
					return m, nil
				}
				// H — hide the column under the cursor.
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.NumCols() > 0 {
					m.results.HideColumn(m.results.CursorCol())
				}
				return m, nil
			case "v":
				// v — open the column-visibility overlay.
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resultsPendingD = false
				if m.results.NumCols() > 0 {
					m.openColumnPicker()
					return m, nil
				}
			case "V":
				// V — enter line-wise visual mode for bulk row marking.
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resultsPendingD = false
				if m.results.IsEditable() && m.results.NumRows() > 0 {
					m.results.SetVisualMode()
				}
				return m, nil
			}
			m.resultsPendingG = false
			m.resultsPendingY = false
			m.results, cmd = m.results.Update(msg)
			return m, cmd
		}

		m.resultsPendingG = false
		m.resultsPendingY = false
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
				// Select the highlighted match in the full sidebar list.
				if item := m.currentSidebarItem(); item != nil && !item.isColumn {
					selected := item.text
					m.sidebarFiltering = false
					m.sidebarFilter = ""
					m.syncSidebarCursorToTable(selected)
					return m, nil
				}
				m.sidebarFiltering = false
				m.sidebarFilter = ""
				m.sidebarCursor = 0
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
		case " ":
			m.sidebarPendingG = false
			m.toggleExpand()
			return m, nil
		case "/":
			m.sidebarFiltering = true
			m.sidebarFilter = ""
			m.sidebarCursor = 0
			return m, nil
		case "enter", "s":
			item := m.currentSidebarItem()
			if item != nil && !item.isColumn {
				m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s;", item.text))
				m.focus = FocusEditor
				m.applyFocus()
				return m, tea.Batch(m.editor.Focus(), m.executeQuery())
			}
		case "d":
			m.sidebarPendingG = false
			if m.sidebarSelectedTable() != "" {
				m.openSchemaPanel()
			}
			return m, nil
		case "T":
			m.sidebarPendingG = false
			item := m.currentSidebarItem()
			if item != nil && !item.isColumn {
				m.truncateConfirm = item.text
			}
		case "D":
			m.sidebarPendingG = false
			item := m.currentSidebarItem()
			if item != nil && !item.isColumn {
				m.dropTableConfirm = item.text
				m.dropTableInput = ""
			}
		case "r":
			m.sidebarPendingG = false
			item := m.currentSidebarItem()
			if item != nil && !item.isColumn {
				return m, m.openTableRenameForm(item.text)
			}
		case "a":
			m.sidebarPendingG = false
			if m.sidebarSelectedTable() != "" {
				return m, m.openAddColumnForm()
			}
		case "N":
			m.sidebarPendingG = false
			return m, m.openCreateTableForm()
		case "X":
			m.sidebarPendingG = false
			m.exportPicker.Show(m.tables, m.currentTable())
			return m, nil
		case "I":
			m.sidebarPendingG = false
			m.importPrompt.Show("~/Downloads/")
			return m, nil
		}
		m.connList, cmd = m.connList.Update(msg)
	case FocusInspector:
		if m.inspector.IsEditing() {
			switch msg.String() {
			case "enter":
				col, val, ok := m.inspector.CommitFieldEdit()
				if ok && !m.inspector.IsInserting() {
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
		if m.inspector.IsFiltering() {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.inspector.CancelFilter()
				m.inspector.cursorField = 0
				return m, nil
			case "enter":
				m.inspector.CommitFilter(m.results)
				return m, nil
			case "backspace":
				m.inspector.FilterBackspace()
				return m, nil
			case "up", "k":
				m.inspector.CursorUp()
				return m, nil
			case "down", "j":
				m.inspector.CursorDown(m.results)
				return m, nil
			}
			if msg.Type == tea.KeyRunes {
				m.inspector.FilterAddChar(msg.String())
				return m, nil
			}
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			m.inspector.pendingG = false
			m.inspector.CursorUp()
			return m, nil
		case "down", "j":
			m.inspector.pendingG = false
			m.inspector.CursorDown(m.results)
			return m, nil
		case "G":
			m.inspector.pendingG = false
			m.inspector.CursorBottom(m.results)
			return m, nil
		case "g":
			if m.inspector.pendingG {
				m.inspector.pendingG = false
				m.inspector.CursorTop()
				return m, nil
			}
			m.inspector.pendingG = true
			return m, nil
		case "/":
			m.inspector.StartFilter()
			return m, nil
		case "e", "i":
			m.inspector.pendingG = false
			col := m.inspector.selectedColumn(m.results)
			if !m.inspector.IsInserting() && m.inspector.IsFieldTruncated(m.results) {
				return m, m.openCellEditPopup(m.results.CursorRow(), col)
			}
			m.inspector.StartFieldEdit(m.results)
			return m, nil
		case "E":
			m.inspector.pendingG = false
			if !m.inspector.IsInserting() {
				col := m.inspector.selectedColumn(m.results)
				return m, m.openCellEditPopup(m.results.CursorRow(), col)
			}
		case "ctrl+s":
			m.inspector.pendingG = false
			if m.results.IsEditable() || m.inspector.IsInserting() {
				return m, m.saveChanges()
			}
			return m, nil
		case "A":
			m.inspector.pendingG = false
			if m.results.IsEditable() && !m.results.HasDirtyCells() && !m.inspector.IsInserting() {
				m.startInsert()
			}
			return m, nil
		case "esc":
			if m.inspector.IsInserting() {
				m.inspector.CancelInsert()
				return m, nil
			}
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

// bestColumnMatch returns the index of the column whose name best matches
// the fuzzy query, or -1 if nothing matches.
func bestColumnMatch(cols []string, query string) int {
	if query == "" {
		return -1
	}
	bestIdx := -1
	bestScore := 0
	for i, c := range cols {
		_, score := fuzzyMatch(query, c)
		if score == 0 {
			continue
		}
		if bestIdx == -1 || score < bestScore {
			bestIdx = i
			bestScore = score
		}
	}
	return bestIdx
}

// compileSearchPattern compiles a user-typed search string as a regex, falling
// back to a literal substring match if the regex is invalid. The returned
// matcher function reports whether a cell value contains a match.
func compileSearchPattern(query string) func(string) bool {
	if query == "" {
		return func(string) bool { return false }
	}
	if re, err := regexp.Compile(query); err == nil {
		return func(s string) bool { return re.MatchString(s) }
	}
	// Literal fallback: case-sensitive substring match.
	q := query
	return func(s string) bool { return strings.Contains(s, q) }
}

// findNextMatch scans row-major from the cell after the cursor (inclusive if
// fromStart) and returns the first matching [row, col], or [-1,-1] if none.
func findNextMatch(r ResultsTable, match func(string) bool, fromStart bool) (int, int) {
	rows := r.NumRows()
	cols := r.NumCols()
	if rows == 0 || cols == 0 {
		return -1, -1
	}
	startRow, startCol := 0, 0
	if !fromStart {
		startRow = r.CursorRow()
		startCol = r.CursorCol() + 1
	}
	for row := startRow; row < rows; row++ {
		for col := 0; col < cols; col++ {
			if row == startRow && col < startCol {
				continue
			}
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				return row, col
			}
		}
	}
	// Wrap around.
	for row := 0; row < startRow; row++ {
		for col := 0; col < cols; col++ {
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				return row, col
			}
		}
	}
	if !fromStart {
		// Final partial pass on the start row up to the cursor.
		for col := 0; col < startCol; col++ {
			if match(r.RowValue(startRow, col)) {
				return startRow, col
			}
		}
	}
	return -1, -1
}

// findPrevMatch scans row-major backwards from the cell before the cursor and
// returns the nearest matching [row, col], or [-1,-1] if none. Wraps around.
func findPrevMatch(r ResultsTable, match func(string) bool) (int, int) {
	rows := r.NumRows()
	cols := r.NumCols()
	if rows == 0 || cols == 0 {
		return -1, -1
	}
	startRow := r.CursorRow()
	startCol := r.CursorCol() - 1
	// Scan backwards from cursor.
	for row := startRow; row >= 0; row-- {
		cEnd := cols - 1
		if row == startRow {
			cEnd = startCol
		}
		for col := cEnd; col >= 0; col-- {
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				return row, col
			}
		}
	}
	// Wrap around: scan from the last row back to the cursor row.
	for row := rows - 1; row > startRow; row-- {
		for col := cols - 1; col >= 0; col-- {
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				return row, col
			}
		}
	}
	// Final partial pass on the start row from the last col down to cursor.
	for col := cols - 1; col >= startCol+1; col-- {
		if match(r.RowValue(startRow, col)) {
			return startRow, col
		}
	}
	return -1, -1
}

// countMatches returns the total number of cells matching across all rows.
func countMatches(r ResultsTable, match func(string) bool) int {
	count := 0
	for row := 0; row < r.NumRows(); row++ {
		for col := 0; col < r.NumCols(); col++ {
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				count++
			}
		}
	}
	return count
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

// syncSidebarCursorToTable moves the cursor to a table in the full sidebar list.
func (m *Model) syncSidebarCursorToTable(tableName string) {
	items := m.sidebarItems()
	for i, item := range items {
		if !item.isColumn && item.text == tableName {
			m.sidebarCursor = i
			return
		}
	}
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

// sidebarSelectedTable returns the table for the current sidebar cursor,
// whether it points at the table row or one of its expanded columns.
func (m Model) sidebarSelectedTable() string {
	items := m.sidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return ""
	}
	for i := m.sidebarCursor; i >= 0; i-- {
		if !items[i].isColumn {
			return items[i].text
		}
	}
	return ""
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

	resultsHeight := m.height - editorHeight - statusHeight - borderOverhead
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

	// Database picker: render on a blank background (like the connection picker).
	if m.dbPicker.IsVisible() {
		pw, ph := popupDim()
		m.dbPicker.SetSize(pw, ph)
		pickerPanel := m.dbPicker.View()
		panelW := lipgloss.Width(pickerPanel)
		panelH := lipgloss.Height(pickerPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - panelH) / 2
		bg := strings.Repeat("\n", m.height-1)
		return placeOverlay(bg, pickerPanel, panelX, panelY)
	}

	return m.viewWorkspace()
}

func (m Model) viewConnections() string {
	popupW, popupH := popupDim()
	borderOverhead := 2
	padH, padW := 0, 2 // Padding(0, 1) → 0 rows, 2 cols

	panelW := popupW - borderOverhead
	panelH := popupH - borderOverhead

	prompt := m.connList.Prompt()
	listH := panelH - 2 - padH // prompt + scroll info
	m.connList.SetSize(panelW-padW, listH)

	// Pin the list to a fixed height so ScrollInfo sits at the bottom.
	listStyled := lipgloss.NewStyle().
		Height(listH).
		Render(m.connList.View())

	connPanel := lipgloss.NewStyle().
		Width(panelW).
		Height(panelH).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(
			lipgloss.JoinVertical(lipgloss.Left,
				prompt,
				listStyled,
				m.connList.ScrollInfo(),
			),
		)

	// Render as an overlay on a blank background.
	panelW2 := lipgloss.Width(connPanel)
	panelH2 := lipgloss.Height(connPanel)
	panelX := (m.width - panelW2) / 2
	panelY := (m.height - panelH2) / 2
	bg := strings.Repeat("\n", m.height-1)
	view := placeOverlay(bg, connPanel, panelX, panelY)

	// Overlay help panel if visible
	if m.help.IsVisible() {
		m.help.SetSize(m.width, m.height)
		view = m.help.View()
	}
	return view
}

func (m Model) viewAddConnection() string {
	popupW, popupH := popupDim()
	borderOverhead := 2
	padding := 2 // padding(0,1) = 1 left + 1 right
	innerW := popupW - borderOverhead - padding
	m.connForm.SetMaxWidth(innerW)

	formPanel := lipgloss.NewStyle().
		Width(popupW - borderOverhead).
		Height(popupH - borderOverhead).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
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
	resultsHeight := m.height - editorHeight - statusHeight - borderOverhead
	if resultsHeight < 3 {
		resultsHeight = 3
	}

	rightWidth := m.width - sidebarWidth - borderOverhead
	if m.inspector.IsVisible() {
		rightWidth -= inspectorWidth
	}

	// Build right column. When the table designer or schema editor is active,
	// it takes over the full editor+results space as a single inline grid.
	var rightPanel string
	if m.tableDesigner.IsVisible() {
		designerHeight := editorHeight + resultsHeight
		m.tableDesigner.SetSize(rightWidth-borderOverhead, designerHeight-borderOverhead)
		rightPanel = lipgloss.NewStyle().
			Width(rightWidth).
			Height(designerHeight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Render(m.tableDesigner.View())
	} else if m.schemaEditor.IsVisible() {
		editorH := editorHeight + resultsHeight
		m.schemaEditor.SetSize(rightWidth-borderOverhead, editorH-borderOverhead)
		rightPanel = lipgloss.NewStyle().
			Width(rightWidth).
			Height(editorH).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Render(m.schemaEditor.View())
	} else {
		editorPanel := lipgloss.NewStyle().
			Width(rightWidth).
			Height(editorHeight - borderOverhead).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.borderForFocus(FocusEditor)).
			Render(m.editor.View())

		resultsPanel := lipgloss.NewStyle().
			Width(rightWidth).
			Height(resultsHeight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.borderForFocus(FocusResults)).
			Render(func() string {
				m.results.SetSort(m.sortCol, m.sortDir)
				view := m.results.View()
				if m.columnJumping {
					prompt := lipgloss.NewStyle().Foreground(colorPrimary).Render(":"+m.columnJump) +
						lipgloss.NewStyle().Foreground(colorAccent).Render("▏")
					view = prompt + "\n" + view
				}
				if m.searching {
					prompt := lipgloss.NewStyle().Foreground(colorPrimary).Render("/"+m.searchQuery) +
						lipgloss.NewStyle().Foreground(colorAccent).Render("▏")
					view = prompt + "\n" + view
				}
				return view
			}())

		rightPanel = lipgloss.JoinVertical(lipgloss.Left,
			editorPanel,
			resultsPanel,
		)
	}

	// Build inspector panel if visible.
	var inspectorPanel string
	if m.inspector.IsVisible() {
		inspectorContentHeight := lipgloss.Height(rightPanel) - borderOverhead
		if inspectorContentHeight < 3 {
			inspectorContentHeight = 3
		}
		m.inspector.SetSize(inspectorWidth-borderOverhead, inspectorContentHeight)
		inspectorPanel = lipgloss.NewStyle().
			Width(inspectorWidth - borderOverhead).
			Height(inspectorContentHeight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.borderForFocus(FocusInspector)).
			Render(m.inspector.View(m.results))
	}

	// Sidebar content height = right panel height minus sidebar's own borders.
	sidebarContentHeight := lipgloss.Height(rightPanel) - borderOverhead
	if sidebarContentHeight < 3 {
		sidebarContentHeight = 3
	}

	// Reserve 1 line for bottom bar (search/scroll info).
	tableAreaHeight := sidebarContentHeight - 1
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
		scrollInfo = renderPalettePrompt(m.sidebarFilter, true)
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
			lipgloss.JoinVertical(lipgloss.Left, tableListStyled, scrollInfo),
		)

	connName := ""
	if m.connection != nil {
		connName = m.connection.Config().Name
	}

	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(statusHeight).
		Foreground(colorMuted).
		Render(" " + m.statusBar(connName))

	var workspace string
	if m.inspector.IsVisible() {
		workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel, inspectorPanel)
	} else {
		workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, workspace, statusBar)

	// Overlay history panel if visible
	if m.history.IsVisible() {
		m.history.SetSize(m.width/2, m.height/2)
		histPanel := m.history.View()
		panelW := lipgloss.Width(histPanel)
		panelH := lipgloss.Height(histPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, histPanel, panelX, panelY)
	}

	// Overlay bookmarks panel if visible
	if m.bookmarks.IsVisible() {
		m.bookmarks.SetSize(m.width/2, m.height/2)
		bmPanel := m.bookmarks.View()
		panelW := lipgloss.Width(bmPanel)
		panelH := lipgloss.Height(bmPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, bmPanel, panelX, panelY)
	}

	// Overlay filter picker if visible
	if m.filterPicker.IsVisible() {
		pw, ph := popupDim()
		m.filterPicker.SetSize(pw, ph)
		filterPanel := m.filterPicker.View()
		panelW := lipgloss.Width(filterPanel)
		panelH := lipgloss.Height(filterPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, filterPanel, panelX, panelY)
	}

	// Overlay column-visibility picker if visible
	if m.columnPicker.IsVisible() {
		pw, ph := popupDim()
		m.columnPicker.SetSize(pw, ph)
		colPanel := m.columnPicker.View()
		panelW := lipgloss.Width(colPanel)
		panelH := lipgloss.Height(colPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, colPanel, panelX, panelY)
	}

	// Overlay discard confirmation dialog if visible
	if m.discardConfirm {
		dialog := renderConfirmDialog("Discard all unsaved changes?")
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay truncate confirmation dialog if visible
	if m.truncateConfirm != "" {
		prompt := fmt.Sprintf("Truncate table %s?\nAll rows will be permanently deleted.", m.truncateConfirm)
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay drop-table typed confirmation dialog if visible.
	if m.dropTableConfirm != "" {
		prompt := fmt.Sprintf("Drop table %s?\nThis permanently deletes the table, data, and indexes.", m.dropTableConfirm)
		dialog := renderTypedConfirmDialog(prompt, m.dropTableConfirm, m.dropTableInput, 52, 0)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay drop-database typed confirmation dialog if visible. Sized to
	// match the database picker so it replaces it cleanly, not a smaller box
	// floating on top.
	if m.dropDBConfirm != "" {
		prompt := fmt.Sprintf("Drop database %s?\nThis permanently deletes every table and all data in the database.", m.dropDBConfirm)
		pw, ph := popupDim()
		dialog := renderTypedConfirmDialog(prompt, m.dropDBConfirm, m.dropDBInput, pw, ph)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay create-database name input dialog if visible.
	if m.createDBActive {
		pw, ph := popupDim()
		dialog := renderInputDialog("Create new database", m.createDBInput, m.createDBErr, pw, ph)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay row deletion confirmation dialog if visible.
	if m.deleteRowsConfirmTable != "" {
		prompt := fmt.Sprintf("Delete %d row%s from %s?\nThis cannot be undone.", m.deleteRowsConfirmCount, pluralIf(m.deleteRowsConfirmCount != 1, "s"), m.deleteRowsConfirmTable)
		dialog := renderConfirmDialog(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	if m.schemaConfirmSQL != "" {
		prompt := fmt.Sprintf("Drop column on %s?\nThis permanently removes the column and its data.", m.schemaConfirmTable)
		dialog := renderSQLConfirmDialog(prompt, m.schemaConfirmSQL)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay clear-history confirmation dialog if visible.
	if m.clearHistoryConfirm {
		dialog := renderConfirmDialog("Clear all query history?\nThis cannot be undone.")
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay clear-bookmarks confirmation dialog if visible.
	if m.clearBookmarksConfirm {
		dialog := renderConfirmDialog("Clear all bookmarks?\nThis cannot be undone.")
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay add-column form.
	if m.addColumnForm.IsVisible() {
		popupW := 58
		borderOverhead := 2
		padding := 4
		innerW := popupW - borderOverhead - padding
		m.addColumnForm.SetMaxWidth(innerW)
		formPanel := lipgloss.NewStyle().
			Width(popupW - borderOverhead).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Render(m.addColumnForm.View())
		panelW := lipgloss.Width(formPanel)
		panelH := lipgloss.Height(formPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, formPanel, panelX, panelY)
	}

	// Overlay table rename form.
	if m.tableRenameForm.IsVisible() {
		popupW := 58
		borderOverhead := 2
		padding := 4
		innerW := popupW - borderOverhead - padding
		m.tableRenameForm.SetMaxWidth(innerW)
		content := m.tableRenameForm.View()
		formPanel := lipgloss.NewStyle().
			Width(popupW - borderOverhead).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Render(content)
		panelW := lipgloss.Width(formPanel)
		panelH := lipgloss.Height(formPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, formPanel, panelX, panelY)
	}

	// Overlay cell-edit popup (expanded editor for truncated cells).
	if m.cellEdit.IsVisible() {
		// Size the popup to ~70% of the screen.
		availW := m.width * 65 / 100
		availH := (m.height - 1) * 65 / 100
		m.cellEdit.SetMaxSize(availW, availH)
		panel := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Render(m.cellEdit.View())
		panelW := lipgloss.Width(panel)
		panelH := lipgloss.Height(panel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, panel, panelX, panelY)
	}

	// Overlay completion popup if visible
	if m.editor.CompletionVisible() {
		cursorLine, cursorCol := m.editor.CursorScreenPos()
		popup := m.editor.CompletionView()
		popupX := sidebarWidth + 2 + cursorCol
		popupY := 2 + cursorLine + 1
		view = placeOverlay(view, popup, popupX, popupY)
	}

	// Overlay export picker if visible
	if m.exportPicker.IsVisible() {
		pw, ph := popupDim()
		m.exportPicker.SetSize(pw, ph)
		exportPanel := m.exportPicker.View()
		panelW := lipgloss.Width(exportPanel)
		panelH := lipgloss.Height(exportPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, exportPanel, panelX, panelY)
	}

	// Overlay import prompt if visible
	if m.importPrompt.IsVisible() {
		importPanel := m.importPrompt.View()
		panelW := lipgloss.Width(importPanel)
		panelH := lipgloss.Height(importPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, importPanel, panelX, panelY)
	}

	// Overlay command palette if visible
	if m.palette.IsVisible() {
		pw, ph := popupDim()
		palPanel := m.palette.View(pw, ph)
		panelW := lipgloss.Width(palPanel)
		panelH := lipgloss.Height(palPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, palPanel, panelX, panelY)
	}

	// Overlay help panel if visible
	if m.help.IsVisible() {
		m.help.SetSize(m.width, m.height-1)
		view = m.help.View()
	}

	return view
}

// popupDim returns the fixed popup dimensions matching the connection form.
func popupDim() (w, h int) {
	return 71, 19
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

// currentTable returns the table the user is currently working with, if known.
// It prefers the editable results source, then the focused sidebar selection.
func (m Model) currentTable() string {
	if t := m.results.SourceTable(); t != "" {
		return t
	}
	if m.focus == FocusConnections && !m.sidebarFiltering {
		if item := m.currentSidebarItem(); item != nil && !item.isColumn {
			return item.text
		}
	}
	return ""
}

// flashSnapshot captures all transient status-bar fields as an opaque value
// so the Update wrapper can detect whether a message changed them.
func (m Model) flashSnapshot() [7]string {
	return [7]string{
		m.statsMsg, m.exportMsg, m.searchMsg,
		m.truncateMsg, m.deleteRowsMsg, m.schemaMsg,
		m.bookmarkMsg,
	}
}

// flashChanged reports whether any transient field differs from a prior
// snapshot taken by flashSnapshot.
func (m Model) flashChanged(prev [7]string) bool {
	return m.statsMsg != prev[0] ||
		m.exportMsg != prev[1] ||
		m.searchMsg != prev[2] ||
		m.truncateMsg != prev[3] ||
		m.deleteRowsMsg != prev[4] ||
		m.schemaMsg != prev[5] ||
		m.bookmarkMsg != prev[6]
}

// anyFlashActive reports whether any transient status-bar field is non-empty.
func (m Model) anyFlashActive() bool {
	return m.statsMsg != "" ||
		m.exportMsg != "" ||
		m.searchMsg != "" ||
		m.truncateMsg != "" ||
		m.deleteRowsMsg != "" ||
		m.schemaMsg != "" ||
		m.bookmarkMsg != ""
}

// clearFlash empties every transient status-bar field.
func (m *Model) clearFlash() {
	m.statsMsg = ""
	m.exportMsg = ""
	m.searchMsg = ""
	m.truncateMsg = ""
	m.deleteRowsMsg = ""
	m.schemaMsg = ""
	m.bookmarkMsg = ""
}

// statusMessage returns the most relevant transient message for the status bar
// (copy confirmation, save state, errors, pagination), or "" if none.
func (m Model) statusMessage() string {
	switch {
	case m.results.SaveError() != "":
		return errorStyle.Render(m.results.SaveError())
	case m.results.IsEditing():
		return mutedStyle.Render("editing")
	case m.inspector.IsInserting():
		return mutedStyle.Render("inserting")
	case m.results.IsCopied():
		return successStyle.Render("copied to clipboard")
	case m.results.IsSaved():
		return successStyle.Render("saved")
	case m.exportMsg != "":
		return successStyle.Render(m.exportMsg)
	case m.truncateMsg != "":
		if strings.HasPrefix(m.truncateMsg, "truncate failed:") {
			return errorStyle.Render(m.truncateMsg)
		}
		return successStyle.Render(m.truncateMsg)
	case m.deleteRowsMsg != "":
		if strings.HasPrefix(m.deleteRowsMsg, "delete failed:") {
			return errorStyle.Render(m.deleteRowsMsg)
		}
		return successStyle.Render(m.deleteRowsMsg)
	case m.schemaMsg != "":
		if strings.HasPrefix(m.schemaMsg, "schema change failed:") {
			return errorStyle.Render(m.schemaMsg)
		}
		return successStyle.Render(m.schemaMsg)
	case m.bookmarkMsg != "":
		return successStyle.Render(m.bookmarkMsg)
	case m.statsMsg != "":
		return lipgloss.NewStyle().Foreground(colorPrimary).Render(m.statsMsg)
	case m.searchMsg != "":
		return lipgloss.NewStyle().Foreground(colorPrimary).Render(m.searchMsg)
	case m.results.HasDirtyCells():
		return mutedStyle.Render(fmt.Sprintf("%d unsaved", m.results.DirtyCellCount()))
	case m.totalRowsSet && m.pageMsg != "":
		return mutedStyle.Render(m.pageMsg)
	case m.results.HasResult() && m.results.Message() != "":
		return successStyle.Render(m.results.Message())
	case m.pageMsg != "":
		return mutedStyle.Render(m.pageMsg)
	}
	return ""
}

// statusBar renders the single-line status bar shown at the bottom of the
// workspace. It carries contextual info (connection, database, table, result
// dimensions, transient messages) plus a single "?" hint for the help overlay.
// All other keybindings live behind the "?" overlay.
func (m Model) statusBar(connName string) string {
	sep := mutedStyle.Render("  │  ")
	parts := []string{m.connectionInfo(connName)}

	if m.focus == FocusEditor {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("[%s]", m.editor.VimModeStr())))
	}

	if m.results.IsVisualMode() {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorAccent).Render(
			fmt.Sprintf("VISUAL %d", m.results.VisualRangeSize())))
	}

	if t := m.currentTable(); t != "" {
		parts = append(parts,
			lipgloss.NewStyle().Foreground(colorLabel).Render(t))
	}

	if n := m.results.MarkCount(); n > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMark).Render(fmt.Sprintf("◆ %d", n)))
	}

	if n := m.results.HiddenCount(); n > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorAccent).Render(fmt.Sprintf("⊫ %d", n)))
	}

	if msg := m.statusMessage(); msg != "" {
		parts = append(parts, msg)
	}

	if len(m.filters) > 0 {
		short := make([]string, len(m.filters))
		for i, f := range m.filters {
			short[i] = compactFilter(f)
		}
		parts = append(parts, successStyle.Render(strings.Join(short, " ")))
	}

	parts = append(parts, lipgloss.NewStyle().Foreground(colorLabel).Render("?")+mutedStyle.Render(" help"))

	left := strings.Join(parts, sep)

	// Right-align context keybinding hints.
	// The caller prepends a single space, so effective width is m.width-1.
	hints := m.hintList()
	if len(hints) > 0 {
		flashActive := m.hintFlash != "" && time.Since(m.hintFlashAt) < hintFlashDuration
		keyStyle := lipgloss.NewStyle().Foreground(colorLabel)
		flashStyle := lipgloss.NewStyle().Foreground(colorFg)
		sepStyle := lipgloss.NewStyle().Foreground(colorMuted)
		var hintsStyled string
		for i, group := range hints {
			for ki, k := range strings.Split(group, "/") {
				if i > 0 && ki == 0 {
					hintsStyled += sepStyle.Render("/")
				}
				if ki > 0 {
					hintsStyled += sepStyle.Render("/")
				}
				if flashActive && k == m.hintFlash {
					hintsStyled += flashStyle.Render(k)
				} else {
					hintsStyled += keyStyle.Render(k)
				}
			}
		}
		gapW := m.width - 1 - lipgloss.Width(left) - lipgloss.Width(hintsStyled)
		if gapW < 1 {
			gapW = 1
		}
		line := left + strings.Repeat(" ", gapW) + hintsStyled
		if lipgloss.Width(line) > m.width {
			line = lipgloss.NewStyle().MaxWidth(m.width).Render(line)
		}
		return line
	}

	if lipgloss.Width(left) > m.width {
		left = lipgloss.NewStyle().MaxWidth(m.width).Render(left)
	}
	return left
}

func (m Model) borderForFocus(f Focus) lipgloss.Color {
	if m.focus == f {
		return colorPrimary
	}
	return colorBorderUnfocused
}

func copyFlashTickCmd() tea.Cmd {
	return tea.Tick(time.Duration(copyFlashInterval)*time.Millisecond, func(time.Time) tea.Msg {
		return copyFlashTickMsg{}
	})
}

func copyFeedbackCmd() tea.Cmd {
	return tea.Batch(
		copyFlashTickCmd(),
		tea.Tick(time.Duration(copyMessageDuration)*time.Second, func(time.Time) tea.Msg {
			return copyCopiedClearMsg{}
		}),
	)
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
