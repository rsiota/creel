package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConnectionEntry holds display data for a saved connection.
type ConnectionEntry struct {
	Name   string
	Driver string
	Detail string
	Group  string
}

// connectionItem is a single connection in the list.
type connectionItem struct {
	name     string
	driver   string
	detail   string
	group    string
	matchIdx []int // fuzzy match indices for highlighting
}

// connRowKind distinguishes group-header rows from connection rows in the
// rendered list.
type connRowKind int

const (
	rowConn connRowKind = iota
	rowGroup
)

// connRow is a single renderable row: either a collapsible group header or a
// connection field box. Unifying them into one sequence lets cursor
// navigation, line-based scroll, mouse mapping, and filtering all operate on a
// single list.
type connRow struct {
	kind  connRowKind
	group string         // header label group; or the owning group for a conn
	conn  connectionItem // valid when kind == rowConn
}

// ConnectionList is a custom component for selecting a saved connection.
type ConnectionList struct {
	items  []connectionItem
	cursor int
	scroll int // line-based: lines scrolled off the top
	width  int
	height int

	// Collapsed groups (by name). Ungrouped uses the "" key. Preserved across
	// SetItems reloads so adding/editing a connection doesn't reset folding.
	collapsed map[string]bool

	// Fuzzy filter
	filter    string
	filtering bool
}

// NewConnectionList creates a new connection list component.
func NewConnectionList() ConnectionList {
	return ConnectionList{}
}

// SetItems populates the list from connection entries. Collapse state is
// preserved across reloads.
func (c *ConnectionList) SetItems(conns []ConnectionEntry) {
	c.items = make([]connectionItem, len(conns))
	for i, conn := range conns {
		c.items[i] = connectionItem{
			name:   conn.Name,
			driver: conn.Driver,
			detail: conn.Detail,
			group:  conn.Group,
		}
	}
	if c.cursor >= len(c.items) {
		c.cursor = len(c.items) - 1
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
}

// hasGroups reports whether any connection belongs to a named group. When
// false the list renders flat (no headers), preserving the pre-groups look.
func (c ConnectionList) hasGroups() bool {
	for _, it := range c.items {
		if it.group != "" {
			return true
		}
	}
	return false
}

// groupSections partitions items into the ungrouped slice (config order) and
// named buckets (config order within each), returning group names in
// alphabetical order. Shared by the grouped renderer and ExpandedHeight.
func (c ConnectionList) groupSections() (ungrouped []connectionItem, order []string, buckets map[string][]connectionItem) {
	buckets = map[string][]connectionItem{}
	for _, it := range c.items {
		if it.group == "" {
			ungrouped = append(ungrouped, it)
			continue
		}
		if _, ok := buckets[it.group]; !ok {
			order = append(order, it.group)
		}
		buckets[it.group] = append(buckets[it.group], it)
	}
	sort.Strings(order)
	return ungrouped, order, buckets
}

// rows returns the renderable row sequence for the current state:
//   - filtering: a flat, fuzzy-ranked list of matching connections (no headers);
//   - no groups: a flat list of all connections (backward-compatible);
//   - otherwise: group headers + their connections, with collapsed groups
//     showing only their header. Ungrouped connections lead under an
//     "Ungrouped" header, then named groups alphabetically.
func (c ConnectionList) rows() []connRow {
	if c.filtering && c.filter != "" {
		return c.filteredRows()
	}
	if !c.hasGroups() {
		out := make([]connRow, len(c.items))
		for i, it := range c.items {
			out[i] = connRow{kind: rowConn, conn: it}
		}
		return out
	}
	return c.groupedRows()
}

// filteredRows ranks all connections by the fuzzy filter and returns them as
// flat connection rows (no group headers), with match indices for highlighting.
func (c ConnectionList) filteredRows() []connRow {
	ranked := fuzzyRank(c.filter, c.items,
		func(it connectionItem) string { return it.name },
		func(a, b fuzzyResult[connectionItem]) bool { return a.Item.name < b.Item.name })
	out := make([]connRow, len(ranked))
	for i, r := range ranked {
		cp := r.Item
		cp.matchIdx = r.MatchIdx
		out[i] = connRow{kind: rowConn, conn: cp}
	}
	return out
}

// groupedRows builds the grouped layout described in rows().
func (c ConnectionList) groupedRows() []connRow {
	ungrouped, order, buckets := c.groupSections()
	var out []connRow
	if len(ungrouped) > 0 {
		out = append(out, connRow{kind: rowGroup, group: ""})
		if !c.collapsed[""] {
			for _, it := range ungrouped {
				out = append(out, connRow{kind: rowConn, conn: it, group: ""})
			}
		}
	}
	for _, g := range order {
		out = append(out, connRow{kind: rowGroup, group: g})
		if c.collapsed[g] {
			continue
		}
		for _, it := range buckets[g] {
			out = append(out, connRow{kind: rowConn, conn: it, group: g})
		}
	}
	return out
}

// rowHeight returns the rendered height of a row: group headers are a single
// line, connection boxes are linesPerField lines.
func rowHeight(r connRow) int {
	if r.kind == rowGroup {
		return 1
	}
	return linesPerField
}

// ExpandedHeight is the total rendered height with every group expanded. Used
// to size the popup so its height stays stable regardless of filtering or
// folding (collapsed/filtered states simply leave breathing room).
func (c ConnectionList) ExpandedHeight() int {
	h := len(c.items) * linesPerField
	if !c.hasGroups() {
		return h
	}
	ungrouped, order, _ := c.groupSections()
	headers := len(order)
	if len(ungrouped) > 0 {
		headers++
	}
	return h + headers
}

// TotalCount returns the total number of connections, ignoring any active
// filter or grouping. Kept for callers that want the raw count.
func (c ConnectionList) TotalCount() int {
	return len(c.items)
}

// visibleItems returns the connection items (flattened) in the current row
// sequence — ranked matches while filtering, otherwise the grouped/flat view.
func (c ConnectionList) visibleItems() []connectionItem {
	rows := c.rows()
	out := make([]connectionItem, 0, len(rows))
	for _, r := range rows {
		if r.kind == rowConn {
			out = append(out, r.conn)
		}
	}
	return out
}

// VisibleItemsForMouse returns the currently visible (possibly filtered)
// connection items. Exported for mouse-click coordinate mapping helpers.
func (c ConnectionList) VisibleItemsForMouse() []connectionItem {
	return c.visibleItems()
}

// prefixTops computes the cumulative top line offset of each row (tops[i] is
// the line where row i begins; tops[len] is the total height).
func prefixTops(rows []connRow) []int {
	tops := make([]int, len(rows)+1)
	for i, r := range rows {
		tops[i+1] = tops[i] + rowHeight(r)
	}
	return tops
}

// SelectedName returns the name of the connection under the cursor, or "" if
// the cursor rests on a group header (which is not connectable).
func (c ConnectionList) SelectedName() string {
	rows := c.rows()
	if c.cursor < 0 || c.cursor >= len(rows) {
		return ""
	}
	r := rows[c.cursor]
	if r.kind != rowConn {
		return ""
	}
	return r.conn.name
}

// SelectedDriver returns the driver of the connection under the cursor.
func (c ConnectionList) SelectedDriver() string {
	rows := c.rows()
	if c.cursor < 0 || c.cursor >= len(rows) {
		return ""
	}
	r := rows[c.cursor]
	if r.kind != rowConn {
		return ""
	}
	return r.conn.driver
}

// Cursor returns the current cursor index (into the row sequence).
func (c ConnectionList) Cursor() int {
	return c.cursor
}

// SetCursor sets the cursor position, clamped to the row sequence.
func (c *ConnectionList) SetCursor(i int) {
	rows := c.rows()
	if len(rows) == 0 {
		c.cursor = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(rows) {
		i = len(rows) - 1
	}
	c.cursor = i
	c.ensureVisible(rows)
}

// MoveCursor moves the cursor by delta rows.
func (c *ConnectionList) MoveCursor(delta int) {
	c.SetCursor(c.cursor + delta)
}

// firstConnRow / lastConnRow return the cursor index of the first/last
// connection row (skipping group headers), so g/G land on a connectable entry.
func (c ConnectionList) firstConnRow() int {
	rows := c.rows()
	for i, r := range rows {
		if r.kind == rowConn {
			return i
		}
	}
	return 0
}

func (c ConnectionList) lastConnRow() int {
	rows := c.rows()
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].kind == rowConn {
			return i
		}
	}
	return 0
}

// CursorOnGroupHeader reports whether the cursor rests on a group header.
func (c ConnectionList) CursorOnGroupHeader() bool {
	rows := c.rows()
	return c.cursor >= 0 && c.cursor < len(rows) && rows[c.cursor].kind == rowGroup
}

// groupHeaderIndex returns the row index of a group's header, or -1.
func groupHeaderIndex(rows []connRow, group string) int {
	for i, r := range rows {
		if r.kind == rowGroup && r.group == group {
			return i
		}
	}
	return -1
}

// ToggleGroupAtCursor folds/unfolds the group of the row under the cursor
// (works whether the cursor is on that group's header or one of its
// connections). After collapsing a group whose connection held the cursor, the
// cursor relocates to that group's header so it never lands on a hidden row.
func (c *ConnectionList) ToggleGroupAtCursor() {
	rows := c.rows()
	if c.cursor < 0 || c.cursor >= len(rows) {
		return
	}
	g := rows[c.cursor].group
	wasCollapsed := c.collapsed[g]
	if c.collapsed == nil {
		c.collapsed = map[string]bool{}
	}
	c.collapsed[g] = !wasCollapsed
	rows = c.rows()
	if !wasCollapsed {
		// Just collapsed: if the cursor was on a now-hidden connection, move
		// it to the group's (still-visible) header.
		if hdr := groupHeaderIndex(rows, g); hdr >= 0 {
			c.cursor = hdr
		}
	}
	if c.cursor >= len(rows) {
		c.cursor = len(rows) - 1
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
	c.ensureVisible(rows)
}

// ensureVisible adjusts the line-based scroll so the cursor row is fully
// visible, keeping the scroll snapped to a row boundary. Among the valid
// boundaries it picks the smallest (most content above the cursor), so the
// cursor sits near the bottom of the viewport rather than jumping to the top.
func (c *ConnectionList) ensureVisible(rows []connRow) {
	if len(rows) == 0 || c.height <= 0 {
		c.scroll = 0
		return
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
	if c.cursor >= len(rows) {
		c.cursor = len(rows) - 1
	}
	tops := prefixTops(rows)
	cursorTop := tops[c.cursor]
	cursorBot := tops[c.cursor+1]
	lo := cursorBot - c.height // smallest scroll that still fits the cursor's bottom
	hi := cursorTop            // largest scroll that keeps the cursor's top visible
	best := -1
	for _, t := range tops {
		if t > hi {
			break
		}
		if t >= lo {
			best = t
			break
		}
	}
	if best < 0 {
		best = cursorTop
	}
	c.scroll = best
}

func (c ConnectionList) maxVisibleItems() int {
	// Each entry renders as a field box (linesPerField lines). This is an
	// approximation (group headers are shorter) used only for sizing heuristics.
	max := c.height / linesPerField
	if max < 1 {
		return 1
	}
	return max
}

// IsFiltering returns whether the filter input is active.
func (c ConnectionList) IsFiltering() bool {
	return c.filtering
}

// StartFilter enters filter mode.
func (c *ConnectionList) StartFilter() {
	c.filtering = true
	c.filter = ""
	c.cursor = 0
	c.scroll = 0
}

// CancelFilter exits filter mode without selecting.
func (c *ConnectionList) CancelFilter() {
	c.filtering = false
	c.filter = ""
	c.cursor = 0
	c.scroll = 0
}

// CommitFilter exits filter mode, relocating the cursor to the selected match
// within the (grouped) layout so it stays on the same connection.
func (c *ConnectionList) CommitFilter() {
	name := c.SelectedName()
	c.filtering = false
	c.filter = ""
	rows := c.rows()
	if name != "" {
		for i, r := range rows {
			if r.kind == rowConn && r.conn.name == name {
				c.cursor = i
				c.ensureVisible(rows)
				return
			}
		}
	}
	c.cursor = 0
	c.ensureVisible(rows)
}

// FilterAddChar appends a character to the filter.
func (c *ConnectionList) FilterAddChar(ch string) {
	c.filter += ch
	c.cursor = 0
	c.scroll = 0
}

// FilterBackspace removes the last character from the filter.
func (c *ConnectionList) FilterBackspace() {
	if len(c.filter) > 0 {
		c.filter = c.filter[:len(c.filter)-1]
	}
	c.cursor = 0
	c.scroll = 0
}

// Update handles messages for the connection list (no-op, keys handled by app).
func (c ConnectionList) Update(msg tea.Msg) (ConnectionList, tea.Cmd) {
	return c, nil
}

// SetSize sets the dimensions of the connection list content area.
func (c *ConnectionList) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.ensureVisible(c.rows())
}

// View renders the connection list. Each connection is an inspector-style
// field box; when grouping is in use, collapsible group headers separate the
// boxes. Only fully visible rows are drawn (partial boxes at the viewport
// edges are skipped), and the scroll is snapped to row boundaries.
func (c ConnectionList) View() string {
	rows := c.rows()
	contentW := c.width
	valueWidth := contentW - 4
	if valueWidth < 1 {
		valueWidth = 1
	}

	if len(rows) == 0 {
		if c.filtering {
			return mutedStyle.Render("  (no matches)")
		}
		return mutedStyle.Render("  No saved connections. Press 'n' to add one.")
	}

	tops := prefixTops(rows)
	totalH := tops[len(rows)]
	maxScroll := totalH - c.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := c.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	var b strings.Builder
	for i, r := range rows {
		top := tops[i]
		bot := tops[i+1]
		if bot <= scroll {
			continue // above the viewport
		}
		if top >= scroll+c.height {
			break // below the viewport
		}
		if bot > scroll+c.height {
			break // would overflow — stop (cursor is kept visible by ensureVisible)
		}
		b.WriteString(c.renderRow(r, i, contentW, valueWidth))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderRow renders a single row (group header or connection field box).
func (c ConnectionList) renderRow(r connRow, idx, contentW, valueWidth int) string {
	isCursor := idx == c.cursor
	if r.kind == rowGroup {
		return renderGroupHeader(r.group, c.collapsed[r.group], isCursor, contentW)
	}

	item := r.conn

	badgeSty := lipgloss.NewStyle().Foreground(colorAccent)
	nameBold := lipgloss.NewStyle().Foreground(colorFg).Bold(true)
	namePlain := lipgloss.NewStyle().Foreground(colorFg)
	detailSty := lipgloss.NewStyle().Foreground(colorMuted)
	detailCurSty := lipgloss.NewStyle().Foreground(colorLabel)

	// Label: the connection name (bold when cursor; fuzzy-highlighted when
	// filtering, in which case per-char styling replaces the bold).
	var labelStr string
	switch {
	case c.filtering:
		labelStr = highlightMatches(item.name, item.matchIdx)
	case isCursor:
		labelStr = nameBold.Render(item.name)
	default:
		labelStr = namePlain.Render(item.name)
	}

	// Marker: the driver badge.
	markerStr := badgeSty.Render("[" + strings.ToUpper(item.driver) + "]")

	// Value: host/db detail, brightened for the cursor entry.
	dSty := detailSty
	if isCursor {
		dSty = detailCurSty
	}
	valueContent := dSty.Render(truncateCell(item.detail, valueWidth))

	return renderFieldBox(labelStr, markerStr, valueContent, contentW, fieldBoxBorder(isCursor))
}

// renderGroupHeader renders a foldable section header: a triangle marker, the
// group name (or "Ungrouped"), and the connection count. The cursor header is
// rendered in the primary colour.
func renderGroupHeader(group string, collapsed, isCursor bool, contentW int) string {
	label := group
	if label == "" {
		label = "Ungrouped"
	}
	marker := "▾"
	if collapsed {
		marker = "▸"
	}
	sty := lipgloss.NewStyle().Foreground(colorLabel).Bold(true)
	if isCursor {
		sty = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	}
	return " " + sty.Render(marker+" "+label)
}

// YToRow maps a y-offset within the list content area to a row index,
// accounting for variable row heights and the current scroll. Returns -1 when
// the offset doesn't land on a row.
func (c ConnectionList) YToRow(y int) int {
	rows := c.rows()
	if len(rows) == 0 {
		return -1
	}
	tops := prefixTops(rows)
	totalH := tops[len(rows)]
	maxScroll := totalH - c.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := c.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	for i := range rows {
		top := tops[i] - scroll
		bot := tops[i+1] - scroll
		if y >= top && y < bot {
			return i
		}
	}
	return -1
}

// Prompt returns the filter prompt line shown at the top of the panel.
func (c ConnectionList) Prompt() string {
	return renderPalettePrompt(c.filter, c.filtering)
}

// ScrollInfo returns the bottom-bar text (scroll position / count).
func (c ConnectionList) ScrollInfo() string {
	rows := c.rows()
	conns := 0
	for _, r := range rows {
		if r.kind == rowConn {
			conns++
		}
	}
	if conns == 0 {
		return ""
	}
	tops := prefixTops(rows)
	totalH := tops[len(rows)]
	if totalH > c.height {
		scroll := c.scroll
		if scroll > totalH-c.height {
			scroll = totalH - c.height
		}
		if scroll < 0 {
			scroll = 0
		}
		// Count connections fully or partially within the viewport.
		firstVis, lastVis := 0, 0
		for i := range rows {
			if tops[i+1] > scroll {
				firstVis = i
				break
			}
		}
		for i := len(rows) - 1; i >= 0; i-- {
			if tops[i] < scroll+c.height {
				lastVis = i
				break
			}
		}
		visConns, before := 0, 0
		for i := 0; i <= lastVis; i++ {
			if rows[i].kind == rowConn {
				if i < firstVis {
					before++
				} else {
					visConns++
				}
			}
		}
		return mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", before+1, before+visConns, conns))
	}
	noun := "connections"
	if conns == 1 {
		noun = "connection"
	}
	return mutedStyle.Render(fmt.Sprintf(" %d %s", conns, noun))
}
