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
	width        int
	height       int
	visible      bool
	cursorField  int
	editing      bool
	editInput    textinput.Model
	scrollRow    int
	pendingG     bool
	filtering    bool
	filter       string
	inserting    bool
	insertValues map[int]string
	editingCol   int
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
}

// FilterBackspace removes the last character from the filter.
func (i *Inspector) FilterBackspace() {
	if len(i.filter) > 0 {
		i.filter = i.filter[:len(i.filter)-1]
	}
	i.cursorField = 0
	i.scrollRow = 0
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

// CursorTop moves the field cursor to the first field.
func (i *Inspector) CursorTop() {
	i.cursorField = 0
	i.ensureFieldVisible()
}

// CursorBottom moves the field cursor to the last field.
func (i *Inspector) CursorBottom(results ResultsTable) {
	n := len(i.fieldList(results))
	if n > 0 {
		i.cursorField = n - 1
	}
	i.ensureFieldVisible(results)
}

// CursorUp moves the field cursor up by one.
func (i *Inspector) CursorUp() {
	if i.cursorField > 0 {
		i.cursorField--
	}
	i.ensureFieldVisible()
}

// CursorDown moves the field cursor down by one.
func (i *Inspector) CursorDown(results ResultsTable) {
	n := len(i.fieldList(results))
	if n > 0 && i.cursorField < n-1 {
		i.cursorField++
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

	// In insert mode the "[new record]" header occupies the first content line.
	y := contentY
	if i.inserting {
		y--
	}
	if y < 0 {
		return -1
	}

	maxFields := i.visibleFieldCount()
	start := i.scrollRow
	if start > numFields-maxFields && numFields > maxFields {
		start = numFields - maxFields
	}
	if start < 0 {
		start = 0
	}

	relField := y / linesPerField
	fieldIdx := start + relField
	if fieldIdx < 0 || fieldIdx >= numFields {
		return -1
	}
	i.cursorField = fieldIdx
	i.ensureFieldVisible(results)
	return fieldIndices[fieldIdx]
}

// visibleFieldCount returns how many complete fields fit in the available height.
func (i Inspector) visibleFieldCount() int {
	avail := i.height
	if i.filtering {
		avail--
	}
	if avail < linesPerField {
		return 1
	}
	return avail / linesPerField
}

// ensureFieldVisible adjusts scrollRow so the cursor field stays in view.
func (i *Inspector) ensureFieldVisible(results ...ResultsTable) {
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
	if len(results) > 0 {
		fieldIndices := i.fieldList(results[0])
		if len(fieldIndices) > 0 && i.scrollRow > len(fieldIndices)-1 {
			i.scrollRow = len(fieldIndices) - 1
		}
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
		fieldsHeight := i.height
		if i.filtering {
			fieldsHeight--
		}
		if fieldsHeight < 1 {
			fieldsHeight = 1
		}
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

	valueWidth := i.width - 4
	if valueWidth < 5 {
		valueWidth = 5
	}

	maxFields := i.visibleFieldCount()

	cursorClamp := i.cursorField
	if cursorClamp >= numFields {
		cursorClamp = numFields - 1
	}
	if cursorClamp < 0 {
		cursorClamp = 0
	}

	start := i.scrollRow
	if start > numFields-maxFields && numFields > maxFields {
		start = numFields - maxFields
	}
	if start < 0 {
		start = 0
	}
	end := start + maxFields
	if end > numFields {
		end = numFields
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

	for fi := start; fi < end; fi++ {
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

		// Label (left) and column type (right, marker slot).
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

		// Value line(s) filling the box interior.
		var valueContent string
		switch {
		case i.editing && isFocused:
			valueContent = renderEditInput(i.editInput, valueWidth, colorEdit)
		case !i.inserting && isFocused:
			if pretty, isJSON := formatJSON(val); isJSON {
				// Multi-line highlighted JSON for the focused field.
				jsonLines := strings.Split(pretty, "\n")
				const maxJSONLines = 6
				if len(jsonLines) > maxJSONLines {
					jsonLines = jsonLines[:maxJSONLines]
				}
				hl := make([]string, len(jsonLines))
				for k, jl := range jsonLines {
					hl[k] = highlightJSON(truncateCell(jl, valueWidth))
				}
				valueContent = strings.Join(hl, "\n")
			}
		}
		if valueContent == "" {
			displayVal := truncateCell(val, valueWidth)
			valStyle := lipgloss.NewStyle().Foreground(colorFg)
			if !i.inserting && (val == "NULL" || db.IsBlobPlaceholder(val)) {
				valStyle = lipgloss.NewStyle().Foreground(colorMuted)
			}
			if isDirty {
				valStyle = lipgloss.NewStyle().Foreground(colorPrimary)
			}
			if i.inserting && val == "" {
				valStyle = lipgloss.NewStyle().Foreground(colorMuted)
				displayVal = truncateCell("(empty)", valueWidth)
			}
			valueContent = valStyle.Render(displayVal)
		}

		rendered.WriteString(renderFieldBox(labelStr, markerStr, valueContent, i.width, fieldBoxBorder(isFocused)))
		rendered.WriteString("\n")
	}

	// Height-constrained fields block fills the panel.
	fieldsHeight := i.height
	if i.filtering {
		fieldsHeight--
	}
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
