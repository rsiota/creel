package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// connectionItem implements list.Item.
type connectionItem struct {
	name   string
	driver string
	detail string
}

func (i connectionItem) Title() string       { return i.name }
func (i connectionItem) Description() string { return i.detail }
func (i connectionItem) FilterValue() string { return i.name }

// ConnectionList is the component for selecting a saved connection.
type ConnectionList struct {
	list        list.Model
	width       int
	height      int
	placeholder string
}

// connectionDelegate renders list items with driver badges.
type connectionDelegate struct{}

func (d connectionDelegate) Height() int                             { return 1 }
func (d connectionDelegate) Spacing() int                            { return 0 }
func (d connectionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d connectionDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	conn, ok := listItem.(connectionItem)
	if !ok {
		return
	}

	cursor := " "
	style := normalStyle
	if index == m.Index() {
		cursor = ">"
		style = selectedStyle
	}

	driverBadge := fmt.Sprintf("[%s]", strings.ToUpper(conn.driver))
	rendered := fmt.Sprintf("%s %s %s", cursor, conn.name, lipgloss.NewStyle().Foreground(colorAccent).Render(driverBadge))
	fmt.Fprintln(w, style.Render(rendered))
}

// NewConnectionList creates a new connection list component.
func NewConnectionList() ConnectionList {
	delegate := connectionDelegate{}
	l := list.New([]list.Item{}, delegate, 40, 10)
	l.Title = "Select a connection"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = mutedStyle
	l.Styles.HelpStyle = helpStyle
	l.AdditionalShortHelpKeys = nil

	return ConnectionList{
		list:        l,
		placeholder: "No saved connections. Press 'n' to add a new SQLite connection.",
	}
}

// SetItems populates the list from a map of connection names to driver+detail.
type ConnectionEntry struct {
	Name   string
	Driver string
	Detail string
}

func (c *ConnectionList) SetItems(conns []ConnectionEntry) {
	items := make([]list.Item, len(conns))
	for i, conn := range conns {
		items[i] = connectionItem{
			name:   conn.Name,
			driver: conn.Driver,
			detail: conn.Detail,
		}
	}
	c.list.SetItems(items)
}

// SelectedName returns the name of the currently highlighted connection.
func (c *ConnectionList) SelectedName() string {
	item, ok := c.list.SelectedItem().(connectionItem)
	if !ok {
		return ""
	}
	return item.name
}

// SelectedDriver returns the driver of the currently highlighted connection.
func (c *ConnectionList) SelectedDriver() string {
	item, ok := c.list.SelectedItem().(connectionItem)
	if !ok {
		return ""
	}
	return item.driver
}

// Update handles messages for the connection list.
func (c ConnectionList) Update(msg tea.Msg) (ConnectionList, tea.Cmd) {
	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

// View renders the connection list.
func (c ConnectionList) View() string {
	if len(c.list.Items()) == 0 {
		return mutedStyle.Render(c.placeholder)
	}
	return c.list.View()
}

// SetSize sets the dimensions of the connection list.
func (c *ConnectionList) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.list.SetSize(width, height)
}
