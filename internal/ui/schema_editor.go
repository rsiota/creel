package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
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
	rowOrigin []int     // maps grid row → index into columns, or -1 for a new (pending) row
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
}

// SchemaEditorResult carries the SQL built from a single cell edit so app.go
// can run it asynchronously. It distinguishes the schema action so the result
// handler knows what to refresh.
type SchemaEditorResult struct {
	SQL     string
	Action  db.SchemaAction
	ErrFunc func(err string)
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

	// Grid navigation and actions.
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

// View renders the editor: header + grid + footer.
func (e SchemaEditor) View() string {
	if !e.visible {
		return ""
	}

	var lines []string
	lines = append(lines, titleStyle.Render(fmt.Sprintf("Edit Schema: %s", e.table)))
	lines = append(lines, "")

	lines = append(lines, e.renderGrid())

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

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (e SchemaEditor) renderGrid() string {
	borderColor := lipgloss.NewStyle().Foreground(colorBorder)

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
				b.WriteString(" " + e.editInput.View() + " ")
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
