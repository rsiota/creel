package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// filterValue is a single distinct column value with its selection state.
type filterValue struct {
	value      string
	selected   bool
	selectedAt int // selection sequence; higher = more recently toggled on
	matchIdx   []int
	score      int
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
	seq       int    // monotonic counter; stamps selectedAt on each toggle
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

// SetValues populates the picker with distinct values. Any values from an
// existing equality/IN filter on this column are pre-checked so the picker
// can be used to refine an existing filter.
func (p *FilterPicker) SetValues(values []string, preSelected map[string]bool) {
	p.loading = false
	if preSelected == nil {
		preSelected = make(map[string]bool)
	}
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
	var out []filterValue
	if p.filter == "" {
		// Copy so we can reorder without mutating p.values, which keeps the
		// canonical order returned by the database.
		out = make([]filterValue, len(p.values))
		copy(out, p.values)
	} else {
		for _, v := range p.values {
			idx, score := fuzzyMatch(p.filter, v.value)
			if idx != nil {
				v.matchIdx = idx
				v.score = score
				out = append(out, v)
			}
		}
	}
	// Sort once here so every consumer (cursor navigation, toggling,
	// rendering) sees the same order:
	//   1. Selected items first, most-recently-toggled at the very top so
	//      the user gets immediate feedback after each space press (the
	//      cursor resets to 0 and lands on the just-toggled value).
	//   2. Unselected items keep their database order when no fuzzy filter
	//      is active, or rank by fuzzy score then alphabetically when the
	//      user is typing.
	filtering := p.filter != ""
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].selected != out[j].selected {
			return out[i].selected
		}
		if out[i].selected {
			return out[i].selectedAt > out[j].selectedAt
		}
		if filtering {
			if out[i].score != out[j].score {
				return out[i].score < out[j].score
			}
			return out[i].value < out[j].value
		}
		return false // preserve DB order
	})
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

// ToggleSelected flips the selection state of the value at the cursor, then
// clears the search filter so the user can immediately find the next value.
func (p *FilterPicker) ToggleSelected() {
	items := p.filteredValues()
	if p.cursor < 0 || p.cursor >= len(items) {
		return
	}
	target := items[p.cursor].value
	for i := range p.values {
		if p.values[i].value == target {
			p.values[i].selected = !p.values[i].selected
			if p.values[i].selected {
				p.seq++
				p.values[i].selectedAt = p.seq
			} else {
				p.values[i].selectedAt = 0
			}
			break
		}
	}
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
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

	if p.loading {
		content := lipgloss.JoinVertical(lipgloss.Left,
			mutedStyle.Render("  Loading values..."),
		)
		return lipgloss.NewStyle().
			Width(p.width-2).
			Height(p.height-2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1).
			Render(content)
	}

	prompt := renderPalettePrompt(p.filter)

	items := p.filteredValues()

	maxVisible := p.height - 3 // border(2) + prompt(1)
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
		if p.filter != "" && i != p.cursor {
			name = highlightMatches(item.value, item.matchIdx)
		}

		tick := ""
		if item.selected {
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

	body := prompt + "\n" + strings.Join(rows, "\n")

	panel := lipgloss.NewStyle().
		Width(p.width - 2).
		Height(p.height - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(body)

	return panel
}
