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

// Fixed columns of the designer grid.
const (
	tdColName = iota
	tdColType
	tdColNull
	tdColDefault
	tdColCount
)

var tdHeaders = []string{"Name", "Type", "Null", "Default"}

// TableDesigner is an inline grid editor for defining a new table. It mirrors
// the look and feel of ResultsTable so users can design a table with the same
// muscle memory they use to browse query results.
//
// The component owns its full view: a table-name input, a bordered grid with
// inline cell editing, and a keybinding footer. It takes over the right panel
// (editor + results space) when active.
type TableDesigner struct {
	driver    db.Driver
	existing  []string // existing table names, for duplicate check
	nameField textinput.Model
	rows      [][]string // each row: [name, type, nullable, default]
	cursorRow int
	cursorCol int
	focusName bool // table-name field focused (vs grid)
	editing   bool
	editInput textinput.Model
	pendingD  bool // vim-style 'dd' state
	visible   bool
	width     int
	height    int
	errMsg    string
	colWidths []int

	// Mouse: last left-click, for double-click-to-edit detection (mirrors the
	// schema editor / results panel).
	lastClickTime time.Time
	lastClickRow  int
	lastClickCol  int
}

// NewTableDesigner creates a hidden designer.
func NewTableDesigner() TableDesigner {
	return TableDesigner{}
}

// Show opens the designer with sensible defaults: one id row and one blank row.
func (d *TableDesigner) Show(driver db.Driver, existing []string) {
	d.driver = driver
	d.existing = append([]string(nil), existing...)
	d.visible = true
	d.errMsg = ""
	d.cursorRow = 0
	d.cursorCol = 0
	d.focusName = true
	d.editing = false
	d.pendingD = false
	d.lastClickTime = time.Time{}

	d.nameField = newAddColumnInput("table name", "")
	d.nameField.Focus()

	d.rows = [][]string{
		{"id", "INTEGER", "no", ""},
		{"", "", "yes", ""},
	}
	d.computeColWidths()
}

// Hide closes the designer and resets state.
func (d *TableDesigner) Hide() {
	d.visible = false
	d.existing = nil
	d.nameField = textinput.Model{}
	d.rows = nil
	d.errMsg = ""
	d.focusName = true
	d.editing = false
	d.pendingD = false
	d.cursorRow = 0
	d.cursorCol = 0
}

// IsVisible reports whether the designer is active.
func (d TableDesigner) IsVisible() bool { return d.visible }

// IsEditing reports whether a cell (or the name field) is being edited inline.
func (d TableDesigner) IsEditing() bool { return d.editing }

// TableName returns the trimmed table name.
func (d TableDesigner) TableName() string {
	return strings.TrimSpace(d.nameField.Value())
}

// SetError sets a validation/execution error message.
func (d *TableDesigner) SetError(msg string) { d.errMsg = msg }

// SetSize sets the dimensions available for the full designer view.
func (d *TableDesigner) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.nameField.Width = width - 16
	if d.nameField.Width < 20 {
		d.nameField.Width = 20
	}
	d.computeColWidths()
}

func (d *TableDesigner) computeColWidths() {
	// Each cell renders as " " + value(w) + " " + "│" = w + 3, plus leading │ = 1.
	available := d.width - 1
	if available < 20 {
		available = 20
	}
	// Fixed, content-appropriate widths for the first three columns; Default
	// (the last) absorbs the remaining space.
	nameW := 20
	typeW := 18
	nullW := 6
	defaultW := available - nameW - typeW - nullW - (tdColCount * 3)
	if defaultW < 10 {
		defaultW = 10
	}
	d.colWidths = []int{nameW, typeW, nullW, defaultW}
}

func (d TableDesigner) maxVisibleRows() int {
	// Reserve: name line (1), blank (1), grid chrome (4), blank (1), footer (2).
	max := d.height - 9
	if max < 1 {
		max = 1
	}
	return max
}

// Update handles keyboard input. Returns a cmd (typically for textinput focus).
func (d TableDesigner) Update(msg tea.Msg) (TableDesigner, tea.Cmd) {
	if !d.visible {
		return d, nil
	}
	var cmd tea.Cmd

	// If inline-editing a cell, route everything to the edit input except
	// commit/cancel.
	if d.editing {
		switch m := msg.(type) {
		case tea.KeyMsg:
			switch m.String() {
			case "enter", "ctrl+s":
				d.commitCellEdit()
				return d, nil
			case "esc", "ctrl+c":
				d.editing = false
				return d, nil
			case "tab":
				d.commitCellEdit()
				d.cursorRight()
				d.startCellEdit()
				return d, nil
			}
		}
		d.editInput, cmd = d.editInput.Update(msg)
		return d, cmd
	}

	// Editing the table name field.
	if d.focusName {
		switch m := msg.(type) {
		case tea.KeyMsg:
			switch m.String() {
			case "enter", "ctrl+s":
				return d, nil // submit handled by app.go
			case "tab", "down", "j":
				d.nameField.Blur()
				d.focusName = false
				d.cursorRow = 0
				d.cursorCol = 0
				return d, nil
			}
		}
		d.nameField, cmd = d.nameField.Update(msg)
		return d, cmd
	}

	// Grid navigation.
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			d.pendingD = false
			d.cursorUp()
			return d, nil
		case "down", "j":
			d.pendingD = false
			d.cursorDown()
			return d, nil
		case "left", "h":
			d.pendingD = false
			d.cursorLeft()
			return d, nil
		case "right", "l":
			d.pendingD = false
			d.cursorRight()
			return d, nil
		case "tab":
			d.pendingD = false
			d.cursorRight()
			return d, nil
		case "shift+tab":
			d.pendingD = false
			d.cursorLeft()
			return d, nil
		case "e", "i", "a":
			d.pendingD = false
			d.startCellEdit()
			return d, nil
		case "o":
			d.pendingD = false
			d.addRowBelow()
			return d, nil
		case "O":
			d.pendingD = false
			d.addRowAbove()
			return d, nil
		case "d":
			if d.pendingD {
				d.removeCurrentRow()
				d.pendingD = false
			} else {
				d.pendingD = true
			}
			return d, nil
		case "enter", "ctrl+s":
			return d, nil // submit handled by app.go
		case "esc":
			return d, nil // exit handled by app.go
		}
	}
	return d, nil
}

func (d *TableDesigner) cursorUp() {
	if d.cursorRow > 0 {
		d.cursorRow--
	} else {
		// Move to table name.
		d.focusName = true
		d.nameField.Focus()
	}
}

func (d *TableDesigner) cursorDown() {
	if d.cursorRow < len(d.rows)-1 {
		d.cursorRow++
	}
}

func (d *TableDesigner) cursorLeft() {
	if d.cursorCol > 0 {
		d.cursorCol--
	}
}

func (d *TableDesigner) cursorRight() {
	if d.cursorCol < tdColCount-1 {
		d.cursorCol++
	} else {
		d.cursorCol = 0
		d.cursorDown()
	}
}

func (d *TableDesigner) startCellEdit() {
	if d.cursorRow >= len(d.rows) {
		return
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	current := d.rows[d.cursorRow][d.cursorCol]
	if current == "NULL" {
		current = ""
	}
	ti.SetValue(current)
	w := d.colWidths[d.cursorCol] - 1
	if w < 5 {
		w = 5
	}
	ti.Width = w
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorEdit)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colorFg).Background(colorBg)
	ti.Focus()
	d.editInput = ti
	d.editing = true
}

func (d *TableDesigner) commitCellEdit() {
	if !d.editing {
		return
	}
	d.rows[d.cursorRow][d.cursorCol] = d.editInput.Value()
	d.editing = false
}

func (d *TableDesigner) addRowBelow() {
	newRow := make([]string, tdColCount)
	newRow[tdColNull] = "yes"
	d.rows = append(d.rows, nil)
	copy(d.rows[d.cursorRow+2:], d.rows[d.cursorRow+1:])
	d.rows[d.cursorRow+1] = newRow
	d.cursorRow++
}

func (d *TableDesigner) addRowAbove() {
	newRow := make([]string, tdColCount)
	newRow[tdColNull] = "yes"
	d.rows = append(d.rows, nil)
	copy(d.rows[d.cursorRow+1:], d.rows[d.cursorRow:])
	d.rows[d.cursorRow] = newRow
}

func (d *TableDesigner) removeCurrentRow() {
	if len(d.rows) <= 1 {
		return
	}
	d.rows = append(d.rows[:d.cursorRow], d.rows[d.cursorRow+1:]...)
	if d.cursorRow >= len(d.rows) {
		d.cursorRow = len(d.rows) - 1
	}
}

// columnDefs converts the grid rows into ColumnDefs for the SQL builder.
func (d TableDesigner) columnDefs() []db.ColumnDef {
	cols := make([]db.ColumnDef, 0, len(d.rows))
	for _, row := range d.rows {
		name := strings.TrimSpace(row[tdColName])
		colType := strings.TrimSpace(row[tdColType])
		nullable := true
		switch strings.ToLower(strings.TrimSpace(row[tdColNull])) {
		case "no", "n", "false":
			nullable = false
		}
		col := db.ColumnDef{
			Name:    name,
			Type:    colType,
			NotNull: !nullable,
		}
		if strings.EqualFold(name, "id") {
			col.PrimaryKey = true
		}
		if dv := strings.TrimSpace(row[tdColDefault]); dv != "" {
			col.HasDefault = true
			col.Default = dv
		}
		cols = append(cols, col)
	}
	return cols
}

// Submit validates and returns the generated SQL or an error message.
func (d TableDesigner) Submit() (string, string) {
	sql, err := db.BuildCreateTableSQL(d.driver, d.TableName(), d.columnDefs(), d.existing)
	if err != nil {
		return "", err.Error()
	}
	return sql, ""
}

// Focus returns focus to the table name field.
func (d *TableDesigner) Focus() tea.Cmd {
	d.focusName = true
	return d.nameField.Focus()
}

// gridColumnAtX maps a content-relative X to a grid column index using the fixed
// colWidths. The grid's leading border is at X=0; each column spans
// colWidths[j]+2 content chars (1-char cell padding) followed by a 1-char
// separator. Returns -1 when X is past the last column.
func (d TableDesigner) gridColumnAtX(x int) int {
	if x < 0 || len(d.colWidths) == 0 {
		return -1
	}
	cursor := 1 // skip the leading │
	for j, w := range d.colWidths {
		cellW := w + 2
		if x < cursor+cellW {
			return j
		}
		cursor += cellW + 1 // +1 for the trailing │
	}
	return -1
}

// blurName unfocuses the table-name field if it is focused.
func (d *TableDesigner) blurName() {
	if d.focusName {
		d.nameField.Blur()
		d.focusName = false
	}
}

// Click handles a left mouse-click at content-relative coordinates (x,y),
// where (0,0) is the first cell inside the panel border. The table-name line
// (y==0) focuses the name field; a grid data cell moves the cursor there and,
// on a double-click, starts an inline edit — mirroring the results table and
// the structure view's Columns grid. Returns a focus command when it focuses
// the name field.
func (d *TableDesigner) Click(x, y int) tea.Cmd {
	if x < 0 || y < 0 {
		return nil
	}
	// A click always abandons an in-progress inline edit (cancel, not commit —
	// press enter to save) so the edit input never lingers on the wrong cell
	// after the cursor moves away.
	if d.editing {
		d.editing = false
	}
	// Table-name line.
	if y == 0 {
		if !d.focusName {
			d.focusName = true
			return d.nameField.Focus()
		}
		return nil
	}
	// Grid layout: name(0), blank(1), top border(2), header(3), separator(4),
	// data rows from y==5.
	if y < 5 {
		d.blurName()
		return nil
	}
	row := d.gridTopRow() + (y - 5)
	col := d.gridColumnAtX(x)
	if row < 0 || row >= len(d.rows) || col < 0 || col >= tdColCount {
		d.blurName()
		return nil
	}
	d.blurName()
	// Double-click on the same cell → inline edit.
	if !d.editing &&
		!d.lastClickTime.IsZero() &&
		time.Since(d.lastClickTime) <= doubleClickInterval &&
		d.lastClickRow == row && d.lastClickCol == col {
		d.lastClickTime = time.Time{}
		d.cursorRow = row
		d.cursorCol = col
		d.startCellEdit()
		return nil
	}
	d.lastClickTime = time.Now()
	d.lastClickRow = row
	d.lastClickCol = col
	d.cursorRow = row
	d.cursorCol = col
	return nil
}

// Wheel scrolls the grid by one row (the viewport is cursor-centered, so the
// cursor moves), matching j/k.
func (d *TableDesigner) Wheel(delta int) {
	d.blurName()
	if delta > 0 {
		d.cursorDown()
	} else {
		d.cursorUp()
	}
}

// View renders the full designer: name input + grid + footer.
func (d TableDesigner) View() string {
	if !d.visible {
		return ""
	}

	var lines []string

	// Table name line.
	nameMarker := mutedStyle.Render("  Name")
	if d.focusName {
		nameMarker = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("→ Name")
	}
	nameInput := d.nameField.View()
	if d.focusName {
		nameInput = lipgloss.NewStyle().Foreground(colorFg).Render(nameInput)
	}
	lines = append(lines, fmt.Sprintf("%s  %s", nameMarker, nameInput))
	lines = append(lines, "")

	// Grid.
	lines = append(lines, d.renderGrid())

	// Error.
	if d.errMsg != "" {
		lines = append(lines, "")
		lines = append(lines, errorStyle.Render(d.errMsg))
	}

	// Pending destructive action indicator.
	if d.pendingD {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(colorAccent).Render("d...")+mutedStyle.Render("  press d again to remove row   esc cancel"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// gridTopRow returns the first visible data-row index, keeping the cursor
// roughly centered. Shared by renderGrid and the click→row mapping.
func (d TableDesigner) gridTopRow() int {
	maxVisible := d.maxVisibleRows()
	rowStart := d.cursorRow - maxVisible/2
	if rowStart < 0 {
		rowStart = 0
	}
	rowEnd := rowStart + maxVisible
	if rowEnd > len(d.rows) {
		rowEnd = len(d.rows)
	}
	rowStart = rowEnd - maxVisible
	if rowStart < 0 {
		rowStart = 0
	}
	return rowStart
}

func (d TableDesigner) renderGrid() string {
	borderColor := lipgloss.NewStyle().Foreground(colorBorder)

	rowStart := d.gridTopRow()
	rowEnd := rowStart + d.maxVisibleRows()
	if rowEnd > len(d.rows) {
		rowEnd = len(d.rows)
	}

	var b strings.Builder

	// Top border.
	b.WriteString(borderColor.Render("┌"))
	for j := 0; j < tdColCount; j++ {
		w := d.colWidths[j] + 2
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if j < tdColCount-1 {
			b.WriteString(borderColor.Render("┬"))
		}
	}
	b.WriteString(borderColor.Render("┐"))
	b.WriteString("\n")

	// Header row.
	b.WriteString(borderColor.Render("│"))
	for j := 0; j < tdColCount; j++ {
		header := tdHeaders[j]
		cell := truncateCell(header, d.colWidths[j])
		style := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
		b.WriteString(style.Render(" " + cell + " "))
		b.WriteString(borderColor.Render("│"))
	}
	b.WriteString("\n")

	// Header separator.
	b.WriteString(borderColor.Render("├"))
	for j := 0; j < tdColCount; j++ {
		w := d.colWidths[j] + 2
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if j < tdColCount-1 {
			b.WriteString(borderColor.Render("┼"))
		}
	}
	b.WriteString(borderColor.Render("┤"))
	b.WriteString("\n")

	// Data rows.
	for rowIdx := rowStart; rowIdx < rowEnd; rowIdx++ {
		row := d.rows[rowIdx]
		isCursorRow := rowIdx == d.cursorRow
		b.WriteString(borderColor.Render("│"))
		for j := 0; j < tdColCount; j++ {
			isCursorCell := isCursorRow && j == d.cursorCol

			// Inline edit input.
			if d.editing && isCursorCell {
				b.WriteString(" " + renderEditInput(d.editInput, d.colWidths[j], colorEdit) + " ")
				b.WriteString(borderColor.Render("│"))
				continue
			}

			val := row[j]
			display := val
			isPlaceholder := false
			if val == "" {
				display = "(empty)"
				isPlaceholder = true
			}
			// Pad/truncate the plain text before applying any style, so ANSI
			// escape codes don't corrupt the width measurement.
			cell := truncateCell(display, d.colWidths[j])

			var style lipgloss.Style
			switch {
			case isCursorCell:
				style = lipgloss.NewStyle().Foreground(colorBg).Background(colorPrimary)
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
	for j := 0; j < tdColCount; j++ {
		w := d.colWidths[j] + 2
		b.WriteString(borderColor.Render(strings.Repeat("─", w)))
		if j < tdColCount-1 {
			b.WriteString(borderColor.Render("┴"))
		}
	}
	b.WriteString(borderColor.Render("┘"))

	return b.String()
}
