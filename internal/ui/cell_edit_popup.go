package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CellEditPopup is a modal multiline editor for cell values whose content is
// too large to edit comfortably inline (truncated with an ellipsis in the grid
// or inspector). It opens a centered textarea large enough to visualize and
// edit the value. On commit the value is staged into the same dirtyCells
// pipeline used by the inline editor (no immediate DB flush).
type CellEditPopup struct {
	ta      textarea.Model
	visible bool
	row     int    // results row index
	col     int    // results column index
	colName string // column header, shown in the popup title line
	width   int    // content width (excludes border + padding)
	height  int    // content height in lines
}

// NewCellEditPopup creates a hidden cell-edit popup.
func NewCellEditPopup() CellEditPopup {
	return CellEditPopup{}
}

// Show opens the popup seeded with the given value.
func (p *CellEditPopup) Show(val string, row, col int, colName string) {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // no limit
	ta.SetValue(val)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorFg)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	ta.BlurredStyle = ta.FocusedStyle
	p.ta = ta
	p.visible = true
	p.row = row
	p.col = col
	p.colName = colName
}

// Hide closes the popup.
func (p *CellEditPopup) Hide() {
	p.visible = false
	p.ta = textarea.Model{}
	p.colName = ""
}

// IsVisible reports whether the popup is open.
func (p CellEditPopup) IsVisible() bool {
	return p.visible
}

// Value returns the current editor contents.
func (p CellEditPopup) Value() string {
	return p.ta.Value()
}

// Row returns the results row the popup was opened on.
func (p CellEditPopup) Row() int {
	return p.row
}

// Col returns the results column the popup was opened on.
func (p CellEditPopup) Col() int {
	return p.col
}

// SetMaxSize clamps the popup's textarea dimensions to fit the available
// terminal area. contentW/contentH are the maximum usable cell counts for the
// textarea body; the bordered frame adds a fixed overhead.
func (p *CellEditPopup) SetMaxSize(contentW, contentH int) {
	// Reserve 4 cols for the "│ " + " │" padding around each textarea line.
	textW := contentW - 4
	if textW < 30 {
		textW = 30
	}
	p.width = textW
	if contentH < 3 {
		contentH = 3
	}
	p.height = contentH
	p.ta.SetWidth(textW)
	p.ta.SetHeight(contentH)
}

// Focus focuses the textarea.
func (p *CellEditPopup) Focus() tea.Cmd {
	if !p.visible {
		return nil
	}
	return p.ta.Focus()
}

// Update forwards messages to the textarea.
func (p CellEditPopup) Update(msg tea.Msg) (CellEditPopup, tea.Cmd) {
	if !p.visible {
		return p, nil
	}
	var cmd tea.Cmd
	p.ta, cmd = p.ta.Update(msg)
	return p, cmd
}

// View renders the popup content (label + bordered textarea) without the outer
// rounded border, which is applied by the caller. The label and value border
// mirror the inspector's styling.
func (p CellEditPopup) View() string {
	label := lipgloss.NewStyle().Foreground(colorLabel).Render(p.colName)

	borderW := p.width + 2 // +2 for the inner " " padding on each side
	bs := lipgloss.NewStyle().Foreground(colorBorder)
	top := bs.Render("┌" + strings.Repeat("─", borderW) + "┐")
	bottom := bs.Render("└" + strings.Repeat("─", borderW) + "┘")

	// Render each textarea line inside the bordered frame.
	var lines []string
	lines = append(lines, " "+label, top)
	for _, line := range strings.Split(p.ta.View(), "\n") {
		lines = append(lines, bs.Render("│ ")+line+bs.Render(" │"))
	}
	lines = append(lines, bottom)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
