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
	Recent bool // true when this name is in the MRU list
}

// connectionItem is a single connection in the list.
type connectionItem struct {
	name     string
	driver   string
	detail   string
	group    string
	recent   bool
	demo     bool  // synthetic first-run "try the demo" row (not a saved connection)
	matchIdx []int // fuzzy match indices for highlighting
}

// connRowKind distinguishes connection rows in the list sequence.
type connRowKind int

const (
	rowConn connRowKind = iota
)

// connRow is a single renderable connection line. Cursor navigation, scroll,
// mouse mapping, and filtering all operate on this sequence.
type connRow struct {
	kind  connRowKind
	group string         // owning group key when groups are in use
	conn  connectionItem // valid when kind == rowConn
}

// ConnectionList is a custom component for selecting a saved connection.
type ConnectionList struct {
	items  []connectionItem
	cursor int
	scroll int // line-based: lines scrolled off the top
	width  int
	height int

	// groupTab is the active group key when connections use groups ("" =
	// Ungrouped). Named groups are selected by their config group string.
	groupTab string

	// Fuzzy filter
	filter    string
	filtering bool

	// padBg fills short viewport rows in View(); empty under transparent_background.
	padBg lipgloss.Color
}

// NewConnectionList creates a new connection list component.
func NewConnectionList() ConnectionList {
	return ConnectionList{}
}

// SetPadBackground sets the background colour for viewport padding rows. Pass
// an empty colour when transparent_background is enabled.
func (c *ConnectionList) SetPadBackground(bg lipgloss.Color) {
	c.padBg = bg
}

// SetItems populates the list from connection entries. An empty slice enables
// the synthetic demo row (see rows). The active group tab is clamped to a
// still-valid group when items change.
func (c *ConnectionList) SetItems(conns []ConnectionEntry) {
	c.items = make([]connectionItem, len(conns))
	for i, conn := range conns {
		c.items[i] = connectionItem{
			name:   conn.Name,
			driver: conn.Driver,
			detail: conn.Detail,
			group:  conn.Group,
			recent: conn.Recent,
		}
	}
	c.clampGroupTab()
	rows := c.rows()
	if c.cursor >= len(rows) {
		c.cursor = len(rows) - 1
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
}

// SelectByName moves the cursor onto the named connection, switching to its
// group tab when groups are in use. Returns false when the name is not found.
func (c *ConnectionList) SelectByName(name string) bool {
	if name == "" {
		return false
	}
	for _, it := range c.items {
		if it.name == name && !it.demo {
			if c.hasGroups() {
				c.setGroupTab(it.group)
			}
			rows := c.rows()
			for i, r := range rows {
				if r.kind == rowConn && r.conn.name == name {
					c.SetCursor(i)
					return true
				}
			}
			return false
		}
	}
	return false
}

// hasGroups reports whether any connection belongs to a named group. When
// false the list renders flat with no tab strip.
func (c ConnectionList) hasGroups() bool {
	for _, it := range c.items {
		if it.group != "" {
			return true
		}
	}
	return false
}

// showGroupTabs reports whether the group tab strip should render. Tabs stay
// visible while filtering so the popup layout does not jump as matches narrow.
func (c ConnectionList) showGroupTabs() bool {
	return c.hasGroups()
}

// availableGroupTabs returns group keys for the tab strip: named groups in
// alphabetical order, then Ungrouped ("") when any ungrouped connection exists.
func (c ConnectionList) availableGroupTabs() []string {
	ungrouped, order, _ := c.groupSections()
	out := append([]string{}, order...)
	if len(ungrouped) > 0 {
		out = append(out, "")
	}
	return out
}

// groupTabLabel is the display label for a group tab key.
func groupTabLabel(group string) string {
	if group == "" {
		return "Ungrouped"
	}
	return group
}

// groupTabAvailable reports whether g is in the current tab strip.
func (c ConnectionList) groupTabAvailable(g string) bool {
	for _, t := range c.availableGroupTabs() {
		if t == g {
			return true
		}
	}
	return false
}

// clampGroupTab moves onto a valid tab when the active one disappeared.
func (c *ConnectionList) clampGroupTab() {
	if !c.hasGroups() {
		c.groupTab = ""
		return
	}
	if c.groupTabAvailable(c.groupTab) {
		return
	}
	tabs := c.availableGroupTabs()
	if len(tabs) == 0 {
		c.groupTab = ""
		return
	}
	c.groupTab = tabs[0]
}

// setGroupTab switches to group g and resets the list cursor to the top.
func (c *ConnectionList) setGroupTab(g string) {
	if !c.hasGroups() || !c.groupTabAvailable(g) {
		return
	}
	if g == c.groupTab {
		return
	}
	c.groupTab = g
	c.cursor = 0
	c.scroll = 0
	c.ensureVisible(c.rows())
}

// MoveGroupTab steps left/right through availableGroupTabs (wrapping).
func (c *ConnectionList) MoveGroupTab(delta int) {
	tabs := c.availableGroupTabs()
	if len(tabs) == 0 {
		return
	}
	idx := 0
	for i, t := range tabs {
		if t == c.groupTab {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(tabs)) % len(tabs)
	c.setGroupTab(tabs[idx])
}

// ActiveGroupTab returns the active group key ("" = Ungrouped / no groups).
func (c ConnectionList) ActiveGroupTab() string {
	return c.groupTab
}

// groupSections partitions items into the ungrouped slice (config order) and
// named buckets (config order within each), returning group names in
// alphabetical order.
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

// demoInvitation is the synthetic row shown when there are no saved
// connections, so first-run users can open the sample database with Enter.
func demoInvitation() connectionItem {
	return connectionItem{
		name:   "Try the demo database",
		driver: "sqlite",
		detail: "sample e-commerce schema — press enter",
		demo:   true,
	}
}

// rows returns the renderable row sequence for the current state:
//   - empty list: a single synthetic demo invitation (first-run);
//   - filtering: a flat, fuzzy-ranked list of matching connections;
//   - no groups: a flat list of all connections;
//   - otherwise: connections in the active group tab only (no headers).
func (c ConnectionList) rows() []connRow {
	if len(c.items) == 0 {
		if c.filtering && c.filter != "" {
			return nil // typed filter with nothing to match
		}
		return []connRow{{kind: rowConn, conn: demoInvitation()}}
	}
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
	return c.groupTabRows()
}

// filteredRows ranks all connections by the fuzzy filter and returns them as
// flat connection rows, with match indices for highlighting.
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

// groupTabRows returns connections belonging to the active group tab.
func (c ConnectionList) groupTabRows() []connRow {
	ungrouped, _, buckets := c.groupSections()
	var items []connectionItem
	if c.groupTab == "" {
		items = ungrouped
	} else {
		items = buckets[c.groupTab]
	}
	out := make([]connRow, len(items))
	for i, it := range items {
		out[i] = connRow{kind: rowConn, conn: it, group: it.group}
	}
	return out
}

// rowHeight returns the rendered height of a row (always one line).
func rowHeight(r connRow) int {
	return 1
}

// ExpandedHeight is the total rendered height of every connection (one line
// each). Kept for tests/diagnostics; popup sizing uses the shared shell.
func (c ConnectionList) ExpandedHeight() int {
	if len(c.items) == 0 {
		return 1 // synthetic demo invitation
	}
	return len(c.items)
}

// listViewportHeight is the height available for connection rows (shell height
// minus the group tab strip when shown).
func (c ConnectionList) listViewportHeight() int {
	h := c.height
	if c.showGroupTabs() {
		h -= formTabBarLines
	}
	if h < 1 {
		return 1
	}
	return h
}

// TotalCount returns the total number of saved connections, ignoring any
// active filter, grouping, or the synthetic demo invitation.
func (c ConnectionList) TotalCount() int {
	return len(c.items)
}

// HasSavedConnections reports whether any real (non-demo) connections exist.
func (c ConnectionList) HasSavedConnections() bool {
	return len(c.items) > 0
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
// the cursor is on the synthetic demo invitation (use SelectedIsDemo instead).
func (c ConnectionList) SelectedName() string {
	rows := c.rows()
	if c.cursor < 0 || c.cursor >= len(rows) {
		return ""
	}
	r := rows[c.cursor]
	if r.kind != rowConn || r.conn.demo {
		return ""
	}
	return r.conn.name
}

// SelectedIsDemo reports whether the cursor is on the first-run demo invitation.
func (c ConnectionList) SelectedIsDemo() bool {
	rows := c.rows()
	if c.cursor < 0 || c.cursor >= len(rows) {
		return false
	}
	r := rows[c.cursor]
	return r.kind == rowConn && r.conn.demo
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

// SetCursor sets the cursor position, clamped to the row sequence. While
// filtering across groups, the active tab highlight follows the connection
// under the cursor.
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
	c.syncGroupTabFromRow(rows[c.cursor])
	c.ensureVisible(rows)
}

// syncGroupTabFromRow updates the tab highlight to match a connection's group
// without resetting the cursor (used while filtering).
func (c *ConnectionList) syncGroupTabFromRow(r connRow) {
	if !c.hasGroups() || r.kind != rowConn || r.conn.demo {
		return
	}
	if !c.groupTabAvailable(r.conn.group) {
		return
	}
	c.groupTab = r.conn.group
}

// MoveCursor moves the cursor by delta rows.
func (c *ConnectionList) MoveCursor(delta int) {
	c.SetCursor(c.cursor + delta)
}

// firstConnRow / lastConnRow return the cursor index of the first/last
// connection row so g/G land on a connectable entry.
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

// ensureVisible adjusts scroll so the cursor row stays in the list viewport,
// keeping the scroll snapped to a row boundary. Among the valid boundaries it
// picks the smallest (most content above the cursor), so the cursor sits near
// the bottom of the viewport rather than jumping to the top.
func (c *ConnectionList) ensureVisible(rows []connRow) {
	vh := c.listViewportHeight()
	if len(rows) == 0 || vh <= 0 {
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
	lo := cursorBot - vh // smallest scroll that still fits the cursor's bottom
	hi := cursorTop      // largest scroll that keeps the cursor's top visible
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
	return c.listViewportHeight()
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
	c.ensureVisible(c.rows())
}

// CommitFilter exits filter mode, switching to the selected connection's group
// tab (when groups are in use) so the cursor stays on that connection.
func (c *ConnectionList) CommitFilter() {
	name := c.SelectedName()
	c.filtering = false
	c.filter = ""
	if name != "" && c.SelectByName(name) {
		return
	}
	c.cursor = 0
	c.ensureVisible(c.rows())
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

// View renders quiet name+host rows for the active group (or ranked filter
// matches). The group tab strip is rendered separately above the filter prompt
// (see GroupTabBar / viewConnections).
func (c ConnectionList) View() string {
	rows := c.rows()
	contentW := c.width
	vh := c.listViewportHeight()

	if len(rows) == 0 {
		var msg string
		if c.filtering {
			msg = mutedStyle.Render("  (no matches)")
		} else {
			msg = mutedStyle.Render("  No saved connections. Press 'n' to add one.")
		}
		return padViewHeight(msg, contentW, vh, c.padBg)
	}

	tops := prefixTops(rows)
	totalH := tops[len(rows)]
	maxScroll := totalH - vh
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
			continue
		}
		if top >= scroll+vh {
			break
		}
		if bot > scroll+vh {
			break
		}
		b.WriteString(c.renderRow(r, i, contentW))
		b.WriteString("\n")
	}
	return padViewHeight(strings.TrimRight(b.String(), "\n"), contentW, vh, c.padBg)
}

// GroupTabBar returns the right-aligned group tab strip, or "" when no groups
// are in use. Rendered above the filter prompt in the connection popup; stays
// visible while filtering.
func (c ConnectionList) GroupTabBar() string {
	if !c.showGroupTabs() {
		return ""
	}
	return c.renderGroupTabBar()
}

// renderGroupTabBar renders available group tabs, right-aligned like the
// connection form's page tabs.
func (c ConnectionList) renderGroupTabBar() string {
	var parts []string
	for _, g := range c.availableGroupTabs() {
		l := groupTabLabel(g)
		var s string
		if g == c.groupTab {
			s = lipgloss.NewStyle().
				Bold(true).
				Background(colorPrimary).
				Foreground(colorBg).
				Render(" " + l + " ")
		} else {
			s = lipgloss.NewStyle().Foreground(colorMuted).Render(" " + l + " ")
		}
		parts = append(parts, s)
	}
	tabs := strings.Join(parts, " ")
	w := c.width
	if w < 1 {
		return tabs
	}
	return lipgloss.NewStyle().Width(w).Align(lipgloss.Right).Render(tabs)
}

// groupTabWidth is the plain-text width of the tab strip for mouse hit-testing.
func (c ConnectionList) groupTabWidth() int {
	tabs := c.availableGroupTabs()
	if len(tabs) == 0 {
		return 0
	}
	total := 0
	for i, g := range tabs {
		total += 1 + len(groupTabLabel(g)) + 1
		if i < len(tabs)-1 {
			total++
		}
	}
	return total
}

// ClickGroupTab switches to the group tab under content-relative coordinates
// within the panel body (y=0 is the tab row). Returns true when a tab was hit.
// While filtering, jumps the cursor to the first match in that group (filter
// stays cross-group); otherwise shows only that group's connections.
func (c *ConnectionList) ClickGroupTab(contentX, contentY int) bool {
	if !c.showGroupTabs() || contentY < 0 || contentY >= formTabBarLines {
		return false
	}
	total := c.groupTabWidth()
	start := c.width - total
	if start < 0 {
		start = 0
	}
	cur := start
	for _, g := range c.availableGroupTabs() {
		w := 1 + len(groupTabLabel(g)) + 1
		if contentX >= cur && contentX < cur+w {
			if c.filtering && c.filter != "" {
				c.groupTab = g
				rows := c.rows()
				for i, r := range rows {
					if r.kind == rowConn && r.conn.group == g {
						c.SetCursor(i)
						return true
					}
				}
				return true
			}
			c.setGroupTab(g)
			return true
		}
		cur += w + 1
	}
	return false
}

// renderRow renders a single connection line (name + muted trailing detail).
func (c ConnectionList) renderRow(r connRow, idx, contentW int) string {
	isCursor := idx == c.cursor
	item := r.conn
	namePlain := item.name
	detailPlain := strings.TrimSpace(item.detail)
	rowW := contentW
	if rowW < 1 {
		rowW = 1
	}

	const minGap = 2
	nameBudget := rowW
	detailBudget := 0
	if detailPlain != "" {
		detailBudget = runeLen(detailPlain)
		nameBudget = rowW - minGap - detailBudget
		if nameBudget < 8 {
			nameBudget = min(rowW/2, rowW-minGap-1)
			if nameBudget < 1 {
				nameBudget = 1
			}
			detailBudget = rowW - minGap - nameBudget
			if detailBudget < 0 {
				detailBudget = 0
			}
		}
	}
	nameDisp := truncateCell(namePlain, nameBudget)
	detailDisp := ""
	if detailBudget > 0 {
		detailDisp = truncateCell(detailPlain, detailBudget)
	}

	var nameStr string
	switch {
	case c.filtering && c.filter != "":
		nameStr = highlightMatches(nameDisp, clampMatchIdx(item.matchIdx, nameDisp))
	case isCursor:
		nameStr = lipgloss.NewStyle().Foreground(colorFg).Bold(true).Render(nameDisp)
	default:
		nameStr = lipgloss.NewStyle().Foreground(colorFg).Render(nameDisp)
	}

	line := nameStr
	if detailDisp != "" {
		pad := rowW - lipgloss.Width(nameStr) - runeLen(detailDisp)
		if pad < minGap {
			pad = minGap
		}
		detailStr := mutedStyle.Render(detailDisp)
		line = nameStr + strings.Repeat(" ", pad) + detailStr
		if lipgloss.Width(line) > rowW {
			line = nameStr + strings.Repeat(" ", minGap) + detailStr
		}
	}
	return line
}

// clampMatchIdx drops fuzzy highlight indices past the displayed (possibly
// truncated) name so highlightMatches does not panic or style garbage.
func clampMatchIdx(idx []int, displayed string) []int {
	if len(idx) == 0 {
		return idx
	}
	n := runeLen(displayed)
	out := make([]int, 0, len(idx))
	for _, i := range idx {
		if i >= 0 && i < n {
			out = append(out, i)
		}
	}
	return out
}

// YToRow maps a y-offset within the list rows area (below any group tab strip)
// to a row index, accounting for the current scroll. Returns -1 when the
// offset doesn't land on a row.
func (c ConnectionList) YToRow(y int) int {
	rows := c.rows()
	if len(rows) == 0 {
		return -1
	}
	vh := c.listViewportHeight()
	tops := prefixTops(rows)
	totalH := tops[len(rows)]
	maxScroll := totalH - vh
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
	vh := c.listViewportHeight()
	if totalH > vh {
		scroll := c.scroll
		if scroll > totalH-vh {
			scroll = totalH - vh
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
			if tops[i] < scroll+vh {
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
