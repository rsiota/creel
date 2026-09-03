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
	"github.com/rsiota/creel/internal/bookmarks"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
	"github.com/rsiota/creel/internal/history"
	"github.com/rsiota/creel/internal/recent"
	"github.com/rsiota/creel/internal/secrets"
	"github.com/rsiota/creel/internal/session"
)

// Focus represents which panel currently has keyboard focus.
type Focus int

const (
	FocusConnections Focus = iota
	FocusTabBar
	FocusEditor
	FocusResults
	FocusInspector
	FocusAssistant
	FocusExplorer
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
	query     string // lastQuery / user statement
	execQuery string // bytes actually sent (may include pagination wrap)
	result    db.Result
	err       error
	page      int
	pageSize  int
	cancelled bool // context was cancelled (superseded by a newer query)
	timedOut  bool // query exceeded the per-query deadline
}

// schemasLoadedMsg carries prefetched table schemas for autocomplete and
// for the AI schema context (columns + primary keys + foreign keys), so an
// AI request can build its prompt from memory instead of re-running
// 1+3N metadata queries every turn.
type schemasLoadedMsg struct {
	schemas map[string][]db.Column
	pks     map[string][]string
	fks     map[string][]db.ForeignKey
}

// schemaTablesLoadedMsg carries per-schema table lists for schema.table completion.
type schemaTablesLoadedMsg struct {
	cache map[string][]string
}

// qualifiedTableSchemaMsg carries columns for a schema.table cache key.
type qualifiedTableSchemaMsg struct {
	key  string
	cols []db.Column
	err  error
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
// query is the statement that was explained (for :aiexplain caching).
// forAI, when set, skips the overlay and hands the plan to the AI explainer.
type explainResultMsg struct {
	result db.Result
	err    error
	query  string
	forAI  bool
	focus  string // optional user focus for :aiexplain (e.g. "why is the join slow")
}

// lookupResultMsg carries a lookup panel's title and result table, produced
// by async ex commands like ":refs" and ":uses". jumps is optional and
// parallel to result.Rows: a non-empty entry makes that row Enter-jumpable.
type lookupResultMsg struct {
	title  string
	result db.Result
	jumps  []string
	err    error
}

// explorerLoadedMsg carries the explorer tree root (the focused row + its
// first-level edges) produced by loadExplorer. root is nil on the empty/error
// paths, in which case emptyMsg/err explains why. depth is the queryStack depth
// at load time.
type explorerLoadedMsg struct {
	root     *expNode
	depth    int
	emptyMsg string
	err      error
}

// explorerChildrenMsg carries lazily-loaded children for one tree node,
// produced by loadExplorerChildren when a node is expanded. parent is matched
// by pointer identity; if the panel was closed or the node removed in the
// meantime the message is ignored.
type explorerChildrenMsg struct {
	parent   *expNode
	children []*expNode
	fold     bool // nothing to show — fold the node back up
	err      error
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

// watchTickMsg is emitted by the :watch timer to trigger a periodic refresh of
// the last query. gen ties it to the active watch generation so restarting
// (:watch with a new interval) or stopping (:watch off) lets a stale chain die
// instead of stacking a second one.
type watchTickMsg struct{ gen uint64 }

// backendSearchTickMsg fires after the debounce delay to execute the query.
type backendSearchTickMsg struct{ input string }

// wheelTickMsg flushes the accumulated mouse-wheel delta for the results grid.
// The Magic Mouse / trackpad emit hundreds of momentum wheel events per swipe;
// coalescing them into one scroll per tick (instead of one Update+render per
// event) stops the renderer falling behind and the grid "scrolling without
// stopping" long after the gesture ends.
type wheelTickMsg struct{}

// flashExpiry is how long a transient status-bar message stays visible before
// auto-clearing.
const flashExpiry = 5 * time.Second

// hintFlashDuration is how long a pressed hint key stays cell-fg+bold.
const hintFlashDuration = 300 * time.Millisecond

// hintDescDuration is how long a pressed key's description is shown inline on
// the status bar after the key is pressed (a touch longer than the key flash,
// so it can actually be read).
const hintDescDuration = 1500 * time.Millisecond

// queryStackEntry stores navigation state for returning after following a FK
// or drilling in from the relationship explorer.
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

	connList  ConnectionList
	connForm  ConnectionForm
	editor    QueryEditor
	results   ResultsTable // active tab's results (synced on tab switch)
	inspector Inspector
	assistant Assistant

	// Tab management
	resultsTabs         []*ResultsTab // All result tabs
	activeTabID         int           // Currently active tab ID
	nextTabID           int           // Counter for generating unique tab IDs
	tabBar              TabBar        // Tab navigation component
	history             HistoryPanel
	bookmarks           BookmarkPanel
	crossSearch         CrossSearchPanel
	dbPicker            DatabasePicker
	help                HelpPanel
	filterPicker        FilterPicker
	columnPicker        ColumnPicker
	exportPicker        ExportPicker
	exportOverlay       ExportOverlay
	themePicker         ThemePicker
	providerPicker      ProviderPicker
	modelBrowser        ModelBrowser
	providerForm        ProviderForm
	importPrompt        ImportPrompt
	addColumnForm       AddColumnForm
	tableRenameForm     TableRenameForm
	tableDesigner       TableDesigner
	schemaEditor        SchemaEditor
	cellEdit            CellEditPopup
	explainPanel        ExplainPanel
	diffPanel           DiffPanel
	lookupPanel         LookupPanel
	erdPanel            ERDPanel
	chartPanel          ChartPanel
	explorer            RelExplorer
	palette             palette
	ex                  exCmd
	sidebarCursor       int
	sidebarScroll       int              // cached scroll offset of the first visible sidebar item
	sidebarViewAnchored bool             // mouse click froze the view; keyboard nav clears it
	tableRowCounts      map[string]int64 // approximate row counts for sidebar display
	expanded            map[string][]db.Column
	columnCache         map[string][]db.Column
	views               map[string]bool            // view names from Views(); badges views in the sidebar
	pkCache             map[string][]string        // table -> PK columns (AI schema context)
	fkCache             map[string][]db.ForeignKey // table -> FKs (AI schema context)
	schemaNames         []string                   // schemas/namespaces for editor completion
	schemaTableCache    map[string][]string        // schema → tables (cross-schema completion)
	recentTables        []string                   // MRU table names (most recent first); for :recent

	// Fuzzy table search
	sidebarFilter    string
	sidebarFiltering bool

	// Pending vim operator for sidebar (e.g. 'g' waiting for second 'g')
	sidebarPendingG bool
	resultsPendingG bool
	resultsPendingY bool
	resultsPendingD bool // dd double-tap state for row deletion

	// yank holds the last cell copied with yy / :copy. Fill (visual/marked p)
	// prefers this over the system clipboard so yy→mark→p stays reliable when
	// the OS pasteboard is empty, flaky, or overwritten.
	yank string

	// Wheel coalescing: rapid wheel events accumulate here and are applied in
	// a single scroll on wheelTickMsg, so a momentum-scroll flood can't outrun
	// the render loop. wheelAccum is signed (+ = scroll down, - = up).
	wheelAccum       int
	wheelTickPending bool

	// View cache: bubbletea calls View() after every message, so a wheel-event
	// flood would rebuild the whole screen thousands of times even though the
	// coalesced events change nothing on screen. A coalesced wheel event marks
	// the frame view-cached (viewCached=true); View() then returns viewBuf as-is.
	// Every other message resets viewCached (in update), forcing a rebuild.
	// viewBuf is a pointer so the value-receiver View() can populate it across
	// the copy bubbletea makes; NewModel allocates it.
	viewBuf    *string
	viewCached bool

	// Double-click-to-edit: records the time and cell of the most recent
	// left-click in the results panel so a second click on the same cell
	// within doubleClickInterval enters inline edit mode.
	lastResultsClickTime   time.Time
	lastResultsClickCell   cellRef
	lastInspectorClickTime time.Time
	lastInspectorClickCol  int // result column index of last inspector click (-1 = none)
	lastConnFormClickTime  time.Time
	lastConnFormClickField int // field index of last connection-form click (-1 = none)
	lastConnFormWheelTime  time.Time // debounce wheel → one field step per notch
	lastERDClickTime       time.Time
	lastERDClickCard       string // table name of last ERD card click ("" = none)

	// Discard confirmation dialog
	discardConfirm bool

	// Editor maximize toggle (ctrl+w)
	editorMaximized bool
	// sidebarVisible / editorVisible toggle the table list and SQL editor;
	// split sizes are preserved for when they are shown again.
	sidebarVisible bool
	editorVisible  bool
	zenActive      bool
	zenSaved       zenSnapshot
	// editorSplitH is the user-chosen outer height of the editor panel
	// (editor↔results split). 0 means defaultEditorHeight. Honoured when not
	// maximized; clamped by workspaceGeom.
	editorSplitH int
	// sidebarSplitW is the user-chosen outer width of the sidebar. 0 means
	// defaultSidebarWidth; clamped by workspaceGeom.
	sidebarSplitW int
	// rightSlotSplitW is the user-chosen outer width of the right slot
	// (inspector / assistant / docked explorer). 0 means the active panel's
	// default (InspectorWidth / AssistantWidth); clamped by workspaceGeom.
	rightSlotSplitW int
	// splitDragging / sidebarDragging / rightSlotDragging track an in-flight
	// mouse resize of the editor↔results, sidebar↔centre, or centre↔right-slot
	// seam. The Off fields keep the divider stuck under the cursor
	// (msg.Y - ResultsTop / msg.X - SidebarWidth / msg.X - EditorRight).
	splitDragging     bool
	splitDragOff      int
	sidebarDragging   bool
	sidebarDragOff    int
	rightSlotDragging bool
	rightSlotDragOff  int
	// colResizeDragging tracks an in-flight drag of a results header
	// separator (│). StartX/StartW anchor width to the press so the edge
	// tracks the cursor; Col is the column being resized.
	colResizeDragging bool
	colResizeCol      int
	colResizeStartX   int
	colResizeStartW   int

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

	// Connection-deletion confirmation dialog (non-empty name while pending),
	// gated by confirm_destructive.
	deleteConnConfirm string

	// AI-provider-deletion confirmation dialog (non-empty name while pending),
	// gated by confirm_destructive. Stacked over the `M` provider picker.
	deleteProviderConfirm string

	config            *config.Config
	connection        *db.Connection
	tx                db.Tx             // active manual transaction (:begin/:commit/:rollback); nil = autocommit
	txIsolation       db.IsolationLevel // isolation requested for tx (status bar)
	forceReadOnly     bool              // --readonly CLI flag: forces every connection read-only
	sessionStore      *session.Store
	recentStore       *recent.Store
	startupFileLoaded bool    // creel -f: suppress the first session restore so the file wins
	startupCmd        tea.Cmd // creel -database/-c: follow-up cmds after auto-connect (focus, prefetch)
	// reconnect / keep-alive (MySQL + Postgres): background Ping + in-place
	// rebuild when the tunnel or idle session dies, without leaving the workspace.
	keepAliveGen   uint64
	reconnecting   bool
	reconnectRetry bool // re-run lastQuery after a successful reconnect
	// colWidthMem is the in-memory column-width map for the active
	// connection+database (table → column → width). Loaded from / saved with
	// the session so widths survive reconnects. Grow-only floor from content.
	colWidthMem map[string]map[string]int
	// colWidthOverride holds exact widths from < / > resize; wins over auto-fit
	// and colWidthMem so a manual shrink sticks across re-queries.
	colWidthOverride map[string]map[string]int
	// erdPosMem is the in-memory ERD card-position map for the active
	// connection+database (scope → table → x,y). Loaded from / saved with
	// the session so a drag or H/J/K/L nudge survives reopen. Scope is "*"
	// for the whole schema, otherwise the focused table name.
	erdPosMem map[string]map[string]session.ERDPos
	// insertTarget is a shadow results table used while inserting into a
	// table that is not the current grid (explorer "insert related"). The
	// inspector and saveInsert read columns from this instead of m.results.
	insertTarget *ResultsTable
	// restoreExplorerAfterInsert re-opens the docked explorer after an insert
	// that borrowed the right slot for the inspector (save or cancel).
	restoreExplorerAfterInsert bool
	historyStore               *history.Store
	historyNavEntries          []string // cached queries for the current browse session
	historyNavIdx              int      // -1 = not browsing; otherwise index into historyNavEntries (most recent = len-1)
	historyNavSaved            string   // editor content before history browse started
	bookmarkStore              *bookmarks.Store
	connError                  string
	tables                     []string

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
	baseQuery   string            // original query without filters
	filters     []string          // active filter expressions, AND-joined
	queryParams map[string]string // :name → value; expanded before execute

	// Quick sort (single-column, server-side ORDER BY)
	sortCol string // column name, "" = no sort
	sortDir string // "ASC" or "DESC"

	// Foreign-key navigation stack (gb to go back).
	queryStack       []queryStackEntry
	restoreCursor    bool
	restoreCursorRow int
	restoreCursorCol int

	// Client-side regex search (g/ to search, n/N to jump between matches).
	searching   bool
	searchQuery string
	lastSearch  string

	// Backend full-text search (/ on results, LIKE across all columns).
	backendSearching   bool
	backendSearchInput string
	backendSearchTimer *time.Timer

	// hintFlash is the individual key currently flashed cell-fg+bold on the status bar.
	hintFlash   string
	hintFlashAt time.Time

	// vimYank is the shared register for yank/paste between the query editor
	// and the cell-edit popup.
	vimYank string

	// hintDesc is the pressed key's registry description shown briefly next to
	// the hint line. It expires on the next render after hintDescDuration
	// (matching how hintFlash expires), so no timer command is needed.
	hintDesc   string
	hintDescAt time.Time

	// Async query execution state
	queryRunning   bool               // true while a query is in flight
	queryCancel    context.CancelFunc // cancels the running query
	querySpinner   int                // spinner animation frame index
	queryStart     time.Time          // when the current query started (for elapsed display)
	queryCancelled bool               // true if the user cancelled the running query
	queryTimeout   time.Duration      // per-query deadline; 0 = wait indefinitely (esc still cancels)
	settings       config.Settings    // effective app-level settings

	// AI (:ai) state. aiRunning gates esc/ctrl+c cancellation; aiCancel aborts
	// the in-flight model request; aiQuestion is shown in the pending hint so
	// the user remembers what they asked; aiStart drives the elapsed timer so a
	// slow model never looks frozen; aiToPanel routes the result to the
	// assistant panel (true) vs the editor (false); aiMsg is the transient
	// result/error.
	aiRunning  bool
	aiToPanel  bool
	aiCancel   context.CancelFunc
	aiStream   <-chan tea.Msg // streamed chunks arrive here while a panel request is in flight
	aiProvider string         // active AI provider name (set via the picker); overrides config default
	aiQuestion string
	aiStart    time.Time
	aiMsg      string

	// Last failed editor query, for :aifix. Cleared on a successful run or
	// when leaving the connection. query is what the user wrote (not the
	// pagination wrap); err is the driver message.
	lastQueryFailSQL string
	lastQueryFailErr string

	// Last successful EXPLAIN plan, for :aiexplain. Cleared on disconnect.
	lastExplainSQL  string
	lastExplainText string

	// :timing — when on, the status bar shows the last query's elapsed time.
	showTiming       bool
	lastQueryElapsed time.Duration

	// :watch — periodic re-execution of the last query (:watch [n] / :watch off).
	// watchGen is a generation counter: restarting with a new interval or
	// stopping bumps it so a stale tick chain dies instead of doubling the rate.
	watchActive   bool
	watchInterval time.Duration
	watchGen      uint64
	watchMode     string // "tail" for :tail, otherwise "watch" — only affects the indicator/stop message
	// watchPrevRows is the previous result page so a refresh can tint
	// new/changed rows. Nil until the first watch snapshot.
	watchPrevRows [][]string

	// lastChart* remembers the most recent successful chart so a results
	// refresh (:watch, ctrl+r, re-run) can redraw it instead of closing it.
	lastChartSpec chartSpec
	lastChartAll  bool
	lastChartOK   bool
}

// defaultPageSize / defaultQueryTimeout mirror the config-package defaults so
// the UI has a single source of truth (page size is also used by NewResultsTab).
const (
	defaultPageSize     = config.DefaultPageSize
	defaultQueryTimeout = config.DefaultQueryTimeout
)

// NewModel creates a new top-level application model.
func NewModel(cfg *config.Config) Model {
	// Create initial tab
	firstTab := NewResultsTab(0, "New Query")

	settings := cfg.Settings.Effective()

	// Apply the configured theme (falls back to the default when unset or
	// unknown) before any component renders, so the whole UI is themed from
	// the first frame. init() already applied the default; this overrides it.
	applyPalette(paletteForTheme(settings.Theme))

	// Apply the icon set the same way: portable triangles by default, Nerd
	// Font angle glyphs when `icons: nerdfont` is set.
	applyIcons(settings.Icons)

	m := Model{
		state:           stateConnections,
		focus:           FocusConnections,
		config:          cfg,
		settings:        settings,
		editor:          NewQueryEditor(),
		results:         firstTab.Results,
		inspector:       NewInspector(),
		explorer:        NewRelExplorer(),
		assistant:       NewAssistant(),
		connList:        NewConnectionList(),
		history:         NewHistoryPanel(),
		bookmarks:       NewBookmarkPanel(),
		dbPicker:        NewDatabasePicker(),
		help:            NewHelpPanel(),
		filterPicker:    NewFilterPicker(),
		columnPicker:    NewColumnPicker(),
		exportPicker:    NewExportPicker(),
		exportOverlay:   NewExportOverlay(),
		themePicker:     NewThemePicker(),
		providerPicker:  NewProviderPicker(),
		modelBrowser:    NewModelBrowser(),
		providerForm:    NewProviderForm(),
		importPrompt:    NewImportPrompt(),
		addColumnForm:   NewAddColumnForm(),
		tableRenameForm: NewTableRenameForm(),
		tableDesigner:   NewTableDesigner(),
		schemaEditor:    NewSchemaEditor(),
		cellEdit:        NewCellEditPopup(),
		chartPanel:      NewChartPanel(),
		sessionStore:    session.NewStore(historyDir()),
		recentStore:     recent.NewStore(historyDir()),
		historyStore:    history.NewStore(historyDir()),
		bookmarkStore:   bookmarks.NewStore(historyDir()),
		expanded:        make(map[string][]db.Column),
		pageSize:        settings.PageSize,
		queryTimeout:    settings.QueryTimeout.Std(),
		viewBuf:         new(string),
		// Tab management
		resultsTabs: []*ResultsTab{firstTab},
		activeTabID: 0,
		nextTabID:   1,
		tabBar:      NewTabBar(),

		sidebarVisible: true,
		editorVisible:  true,
	}
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	m.editor.BindYank(&m.vimYank)
	m.cellEdit.BindYank(&m.vimYank)
	m.loadConnections()
	if len(m.config.Connections) > 0 {
		m.connList.StartFilter()
		m.selectRecentConnection() // StartFilter resets cursor; re-apply MRU
	}
	return m
}

func (m *Model) loadConnections() {
	recentNames := map[string]bool{}
	if m.recentStore != nil {
		if names, err := m.recentStore.Names(); err == nil {
			for _, n := range names {
				recentNames[n] = true
			}
		}
	}
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
			detail = conn.SSHHost
		}
		entries = append(entries, ConnectionEntry{
			Name:   conn.Name,
			Driver: conn.Driver,
			Detail: detail,
			Group:  conn.Group,
			Recent: recentNames[conn.Name],
		})
	}
	m.connList.SetItems(entries)
	m.selectRecentConnection()
}

// selectRecentConnection moves the picker cursor onto the most recent saved
// connection that still exists. No-op when the MRU is empty or stale.
func (m *Model) selectRecentConnection() {
	if m.recentStore == nil {
		return
	}
	last, err := m.recentStore.Last()
	if err != nil || last == "" {
		return
	}
	m.connList.SelectByName(last)
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
			m.insertTarget = nil
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
		m.insertTarget = nil
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

// refreshSchema reloads table/column metadata and re-runs the last query,
// matching ctrl+r. Shared by the keybinding and the ":refresh" command so the
// two cannot drift. A no-op while edits are pending (a re-run would discard
// them). It is a pointer method so both the value-receiver key dispatch
// (invoked as a statement before return) and the pointer-receiver ex dispatch
// share one implementation.
func (m *Model) refreshSchema() tea.Cmd {
	if m.results.IsEditing() || m.results.HasDirtyCells() || m.inspector.IsInserting() {
		return nil
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
		return tea.Batch(cmd, m.runPageQuery())
	}
	m.schemaMsg = "refreshed schema"
	return cmd
}

// toggleHistory opens/closes the query history panel, loading entries for the
// current connection. Shared by ctrl+y and ":history".
func (m *Model) toggleHistory() {
	if m.connection == nil {
		return
	}
	if m.history.IsVisible() {
		m.history.Toggle()
		return
	}
	if entries, err := m.historyStore.Get(m.connection.Config().Name); err == nil {
		m.history.SetEntries(entries)
	}
	m.history.Toggle()
}

// toggleBookmarks opens/closes the bookmarks panel, loading entries for the
// current connection. Shared by ctrl+g and ":bookmarks".
func (m *Model) toggleBookmarks() {
	if m.connection == nil {
		return
	}
	if m.bookmarks.IsVisible() {
		m.bookmarks.Toggle()
		return
	}
	if entries, err := m.bookmarkStore.Get(m.connection.Config().Name); err == nil {
		m.bookmarks.SetEntries(entries)
	}
	m.bookmarks.Toggle()
}

// paletteJumpSrc collects tables and bookmarks for the jump-anywhere palette.
// Themes are appended inside buildPaletteItems. History stays on Ctrl+Y so the
// palette doesn't drown in recent queries.
func (m Model) paletteJumpSrc() paletteJumpSrc {
	src := paletteJumpSrc{
		Tables: append([]string(nil), m.tables...),
	}
	if m.connection == nil || m.bookmarkStore == nil {
		return src
	}
	name := m.connection.Config().Name
	if entries, err := m.bookmarkStore.Get(name); err == nil {
		for i := len(entries) - 1; i >= 0 && len(src.Bookmarks) < maxPaletteBookmarks; i-- {
			src.Bookmarks = append(src.Bookmarks, entries[i].Query)
		}
	}
	return src
}

// applyPaletteJump runs the action for a confirmed jump-anywhere palette row.
func (m *Model) applyPaletteJump(msg paletteJumpMsg) tea.Cmd {
	switch msg.kind {
	case paletteJumpTable:
		return m.openTable(msg.payload)
	case paletteJumpBookmark:
		m.editor.SetValue(msg.payload)
		m.focus = FocusEditor
		m.applyFocus()
		return m.editor.Focus()
	case paletteJumpTheme:
		return m.exTheme(msg.payload)
	}
	return nil
}

// handleTabKey processes workspace-global g-chord keybindings that work
// from any focused panel (except the editor in insert mode). Returns true if
// the key was consumed.
//
// g t / g T — next / previous tab
// g x       — close tab
// g 1-9     — go to tab N
// g c       — open theme picker (live preview)
// t         — new tab (sidebar, results, tab bar only)
func (m *Model) handleTabKey(msg tea.KeyMsg) bool {
	if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() ||
		m.searching || m.ex.visible || m.backendSearching ||
		m.sidebarFiltering || m.inspector.IsFiltering() ||
		(m.focus == FocusEditor && m.editor.CapturingKeys()) {
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
		case "c":
			// g c — open the theme picker for live-preview theme switching.
			m.clearPendingG()
			m.themePicker.Show(m.settings.Theme)
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
	return m.startupCmd
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
	// Invalidate the view cache by default. A coalesced results wheel event is
	// the only view-neutral message; it re-enables the cache in its handler so
	// the wheel flood doesn't rebuild the screen thousands of times.
	m.viewCached = false
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.updateLayout()
		if m.erdPanel.IsVisible() {
			m.erdPanel.SetSize(m.width, m.height-1)
		}
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

		// While an AI request is in flight (:ai), esc and ctrl+c cancel it.
		// Other keys are swallowed, matching the query model, so a slow model
		// can't race a second request.
		if m.aiRunning {
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "ctrl+c"))) {
				if m.aiCancel != nil {
					m.aiCancel()
					m.aiCancel = nil
				}
				m.aiRunning = false
				m.aiQuestion = ""
				m.aiMsg = "ai: cancelled"
				return m, nil
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			m.beginQuit()
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+q"))):
			m.beginQuit()
			return m, tea.Quit
		}

		// Flash the matching hint group on the status bar (set before
		// dispatch so the value survives the value-receiver copy), and stage
		// the pressed key's description to show inline for a moment.
		if matched := matchHint(m.hintList(), msg.String()); matched != "" {
			m.hintFlash = matched
			m.hintFlashAt = time.Now()
			if d := m.hintDescription(matched); d != "" {
				m.hintDesc = d
				m.hintDescAt = time.Now()
			}
		} else {
			m.hintFlash = ""
			m.hintDesc = ""
		}

		if m.state == stateConnections {
			return m.updateConnections(msg)
		}
		if m.state == stateAddConnection {
			return m.updateAddConnection(msg)
		}
		nm, ncmd := m.updateWorkspace(msg)
		// Docked explorer is cursor-driven: if the results cursor landed on a
		// different row, re-root the tree. Cheap anchor check; reloads only on a
		// real change.
		if mm, ok := nm.(Model); ok {
			if rcmd := (&mm).maybeReloadDockedExplorer(); rcmd != nil {
				ncmd = tea.Batch(ncmd, rcmd)
			}
			return mm, ncmd
		}
		return nm, ncmd

	case tea.MouseMsg:
		// Help overlay is modal: the mouse wheel scrolls it; other mouse
		// events are ignored while it's open.
		if m.help.IsVisible() {
			return m.handleHelpMouse(msg)
		}
		// ERD panel is a full-screen overlay: it owns all mouse events while
		// visible (and stops them leaking through to the workspace behind it).
		if m.erdPanel.IsVisible() {
			// Size the persistent panel (View only sizes a discarded copy), so
			// the click hit-test reads the real viewport, not a zero-sized one.
			m.erdPanel.SetSize(m.width, m.height-1)
			return m.handleERDMouse(msg)
		}
		if m.state == stateConnections {
			return m.handleConnectionsMouse(msg)
		}
		if m.state == stateAddConnection {
			return m.handleConnectionFormMouse(msg)
		}
		if m.state == stateWorkspace {
			return m.handleWorkspaceMouse(msg)
		}
		return m, nil

	case paletteJumpMsg:
		return m, m.applyPaletteJump(msg)

	case queryExecutedMsg:
		m.queryRunning = false
		m.queryCancel = nil

		// Silently discard results from queries that were superseded (not
		// user-cancelled) — the newer query's result will replace them.
		if msg.cancelled && !m.queryCancelled {
			return m, nil
		}

		// Capture elapsed for :timing (after the superseded-query discard, so a
		// stale cancelled query doesn't overwrite the displayed duration).
		m.lastQueryElapsed = time.Since(m.queryStart)

		m.layoutWorkspace()

		// User-cancelled queries show a message but keep existing results.
		if m.queryCancelled {
			m.results.SetError("Query cancelled")
			m.queryCancelled = false
			return m, nil
		}

		// A query that exceeded the deadline gets a clear, distinct message
		// (the raw driver error is opaque). Existing results are kept.
		if msg.timedOut {
			m.results.SetError(fmt.Sprintf("Query timed out (limit %s) — press esc in-flight to cancel sooner", m.queryTimeout))
			return m, nil
		}

		// Record to history
		if m.connection != nil && m.historyStore != nil {
			m.historyStore.Record(m.connection.Config().Name, msg.query, m.lastQueryElapsed, msg.err == nil)
		}
		var cmd tea.Cmd
		if msg.err != nil {
			if ok, rcmd := m.maybeReconnectOnError(msg.err, true); ok {
				return m, rcmd
			}
			m.recordQueryFailure(msg.query, msg.err)
			m.results.SetError(msg.err.Error())
			if m.restoreCursor {
				m.restoreCursor = false
			}
			m.maybeJumpToQueryError(msg.err, msg.query, msg.execQuery)
		} else {
			m.clearQueryFailure()
			cols := make([]string, len(msg.result.Columns))
			for i, c := range msg.result.Columns {
				cols[i] = c.Name
			}

			// Check for "has next page" — we fetched pageSize+1 rows
			rows := msg.result.Rows
			blobs := msg.result.Blobs
			hasNext := false
			if len(rows) > msg.pageSize {
				hasNext = true
				rows = rows[:msg.pageSize]
				blobs = db.TrimBlobs(blobs, msg.pageSize)
			}

			m.results.SetResult(cols, rows, msg.result.Message)
			m.results.SetBlobs(blobs)

			// Watch/tail: tint rows whose content wasn't on the previous page.
			if m.watchActive {
				if m.watchPrevRows != nil {
					m.results.SetWatchDelta(computeWatchDelta(m.watchPrevRows, rows))
				}
				m.watchPrevRows = cloneResultRows(rows)
			} else {
				m.watchPrevRows = nil
			}

			// Keep an open chart in sync with the new page (or re-fetch bang
			// charts). Manual queries that weren't charting still close it.
			redrawChart := m.chartPanel.IsVisible() && m.lastChartOK
			var chartCmd tea.Cmd
			if redrawChart {
				chartCmd = m.redrawLastChart(false)
			} else {
				m.chartPanel.Hide()
			}

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
			if chartCmd != nil {
				cmd = tea.Batch(cmd, chartCmd)
			}

			// Enable inline editing and foreign-key navigation for simple table SELECTs.
			m.detectResultMetadata(msg.query)
			// Apply + grow remembered column widths for the backing table so
			// paging onto a short page (or re-querying) does not shrink columns.
			m.syncColWidthMemory()
			m.inspector.Reset()
			m.insertTarget = nil
			if m.restoreCursor {
				m.results.SetCursor(m.restoreCursorRow, m.restoreCursorCol)
				m.restoreCursor = false
			}
			// If the relationship explorer is open, refresh it for the new
			// focused row. This covers drill-in (Enter), back, and any
			// manual query — the panel always reflects the current location.
			if m.explorer.IsVisible() {
				if cmd == nil {
					cmd = m.loadExplorer()
				} else {
					cmd = tea.Batch(cmd, m.loadExplorer())
				}
			}
		}
		return m, cmd

	case spinnerTickMsg:
		if !m.queryRunning && !m.aiRunning && !m.reconnecting {
			return m, nil
		}
		m.querySpinner = (m.querySpinner + 1) % len(spinnerFrames)
		return m, spinnerTick()

	case saveResultMsg:
		if msg.err != nil {
			if ok, rcmd := m.maybeReconnectOnError(msg.err, false); ok {
				return m, rcmd
			}
			m.results.SetSaveError(msg.err.Error())
		} else {
			m.results.ApplySavedEdits()
		}
		return m, nil

	case insertResultMsg:
		if msg.err != nil {
			if ok, rcmd := m.maybeReconnectOnError(msg.err, false); ok {
				return m, rcmd
			}
			m.results.SetSaveError(msg.err.Error())
		} else {
			m.inspector.CancelInsert()
			m.results.ConfirmSaved()
			m.insertTarget = nil
			if m.restoreExplorerAfterInsert {
				m.restoreExplorerPanel()
				m.explorer.markLoading()
				return m, m.loadExplorer()
			}
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
				delete(m.pkCache, msg.table)
				delete(m.fkCache, msg.table)
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
		m.pkCache = msg.pks
		m.fkCache = msg.fks
		m.refreshCompletionCandidates()
		return m, nil

	case schemaTablesLoadedMsg:
		m.schemaTableCache = msg.cache
		m.refreshCompletionCandidates()
		return m, m.ensureSchemaCompletionFetch()

	case qualifiedTableSchemaMsg:
		if msg.err == nil && msg.key != "" {
			if m.columnCache == nil {
				m.columnCache = make(map[string][]db.Column)
			}
			m.columnCache[msg.key] = msg.cols
			m.refreshCompletionCandidates()
			if m.editor.CompletionVisible() {
				m.editor.StartCompletion() // refilter with new columns
			}
		}
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
			m.connForm.SetTestResult("✗ "+db.FormatConnectError(msg.driver, msg.err), msg.err)
		} else {
			m.connForm.SetTestResult(fmt.Sprintf("✓ Connected (%s)", msg.driver), nil)
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

	case wheelTickMsg:
		// Apply the accumulated wheel delta in one scroll and release the
		// pending flag so the next wheel event can arm a fresh tick.
		m.wheelTickPending = false
		delta := m.wheelAccum
		m.wheelAccum = 0
		if delta != 0 {
			m.results.ScrollBy(delta)
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

	case chartReadyMsg:
		m.applyChartReady(msg)
		m.layoutWorkspace()
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

	case watchTickMsg:
		return m.handleWatchTick(msg)

	case keepAliveTickMsg:
		return m.handleKeepAliveTick(msg)

	case keepAliveFailMsg:
		return m.handleKeepAliveFail(msg)

	case reconnectResultMsg:
		return m.handleReconnectResult(msg)

	case explainResultMsg:
		if msg.err != nil {
			if msg.forAI {
				m.aiMsg = fmt.Sprintf("EXPLAIN error: %v", msg.err)
			} else {
				m.statsMsg = fmt.Sprintf("EXPLAIN error: %v", msg.err)
			}
			return m, nil
		}
		driver := db.DriverSQLite
		if m.connection != nil {
			driver = m.connection.Config().Driver
		}
		planText := formatExplainPlan(msg.result, driver)
		if msg.query != "" && planText != "" {
			m.lastExplainSQL = msg.query
			m.lastExplainText = planText
		}
		if msg.forAI {
			return m, m.dispatchAIExplain(msg.query, planText, msg.focus)
		}
		m.explainPanel.Show(msg.result, driver)
		return m, nil
	case lookupResultMsg:
		if msg.err != nil {
			m.schemaMsg = fmt.Sprintf("lookup failed: %v", msg.err)
			return m, nil
		}
		m.lookupPanel.Show(msg.title, msg.result, msg.jumps)
		return m, nil
	case explorerLoadedMsg:
		// Only apply if the panel is still open (the user may have closed it
		// while a load was in flight).
		if !m.explorer.IsVisible() {
			return m, nil
		}
		switch {
		case msg.err != nil:
			m.explorer.applyRootError(msg.depth, msg.err)
		case msg.root == nil:
			m.explorer.applyEmpty(msg.depth, msg.emptyMsg)
		default:
			m.explorer.applyRoot(msg.root, msg.depth)
		}
		// Record the row this tree is now rooted at, so the docked panel can tell
		// a real cursor move from a redundant reload.
		m.explorer.anchor = m.explorerAnchor()
		return m, nil
	case explorerChildrenMsg:
		if !m.explorer.IsVisible() || msg.parent == nil {
			return m, nil
		}
		switch {
		case msg.err != nil:
			m.explorer.applyChildrenError(msg.parent, msg.err)
		case msg.fold:
			m.explorer.applyFold(msg.parent)
		default:
			m.explorer.applyChildren(msg.parent, msg.children)
		}
		return m, nil
	case aiStreamChunkMsg:
		// A token batch from the streamed reply: grow the live preview in the
		// panel (content + any reasoning), then keep draining.
		m.assistant.AppendStreamDelta(msg.content, msg.reasoning)
		if m.aiStream != nil {
			return m, waitAIStream(m.aiStream)
		}
		return m, nil

	case aiResultMsg:
		// An AI request finished. Clear the in-flight state first so the
		// pending hint and esc-cancel gating are gone even if we error.
		m.aiRunning = false
		m.aiCancel = nil
		m.aiStream = nil
		q := m.aiQuestion
		m.aiQuestion = ""
		if msg.err != nil {
			switch {
			case msg.toPanel:
				m.assistant.SetPending(false)
				m.assistant.AppendError(errString(msg.err) + aiAuthHint(msg.err))
			default:
				m.aiMsg = fmt.Sprintf("ai failed: %v%s", msg.err, aiAuthHint(msg.err))
			}
			return m, nil
		}
		switch {
		case msg.toPanel:
			m.assistant.SetPending(false)
			if q == aiExplainQuestion {
				// Prose explanation — keep the full reply; don't offer Apply SQL.
				m.assistant.AppendAssistant(strings.TrimSpace(msg.reply), "")
			} else {
				m.assistant.AppendAssistant(summaryFor(msg), msg.sql)
			}
			return m, nil
		default:
			// Drop the generated SQL into the editor for review. The user runs
			// it explicitly (ctrl+e) — neither :ai nor the panel auto-executes,
			// so a misunderstood request can't mutate data behind the user's back.
			m.editor.SetValue(msg.sql)
			m.focus = FocusEditor
			m.applyFocus()
			if q == aiFixQuestion {
				m.aiMsg = "AI fixed query — review then ctrl+e to run"
			} else {
				m.aiMsg = fmt.Sprintf("AI generated query for %q — review then ctrl+e to run", q)
			}
			return m, nil
		}

	case submitAssistantMsg:
		// The panel submitted a question. Record it in the transcript
		// immediately (so the user sees it), mark pending, and dispatch.
		if m.aiRunning {
			return m, nil // one request at a time
		}
		m.assistant.AppendUser(msg.question)
		m.assistant.SetPending(true)
		m.assistant.CancelCompose() // back to browse: watch the stream / apply SQL / ask a follow-up with `i`
		return m, m.sendAssistant(msg.question)

	case applyAssistantSQLMsg:
		// Apply the latest assistant SQL to the editor for review/run.
		sql := m.assistant.LatestSQL()
		if sql == "" {
			m.aiMsg = "no SQL to apply yet"
			return m, nil
		}
		m.editor.SetValue(sql)
		m.focus = FocusEditor
		m.applyFocus()
		m.aiMsg = "applied AI query — ctrl+e to run"
		return m, nil

	case closeAssistantMsg:
		m.assistant.Hide()
		if m.focus == FocusAssistant {
			m.focus = FocusResults
			m.applyFocus()
		}
		m.layoutWorkspace()
		return m, nil

	case openProviderPickerMsg:
		// Always open the picker, even with no providers configured: that is
		// exactly the state where the user wants to add one (n), and bailing
		// out here would make the form unreachable. The empty picker renders a
		// "press n to add" placeholder.
		m.providerPicker.Show(m.config.AI.Providers, m.effectiveProviderName())
		return m, nil

	case openModelBrowserMsg:
		// `m` browses the models for the active provider. With no provider
		// configured (env-only mode) there is no /models endpoint to query.
		p, ok := m.activeProvider()
		if !ok {
			m.aiMsg = "configure an ai: provider in ~/.config/creel/config.yaml to browse models"
			return m, nil
		}
		m.modelBrowser.Show(p.Name, p.Model)
		return m, m.fetchModelsCmd()

	case fetchModelsMsg:
		// Populate the browser, or surface the fetch failure inline. The
		// browser is only open if `m` was just pressed (esc / a successful
		// pick closes it), so a stray msg with no visible browser is ignored.
		if !m.modelBrowser.IsVisible() {
			return m, nil
		}
		if msg.err != nil {
			m.modelBrowser.SetError(errString(msg.err))
			return m, nil
		}
		if p, ok := m.activeProvider(); ok {
			m.modelBrowser.SetModels(msg.models, p.Model)
		} else {
			m.modelBrowser.SetModels(msg.models, "")
		}
		if len(msg.models) == 0 {
			m.modelBrowser.SetError("provider returned no models")
		}
		return m, nil

	case openProviderFormAddMsg:
		// `n` from the `M` picker: open the provider form in add mode. The
		// picker is hidden (the form returns to it on esc/save).
		m.providerPicker.Hide()
		m.providerForm.Show()
		iw, _ := popupContentSize(m.height)
		m.providerForm.SetSize(iw)
		return m, nil

	case openProviderFormEditMsg:
		// `e` from the `M` picker: open the form pre-filled from the selected
		// provider. A missing name (empty picker) is a no-op.
		if msg.name == "" {
			return m, nil
		}
		p := m.config.GetAIProvider(msg.name)
		if p == nil {
			return m, nil
		}
		m.providerPicker.Hide()
		m.providerForm.ShowEdit(*p)
		iw, _ := popupContentSize(m.height)
		m.providerForm.SetSize(iw)
		return m, nil

	case providerTestResultMsg:
		// ctrl+t from the provider form: route the /models probe result back to
		// the form (field tinting + message). A stray msg with no visible form
		// is ignored.
		if !m.providerForm.IsVisible() {
			return m, nil
		}
		if msg.err != nil {
			m.providerForm.SetTestResult("✗ "+msg.err.Error()+aiAuthHint(msg.err), msg.err)
		} else {
			m.providerForm.SetTestResult("✓ reachable — key and endpoint valid", nil)
		}
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
			return m, tea.Batch(cmd, m.ensureSchemaCompletionFetch())
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
	// Help overlay is modal: scroll/tab keys navigate it, any other key
	// (incl. esc/?) closes it.
	if m.help.IsVisible() {
		if m.help.HandleKey(msg) {
			return m, nil
		}
		m.help.Hide()
		return m, nil
	}

	// Connection-deletion confirmation is modal — intercept all keys while the
	// y/n prompt is up. y/enter runs the delete (and its keychain purge); n/esc
	// cancels, leaving the selection untouched.
	if m.deleteConnConfirm != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			name := m.deleteConnConfirm
			m.deleteConnConfirm = ""
			return m.execDeleteConnection(name)
		case "n", "N", "esc", "ctrl+c":
			m.deleteConnConfirm = ""
			return m, nil
		}
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
		if ch, ok := keyFilterChar(msg); ok {
			m.connList.FilterAddChar(ch)
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "enter":
		return m, m.connectToDB()
	case "[", "left", "h":
		if m.connList.hasGroups() {
			m.connList.MoveGroupTab(-1)
			return m, nil
		}
	case "]", "right", "l":
		if m.connList.hasGroups() {
			m.connList.MoveGroupTab(1)
			return m, nil
		}
	case "?":
		m.help.Show()
		return m, nil
	case "n":
		m.state = stateAddConnection
		m.connForm = NewConnectionForm()
		m.connForm.setDriverField(m.settings.DefaultDriver)
		iw, ch := popupContentSize(m.height)
		m.connForm.SetSize(iw, ch)
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
		m.beginQuit()
		return m, tea.Quit
	case "up", "k":
		m.connList.MoveCursor(-1)
		return m, nil
	case "down", "j":
		m.connList.MoveCursor(1)
		return m, nil
	case "G":
		m.connList.SetCursor(m.connList.lastConnRow())
		return m, nil
	case "g":
		m.connList.SetCursor(m.connList.firstConnRow())
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
	iw, ch := popupContentSize(m.height)
	m.connForm.SetSize(iw, ch)
	cmd := m.connForm.Focus()
	return m, cmd
}

func (m Model) deleteSelectedConnection() (tea.Model, tea.Cmd) {
	name := m.connList.SelectedName()
	if name == "" {
		return m, nil
	}
	if m.confirmDestructive() {
		m.deleteConnConfirm = name
		return m, nil
	}
	return m.execDeleteConnection(name)
}

// execDeleteConnection removes the named connection and its keychain secrets.
// Shared by the gated (y-confirmed) and ungated (confirm_destructive: false)
// paths so the confirmed action is identical either way.
func (m Model) execDeleteConnection(name string) (tea.Model, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	// Best-effort purge of any keychain secrets for this connection. A missing
	// key (the connection never used the keychain) is not an error.
	_ = secrets.DeleteAll(name)
	if m.recentStore != nil {
		_ = m.recentStore.Remove(name)
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
	// ctrl+t tests the connection from any mode (disabled while a test runs).
	if msg.String() == "ctrl+t" {
		if m.connForm.testing {
			return m, nil
		}
		return m, m.testConnection()
	}

	// Submit (enter) and cancel (esc) only fire from normal mode. In insert
	// mode they fall through to the form, which exits insert instead.
	if !m.connForm.editing {
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
				m.config.RemoveConnection(m.connForm.editName)
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
// The secret fields the form exposes (password, ssh_password, ssh_passphrase)
// are managed here. Each is migrated to the OS keychain when mode is
// "keychain", replacing the plaintext value in the config with an opaque
// reference. Empty values and existing references are left untouched.
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
		{secrets.FieldSSHPassphrase, cfg.SSHPassphrase},
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
		case secrets.FieldSSHPassphrase:
			cfg.SSHPassphrase = ref
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

	// Help overlay is modal: scroll/tab keys navigate it, any other key
	// (incl. esc/?) closes it.
	if m.help.IsVisible() {
		if m.help.HandleKey(msg) {
			return m, nil
		}
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
		if ch, ok := keyFilterChar(msg); ok {
			m.dropDBInput += ch
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
		if ch, ok := keyFilterChar(msg); ok {
			m.createDBInput += ch
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
			if ch, ok := keyFilterChar(msg); ok {
				m.dbPicker.FilterAddChar(ch)
				return m, nil
			}
			return m, nil
		}

		// Normal mode: single-letter commands.
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.dbPicker.MustChoose() {
				m.rollbackTxn()
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
				if m.confirmDestructive() {
					m.dropDBConfirm = name
					m.dropDBInput = ""
					return m, nil
				}
				return m, m.execDropDatabase(name)
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
			// Prefer accepting the path-completion dropdown (mirrors Tab / ex-line
			// Enter) over submitting the import.
			if m.importPrompt.AcceptPathCompletion() {
				return m, nil
			}
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

	// Export dialog (g X) is modal — intercept all keys.
	if m.exportOverlay.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.exportOverlay.Hide()
			return m, nil
		case "enter":
			format, cols, scope := m.exportOverlay.Commit()
			return m, m.exportResults(format, cols, scope)
		case " ":
			m.exportOverlay.Activate()
			return m, nil
		case "a":
			m.exportOverlay.SelectAllCols()
			return m, nil
		case "n":
			m.exportOverlay.SelectNoneCols()
			return m, nil
		case "up", "k":
			m.exportOverlay.CursorUp()
			return m, nil
		case "down", "j":
			m.exportOverlay.CursorDown()
			return m, nil
		}
		return m, nil
	}

	// Theme picker (g c) is modal — intercept all keys. Moving the cursor
	// live-previews the theme (the picker applies the palette itself); enter
	// persists the choice to the config, esc reverts to the open-time theme.
	// Provider picker (M from the assistant panel) is modal — intercept all
	// keys. j/k or up/down moves the cursor, enter commits the choice (and
	// persists it as the config's active provider), esc cancels.
	// Provider-deletion confirmation is stacked over the picker — it must be
	// checked first so y/enter run the delete and n/esc cancel while the prompt
	// is up (the picker swallows keys below this).
	if m.deleteProviderConfirm != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			name := m.deleteProviderConfirm
			m.deleteProviderConfirm = ""
			return m, m.deleteProvider(name)
		case "n", "N", "esc", "ctrl+c":
			m.deleteProviderConfirm = ""
			return m, nil
		}
		return m, nil
	}
	if m.providerPicker.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.providerPicker.Hide()
			return m, nil
		case "enter":
			name := m.providerPicker.Selected()
			m.providerPicker.Hide()
			if name == "" {
				return m, nil
			}
			m.aiProvider = name
			m.config.AI.Default = name
			_ = m.config.Save()
			if p, ok := m.activeProvider(); ok && p.Model != "" {
				m.aiMsg = "provider: " + name + " (" + p.Model + ")"
			} else {
				m.aiMsg = "provider: " + name
			}
			return m, nil
		case "up", "k":
			m.providerPicker.Up()
			return m, nil
		case "down", "j":
			m.providerPicker.Down()
			return m, nil
		case "n":
			// New provider: open the add/edit form over the workspace.
			return m, func() tea.Msg { return openProviderFormAddMsg{} }
		case "e":
			return m, func() tea.Msg { return openProviderFormEditMsg{name: m.providerPicker.Selected()} }
		case "d":
			return m, m.deleteSelectedProvider()
		}
		return m, nil // swallow other keys while open
	}

	// Model browser (m from the assistant panel) is modal — intercept all keys.
	// j/k or up/down moves the cursor, enter commits the chosen model to the
	// active provider's config (and persists it), esc cancels. While the list
	// is still loading, navigation and enter are no-ops (Selected returns "").
	if m.modelBrowser.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.modelBrowser.Hide()
			return m, nil
		case "enter":
			sel := m.modelBrowser.Selected()
			if sel == "" {
				return m, nil // still loading or errored
			}
			m.modelBrowser.Hide()
			// Persist the chosen model to the active provider's `model:`.
			if name := m.modelBrowser.Provider(); name != "" {
				for i := range m.config.AI.Providers {
					if m.config.AI.Providers[i].Name == name {
						m.config.AI.Providers[i].Model = sel
						break
					}
				}
				_ = m.config.Save()
			}
			m.aiMsg = "model: " + sel
			return m, nil
		case "up", "k":
			m.modelBrowser.Up()
			return m, nil
		case "down", "j":
			m.modelBrowser.Down()
			return m, nil
		}
		return m, nil // swallow other keys while open
	}

	// Provider form (n/e from the `M` picker) is modal — intercept all keys.
	// It shares the connection form's vim model: ctrl+t probes /models, enter
	// saves (persisting the key to the keychain when requested), esc returns to
	// the provider picker. Insert-mode keys (esc/enter to commit, then back to
	// normal) are handled inside the form.
	if m.providerForm.IsVisible() {
		// ctrl+t tests from any mode (disabled while a probe is in flight).
		if msg.String() == "ctrl+t" {
			if m.providerForm.testing {
				return m, nil
			}
			return m, m.testProvider()
		}
		if !m.providerForm.editing {
			switch msg.String() {
			case "esc":
				m.providerForm.Hide()
				m.providerPicker.Show(m.config.AI.Providers, m.effectiveProviderName())
				return m, nil
			case "enter":
				return m, m.saveProviderForm()
			}
		}
		var cmd tea.Cmd
		m.providerForm, cmd = m.providerForm.Update(msg)
		return m, cmd
	}

	// Theme picker (g c) is modal — intercept all keys. Arrow keys move the
	// cursor (live-previewing the theme); every other key filters the list by
	// display name. enter persists the choice to the config (a no-op if the
	// filter has no matches), esc reverts to the open-time theme.
	if m.themePicker.IsVisible() {
		switch msg.String() {
		case "esc", "ctrl+c":
			applyPalette(paletteForTheme(m.themePicker.AppliedAtOpen()))
			m.themePicker.Hide()
			return m, nil
		case "enter":
			name := m.themePicker.Commit()
			if name == "" {
				return m, nil // no match — keep the picker open
			}
			m.settings.Theme = name
			m.config.Settings.Theme = name
			_ = m.config.Save()
			return m, nil
		case "up":
			m.themePicker.Up()
			return m, nil
		case "down":
			m.themePicker.Down()
			return m, nil
		case "backspace":
			m.themePicker.FilterBackspace()
			return m, nil
		}
		if ch, ok := keyFilterChar(msg); ok {
			m.themePicker.FilterAddChar(ch)
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
				return m, m.execDropTable(table)
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
		if ch, ok := keyFilterChar(msg); ok {
			m.dropTableInput += ch
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

	// Cell-edit popup is modal — vim editing; ctrl+s stages; esc insert→normal→close.
	if m.cellEdit.IsVisible() {
		switch msg.String() {
		case "ctrl+s":
			if m.cellEdit.IsReadOnly() {
				return m, nil
			}
			val := m.cellEdit.Value()
			if compacted, ok := compactJSON(val); ok {
				val = compacted
			}
			orig := m.results.RawRowValue(m.cellEdit.Row(), m.cellEdit.Col())
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
			if handled, close := m.cellEdit.ConsumeEsc(); handled {
				if close {
					m.cellEdit.Hide()
				}
				return m, nil
			}
		case "q":
			if !m.cellEdit.IsReadOnly() && m.cellEdit.VimMode() == VimNormal {
				m.cellEdit.Hide()
				return m, nil
			}
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

	// Diff panel is modal — j/k scroll, a toggles all/changes, esc/q close.
	if m.diffPanel.IsVisible() {
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.diffPanel.Hide()
			return m, nil
		}
		m.diffPanel = m.diffPanel.Update(msg)
		return m, nil
	}

	// Chart panel replaces the results grid — j/k scroll, esc/q close.
	// Keep : / ? / ctrl+p available so :watch (and help/palette) work without
	// closing the chart first; once the ex line or palette is open they own keys.
	if m.chartPanel.IsVisible() {
		if m.ex.visible {
			return m.handleExKey(msg)
		}
		if m.palette.visible {
			var cmd tea.Cmd
			m.palette, cmd = m.palette.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case ":":
			m.ex.Open()
			m.layoutWorkspace()
			return m, nil
		case "?":
			m.help.Show()
			return m, nil
		case "ctrl+p":
			m.palette.Open(m.paletteJumpSrc())
			return m, nil
		case "esc", "q", "ctrl+c":
			m.chartPanel.Hide()
			return m, nil
		case "enter":
			return m, m.drillChartBar()
		}
		m.chartPanel = m.chartPanel.Update(msg)
		return m, nil
	}

	// Lookup panel is modal — j/k scroll, enter jumps, esc/q close.
	if m.lookupPanel.IsVisible() {
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.lookupPanel.Hide()
			return m, nil
		case "enter":
			if jump := m.lookupPanel.SelectedJump(); jump != "" {
				m.lookupPanel.Hide()
				m.syncSidebarCursorToTable(jump)
				return m, m.openTable(jump)
			}
			return m, nil
		}
		m.lookupPanel = m.lookupPanel.Update(msg)
		return m, nil
	}

	// ERD panel is modal — j/k scroll, y copy, s save, esc/q close.
	if m.erdPanel.IsVisible() {
		// A mouse drag is modal: esc cancels it (restoring the card), and every
		// other key is swallowed so keyboard focus can't race the in-flight move.
		if m.erdPanel.dragCard != "" {
			if msg.String() == "esc" {
				m.erdPanel = m.erdPanel.dragCancel()
			}
			return m, nil
		}
		// While the panel's "/" jump bar is open it consumes all keys
		// (including esc/enter, which the app would otherwise grab).
		if !m.erdPanel.searching {
			switch msg.String() {
			case "esc":
				// Esc steps back: clear an active FK path before closing the panel.
				if m.erdPanel.pathFrom != "" || len(m.erdPanel.pathCards) > 0 {
					m.erdPanel.zPrefix = false
					m.erdPanel = m.erdPanel.clearPath()
					return m, nil
				}
				m.hideERD()
				return m, nil
			case "q", "ctrl+c":
				m.hideERD()
				return m, nil
			case "y", "Y":
				m.erdPanel.zPrefix = false // these app-level actions aren't fold
				_ = clipboard.WriteAll(joinERDLines(m.erdPanel.MermaidLines()))
				m.schemaMsg = "erd copied to clipboard"
				return m, nil
			case "s":
				m.erdPanel.zPrefix = false // family second keys, so drop a pending `z`
				m.saveERDToFile("erd.mmd", m.erdPanel.MermaidLines())
				return m, nil
			case "enter":
				// Browse the focused card (SELECT *); neighbourhood drill is `f`.
				nm, cmd := m.erdEnter()
				return nm, cmd
			case "f":
				nm, cmd := m.erdDrillIn()
				return nm, cmd
			case "i", "I":
				m.erdPanel.zPrefix = false
				if len(m.erdPanel.pathCards) >= 2 {
					return m.erdInsertPathJoin()
				}
				m.schemaMsg = "trace an FK path first (p)"
				return m, nil
			}
		}
		m.erdPanel = m.erdPanel.Update(msg)
		m.snapshotERDPositions()
		return m, nil
	}

	// Command palette is modal — intercept all keys when visible.
	if m.palette.visible {
		var cmd tea.Cmd
		m.palette, cmd = m.palette.Update(msg)
		return m, cmd
	}

	// Ex command line (":") is modal — intercept all keys when visible.
	if m.ex.visible {
		return m.handleExKey(msg)
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
	case ":":
		// Ex command line — not while a text-input / editing mode is active.
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() || m.inspector.IsFiltering() ||
			m.sidebarFiltering || m.searching || m.backendSearching ||
			(m.focus == FocusEditor && m.editor.CapturingKeys()) {
			break
		}
		m.ex.Open()
		return m, nil
	case "ctrl+p":
		// Jump-anywhere palette — but not while the editor is in insert mode,
		// where ctrl+p navigates the completion popup.
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m.palette.Open(m.paletteJumpSrc())
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
			m.focus == FocusAssistant ||
			(m.focus == FocusEditor && m.editor.CapturingKeys()) {
			break
		}
		m.beginQuit()
		return m, tea.Quit
	case "ctrl+y":
		m.toggleHistory()
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
		cmd := m.refreshSchema()
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
		m.toggleBookmarks()
		return m, nil
	case "B":
		// Bookmark the current editor query (shared with :bookmark). Don't
		// intercept it while typing in the editor's insert mode.
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m.bookmarkCurrentQuery()
		return m, nil
	case "ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l":
		// Directional panel navigation — not while editing or in insert mode.
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break // let it fall through to the editor
		}
		m = m.moveFocus(msg.String())
		return m, nil
	case "alt+h", "alt+j", "alt+k", "alt+l",
		"alt+ctrl+h", "alt+ctrl+j", "alt+ctrl+k", "alt+ctrl+l":
		// Nudge the adjacent seam in that direction. alt+ctrl+… is the
		// Bubble Tea encoding of ctrl+alt+letter (useful when plain alt is
		// claimed by the window manager). Same guards as focus movement so
		// typing in insert mode is unaffected.
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m = m.resizePane(msg.String())
		return m, nil
	case "alt+b":
		if m.state != stateWorkspace {
			return m, nil
		}
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m.toggleSidebar()
		return m, nil
	case "alt+e":
		if m.state != stateWorkspace {
			return m, nil
		}
		if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
			return m, nil
		}
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			break
		}
		m.toggleEditor()
		return m, nil
	case "ctrl+o":
		m.toggleInspector()
		return m, nil
	case "ctrl+f":
		m.toggleAssistant()
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
		return m, m.showConnectionList()
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
			return m, m.maybeRestoreExplorerAfterInsert()
		}
		// In insert mode, esc goes to the editor for vim mode switching.
		// In normal mode, esc is a no-op (or could blur the editor).
		if m.focus == FocusEditor && m.editor.CapturingKeys() {
			m.editor, cmd = m.editor.Update(msg)
			return m, cmd
		}
		// The assistant handles esc itself: leave compose (insert) mode, or
		// close the panel in browse mode. Break out of this global switch so
		// it reaches the focus routing (the panel's HandleKey).
		if m.focus == FocusAssistant {
			break
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
		case "s":
			m.history.ToggleSort()
			return m, nil
		case "D":
			if m.confirmDestructive() {
				m.clearHistoryConfirm = true
				return m, nil
			}
			if m.connection != nil && m.historyStore != nil {
				m.historyStore.Clear(m.connection.Config().Name)
			}
			m.history.SetEntries(nil)
			m.history.StartFilter()
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
		if ch, ok := keyFilterChar(msg); ok {
			m.history.filter += ch
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
			if m.confirmDestructive() {
				m.clearBookmarksConfirm = true
				return m, nil
			}
			if m.connection != nil && m.bookmarkStore != nil {
				m.bookmarkStore.Clear(m.connection.Config().Name)
			}
			m.bookmarks.SetEntries(nil)
			m.bookmarks.StartFilter()
			return m, nil
		}
		// Printable characters extend the filter.
		if ch, ok := keyFilterChar(msg); ok {
			m.bookmarks.filter += ch
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
		if ch, ok := keyFilterChar(msg); ok {
			m.crossSearch.AddQueryChar(ch)
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
		return m, tea.Batch(cmd, m.ensureSchemaCompletionFetch())
	case FocusResults:
		// Clear dd pending state on any non-'d' key.
		if msg.String() != "d" {
			m.resultsPendingD = false
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
				query := m.searchQuery
				m.searching = false
				m.searchQuery = ""
				m.applySearch(query)
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
			if ch, ok := keyFilterChar(msg); ok {
				m.searchQuery += ch
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
			if ch, ok := keyFilterChar(msg); ok {
				m.backendSearchInput += ch
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
			case "p":
				// Fill current column across the visual range (dirty only).
				m.fillVisualRange()
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

		// g X — open the export dialog (format + columns + scope). Capital X
		// avoids the g x (close tab) tab-management prefix.
		if msg.String() == "X" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			if m.results.NumRows() > 0 {
				m.exportOverlay.Show(
					m.results.ColumnNames(),
					m.results.SourceTable() != "",
					m.results.MarkCount(),
					m.results.NumRows(),
					m.totalRows, m.totalRowsSet,
				)
			}
			return m, nil
		}

		// g r — relationship explorer: toggles the docked panel, a navigable
		// object-graph view of the focused row's inbound + outbound FK edges
		// with live counts that re-roots as the cursor moves. Same as `:explore`.
		if msg.String() == "r" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			return m, m.openDockedExplorer()
		}

		// g R — static ERD: a Mermaid erDiagram of the current table's FK
		// neighbourhood (or the whole schema when no table is focused), shown in
		// a scrollable panel. Same as `:erd [table]`.
		if msg.String() == "R" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			return m, m.openERD(m.currentTable())
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
					return m, m.startDeleteRows()
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
					return m, m.copyCursorCell()
				}
				m.resultsPendingY = true
				return m, nil
			case "r":
				// y r — copy marked/cursor rows as TSV (same as :copyrow).
				// Completes the pending-Y chord; yy remains copy-cell, g r
				// remains the explorer (handled above when pending-G).
				if m.resultsPendingY {
					m.resultsPendingY = false
					m.resultsPendingG = false
					return m, m.copyRowsDelimited(fmtTSV)
				}
			case "p":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if !m.results.IsEditable() || !m.results.HasPrimaryKey() {
					return m, nil
				}
				// With marks, fill the current column across marked rows
				// (dirty only). Without marks, paste into the cursor cell
				// and save immediately. Allowed even when the inspector is
				// open — focus is still on results here.
				if m.results.MarkCount() > 0 {
					m.fillMarkedRows()
					return m, nil
				}
				colName := m.results.ColumnName(m.results.CursorCol())
				if m.results.isPKColumn(colName) {
					return m, nil
				}
				if m.results.IsBlobCell(m.results.CursorRow(), m.results.CursorCol()) {
					m.exportMsg = "binary cell — use :saveblob to export"
					return m, nil
				}
				clip, err := clipboard.ReadAll()
				val := ""
				if err == nil && clip != "" {
					val = clip
				} else {
					val = m.yank
				}
				if val == "" {
					m.exportMsg = "clipboard is empty"
					return m, copyFeedbackCmd()
				}
				m.results.SetDirtyCell(m.results.CursorRow(), m.results.CursorCol(), val)
				return m, m.saveChanges()
			case "s":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					return m, m.fetchColumnStats()
				}
			case "e", "i":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.startResultsCellEdit()
			case "E":
				// Expand opens a multi-line peek of the cell under the cursor.
				// It doubles as a read-only viewer when the results can't be
				// written back (read-only mode, custom queries, PK-less views),
				// so it is intentionally not gated on editability like e/i.
				// Close the inspector first when safe (same as e/i / double-click).
				m.resultsPendingG = false
				m.resultsPendingY = false
				if !m.prepareResultsEdit() {
					return m, nil
				}
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
				m.discardResultsEdits(false)
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
				if m.results.MarkCount() > 0 || m.results.ColumnMarkCount() > 0 {
					m.results.ClearAllMarks()
				}
				return m, nil
			case "M":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.NumCols() == 0 {
					return m, nil
				}
				if !m.results.ToggleColumnMark() {
					m.schemaMsg = "mark at most 2 columns (label, then value) — then :bar / :line"
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
				if cmd := m.copyRowsAsInsert(); cmd != nil {
					return m, cmd
				}
			case "P":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.cloneRows()
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
			case ">":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resizeResultsColumn(colResizeStep)
				return m, nil
			case "<":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resizeResultsColumn(-colResizeStep)
				return m, nil
			case "=":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resetResultsColumnWidth()
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
			if ch, ok := keyFilterChar(msg); ok {
				m.sidebarFilter += ch
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
		case "l", "right":
			m.sidebarPendingG = false
			m.focus = FocusResults
			m.applyFocus()
			return m, nil
		case "/":
			m.sidebarFiltering = true
			m.sidebarFilter = ""
			m.sidebarCursor = 0
			return m, nil
		case "enter", "s":
			item := m.currentSidebarItem()
			if item != nil && !item.isColumn {
				return m, m.openTable(item.text)
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
				if m.confirmDestructive() {
					m.truncateConfirm = item.text
					return m, nil
				}
				return m, m.execTruncate(item.text)
			}
		case "D":
			m.sidebarPendingG = false
			item := m.currentSidebarItem()
			if item != nil && !item.isColumn {
				if m.confirmDestructive() {
					m.dropTableConfirm = item.text
					m.dropTableInput = ""
					return m, nil
				}
				return m, m.execDropTable(item.text)
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
				m.commitInspectorFieldEdit()
				return m, nil
			case "esc", "ctrl+c":
				m.inspector.CancelEdit()
				return m, nil
			case "up", "k":
				// Moving the field cursor must end the in-flight edit first;
				// otherwise the shared textinput keeps rendering on the newly
				// focused field (same failure mode as results-grid click-away).
				m.commitInspectorFieldEdit()
				m.inspector.CursorUp()
				return m, nil
			case "down", "j":
				m.commitInspectorFieldEdit()
				m.inspector.CursorDown(m.inspectorResults())
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
				m.inspector.CommitFilter(m.inspectorResults())
				return m, nil
			case "backspace":
				m.inspector.FilterBackspace()
				return m, nil
			case "up", "k":
				m.inspector.CursorUp()
				return m, nil
			case "down", "j":
				m.inspector.CursorDown(m.inspectorResults())
				return m, nil
			}
			if ch, ok := keyFilterChar(msg); ok {
				m.inspector.FilterAddChar(ch)
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
			m.inspector.CursorDown(m.inspectorResults())
			return m, nil
		case "G":
			m.inspector.pendingG = false
			m.inspector.CursorBottom(m.inspectorResults())
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
			src := m.inspectorResults()
			col := m.inspector.selectedColumn(src)
			if !m.inspector.IsInserting() && m.results.IsBlobCell(m.results.CursorRow(), col) {
				return m, m.openCellEditPopup(m.results.CursorRow(), col)
			}
			if !m.inspector.IsInserting() && m.inspector.IsFieldTruncated(src) {
				return m, m.openCellEditPopup(m.results.CursorRow(), col)
			}
			m.inspector.StartFieldEdit(src)
			return m, nil
		case "E":
			m.inspector.pendingG = false
			if !m.inspector.IsInserting() {
				col := m.inspector.selectedColumn(m.inspectorResults())
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
				return m, m.maybeRestoreExplorerAfterInsert()
			}
		case "D":
			m.inspector.pendingG = false
			if m.results.HasDirtyCells() {
				if m.confirmDestructive() {
					m.discardConfirm = true
					return m, nil
				}
				m.results.DiscardEdits()
			}
			return m, nil
		}
		return m, nil
	case FocusAssistant:
		// Route all keys to the panel. Global focus movement (ctrl+h/j/k/l)
		// is handled before this switch, so it still works; esc in compose
		// mode just leaves compose, and esc in browse mode closes the panel.
		a, acmd := m.assistant.HandleKey(msg)
		m.assistant = a
		return m, acmd
	case FocusExplorer:
		// Docked relationship-explorer panel. Non-modal: global focus movement
		// (ctrl+h/l) is handled before this switch. Tree nav: j/k move, → expands,
		// ← collapses, Enter re-roots the grid, t opens the node in a new tab,
		// A inserts related, u/g b goes back, r retargets, esc/q close the panel.
		switch msg.String() {
		case "esc", "q":
			m.closeDockedExplorer()
			return m, nil
		case "enter":
			return m, m.explorerActivate()
		case "t":
			return m, m.explorerOpenInTab()
		case "right", "l":
			return m, m.explorerExpand()
		case "left", "h":
			m.explorerCollapse()
			return m, nil
		case "A":
			return m, m.explorerInsertRelated()
		case "u", "backspace":
			if len(m.queryStack) == 0 {
				m.schemaMsg = "nothing to go back to"
				return m, nil
			}
			return m, m.goBackQuery()
		case "b":
			// g b from the explorer — same as results.
			if m.resultsPendingG {
				m.resultsPendingG = false
				if len(m.queryStack) == 0 {
					m.schemaMsg = "nothing to go back to"
					return m, nil
				}
				return m, m.goBackQuery()
			}
		case "g":
			m.resultsPendingG = true
			return m, nil
		case "r":
			m.resultsPendingG = false
			m.explorer.markLoading()
			return m, m.loadExplorer()
		}
		m.resultsPendingG = false
		m.explorer = m.explorer.Update(msg)
		return m, nil
	}
	return m, cmd
}

// refreshCompletionCandidates rebuilds the editor's candidate list from
// keywords, schemas, tables (active + cached cross-schema), and columns.
func (m *Model) refreshCompletionCandidates() {
	var candidates []completionItem

	for _, kw := range sqlKeywords {
		candidates = append(candidates, completionItem{text: kw, kind: kindKeyword})
	}
	for _, s := range m.schemaNames {
		candidates = append(candidates, completionItem{text: s, kind: kindSchema})
	}
	for _, t := range m.tables {
		candidates = append(candidates, completionItem{text: t, kind: kindTable})
	}
	for schema, tables := range m.schemaTableCache {
		for _, t := range tables {
			candidates = append(candidates, completionItem{text: t, kind: kindTable, schema: schema})
		}
	}

	for table, cols := range m.columnCache {
		for _, c := range cols {
			candidates = append(candidates, completionItem{text: c.Name, kind: kindColumn, table: table})
		}
	}

	m.editor.SetActiveSchema(m.currentSchemaName())
	m.editor.SetCandidates(candidates)
}

// View renders the entire application.
func (m Model) View() string {
	if m.viewCached && m.viewBuf != nil && *m.viewBuf != "" {
		return *m.viewBuf
	}
	s := m.buildView()
	if m.viewBuf != nil {
		*m.viewBuf = s
	}
	return s
}

func (m Model) buildView() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return "Loading..."
	}

	if m.state == stateAddConnection {
		return m.paintBg(m.viewAddConnection())
	}

	if m.state == stateConnections {
		return m.paintBg(m.viewConnections())
	}

	// Database picker: same shell as the connection list / form.
	if m.dbPicker.IsVisible() {
		pw, ph := popupOuterSize(m.height)
		m.dbPicker.SetSize(pw, ph)
		pickerPanel := m.dbPicker.View()
		view := lipgloss.Place(m.width, m.height-1,
			lipgloss.Center, lipgloss.Center,
			pickerPanel,
			canvasPlaceOptions(m.canvasBackground())...)

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
		return m.paintBg(lipgloss.JoinVertical(lipgloss.Left, view, statusBar))
	}

	return m.paintBg(m.viewWorkspace())
}

// connFormPopupDims returns the screen bounds of the add/edit connection form
// popup, matching viewAddConnection's centering and dynamic height.
func (m Model) connFormPopupDims() (panelW, panelH, panelX, panelY int) {
	popupW, _ := popupDim()
	const borderOverhead = 2
	innerW, capH := popupContentSize(m.height)
	m.connForm.SetSize(innerW, capH)
	contentH := m.connForm.effectiveHeight()
	popupH := contentH + borderOverhead
	panelW = popupW
	panelH = popupH
	panelX = (m.width - panelW) / 2
	panelY = (m.height - 1 - panelH) / 2
	return panelW, panelH, panelX, panelY
}

// connListPopupDims returns the (width, height) of the connection-list popup.
// The footprint matches the connection form and database picker (see
// popupOuterSize) so transitions between those screens stay calm. The list
// scrolls internally when there are more connections than fit.
func (m Model) connListPopupDims() (w, h int) {
	return popupOuterSize(m.height)
}

// connListContentDims returns the content width and list-area height the
// connection list should be sized to, derived from connListPopupDims. Shared
// by layout.go (which sizes the real model) and viewConnections (render) so the
// scroll math and what is drawn always agree.
func (m Model) connListContentDims() (contentW, listH int) {
	pw, ph := m.connListPopupDims()
	panelW := pw - 2   // border
	panelH := ph - 2   // border
	listH = panelH - 2 // prompt + scroll-info (chrome)
	if listH < linesPerField {
		listH = linesPerField
	}
	contentW = panelW - 2 // Padding(0,1) → 2 cols
	return contentW, listH
}

func (m Model) viewConnections() string {
	popupW, popupH := m.connListPopupDims()
	borderOverhead := 2

	panelW := popupW - borderOverhead
	panelH := popupH - borderOverhead

	prompt := m.connList.Prompt()
	contentW, listH := m.connListContentDims()
	m.connList.SetSize(contentW, listH)
	m.connList.SetPadBackground(m.canvasBackground())

	listStyled := m.connList.View()
	// Tabs sit above the filter prompt so the fuzzy line stays next to the list.
	parts := make([]string, 0, 4)
	if tabs := m.connList.GroupTabBar(); tabs != "" {
		parts = append(parts, tabs)
	}
	parts = append(parts, prompt, listStyled, m.connList.ScrollInfo())

	panelStyle := lipgloss.NewStyle().
		Width(panelW).
		Height(panelH).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1)
	if bg := m.canvasBackground(); string(bg) != "" {
		panelStyle = panelStyle.Background(bg)
	}
	connPanel := panelStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, parts...),
	)

	// Space-filled canvas so paintBg covers the whole screen behind the popup.
	view := lipgloss.Place(m.width, m.height-1,
		lipgloss.Center, lipgloss.Center,
		connPanel,
		canvasPlaceOptions(m.canvasBackground())...)

	// Overlay help panel if visible (sized to leave the status bar showing).
	if m.help.IsVisible() {
		m.help.SetSize(m.width, m.height-1)
		view = m.help.View()
	}

	// Overlay connection-deletion confirmation if pending.
	if m.deleteConnConfirm != "" {
		prompt := fmt.Sprintf("Delete connection %s?\nIts keychain secrets are also removed.", m.deleteConnConfirm)
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
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
	// Sizing is shared with layout.go via popupContentSize so the scroll model
	// and rendering agree. The form uses the fixed shell height (6-field
	// footprint) so it matches the connection list and database picker.
	popupW, _ := popupDim() // width stays fixed
	borderOverhead := 2
	innerW, capH := popupContentSize(m.height)
	m.connForm.SetSize(innerW, capH)
	contentH := m.connForm.effectiveHeight()
	popupH := contentH + borderOverhead

	formPanel := lipgloss.NewStyle().
		Width(popupW-borderOverhead).
		Height(popupH-borderOverhead).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(m.connForm.View())

	panelW := lipgloss.Width(formPanel)
	panelH := lipgloss.Height(formPanel)
	panelX := (m.width - panelW) / 2
	panelY := (m.height - 1 - panelH) / 2

	// Center the popup in the area above the status bar, then append the
	// status bar so the keybinding hints (enter / ctrl+t / esc) are visible.
	placed := lipgloss.Place(m.width, m.height-1,
		lipgloss.Center, lipgloss.Center,
		formPanel,
		lipgloss.WithWhitespaceChars(" "))

	if comp := m.connForm.CompletionView(); comp != "" {
		if row := m.connForm.completionLineOffset(); row >= 0 {
			placed = placeOverlay(placed, comp, panelX+4, panelY+1+row)
		}
	}
	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(1).
		Foreground(colorMuted).
		Background(colorStatusBarBg).
		Render(" " + m.statusBar(""))
	return lipgloss.JoinVertical(lipgloss.Left, placed, statusBar)
}

func (m Model) viewWorkspace() string {
	g := m.workspaceGeom()
	sidebarWidth := g.SidebarWidth
	slotWidth := g.RightSlotW
	statusHeight := g.StatusH
	borderOverhead := g.BorderOH
	editorHeight := g.EditorHeight
	resultsHeight := g.ResultsHeight
	rightWidth := g.RightWidth

	// Build the content area (tabs are inside the editor panel).
	var contentPanel string
	if m.tableDesigner.IsVisible() {
		designerHeight := editorHeight + resultsHeight
		m.tableDesigner.SetSize(rightWidth-borderOverhead, designerHeight-borderOverhead)
		contentPanel = lipgloss.NewStyle().
			Width(rightWidth).
			Height(designerHeight).
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Render(m.tableDesigner.View())
	} else if m.schemaEditor.IsVisible() {
		editorH := editorHeight + resultsHeight
		m.schemaEditor.SetSize(rightWidth-borderOverhead, editorH-borderOverhead)
		contentPanel = lipgloss.NewStyle().
			Width(rightWidth).
			Height(editorH).
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Render(m.schemaEditor.View())
	} else {
		var resultsPanel string
		if m.chartPanel.IsVisible() {
			resultsPanel = m.chartPanel.View()
		} else if m.queryRunning && !m.backendSearching {
			// Show an animated spinner while the query executes.
			frame := spinnerFrames[m.querySpinner%len(spinnerFrames)]
			elapsed := time.Since(m.queryStart).Round(time.Millisecond)
			content := lipgloss.NewStyle().Foreground(colorPrimary).Render(frame) +
				"  " + mutedStyle.Render(fmt.Sprintf("running query… %s", elapsed)) +
				"  " + lipgloss.NewStyle().Foreground(colorMuted).Render(m.cancelHint())
			resultsPanel = lipgloss.NewStyle().
				Width(rightWidth).
				Height(resultsHeight).
				Border(panelBorder()).
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
					Border(panelBorder()).
					BorderForeground(m.borderForFocus(FocusResults))
			}
			resultsPanel = resultsStyle.Render(func() string {
				m.results.SetSort(m.sortCol, m.sortDir)
				// The prompt no longer lives inside this panel; the table fills
				// it at full height. CmdHeight in workspaceGeom already reserved
				// the row the bottom command line occupies.
				m.results.SetSize(rightWidth+borderOverhead, resultsHeight+borderOverhead)
				return m.results.View()
			}())
		}

		if m.editorVisible {
			editorPanel := lipgloss.NewStyle().
				Width(rightWidth).
				Height(editorHeight - borderOverhead).
				Border(panelBorder()).
				BorderForeground(m.borderForFocus(FocusEditor)).
				Render(lipgloss.JoinVertical(lipgloss.Left,
					m.tabBar.View(),
					lipgloss.NewStyle().Foreground(colorBorder).
						Render(strings.Repeat("─", rightWidth)),
					m.editor.View(),
				))
			contentPanel = lipgloss.JoinVertical(lipgloss.Left,
				editorPanel,
				resultsPanel,
			)
		} else {
			contentPanel = resultsPanel
		}
	}

	rightPanel := contentPanel

	// Build the right-hand slot panel: the inspector, assistant, and docked
	// relationship explorer are mutually exclusive, so at most one is rendered.
	var slotPanel string
	if m.inspector.IsVisible() {
		slotContentHeight := lipgloss.Height(rightPanel) - borderOverhead
		if slotContentHeight < 3 {
			slotContentHeight = 3
		}
		m.inspector.SetSize(slotWidth-borderOverhead, slotContentHeight)
		slotPanel = lipgloss.NewStyle().
			Width(slotWidth - borderOverhead).
			Height(slotContentHeight).
			Border(panelBorder()).
			BorderForeground(m.borderForFocus(FocusInspector)).
			Render(m.inspector.View(m.inspectorResults()))
	} else if m.assistant.IsVisible() {
		slotContentHeight := lipgloss.Height(rightPanel) - borderOverhead
		if slotContentHeight < 3 {
			slotContentHeight = 3
		}
		m.assistant.SetSize(slotWidth-borderOverhead, slotContentHeight)
		m.assistant.spinner = m.querySpinner // keep the pending spinner in sync
		m.assistant.SetModel(m.effectiveAIModel())
		slotPanel = lipgloss.NewStyle().
			Width(slotWidth - borderOverhead).
			Height(slotContentHeight).
			Border(panelBorder()).
			BorderForeground(m.borderForFocus(FocusAssistant)).
			Render(m.assistant.View())
	} else if m.explorer.IsVisible() && m.explorer.docked {
		// The explorer's View() already draws its own border, so place it
		// directly (no second border) sized to the full slot. Its border color
		// mirrors focus, like the inspector/assistant.
		slotH := lipgloss.Height(rightPanel)
		m.explorer.focused = m.focus == FocusExplorer
		m.explorer.SetSize(slotWidth, slotH)
		slotPanel = m.explorer.View()
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
			expandIcon := icons.collapsed
			if _, ok := m.expanded[item.text]; ok {
				expandIcon = icons.expanded
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
			if item.isView {
				line += " " + mutedStyle.Render("view")
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
		Border(panelBorder()).
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
	if m.inspector.IsVisible() || m.assistant.IsVisible() || (m.explorer.IsVisible() && m.explorer.docked) {
		if m.sidebarVisible && sidebarWidth > 0 {
			workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel, slotPanel)
		} else {
			workspace = lipgloss.JoinHorizontal(lipgloss.Top, rightPanel, slotPanel)
		}
	} else if m.sidebarVisible && sidebarWidth > 0 {
		workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel)
	} else {
		workspace = rightPanel
	}

	// Dim the workspace panels behind long-lived editing overlays.
	// The status bar is kept undimmed so hints remain clearly visible.
	if m.cellEdit.IsVisible() || m.history.IsVisible() || m.bookmarks.IsVisible() || m.crossSearch.IsVisible() || m.explainPanel.IsVisible() || m.diffPanel.IsVisible() || m.lookupPanel.IsVisible() {
		workspace = dimBackground(workspace)
	}

	// Stack workspace, status bar, and the bottom command line. The command
	// line is omitted entirely when no prompt is active, so it adds no height
	// at rest (cmdHeight is 0 then, keeping the layout identical to before).
	layers := []string{workspace, statusBar}
	if cmd := m.commandLine(); cmd != "" {
		layers = append(layers, cmd)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, layers...)

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

	// Overlay result-set diff panel if visible
	if m.diffPanel.IsVisible() {
		m.diffPanel.SetSize(m.width*80/100, (m.height-1)*75/100)
		diffPanelView := m.diffPanel.View()
		panelW := lipgloss.Width(diffPanelView)
		panelH := lipgloss.Height(diffPanelView)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, diffPanelView, panelX, panelY)
	}

	// Overlay lookup panel if visible
	if m.lookupPanel.IsVisible() {
		m.lookupPanel.SetSize(m.width*70/100, (m.height-1)*70/100)
		lookupPanelView := m.lookupPanel.View()
		panelW := lipgloss.Width(lookupPanelView)
		panelH := lipgloss.Height(lookupPanelView)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, lookupPanelView, panelX, panelY)
	}

	// Overlay ERD panel if visible — fills the whole workspace area (above the
	// status line), edge to edge, with no frame so the diagram gets maximum room.
	if m.erdPanel.IsVisible() {
		m.erdPanel.SetSize(m.width, m.height-1)
		view = placeOverlay(view, m.erdPanel.View(), 0, 0)
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
		dialog := renderConfirmDialogBare("Discard all unsaved changes?")
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
			Border(panelBorder()).
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
			Border(panelBorder()).
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
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Render(m.cellEdit.View())
		panelW := lipgloss.Width(panel)
		panelH := lipgloss.Height(panel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, panel, panelX, panelY)
	}

	// Overlay completion popup if visible. Anchor below the cursor, then clamp
	// into the workspace (above status/cmd, left of the right slot) so a long
	// list or a cursor near the edge does not clip off-screen.
	if m.editor.CompletionVisible() {
		cursorLine, cursorCol := m.editor.CursorScreenPos()
		popup := m.editor.CompletionView()
		popupW := lipgloss.Width(popup)
		popupH := lipgloss.Height(popup)
		// Editor panel: top border (1) + tab bar (1) + separator (1) → content.
		const editorContentTop = 1 + 1 + 1
		cursorTop := editorContentTop + cursorLine
		popupX := sidebarWidth + 2 + cursorCol
		popupY := cursorTop + 1 // one row below the cursor line
		maxW := g.EditorRight
		if maxW <= 0 {
			maxW = m.width
		}
		maxH := m.height - statusHeight - g.CmdHeight
		popupX, popupY = fitCompletionPopup(popupX, popupY, cursorTop, popupW, popupH, maxW, maxH)
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

	// Overlay export dialog (g X) if visible
	if m.exportOverlay.IsVisible() {
		pw := 72
		if pw > m.width-4 {
			pw = m.width - 4
		}
		ph := m.height - 2
		if ph > 26 {
			ph = 26
		}
		m.exportOverlay.SetSize(pw, ph)
		exportPanel := m.exportOverlay.View()
		panelW := lipgloss.Width(exportPanel)
		panelH := lipgloss.Height(exportPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, exportPanel, panelX, panelY)
	}

	// Overlay theme picker (g c) if visible
	if m.themePicker.IsVisible() {
		themePanel := m.themePicker.View()
		panelW := lipgloss.Width(themePanel)
		panelH := lipgloss.Height(themePanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, themePanel, panelX, panelY)
	}

	// Overlay provider picker (M) if visible
	if m.providerPicker.IsVisible() {
		providerPanel := m.providerPicker.View()
		panelW := lipgloss.Width(providerPanel)
		panelH := lipgloss.Height(providerPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, providerPanel, panelX, panelY)
	}

	// Overlay provider-deletion confirmation (stacked over the picker) if pending.
	if m.deleteProviderConfirm != "" {
		prompt := fmt.Sprintf("Delete provider %s?\nIts keychain API key is also removed.", m.deleteProviderConfirm)
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay model browser (m) if visible
	if m.modelBrowser.IsVisible() {
		modelPanel := m.modelBrowser.View()
		panelW := lipgloss.Width(modelPanel)
		panelH := lipgloss.Height(modelPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, modelPanel, panelX, panelY)
	}

	// Overlay provider form (n/e from the `M` picker) if visible. Sized like
	// the connection form popup (fixed popupDim width, content-tall height)
	// so it renders identically to the other bordered-field form.
	if m.providerForm.IsVisible() {
		popupW, _ := popupDim()
		borderOverhead := 2
		innerW, _ := popupContentSize(m.height)
		m.providerForm.SetSize(innerW)
		formPanel := lipgloss.NewStyle().
			Width(popupW-borderOverhead).
			Height(m.providerForm.effectiveHeight()).
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1).
			Render(m.providerForm.View())
		panelW := lipgloss.Width(formPanel)
		panelH := lipgloss.Height(formPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, formPanel, panelX, panelY)
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

	// Overlay ":" verb-completion popup directly above the command line.
	if m.ex.visible {
		if popup := m.ex.completionView(m.width); popup != "" {
			ph := lipgloss.Height(popup)
			view = placeOverlay(view, popup, 1, m.height-1-ph)
		}
	}

	// Overlay help panel if visible, leaving the status bar visible below it
	// (help fills the top height-1 rows; the status bar is the last row).
	if m.help.IsVisible() {
		m.help.SetSize(m.width, m.height-1)
		view = placeOverlay(view, m.help.View(), 0, 0)
	}

	return view
}
