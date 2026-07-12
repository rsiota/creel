package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConnectionEntry holds display data for a saved connection.
type ConnectionEntry struct {
	Name   string
	Driver string
	Detail string
}

// connectionItem is a single connection in the list.
type connectionItem struct {
	name     string
	driver   string
	detail   string
	matchIdx []int // fuzzy match indices for highlighting
}

// ConnectionList is a custom component for selecting a saved connection.
type ConnectionList struct {
	items    []connectionItem
	cursor   int
	scroll   int
	width    int
	height   int

	// Fuzzy filter
	filter    string
	filtering bool
}

// NewConnectionList creates a new connection list component.
func NewConnectionList() ConnectionList {
	return ConnectionList{}
}

// SetItems populates the list from connection entries.
func (c *ConnectionList) SetItems(conns []ConnectionEntry) {
	c.items = make([]connectionItem, len(conns))
	for i, conn := range conns {
		c.items[i] = connectionItem{
			name:   conn.Name,
			driver: conn.Driver,
			detail: conn.Detail,
		}
	}
	if c.cursor >= len(c.items) {
		c.cursor = len(c.items) - 1
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
}

// visibleItems returns the items to display, filtered if in filter mode.
func (c ConnectionList) visibleItems() []connectionItem {
	return c.VisibleItemsForMouse()
}

// VisibleItemsForMouse returns the currently visible (possibly filtered)
// connection items. Exported for mouse-click coordinate mapping.
func (c ConnectionList) VisibleItemsForMouse() []connectionItem {
	if !c.filtering || c.filter == "" {
		return c.items
	}
	ranked := fuzzyRank(c.filter, c.items,
		func(it connectionItem) string { return it.name },
		func(a, b fuzzyResult[connectionItem]) bool { return a.Item.name < b.Item.name })
	items := make([]connectionItem, len(ranked))
	for i, r := range ranked {
		cp := r.Item
		cp.matchIdx = r.MatchIdx
		items[i] = cp
	}
	return items
}

// TotalCount returns the total number of connections, ignoring any active
// filter. Used for sizing the popup so its height stays constant while the
// user filters, instead of growing/shrinking with the match count.
func (c ConnectionList) TotalCount() int {
	return len(c.items)
}

// SelectedName returns the name of the currently highlighted connection.
func (c ConnectionList) SelectedName() string {
	items := c.visibleItems()
	if c.cursor < 0 || c.cursor >= len(items) {
		return ""
	}
	return items[c.cursor].name
}

// SelectedDriver returns the driver of the currently highlighted connection.
func (c ConnectionList) SelectedDriver() string {
	items := c.visibleItems()
	if c.cursor < 0 || c.cursor >= len(items) {
		return ""
	}
	return items[c.cursor].driver
}

// Cursor returns the current cursor index.
func (c ConnectionList) Cursor() int {
	return c.cursor
}

// SetCursor sets the cursor position.
func (c *ConnectionList) SetCursor(i int) {
	items := c.visibleItems()
	if len(items) == 0 {
		c.cursor = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(items) {
		i = len(items) - 1
	}
	c.cursor = i
	c.ensureVisible()
}

// MoveCursor moves the cursor by delta.
func (c *ConnectionList) MoveCursor(delta int) {
	c.SetCursor(c.cursor + delta)
}

// ensureVisible adjusts scroll so the cursor stays in view.
func (c *ConnectionList) ensureVisible() {
	maxVisible := c.maxVisibleItems()
	if c.cursor < c.scroll {
		c.scroll = c.cursor
	}
	if c.cursor >= c.scroll+maxVisible {
		c.scroll = c.cursor - maxVisible + 1
	}
	if c.scroll < 0 {
		c.scroll = 0
	}
}

func (c ConnectionList) maxVisibleItems() int {
	// Each entry renders as a field box (linesPerField lines).
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

// CommitFilter exits filter mode, keeping the cursor on the selected match.
func (c *ConnectionList) CommitFilter() {
	c.filtering = false
	c.filter = ""
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
	c.ensureVisible()
}

// View renders the connection list as a column of inspector-style field
// boxes: the connection name and driver badge on the label line, and the
// connection detail inside a bordered value box. The cursor entry gets the
// primary-coloured border, mirroring the focused field boxes in the form and
// inspector. (Without outer border; the popup chrome is added by viewConnections.)
func (c ConnectionList) View() string {
	items := c.visibleItems()
	contentW := c.width
	valueWidth := contentW - 4
	if valueWidth < 1 {
		valueWidth = 1
	}

	if len(items) == 0 {
		if c.filtering {
			return mutedStyle.Render("  (no matches)")
		}
		return mutedStyle.Render("  No saved connections. Press 'n' to add one.")
	}

	maxVisible := c.maxVisibleItems()
	scroll := c.scroll
	if scroll > len(items)-maxVisible && len(items) > maxVisible {
		scroll = len(items) - maxVisible
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + maxVisible
	if end > len(items) {
		end = len(items)
	}

	badgeSty := lipgloss.NewStyle().Foreground(colorAccent)
	nameBold := lipgloss.NewStyle().Foreground(colorFg).Bold(true)
	namePlain := lipgloss.NewStyle().Foreground(colorFg)
	detailSty := lipgloss.NewStyle().Foreground(colorMuted)
	detailCurSty := lipgloss.NewStyle().Foreground(colorLabel)

	var b strings.Builder
	for i := scroll; i < end; i++ {
		item := items[i]
		isCursor := i == c.cursor

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

		// Marker: the driver badge, right-aligned.
		markerStr := badgeSty.Render("[" + strings.ToUpper(item.driver) + "]")

		// Value: host/db detail, brightened for the cursor entry.
		dSty := detailSty
		if isCursor {
			dSty = detailCurSty
		}
		valueContent := dSty.Render(truncateCell(item.detail, valueWidth))

		b.WriteString(renderFieldBox(labelStr, markerStr, valueContent, contentW, isCursor))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// Prompt returns the filter prompt line shown at the top of the panel.
func (c ConnectionList) Prompt() string {
	return renderPalettePrompt(c.filter, c.filtering)
}

// ScrollInfo returns the bottom-bar text (scroll position).
func (c ConnectionList) ScrollInfo() string {
	items := c.visibleItems()
	if len(items) == 0 {
		return ""
	}
	maxVisible := c.maxVisibleItems()
	if len(items) > maxVisible {
		scroll := c.scroll
		if scroll > len(items)-maxVisible {
			scroll = len(items) - maxVisible
		}
		end := scroll + maxVisible
		if end > len(items) {
			end = len(items)
		}
		return mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", scroll+1, end, len(items)))
	}
	noun := "connections"
	if len(items) == 1 {
		noun = "connection"
	}
	return mutedStyle.Render(fmt.Sprintf(" %d %s", len(items), noun))
}
