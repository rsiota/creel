package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/db"
)

// InspectorWidth is the column width reserved for the inspector panel
// (including borders) when it is visible.
const InspectorWidth = 45

// Inspector is a right-side panel that displays all column values of the
// currently selected result row as a vertical form. Each field shows the
// column name as a label above a bordered value box. When the underlying
// results are editable, individual fields can be modified inline.
type Inspector struct {
	width          int
	height         int
	visible        bool
	cursorField    int
	editing        bool
	editInput      textinput.Model
	scrollRow      int
	pendingG       bool
	filtering      bool
	filter         string
	inserting      bool
	insertValues   map[int]string
	editingCol     int
	editOriginal   string          // value loaded into editInput; used to skip no-op commits
	jsonExpanded   bool            // field-level fold: tree visible for focused JSON
	jsonTreeOpen   map[string]bool // which container paths are expanded
	jsonTreeCursor int             // selected row in the visible tree
	jsonTreeScroll int             // first visible tree row (viewport)
}

// NewInspector creates a new inspector component.
func NewInspector() Inspector {
	return Inspector{}
}

// Toggle shows or hides the inspector.
func (i *Inspector) Toggle() {
	i.visible = !i.visible
	i.editing = false
	i.cursorField = 0
	i.scrollRow = 0
	i.filtering = false
	i.filter = ""
	i.inserting = false
	i.insertValues = nil
	i.clearJSONTree()
}

// IsInserting returns whether the inspector is in new-record mode.
func (i Inspector) IsInserting() bool {
	return i.inserting
}

// StartInsert enters new-record mode with empty field values.
func (i *Inspector) StartInsert() {
	i.inserting = true
	i.insertValues = make(map[int]string)
	i.cursorField = 0
	i.scrollRow = 0
	i.editing = false
	i.filtering = false
	i.filter = ""
	i.clearJSONTree()
}

// SetInsertValues replaces pending insert field values (column index → text).
// Call after StartInsert to prefill FKs for "insert related".
func (i *Inspector) SetInsertValues(vals map[int]string) {
	if !i.inserting {
		return
	}
	if i.insertValues == nil {
		i.insertValues = make(map[int]string, len(vals))
	}
	for k, v := range vals {
		i.insertValues[k] = v
	}
}

// CancelInsert exits new-record mode.
func (i *Inspector) CancelInsert() {
	i.inserting = false
	i.insertValues = nil
	i.editing = false
}

// InsertValues returns pending insert field values keyed by column index.
func (i Inspector) InsertValues() map[int]string {
	if len(i.insertValues) == 0 {
		return nil
	}
	out := make(map[int]string, len(i.insertValues))
	for k, v := range i.insertValues {
		out[k] = v
	}
	return out
}

// Show opens the inspector (no-op if already visible). Resets edit/filter
// state the same way Toggle does when opening.
func (i *Inspector) Show() {
	if i.visible {
		return
	}
	i.visible = true
	i.editing = false
	i.cursorField = 0
	i.scrollRow = 0
	i.filtering = false
	i.filter = ""
	i.inserting = false
	i.insertValues = nil
	i.clearJSONTree()
}

// Hide forcibly closes the inspector.
func (i *Inspector) Hide() {
	i.visible = false
	i.editing = false
	i.cursorField = 0
	i.scrollRow = 0
	i.filtering = false
	i.filter = ""
	i.inserting = false
	i.insertValues = nil
	i.clearJSONTree()
}

// IsVisible returns whether the inspector panel is currently shown.
func (i Inspector) IsVisible() bool {
	return i.visible
}

// IsEditing returns whether a field is currently being edited.
func (i Inspector) IsEditing() bool {
	return i.editing
}

// SetSize sets the content dimensions of the inspector panel.
func (i *Inspector) SetSize(width, height int) {
	i.width = width
	i.height = height
	i.ensureFieldVisible()
}

// Reset clears the cursor and scroll state (e.g. when results change).
func (i *Inspector) Reset() {
	i.cursorField = 0
	i.scrollRow = 0
	i.editing = false
	i.filtering = false
	i.filter = ""
	i.inserting = false
	i.insertValues = nil
	i.clearJSONTree()
}

// IsFiltering returns whether the inspector filter input is active.
func (i Inspector) IsFiltering() bool {
	return i.filtering
}

// StartFilter enters inspector filter mode.
func (i *Inspector) StartFilter() {
	i.filtering = true
	i.filter = ""
	i.cursorField = 0
	i.scrollRow = 0
	i.clearJSONTree()
}

// CancelFilter exits filter mode and clears the query.
func (i *Inspector) CancelFilter() {
	i.filtering = false
	i.filter = ""
}

// CommitFilter exits filter mode, keeping the cursor on the selected field.
func (i *Inspector) CommitFilter(results ResultsTable) {
	col := i.selectedColumn(results)
	i.filtering = false
	i.filter = ""
	i.cursorField = col
	i.ensureFieldVisible(results)
}

// FilterAddChar appends a character to the filter.
func (i *Inspector) FilterAddChar(ch string) {
	i.filter += ch
	i.cursorField = 0
	i.scrollRow = 0
	i.clearJSONTree()
}

// FilterBackspace removes the last character from the filter.
func (i *Inspector) FilterBackspace() {
	if len(i.filter) > 0 {
		i.filter = i.filter[:len(i.filter)-1]
	}
	i.cursorField = 0
	i.scrollRow = 0
	i.clearJSONTree()
}

// fieldList returns visible field column indices, filtered and sorted when active.
func (i Inspector) fieldList(results ResultsTable) []int {
	n := results.NumCols()
	if n == 0 {
		return nil
	}
	if !i.filtering {
		indices := make([]int, n)
		for j := range indices {
			indices[j] = j
		}
		return indices
	}

	type scored struct {
		col   int
		score int
	}
	var matches []scored
	for c := 0; c < n; c++ {
		name := results.ColumnName(c)
		idx, score := fuzzyMatch(i.filter, name)
		if idx != nil || i.filter == "" {
			matches = append(matches, scored{col: c, score: score})
		}
	}
	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].score != matches[b].score {
			return matches[a].score < matches[b].score
		}
		return results.ColumnName(matches[a].col) < results.ColumnName(matches[b].col)
	})
	indices := make([]int, len(matches))
	for j, m := range matches {
		indices[j] = m.col
	}
	return indices
}

// selectedColumn returns the result column index for the current field cursor.
func (i Inspector) selectedColumn(results ResultsTable) int {
	fields := i.fieldList(results)
	if len(fields) == 0 {
		return 0
	}
	cf := i.cursorField
	if cf >= len(fields) {
		cf = len(fields) - 1
	}
	if cf < 0 {
		cf = 0
	}
	return fields[cf]
}

// SyncToColumn moves the field cursor to the given result column index, if
// that column is in the current field list (respects inspector "/"). No-op
// when the column is filtered out.
func (i *Inspector) SyncToColumn(col int, results ResultsTable) {
	fields := i.fieldList(results)
	for fi, c := range fields {
		if c == col {
			if i.cursorField != fi {
				i.clearJSONTree()
			}
			i.cursorField = fi
			i.ensureFieldVisible(results)
			return
		}
	}
}

// IsFieldTruncated reports whether the value of the currently selected field
// is wider than the inspector's value box (shown with an ellipsis). This is
// the condition under which the expanded cell popup replaces the inline editor.
func (i Inspector) IsFieldTruncated(results ResultsTable) bool {
	row := results.CursorRow()
	col := i.selectedColumn(results)
	if row < 0 || row >= results.NumRows() || col < 0 || col >= results.NumCols() {
		return false
	}
	val := results.RowValue(row, col)
	valueWidth := i.width - 4
	if valueWidth < 5 {
		valueWidth = 5
	}
	return runeLen(val) > valueWidth
}

// FocusedFieldIsJSON reports whether the focused field holds a JSON object
// or array (foldable in the inspector; edited via the E popup).
func (i Inspector) FocusedFieldIsJSON(results ResultsTable) bool {
	val, ok := i.focusedFieldRaw(results)
	return ok && isJSONValue(val)
}

// JSONTreeActive reports whether the focused JSON field fold is open (tree
// navigation captures j/k / o / enter / esc).
func (i Inspector) JSONTreeActive() bool {
	return i.jsonExpanded
}

func (i Inspector) focusedFieldRaw(results ResultsTable) (string, bool) {
	col := i.selectedColumn(results)
	if col < 0 || col >= results.NumCols() {
		return "", false
	}
	if i.inserting {
		return i.insertValues[col], true
	}
	row := results.CursorRow()
	if row < 0 || row >= results.NumRows() {
		return "", false
	}
	return results.RowValue(row, col), true
}

func (i *Inspector) clearJSONTree() {
	i.jsonExpanded = false
	i.jsonTreeOpen = nil
	i.jsonTreeCursor = 0
	i.jsonTreeScroll = 0
}

// CollapseJSONTree closes the field-level JSON fold (esc while browsing).
func (i *Inspector) CollapseJSONTree() {
	i.clearJSONTree()
}

func (i Inspector) jsonTreeRows(results ResultsTable) []jsonTreeRow {
	val, ok := i.focusedFieldRaw(results)
	if !ok {
		return nil
	}
	v, ok := parseJSONContainer(val)
	if !ok {
		return nil
	}
	return buildJSONTreeRows(v, i.jsonTreeOpen)
}

func (i *Inspector) clampJSONTree(results ResultsTable) {
	rows := i.jsonTreeRows(results)
	if len(rows) == 0 {
		i.jsonTreeCursor = 0
		i.jsonTreeScroll = 0
		return
	}
	if i.jsonTreeCursor >= len(rows) {
		i.jsonTreeCursor = len(rows) - 1
	}
	if i.jsonTreeCursor < 0 {
		i.jsonTreeCursor = 0
	}
	if i.jsonTreeCursor < i.jsonTreeScroll {
		i.jsonTreeScroll = i.jsonTreeCursor
	}
	if i.jsonTreeCursor >= i.jsonTreeScroll+jsonTreeMaxLines {
		i.jsonTreeScroll = i.jsonTreeCursor - jsonTreeMaxLines + 1
	}
	if i.jsonTreeScroll < 0 {
		i.jsonTreeScroll = 0
	}
}

// ToggleJSONFold opens the field fold, toggles the tree node under the cursor,
// or closes the fold when collapsing the root. Returns false when the field
// is not a JSON object/array.
func (i *Inspector) ToggleJSONFold(results ResultsTable) bool {
	if !i.FocusedFieldIsJSON(results) {
		return false
	}
	if !i.jsonExpanded {
		i.jsonExpanded = true
		i.jsonTreeOpen = map[string]bool{jsonTreeRootPath: true}
		i.jsonTreeCursor = 0
		i.jsonTreeScroll = 0
		i.ensureFieldVisible(results)
		return true
	}
	rows := i.jsonTreeRows(results)
	if len(rows) == 0 {
		return true
	}
	if i.jsonTreeCursor < 0 || i.jsonTreeCursor >= len(rows) {
		i.clampJSONTree(results)
	}
	row := rows[i.jsonTreeCursor]
	if !row.foldable {
		return true
	}
	if row.path == jsonTreeRootPath && row.open {
		i.clearJSONTree()
		i.ensureFieldVisible(results)
		return true
	}
	if i.jsonTreeOpen == nil {
		i.jsonTreeOpen = map[string]bool{jsonTreeRootPath: true}
	}
	i.jsonTreeOpen[row.path] = !i.jsonTreeOpen[row.path]
	i.clampJSONTree(results)
	i.ensureFieldVisible(results)
	return true
}

func jsonTreeParentPath(path string) string {
	if path == jsonTreeRootPath || path == "" {
		return ""
	}
	i := strings.LastIndex(path, "/")
	if i <= 0 {
		return jsonTreeRootPath
	}
	return path[:i]
}

// JSONTreeExpand (l / →) opens the node under the cursor. If it is already
// open, moves to its first child. When the field fold is closed, opens it.
func (i *Inspector) JSONTreeExpand(results ResultsTable) bool {
	if !i.FocusedFieldIsJSON(results) {
		return false
	}
	if !i.jsonExpanded {
		return i.ToggleJSONFold(results)
	}
	rows := i.jsonTreeRows(results)
	if len(rows) == 0 {
		return true
	}
	i.clampJSONTree(results)
	row := rows[i.jsonTreeCursor]
	if row.foldable && !row.open {
		if i.jsonTreeOpen == nil {
			i.jsonTreeOpen = map[string]bool{jsonTreeRootPath: true}
		}
		i.jsonTreeOpen[row.path] = true
		i.clampJSONTree(results)
		i.ensureFieldVisible(results)
		return true
	}
	if row.foldable && row.open && i.jsonTreeCursor+1 < len(rows) &&
		rows[i.jsonTreeCursor+1].depth == row.depth+1 {
		i.jsonTreeCursor++
		i.clampJSONTree(results)
	}
	return true
}

// JSONTreeCollapse (h / ←) folds the node under the cursor. If it is already
// collapsed (or a leaf), moves to its parent — collapsing the root closes the
// field fold.
func (i *Inspector) JSONTreeCollapse(results ResultsTable) bool {
	if !i.jsonExpanded {
		return false
	}
	rows := i.jsonTreeRows(results)
	if len(rows) == 0 {
		return true
	}
	i.clampJSONTree(results)
	row := rows[i.jsonTreeCursor]
	if row.foldable && row.open {
		if row.path == jsonTreeRootPath {
			i.clearJSONTree()
			i.ensureFieldVisible(results)
			return true
		}
		i.jsonTreeOpen[row.path] = false
		i.clampJSONTree(results)
		i.ensureFieldVisible(results)
		return true
	}
	parent := jsonTreeParentPath(row.path)
	if parent == "" {
		i.clearJSONTree()
		i.ensureFieldVisible(results)
		return true
	}
	for idx, r := range rows {
		if r.path == parent {
			i.jsonTreeCursor = idx
			i.clampJSONTree(results)
			return true
		}
	}
	return true
}

// JSONTreeUp moves the tree cursor up. Returns false when already at the top
// (caller should collapse and move to the previous field).
func (i *Inspector) JSONTreeUp(results ResultsTable) bool {
	if !i.jsonExpanded {
		return false
	}
	if i.jsonTreeCursor > 0 {
		i.jsonTreeCursor--
		i.clampJSONTree(results)
		return true
	}
	i.clearJSONTree()
	return false
}

// JSONTreeDown moves the tree cursor down. Returns false when already at the
// bottom (caller should collapse and move to the next field).
func (i *Inspector) JSONTreeDown(results ResultsTable) bool {
	if !i.jsonExpanded {
		return false
	}
	rows := i.jsonTreeRows(results)
	if i.jsonTreeCursor < len(rows)-1 {
		i.jsonTreeCursor++
		i.clampJSONTree(results)
		return true
	}
	i.clearJSONTree()
	return false
}

// JSONTreeTop moves the tree cursor to the root row.
func (i *Inspector) JSONTreeTop(results ResultsTable) {
	if !i.jsonExpanded {
		return
	}
	i.jsonTreeCursor = 0
	i.clampJSONTree(results)
}

// JSONTreeBottom moves the tree cursor to the last visible row.
func (i *Inspector) JSONTreeBottom(results ResultsTable) {
	if !i.jsonExpanded {
		return
	}
	rows := i.jsonTreeRows(results)
	if len(rows) == 0 {
		return
	}
	i.jsonTreeCursor = len(rows) - 1
	i.clampJSONTree(results)
}

// CursorTop moves the field cursor to the first field.
func (i *Inspector) CursorTop(results ...ResultsTable) {
	if i.cursorField != 0 {
		i.clearJSONTree()
	}
	i.cursorField = 0
	i.ensureFieldVisible(results...)
}

// CursorBottom moves the field cursor to the last field.
func (i *Inspector) CursorBottom(results ResultsTable) {
	n := len(i.fieldList(results))
	next := 0
	if n > 0 {
		next = n - 1
	}
	if i.cursorField != next {
		i.clearJSONTree()
	}
	i.cursorField = next
	i.ensureFieldVisible(results)
}

// CursorUp moves the field cursor up by one.
func (i *Inspector) CursorUp(results ...ResultsTable) {
	if i.cursorField > 0 {
		i.cursorField--
		i.clearJSONTree()
	}
	i.ensureFieldVisible(results...)
}

// CursorDown moves the field cursor down by one.
func (i *Inspector) CursorDown(results ResultsTable) {
	n := len(i.fieldList(results))
	if n > 0 && i.cursorField < n-1 {
		i.cursorField++
		i.clearJSONTree()
	}
	i.ensureFieldVisible(results)
}

// ClickField moves the field cursor to the field at the given content-relative
// Y coordinate (0 = first line below the inspector's top border) and returns
// the result column index of that field. Returns -1 if the Y does not land on
// a field (e.g. on the "[new record]" header or empty padding). It accounts
// for insert-mode header and the current scroll offset.
func (i *Inspector) ClickField(contentY int, results ResultsTable) int {
	fieldIndices := i.fieldList(results)
	numFields := len(fieldIndices)
	if numFields == 0 {
		return -1
	}

	y := contentY
	if i.inserting {
		y--
	}
	if y < 0 {
		return -1
	}

	start := i.scrollStart(results)
	acc := 0
	for fi := start; fi < numFields; fi++ {
		h := i.fieldLineCount(results, fi)
		if y >= acc && y < acc+h {
			if i.cursorField != fi {
				i.clearJSONTree()
			}
			i.cursorField = fi
			i.ensureFieldVisible(results)
			return fieldIndices[fi]
		}
		acc += h
	}
	return -1
}

// fieldsAvailHeight is the vertical budget for field boxes (and the insert
// header when present).
func (i Inspector) fieldsAvailHeight() int {
	avail := i.height
	if i.filtering {
		avail--
	}
	if avail < 1 {
		return 1
	}
	return avail
}

// visibleFieldCount returns how many complete fields fit when every field is
// the default single-value height. Used as a fallback when results are not
// available (e.g. SetSize before the first View).
func (i Inspector) visibleFieldCount() int {
	avail := i.fieldsAvailHeight()
	if i.inserting {
		avail--
	}
	if avail < linesPerField {
		return 1
	}
	return avail / linesPerField
}

func (i Inspector) valueWidth() int {
	w := i.width - 4
	if w < 5 {
		return 5
	}
	return w
}

// fieldLineCount is the rendered height of field index fi (label + borders +
// value lines), including the JSON fold when that field is focused.
func (i Inspector) fieldLineCount(results ResultsTable, fi int) int {
	fieldIndices := i.fieldList(results)
	if fi < 0 || fi >= len(fieldIndices) {
		return linesPerField
	}
	col := fieldIndices[fi]
	row := results.CursorRow()
	if i.inserting {
		row = 0
	}
	val := ""
	if i.inserting {
		val = i.insertValues[col]
	} else if row >= 0 && row < results.NumRows() {
		val = results.RowValue(row, col)
	}
	focused := fi == i.cursorField
	editing := i.editing && focused
	dirty := false
	if i.inserting {
		dirty = i.insertValues[col] != ""
	} else if row >= 0 && row < results.NumRows() {
		dirty = results.IsDirty(row, col)
	}
	content := i.valueContentFor(val, focused, editing, dirty)
	return 3 + strings.Count(content, "\n") + 1
}

func (i Inspector) valueContentFor(val string, focused, editing, dirty bool) string {
	vw := i.valueWidth()
	switch {
	case editing:
		return renderEditInput(i.editInput, vw, colorEdit)
	case focused:
		if content, ok := jsonTreeValueContent(val, vw, i.jsonExpanded, i.jsonTreeOpen, i.jsonTreeCursor, i.jsonTreeScroll); ok {
			return content
		}
	}
	displayVal := truncateCell(val, vw)
	valStyle := lipgloss.NewStyle().Foreground(colorFg)
	if !i.inserting && (val == "NULL" || db.IsBlobPlaceholder(val)) {
		valStyle = lipgloss.NewStyle().Foreground(colorMuted)
	}
	if dirty {
		valStyle = lipgloss.NewStyle().Foreground(colorPrimary)
	}
	if i.inserting && val == "" {
		valStyle = lipgloss.NewStyle().Foreground(colorMuted)
		displayVal = truncateCell("(empty)", vw)
	}
	return valStyle.Render(displayVal)
}

// scrollStart returns the first visible field index, clamped so the viewport
// still fills when near the bottom.
func (i Inspector) scrollStart(results ResultsTable) int {
	fieldIndices := i.fieldList(results)
	numFields := len(fieldIndices)
	if numFields == 0 {
		return 0
	}
	start := i.scrollRow
	if start < 0 {
		start = 0
	}
	if start >= numFields {
		start = numFields - 1
	}
	return start
}

// ensureFieldVisible adjusts scrollRow so the cursor field stays in view.
func (i *Inspector) ensureFieldVisible(results ...ResultsTable) {
	if len(results) == 0 {
		max := i.visibleFieldCount()
		if i.cursorField < i.scrollRow {
			i.scrollRow = i.cursorField
		}
		if i.cursorField >= i.scrollRow+max {
			i.scrollRow = i.cursorField - max + 1
		}
		if i.scrollRow < 0 {
			i.scrollRow = 0
		}
		return
	}
	r := results[0]
	fieldIndices := i.fieldList(r)
	numFields := len(fieldIndices)
	if numFields == 0 {
		i.scrollRow = 0
		return
	}
	if i.cursorField >= numFields {
		i.cursorField = numFields - 1
	}
	if i.cursorField < 0 {
		i.cursorField = 0
	}
	if i.cursorField < i.scrollRow {
		i.scrollRow = i.cursorField
	}

	avail := i.fieldsAvailHeight()
	if i.inserting {
		avail--
	}
	if avail < 1 {
		avail = 1
	}

	// Grow scrollRow until the cursor field fits in the remaining budget.
	for i.scrollRow < i.cursorField {
		used := 0
		for fi := i.scrollRow; fi <= i.cursorField; fi++ {
			used += i.fieldLineCount(r, fi)
		}
		if used <= avail {
			break
		}
		i.scrollRow++
	}
	if i.scrollRow < 0 {
		i.scrollRow = 0
	}
	if i.scrollRow > numFields-1 {
		i.scrollRow = numFields - 1
	}
}

// StartFieldEdit begins editing the currently focused field.
func (i *Inspector) StartFieldEdit(results ResultsTable) {
	if results.NumCols() == 0 {
		return
	}
	col := i.selectedColumn(results)

	if i.inserting {
		if results.IsAutoIncrementCol(col) {
			return
		}
		val := i.insertValues[col]
		i.beginFieldEdit(val)
		i.editingCol = col
		return
	}

	if !results.IsEditable() || !results.HasPrimaryKey() || results.NumRows() == 0 {
		return
	}
	colName := results.ColumnName(col)
	if results.isPKColumn(colName) {
		return
	}

	row := results.CursorRow()
	if results.IsBlobCell(row, col) {
		return
	}
	val := results.RowValue(row, col)
	if val == "NULL" {
		val = ""
	}
	i.beginFieldEdit(val)
	i.editingCol = col
}

func (i *Inspector) beginFieldEdit(val string) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.SetValue(val)

	valueWidth := i.width - 4
	if valueWidth < 10 {
		valueWidth = 10
	}
	// textinput.View() renders Width+1 chars (cursor takes a column), so
	// subtract 1 to keep the value line within the bordered field box.
	ti.Width = valueWidth - 1

	ti.TextStyle = lipgloss.NewStyle().Foreground(colorFg)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colorFg).Background(colorBg)

	ti.Focus()
	i.editInput = ti
	i.editOriginal = val
	i.editing = true
}

// CommitFieldEdit finalizes the edit and returns the column index and new value.
func (i *Inspector) CommitFieldEdit() (col int, val string, ok bool) {
	if !i.editing {
		return 0, "", false
	}
	col = i.editingCol
	val = i.editInput.Value()
	i.editing = false
	if i.inserting {
		i.insertValues[col] = val
	}
	return col, val, true
}

// FieldEditChanged reports whether the in-flight edit buffer differs from the
// value that StartFieldEdit loaded (so no-op Enter / click-away can skip dirty).
func (i Inspector) FieldEditChanged() bool {
	if !i.editing {
		return false
	}
	return i.editInput.Value() != i.editOriginal
}

// CancelEdit discards the current field edit.
func (i *Inspector) CancelEdit() {
	i.editing = false
}

// Update handles messages for the inspector (textinput routing when editing).
func (i Inspector) Update(msg tea.Msg) (Inspector, tea.Cmd) {
	if i.editing {
		var cmd tea.Cmd
		i.editInput, cmd = i.editInput.Update(msg)
		return i, cmd
	}
	return i, nil
}

// View renders the inspector content (without outer border or title) as a
// vertical form. Each field is: column-name label (with type right-aligned),
// then a bordered value box. The focused field's box border uses the primary
// color; all others use the table grid color. Row info is pinned to the bottom.
func (i Inspector) View(results ResultsTable) string {
	fieldIndices := i.fieldList(results)
	numFields := len(fieldIndices)
	filterBar := ""
	if i.filtering {
		filterBar = renderPalettePrompt(i.filter, true)
	}

	if numFields == 0 || (!i.inserting && results.NumRows() == 0) {
		fieldsHeight := i.fieldsAvailHeight()
		var body strings.Builder
		if i.filtering && numFields == 0 && results.NumCols() > 0 {
			body.WriteString(mutedStyle.Render(" (no matches)"))
			body.WriteString("\n")
		}
		empty := lipgloss.NewStyle().Height(fieldsHeight).Render(body.String())
		if filterBar != "" {
			return empty + "\n" + filterBar
		}
		return empty
	}

	// Always track the results cell cursor. Using ScrollRow for non-editable
	// grids was wrong: scroll is the first visible row, so a mid-viewport
	// cursor showed the wrong record.
	row := results.CursorRow()
	if i.inserting {
		row = 0
	}
	if row >= results.NumRows() {
		row = results.NumRows() - 1
	}
	if row < 0 {
		row = 0
	}

	cursorClamp := i.cursorField
	if cursorClamp >= numFields {
		cursorClamp = numFields - 1
	}
	if cursorClamp < 0 {
		cursorClamp = 0
	}

	start := i.scrollStart(results)
	avail := i.fieldsAvailHeight()
	used := 0
	if i.inserting {
		used = 1
	}

	labelStyle := lipgloss.NewStyle().Foreground(colorLabel)
	pkLabelStyle := lipgloss.NewStyle().Foreground(colorLabel).Bold(true)
	typeStyle := lipgloss.NewStyle().Foreground(colorMuted)

	var rendered strings.Builder
	if i.inserting {
		title := " [new record]"
		if t := results.SourceTable(); t != "" {
			title += " " + t
		}
		rendered.WriteString(successStyle.Render(title))
		rendered.WriteString("\n")
	}

	for fi := start; fi < numFields; fi++ {
		c := fieldIndices[fi]
		colName := results.ColumnName(c)
		isPK := results.isPKColumn(colName)
		isDirty := results.IsDirty(row, c)
		if i.inserting {
			isDirty = i.insertValues[c] != ""
		}
		isFocused := fi == cursorClamp
		val := results.RowValue(row, c)
		if i.inserting {
			val = i.insertValues[c]
		}

		labelRaw := colName
		if isPK {
			labelRaw = "* " + labelRaw
		}
		if isDirty {
			labelRaw += " ●"
		}
		if i.inserting && results.IsAutoIncrementCol(c) {
			labelRaw += " (auto)"
		}
		ls := labelStyle
		if isPK {
			ls = pkLabelStyle
		}
		labelStr := ls.Render(labelRaw)
		markerStr := typeStyle.Render(strings.ToLower(results.ColumnType(c)))
		if fk, ok := results.ForeignKeyAt(c); ok {
			target := "→ " + fk.RefTable + "." + fk.RefColumn
			fkStyle := lipgloss.NewStyle().Foreground(colorFK)
			if !isFocused {
				fkStyle = typeStyle
			}
			markerStr = fkStyle.Render(target)
		}

		editing := i.editing && isFocused
		valueContent := i.valueContentFor(val, isFocused, editing, isDirty)
		box := renderFieldBox(labelStr, markerStr, valueContent, i.width, fieldBoxBorder(isFocused))
		boxH := strings.Count(box, "\n") + 1
		if used > 0 && used+boxH > avail {
			break
		}
		rendered.WriteString(box)
		rendered.WriteString("\n")
		used += boxH
	}

	fieldsHeight := i.fieldsAvailHeight()
	if fieldsHeight < linesPerField {
		fieldsHeight = linesPerField
	}
	fieldsBlock := lipgloss.NewStyle().
		Height(fieldsHeight).
		Render(strings.TrimRight(rendered.String(), "\n"))

	if filterBar != "" {
		return fieldsBlock + "\n" + filterBar
	}
	return fieldsBlock
}
