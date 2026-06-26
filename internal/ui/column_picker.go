package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// colItem is a single column with its visibility state and fuzzy metadata.
type colItem struct {
	name     string
	visible  bool // true = shown in results, false = hidden
	matchIdx []int
	score    int
}

// ColumnPicker is a popup overlay for toggling column visibility. It lists
// every column with a checkbox (✓ = visible); unchecked columns are hidden in
// the results table on apply. Changes are committed with enter, discarded with
// esc.
type ColumnPicker struct {
	items     []colItem
	filter    string
	cursor    int
	scrollRow int
	visible   bool
	width     int
	height    int
}

// NewColumnPicker creates a hidden column picker.
func NewColumnPicker() ColumnPicker {
	return ColumnPicker{}
}

// Show populates the picker with all columns and their current visibility.
func (p *ColumnPicker) Show(columns []string, hidden map[string]bool) {
	if hidden == nil {
		hidden = make(map[string]bool)
	}
	p.items = make([]colItem, len(columns))
	for i, c := range columns {
		p.items[i] = colItem{name: c, visible: !hidden[c]}
	}
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
	p.visible = true
}

// Hide closes the picker.
func (p *ColumnPicker) Hide() {
	p.visible = false
	p.items = nil
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
}

// IsVisible returns whether the picker is shown.
func (p ColumnPicker) IsVisible() bool { return p.visible }

// SetSize sets the dimensions of the picker.
func (p *ColumnPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// HiddenColumns returns the names of unchecked (hidden) columns, in their
// original column order.
func (p ColumnPicker) HiddenColumns() []string {
	var out []string
	for _, it := range p.items {
		if !it.visible {
			out = append(out, it.name)
		}
	}
	return out
}

// VisibleCount returns how many columns are currently checked.
func (p ColumnPicker) VisibleCount() int {
	n := 0
	for _, it := range p.items {
		if it.visible {
			n++
		}
	}
	return n
}

func (p ColumnPicker) filteredItems() []colItem {
	if p.filter == "" {
		return p.items
	}
	var out []colItem
	for _, it := range p.items {
		if idx, score := fuzzyMatch(p.filter, it.name); idx != nil {
			it.matchIdx = idx
			it.score = score
			out = append(out, it)
		}
	}
	// Rank best matches first; alphabetical as a tiebreaker.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score < out[j].score
		}
		return out[i].name < out[j].name
	})
	return out
}

// CursorUp moves the selection up.
func (p *ColumnPicker) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.adjustScroll()
	}
}

// CursorDown moves the selection down.
func (p *ColumnPicker) CursorDown() {
	items := p.filteredItems()
	if p.cursor < len(items)-1 {
		p.cursor++
		p.adjustScroll()
	}
}

func (p *ColumnPicker) adjustScroll() {
	maxVisible := p.height - 6
	if maxVisible < 1 {
		maxVisible = 1
	}
	if p.cursor < p.scrollRow {
		p.scrollRow = p.cursor
	}
	if p.cursor >= p.scrollRow+maxVisible {
		p.scrollRow = p.cursor - maxVisible + 1
	}
}

// FilterAddChar appends a character to the fuzzy filter.
func (p *ColumnPicker) FilterAddChar(ch string) {
	p.filter += ch
	p.cursor = 0
	p.scrollRow = 0
}

// FilterBackspace removes the last character from the filter.
func (p *ColumnPicker) FilterBackspace() {
	if len(p.filter) > 0 {
		p.filter = p.filter[:len(p.filter)-1]
		p.cursor = 0
		p.scrollRow = 0
	}
}

// filtering returns true when the user is actively typing a filter.
func (p ColumnPicker) filtering() bool { return p.filter != "" }

// ToggleSelected flips the visibility of the column at the cursor. The last
// visible column is never hidden, so the results table is never empty. The
// cursor stays on the toggled column so the user sees its state change in
// place — no reordering.
func (p *ColumnPicker) ToggleSelected() {
	items := p.filteredItems()
	if p.cursor < 0 || p.cursor >= len(items) {
		return
	}
	target := items[p.cursor].name
	toggled := false
	for i := range p.items {
		if p.items[i].name == target {
			// Refuse to hide the final visible column.
			if p.items[i].visible && p.VisibleCount() <= 1 {
				return
			}
			p.items[i].visible = !p.items[i].visible
			toggled = true
			break
		}
	}
	// Clear the filter so the full list is visible again, but keep the
	// cursor on the column that was just toggled.
	p.filter = ""
	if toggled {
		for i, it := range p.items {
			if it.name == target {
				p.cursor = i
				break
			}
		}
	}
	p.adjustScroll()
}

// SelectAll marks every column visible.
func (p *ColumnPicker) SelectAll() {
	for i := range p.items {
		p.items[i].visible = true
	}
}

// SelectNone hides every column except the last one (keeping at least one
// visible), since an empty table is invalid.
func (p *ColumnPicker) SelectNone() {
	for i := range p.items {
		p.items[i].visible = false
	}
	if len(p.items) > 0 {
		p.items[0].visible = true
	}
}

// View renders the picker overlay.
func (p ColumnPicker) View() string {
	if !p.visible {
		return ""
	}

	title := titleStyle.Render("Column Visibility")
	prompt := renderPalettePrompt(p.filter, true)

	items := p.filteredItems()

	maxVisible := p.height - 7 // border(2) + title(1) + prompt(1) + hint(1) + padding(2)
	if maxVisible < 1 {
		maxVisible = 1
	}

	end := p.scrollRow + maxVisible
	if end > len(items) {
		end = len(items)
	}

	var rows []string
	for i := p.scrollRow; i < end; i++ {
		item := items[i]
		name := item.name
		if p.filter != "" && i != p.cursor {
			name = highlightMatches(item.name, item.matchIdx)
		}

		tick := ""
		if item.visible {
			tick = lipgloss.NewStyle().Foreground(colorFg).Render("●")
		}

		rows = append(rows, renderPaletteRowWithTick(name, tick, i == p.cursor, p.width-6))
	}

	if len(items) == 0 {
		rows = append(rows, mutedStyle.Render("  no matches"))
	}

	// Pad to fixed height.
	for len(rows) < maxVisible {
		rows = append(rows, "")
	}

	listStyled := lipgloss.NewStyle().
		Height(maxVisible).
		Render(strings.Join(rows, "\n"))

	summary := mutedStyle.Render(fmt.Sprintf("%d shown", p.VisibleCount()))
	hint := mutedStyle.Render("space toggle  ctrl+a all  ctrl+n none  enter apply  esc cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		prompt,
		listStyled,
		summary+"  "+hint,
	)

	panel := lipgloss.NewStyle().
		Width(p.width - 2).
		Height(p.height - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Render(content)

	return panel
}
