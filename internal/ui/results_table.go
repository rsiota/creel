package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

const maxCellWidth = 40

const (
	copyFlashInterval    = 150 // milliseconds between flash toggles
	copyFlashTickCount   = 6   // three on/off cycles
	copyMessageDuration  = 2   // seconds to show "copied to clipboard"
)

// cellRef identifies a single cell by row and column index.
type cellRef struct {
	row int
	col int
}

// ResultsTable renders query results as a scrollable table with
// per-cell truncation and horizontal scrolling. When editable, it
// supports a cell cursor and inline editing.
type ResultsTable struct {
	columns   []string
	rows      [][]string
	scrollRow int
	scrollCol int
	colWidths []int
	width     int
	height    int
	message   string
	hasResult bool

	// Inline editing
	editable   bool              // can these results be edited?
	sourceTable string            // table backing these results
	pkColumns  []string          // primary key column names
	cursorRow  int               // highlighted cell cursor row
	cursorCol  int               // highlighted cell cursor col
	editing    bool              // inline edit mode active
	editInput  textinput.Model   // input buffer for the cell being edited
	dirtyCells map[cellRef]string // pending unsaved edits (new values)
	saved      bool              // all dirty cells were saved (show confirmation)
	saveError  string            // last save error
	copied     bool              // show clipboard copy confirmation
	copyFlash       cellRef
	copyFlashActive bool
	copyFlashOn     bool
	copyFlashTicks  int
	resultTable     string
	foreignKeys     map[string]db.ForeignKey // keyed by lowercase column name
	columnTypes map[string]string // column name -> database type (for inspector)
	tableColumns []db.TableColumnInfo
}

// NewResultsTable creates a new results table component.
func NewResultsTable() ResultsTable {
	return ResultsTable{}
}

// SetEditable configures the table for inline editing.
func (r *ResultsTable) SetEditable(table string, pkCols []string) {
	r.editable = true
	r.sourceTable = table
	r.pkColumns = pkCols
	r.cursorRow = 0
	r.cursorCol = 0
	r.dirtyCells = make(map[cellRef]string)
	r.editing = false
	r.saveError = ""
}

// SetTableColumns stores schema metadata for inserts.
func (r *ResultsTable) SetTableColumns(cols []db.TableColumnInfo) {
	r.tableColumns = cols
}

// TableColumns returns schema metadata for the editable table.
func (r ResultsTable) TableColumns() []db.TableColumnInfo {
	return r.tableColumns
}

// IsAutoIncrementCol reports whether a column is an auto-increment primary key.
func (r ResultsTable) IsAutoIncrementCol(col int) bool {
	info, ok := r.columnInfo(col)
	return ok && info.AutoIncrement
}

func (r ResultsTable) columnInfo(col int) (db.TableColumnInfo, bool) {
	name := r.ColumnName(col)
	for _, c := range r.tableColumns {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return db.TableColumnInfo{}, false
}

// ClearForeignKeys removes foreign-key navigation metadata.
func (r *ResultsTable) ClearForeignKeys() {
	r.resultTable = ""
	r.foreignKeys = nil
}

// SetForeignKeys records outbound foreign keys for the current result table.
func (r *ResultsTable) SetForeignKeys(table string, fks []db.ForeignKey) {
	r.resultTable = table
	r.foreignKeys = make(map[string]db.ForeignKey)
	colSet := make(map[string]bool, len(r.columns))
	for _, c := range r.columns {
		colSet[strings.ToLower(c)] = true
	}
	for _, fk := range fks {
		key := strings.ToLower(fk.Column)
		if colSet[key] {
			r.foreignKeys[key] = fk
		}
	}
}

// HasForeignKeys returns whether the current results expose FK navigation.
func (r ResultsTable) HasForeignKeys() bool {
	return len(r.foreignKeys) > 0
}

// ForeignKeyAt returns the foreign key for a column index, if any.
func (r ResultsTable) ForeignKeyAt(col int) (db.ForeignKey, bool) {
	name := strings.ToLower(r.ColumnName(col))
	fk, ok := r.foreignKeys[name]
	return fk, ok
}

// ForeignKeyAtCursor returns the foreign key for the cell under the cursor.
func (r ResultsTable) ForeignKeyAtCursor() (db.ForeignKey, bool) {
	return r.ForeignKeyAt(r.cursorCol)
}

// IsNavigableForeignKey reports whether a cell can be followed via gd.
func (r ResultsTable) IsNavigableForeignKey(row, col int) bool {
	if _, ok := r.ForeignKeyAt(col); !ok {
		return false
	}
	val := r.RowValue(row, col)
	return val != "" && val != "NULL"
}

// ResultTable returns the source table for the current results, if known.
func (r ResultsTable) ResultTable() string {
	return r.resultTable
}

// CursorCol returns the current cursor column index.
func (r ResultsTable) CursorCol() int {
	return r.cursorCol
}

// SetCursor moves the cell cursor and keeps it visible.
func (r *ResultsTable) SetCursor(row, col int) {
	r.cursorRow = row
	r.cursorCol = col
	r.clampCursor()
	r.ensureCursorVisible()
}

// ClearEditable disables inline editing.
func (r *ResultsTable) ClearEditable() {
	r.editable = false
	r.sourceTable = ""
	r.pkColumns = nil
	r.dirtyCells = nil
	r.editing = false
	r.tableColumns = nil
}

// IsEditable returns whether the current results support inline editing.
func (r ResultsTable) IsEditable() bool {
	return r.editable
}

// IsEditing returns whether a cell is currently being edited inline.
func (r ResultsTable) IsEditing() bool {
	return r.editing
}

// HasDirtyCells returns whether there are unsaved edits.
func (r ResultsTable) HasDirtyCells() bool {
	return len(r.dirtyCells) > 0
}

// DirtyCells returns all pending cell edits as a slice of (row, col, value).
func (r ResultsTable) DirtyCells() []CellEdit {
	var edits []CellEdit
	for ref, val := range r.dirtyCells {
		edits = append(edits, CellEdit{
			Row:     ref.row,
			Col:     ref.col,
			NewValue: val,
		})
	}
	return edits
}

// CellEdit represents a single pending cell modification.
type CellEdit struct {
	Row      int
	Col      int
	NewValue string
}

// ColumnName returns the column name for a given column index.
func (r ResultsTable) ColumnName(col int) string {
	if col < 0 || col >= len(r.columns) {
		return ""
	}
	return r.columns[col]
}

// RowValue returns the current value at (row, col), accounting for dirty edits.
func (r ResultsTable) RowValue(row, col int) string {
	ref := cellRef{row: row, col: col}
	if val, ok := r.dirtyCells[ref]; ok {
		return val
	}
	if row < 0 || row >= len(r.rows) {
		return ""
	}
	if col < 0 || col >= len(r.rows[row]) {
		return ""
	}
	return r.rows[row][col]
}

// SourceTable returns the table name backing editable results.
func (r ResultsTable) SourceTable() string {
	return r.sourceTable
}

// PKColumns returns the primary key column names.
func (r ResultsTable) PKColumns() []string {
	return r.pkColumns
}

// ConfirmSaved clears dirty cells and shows a confirmation message.
func (r *ResultsTable) ConfirmSaved() {
	r.dirtyCells = make(map[cellRef]string)
	r.saved = true
	r.saveError = ""
	r.copied = false
}

// StartCopyFeedback marks the current cell as copied and begins a flash animation.
func (r *ResultsTable) StartCopyFeedback() {
	r.copied = true
	r.copyFlash = cellRef{row: r.cursorRow, col: r.cursorCol}
	r.copyFlashActive = true
	r.copyFlashOn = true
	r.copyFlashTicks = copyFlashTickCount
}

// AdvanceCopyFlash toggles the copy flash state. It returns whether more ticks remain.
func (r *ResultsTable) AdvanceCopyFlash() bool {
	if !r.copyFlashActive || r.copyFlashTicks <= 0 {
		return false
	}
	r.copyFlashTicks--
	r.copyFlashOn = !r.copyFlashOn
	if r.copyFlashTicks <= 0 {
		r.copyFlashActive = false
		return false
	}
	return true
}

// ClearCopiedMessage hides the clipboard confirmation in the status line.
func (r *ResultsTable) ClearCopiedMessage() {
	r.copied = false
}

func (r ResultsTable) isCopyFlashCell(row, col int) bool {
	return r.copyFlashActive && r.copyFlash.row == row && r.copyFlash.col == col
}

// DiscardEdits clears all pending cell edits.
func (r *ResultsTable) DiscardEdits() {
	r.dirtyCells = make(map[cellRef]string)
	r.saved = false
	r.saveError = ""
}

// SetSaveError records a save error message.
func (r *ResultsTable) SetSaveError(msg string) {
	r.saveError = msg
	r.saved = false
}

// SetResult populates the table with query results.
func (r *ResultsTable) SetResult(cols []string, rows [][]string, message string) {
	r.columns = cols
	r.rows = rows
	r.message = message
	r.hasResult = true
	r.scrollRow = 0
	r.scrollCol = 0
	r.colWidths = nil
	r.cursorRow = 0
	r.cursorCol = 0
	r.dirtyCells = make(map[cellRef]string)
	r.editing = false
	r.saved = false
	r.saveError = ""
	r.copied = false
	r.copyFlashActive = false
	r.ClearForeignKeys()
	r.computeColWidths()
}

// SetError displays an error message in the results area.
func (r *ResultsTable) SetError(err string) {
	r.columns = nil
	r.rows = nil
	r.message = err
	r.hasResult = true
}

// Clear resets the table to empty.
func (r *ResultsTable) Clear() {
	r.columns = nil
	r.rows = nil
	r.message = ""
	r.hasResult = false
	r.scrollRow = 0
	r.scrollCol = 0
	r.colWidths = nil
	r.cursorRow = 0
	r.cursorCol = 0
	r.dirtyCells = make(map[cellRef]string)
	r.editing = false
	r.copied = false
	r.copyFlashActive = false
	r.ClearForeignKeys()
}

// Message returns the current status message.
func (r ResultsTable) Message() string {
	return r.message
}

// HasResult returns whether the table has data to display.
func (r ResultsTable) HasResult() bool {
	return r.hasResult
}

// CursorRow returns the current cursor row index (editable mode).
func (r ResultsTable) CursorRow() int {
	return r.cursorRow
}

// ScrollRow returns the current vertical scroll position.
func (r ResultsTable) ScrollRow() int {
	return r.scrollRow
}

// NumRows returns the number of data rows.
func (r ResultsTable) NumRows() int {
	return len(r.rows)
}

// NumCols returns the number of columns.
func (r ResultsTable) NumCols() int {
	return len(r.columns)
}

// IsDirty returns whether a cell has a pending unsaved edit.
func (r ResultsTable) IsDirty(row, col int) bool {
	_, ok := r.dirtyCells[cellRef{row: row, col: col}]
	return ok
}

// DirtyCellCount returns the number of pending unsaved edits.
func (r ResultsTable) DirtyCellCount() int {
	return len(r.dirtyCells)
}

// IsCopied reports whether the copy-to-clipboard confirmation is showing.
func (r ResultsTable) IsCopied() bool { return r.copied }

// IsSaved reports whether the save confirmation is showing.
func (r ResultsTable) IsSaved() bool { return r.saved }

// SaveError returns the last save error message, if any.
func (r ResultsTable) SaveError() string { return r.saveError }

// SetDirtyCell records a pending cell edit (e.g. from the inspector).
func (r *ResultsTable) SetDirtyCell(row, col int, val string) {
	ref := cellRef{row: row, col: col}
	if r.dirtyCells == nil {
		r.dirtyCells = make(map[cellRef]string)
	}
	r.dirtyCells[ref] = val
}

// SetColumnTypes stores column type metadata from the query result.
func (r *ResultsTable) SetColumnTypes(types map[string]string) {
	r.columnTypes = types
}

// ColumnType returns the database type for a given column index.
func (r ResultsTable) ColumnType(col int) string {
	if col < 0 || col >= len(r.columns) {
		return ""
	}
	if r.columnTypes == nil {
		return ""
	}
	return r.columnTypes[r.columns[col]]
}

// SetSize sets the dimensions of the results panel.
func (r *ResultsTable) SetSize(width, height int) {
	r.width = width
	r.height = height
	r.clampScrollRow()
	r.clampScrollCol()
	r.clampCursor()
}

func (r *ResultsTable) computeColWidths() {
	if len(r.columns) == 0 {
		return
	}

	r.colWidths = make([]int, len(r.columns))

	for i, col := range r.columns {
		r.colWidths[i] = runeLen(col)
	}

	for _, row := range r.rows {
		for i := 0; i < len(r.columns) && i < len(row); i++ {
			l := runeLen(row[i])
			if l > r.colWidths[i] {
				r.colWidths[i] = l
			}
		}
	}

	for i := range r.colWidths {
		if r.colWidths[i] > maxCellWidth {
			r.colWidths[i] = maxCellWidth
		}
	}
}

func runeLen(s string) int {
	count := 0
	for range s {
		count++
	}
	return count
}

func (r *ResultsTable) clampScrollRow() {
	maxVisible := r.maxVisibleRows()
	if r.scrollRow < 0 {
		r.scrollRow = 0
	}
	if r.scrollRow > len(r.rows)-maxVisible && len(r.rows) > maxVisible {
		r.scrollRow = len(r.rows) - maxVisible
	}
}

func (r *ResultsTable) clampScrollCol() {
	if r.scrollCol < 0 {
		r.scrollCol = 0
	}
	totalCols := len(r.colWidths)
	if r.scrollCol >= totalCols {
		r.scrollCol = totalCols - 1
	}
	if r.scrollCol < 0 {
		r.scrollCol = 0
	}
}

func (r *ResultsTable) clampCursor() {
	if len(r.rows) == 0 {
		r.cursorRow = 0
	} else if r.cursorRow >= len(r.rows) {
		r.cursorRow = len(r.rows) - 1
	}
	if r.cursorRow < 0 {
		r.cursorRow = 0
	}

	if len(r.columns) == 0 {
		r.cursorCol = 0
	} else if r.cursorCol >= len(r.columns) {
		r.cursorCol = len(r.columns) - 1
	}
	if r.cursorCol < 0 {
		r.cursorCol = 0
	}
}

// ensureCursorVisible adjusts scroll so the cursor cell is in view.
func (r *ResultsTable) ensureCursorVisible() {
	maxVisible := r.maxVisibleRows()
	if r.cursorRow < r.scrollRow {
		r.scrollRow = r.cursorRow
	}
	if r.cursorRow >= r.scrollRow+maxVisible {
		r.scrollRow = r.cursorRow - maxVisible + 1
	}
	r.clampScrollRow()

	// Horizontal: keep cursor column visible
	colStart, _ := r.visibleColRange()
	if r.cursorCol < colStart {
		r.scrollCol = r.cursorCol
	}
	for {
		cs, ce := r.visibleColRange()
		if r.cursorCol >= cs && r.cursorCol < ce {
			break
		}
		if r.cursorCol >= ce && r.scrollCol < len(r.colWidths)-1 {
			r.scrollCol++
		} else {
			break
		}
	}
}

func (r ResultsTable) maxVisibleRows() int {
	max := r.height - 4
	if max < 1 {
		max = 1
	}
	return max
}

// ScrollDown moves the visible rows down by one.
func (r *ResultsTable) ScrollDown() {
	maxVisible := r.maxVisibleRows()
	if r.scrollRow < len(r.rows)-maxVisible {
		r.scrollRow++
	}
}

// ScrollUp moves the visible rows up by one.
func (r *ResultsTable) ScrollUp() {
	if r.scrollRow > 0 {
		r.scrollRow--
	}
}

// ScrollRight moves the visible columns right by one.
func (r *ResultsTable) ScrollRight() {
	if r.scrollCol < len(r.colWidths)-1 {
		r.scrollCol++
	}
}

// ScrollLeft moves the visible columns left by one.
func (r *ResultsTable) ScrollLeft() {
	if r.scrollCol > 0 {
		r.scrollCol--
	}
}

// ScrollTop scrolls to the first row.
func (r *ResultsTable) ScrollTop() {
	r.scrollRow = 0
}

// ScrollBottom scrolls to the last row.
func (r *ResultsTable) ScrollBottom() {
	maxVisible := r.maxVisibleRows()
	if len(r.rows) > maxVisible {
		r.scrollRow = len(r.rows) - maxVisible
	}
}

// CursorCellValue returns the full value at the current cell cursor.
func (r ResultsTable) CursorCellValue() string {
	return r.RowValue(r.cursorRow, r.cursorCol)
}

// hasCellCursor reports whether the table shows a cell selection cursor.
func (r ResultsTable) hasCellCursor() bool {
	return r.hasResult && len(r.rows) > 0
}

// CursorTop moves the cell cursor to the first row.
func (r *ResultsTable) CursorTop() {
	r.cursorRow = 0
	r.ensureCursorVisible()
}

// CursorBottom moves the cell cursor to the last row.
func (r *ResultsTable) CursorBottom() {
	r.cursorRow = len(r.rows)
	r.clampCursor()
	r.ensureCursorVisible()
}

// CursorDown moves the cell cursor down.
func (r *ResultsTable) CursorDown() {
	if r.cursorRow < len(r.rows)-1 {
		r.cursorRow++
	}
	r.ensureCursorVisible()
}

// CursorUp moves the cell cursor up.
func (r *ResultsTable) CursorUp() {
	if r.cursorRow > 0 {
		r.cursorRow--
	}
	r.ensureCursorVisible()
}

// CursorRight moves the cell cursor right (editable mode).
func (r *ResultsTable) CursorRight() {
	if r.cursorCol < len(r.columns)-1 {
		r.cursorCol++
	}
	r.ensureCursorVisible()
}

// CursorLeft moves the cell cursor left (editable mode).
func (r *ResultsTable) CursorLeft() {
	if r.cursorCol > 0 {
		r.cursorCol--
	}
	r.ensureCursorVisible()
}

// StartEdit enters inline edit mode on the current cell.
func (r *ResultsTable) StartEdit() {
	if !r.editable || len(r.rows) == 0 {
		return
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0 // no limit
	currentVal := r.RowValue(r.cursorRow, r.cursorCol)
	if currentVal == "NULL" {
		currentVal = ""
	}
	ti.SetValue(currentVal)
	colWidth := maxCellWidth
	if r.cursorCol < len(r.colWidths) {
		colWidth = r.colWidths[r.cursorCol]
	}
	ti.Width = colWidth
	ti.Focus()
	r.editInput = ti
	r.editing = true
	r.saved = false
}

// CommitEdit saves the current edit buffer to dirtyCells and exits edit mode.
func (r *ResultsTable) CommitEdit() {
	if !r.editing {
		return
	}
	newVal := r.editInput.Value()
	ref := cellRef{row: r.cursorRow, col: r.cursorCol}
	r.dirtyCells[ref] = newVal
	r.editing = false
}

// CancelEdit discards the current edit and exits edit mode.
func (r *ResultsTable) CancelEdit() {
	r.editing = false
}

// Update handles messages for the results table.
func (r ResultsTable) Update(msg tea.Msg) (ResultsTable, tea.Cmd) {
	if r.editing {
		var cmd tea.Cmd
		r.editInput, cmd = r.editInput.Update(msg)
		return r, cmd
	}
	return r, nil
}

// truncateCell truncates a string to fit within width characters,
// appending "…" if truncated.
func truncateCell(s string, width int) string {
	l := runeLen(s)
	if l <= width {
		return s + strings.Repeat(" ", width-l)
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(s)
	return string(runes[:width-1]) + "…"
}

// visibleColRange returns the start and end column indices that fit
// within the available width, starting from scrollCol.
func (r ResultsTable) visibleColRange() (int, int) {
	// Each column renders as: " " + value(colWidth) + " " + "│" = colWidth + 3
	// The leftmost "│" is 1 extra char.
	available := r.width - 1
	if available < 1 {
		available = 1
	}

	start := r.scrollCol
	used := 0
	end := start

	for i := start; i < len(r.colWidths); i++ {
		colW := r.colWidths[i] + 3
		if used+colW > available && end > start {
			break
		}
		used += colW
		end = i + 1
	}

	return start, end
}

// isPKColumn returns true if colName is a primary key column.
func (r ResultsTable) isPKColumn(colName string) bool {
	for _, pk := range r.pkColumns {
		if pk == colName {
			return true
		}
	}
	return false
}

// View renders the results table.
func (r ResultsTable) View() string {
	if !r.hasResult {
		return mutedStyle.Render(" Run a query to see results.")
	}

	if r.message != "" && len(r.columns) == 0 {
		return errorStyle.Render(r.message)
	}

	if len(r.columns) == 0 || r.height < 4 {
		return mutedStyle.Render(r.message)
	}

	maxVisible := r.maxVisibleRows()

	rowStart := r.scrollRow
	rowEnd := rowStart + maxVisible
	if rowEnd > len(r.rows) {
		rowEnd = len(r.rows)
	}

	colStart, colEnd := r.visibleColRange()

	var b strings.Builder

	// Top border
	borderColor := lipgloss.NewStyle().Foreground(colorBorder)
	b.WriteString(borderColor.Render("┌"))
	for i := colStart; i < colEnd; i++ {
		w := r.colWidths[i] + 2 // match cell content: " " + value + " "
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if i < colEnd-1 {
			b.WriteString(borderColor.Render("┬"))
		}
	}
	b.WriteString(borderColor.Render("┐"))
	b.WriteString("\n")

	// Header row
	b.WriteString(borderColor.Render("│"))
	for i := colStart; i < colEnd; i++ {
		header := r.columns[i]
		if r.editable && r.isPKColumn(header) {
			header = header + " 🔑"
		}
		cell := truncateCell(header, r.colWidths[i])
		style := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
		if r.hasCellCursor() && i == r.cursorCol {
			style = style.Underline(true)
		}
		b.WriteString(style.Render(" " + cell + " "))
		b.WriteString(borderColor.Render("│"))
	}
	b.WriteString("\n")

	// Header separator
	b.WriteString(borderColor.Render("├"))
	for i := colStart; i < colEnd; i++ {
		w := r.colWidths[i] + 2
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if i < colEnd-1 {
			b.WriteString(borderColor.Render("┼"))
		}
	}
	b.WriteString(borderColor.Render("┤"))
	b.WriteString("\n")

	// Data rows
	for rowIdx := rowStart; rowIdx < rowEnd; rowIdx++ {
		row := r.rows[rowIdx]
		isCursorRow := r.hasCellCursor() && rowIdx == r.cursorRow
		b.WriteString(borderColor.Render("│"))
		for i := colStart; i < colEnd; i++ {
			ref := cellRef{row: rowIdx, col: i}
			dirtyVal, isDirty := r.dirtyCells[ref]
			isCursorCell := isCursorRow && i == r.cursorCol
			isCopyFlash := r.isCopyFlashCell(rowIdx, i)

			val := ""
			if i < len(row) {
				val = row[i]
			}
			if isDirty {
				val = dirtyVal
			}

			// If this is the cell being edited, show the input buffer.
			if r.editing && isCursorCell {
				inputText := r.editInput.Value()
				cell := truncateCell(inputText, r.colWidths[i])
				b.WriteString(lipgloss.NewStyle().
					Foreground(colorFg).
					Background(colorHighlight).
					Render(" " + cell + " "))
				b.WriteString(borderColor.Render("│"))
				continue
			}

			cell := truncateCell(val, r.colWidths[i])
			if isCursorCell && r.IsNavigableForeignKey(rowIdx, i) {
				arrow := " →"
				arrowW := lipgloss.Width(arrow)
				cell = truncateCell(val, r.colWidths[i]-arrowW) + arrow
			}

			// Style the cell
			var style lipgloss.Style
			switch {
			case isCopyFlash && r.copyFlashOn:
				style = lipgloss.NewStyle().Foreground(colorBg).Background(colorSuccess)
			case isCursorCell:
				style = lipgloss.NewStyle().Foreground(colorBg).Background(colorPrimary)
			case isDirty:
				style = lipgloss.NewStyle().Foreground(colorBg).Background(lipgloss.Color("#e0af68"))
			default:
				style = lipgloss.NewStyle().Foreground(colorFg)
			}
			b.WriteString(style.Render(" " + cell + " "))
			b.WriteString(borderColor.Render("│"))
		}
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString(borderColor.Render("└"))
	for i := colStart; i < colEnd; i++ {
		w := r.colWidths[i] + 2
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if i < colEnd-1 {
			b.WriteString(borderColor.Render("┴"))
		}
	}
	b.WriteString(borderColor.Render("┘"))

	return b.String()
}
