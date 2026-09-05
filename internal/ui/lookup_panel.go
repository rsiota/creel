package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/db"
)

// LookupPanel is a scrollable, read-only overlay that displays a titled table
// of lookup results (a db.Result). It backs ":sizes", ":tables", ":refs",
// ":uses", ":locks", ":who", ":diagnose", ":search", and related commands. The cursor moves among data rows
// only (title and column header stay fixed); Enter opens the jump target for
// the selected row when one was supplied (e.g. a table name from :sizes).
type LookupPanel struct {
	visible bool
	title   string
	result  db.Result
	jumps   []string // parallel to result.Rows; empty entry = no Enter action
	cursor  int      // index into result.Rows
	scroll  int      // first visible data-row index
	width   int
	height  int
}

func (p LookupPanel) IsVisible() bool { return p.visible }

// Show populates the panel with a title and result table and makes it visible.
// jumps is optional and parallel to result.Rows: a non-empty entry makes that
// row Enter-jumpable (typically a table name). Pass nil when nothing is
// jumpable (e.g. :peek).
func (p *LookupPanel) Show(title string, result db.Result, jumps []string) {
	p.visible = true
	p.title = title
	p.result = result
	p.jumps = jumps
	p.cursor = 0
	p.scroll = 0
	if len(result.Rows) == 0 {
		p.cursor = -1
	}
}

// Hide hides the panel.
func (p *LookupPanel) Hide() { p.visible = false }

// SetSize sets the content dimensions of the panel (including border).
func (p *LookupPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SelectedJump returns the jump target for the cursor row, or "" when the
// cursor is unset or that row has no jump.
func (p LookupPanel) SelectedJump() string {
	if p.cursor < 0 || p.cursor >= len(p.jumps) {
		return ""
	}
	return p.jumps[p.cursor]
}

// HasJumps reports whether any row is Enter-jumpable (for hints).
func (p LookupPanel) HasJumps() bool {
	for _, j := range p.jumps {
		if j != "" {
			return true
		}
	}
	return false
}

func (p LookupPanel) contentHeight() int {
	h := p.height - borderOverhead
	if h < 1 {
		h = 1
	}
	return h
}

// fixedHeaderLines is the number of non-scrollable lines pinned above the
// data rows (title + optional column header).
func (p LookupPanel) fixedHeaderLines() int {
	n := 1 // title
	if len(p.result.Columns) > 0 {
		n++
	}
	return n
}

// dataViewportHeight is how many data rows fit below the fixed header.
func (p LookupPanel) dataViewportHeight() int {
	vh := p.contentHeight() - p.fixedHeaderLines()
	if vh < 1 {
		vh = 1
	}
	return vh
}

// Update handles keyboard input (data-row navigation). Returns the updated panel.
func (p LookupPanel) Update(msg tea.KeyMsg) LookupPanel {
	n := len(p.result.Rows)
	if n == 0 {
		return p
	}
	vh := p.dataViewportHeight()
	switch msg.String() {
	case "j", "down":
		if p.cursor < n-1 {
			p.cursor++
			p.adjustScroll(vh)
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
			p.adjustScroll(vh)
		}
	case "g":
		p.cursor = 0
		p.scroll = 0
	case "G":
		p.cursor = n - 1
		p.adjustScroll(vh)
	case "ctrl+d":
		p.cursor += vh / 2
		if p.cursor >= n {
			p.cursor = n - 1
		}
		p.adjustScroll(vh)
	case "ctrl+u":
		p.cursor -= vh / 2
		if p.cursor < 0 {
			p.cursor = 0
		}
		p.adjustScroll(vh)
	}
	return p
}

func (p *LookupPanel) adjustScroll(vh int) {
	if p.cursor < p.scroll {
		p.scroll = p.cursor
	}
	if p.cursor >= p.scroll+vh {
		p.scroll = p.cursor - vh + 1
	}
	if p.scroll < 0 {
		p.scroll = 0
	}
}

// View renders the panel with a border. Title and column header stay fixed;
// the selected data row is highlighted like the command palette.
func (p LookupPanel) View() string {
	contentW := p.width - borderOverhead - 2 // border + Padding(0,1)
	if contentW < 1 {
		contentW = 1
	}
	vh := p.dataViewportHeight()

	title := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(
		fmt.Sprintf("%s (%d)", p.title, len(p.result.Rows)))

	var lines []string
	lines = append(lines, title)

	header, dataRows := lookupTableParts(p.result)
	if header != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(header))
	}

	end := p.scroll + vh
	if end > len(dataRows) {
		end = len(dataRows)
	}
	start := p.scroll
	if start > len(dataRows) {
		start = 0
	}
	for i := start; i < end; i++ {
		raw := padLookupRow(dataRows[i], contentW)
		if i == p.cursor {
			lines = append(lines, lipgloss.NewStyle().
				Background(colorPrimary).Foreground(colorBg).
				Render(raw))
		} else {
			lines = append(lines, lipgloss.NewStyle().Foreground(colorFg).Render(raw))
		}
	}
	for len(lines) < p.contentHeight() {
		lines = append(lines, "")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)
}

// padLookupRow right-pads s to width display columns so the highlight bar
// fills the panel width.
func padLookupRow(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return truncateCell(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

// lookupTableParts returns a plain column-header line and one plain line per
// data row (no ANSI), sharing the width rules of renderGenericPlan.
func lookupTableParts(result db.Result) (header string, rows []string) {
	if len(result.Columns) == 0 {
		return "", nil
	}
	headers := make([]string, len(result.Columns))
	for i, c := range result.Columns {
		headers[i] = c.Name
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range result.Rows {
		for i := range headers {
			if i < len(row) && len(row[i]) > widths[i] {
				widths[i] = len(row[i])
			}
		}
	}
	headerCells := make([]string, len(headers))
	for i, h := range headers {
		headerCells[i] = padRight(h, widths[i])
	}
	header = strings.Join(headerCells, " │ ")
	rows = make([]string, len(result.Rows))
	for ri, row := range result.Rows {
		cells := make([]string, len(headers))
		for i := range headers {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			cells[i] = padRight(val, widths[i])
		}
		rows[ri] = strings.Join(cells, " │ ")
	}
	return header, rows
}
