package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

// ExplainPanel displays the output of EXPLAIN/EXPLAIN QUERY PLAN as a
// scrollable overlay. SQLite's plan is rendered as an indented tree based on
// the id/parent columns; MySQL's tabular output is shown as a table;
// PostgreSQL's single-column text output is shown verbatim.
type ExplainPanel struct {
	visible bool
	result  db.Result
	driver  db.Driver
	scroll  int
	cursor  int
	width   int
	height  int
}

// IsVisible reports whether the panel is shown.
func (e ExplainPanel) IsVisible() bool {
	return e.visible
}

// Show populates the panel with an EXPLAIN result and makes it visible.
func (e *ExplainPanel) Show(result db.Result, driver db.Driver) {
	e.visible = true
	e.result = result
	e.driver = driver
	e.scroll = 0
	e.cursor = 0
}

// Hide hides the panel.
func (e *ExplainPanel) Hide() {
	e.visible = false
}

// SetSize sets the content dimensions of the panel (excluding border).
func (e *ExplainPanel) SetSize(width, height int) {
	e.width = width
	e.height = height
}

// borderOverhead is the vertical cost of the rounded border (top + bottom).
const borderOverhead = 2

// contentHeight returns the number of content rows inside the border.
func (e ExplainPanel) contentHeight() int {
	h := e.height - borderOverhead
	if h < 1 {
		h = 1
	}
	return h
}

// Update handles keyboard input. Returns the updated panel and a command.
func (e ExplainPanel) Update(msg tea.KeyMsg) ExplainPanel {
	n := len(e.renderedLines())
	vh := e.contentHeight()
	switch msg.String() {
	case "j", "down":
		if e.cursor < n-1 {
			e.cursor++
			e.adjustScroll(vh)
		}
	case "k", "up":
		if e.cursor > 0 {
			e.cursor--
			e.adjustScroll(vh)
		}
	case "g":
		e.cursor = 0
		e.scroll = 0
	case "G":
		e.cursor = n - 1
		e.adjustScroll(vh)
	case "ctrl+d":
		e.cursor += vh / 2
		if e.cursor >= n {
			e.cursor = n - 1
		}
		e.adjustScroll(vh)
	case "ctrl+u":
		e.cursor -= vh / 2
		if e.cursor < 0 {
			e.cursor = 0
		}
		e.adjustScroll(vh)
	}
	return e
}

func (e *ExplainPanel) adjustScroll(vh int) {
	if e.cursor < e.scroll {
		e.scroll = e.cursor
	}
	if e.cursor >= e.scroll+vh {
		e.scroll = e.cursor - vh + 1
	}
}

// View renders the panel with a border.
func (e ExplainPanel) View() string {
	lines := e.renderedLines()

	// Content width = total width minus border (2) minus horizontal padding (2).
	contentW := e.width - borderOverhead - 2
	if contentW < 1 {
		contentW = 1
	}

	vh := e.contentHeight()

	var visible []string
	end := e.scroll + vh
	if end > len(lines) {
		end = len(lines)
	}
	if e.scroll > len(lines) {
		e.scroll = 0
	}
	if e.scroll < 0 {
		e.scroll = 0
	}
	for i := e.scroll; i < end; i++ {
		visible = append(visible, lines[i])
	}
	for len(visible) < vh {
		visible = append(visible, "")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, visible...)

	return lipgloss.NewStyle().
		Width(e.width).
		Height(e.height).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)
}

// renderedLines converts the EXPLAIN result into display lines appropriate for
// the driver.
func (e ExplainPanel) renderedLines() []string {
	if len(e.result.Rows) == 0 {
		return []string{mutedStyle.Render("(no plan output)")}
	}

	if e.driver == db.DriverSQLite {
		return renderSQLitePlan(e.result)
	}
	if e.driver == db.DriverPostgres {
		return renderPostgresPlan(e.result)
	}
	// MySQL (and default): render as a table.
	return renderMySQLPlan(e.result)
}

// renderSQLitePlan builds an indented tree from SQLite's EXPLAIN QUERY PLAN
// output. SQLite returns columns: id, parent, notused, detail. The id/parent
// columns encode the tree nesting.
func renderSQLitePlan(result db.Result) []string {
	if len(result.Columns) < 4 {
		return renderGenericPlan(result)
	}

	type node struct {
		id     int
		parent int
		detail string
		depth  int
	}
	var nodes []node
	for _, row := range result.Rows {
		if len(row) < 4 {
			continue
		}
		id, _ := strconv.Atoi(row[0])
		parent, _ := strconv.Atoi(row[1])
		nodes = append(nodes, node{
			id:     id,
			parent: parent,
			detail: row[3],
		})
	}

	// Compute depth: walk parent chain.
	idToIdx := make(map[int]int)
	for i, n := range nodes {
		idToIdx[n.id] = i
	}
	for i := range nodes {
		depth := 0
		p := nodes[i].parent
		for p != 0 {
			depth++
			if idx, ok := idToIdx[p]; ok {
				p = nodes[idx].parent
			} else {
				break
			}
			if depth > 20 {
				break
			}
		}
		nodes[i].depth = depth
	}

	var lines []string
	for _, n := range nodes {
		indent := strings.Repeat("  ", n.depth)
		lines = append(lines, indent+n.detail)
	}
	return lines
}

// renderPostgresPlan shows PostgreSQL's EXPLAIN output verbatim. PG returns a
// single column ("QUERY PLAN") with one row per line of the plan text.
func renderPostgresPlan(result db.Result) []string {
	var lines []string
	for _, row := range result.Rows {
		if len(row) > 0 {
			lines = append(lines, row[0])
		}
	}
	return lines
}

// renderMySQLPlan renders MySQL's EXPLAIN output as a compact table. MySQL
// returns many columns (id, select_type, table, type, possible_keys, key,
// rows, Extra, ...) — we show the most useful ones.
func renderMySQLPlan(result db.Result) []string {
	// Find the column indices we care about.
	colIdx := make(map[string]int)
	for i, c := range result.Columns {
		colIdx[strings.ToLower(c.Name)] = i
	}

	// Pick the most useful columns to display.
	displayCols := []string{"id", "select_type", "table", "type", "key", "rows", "extra"}
	var indices []int
	var headers []string
	for _, name := range displayCols {
		if idx, ok := colIdx[name]; ok {
			indices = append(indices, idx)
			headers = append(headers, strings.ToUpper(name))
		}
	}
	if len(indices) == 0 {
		return renderGenericPlan(result)
	}

	// Compute column widths across headers and all data rows.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range result.Rows {
		for i, idx := range indices {
			w := len(row[idx])
			if idx < len(row) && w > widths[i] {
				widths[i] = w
			}
		}
	}

	sepChar := lipgloss.NewStyle().Foreground(colorBorder).Render("│")
	headerStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)

	// Render header cells, each padded to its column width.
	headerCells := make([]string, len(headers))
	for i, h := range headers {
		headerCells[i] = headerStyle.Render(padRight(h, widths[i]))
	}
	headerLine := strings.Join(headerCells, " "+sepChar+" ")

	// Separator line matching the total display width.
	totalW := 0
	for i, c := range headerCells {
		totalW += lipgloss.Width(c)
		if i > 0 {
			totalW += 3 // " │ "
		}
	}
	separator := lipgloss.NewStyle().Foreground(colorBorder).
		Render(strings.Repeat("─", totalW))

	lines := []string{headerLine, separator}
	for _, row := range result.Rows {
		cells := make([]string, len(indices))
		for i, idx := range indices {
			val := ""
			if idx < len(row) {
				val = row[idx]
			}
			cells[i] = padRight(val, widths[i])
		}
		lines = append(lines, strings.Join(cells, " "+sepChar+" "))
	}
	return lines
}

// renderGenericPlan is a fallback that shows all columns/values.
func renderGenericPlan(result db.Result) []string {
	var headers []string
	for _, c := range result.Columns {
		headers = append(headers, c.Name)
	}

	// Compute column widths.
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

	sepChar := lipgloss.NewStyle().Foreground(colorBorder).Render("│")
	headerStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)

	headerCells := make([]string, len(headers))
	for i, h := range headers {
		headerCells[i] = headerStyle.Render(padRight(h, widths[i]))
	}
	lines := []string{strings.Join(headerCells, " "+sepChar+" ")}
	for _, row := range result.Rows {
		cells := make([]string, len(headers))
		for i := range headers {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			cells[i] = padRight(val, widths[i])
		}
		lines = append(lines, strings.Join(cells, " "+sepChar+" "))
	}
	return lines
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
