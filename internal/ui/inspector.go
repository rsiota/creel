package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InspectorWidth is the column width reserved for the inspector panel
// (including borders) when it is visible.
const InspectorWidth = 45

// linesPerField is the vertical space each field occupies in the form layout:
// label line + top border + value line + bottom border.
const linesPerField = 4

// Inspector is a right-side panel that displays all column values of the
// currently selected result row as a vertical form. Each field shows the
// column name as a label above a bordered value box. When the underlying
// results are editable, individual fields can be modified inline.
type Inspector struct {
	width       int
	height      int
	visible     bool
	cursorField int
	editing     bool
	editInput   textinput.Model
	scrollRow   int
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
}

// Hide forcibly closes the inspector.
func (i *Inspector) Hide() {
	i.visible = false
	i.editing = false
	i.cursorField = 0
	i.scrollRow = 0
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
}

// CursorUp moves the field cursor up by one.
func (i *Inspector) CursorUp() {
	if i.cursorField > 0 {
		i.cursorField--
	}
	i.ensureFieldVisible()
}

// CursorDown moves the field cursor down by one.
func (i *Inspector) CursorDown(numFields int) {
	if i.cursorField < numFields-1 {
		i.cursorField++
	}
	i.ensureFieldVisible()
}

// visibleFieldCount returns how many complete fields fit in the available height.
func (i Inspector) visibleFieldCount() int {
	avail := i.height - 4 // title + row info + blank + help
	if avail < linesPerField {
		return 1
	}
	return avail / linesPerField
}

// ensureFieldVisible adjusts scrollRow so the cursor field stays in view.
func (i *Inspector) ensureFieldVisible() {
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
}

// StartFieldEdit begins editing the currently focused field.
func (i *Inspector) StartFieldEdit(results ResultsTable) {
	if !results.IsEditable() || results.NumCols() == 0 || results.NumRows() == 0 {
		return
	}
	colName := results.ColumnName(i.cursorField)
	if results.isPKColumn(colName) {
		return
	}

	row := results.CursorRow()
	val := results.RowValue(row, i.cursorField)
	if val == "NULL" {
		val = ""
	}

	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.SetValue(val)

	valueWidth := i.width - 4 // "│ " + value + " │"
	if valueWidth < 10 {
		valueWidth = 10
	}
	ti.Width = valueWidth
	ti.Focus()
	i.editInput = ti
	i.editing = true
}

// CommitFieldEdit finalizes the edit and returns the column index and new value.
func (i *Inspector) CommitFieldEdit() (col int, val string, ok bool) {
	if !i.editing {
		return 0, "", false
	}
	col = i.cursorField
	val = i.editInput.Value()
	i.editing = false
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

// View renders the inspector content (without outer border) as a vertical form.
// Each field is: column-name label, then a bordered value box. The focused
// field's box border uses the primary color; all others use the table grid color.
func (i Inspector) View(results ResultsTable) string {
	numFields := results.NumCols()
	if numFields == 0 || results.NumRows() == 0 {
		return mutedStyle.Render(" No row selected")
	}

	row := results.CursorRow()
	if !results.IsEditable() {
		row = results.ScrollRow()
	}
	if row >= results.NumRows() {
		row = results.NumRows() - 1
	}
	if row < 0 {
		row = 0
	}

	tableName := results.SourceTable()
	if tableName == "" {
		tableName = "Row"
	}

	// Value box: "│ " + value(valueWidth) + " │"
	valueWidth := i.width - 4
	if valueWidth < 5 {
		valueWidth = 5
	}
	borderWidth := valueWidth + 2

	// Determine visible field range.
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

	borderNormal := lipgloss.NewStyle().Foreground(colorBorder)
	borderFocused := lipgloss.NewStyle().Foreground(colorPrimary)

	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render(tableName))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("row %d of %d", row+1, results.NumRows())))
	b.WriteString("\n")

	// Fields
	for c := start; c < end; c++ {
		colName := results.ColumnName(c)
		isPK := results.isPKColumn(colName)
		isDirty := results.IsDirty(row, c)
		isFocused := c == cursorClamp
		val := results.RowValue(row, c)

		// Label line
		label := " " + colName
		if isPK {
			label = " 🔑 " + colName
		}
		if isDirty {
			label += " ●"
		}
		label = truncateSidebarLine(label, i.width)
		labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
		if isPK {
			labelStyle = lipgloss.NewStyle().Foreground(colorAccent)
		}
		b.WriteString(labelStyle.Render(label))
		b.WriteString("\n")

		// Choose border color for this field's value box.
		bs := borderNormal
		if isFocused {
			bs = borderFocused
		}

		// Top border
		b.WriteString(bs.Render("┌" + strings.Repeat("─", borderWidth) + "┐"))
		b.WriteString("\n")

		// Value line
		var displayVal string
		valStyle := lipgloss.NewStyle().Foreground(colorFg)

		if i.editing && isFocused {
			displayVal = truncateCell(i.editInput.Value(), valueWidth)
			valStyle = lipgloss.NewStyle().
				Foreground(colorFg).
				Background(colorHighlight)
		} else {
			displayVal = truncateCell(val, valueWidth)
			if val == "NULL" {
				valStyle = lipgloss.NewStyle().Foreground(colorMuted)
			}
			if isDirty {
				valStyle = lipgloss.NewStyle().Foreground(colorSuccess)
			}
		}

		b.WriteString(bs.Render("│ ") + valStyle.Render(displayVal) + bs.Render(" │"))
		b.WriteString("\n")

		// Bottom border
		b.WriteString(bs.Render("└" + strings.Repeat("─", borderWidth) + "┘"))
		b.WriteString("\n")
	}

	// Bottom help
	b.WriteString("\n")
	if i.editing {
		b.WriteString(mutedStyle.Render("enter: commit  esc: cancel"))
	} else if results.IsEditable() {
		if results.HasDirtyCells() {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("%d unsaved · ctrl+s: save", results.DirtyCellCount())))
		} else {
			b.WriteString(mutedStyle.Render("enter: edit  ctrl+s: save"))
		}
	} else {
		b.WriteString(mutedStyle.Render("read-only"))
	}

	return b.String()
}
