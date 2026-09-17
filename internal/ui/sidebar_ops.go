package ui

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rsiota/creel/internal/db"
)

// sidebarItem is a flat entry in the sidebar (schema header, table, or column).
type sidebarItem struct {
	text     string
	isColumn bool
	isSchema bool   // collapsible schema section header (Postgres multi-schema)
	isView   bool   // true for views (badged in the sidebar)
	schema   string // owning schema for table rows when grouped; empty in flat mode
	colType  string
	matchIdx []int // rune indices of fuzzy-matched chars (for highlighting)
}

// isTableRow reports a browsable table/view row (not a schema header or column).
func (it sidebarItem) isTableRow() bool {
	return !it.isColumn && !it.isSchema
}

// sidebarItems builds the flat list of tables + expanded columns. On Postgres
// with a populated schemaTableCache and 2+ schemas, inserts collapsible schema
// headers (active first). Filtering and single-schema connections stay flat.
func (m Model) sidebarItems() []sidebarItem {
	if m.sidebarFiltering {
		return m.filteredTables()
	}
	if m.useGroupedSidebar() {
		return m.groupedSidebarItems()
	}
	var items []sidebarItem
	for _, t := range m.tables {
		items = append(items, sidebarItem{text: t, isView: m.views[t]})
		if cols, ok := m.expanded[t]; ok {
			for _, c := range cols {
				items = append(items, sidebarItem{text: c.Name, isColumn: true, colType: c.Type})
			}
		}
	}
	return items
}

// useGroupedSidebar is true when the Postgres sidebar should show schema
// section headers. MySQL keeps a flat list (schemas are databases → :db).
func (m Model) useGroupedSidebar() bool {
	if m.connection == nil || m.connection.Config().Driver != db.DriverPostgres {
		return false
	}
	if len(m.schemaNames) < 2 || len(m.schemaTableCache) == 0 {
		return false
	}
	return true
}

// groupedSidebarItems builds schema headers + tables from schemaTableCache.
// The active schema is listed first and marked; other schemas start collapsed.
func (m Model) groupedSidebarItems() []sidebarItem {
	active := m.currentSchemaName()
	var items []sidebarItem
	for _, schema := range m.sidebarSchemaOrder() {
		items = append(items, sidebarItem{text: schema, isSchema: true})
		if !m.isSchemaSectionExpanded(schema) {
			continue
		}
		tables := m.tablesForSchemaSection(schema, active)
		for _, t := range tables {
			it := sidebarItem{text: t, schema: schema, isView: schema == active && m.views[t]}
			items = append(items, it)
			// Column expand only for the active schema (bare-name expand map).
			if schema == active {
				if cols, ok := m.expanded[t]; ok {
					for _, c := range cols {
						items = append(items, sidebarItem{text: c.Name, isColumn: true, colType: c.Type, schema: schema})
					}
				}
			}
		}
	}
	return items
}

// sidebarSchemaOrder returns schemas for the grouped sidebar: active first,
// then the rest of schemaNames that appear in the cache (or are active).
func (m Model) sidebarSchemaOrder() []string {
	active := m.currentSchemaName()
	seen := make(map[string]bool, len(m.schemaNames)+1)
	var order []string
	if active != "" {
		order = append(order, active)
		seen[active] = true
	}
	for _, s := range m.schemaNames {
		if seen[s] {
			continue
		}
		if _, ok := m.schemaTableCache[s]; !ok && s != active {
			continue
		}
		order = append(order, s)
		seen[s] = true
	}
	return order
}

// tablesForSchemaSection lists tables under a schema header. The active schema
// prefers the live m.tables list so expand/views stay in sync before the cache
// refreshes; other schemas read schemaTableCache.
func (m Model) tablesForSchemaSection(schema, active string) []string {
	if schema == active && len(m.tables) > 0 {
		return m.tables
	}
	return m.schemaTableCache[schema]
}

// isSchemaSectionExpanded reports whether a schema header's tables are shown.
// Default: only the active schema is expanded.
func (m Model) isSchemaSectionExpanded(schema string) bool {
	if m.sidebarSchemaExpanded != nil {
		if v, ok := m.sidebarSchemaExpanded[schema]; ok {
			return v
		}
	}
	return schema == m.currentSchemaName()
}

// toggleSchemaSection flips the expand/collapse state of a schema header.
func (m *Model) toggleSchemaSection(schema string) {
	if schema == "" {
		return
	}
	if m.sidebarSchemaExpanded == nil {
		m.sidebarSchemaExpanded = make(map[string]bool)
	}
	m.sidebarSchemaExpanded[schema] = !m.isSchemaSectionExpanded(schema)
}

// filteredTables returns tables matching the fuzzy filter, best match first.
func (m Model) filteredTables() []sidebarItem {
	if m.sidebarFilter == "" {
		items := make([]sidebarItem, len(m.tables))
		for i, t := range m.tables {
			items[i] = sidebarItem{text: t, isView: m.views[t]}
		}
		return items
	}
	ranked := fuzzyRank(m.sidebarFilter, m.tables,
		func(t string) string { return t },
		func(a, b fuzzyResult[string]) bool { return a.Item < b.Item })
	items := make([]sidebarItem, len(ranked))
	for i, r := range ranked {
		items[i] = sidebarItem{text: r.Item, isView: m.views[r.Item], matchIdx: r.MatchIdx}
	}
	return items
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

// applySearch applies a regex search pattern to the current results page: sets
// the matcher, jumps the cursor to the first match, and reports the match count
// via searchMsg. An empty pattern clears the matcher. Shared by the g/ search
// mode's enter handler and :regex, so the two entry points stay in sync.
func (m *Model) applySearch(pattern string) {
	m.lastSearch = pattern
	if pattern == "" {
		m.results.SetSearchMatcher(nil)
		return
	}
	match := compileSearchPattern(pattern)
	m.results.SetSearchMatcher(match)
	if row, col := findNextMatch(m.results, match, true); row >= 0 {
		m.results.SetCursor(row, col)
		n := countMatches(m.results, match)
		m.searchMsg = fmt.Sprintf("%d match%s (this page)", n, pluralIf(n != 1, "es"))
	} else {
		m.searchMsg = "no matches on this page"
	}
}

// highlightMatches renders text with matched characters in the accent color
// and the rest in the theme foreground. Unmatched (and empty-query) runs must
// not fall through to the terminal's default FG: paintBg fills the theme
// background under every cell, so a light theme on a dark terminal would leave
// light-on-white text if we left those runes unstyled.
func highlightMatches(text string, matchIdx []int) string {
	base := lipgloss.NewStyle().Foreground(colorFg)
	if len(matchIdx) == 0 {
		return base.Render(text)
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
			b.WriteString(base.Render(string(r)))
		}
	}
	return b.String()
}

// syncSidebarCursorToTable moves the cursor to a table in the full sidebar list.
func (m *Model) syncSidebarCursorToTable(tableName string) {
	items := m.sidebarItems()
	active := m.currentSchemaName()
	for i, item := range items {
		if !item.isTableRow() || item.text != tableName {
			continue
		}
		// Prefer the active-schema row when the same name exists elsewhere.
		if item.schema == "" || item.schema == active {
			m.sidebarCursor = i
			m.sidebarViewAnchored = false
			return
		}
	}
	// Fall back to the first matching table row in any schema.
	for i, item := range items {
		if item.isTableRow() && item.text == tableName {
			m.sidebarCursor = i
			m.sidebarViewAnchored = false
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
	m.sidebarViewAnchored = false // keyboard nav re-centers
	return m
}

// sidebarMaxVisible returns the number of sidebar item lines the current
// terminal can show. It mirrors the renderer's derivation: the sidebar content
// area is the workspace height (m.height minus the status bar) minus the
// sidebar's top/bottom borders, with one line reserved for the scroll-info
// footer. (rightPanel height always equals m.height-1, so this matches the
// renderer's lipgloss.Height(rightPanel)-borderOverhead exactly.)
func (m Model) sidebarMaxVisible() int {
	const statusHeight, borderOverhead, footer = 1, 2, 1
	content := m.height - statusHeight - borderOverhead
	if content < 3 {
		content = 3
	}
	v := content - footer
	if v < 1 {
		v = 1
	}
	return v
}

// sidebarScrollOffset returns the index of the first visible sidebar item using
// the same cursor-centered windowing as the renderer. Both the renderer and the
// mouse handler call this so a click always maps to the item that was actually
// drawn. The offset is recomputed rather than read from the cached sidebarScroll
// field because that field is assigned inside the value-receiver View, where the
// write is discarded and never reaches the model the mouse handler sees.
func (m Model) sidebarScrollOffset() int {
	items := m.sidebarItems()
	maxVisible := m.sidebarMaxVisible()
	half := maxVisible / 2
	start := m.sidebarCursor - half
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(items) {
		end = len(items)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}
	return start
}

// sidebarStickyStart returns a scroll offset that keeps the cursor visible with
// the smallest possible movement, preferring to leave prevStart unchanged. This
// is the "don't jump" policy used after a mouse click: the clicked item is
// already on screen, so the offset does not move.
func sidebarStickyStart(cursor, prevStart, maxVisible, numItems int) int {
	if numItems <= maxVisible {
		return 0
	}
	start := prevStart
	if start < 0 {
		start = 0
	}
	if cursor < start {
		start = cursor
	}
	if cursor >= start+maxVisible {
		start = cursor - maxVisible + 1
	}
	if maxStart := numItems - maxVisible; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return start
}

// sidebarRenderedStart returns the scroll offset the renderer is currently using
// for the sidebar: the frozen offset when the view is anchored (after a mouse
// click), otherwise the cursor-centered offset (keyboard navigation). The
// renderer and the mouse handler both call this so a click always maps to the
// item that was drawn, and a click freezes that exact view.
func (m Model) sidebarRenderedStart() int {
	items := m.sidebarItems()
	maxVisible := m.sidebarMaxVisible()
	if m.sidebarViewAnchored {
		return sidebarStickyStart(m.sidebarCursor, m.sidebarScroll, maxVisible, len(items))
	}
	return m.sidebarScrollOffset()
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
// whether it points at the table row or one of its expanded columns. Tables
// under a non-active schema are ignored (phase 1: structure/open stay on the
// active search_path schema).
func (m Model) sidebarSelectedTable() string {
	items := m.sidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return ""
	}
	active := m.currentSchemaName()
	for i := m.sidebarCursor; i >= 0; i-- {
		if items[i].isTableRow() {
			if items[i].schema != "" && items[i].schema != active {
				return ""
			}
			return items[i].text
		}
		if items[i].isSchema {
			return ""
		}
	}
	return ""
}

// sidebarActivateItem handles Enter / click on a sidebar row: schema headers
// toggle; active-schema tables open; other-schema tables prompt :schema.
func (m *Model) sidebarActivateItem(item *sidebarItem) tea.Cmd {
	if item == nil || item.isColumn {
		return nil
	}
	if item.isSchema {
		m.toggleSchemaSection(item.text)
		return nil
	}
	active := m.currentSchemaName()
	if item.schema != "" && item.schema != active {
		m.schemaMsg = fmt.Sprintf("use :schema %s to switch", item.schema)
		return nil
	}
	return m.openTable(item.text)
}

// toggleExpand loads or clears the schema for the selected table, or toggles
// a schema section header.
func (m *Model) toggleExpand() {
	item := m.currentSidebarItem()
	if item == nil || item.isColumn {
		return
	}
	if item.isSchema {
		m.toggleSchemaSection(item.text)
		return
	}
	active := m.currentSchemaName()
	if item.schema != "" && item.schema != active {
		m.schemaMsg = fmt.Sprintf("use :schema %s to switch", item.schema)
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
