package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

// RefsPanel lists the foreign keys that reference a given table (":refs
// <table>"): every (child table, child column) pointing at it — the reverse of
// g d / ForeignKeys. Shown as a scrollable, read-only overlay, mirroring the
// EXPLAIN panel's machinery. A later revision can add enter-to-jump (navigate
// to the selected child table).
type RefsPanel struct {
	visible bool
	table   string // the parent table being inspected
	result  db.Result
	cursor  int
	scroll  int
	width   int
	height  int
}

// IsVisible reports whether the panel is shown.
func (r RefsPanel) IsVisible() bool { return r.visible }

// Show populates the panel from a referrer list and makes it visible. The
// referrers are shaped into a 3-column result (Table / Column / References)
// so the shared generic table renderer can display them.
func (r *RefsPanel) Show(table string, refs []db.Referrer) {
	r.visible = true
	r.table = table
	r.cursor = 0
	r.scroll = 0
	cols := []db.Column{{Name: "Table"}, {Name: "Column"}, {Name: "References"}}
	rows := make([][]string, 0, len(refs))
	for _, ref := range refs {
		rows = append(rows, []string{ref.Table, ref.Column, table + "." + ref.RefColumn})
	}
	r.result = db.Result{Columns: cols, Rows: rows}
}

// Hide hides the panel.
func (r *RefsPanel) Hide() { r.visible = false }

// SetSize sets the content dimensions of the panel (excluding border).
func (r *RefsPanel) SetSize(width, height int) {
	r.width = width
	r.height = height
}

func (r RefsPanel) contentHeight() int {
	h := r.height - borderOverhead
	if h < 1 {
		h = 1
	}
	return h
}

// Update handles keyboard input (scrolling). Returns the updated panel.
func (e RefsPanel) Update(msg tea.KeyMsg) RefsPanel {
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

func (e *RefsPanel) adjustScroll(vh int) {
	if e.cursor < e.scroll {
		e.scroll = e.cursor
	}
	if e.cursor >= e.scroll+vh {
		e.scroll = e.cursor - vh + 1
	}
}

// View renders the panel with a border.
func (e RefsPanel) View() string {
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

// lines returns the display rows: a header naming the target table and its
// referrer count, then the referrer table via the shared generic-plan table
// renderer. An empty result yields a single explanatory line.
func (e RefsPanel) lines() []string {
	if len(e.result.Rows) == 0 {
		return []string{mutedStyle.Render(
			fmt.Sprintf("no foreign keys reference %s", e.table))}
	}
	header := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(
		fmt.Sprintf("References to %s (%d)", e.table, len(e.result.Rows)))
	return append([]string{header}, renderGenericPlan(e.result)...)
}
