package ui

import (
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// sidebarItem is a flat entry in the sidebar (table or column).
type sidebarItem struct {
	text     string
	isColumn bool
	colType  string
	matchIdx []int // rune indices of fuzzy-matched chars (for highlighting)
}

// sidebarItems builds the flat list of tables + expanded columns.
func (m Model) sidebarItems() []sidebarItem {
	if m.sidebarFiltering {
		return m.filteredTables()
	}
	var items []sidebarItem
	for _, t := range m.tables {
		items = append(items, sidebarItem{text: t})
		if cols, ok := m.expanded[t]; ok {
			for _, c := range cols {
				items = append(items, sidebarItem{text: c.Name, isColumn: true, colType: c.Type})
			}
		}
	}
	return items
}

// filteredTables returns tables matching the fuzzy filter, best match first.
func (m Model) filteredTables() []sidebarItem {
	type scored struct {
		item  sidebarItem
		score int
	}
	var results []scored
	for _, t := range m.tables {
		idx, score := fuzzyMatch(m.sidebarFilter, t)
		if idx != nil || m.sidebarFilter == "" {
			results = append(results, scored{sidebarItem{text: t, matchIdx: idx}, score})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score < results[j].score
		}
		return results[i].item.text < results[j].item.text
	})
	items := make([]sidebarItem, len(results))
	for i, r := range results {
		items[i] = r.item
	}
	return items
}

// fuzzyMatch performs case-insensitive subsequence matching.
// Returns matched rune indices (nil if no match) and a score (lower = better).
func fuzzyMatch(query, s string) ([]int, int) {
	if query == "" {
		return nil, 0
	}
	q := []rune(strings.ToLower(query))
	target := []rune(strings.ToLower(s))

	var indices []int
	qi := 0
	for si := 0; si < len(target) && qi < len(q); si++ {
		if target[si] == q[qi] {
			indices = append(indices, si)
			qi++
		}
	}
	if qi < len(q) {
		return nil, 0
	}

	// Score: lower = better. Penalize gaps, reward consecutive/boundary matches.
	score := len(target) // mild preference for shorter names
	for i := 1; i < len(indices); i++ {
		gap := indices[i] - indices[i-1] - 1
		score += gap * 3
		if indices[i] == indices[i-1]+1 {
			score -= 5
		}
	}
	for _, idx := range indices {
		if idx == 0 || target[idx-1] == '_' || target[idx-1] == '.' {
			score -= 2
		}
	}
	return indices, score
}

// bestColumnMatch returns the index of the column whose name best matches
// the fuzzy query, or -1 if nothing matches.
func bestColumnMatch(cols []string, query string) int {
	if query == "" {
		return -1
	}
	bestIdx := -1
	bestScore := 0
	for i, c := range cols {
		_, score := fuzzyMatch(query, c)
		if score == 0 {
			continue
		}
		if bestIdx == -1 || score < bestScore {
			bestIdx = i
			bestScore = score
		}
	}
	return bestIdx
}

// compileSearchPattern compiles a user-typed search string as a regex, falling
// back to a literal substring match if the regex is invalid. The returned
// matcher function reports whether a cell value contains a match.
func compileSearchPattern(query string) func(string) bool {
	if query == "" {
		return func(string) bool { return false }
	}
	if re, err := regexp.Compile(query); err == nil {
		return func(s string) bool { return re.MatchString(s) }
	}
	// Literal fallback: case-sensitive substring match.
	q := query
	return func(s string) bool { return strings.Contains(s, q) }
}

// findNextMatch scans row-major from the cell after the cursor (inclusive if
// fromStart) and returns the first matching [row, col], or [-1,-1] if none.
func findNextMatch(r ResultsTable, match func(string) bool, fromStart bool) (int, int) {
	rows := r.NumRows()
	cols := r.NumCols()
	if rows == 0 || cols == 0 {
		return -1, -1
	}
	startRow, startCol := 0, 0
	if !fromStart {
		startRow = r.CursorRow()
		startCol = r.CursorCol() + 1
	}
	for row := startRow; row < rows; row++ {
		for col := 0; col < cols; col++ {
			if row == startRow && col < startCol {
				continue
			}
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				return row, col
			}
		}
	}
	// Wrap around.
	for row := 0; row < startRow; row++ {
		for col := 0; col < cols; col++ {
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				return row, col
			}
		}
	}
	if !fromStart {
		// Final partial pass on the start row up to the cursor.
		for col := 0; col < startCol; col++ {
			if match(r.RowValue(startRow, col)) {
				return startRow, col
			}
		}
	}
	return -1, -1
}

// findPrevMatch scans row-major backwards from the cell before the cursor and
// returns the nearest matching [row, col], or [-1,-1] if none. Wraps around.
func findPrevMatch(r ResultsTable, match func(string) bool) (int, int) {
	rows := r.NumRows()
	cols := r.NumCols()
	if rows == 0 || cols == 0 {
		return -1, -1
	}
	startRow := r.CursorRow()
	startCol := r.CursorCol() - 1
	// Scan backwards from cursor.
	for row := startRow; row >= 0; row-- {
		cEnd := cols - 1
		if row == startRow {
			cEnd = startCol
		}
		for col := cEnd; col >= 0; col-- {
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				return row, col
			}
		}
	}
	// Wrap around: scan from the last row back to the cursor row.
	for row := rows - 1; row > startRow; row-- {
		for col := cols - 1; col >= 0; col-- {
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				return row, col
			}
		}
	}
	// Final partial pass on the start row from the last col down to cursor.
	for col := cols - 1; col >= startCol+1; col-- {
		if match(r.RowValue(startRow, col)) {
			return startRow, col
		}
	}
	return -1, -1
}

// countMatches returns the total number of cells matching across all rows.
func countMatches(r ResultsTable, match func(string) bool) int {
	count := 0
	for row := 0; row < r.NumRows(); row++ {
		for col := 0; col < r.NumCols(); col++ {
			if !r.IsColumnHidden(col) && match(r.RowValue(row, col)) {
				count++
			}
		}
	}
	return count
}

// highlightMatches renders text with matched characters in the accent color.
func highlightMatches(text string, matchIdx []int) string {
	if len(matchIdx) == 0 {
		return text
	}
	matchSet := make(map[int]bool, len(matchIdx))
	for _, i := range matchIdx {
		matchSet[i] = true
	}
	accent := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	var b strings.Builder
	for i, r := range []rune(text) {
		if matchSet[i] {
			b.WriteString(accent.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// syncSidebarCursorToTable moves the cursor to a table in the full sidebar list.
func (m *Model) syncSidebarCursorToTable(tableName string) {
	items := m.sidebarItems()
	for i, item := range items {
		if !item.isColumn && item.text == tableName {
			m.sidebarCursor = i
			return
		}
	}
}

// scrollSidebar moves the cursor through the flat sidebar item list.
func (m Model) scrollSidebar(delta int) Model {
	items := m.sidebarItems()
	if len(items) == 0 {
		m.sidebarCursor = 0
		return m
	}
	m.sidebarCursor += delta
	if m.sidebarCursor < 0 {
		m.sidebarCursor = 0
	}
	if m.sidebarCursor > len(items)-1 {
		m.sidebarCursor = len(items) - 1
	}
	return m
}

// currentSidebarItem returns the item under the cursor.
func (m Model) currentSidebarItem() *sidebarItem {
	items := m.sidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return nil
	}
	return &items[m.sidebarCursor]
}

// sidebarSelectedTable returns the table for the current sidebar cursor,
// whether it points at the table row or one of its expanded columns.
func (m Model) sidebarSelectedTable() string {
	items := m.sidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return ""
	}
	for i := m.sidebarCursor; i >= 0; i-- {
		if !items[i].isColumn {
			return items[i].text
		}
	}
	return ""
}

// toggleExpand loads or clears the schema for the selected table.
func (m *Model) toggleExpand() {
	item := m.currentSidebarItem()
	if item == nil || item.isColumn {
		return
	}
	table := item.text
	if _, ok := m.expanded[table]; ok {
		delete(m.expanded, table)
		m.refreshCompletionCandidates()
		return
	}
	cols, err := m.connection.DB().TableSchema(table)
	if err != nil {
		m.connError = err.Error()
		return
	}
	m.expanded[table] = cols
	m.refreshCompletionCandidates()
}
