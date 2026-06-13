package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// ResultsTable renders query results as a scrollable table.
type ResultsTable struct {
	columns   []string
	rows      [][]string
	scrollRow int
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

	maxVisible := height - 5
	if maxVisible < 1 {
		maxVisible = 1
	}
	if r.scrollRow > len(r.rows)-maxVisible && len(r.rows) > maxVisible {
		r.scrollRow = len(r.rows) - maxVisible
	}
}

// ScrollDown moves the visible rows down by one.
func (r *ResultsTable) ScrollDown() {
	maxVisible := r.height - 5
	if maxVisible < 1 {
		maxVisible = 1
	}
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

// Update handles messages for the results table.
func (r ResultsTable) Update(msg tea.Msg) (ResultsTable, tea.Cmd) {
	return r, nil
}

// View renders the results table.
func (r ResultsTable) View() string {
	if !r.hasResult {
		return mutedStyle.Render("Run a query to see results.")
	}

	if r.message != "" && len(r.columns) == 0 {
		return errorStyle.Render(r.message)
	}

	maxVisible := r.height - 6
	if maxVisible < 1 {
		maxVisible = 1
	}

	end := r.scrollRow + maxVisible
	if end > len(r.rows) {
		end = len(r.rows)
	}

	visibleRows := r.rows[r.scrollRow:end]
	if visibleRows == nil {
		visibleRows = [][]string{}
	}

	colStyles := make([]lipgloss.Style, len(r.columns))
	for i := range colStyles {
		colStyles[i] = lipgloss.NewStyle().Foreground(colorFg).Padding(0, 1)
	}

	headerStyles := make([]lipgloss.Style, len(r.columns))
	for i := range headerStyles {
		headerStyles[i] = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Padding(0, 1)
	}

	cellStyles := make([][]lipgloss.Style, len(visibleRows))
	for i := range cellStyles {
		cellStyles[i] = colStyles
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(colorBorder)).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return headerStyles[col]
			}
			return colStyles[col]
		}).
		Headers(r.columns...).
		Rows(visibleRows...).
		Width(r.width)

	var b strings.Builder
	b.WriteString(t.Render())
	b.WriteByte('\n')

	prefix := mutedStyle.Render(fmt.Sprintf("%d-%d of %d", r.scrollRow+1, end, len(r.rows)))
	b.WriteString(fmt.Sprintf("%s  %s", prefix, successStyle.Render(r.message)))

	return b.String()
}

// HelpText returns keybinding hints for the results panel.
func (r ResultsTable) HelpText() string {
	return fmt.Sprintf("%s/%s scroll results",
		mutedStyle.Render("Ctrl+↑"),
		mutedStyle.Render("Ctrl+↓"),
	)
}
