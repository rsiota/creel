package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/bookmarks"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
	"github.com/rsiota/creel/internal/history"
	"github.com/rsiota/creel/internal/recent"
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
	// Multi-statement (:runall / :source) metadata. multiTotal == 0 means
	// the single-statement path (ctrl+e / :run).
	multiRan   int // statements completed successfully before finish/error
	multiTotal int // total statements in the batch
	multiFail  int // 1-based index of the failing statement (0 if none)
	// wrapSource is the alias-rewritten SELECT when this run disambiguated
	// duplicate output names. setWrap reports whether to store it.
	wrapSource string
	setWrap    bool
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
	gen         uint64 // must match CrossSearchPanel.Gen or the msg is dropped
	results     []SearchResult
	tablesDone  int
	skipped     int  // tables skipped due to schema/query errors in this batch
	capped      bool // this batch filled the current hit budget
	done        bool // no further tables or deferred pages remain
	nextTable   int
	tableOffset int
	deferred    []crossSearchPos
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
// analyze marks EXPLAIN ANALYZE (timed) vs plan-only EXPLAIN.
type explainResultMsg struct {
	result  db.Result
	err     error
	query   string
	forAI   bool
	analyze bool
	focus   string // optional user focus for :aiexplain (e.g. "why is the join slow")
}

// diagnoseResultMsg carries rule-based EXPLAIN findings for the lookup overlay.
type diagnoseResultMsg struct {
	query    string
	planText string
	title    string
	result   db.Result
	jumps    []string
	err      error
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

// killDoneMsg carries the result of an async :kill session termination.
type killDoneMsg struct {
	pid string
	err error
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

// backupDoneMsg carries the result of an async :backup (mysqldump) run.
type backupDoneMsg struct {
	path  string
	bytes int64
	err   error
}

// backupPickerMsg carries table sizes for the :backup selection overlay.
type backupPickerMsg struct {
	sizes []db.TableSize
	bin   string
	err   error
}

// backupProgressMsg carries live byte-count updates during :backup.
type backupProgressMsg struct {
	bytes int64
	path  string
}

// backupProgressWrapper re-issues the progress poll after each status update.
type backupProgressWrapper struct {
	msg      backupProgressMsg
	progress <-chan backupProgressMsg
	done     <-chan backupDoneMsg
}

// restoreDoneMsg carries the result of an async :restore (mysql/psql) run.
type restoreDoneMsg struct {
	path         string
	bytes        int64
	err          error
	clientStderr string
	continued    bool
}

// restoreProgressMsg carries live byte-count updates during :restore.
type restoreProgressMsg struct {
	bytes int64
	path  string
}

// restoreProgressWrapper re-issues the progress poll after each status update.
type restoreProgressWrapper struct {
	msg      restoreProgressMsg
	progress <-chan restoreProgressMsg
	done     <-chan restoreDoneMsg
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
	resultsTabs           []*ResultsTab // All result tabs
	activeTabID           int           // Currently active tab ID
	nextTabID             int           // Counter for generating unique tab IDs
	tabBar                TabBar        // Tab navigation component
	history               HistoryPanel
	bookmarks             BookmarkPanel
	crossSearch           CrossSearchPanel
	dbPicker              DatabasePicker
	help                  HelpPanel
	filterPicker          FilterPicker
	columnPicker          ColumnPicker
	exportPicker          ExportPicker
	backupPicker          BackupPicker
	exportOverlay         ExportOverlay
	themePicker           ThemePicker
	providerPicker        ProviderPicker
	modelBrowser          ModelBrowser
	providerForm          ProviderForm
	importPrompt          ImportPrompt
	addColumnForm         AddColumnForm
	tableRenameForm       TableRenameForm
	tableDesigner         TableDesigner
	schemaEditor          SchemaEditor
	cellEdit              CellEditPopup
	explainPanel          ExplainPanel
	diffPanel             DiffPanel
	lookupPanel           LookupPanel
	erdPanel              ERDPanel
	chartPanel            ChartPanel
	explorer              RelExplorer
	palette               palette
	ex                    exCmd
	sidebarCursor         int
	sidebarScroll         int              // cached scroll offset of the first visible sidebar item
	sidebarViewAnchored   bool             // mouse click froze the view; keyboard nav clears it
	tableRowCounts        map[string]int64 // approximate row counts for sidebar display
	expanded              map[string][]db.Column
	columnCache           map[string][]db.Column
	views                 map[string]bool            // view names from Views(); badges views in the sidebar
	pkCache               map[string][]string        // table -> PK columns (AI schema context)
	fkCache               map[string][]db.ForeignKey // table -> FKs (AI schema context)
	schemaNames           []string                   // schemas/namespaces for editor completion
	schemaTableCache      map[string][]string        // schema → tables (cross-schema completion)
	sidebarSchemaExpanded map[string]bool            // schema header expand override (default: active only)
	recentTables          []string                   // MRU table names (most recent first); for :recent

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

	// OSC 52 paste query, used when the OS clipboard cannot be read (SSH).
	// clipSinking swallows a reply that arrives after the query timed out so
	// the base64 is not typed into the editor.
	clipKind       clipKind
	clipSinking    bool
	clipGen        uint64
	clipFallback   string
	clipCollect    osc52Collect
	osc52PasteDead bool

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
	lastInspectorClickCol  int       // result column index of last inspector click (-1 = none)
	lastInspectorWheelTime time.Time // debounce wheel → one field step per notch
	lastConnFormClickTime  time.Time
	lastConnFormClickField int       // field index of last connection-form click (-1 = none)
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

	// Kill-session confirmation dialog (non-empty pid while pending).
	killConfirm string

	// EXPLAIN ANALYZE confirmation (true while the y/enter prompt is up).
	// ANALYZE actually runs the statement, so it is gated like other
	// confirm_destructive actions.
	explainAnalyzeConfirm bool

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
	page           int
	pageSize       int
	lastQuery      string
	pageMsg        string
	totalRows      int       // total rows in the current table (0 = unknown)
	totalRowsSet   bool      // whether totalRows has been fetched for this query
	statsMsg       string    // transient column statistics display
	exportMsg      string    // transient CSV export / backup / import status
	backupStarted  time.Time // wall clock when the current :backup started (for MB/s)
	restoreStarted time.Time // wall clock when the current :restore started (for MB/s)
	searchMsg      string    // transient regex search result display
	truncateMsg    string    // transient truncate result display
	deleteRowsMsg  string    // transient row deletion result display
	schemaMsg      string    // transient schema change result display
	bookmarkMsg    string    // transient bookmark result display

	// flashGen tracks the current "generation" of the transient status message.
	// Each time a new flash is set, the wrapper increments this and arms a
	// flashTickMsg. When the tick fires it only clears the flash if the
	// generation still matches, preventing it from wiping a newer message.
	flashGen uint64

	// Quick filters (cell-based, server-side WHERE injection)
	baseQuery   string            // original query without filters
	filters     []string          // active filter expressions, AND-joined
	queryParams map[string]string // :name → value; expanded before execute
	// wrapSource is the user's SELECT with duplicate output names aliased, so
	// filter/sort can wrap it. Empty when baseQuery's names are already unique.
	wrapSource string
	// colsDisambiguated is set when the grid renamed duplicate headers but the
	// SQL still has them, so filter/sort must stay off.
	colsDisambiguated bool

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
	// theme_overrides (if any) patch semantic slots before styles rebuild.
	applyTheme(settings.Theme, settings.ThemeOverrides)

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
		backupPicker:    NewBackupPicker(),
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
	if msg := themeOverrideSkipMessage(settings.ThemeOverrides); msg != "" {
		m.schemaMsg = msg
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
			m.syncInspectorFieldFromGrid()
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
	m.layoutWorkspace()
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
	m.layoutWorkspace()
}

// actionMenuSection returns the registry section title for the contextual
// action menu (g m / :menu). Prefers hintSection(); when that is empty
// (compose / filter chrome), falls back to the focused panel's section.
func (m Model) actionMenuSection() string {
	if sec := m.hintSection(); sec != "" {
		return sec
	}
	switch {
	case m.state == stateConnections:
		return "Connections"
	case m.focus == FocusEditor:
		return "Editor (Vim)"
	case m.focus == FocusResults:
		return "Results"
	case m.focus == FocusConnections:
		return "Sidebar (Tables)"
	case m.focus == FocusInspector:
		return "Inspector"
	case m.focus == FocusAssistant:
		return "Assistant"
	case m.focus == FocusTabBar:
		return "Tab Bar"
	case m.focus == FocusExplorer:
		return "Relationship Explorer"
	}
	return ""
}

// openActionMenu opens the contextual palette for the current panel plus
// Global (executable bindings only). Sets schemaMsg when nothing applies.
func (m *Model) openActionMenu() {
	sec := m.actionMenuSection()
	if sec == "" {
		m.schemaMsg = "no actions here"
		return
	}
	sections := []string{sec}
	if sec != "Global" {
		sections = append(sections, "Global")
	}
	m.palette.OpenContextual(sections, sec)
}

// handlePaletteKey routes keys to the open command palette. Many bindings only
// fire in a specific panel — the usual post-connect focus is the editor, so
// confirming those rows from Ctrl+P used to replay into vim and look like a
// no-op. Focus the owning panel before the replay.
func (m Model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	confirming := msg.String() == "enter"
	var section, display string
	if confirming {
		it := m.palette.selectedItem()
		section = it.section
		display = it.display
	}
	var cmd tea.Cmd
	m.palette, cmd = m.palette.Update(msg)
	if confirming && cmd != nil {
		switch {
		case section == "Sidebar (Tables)":
			// Workspace table list reuses FocusConnections.
			m.focus = FocusConnections
			m.applyFocus()
		case section == "Results" || section == "Tabs" || section == "Theme Picker":
			// g-chords (g R, g t, g c, …) need a panel pending-G flag.
			m.focus = FocusResults
			m.applyFocus()
		case section == "Global" && display == "\\":
			// \ only runs the query from the editor in vim normal mode.
			m.focus = FocusEditor
			m.applyFocus()
		}
	}
	return m, cmd
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
// g t / g T — next / previous tab (separate registry rows for palette replay)
// g x       — close tab
// g 1-9     — go to tab N
// g c       — open theme picker (live preview)
// g m       — contextual action menu for the focused panel
// t         — new tab (sidebar, results, tab bar only)
func (m *Model) handleTabKey(msg tea.KeyMsg) bool {
	if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() ||
		m.searching || m.ex.visible || m.backendSearching ||
		m.sidebarFiltering || m.inspector.IsFiltering() ||
		m.crossSearch.IsVisible() || m.history.IsVisible() || m.bookmarks.IsVisible() ||
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
			m.themePicker.Show(m.settings.Theme, m.settings.ThemeOverrides)
			return true
		case "m":
			// g m — contextual action menu (focused panel + Global).
			m.clearPendingG()
			m.openActionMenu()
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
