package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

// LookupPanel is a scrollable, read-only overlay that displays a titled table
// of lookup results (a db.Result). It backs the ":refs" (reverse foreign keys)
// and ":uses" (object dependents) commands, both of which populate it with a
// header and a synthesized result table. It mirrors the EXPLAIN panel's
// machinery; a later revision could add enter-to-jump on the selected row.
type LookupPanel struct {
	visible bool
	title   string
	result  db.Result
	cursor  int
	scroll  int
	width   int
	height  int
}

func (p LookupPanel) IsVisible() bool { return p.visible }

// Show populates the panel with a title and result table and makes it visible.
// The title is rendered with the result's row count.
func (p *LookupPanel) Show(title string, result db.Result) {
	p.visible = true
	p.title = title
	p.cursor = 0
	p.scroll = 0
	p.result = result
}

// Hide hides the panel.
func (p *LookupPanel) Hide() { p.visible = false }

// SetSize sets the content dimensions of the panel (including border).
func (p *LookupPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p LookupPanel) contentHeight() int {
	h := p.height - borderOverhead
	if h < 1 {
		h = 1
	}
	return h
}

// Update handles keyboard input (scrolling). Returns the updated panel.
func (e LookupPanel) Update(msg tea.KeyMsg) LookupPanel {
	n := len(e.lines())
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

func (e *LookupPanel) adjustScroll(vh int) {
	if e.cursor < e.scroll {
		e.scroll = e.cursor
	}
	if e.cursor >= e.scroll+vh {
		e.scroll = e.cursor - vh + 1
	}
}

// View renders the panel with a border.
func (e LookupPanel) View() string {
	lines := e.lines()
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
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)
}

// lines returns the display rows: a header with the title and row count, then
// the result table via the shared generic-plan renderer. When there are no
// rows, only the header (with count 0) is shown.
func (e LookupPanel) lines() []string {
	header := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(
		fmt.Sprintf("%s (%d)", e.title, len(e.result.Rows)))
	if len(e.result.Rows) == 0 {
		return []string{header}
	}
	return append([]string{header}, renderGenericPlan(e.result)...)
}
