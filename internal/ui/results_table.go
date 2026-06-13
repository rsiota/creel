package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maxCellWidth = 40

// ResultsTable renders query results as a scrollable table with
// per-cell truncation and horizontal scrolling.
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
}

// NewResultsTable creates a new results table component.
func NewResultsTable() ResultsTable {
	return ResultsTable{}
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
}

// Message returns the current status message.
func (r ResultsTable) Message() string {
	return r.message
}

// HasResult returns whether the table has data to display.
func (r ResultsTable) HasResult() bool {
	return r.hasResult
}

// SetSize sets the dimensions of the results panel.
func (r *ResultsTable) SetSize(width, height int) {
	r.width = width
	r.height = height
	r.clampScrollRow()
	r.clampScrollCol()
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

func (r ResultsTable) maxVisibleRows() int {
	max := r.height - 5
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

// Update handles messages for the results table.
func (r ResultsTable) Update(msg tea.Msg) (ResultsTable, tea.Cmd) {
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

// View renders the results table.
func (r ResultsTable) View() string {
	if !r.hasResult {
		return mutedStyle.Render("Run a query to see results.")
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
		cell := truncateCell(r.columns[i], r.colWidths[i])
		b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(" " + cell + " "))
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
	visibleRows := r.rows[rowStart:rowEnd]
	for _, row := range visibleRows {
		b.WriteString(borderColor.Render("│"))
		for i := colStart; i < colEnd; i++ {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			cell := truncateCell(val, r.colWidths[i])
			b.WriteString(lipgloss.NewStyle().Foreground(colorFg).Render(" " + cell + " "))
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
	b.WriteString("\n")

	// Status line
	colInfo := fmt.Sprintf("cols %d-%d of %d", colStart+1, colEnd, len(r.columns))
	if len(r.colWidths) <= colEnd-colStart {
		colInfo = fmt.Sprintf("%d cols", len(r.columns))
	}
	rowInfo := fmt.Sprintf("rows %d-%d of %d", rowStart+1, rowEnd, len(r.rows))
	b.WriteString(fmt.Sprintf("%s  %s  %s",
		mutedStyle.Render(rowInfo),
		mutedStyle.Render(colInfo),
		successStyle.Render(r.message),
	))

	return b.String()
}

// HelpText returns keybinding hints for the results panel.
func (r ResultsTable) HelpText() string {
	return fmt.Sprintf("%s/%s scroll  %s/%s horizontal",
		mutedStyle.Render("j/k"),
		mutedStyle.Render(""),
		mutedStyle.Render("h/l"),
		mutedStyle.Render(""),
	)
}
