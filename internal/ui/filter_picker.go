package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// filterValue is a single distinct column value with its selection state.
type filterValue struct {
	value    string
	selected bool
	matchIdx []int
}

// FilterPicker is a popup overlay for selecting one or more column values
// to filter results by. It fetches DISTINCT values from the database and
// lets the user toggle them with fuzzy search, then builds an IN clause.
type FilterPicker struct {
	values    []filterValue
	filter    string
	cursor    int
	scrollRow int
	visible   bool
	width     int
	height    int
	column    string // column being filtered
	loading   bool   // waiting for async query
}

// NewFilterPicker creates a hidden filter picker.
func NewFilterPicker() FilterPicker {
	return FilterPicker{}
}

// Show opens the picker for a given column in loading state.
func (p *FilterPicker) Show(column string) {
	p.visible = true
	p.column = column
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
	p.loading = true
	p.values = nil
}

// SetValues populates the picker with distinct values. Previously selected
// values are pre-checked based on any existing equality/IN filter.
func (p *FilterPicker) SetValues(values []string, preSelected map[string]bool) {
	p.loading = false
	p.values = make([]filterValue, len(values))
	for i, v := range values {
		p.values[i] = filterValue{
			value:    v,
			selected: preSelected[v],
		}
	}
	p.cursor = 0
	p.scrollRow = 0
}

// Hide closes the picker.
func (p *FilterPicker) Hide() {
	p.visible = false
	p.values = nil
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
}

// IsVisible returns whether the picker is shown.
func (p FilterPicker) IsVisible() bool { return p.visible }

// filtering returns true when the user is actively typing a filter.
func (p FilterPicker) filtering() bool { return p.filter != "" }

// SetSize sets the dimensions of the picker.
func (p *FilterPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// Column returns the column being filtered.
func (p FilterPicker) Column() string { return p.column }

// SelectedValues returns the set of selected values.
func (p FilterPicker) SelectedValues() []string {
	var out []string
	for _, v := range p.values {
		if v.selected {
			out = append(out, v.value)
		}
	}
	return out
}

func (p FilterPicker) filteredValues() []filterValue {
	if p.filter == "" {
		return p.values
	}
	var out []filterValue
	for _, v := range p.values {
		idx, _ := fuzzyMatch(p.filter, v.value)
		if idx != nil {
			v.matchIdx = idx
			out = append(out, v)
		}
	}
	return out
}

// CursorUp moves the selection up.
func (p *FilterPicker) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.adjustScroll()
	}
}

// CursorDown moves the selection down.
func (p *FilterPicker) CursorDown() {
	items := p.filteredValues()
	if p.cursor < len(items)-1 {
		p.cursor++
		p.adjustScroll()
	}
}

func (p *FilterPicker) adjustScroll() {
	maxVisible := p.height - 6
	if maxVisible < 1 {
		maxVisible = 1
	}
	if p.cursor < p.scrollRow {
		p.scrollRow = p.cursor
	}
	if p.cursor >= p.scrollRow + maxVisible {
		p.scrollRow = p.cursor - maxVisible + 1
	}
}

// FilterAddChar appends a character to the fuzzy filter.
func (p *FilterPicker) FilterAddChar(ch string) {
	p.filter += ch
	p.cursor = 0
	p.scrollRow = 0
}

// FilterBackspace removes the last character from the fuzzy filter.
func (p *FilterPicker) FilterBackspace() {
	if len(p.filter) > 0 {
		p.filter = p.filter[:len(p.filter)-1]
		p.cursor = 0
		p.scrollRow = 0
	}
}

// ToggleSelected flips the selection state of the value at the cursor.
func (p *FilterPicker) ToggleSelected() {
	items := p.filteredValues()
	if p.cursor < 0 || p.cursor >= len(items) {
		return
	}
	target := items[p.cursor].value
	for i := range p.values {
		if p.values[i].value == target {
			p.values[i].selected = !p.values[i].selected
			break
		}
	}
}

// SelectAll marks all values as selected.
func (p *FilterPicker) SelectAll() {
	for i := range p.values {
		p.values[i].selected = true
	}
}

// SelectNone clears all selections.
func (p *FilterPicker) SelectNone() {
	for i := range p.values {
		p.values[i].selected = false
	}
}

// View renders the picker overlay.
func (p FilterPicker) View() string {
	if !p.visible {
		return ""
	}

	title := titleStyle.Render(fmt.Sprintf("Filter: %s", p.column))

	if p.loading {
		content := lipgloss.JoinVertical(lipgloss.Left,
			title,
			"",
			mutedStyle.Render("  Loading values..."),
		)
		return lipgloss.NewStyle().
			Width(p.width-2).
			Height(p.height-2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Render(content)
	}

	items := p.filteredValues()

	// Sort: selected first, then alphabetically.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].selected != items[j].selected {
			return items[i].selected
		}
		return items[i].value < items[j].value
	})

	maxVisible := p.height - 6
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
		name := item.value
		if p.filter != "" {
			name = highlightMatches(item.value, item.matchIdx)
		}

		check := " "
		if item.selected {
			check = lipgloss.NewStyle().Foreground(colorSuccess).Render("✓")
		}

		marker := " "
		if i == p.cursor {
			marker = lipgloss.NewStyle().Foreground(colorPrimary).Render("▶")
		}

		line := fmt.Sprintf("%s %s  %s", marker, check, name)
		if i == p.cursor {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		} else {
			line = normalStyle.Render(line)
		}
		rows = append(rows, line)
	}

	if len(items) == 0 {
		rows = append(rows, mutedStyle.Render("  (no matches)"))
	}

	listStyled := lipgloss.NewStyle().
		Height(maxVisible).
		Render(strings.Join(rows, "\n"))

	filterLine := lipgloss.NewStyle().Foreground(colorPrimary).Render("/"+p.filter) +
		lipgloss.NewStyle().Foreground(colorAccent).Render("▏")

	selectedCount := len(p.SelectedValues())
	scrollInfo := ""
	if len(items) > maxVisible {
		scrollInfo = mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", p.scrollRow+1, end, len(items)))
	}
	summary := mutedStyle.Render(fmt.Sprintf("%d selected", selectedCount))

	footer := filterLine + "  " + scrollInfo + "  " + summary

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		listStyled,
		footer,
	)

	panel := lipgloss.NewStyle().
		Width(p.width - 2).
		Height(p.height - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Render(content)

	return panel
}
