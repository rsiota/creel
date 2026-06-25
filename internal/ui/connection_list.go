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
	if !c.filtering || c.filter == "" {
		return c.items
	}
	type scored struct {
		item  connectionItem
		score int
	}
	var results []scored
	for _, item := range c.items {
		idx, score := fuzzyMatch(c.filter, item.name)
		if idx != nil {
			cp := item
			cp.matchIdx = idx
			results = append(results, scored{cp, score})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score < results[j].score
		}
		return results[i].item.name < results[j].item.name
	})
	items := make([]connectionItem, len(results))
	for i, r := range results {
		items[i] = r.item
	}
	return items
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
	// Each entry renders as 2 lines (name + detail). Reserve margin for
	// the bottom scroll-info line.
	max := c.height - 3
	if max < 1 {
		max = 1
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

// View renders the connection list content (without outer border).
func (c ConnectionList) View() string {
	items := c.visibleItems()

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

	var b strings.Builder

	if len(items) == 0 {
		emptyHeight := c.height - 2
		if emptyHeight < 1 {
			emptyHeight = 1
		}
		if c.filtering {
			b.WriteString(mutedStyle.Render("  (no matches)"))
		} else {
			b.WriteString(mutedStyle.Render("  No saved connections. Press 'n' to add one."))
		}
		padding := emptyHeight - 1
		for i := 0; i < padding; i++ {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Connection entries — 2 lines each (name + detail)
	for i := scroll; i < end; i++ {
		item := items[i]
		isCursor := i == c.cursor

		// Name line with driver badge
		nameText := item.name
		if c.filtering {
			nameText = highlightMatches(item.name, item.matchIdx)
		}

		driverBadge := lipgloss.NewStyle().Foreground(colorAccent).Render(
			fmt.Sprintf("[%s]", strings.ToUpper(item.driver)),
		)

		nameLine := nameText + "  " + driverBadge
		if isCursor {
			nameLine = lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(nameLine)
		} else {
			nameLine = normalStyle.Render(nameLine)
		}
		b.WriteString(nameLine)
		b.WriteString("\n")

		// Detail line
		detailStyle := lipgloss.NewStyle().Foreground(colorMuted)
		if isCursor {
			detailStyle = lipgloss.NewStyle().Foreground(colorLabel)
		}
		detail := truncateSidebarLine("  "+item.detail, c.width)
		b.WriteString(detailStyle.Render(detail))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// Prompt returns the filter prompt line shown at the top of the panel.
func (c ConnectionList) Prompt() string {
	return renderPalettePrompt(c.filter)
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
	return ""
}
