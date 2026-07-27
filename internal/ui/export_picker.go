package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

// exportItem represents a table in the export picker.
type exportItem struct {
	name   string
	marked bool
}

// ExportPicker is a multi-select overlay for choosing tables to export.
type ExportPicker struct {
	items     []exportItem
	cursor    int
	scrollRow int
	visible   bool
	width     int
	height    int
	format    db.Format
}

// NewExportPicker returns a zero-value ExportPicker.
func NewExportPicker() ExportPicker {
	return ExportPicker{format: db.FormatSQL}
}

// Show populates the picker with tables and marks them all selected by default.
// selectedTable (if non-empty) positions the cursor on that table.
func (p *ExportPicker) Show(tables []string, selectedTable string) {
	p.items = make([]exportItem, len(tables))
	for i, t := range tables {
		p.items[i] = exportItem{name: t, marked: true}
	}
	// Position cursor on the selected table if present.
	p.cursor = 0
	for i, item := range p.items {
		if item.name == selectedTable {
			p.cursor = i
			break
		}
	}
	p.scrollRow = 0
	p.visible = true
}

// Hide clears all state and hides the picker.
func (p *ExportPicker) Hide() {
	p.items = nil
	p.cursor = 0
	p.scrollRow = 0
	p.visible = false
}

// IsVisible reports whether the picker is shown.
func (p ExportPicker) IsVisible() bool { return p.visible }

// SetSize sets the rendering dimensions for the picker panel.
func (p *ExportPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// CursorUp moves the cursor up by one.
func (p *ExportPicker) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
	}
	p.adjustScroll()
}

// CursorDown moves the cursor down by one.
func (p *ExportPicker) CursorDown() {
	if p.cursor < len(p.items)-1 {
		p.cursor++
	}
	p.adjustScroll()
}

func (p *ExportPicker) adjustScroll() {
	maxVisible := p.maxVisible()
	if p.cursor < p.scrollRow {
		p.scrollRow = p.cursor
	}
	if p.cursor >= p.scrollRow+maxVisible {
		p.scrollRow = p.cursor - maxVisible + 1
	}
}

func (p ExportPicker) maxVisible() int {
	// Content area = height - 2 (border) - 1 (spacer) - 1 (footer).
	mv := p.height - 4
	if mv < 1 {
		mv = 1
	}
	return mv
}

// ToggleSelected flips the marked state of the cursor row.
func (p *ExportPicker) ToggleSelected() {
	if p.cursor >= 0 && p.cursor < len(p.items) {
		p.items[p.cursor].marked = !p.items[p.cursor].marked
	}
}

// SelectAll marks every table.
func (p *ExportPicker) SelectAll() {
	for i := range p.items {
		p.items[i].marked = true
	}
}

// SelectNone clears every table mark.
func (p *ExportPicker) SelectNone() {
	for i := range p.items {
		p.items[i].marked = false
	}
}

// MarkedCount returns how many tables are currently marked.
func (p ExportPicker) MarkedCount() int {
	n := 0
	for _, item := range p.items {
		if item.marked {
			n++
		}
	}
	return n
}

// SelectedTables returns the marked table names in list order.
func (p ExportPicker) SelectedTables() []string {
	var tables []string
	for _, item := range p.items {
		if item.marked {
			tables = append(tables, item.name)
		}
	}
	return tables
}

// CurrentFormat returns the active export format.
func (p ExportPicker) CurrentFormat() db.Format { return p.format }

// CycleFormat advances to the next supported format.
func (p *ExportPicker) CycleFormat() {
	switch p.format {
	case db.FormatSQL:
		// CSV/JSON not yet wired — keep on SQL for now.
		p.format = db.FormatSQL
	default:
		p.format = db.FormatSQL
	}
}

// View renders the picker panel.
func (p ExportPicker) View() string {
	if !p.visible {
		return ""
	}

	maxVisible := p.maxVisible()
	end := p.scrollRow + maxVisible
	if end > len(p.items) {
		end = len(p.items)
	}

	var rows []string
	for i := p.scrollRow; i < end; i++ {
		item := p.items[i]

		tick := ""
		if item.marked {
			tick = lipgloss.NewStyle().Foreground(colorFg).Render("●")
		}

		rows = append(rows, renderPaletteRowWithTick(item.name, tick, i == p.cursor, p.width-4))
	}

	if len(p.items) == 0 {
		rows = append(rows, mutedStyle.Render("  no tables"))
	}

	// Pad to fixed height.
	for len(rows) < maxVisible {
		rows = append(rows, "")
	}

	listStyled := lipgloss.NewStyle().
		Height(maxVisible).
		Render(strings.Join(rows, "\n"))

	formatLabel := strings.ToUpper(string(p.format))
	footer := mutedStyle.Render("  Export | " + formatLabel + " | " + fmt.Sprintf("%d-%d", p.MarkedCount(), len(p.items)))

	content := lipgloss.JoinVertical(lipgloss.Left,
		listStyled,
		"",
		footer,
	)

	panel := lipgloss.NewStyle().
		Width(p.width-2).
		Height(p.height-2).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)

	return panel
}
