package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// SearchResult represents a single matching cell found during cross-table search.
type SearchResult struct {
	Table  string
	Column string
	Value  string
	Row    []string // full row values for context
}

// CrossSearchPanel renders a scrollable list of cross-table search results.
type CrossSearchPanel struct {
	visible   bool
	searching bool
	query     string
	results   []SearchResult
	cursor    int
	scrollRow int
	width     int
	height    int
	searched  int // tables searched so far
	total     int // total tables to search
	done      bool
}

// NewCrossSearchPanel creates a new cross-search panel.
func NewCrossSearchPanel() CrossSearchPanel {
	return CrossSearchPanel{}
}

// IsVisible returns whether the panel is currently shown.
func (c CrossSearchPanel) IsVisible() bool { return c.visible }

// IsSearching returns whether a search is in progress.
func (c CrossSearchPanel) IsSearching() bool { return c.searching }

// Show opens the panel in input mode (not yet searching).
func (c *CrossSearchPanel) Show() {
	c.visible = true
	c.searching = false
	c.query = ""
	c.results = nil
	c.cursor = 0
	c.scrollRow = 0
	c.searched = 0
	c.total = 0
	c.done = false
}

// SetQuery replaces the search query (used by :grep to prefill).
func (c *CrossSearchPanel) SetQuery(q string) {
	c.query = q
}

// Hide closes the panel.
func (c *CrossSearchPanel) Hide() {
	c.visible = false
	c.searching = false
}

// Query returns the current search query.
func (c CrossSearchPanel) Query() string { return c.query }

// AddQueryChar appends a character to the search query.
func (c *CrossSearchPanel) AddQueryChar(ch string) {
	c.query += ch
}

// Backspace removes the last character from the search query.
func (c *CrossSearchPanel) Backspace() {
	if len(c.query) > 0 {
		c.query = c.query[:len(c.query)-1]
	}
}

// StartSearch begins the search process, resetting results.
func (c *CrossSearchPanel) StartSearch(totalTables int) {
	c.results = nil
	c.cursor = 0
	c.scrollRow = 0
	c.searched = 0
	c.total = totalTables
	c.done = false
	c.searching = true
}

// AddResults appends search results from a table and marks progress.
func (c *CrossSearchPanel) AddResults(results []SearchResult, tableCount int) {
	c.results = append(c.results, results...)
	c.searched += tableCount
}

// FinishSearch marks the search as complete.
func (c *CrossSearchPanel) FinishSearch() {
	c.searching = false
	c.done = true
}

// SelectedResult returns the result at the cursor, or nil if none.
func (c CrossSearchPanel) SelectedResult() *SearchResult {
	if len(c.results) == 0 || c.cursor < 0 || c.cursor >= len(c.results) {
		return nil
	}
	return &c.results[c.cursor]
}

// SetSize sets the dimensions of the panel and re-clamps scroll so j/k uses
// the real viewport. Must be called from Update/layout — View is a value
// receiver and cannot persist size.
func (c *CrossSearchPanel) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.adjustScroll()
}

// CursorUp moves the selection up.
func (c *CrossSearchPanel) CursorUp() {
	if c.cursor > 0 {
		c.cursor--
		c.adjustScroll()
	}
}

// CursorDown moves the selection down.
func (c *CrossSearchPanel) CursorDown() {
	if c.cursor < len(c.results)-1 {
		c.cursor++
		c.adjustScroll()
	}
}

func (c *CrossSearchPanel) adjustScroll() {
	maxVisible := c.maxVisible()
	if c.cursor < c.scrollRow {
		c.scrollRow = c.cursor
	}
	if c.cursor >= c.scrollRow+maxVisible {
		c.scrollRow = c.cursor - maxVisible + 1
	}
	if c.scrollRow < 0 {
		c.scrollRow = 0
	}
}

func (c CrossSearchPanel) maxVisible() int {
	mv := c.height - 4
	if mv < 1 {
		mv = 1
	}
	return mv
}

// View renders the cross-search panel.
func (c CrossSearchPanel) View() string {
	if !c.visible {
		return ""
	}

	prompt := renderPalettePrompt(c.query, true)

	// Status line: searching progress or result count.
	var status string
	switch {
	case c.searching:
		status = mutedStyle.Render(fmt.Sprintf(" searching %d/%d tables…  %d hits", c.searched, c.total, len(c.results)))
	case c.done:
		if len(c.results) == 0 {
			status = mutedStyle.Render(" no matches found")
		} else {
			status = mutedStyle.Render(fmt.Sprintf(" %d matches", len(c.results)))
		}
	}

	avail := c.maxVisible()

	maxVisible := avail
	end := c.scrollRow + maxVisible
	if end > len(c.results) {
		end = len(c.results)
	}

	// Inner content width matches the bordered panel's Width below.
	contentW := c.width - 2
	if contentW < 1 {
		contentW = 1
	}

	var rows []string
	for i := c.scrollRow; i < end; i++ {
		rows = append(rows, c.renderResultRow(c.results[i], i == c.cursor, contentW))
	}

	if len(c.results) == 0 && c.done {
		rows = append(rows, mutedStyle.Render("  (no matches)"))
	}
	if len(c.results) == 0 && c.searching {
		rows = append(rows, mutedStyle.Render("  (searching…)"))
	}

	content := strings.Join(rows, "\n")

	var sections []string
	sections = append(sections, " "+prompt)
	if status != "" {
		sections = append(sections, status)
	}
	if content != "" {
		sections = append(sections, content)
	}
	fullContent := strings.Join(sections, "\n")

	panel := lipgloss.NewStyle().
		Width(c.width - 2).
		Height(c.height - 2).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Render(fullContent)

	return panel
}

// renderResultRow draws one hit, truncating the value so the row spans the
// full inner panel width (no style padding — that was leaving a right gutter).
func (c CrossSearchPanel) renderResultRow(r SearchResult, selected bool, contentW int) string {
	tableLabel := fmt.Sprintf("%s.%s", r.Table, r.Column)
	val := sanitizeCellValue(r.Value)

	const prefixW = 2 // "❯ " / "  "
	const gapW = 2    // "  " between label and value
	labelW := lipgloss.Width(tableLabel)
	valBudget := contentW - prefixW - labelW - gapW
	if valBudget < 1 {
		labelBudget := contentW - prefixW - gapW - 1
		if labelBudget < 1 {
			labelBudget = 1
		}
		tableLabel = ansi.Truncate(tableLabel, labelBudget, "…")
		labelW = lipgloss.Width(tableLabel)
		valBudget = contentW - prefixW - labelW - gapW
		if valBudget < 1 {
			valBudget = 1
		}
	}
	valDisplay := ansi.Truncate(val, valBudget, "…")

	if selected {
		line := fmt.Sprintf("❯ %s  %s", tableLabel, valDisplay)
		if pad := contentW - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		return lipgloss.NewStyle().
			Foreground(colorBg).
			Background(colorPrimary).
			Render(line)
	}
	styledTable := lipgloss.NewStyle().Foreground(colorAccent).Render(tableLabel)
	line := fmt.Sprintf("  %s  %s", styledTable, valDisplay)
	if pad := contentW - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return lipgloss.NewStyle().Foreground(colorFg).Render(line)
}
