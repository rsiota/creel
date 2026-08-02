package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/creel/internal/db"
)

// SchemaEditor is an inline grid editor for modifying an existing table's
// schema. It mirrors the look of ResultsTable / TableDesigner so users can
// edit column properties with the same muscle memory.
//
// Each row is a column; each cell is an editable property. The action is
// inferred from which cell the user edits — no action menu required:
//   - Name cell    → RENAME COLUMN
//   - Type cell    → MODIFY COLUMN (MySQL only)
//   - Null cell    → MODIFY COLUMN (MySQL only)
//   - Default cell → MODIFY COLUMN (MySQL only)
//
// Each cell edit is its own DDL statement, committed immediately on enter.
type SchemaEditor struct {
	table     string
	driver    db.Driver
	columns   []db.TableColumnInfo
	rows      [][]string // [col][prop] mirrors columns: name, type, null, default
	rowOrigin []int      // maps grid row → index into columns, or -1 for a new (pending) row
	cursorRow int
	cursorCol int
	editing   bool
	editInput textinput.Model
	pendingD  bool
	visible   bool
	width     int
	height    int
	errMsg    string
	notice    string
	colWidths []int

	// Tabbed structure view (read-only metadata).
	activeTab       int // one of seTab*
	structure       structureData
	structLoaded    bool
	roCursor        int  // row cursor for the active read-only tab
	roScroll        int  // scroll offset for read-only tabs / definition
	triggerExpanded bool // triggers tab: show the selected trigger's statement

	// Mouse: last left-click, for double-click-to-edit detection on the
	// Columns grid (mirrors the results panel).
	lastClickTime time.Time
	lastClickRow  int
	lastClickCol  int
}

// SchemaEditorResult carries the SQL built from a single cell edit so app.go
// can run it asynchronously. It distinguishes the schema action so the result
// handler knows what to refresh.
type SchemaEditorResult struct {
	SQL     string
	Action  db.SchemaAction
	ErrFunc func(err string)
}

// structureData is the read-only metadata payload delivered to the schema
// editor's Indexes / Foreign Keys / Triggers / Definition tabs. It is
// populated by the async loader (loadStructureMetadata) and carries
// per-section errors so one failing catalog query does not blank a tab.
type structureData struct {
	err        string // reserved (columns load synchronously, so unused today)
	pk         []string
	fks        []db.ForeignKey
	indexes    []db.Index
	triggers   []db.Trigger
	checks     []db.CheckConstraint
	viewDef    string
	indexErr   string
	triggerErr string
	checkErr   string
	viewErr    string
}

// Grid column indices.
const (
	seColName = iota
	seColType
	seColNull
	seColDefault
	seColCount
)

var seHeaders = []string{"Name", "Type", "Null", "Default"}

// Structure-view tabs. Columns is always present and editable; the rest are
// read-only metadata loaded asynchronously (see LoadStructure). Checks sits
// between Foreign Keys and Triggers (both are constraints). Definition is
// only shown for views.
const (
	seTabColumns = iota
	seTabIndexes
	seTabFK
	seTabChecks
	seTabTriggers
	seTabDefinition
)

var seTabLabels = map[int]string{
	seTabColumns:    "Columns",
	seTabIndexes:    "Indexes",
	seTabFK:         "Foreign Keys",
	seTabChecks:     "Checks",
	seTabTriggers:   "Triggers",
	seTabDefinition: "Definition",
}

// NewSchemaEditor creates a hidden editor.
func NewSchemaEditor() SchemaEditor {
	return SchemaEditor{}
}

// Show opens the editor for a table with its current column metadata.
func (e *SchemaEditor) Show(table string, driver db.Driver, columns []db.TableColumnInfo) {
	e.table = table
	e.driver = driver
	e.columns = append([]db.TableColumnInfo(nil), columns...)
	e.visible = true
	e.errMsg = ""
	e.notice = ""
	e.cursorRow = 0
	e.cursorCol = 0
	e.editing = false
	e.pendingD = false

	// Start on the editable Columns tab; read-only metadata loads async.
	e.activeTab = seTabColumns
	e.structure = structureData{}
	e.structLoaded = false
	e.roCursor = 0
	e.roScroll = 0
	e.triggerExpanded = false
	e.lastClickTime = time.Time{}

	e.rows = make([][]string, len(columns))
	e.rowOrigin = make([]int, len(columns))
	for i, col := range columns {
		e.rows[i] = []string{
			col.Name,
			col.Type,
			nullDisplay(col.NotNull),
			defaultDisplay(col.HasDefault, col.DefaultValue),
		}
		e.rowOrigin[i] = i
	}
	e.computeColWidths()
}

// Hide closes the editor.
func (e *SchemaEditor) Hide() {
	e.visible = false
	e.table = ""
	e.columns = nil
	e.rows = nil
	e.rowOrigin = nil
	e.errMsg = ""
	e.notice = ""
	e.editing = false
	e.pendingD = false
	e.structure = structureData{}
	e.structLoaded = false
	e.roCursor = 0
	e.roScroll = 0
	e.triggerExpanded = false
}

// IsVisible reports whether the editor is open.
func (e SchemaEditor) IsVisible() bool { return e.visible }

// IsEditing reports whether a cell is being edited inline.
func (e SchemaEditor) IsEditing() bool { return e.editing }

// Table returns the table being edited.
func (e SchemaEditor) Table() string { return e.table }

// SetError sets an error message from a failed DDL execution.
func (e *SchemaEditor) SetError(msg string) { e.errMsg = msg }

// SetNotice sets a transient informational message.
func (e *SchemaEditor) SetNotice(msg string) { e.notice = msg }

// SetColumns replaces column metadata (e.g. after a successful DDL) and
// refreshes the grid rows in place, preserving the cursor if possible.
func (e *SchemaEditor) SetColumns(columns []db.TableColumnInfo) {
	e.columns = append([]db.TableColumnInfo(nil), columns...)
	prevRow := e.cursorRow
	prevCol := e.cursorCol
	e.rows = make([][]string, len(columns))
	e.rowOrigin = make([]int, len(columns))
	for i, col := range columns {
		e.rows[i] = []string{
			col.Name,
			col.Type,
			nullDisplay(col.NotNull),
			defaultDisplay(col.HasDefault, col.DefaultValue),
		}
		e.rowOrigin[i] = i
	}
	if prevRow >= len(e.rows) {
		prevRow = len(e.rows) - 1
	}
	if prevRow < 0 {
		prevRow = 0
	}
	e.cursorRow = prevRow
	e.cursorCol = prevCol
}

// SetSize sets the dimensions available for the editor.
func (e *SchemaEditor) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.computeColWidths()
}

func (e *SchemaEditor) computeColWidths() {
	available := e.width - 1
	if available < 20 {
		available = 20
	}
	nameW := 20
	typeW := 18
	nullW := 8
	defaultW := available - nameW - typeW - nullW - (seColCount * 3)
	if defaultW < 10 {
		defaultW = 10
	}
	e.colWidths = []int{nameW, typeW, nullW, defaultW}
}

func (e SchemaEditor) maxVisibleRows() int {
	max := e.height - 6
	if max < 1 {
		max = 1
	}
	return max
}

// cellEditable reports whether the current cell can be edited for this driver.
func (e SchemaEditor) cellEditable(row, col int) bool {
	// New (pending) rows are always fully editable.
	if e.isNewRow(row) {
		return true
	}
	origIdx := e.rowOrigin[row]
	if origIdx < 0 || origIdx >= len(e.columns) {
		return false
	}
	info := e.columns[origIdx]
	// Auto-increment and PK columns are managed by the DB — disallow edits to
	// avoid confusing failures.
	if info.AutoIncrement {
		return false
	}
	if col == seColName {
		// RENAME COLUMN is supported on both SQLite and MySQL, but not for
		// auto-increment PK columns.
		return !info.PrimaryKey || !info.AutoIncrement
	}
	// Type, Null, Default all require MODIFY COLUMN (MySQL) or ALTER COLUMN (Postgres).
	return e.driver == db.DriverMySQL || e.driver == db.DriverPostgres
}

// isNewRow reports whether the grid row is a pending new column (not yet in the DB).
func (e SchemaEditor) isNewRow(row int) bool {
	return row >= 0 && row < len(e.rowOrigin) && e.rowOrigin[row] == -1
}

// IsNewRow is the exported version for app.go to check the cursor row.
func (e SchemaEditor) IsNewRow() bool {
	return e.isNewRow(e.cursorRow)
}

// addRowBelow inserts a blank new-row entry below the cursor.
func (e *SchemaEditor) addRowBelow() {
	insertAt := e.cursorRow + 1
	newRow := []string{"", "", "yes", ""}
	e.rows = append(e.rows, nil)
	copy(e.rows[insertAt+1:], e.rows[insertAt:])
	e.rows[insertAt] = newRow

	e.rowOrigin = append(e.rowOrigin, 0)
	copy(e.rowOrigin[insertAt+1:], e.rowOrigin[insertAt:])
	e.rowOrigin[insertAt] = -1

	e.cursorRow = insertAt
	e.cursorCol = seColName
}

// removeRow deletes the grid row at cursorRow without any DB action.
func (e *SchemaEditor) removeRow() {
	if len(e.rows) <= 1 {
		return
	}
	e.rows = append(e.rows[:e.cursorRow], e.rows[e.cursorRow+1:]...)
	e.rowOrigin = append(e.rowOrigin[:e.cursorRow], e.rowOrigin[e.cursorRow+1:]...)
	if e.cursorRow >= len(e.rows) {
		e.cursorRow = len(e.rows) - 1
	}
}

// Update handles keyboard input.
func (e SchemaEditor) Update(msg tea.Msg) (SchemaEditor, tea.Cmd) {
	if !e.visible {
		return e, nil
	}
	var cmd tea.Cmd

	// Inline edit mode: route to the textinput except for commit/cancel.
	if e.editing {
		switch m := msg.(type) {
		case tea.KeyMsg:
			switch m.String() {
			case "enter":
				e.commitCellEdit()
				return e, nil
			case "esc", "ctrl+c":
				e.editing = false
				return e, nil
			case "tab":
				e.commitCellEdit()
				e.cursorRight()
				return e, nil
			}
		}
		e.editInput, cmd = e.editInput.Update(msg)
		return e, cmd
	}

	// Tab switching (H/L) is available on every tab except while editing.
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "L":
			e.switchTab(1)
			return e, nil
		case "H":
			e.switchTab(-1)
			return e, nil
		}
	}

	// Read-only metadata tabs: navigation only.
	if e.activeTab != seTabColumns {
		e.updateReadOnly(msg)
		return e, nil
	}

	// Columns tab: grid navigation and actions.
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			e.pendingD = false
			e.cursorUp()
			return e, nil
		case "down", "j":
			e.pendingD = false
			e.cursorDown()
			return e, nil
		case "left", "h":
			e.pendingD = false
			e.cursorLeft()
			return e, nil
		case "right", "l":
			e.pendingD = false
			e.cursorRight()
			return e, nil
		case "tab":
			e.pendingD = false
			e.cursorRight()
			return e, nil
		case "shift+tab":
			e.pendingD = false
			e.cursorLeft()
			return e, nil
		case "e", "i":
			e.pendingD = false
			e.startCellEdit()
			return e, nil
		case "o":
			e.pendingD = false
			e.addRowBelow()
			return e, nil
		case "d":
			if e.pendingD {
				e.pendingD = false
				if e.isNewRow(e.cursorRow) {
					e.removeRow()
				}
				// For existing rows, app.go intercepts the second 'd' and
				// runs the confirmation flow — we won't reach here.
			} else {
				e.pendingD = true
			}
			return e, nil
		}
	}
	return e, nil
}

func (e *SchemaEditor) cursorUp() {
	if e.cursorRow > 0 {
		e.cursorRow--
	}
}

func (e *SchemaEditor) cursorDown() {
	if e.cursorRow < len(e.rows)-1 {
		e.cursorRow++
	}
}

func (e *SchemaEditor) cursorLeft() {
	if e.cursorCol > 0 {
		e.cursorCol--
	}
}

func (e *SchemaEditor) cursorRight() {
	if e.cursorCol < seColCount-1 {
		e.cursorCol++
	} else if e.cursorRow < len(e.rows)-1 {
		e.cursorCol = 0
		e.cursorRow++
	}
}

func (e *SchemaEditor) startCellEdit() {
	if !e.cellEditable(e.cursorRow, e.cursorCol) {
		e.notice = editBlockedNotice(e.driver, e.cursorCol)
		return
	}
	e.notice = ""
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.SetValue(e.rows[e.cursorRow][e.cursorCol])
	w := e.colWidths[e.cursorCol] - 1
	if w < 5 {
		w = 5
	}
	ti.Width = w
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorEdit)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colorFg).Background(colorBg)
	ti.Focus()
	e.editInput = ti
	e.editing = true
}

// commitCellEdit stores the edited value back into the grid row. The actual
// DDL is built and executed by BuildPendingDDL() called from app.go.
func (e *SchemaEditor) commitCellEdit() {
	if !e.editing {
		return
	}
	e.rows[e.cursorRow][e.cursorCol] = e.editInput.Value()
	e.editing = false
}

// PendingEditDDL checks if the current cell value differs from the original
// column metadata and, if so, returns the DDL statement and action to run.
// Returns ok=false when there is no change to apply.
func (e SchemaEditor) PendingEditDDL() (sql string, action db.SchemaAction, err string) {
	// New rows: all cells editable, build ADD COLUMN on commit.
	if e.isNewRow(e.cursorRow) {
		return e.buildAddColumnDDL()
	}
	if e.cursorRow < 0 || e.cursorRow >= len(e.rowOrigin) {
		return "", "", "no column selected"
	}
	origIdx := e.rowOrigin[e.cursorRow]
	if origIdx < 0 || origIdx >= len(e.columns) {
		return "", "", "no column selected"
	}
	col := e.columns[origIdx]
	newVal := strings.TrimSpace(e.rows[e.cursorRow][e.cursorCol])

	switch e.cursorCol {
	case seColName:
		if newVal == col.Name {
			return "", "", ""
		}
		sql, err := db.BuildRenameColumnSQL(e.driver, e.table, col.Name, newVal, e.columnNames())
		if err != nil {
			return "", db.SchemaRenameColumn, err.Error()
		}
		return sql, db.SchemaRenameColumn, ""

	case seColType:
		if newVal == col.Type {
			return "", "", ""
		}
		def := db.ColumnDefFromInfo(col)
		def.Type = newVal
		sql, err := db.BuildModifyColumnSQL(e.driver, e.table, def)
		if err != nil {
			return "", db.SchemaModifyType, err.Error()
		}
		return sql, db.SchemaModifyType, ""

	case seColNull:
		newNotNull := parseNullCell(newVal)
		if newNotNull == col.NotNull {
			return "", "", ""
		}
		def := db.ColumnDefFromInfo(col)
		def.NotNull = newNotNull
		sql, err := db.BuildModifyColumnSQL(e.driver, e.table, def)
		if err != nil {
			return "", db.SchemaModifyNullable, err.Error()
		}
		return sql, db.SchemaModifyNullable, ""

	case seColDefault:
		newDefault := newVal
		changed := false
		if newDefault == "" && col.HasDefault {
			changed = true
		} else if newDefault != "" && (!col.HasDefault || newDefault != col.DefaultValue) {
			changed = true
		}
		if !changed {
			return "", "", ""
		}
		def := db.ColumnDefFromInfo(col)
		if newDefault == "" {
			def.HasDefault = false
			def.Default = ""
		} else {
			def.HasDefault = true
			def.Default = newDefault
		}
		sql, err := db.BuildModifyColumnSQL(e.driver, e.table, def)
		if err != nil {
			return "", db.SchemaModifyDefault, err.Error()
		}
		return sql, db.SchemaModifyDefault, ""
	}
	return "", "", ""
}

// buildAddColumnDDL constructs an ADD COLUMN statement from the current row's
// cells. Called when the user presses enter on a pending new row.
func (e SchemaEditor) buildAddColumnDDL() (string, db.SchemaAction, string) {
	row := e.rows[e.cursorRow]
	name := strings.TrimSpace(row[seColName])
	colType := strings.TrimSpace(row[seColType])
	nullable, errMsg := parseNullable(row[seColNull])
	if errMsg != "" {
		return "", db.SchemaAddColumn, errMsg
	}
	def := db.ColumnDef{
		Name:    name,
		Type:    colType,
		NotNull: !nullable,
	}
	if dv := strings.TrimSpace(row[seColDefault]); dv != "" {
		def.HasDefault = true
		def.Default = dv
	}
	sql, err := db.BuildAddColumnSQL(e.driver, e.table, def, e.columnNames())
	if err != nil {
		return "", db.SchemaAddColumn, err.Error()
	}
	return sql, db.SchemaAddColumn, ""
}

// PendingDropColumn returns the column info for a dd-removal so app.go can
// run it through the existing confirmation + BuildDropColumnSQL flow.
// Returns ok=false for new rows (they're removed locally without confirm).
func (e SchemaEditor) PendingDropColumn() (db.TableColumnInfo, bool) {
	if e.isNewRow(e.cursorRow) {
		return db.TableColumnInfo{}, false
	}
	if e.cursorRow < 0 || e.cursorRow >= len(e.rowOrigin) {
		return db.TableColumnInfo{}, false
	}
	origIdx := e.rowOrigin[e.cursorRow]
	if origIdx < 0 || origIdx >= len(e.columns) {
		return db.TableColumnInfo{}, false
	}
	return e.columns[origIdx], true
}

func (e SchemaEditor) columnNames() []string {
	names := make([]string, len(e.columns))
	for i, c := range e.columns {
		names[i] = c.Name
	}
	return names
}

// ActiveTab returns the index of the currently selected structure tab.
func (e SchemaEditor) ActiveTab() int { return e.activeTab }

// SetActiveTab selects a structure tab if it is available for the current
// relation, resetting the read-only cursor/scroll. Used by :indexes / :columns
// / :fk / :constraints to land on the right tab after openSchemaPanel.
func (e *SchemaEditor) SetActiveTab(tab int) {
	if !e.tabAvailable(tab) {
		return
	}
	e.activeTab = tab
	e.roCursor = 0
	e.roScroll = 0
	e.triggerExpanded = false
}

// LoadStructure populates the read-only metadata tabs from an async fetch.
func (e *SchemaEditor) LoadStructure(data structureData) {
	e.structure = data
	e.structLoaded = true
	// If the active tab dropped out of the available set (e.g. Definition for a
	// table once we learn it isn't a view), fall back to Columns.
	if !e.tabAvailable(e.activeTab) {
		e.activeTab = seTabColumns
	}
}

// tabAvailable reports whether a tab is shown for the current relation.
func (e SchemaEditor) tabAvailable(tab int) bool {
	for _, t := range e.availableTabs() {
		if t == tab {
			return true
		}
	}
	return false
}

// availableTabs returns the tab set: Columns/Indexes/FK/Checks/Triggers
// always, plus Definition when the relation is a view.
func (e SchemaEditor) availableTabs() []int {
	tabs := []int{seTabColumns, seTabIndexes, seTabFK, seTabChecks, seTabTriggers}
	if e.structure.viewDef != "" {
		tabs = append(tabs, seTabDefinition)
	}
	return tabs
}

// switchTab moves to the next/previous available tab and resets the read-only
// cursor/scroll.
func (e *SchemaEditor) switchTab(delta int) {
	tabs := e.availableTabs()
	cur := 0
	for i, t := range tabs {
		if t == e.activeTab {
			cur = i
			break
		}
	}
	cur += delta
	if cur < 0 {
		cur = 0
	}
	if cur >= len(tabs) {
		cur = len(tabs) - 1
	}
	e.activeTab = tabs[cur]
	e.roCursor = 0
	e.roScroll = 0
	e.triggerExpanded = false
}

// roCount returns the number of selectable rows in the active read-only tab.
func (e SchemaEditor) roCount() int {
	switch e.activeTab {
	case seTabIndexes:
		return len(e.structure.indexes)
	case seTabFK:
		return len(e.structure.fks)
	case seTabChecks:
		return len(e.structure.checks)
	case seTabTriggers:
		return len(e.structure.triggers)
	}
	return 0
}

// roVisible returns the viewport height (in rows) for read-only tab content.
func (e SchemaEditor) roVisible() int {
	// Reserve title (1), blank (1), tab bar (1), blank (1) = 4 lines overhead.
	v := e.height - 4
	if v < 1 {
		v = 1
	}
	return v
}

func (e *SchemaEditor) roDown() {
	n := e.roCount()
	if n == 0 {
		return
	}
	if e.roCursor < n-1 {
		e.roCursor++
	}
	e.roClampScroll()
}

func (e *SchemaEditor) roUp() {
	if e.roCursor > 0 {
		e.roCursor--
	}
	e.roClampScroll()
}

func (e *SchemaEditor) roClampScroll() {
	vh := e.roVisible()
	if e.roCursor < e.roScroll {
		e.roScroll = e.roCursor
	}
	if e.roCursor >= e.roScroll+vh {
		e.roScroll = e.roCursor - vh + 1
	}
}

// cursorTriggerStatement returns the statement of the trigger under the
// read-only cursor ("" when out of range).
func (e SchemaEditor) cursorTriggerStatement() string {
	if e.roCursor < 0 || e.roCursor >= len(e.structure.triggers) {
		return ""
	}
	return e.structure.triggers[e.roCursor].Statement
}

// codeMode reports whether the active tab renders a scrollable code listing
// (Definition, or an expanded trigger statement) whose navigation scrolls
// lines rather than moving a row cursor.
func (e SchemaEditor) codeMode() bool {
	return e.activeTab == seTabDefinition ||
		(e.activeTab == seTabTriggers && e.triggerExpanded)
}

// activeCodeText returns the code listing shown in code mode ("" otherwise).
func (e SchemaEditor) activeCodeText() string {
	if e.activeTab == seTabDefinition {
		return e.structure.viewDef
	}
	if e.activeTab == seTabTriggers && e.triggerExpanded {
		return e.cursorTriggerStatement()
	}
	return ""
}

// maxCodeScroll returns the largest valid top-line offset for the current code
// listing so it never scrolls past the end.
func (e SchemaEditor) maxCodeScroll() int {
	lines := len(strings.Split(strings.TrimRight(e.activeCodeText(), "\n"), "\n"))
	max := lines - e.roVisible()
	if max < 0 {
		max = 0
	}
	return max
}

// updateReadOnly handles keys on the read-only metadata tabs.
func (e *SchemaEditor) updateReadOnly(msg tea.Msg) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return
	}
	code := e.codeMode()
	switch km.String() {
	case "j", "down":
		if code {
			if e.roScroll < e.maxCodeScroll() {
				e.roScroll++
			}
		} else {
			e.roDown()
		}
	case "k", "up":
		if code {
			if e.roScroll > 0 {
				e.roScroll--
			}
		} else {
			e.roUp()
		}
	case "g":
		e.roCursor = 0
		e.roScroll = 0
	case "G":
		if code {
			e.roScroll = e.maxCodeScroll()
		} else {
			e.roCursor = e.roCount() - 1
			if e.roCursor < 0 {
				e.roCursor = 0
			}
			e.roClampScroll()
		}
	case "enter":
		if e.activeTab == seTabTriggers {
			if e.triggerExpanded {
				e.triggerExpanded = false
			} else if e.roCount() > 0 {
				e.triggerExpanded = true
			}
			e.roScroll = 0
			e.roClampScroll()
		}
	case "esc":
		if e.activeTab == seTabTriggers && e.triggerExpanded {
			e.triggerExpanded = false
			e.roScroll = 0
			e.roClampScroll()
		}
	}
}

// tabAtX maps a content-relative X (0-based, inside the panel border) to the
// structure tab whose rendered segment covers it. The tab bar left-aligns at
// X=0; each tab is rendered with 1-char padding on both sides and a single
// space between tabs (a click on that gap resolves to no tab). Returns the
// tab index and true when X lands on a tab.
func (e SchemaEditor) tabAtX(x int) (int, bool) {
	if x < 0 {
		return 0, false
	}
	cursor := 0
	for _, t := range e.availableTabs() {
		segWidth := len(seTabLabels[t]) + 2 // 1-char padding each side
		if x < cursor+segWidth {
			return t, true
		}
		cursor += segWidth + 1 // +1 for the space separator after this tab
		// x lands in the gap before the next tab → no tab.
		if x < cursor {
			return 0, false
		}
	}
	return 0, false
}

// gridColumnAtX maps a content-relative X to a Columns-grid column index using
// the fixed colWidths. The grid's leading border is at X=0; each column spans
// colWidths[j]+2 content chars (1-char cell padding) followed by a 1-char
// separator. Returns -1 when X is past the last column.
func (e SchemaEditor) gridColumnAtX(x int) int {
	if x < 0 || len(e.colWidths) == 0 {
		return -1
	}
	cursor := 1 // skip the leading │
	for j, w := range e.colWidths {
		cellW := w + 2
		if x < cursor+cellW {
			return j
		}
		cursor += cellW + 1 // +1 for the trailing │
	}
	return -1
}

// Click handles a left mouse-click at content-relative coordinates (x,y),
// where (0,0) is the first cell inside the panel border. The tab-bar row
// (y==2) switches tabs; the grid area moves the cursor — Columns grid to
// (row, col), read-only tabs to a row. A double-click on an editable Columns
// cell starts an inline edit, mirroring the results panel.
func (e *SchemaEditor) Click(x, y int) {
	if x < 0 || y < 0 {
		return
	}
	// A click always abandons an in-progress inline edit (cancel, not commit —
	// press enter to save) so the edit input never lingers on the wrong cell
	// after the cursor moves away.
	if e.editing {
		e.editing = false
	}
	// Tab-bar row.
	if y == 2 {
		if t, ok := e.tabAtX(x); ok && t != e.activeTab {
			e.activeTab = t
			e.roCursor = 0
			e.roScroll = 0
			e.triggerExpanded = false
		}
		return
	}
	// Grid box: top border (y==4), header (5), separator (6), data from y==7.
	if y < 4 {
		return
	}
	if y < 7 {
		return // header / borders: no action
	}
	dataRel := y - 7
	if e.activeTab == seTabColumns {
		row := e.gridTopRow() + dataRel
		col := e.gridColumnAtX(x)
		if row < 0 || row >= len(e.rows) || col < 0 || col >= seColCount {
			return
		}
		// Double-click on the same editable cell → inline edit.
		if !e.editing &&
			!e.lastClickTime.IsZero() &&
			time.Since(e.lastClickTime) <= doubleClickInterval &&
			e.lastClickRow == row && e.lastClickCol == col &&
			e.cellEditable(row, col) {
			e.lastClickTime = time.Time{}
			e.cursorRow = row
			e.cursorCol = col
			e.startCellEdit()
			return
		}
		e.lastClickTime = time.Now()
		e.lastClickRow = row
		e.lastClickCol = col
		e.cursorRow = row
		e.cursorCol = col
		return
	}
	// Read-only metadata tab: row selection only.
	n := e.roCount()
	row := e.roScroll + dataRel
	if row < 0 || row >= n {
		return
	}
	e.roCursor = row
	e.roClampScroll()
}

// Wheel scrolls the active grid by one row in the given direction (+1 = down,
// -1 = up). The Columns viewport follows the cursor, so the cursor moves; on
// the read-only tabs the row cursor moves, matching j/k.
func (e *SchemaEditor) Wheel(delta int) {
	if e.activeTab == seTabColumns {
		if delta > 0 {
			e.cursorDown()
		} else {
			e.cursorUp()
		}
		return
	}
	if delta > 0 {
		e.roDown()
	} else {
		e.roUp()
	}
}

// View renders the editor: header + grid + footer.
func (e SchemaEditor) View() string {
	if !e.visible {
		return ""
	}

	var lines []string
	lines = append(lines, e.renderHeader())
	lines = append(lines, "")
	lines = append(lines, e.renderTabBar())
	lines = append(lines, "")

	switch e.activeTab {
	case seTabIndexes:
		lines = append(lines, e.renderIndexesTab())
	case seTabFK:
		lines = append(lines, e.renderFKTab())
	case seTabChecks:
		lines = append(lines, e.renderChecksTab())
	case seTabTriggers:
		lines = append(lines, e.renderTriggersTab())
	case seTabDefinition:
		lines = append(lines, e.renderDefinitionTab())
	default:
		lines = append(lines, e.renderGrid())
	}

	// Errors/notices only surface on the Columns tab (where edits happen).
	if e.activeTab == seTabColumns {
		if e.errMsg != "" {
			lines = append(lines, "")
			lines = append(lines, errorStyle.Render(e.errMsg))
		} else if e.notice != "" {
			lines = append(lines, "")
			lines = append(lines, mutedStyle.Render(e.notice))
		}

		if e.pendingD {
			lines = append(lines, "")
			lines = append(lines, lipgloss.NewStyle().Foreground(colorAccent).Render("d...")+mutedStyle.Render("  press d again to drop column   esc cancel"))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderHeader renders the title line with a table/view badge.
func (e SchemaEditor) renderHeader() string {
	kind := "table"
	if e.structure.viewDef != "" {
		kind = "view"
	}
	badge := lipgloss.NewStyle().Foreground(colorAccent).Render(fmt.Sprintf("[%s]", kind))
	return titleStyle.Render(fmt.Sprintf("Structure: %s", e.table)) + "  " + badge
}

// renderTabBar renders the H/L-selectable tab strip.
func (e SchemaEditor) renderTabBar() string {
	active := lipgloss.NewStyle().Foreground(colorBg).Background(colorPrimary).Bold(true).Padding(0, 1)
	inactive := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)

	var parts []string
	for _, t := range e.availableTabs() {
		label := seTabLabels[t]
		if t == e.activeTab {
			parts = append(parts, active.Render(label))
		} else {
			parts = append(parts, inactive.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

// roTableWidth is the content width available to a read-only box table.
func (e SchemaEditor) roTableWidth() int {
	w := e.width - 2 // outer padding
	if w < 20 {
		w = 20
	}
	return w
}

// renderIndexesTab renders the Indexes tab as a box table.
func (e SchemaEditor) renderIndexesTab() string {
	if !e.structLoaded {
		return mutedStyle.Render("Loading indexes…")
	}
	if e.structure.indexErr != "" {
		return errorStyle.Render("Indexes unavailable: " + e.structure.indexErr)
	}
	if len(e.structure.indexes) == 0 {
		return mutedStyle.Render("(no indexes)")
	}

	headers := []string{"Name", "Unique", "Columns"}
	rows := make([][]string, len(e.structure.indexes))
	for i, ix := range e.structure.indexes {
		unique := ""
		if ix.Unique {
			unique = "yes"
		}
		rows[i] = []string{ix.Name, unique, strings.Join(ix.Columns, ", ")}
	}
	window := e.windowRows(rows)
	out := renderBoxTable(headers, window, e.roTableWidth())

	// Partial-index predicates are appended under their index row.
	var extra []string
	for _, ix := range e.structure.indexes {
		if ix.Partial != "" {
			extra = append(extra, mutedStyle.Render("  WHERE "+ix.Partial))
		}
	}
	if len(extra) > 0 {
		out += "\n\n" + mutedStyle.Render("Partial predicates:") + "\n" + strings.Join(extra, "\n")
	}
	return out
}

// renderFKTab renders the Foreign Keys tab as a box table.
func (e SchemaEditor) renderFKTab() string {
	if !e.structLoaded {
		return mutedStyle.Render("Loading foreign keys…")
	}
	if len(e.structure.fks) == 0 {
		return mutedStyle.Render("(no foreign keys)")
	}
	headers := []string{"Column", "References"}
	rows := make([][]string, len(e.structure.fks))
	for i, fk := range e.structure.fks {
		rows[i] = []string{fk.Column, fmt.Sprintf("%s(%s)", fk.RefTable, fk.RefColumn)}
	}
	window := e.windowRows(rows)
	return renderBoxTable(headers, window, e.roTableWidth())
}

// renderChecksTab renders the Checks tab as a box table. Check expressions
// can be long; the box renderer shrinks columns to fit the panel width (widen
// the terminal or the editor to see more of the expression).
func (e SchemaEditor) renderChecksTab() string {
	if !e.structLoaded {
		return mutedStyle.Render("Loading check constraints…")
	}
	if e.structure.checkErr != "" {
		return errorStyle.Render("Checks unavailable: " + e.structure.checkErr)
	}
	if len(e.structure.checks) == 0 {
		return mutedStyle.Render("(no check constraints)")
	}
	headers := []string{"Name", "Column", "Expression"}
	rows := make([][]string, len(e.structure.checks))
	for i, c := range e.structure.checks {
		rows[i] = []string{c.Name, c.Column, c.Expression}
	}
	window := e.windowRows(rows)
	return renderBoxTable(headers, window, e.roTableWidth())
}

// renderTriggersTab renders the Triggers tab: a summary box table, or the
// expanded statement of the selected trigger after `enter`.
func (e SchemaEditor) renderTriggersTab() string {
	if !e.structLoaded {
		return mutedStyle.Render("Loading triggers…")
	}
	if e.structure.triggerErr != "" {
		return errorStyle.Render("Triggers unavailable: " + e.structure.triggerErr)
	}
	if len(e.structure.triggers) == 0 {
		return mutedStyle.Render("(no triggers)")
	}

	if e.triggerExpanded {
		return e.renderTriggerStatement()
	}

	headers := []string{"Name", "Timing", "Event"}
	rows := make([][]string, len(e.structure.triggers))
	for i, tr := range e.structure.triggers {
		rows[i] = []string{tr.Name, tr.Timing, tr.Event}
	}
	window := e.windowRows(rows)
	hint := mutedStyle.Render("\nenter: view statement")
	return renderBoxTable(headers, window, e.roTableWidth()) + hint
}

// renderTriggerStatement renders the selected trigger's statement as a code
// listing, scrollable via j/k.
func (e SchemaEditor) renderTriggerStatement() string {
	if e.roCursor < 0 || e.roCursor >= len(e.structure.triggers) {
		return mutedStyle.Render("(no trigger selected)")
	}
	tr := e.structure.triggers[e.roCursor]
	head := lipgloss.NewStyle().Foreground(colorLabel).Bold(true).
		Render(fmt.Sprintf("← esc back   %s (%s %s)", tr.Name, tr.Timing, tr.Event))
	return head + "\n\n" + e.renderCodeScroll(tr.Statement)
}

// renderDefinitionTab renders the view definition as a scrollable code listing.
func (e SchemaEditor) renderDefinitionTab() string {
	if !e.structLoaded {
		return mutedStyle.Render("Loading definition…")
	}
	if e.structure.viewDef == "" {
		return mutedStyle.Render("This relation is not a view.")
	}
	return e.renderCodeScroll(e.structure.viewDef)
}

// renderCodeScroll renders a multi-line string as a scrollable code listing
// using roScroll as the top-line offset (clamped to the end). Used by the
// Definition tab and the expanded trigger statement.
func (e SchemaEditor) renderCodeScroll(text string) string {
	allLines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	max := len(allLines) - e.roVisible()
	if max < 0 {
		max = 0
	}
	if e.roScroll > max {
		e.roScroll = max
	}
	if e.roScroll < 0 {
		e.roScroll = 0
	}
	end := e.roScroll + e.roVisible()
	if end > len(allLines) {
		end = len(allLines)
	}
	codeStyle := lipgloss.NewStyle().Foreground(colorFg)
	w := e.roTableWidth() - 1
	var out []string
	for i := e.roScroll; i < end; i++ {
		out = append(out, codeStyle.Render(" "+truncateSidebarLine(allLines[i], w)))
	}
	return strings.Join(out, "\n")
}

// windowRows slices a read-only tab's rows to the visible scroll window.
func (e SchemaEditor) windowRows(rows [][]string) (window [][]string) {
	total := len(rows)
	if total == 0 {
		return nil
	}
	// For box tables reserve ~3 lines of border/header overhead per the tab.
	vh := e.roVisible() - 3
	if vh < 1 {
		vh = 1
	}
	if e.roCursor < e.roScroll {
		e.roScroll = e.roCursor
	}
	if e.roCursor >= e.roScroll+vh {
		e.roScroll = e.roCursor - vh + 1
	}
	if e.roScroll > total-vh && total >= vh {
		e.roScroll = total - vh
	}
	if e.roScroll < 0 {
		e.roScroll = 0
	}
	end := e.roScroll + vh
	if end > total {
		end = total
	}
	return rows[e.roScroll:end]
}

// gridTopRow returns the first visible data-row index of the Columns grid,
// keeping the cursor roughly centered in the viewport. Shared by renderGrid
// and the click→row mapping so the two never drift apart.
func (e SchemaEditor) gridTopRow() int {
	maxVisible := e.maxVisibleRows()
	rowStart := e.cursorRow - maxVisible/2
	if rowStart < 0 {
		rowStart = 0
	}
	rowEnd := rowStart + maxVisible
	if rowEnd > len(e.rows) {
		rowEnd = len(e.rows)
	}
	rowStart = rowEnd - maxVisible
	if rowStart < 0 {
		rowStart = 0
	}
	return rowStart
}

func (e SchemaEditor) renderGrid() string {
	borderColor := lipgloss.NewStyle().Foreground(colorBorder)

	rowStart := e.gridTopRow()
	rowEnd := rowStart + e.maxVisibleRows()
	if rowEnd > len(e.rows) {
		rowEnd = len(e.rows)
	}

	var b strings.Builder

	// Top border.
	b.WriteString(borderColor.Render("┌"))
	for j := 0; j < seColCount; j++ {
		w := e.colWidths[j] + 2
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if j < seColCount-1 {
			b.WriteString(borderColor.Render("┬"))
		}
	}
	b.WriteString(borderColor.Render("┐"))
	b.WriteString("\n")

	// Header row.
	b.WriteString(borderColor.Render("│"))
	for j := 0; j < seColCount; j++ {
		cell := truncateCell(seHeaders[j], e.colWidths[j])
		style := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
		b.WriteString(style.Render(" " + cell + " "))
		b.WriteString(borderColor.Render("│"))
	}
	b.WriteString("\n")

	// Header separator.
	b.WriteString(borderColor.Render("├"))
	for j := 0; j < seColCount; j++ {
		w := e.colWidths[j] + 2
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if j < seColCount-1 {
			b.WriteString(borderColor.Render("┼"))
		}
	}
	b.WriteString(borderColor.Render("┤"))
	b.WriteString("\n")

	// Data rows.
	for rowIdx := rowStart; rowIdx < rowEnd; rowIdx++ {
		row := e.rows[rowIdx]
		isCursorRow := rowIdx == e.cursorRow
		b.WriteString(borderColor.Render("│"))
		for j := 0; j < seColCount; j++ {
			isCursorCell := isCursorRow && j == e.cursorCol

			if e.editing && isCursorCell {
				b.WriteString(" " + renderEditInput(e.editInput, e.colWidths[j], colorEdit) + " ")
				b.WriteString(borderColor.Render("│"))
				continue
			}

			val := row[j]
			editable := e.cellEditable(rowIdx, j)

			display := val
			isPlaceholder := false
			if val == "" {
				display = "(none)"
				isPlaceholder = true
			}

			cell := truncateCell(display, e.colWidths[j])

			var style lipgloss.Style
			switch {
			case isCursorCell && editable:
				style = lipgloss.NewStyle().Foreground(colorBg).Background(colorPrimary)
			case isCursorCell && !editable:
				style = lipgloss.NewStyle().Foreground(colorMuted).Background(colorHighlight)
			case !editable:
				style = mutedStyle
			case isPlaceholder:
				style = mutedStyle
			default:
				style = lipgloss.NewStyle().Foreground(colorFg)
			}
			b.WriteString(style.Render(" " + cell + " "))
			b.WriteString(borderColor.Render("│"))
		}
		b.WriteString("\n")
	}

	// Bottom border.
	b.WriteString(borderColor.Render("└"))
	for j := 0; j < seColCount; j++ {
		w := e.colWidths[j] + 2
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if j < seColCount-1 {
			b.WriteString(borderColor.Render("┴"))
		}
	}
	b.WriteString(borderColor.Render("┘"))

	return b.String()
}

func nullDisplay(notNull bool) string {
	if notNull {
		return "NOT NULL"
	}
	return "NULL"
}

func defaultDisplay(hasDefault bool, value string) string {
	if !hasDefault {
		return ""
	}
	if value == "" {
		return "NULL"
	}
	return value
}

// parseNullCell interprets the Null column value. Accepts NOT NULL / NULL and
// yes/no shorthand for convenience.
func parseNullCell(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "not null", "no", "n", "false":
		return true
	case "null", "yes", "y", "true", "":
		return false
	}
	return false
}

func editBlockedNotice(driver db.Driver, col int) string {
	if driver == db.DriverSQLite && col != seColName {
		return fmt.Sprintf("%s can only be changed on MySQL/Postgres (SQLite supports rename only)", seHeaders[col])
	}
	return fmt.Sprintf("this column cannot be edited")
}
