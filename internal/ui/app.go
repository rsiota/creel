package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/bookmarks"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
	"github.com/ruben/gsql/internal/history"
	"github.com/ruben/gsql/internal/secrets"
)

// Focus represents which panel currently has keyboard focus.
type Focus int

const (
	FocusConnections Focus = iota
	FocusTabBar
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

// cloneResultMsg carries the result of an async row clone.
type cloneResultMsg struct {
	table string
	count int
	err   error
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

// spinnerTickMsg advances the query-in-flight spinner animation.
type spinnerTickMsg struct{}

// queryExecutedMsg is sent when a query finishes executing.
type queryExecutedMsg struct {
	query     string
	result    db.Result
	err       error
	page      int
	pageSize  int
	cancelled bool
}

// schemasLoadedMsg carries prefetched table schemas for autocomplete.
type schemasLoadedMsg struct {
	schemas map[string][]db.Column
}

// tableRowCountsMsg carries approximate row counts for sidebar display.
type tableRowCountsMsg struct {
	counts map[string]int64
}

// structureLoadedMsg delivers the metadata for the StructurePanel. The table
// name identifies which load completed so stale results (from a rapid
// re-open) are ignored.
type structureLoadedMsg struct {
	table string
	data  structureData
}

// connTestResultMsg carries the outcome of a connection test initiated from
// the add/edit form. err is nil on success.
type connTestResultMsg struct {
	driver db.Driver
	err    error
}

// crossSearchResultMsg carries partial results from one batch of tables.
type crossSearchResultMsg struct {
	results    []SearchResult
	tablesDone int
	batchEnd   int
	done       bool
}

// crossSearchStartMsg signals the search to begin executing.
type crossSearchStartMsg struct{}

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

// explainResultMsg carries the EXPLAIN query plan result.
type explainResultMsg struct {
	result db.Result
	err    error
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

// backendSearchTickMsg fires after the debounce delay to execute the query.
type backendSearchTickMsg struct{ input string }

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

	connList        ConnectionList
	connForm        ConnectionForm
	editor          QueryEditor
	results         ResultsTable // active tab's results (synced on tab switch)
	inspector       Inspector

	// Tab management
	resultsTabs  []*ResultsTab // All result tabs
	activeTabID  int           // Currently active tab ID
	nextTabID    int           // Counter for generating unique tab IDs
	tabBar       TabBar        // Tab navigation component
	history         HistoryPanel
	bookmarks       BookmarkPanel
	crossSearch     CrossSearchPanel
	dbPicker        DatabasePicker
	help            HelpPanel
	filterPicker    FilterPicker
	columnPicker    ColumnPicker
	exportPicker    ExportPicker
	importPrompt    ImportPrompt
	addColumnForm   AddColumnForm
	tableRenameForm TableRenameForm
	tableDesigner   TableDesigner
	schemaEditor    SchemaEditor
	cellEdit        CellEditPopup
	explainPanel    ExplainPanel
	palette         palette
	sidebarCursor   int
	sidebarScroll   int              // cached scroll offset of the first visible sidebar item
	sidebarViewAnchored bool        // mouse click froze the view; keyboard nav clears it
	tableRowCounts  map[string]int64 // approximate row counts for sidebar display
	expanded        map[string][]db.Column
	columnCache     map[string][]db.Column

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

	// Editor maximize toggle (ctrl+w)
	editorMaximized bool

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
	createDBInput  string
	createDBErr    string

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

	config            *config.Config
	connection        *db.Connection
	forceReadOnly     bool // --readonly CLI flag: forces every connection read-only
	historyStore      *history.Store
	historyNavEntries []string // cached queries for the current browse session
	historyNavIdx     int      // -1 = not browsing; otherwise index into historyNavEntries (most recent = len-1)
	historyNavSaved   string   // editor content before history browse started
	bookmarkStore     *bookmarks.Store
	connError         string
	tables            []string

	// Pagination
	page          int
	pageSize      int
	lastQuery     string
	pageMsg       string
	totalRows     int    // total rows in the current table (0 = unknown)
	totalRowsSet  bool   // whether totalRows has been fetched for this query
	statsMsg      string // transient column statistics display
	exportMsg     string // transient CSV export result display
	searchMsg     string // transient regex search result display
	truncateMsg   string // transient truncate result display
	deleteRowsMsg string // transient row deletion result display
	schemaMsg     string // transient schema change result display
	bookmarkMsg   string // transient bookmark result display

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
	queryStack       []queryStackEntry
	restoreCursor    bool
	restoreCursorRow int
	restoreCursorCol int

	// Column jump (: to fuzzy-match and move the column cursor).
	columnJumping bool
	columnJump    string

	// Client-side regex search (g/ to search, n/N to jump between matches).
	searching   bool
	searchQuery string
	lastSearch  string

	// Backend full-text search (/ on results, LIKE across all columns).
	backendSearching   bool
	backendSearchInput string
	backendSearchTimer *time.Timer

	// hintFlash is the individual key currently highlighted white on the status bar.
	hintFlash   string
	hintFlashAt time.Time

	// Async query execution state
	queryRunning   bool               // true while a query is in flight
	queryCancel    context.CancelFunc // cancels the running query
	querySpinner   int                // spinner animation frame index
	queryStart     time.Time          // when the current query started (for elapsed display)
	queryCancelled bool               // true if the user cancelled the running query
}

const defaultPageSize = 200

// NewModel creates a new top-level application model.
func NewModel(cfg *config.Config) Model {
	// Create initial tab
	firstTab := NewResultsTab(0, "New Query")

	m := Model{
		state:           stateConnections,
		focus:           FocusConnections,
		config:          cfg,
		editor:          NewQueryEditor(),
		results:         firstTab.Results,
		inspector:       NewInspector(),
		connList:        NewConnectionList(),
		history:         NewHistoryPanel(),
		bookmarks:       NewBookmarkPanel(),
		dbPicker:        NewDatabasePicker(),
		help:            NewHelpPanel(),
		filterPicker:    NewFilterPicker(),
		columnPicker:    NewColumnPicker(),
		exportPicker:    NewExportPicker(),
		importPrompt:    NewImportPrompt(),
		addColumnForm:   NewAddColumnForm(),
		tableRenameForm: NewTableRenameForm(),
		tableDesigner:   NewTableDesigner(),
		schemaEditor:    NewSchemaEditor(),
		cellEdit:        NewCellEditPopup(),
		historyStore:    history.NewStore(historyDir()),
		bookmarkStore:   bookmarks.NewStore(historyDir()),
		expanded:        make(map[string][]db.Column),
		pageSize:        defaultPageSize,
		// Tab management
		resultsTabs:  []*ResultsTab{firstTab},
		activeTabID:  0,
		nextTabID:     1,
		tabBar:       NewTabBar(),
	}
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
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
		if conn.Driver == "mysql" || conn.Driver == "postgres" {
			detail = conn.Host
			defaultPort := 3306
			if conn.Driver == "postgres" {
				defaultPort = 5432
			}
			if conn.Port != 0 && conn.Port != defaultPort {
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

// Tab management helper methods

// activeTab returns the currently active tab, or nil if no tabs exist.
func (m *Model) activeTab() *ResultsTab {
	for _, tab := range m.resultsTabs {
		if tab.ID == m.activeTabID {
			return tab
		}
	}
	return nil
}

// setActiveTab saves the current tab state, switches to the given tab, and
// restores its state into the Model.
func (m *Model) setActiveTab(id int) {
	m.saveTabState()
	m.cancelTransientModes()
	for _, tab := range m.resultsTabs {
		if tab.ID == id {
			m.activeTabID = id
			m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
			m.restoreTabState()
			m.inspector.Reset()
			m.layoutWorkspace()
			return
		}
	}
}

// addTab saves the current tab state, creates a new tab, and makes it active.
func (m *Model) addTab(title string, query string) {
	m.saveTabState()
	m.cancelTransientModes()
	tab := NewResultsTab(m.nextTabID, title)
	m.nextTabID++
	if query != "" {
		tab.SetQuery(query)
		tab.EditorQuery = query
	}
	m.resultsTabs = append(m.resultsTabs, tab)
	m.activeTabID = tab.ID
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	m.restoreTabState()
}

// closeTab removes a tab by ID. If the active tab is closed, state is
// restored from the adjacent tab.
func (m *Model) closeTab(id int) {
	if len(m.resultsTabs) <= 1 {
		return
	}

	closingActive := id == m.activeTabID
	var newTabs []*ResultsTab
	switchToID := -1
	for i, tab := range m.resultsTabs {
		if tab.ID != id {
			newTabs = append(newTabs, tab)
		} else if closingActive {
			if i > 0 {
				switchToID = m.resultsTabs[i-1].ID
			} else {
				switchToID = m.resultsTabs[i+1].ID
			}
		}
	}

	m.resultsTabs = newTabs
	if closingActive && switchToID >= 0 {
		m.activeTabID = switchToID
		m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
		m.cancelTransientModes()
		m.restoreTabState()
		m.inspector.Reset()
		m.layoutWorkspace()
	} else {
		m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	}
}

// generateTabTitle creates a concise title from a query.
func generateTabTitle(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "New Query"
	}

	// Extract table name from simple queries
	lower := strings.ToLower(query)
	if strings.Contains(lower, "from") {
		parts := strings.Split(lower, "from")
		if len(parts) > 1 {
			tableName := strings.TrimSpace(strings.Split(parts[1], " ")[0])
			// Remove schema prefix if present
			if idx := strings.LastIndex(tableName, "."); idx >= 0 {
				tableName = tableName[idx+1:]
			}
			return tableName
		}
	}

	// Truncate long queries
	if len(query) > 20 {
		return query[:17] + "…"
	}
	return query
}

// clearPendingG resets all pending-G flags across panels.
func (m *Model) clearPendingG() {
	m.resultsPendingG = false
	m.sidebarPendingG = false
	m.inspector.pendingG = false
}

// handleTabKey processes tab-related keybindings that work from any focused
// panel (except the editor in insert mode). Returns true if the key was
// consumed.
//
// g t / g T — next / previous tab
// g x       — close tab
// g 1-9     — go to tab N
// t         — new tab (sidebar, results, tab bar only)
func (m *Model) handleTabKey(msg tea.KeyMsg) bool {
	if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() ||
		m.searching || m.columnJumping || m.backendSearching ||
		m.sidebarFiltering || m.inspector.IsFiltering() ||
		(m.focus == FocusEditor && m.editor.VimMode() == VimInsert) {
		return false
	}

	s := msg.String()
	anyPendingG := m.resultsPendingG || m.sidebarPendingG || m.inspector.pendingG

	if anyPendingG {
		switch s {
		case "t":
			m.clearPendingG()
			if nextID := m.tabBar.NextTab(); nextID >= 0 {
				m.setActiveTab(nextID)
			}
			return true
		case "T":
			m.clearPendingG()
			if prevID := m.tabBar.PrevTab(); prevID >= 0 {
				m.setActiveTab(prevID)
			}
			return true
		case "x":
			m.clearPendingG()
			m.closeTab(m.activeTabID)
			return true
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			m.clearPendingG()
			n := int(s[0] - '0')
			if tabID := m.tabBar.GotoTab(n); tabID >= 0 {
				m.setActiveTab(tabID)
			}
			return true
		}
	}

	if s == "t" && !anyPendingG &&
		(m.focus == FocusConnections || m.focus == FocusResults || m.focus == FocusTabBar) {
		query := m.editor.Value()
		m.addTab(generateTabTitle(query), query)
		return true
	}

	return false
}

// Init initializes the application.
func (m Model) Init() tea.Cmd {
	return nil
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
		// While a query is in flight, esc and ctrl+c cancel the query
		// instead of their normal behaviour. All other keys are swallowed
		// so the user can't trigger overlapping operations.
		// Backend search mode is exempt: it needs keystrokes to pass
		// through to updateWorkspace so the user can keep typing while
		// the previous search query is still running.
		if m.queryRunning && !m.backendSearching {
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "ctrl+c"))) {
				m.queryCancelled = true
				if m.queryCancel != nil {
					m.queryCancel()
					m.queryCancel = nil
				}
				return m, nil
			}
			return m, nil
		}

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

	case tea.MouseMsg:
		if m.state == stateConnections {
			return m.handleConnectionsMouse(msg)
		}
		if m.state == stateWorkspace {
			return m.handleWorkspaceMouse(msg)
		}
		return m, nil

	case queryExecutedMsg:
		m.queryRunning = false
		m.queryCancel = nil

		// Silently discard results from queries that were superseded (not
		// user-cancelled) — the newer query's result will replace them.
		if msg.cancelled && !m.queryCancelled {
			return m, nil
		}

		m.layoutWorkspace()

		// User-cancelled queries show a message but keep existing results.
		if m.queryCancelled {
			m.results.SetError("Query cancelled")
			m.queryCancelled = false
			return m, nil
		}

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

	case spinnerTickMsg:
		if !m.queryRunning {
			return m, nil
		}
		m.querySpinner = (m.querySpinner + 1) % len(spinnerFrames)
		return m, spinnerTick()

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

	case cloneResultMsg:
		if msg.err != nil {
			m.schemaMsg = fmt.Sprintf("clone failed: %v", msg.err)
		} else {
			m.schemaMsg = fmt.Sprintf("cloned %d row%s into %s", msg.count, plural(msg.count), msg.table)
			m.results.ClearMarks()
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

	case tableRowCountsMsg:
		m.tableRowCounts = msg.counts
		return m, nil

	case structureLoadedMsg:
		// Route read-only metadata into the schema editor's structure tabs.
		// Ignore stale results from a previous table.
		if m.schemaEditor.IsVisible() && m.schemaEditor.Table() == msg.table {
			m.schemaEditor.LoadStructure(msg.data)
		}
		return m, nil

	case connTestResultMsg:
		// Only relevant while the form is open; a save (enter) leaves the
		// form state, so a late result is dropped.
		if m.state != stateAddConnection {
			return m, nil
		}
		if msg.err != nil {
			m.connForm.SetTestResult(msg.err.Error(), false)
		} else {
			m.connForm.SetTestResult(fmt.Sprintf("✓ Connected (%s)", msg.driver), true)
		}
		return m, nil

	case crossSearchStartMsg:
		// Begin searching from the first table.
		query := m.crossSearch.Query()
		m.crossSearch.StartSearch(len(m.tables))
		return m, m.runCrossSearchBatch(query, 0)

	case crossSearchResultMsg:
		m.crossSearch.AddResults(msg.results, msg.tablesDone)
		if msg.done || len(m.crossSearch.results) >= crossSearchMaxResults {
			m.crossSearch.FinishSearch()
			return m, nil
		}
		// Continue with next batch.
		return m, m.runCrossSearchBatch(m.crossSearch.Query(), msg.batchEnd)

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

	case explainResultMsg:
		if msg.err != nil {
			m.statsMsg = fmt.Sprintf("EXPLAIN error: %v", msg.err)
			return m, nil
		}
		driver := db.DriverSQLite
		if m.connection != nil {
			driver = m.connection.Config().Driver
		}
		m.explainPanel.Show(msg.result, driver)
		return m, nil

	case backendSearchTickMsg:
		// Only execute if the input still matches (user may have typed more).
		if m.backendSearching && msg.input == m.backendSearchInput {
			return m, m.runBackendSearch(msg.input)
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
	// Best-effort purge of any keychain secrets for this connection. A missing
	// key (the connection never used the keychain) is not an error.
	_ = secrets.DeleteAll(name)
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
	case "ctrl+t":
		// Test the connection without saving. Disabled while a test is
		// already in flight to avoid opening parallel connections.
		if m.connForm.testing {
			return m, nil
		}
		return m, m.testConnection()
	case "enter":
		connCfg, errMsg := m.connForm.EnterPressed()
		if errMsg != "" {
			m.connForm.SetError(errMsg)
			return m, nil
		}

		if m.connForm.mode == formModeEdit {
			// Preserve fields the form does not expose (currently
			// ssh_passphrase) so editing a connection does not wipe them.
			var oldPassphrase string
			if existing := m.config.GetConnection(m.connForm.editing); existing != nil {
				oldPassphrase = existing.SSHPassphrase
			}
			m.config.RemoveConnection(m.connForm.editing)
			connCfg.SSHPassphrase = oldPassphrase
		}

		// Migrate secret fields to the OS keychain when requested. Falls back
		// to plaintext (in the config file) if the keychain is unavailable.
		connCfg, secErr := storeConnSecrets(connCfg, m.connForm.secretsMode())
		m.connError = ""
		if secErr != nil {
			m.connError = secErr.Error()
		}

		m.config.AddConnection(connCfg)
		if err := m.config.Save(); err != nil {
			m.connForm.SetError(err.Error())
			return m, nil
		}

		m.state = stateConnections
		m.loadConnections()
		return m, nil
	}

	var cmd tea.Cmd
	m.connForm, cmd = m.connForm.Update(msg)
	return m, cmd
}

// storeConnSecrets migrates a connection's secret fields to the OS keychain
// when mode is "keychain", replacing them in the config with opaque references.
// It returns the (possibly modified) config and an error describing why the
// keychain could not be used; in that case the config is returned unchanged so
// the caller falls back to storing plaintext.
//
// Only fields the form exposes (password, ssh_password) are managed here.
// ssh_passphrase is resolved at connect time but not edited via the form.
func storeConnSecrets(cfg config.ConnectionConfig, mode string) (config.ConnectionConfig, error) {
	if mode != "keychain" {
		return cfg, nil
	}
	if !secrets.Available() {
		return cfg, fmt.Errorf("keychain unavailable on this system; secrets stored in config file")
	}
	type secretField struct {
		name string
		val  string
	}
	fields := []secretField{
		{secrets.FieldPassword, cfg.Password},
		{secrets.FieldSSHPassword, cfg.SSHPassword},
	}
	for _, fl := range fields {
		if fl.val == "" || secrets.IsReference(fl.val) {
			continue
		}
		ref, err := secrets.Store(cfg.Name, fl.name, fl.val)
		if err != nil {
			return cfg, fmt.Errorf("storing %s: %w", fl.name, err)
		}
		switch fl.name {
		case secrets.FieldPassword:
			cfg.Password = ref
		case secrets.FieldSSHPassword:
			cfg.SSHPassword = ref
		}
	}
	return cfg, nil
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
		// Filter mode (default): typing filters, esc → normal mode.
		if m.dbPicker.Filtering() {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.dbPicker.StopFiltering()
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

		// Normal mode: single-letter commands.
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.dbPicker.MustChoose() {
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
		case "/":
			m.dbPicker.StartFiltering()
			return m, nil
		case "j", "down":
			m.dbPicker.CursorDown()
			return m, nil
		case "k", "up":
			m.dbPicker.CursorUp()
			return m, nil
		case "N":
			m.createDBActive = true
			m.createDBInput = ""
			m.createDBErr = ""
			return m, nil
		case "D":
			name := m.dbPicker.SelectedDatabase()
			if name != "" {
				m.dropDBConfirm = name
				m.dropDBInput = ""
			}
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
		onColumnsTab := m.schemaEditor.ActiveTab() == seTabColumns
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.schemaEditor.IsEditing() {
				m.schemaEditor, _ = m.schemaEditor.Update(msg)
				return m, nil
			}
			m.schemaEditor.Hide()
			return m, nil
		case "enter":
			// Column editing only applies on the Columns tab; read-only tabs
			// (e.g. expand a trigger) handle enter inside Update.
			if onColumnsTab {
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
			}
		case "d":
			// dd to drop column — only existing rows go through the confirm
			// flow, and only on the Columns tab. New rows are removed locally
			// by the editor's Update.
			if onColumnsTab && m.schemaEditor.pendingD && !m.schemaEditor.IsNewRow() {
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
				m.history.StartFilter()
				return m, nil
			}
			if m.clearBookmarksConfirm {
				m.clearBookmarksConfirm = false
				if m.connection != nil && m.bookmarkStore != nil {
					m.bookmarkStore.Clear(m.connection.Config().Name)
				}
				m.bookmarks.SetEntries(nil)
				m.bookmarks.StartFilter()
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
			val := m.cellEdit.Value()
			// Compact JSON so pretty-printing in the popup doesn't create
			// a false dirty cell when the user just views and saves.
			if compacted, ok := compactJSON(val); ok {
				val = compacted
			}
			orig := m.results.RowValue(m.cellEdit.Row(), m.cellEdit.Col())
			if orig == "NULL" {
				orig = ""
			}
			if origCompacted, ok := compactJSON(orig); ok {
				orig = origCompacted
			}
			if val != orig {
				m.results.SetDirtyCell(m.cellEdit.Row(), m.cellEdit.Col(), val)
			}
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

	// Explain panel is modal — j/k scroll, esc/q close.
	if m.explainPanel.IsVisible() {
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.explainPanel.Hide()
			return m, nil
		}
		m.explainPanel = m.explainPanel.Update(msg)
		return m, nil
	}

	// Command palette is modal — intercept all keys when visible.
	if m.palette.visible {
		var cmd tea.Cmd
		m.palette, cmd = m.palette.Update(msg)
		return m, cmd
	}

	// Tab navigation keys work from any panel (except editor insert mode).
	if m.handleTabKey(msg) {
		return m, nil
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
			m.history.IsVisible() ||
			m.bookmarks.IsVisible() ||
			m.backendSearching ||
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
		// Refresh schema (tables + columns) and re-run the last query.
		// Block while editing to avoid discarding unsaved changes.
		if m.results.IsEditing() || m.results.HasDirtyCells() || m.inspector.IsInserting() {
			return m, nil
		}
		m.loadTables()
		cmd := m.prefetchSchemas()
		if m.lastQuery != "" {
			m.page = 0
			m.filters = nil
			m.sortCol = ""
			m.sortDir = ""
			m.queryStack = nil
			m.schemaMsg = "refreshed schema & results"
			return m, tea.Batch(cmd, m.runPageQuery())
		}
		m.schemaMsg = "refreshed schema"
		return m, cmd
	case "ctrl+w":
		m.editorMaximized = !m.editorMaximized
		m.layoutWorkspace()
		return m, nil
	case "ctrl+b":
		// Browse databases (MySQL only).
		if m.connection != nil && (m.connection.Config().Driver == db.DriverMySQL || m.connection.Config().Driver == db.DriverPostgres) {
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
		// Close cross-search panel if visible.
		if m.crossSearch.IsVisible() {
			m.crossSearch.Hide()
			return m, nil
		}
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
		// If in backend search mode, let the focused panel handle esc.
		if m.backendSearching {
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
		case "esc":
			m.history.CancelFilter()
			m.history.Toggle()
			return m, nil
		case "ctrl+c":
			m.history.CancelFilter()
			m.history.Toggle()
			return m, nil
		case "enter":
			q := m.history.SelectedQuery()
			if q != "" {
				m.editor.SetValue(q)
				m.focus = FocusEditor
				m.applyFocus()
			}
			m.history.CancelFilter()
			m.history.Toggle()
			return m, m.editor.Focus()
		case "backspace":
			if len(m.history.filter) > 0 {
				m.history.filter = m.history.filter[:len(m.history.filter)-1]
				m.history.cursor = 0
				m.history.scrollRow = 0
			}
			return m, nil
		case "up", "k":
			m.history.CursorUp()
			return m, nil
		case "down", "j":
			m.history.CursorDown()
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
			m.history.CancelFilter()
			m.history.Toggle()
			return m, nil
		}
		// Printable characters extend the filter.
		if msg.Type == tea.KeyRunes {
			m.history.filter += msg.String()
			m.history.cursor = 0
			m.history.scrollRow = 0
			return m, nil
		}
	}

	// Bookmarks panel takes over navigation when visible
	if m.bookmarks.IsVisible() {
		switch msg.String() {
		case "esc":
			m.bookmarks.CancelFilter()
			m.bookmarks.Toggle()
			return m, nil
		case "ctrl+c":
			m.bookmarks.CancelFilter()
			m.bookmarks.Toggle()
			return m, nil
		case "enter":
			q := m.bookmarks.SelectedQuery()
			if q != "" {
				m.editor.SetValue(q)
				m.focus = FocusEditor
				m.applyFocus()
			}
			m.bookmarks.CancelFilter()
			m.bookmarks.Toggle()
			return m, m.editor.Focus()
		case "backspace":
			if len(m.bookmarks.filter) > 0 {
				m.bookmarks.filter = m.bookmarks.filter[:len(m.bookmarks.filter)-1]
				m.bookmarks.cursor = 0
				m.bookmarks.scrollRow = 0
			}
			return m, nil
		case "up", "k":
			m.bookmarks.CursorUp()
			return m, nil
		case "down", "j":
			m.bookmarks.CursorDown()
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
		// Printable characters extend the filter.
		if msg.Type == tea.KeyRunes {
			m.bookmarks.filter += msg.String()
			m.bookmarks.cursor = 0
			m.bookmarks.scrollRow = 0
			return m, nil
		}
	}

	// Cross-search panel takes over navigation when visible
	if m.crossSearch.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.crossSearch.Hide()
			return m, nil
		case "enter":
			// If we have results (search done), navigate to selected result.
			if !m.crossSearch.searching && len(m.crossSearch.results) > 0 {
				if r := m.crossSearch.SelectedResult(); r != nil {
					m.crossSearch.Hide()
					m.syncSidebarCursorToTable(r.Table)
					m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s WHERE %s = '%s';",
						r.Table, r.Column, strings.ReplaceAll(r.Value, "'", "''")))
					return m, m.executeQuery()
				}
				return m, nil
			}
			// Otherwise, start the search if a query is typed.
			if m.crossSearch.Query() != "" && !m.crossSearch.searching {
				return m, m.startCrossSearch()
			}
			return m, nil
		case "backspace":
			m.crossSearch.Backspace()
			return m, nil
		case "up":
			m.crossSearch.CursorUp()
			return m, nil
		case "down":
			m.crossSearch.CursorDown()
			return m, nil
		}
		if msg.Type == tea.KeyRunes || msg.String() == " " {
			m.crossSearch.AddQueryChar(msg.String())
			return m, nil
		}
	}

	// Dispatch to focused panel
	switch m.focus {
	case FocusTabBar:
		switch msg.String() {
		case "h", "left":
			if prevID := m.tabBar.PrevTab(); prevID >= 0 {
				m.setActiveTab(prevID)
			}
			return m, nil
		case "l", "right":
			if nextID := m.tabBar.NextTab(); nextID >= 0 {
				m.setActiveTab(nextID)
			}
			return m, nil
		case "j", "down", "enter":
			m.focus = FocusEditor
			m.applyFocus()
			return m, nil
		}
		return m, nil
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

		// Command history navigation: up/down arrow in vim normal mode.
		if m.editor.VimMode() == VimNormal && !m.editor.CompletionVisible() {
			switch msg.String() {
			case "up":
				if m.connection != nil && m.historyStore != nil {
					if m.historyNavIdx == -1 {
						entries, err := m.historyStore.Get(m.connection.Config().Name)
						if err != nil || len(entries) == 0 {
							return m, nil
						}
						m.historyNavEntries = make([]string, len(entries))
						for i, e := range entries {
							m.historyNavEntries[i] = e.Query
						}
						m.historyNavSaved = m.editor.Value()
						m.historyNavIdx = len(m.historyNavEntries) - 1
					} else if m.historyNavIdx > 0 {
						m.historyNavIdx--
					} else {
						return m, nil // already at oldest
					}
					m.editor.SetValue(m.historyNavEntries[m.historyNavIdx])
				}
				return m, nil
			case "down":
				if m.historyNavIdx >= 0 {
					if m.historyNavIdx < len(m.historyNavEntries)-1 {
						m.historyNavIdx++
						m.editor.SetValue(m.historyNavEntries[m.historyNavIdx])
					} else {
						m.historyNavIdx = -1
						m.editor.SetValue(m.historyNavSaved)
					}
					return m, nil
				}
			}
			// Reset history navigation on any other key in normal mode.
			if msg.String() != "up" && msg.String() != "down" {
				m.historyNavIdx = -1
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
		// Backend search mode intercepts all keys.
		if m.backendSearching {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.cancelBackendSearch()
				return m, nil
			case "enter":
				m.commitBackendSearch()
				return m, nil
			case "backspace":
				if len(m.backendSearchInput) > 0 {
					m.backendSearchInput = m.backendSearchInput[:len(m.backendSearchInput)-1]
					return m, m.scheduleBackendSearch()
				}
				return m, nil
			}
			if msg.Type == tea.KeyRunes {
				m.backendSearchInput += msg.String()
				return m, m.scheduleBackendSearch()
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

		// g e — explain query plan (works regardless of whether results are loaded).
		if msg.String() == "e" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			return m, m.explainQuery()
		}

		// g/G navigation works on empty tables too.
		if msg.String() == "g" && !m.resultsPendingG {
			m.resultsPendingG = true
			return m, nil
		}
		if msg.String() == "G" {
			m.resultsPendingG = false
			m.resultsPendingY = false
			m.results.CursorBottom()
			return m, nil
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
			case "0":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorFirstCol()
				return m, nil
			case "$":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorLastCol()
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
			case "p":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if !m.results.IsEditable() || !m.results.HasPrimaryKey() || m.inspector.IsVisible() {
					return m, nil
				}
				colName := m.results.ColumnName(m.results.CursorCol())
				if m.results.isPKColumn(colName) {
					return m, nil
				}
				clip, err := clipboard.ReadAll()
				if err != nil || clip == "" {
					m.exportMsg = "clipboard is empty"
					return m, copyFeedbackCmd()
				}
				m.results.SetDirtyCell(m.results.CursorRow(), m.results.CursorCol(), clip)
				return m, m.saveChanges()
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
					// g/ — client-side regex search on loaded page.
					m.resultsPendingG = false
					m.resultsPendingY = false
					if m.results.NumRows() > 0 {
						m.searching = true
						m.searchQuery = ""
					}
					return m, nil
				}
				// / — backend full-text search across all columns.
				m.resultsPendingY = false
				if m.canFilter() {
					m.backendSearching = true
					m.backendSearchInput = ""
					return m, nil
				}
			case "f":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					if m.canFilter() {
						return m, m.openFilterPicker()
					}
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
			case "Y":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.NumRows() > 0 {
					sql, count := m.results.CopyAsInsert()
					if count > 0 {
						_ = clipboard.WriteAll(sql)
						m.results.StartCopyFeedback()
						if count >= copyAsInsertMaxRows {
							m.exportMsg = fmt.Sprintf("copied %d rows as INSERT (cap %d)", count, copyAsInsertMaxRows)
						} else {
							m.exportMsg = fmt.Sprintf("copied %d rows as INSERT", count)
						}
						return m, copyFeedbackCmd()
					}
				}
			case "P":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.cloneRows()
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
			case " ":
				// Exit filter mode and toggle expand on the highlighted table.
				if item := m.currentSidebarItem(); item != nil && !item.isColumn {
					selected := item.text
					m.sidebarFiltering = false
					m.sidebarFilter = ""
					m.syncSidebarCursorToTable(selected)
					m.toggleExpand()
					return m, nil
				}
				m.sidebarFiltering = false
				m.sidebarFilter = ""
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
				return m, m.executeQuery()
			}
		case "d":
			m.sidebarPendingG = false
			if m.sidebarSelectedTable() != "" {
				return m, m.openSchemaPanel()
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
		case "S":
			m.sidebarPendingG = false
			if m.connection != nil && len(m.tables) > 0 {
				m.crossSearch.Show()
				return m, m.editor.Focus()
			}
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
		panelY := (m.height - 1 - panelH) / 2
		bg := strings.Repeat("\n", m.height-2)
		view := placeOverlay(bg, pickerPanel, panelX, panelY)

		// Overlay create-database dialog on top of the picker if active.
		if m.createDBActive {
			dialog := renderInputDialogBare("Create new database", m.createDBInput, m.createDBErr)
			dw := lipgloss.Width(dialog)
			dh := lipgloss.Height(dialog)
			view = placeOverlay(view, dialog, (m.width-dw)/2, (m.height-1-dh)/2)
		}

		// Overlay drop-database confirmation on top of the picker if active.
		if m.dropDBConfirm != "" {
			dialog := renderTypedConfirmDialogBare(
				"Drop database "+m.dropDBConfirm+"?",
				m.dropDBConfirm,
				m.dropDBInput,
			)
			view = placeOverlay(view, dialog, (m.width-lipgloss.Width(dialog))/2, (m.height-1-lipgloss.Height(dialog))/2)
		}

		// Append status bar.
		connName := ""
		if m.connection != nil {
			connName = m.connection.Config().Name
		}
		statusBar := lipgloss.NewStyle().
			Width(m.width).
			Height(1).
			Foreground(colorMuted).
			Background(colorStatusBarBg).
			Render(" " + m.statusBar(connName))
		return lipgloss.JoinVertical(lipgloss.Left, view, statusBar)
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
	panelY := (m.height - 1 - panelH2) / 2
	bg := strings.Repeat("\n", m.height-2)
	view := placeOverlay(bg, connPanel, panelX, panelY)

	// Overlay help panel if visible
	if m.help.IsVisible() {
		m.help.SetSize(m.width, m.height)
		view = m.help.View()
		return view
	}

	// Append status bar.
	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(1).
		Foreground(colorMuted).
		Background(colorStatusBarBg).
		Render(" " + m.statusBar(""))
	return lipgloss.JoinVertical(lipgloss.Left, view, statusBar)
}

func (m Model) viewAddConnection() string {
	popupW, popupH := popupDim()
	borderOverhead := 2
	padding := 2 // padding(0,1) = 1 left + 1 right
	innerW := popupW - borderOverhead - padding
	m.connForm.SetMaxWidth(innerW)

	formPanel := lipgloss.NewStyle().
		Width(popupW-borderOverhead).
		Height(popupH-borderOverhead).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(m.connForm.View())

	// Center the popup in the area above the status bar, then append the
	// status bar so the keybinding hints (enter / ctrl+t / esc) are visible.
	placed := lipgloss.Place(m.width, m.height-1,
		lipgloss.Center, lipgloss.Center,
		formPanel,
		lipgloss.WithWhitespaceChars(" "))
	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(1).
		Foreground(colorMuted).
		Background(colorStatusBarBg).
		Render(" " + m.statusBar(""))
	return lipgloss.JoinVertical(lipgloss.Left, placed, statusBar)
}

func (m Model) viewWorkspace() string {
	sidebarWidth := 30
	inspectorWidth := InspectorWidth
	statusHeight := 1
	tabBarHeight := 0 // tabs are inside the editor panel
	borderOverhead := 2
	editorHeight := 12

	if m.editorMaximized {
		editorHeight = m.height - statusHeight - tabBarHeight - borderOverhead - 12
		if editorHeight < 8 {
			editorHeight = 8
		}
	}

	resultsHeight := m.height - tabBarHeight - editorHeight - statusHeight - borderOverhead
	if resultsHeight < 3 {
		resultsHeight = 3
	}

	rightWidth := m.width - sidebarWidth - borderOverhead
	if m.inspector.IsVisible() {
		rightWidth -= inspectorWidth
	}

	// Build the content area (tabs are inside the editor panel).
	var contentPanel string
	if m.tableDesigner.IsVisible() {
		designerHeight := editorHeight + resultsHeight
		m.tableDesigner.SetSize(rightWidth-borderOverhead, designerHeight-borderOverhead)
		contentPanel = lipgloss.NewStyle().
			Width(rightWidth).
			Height(designerHeight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Render(m.tableDesigner.View())
	} else if m.schemaEditor.IsVisible() {
		editorH := editorHeight + resultsHeight
		m.schemaEditor.SetSize(rightWidth-borderOverhead, editorH-borderOverhead)
		contentPanel = lipgloss.NewStyle().
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
			Render(lipgloss.JoinVertical(lipgloss.Left,
				m.tabBar.View(),
				lipgloss.NewStyle().Foreground(colorBorder).
					Render(strings.Repeat("─", rightWidth)),
				m.editor.View(),
			))

		var resultsPanel string
		if m.queryRunning && !m.backendSearching {
			// Show an animated spinner while the query executes.
			frame := spinnerFrames[m.querySpinner%len(spinnerFrames)]
			elapsed := time.Since(m.queryStart).Round(time.Millisecond)
			content := lipgloss.NewStyle().Foreground(colorPrimary).Render(frame) +
				"  " + mutedStyle.Render(fmt.Sprintf("running query… %s", elapsed)) +
				"  " + lipgloss.NewStyle().Foreground(colorMuted).Render("(esc to cancel)")
			resultsPanel = lipgloss.NewStyle().
				Width(rightWidth).
				Height(resultsHeight).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(m.borderForFocus(FocusResults)).
				Align(lipgloss.Center, lipgloss.Center).
				Render(content)
		} else {
			// When the table has results it draws its own border, merging
			// seamlessly with the panel frame. Otherwise fall back to a
			// standard rounded border around the placeholder message.
			hasTable := m.results.HasResult() && m.results.NumCols() > 0
			m.results.SetBorderColor(m.borderForFocus(FocusResults))

			var resultsStyle lipgloss.Style
			if hasTable {
				resultsStyle = lipgloss.NewStyle().
					Width(rightWidth + borderOverhead).
					Height(resultsHeight + borderOverhead)
			} else {
				resultsStyle = lipgloss.NewStyle().
					Width(rightWidth).
					Height(resultsHeight).
					Border(lipgloss.RoundedBorder()).
					BorderForeground(m.borderForFocus(FocusResults))
			}
			resultsPanel = resultsStyle.Render(func() string {
				m.results.SetSort(m.sortCol, m.sortDir)
				// Shrink the table by one row when a prompt line is
				// shown above it, so the total height stays the same.
				if m.columnJumping || m.searching || m.backendSearching {
					m.results.SetSize(rightWidth+borderOverhead, resultsHeight+borderOverhead-1)
				}
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
				if m.backendSearching {
					view = " " + renderPalettePrompt(m.backendSearchInput, true) + "\n" + view
				}
				return view
			}())
		}

		contentPanel = lipgloss.JoinVertical(lipgloss.Left,
			editorPanel,
			resultsPanel,
		)
	}

	rightPanel := contentPanel

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

	// Scroll window: cursor-centered for keyboard nav, frozen when the view was
	// anchored by a mouse click. The shared helper is also used by the mouse
	// handler so a click always maps to the rendered item.
	start := m.sidebarRenderedStart()
	end := start + maxVisible
	if end > len(items) {
		end = len(items)
	}
	m.sidebarScroll = start

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
			if isCursor && m.sidebarFiltering {
				line = selectedStyle.Render(expandIcon + " " + item.text)
			} else {
				if m.sidebarFiltering {
					tableName = highlightMatches(item.text, item.matchIdx)
				}
				line = style.Render(expandIcon + " " + tableName)
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
		Background(colorStatusBarBg).
		Render(" " + m.statusBar(connName))

	var workspace string
	if m.inspector.IsVisible() {
		workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel, inspectorPanel)
	} else {
		workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel)
	}

	// Dim the workspace panels behind long-lived editing overlays.
	// The status bar is kept undimmed so hints remain clearly visible.
	if m.cellEdit.IsVisible() || m.history.IsVisible() || m.bookmarks.IsVisible() || m.crossSearch.IsVisible() || m.explainPanel.IsVisible() {
		workspace = dimBackground(workspace)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, workspace, statusBar)

	// Overlay history panel if visible
	if m.history.IsVisible() {
		m.history.SetSize(m.width*65/100, (m.height-1)*65/100)
		histPanel := m.history.View()
		panelW := lipgloss.Width(histPanel)
		panelH := lipgloss.Height(histPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, histPanel, panelX, panelY)
	}

	// Overlay bookmarks panel if visible
	if m.bookmarks.IsVisible() {
		m.bookmarks.SetSize(m.width*65/100, (m.height-1)*65/100)
		bmPanel := m.bookmarks.View()
		panelW := lipgloss.Width(bmPanel)
		panelH := lipgloss.Height(bmPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, bmPanel, panelX, panelY)
	}

	// Overlay cross-search panel if visible
	if m.crossSearch.IsVisible() {
		m.crossSearch.SetSize(m.width*65/100, (m.height-1)*65/100)
		csPanel := m.crossSearch.View()
		panelW := lipgloss.Width(csPanel)
		panelH := lipgloss.Height(csPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, csPanel, panelX, panelY)
	}

	// Overlay explain panel if visible
	if m.explainPanel.IsVisible() {
		m.explainPanel.SetSize(m.width*70/100, (m.height-1)*70/100)
		explainPanelView := m.explainPanel.View()
		panelW := lipgloss.Width(explainPanelView)
		panelH := lipgloss.Height(explainPanelView)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, explainPanelView, panelX, panelY)
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
		dialog := renderConfirmDialogBare(prompt)
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
		dialog := renderConfirmDialogBare("Clear all query history?\nThis cannot be undone.")
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay clear-bookmarks confirmation dialog if visible.
	if m.clearBookmarksConfirm {
		dialog := renderConfirmDialogBare("Clear all bookmarks?\nThis cannot be undone.")
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
			Width(popupW-borderOverhead).
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
			Width(popupW-borderOverhead).
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
		// Size the popup to ~65% of the screen, matching history/bookmarks.
		availW := m.width * 65 / 100
		availH := (m.height - 1) * 65 / 100
		// Subtract the cell editor's fixed overhead: label (1) + inner border
		// top/bottom (2) + outer rounded border top/bottom (2) = 5 rows.
		m.cellEdit.SetMaxSize(availW, availH-5)
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
		popupY := 1 + 2 + cursorLine + 1 // border + tab line + separator + cursor line + 1
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

		// Overlay completion dropdown as a floating popup below the input line.
		// The input line is the 5th visual row (0-indexed: border, padding,
		// title, blank, input = row 4), so the dropdown starts at row 5.
		if comp := m.importPrompt.CompletionView(); comp != "" {
			view = placeOverlay(view, comp, panelX+9, panelY+5)
		}
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
