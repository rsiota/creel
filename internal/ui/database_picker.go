package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// dbItem is a single database entry with fuzzy match metadata.
type dbItem struct {
	name     string
	matchIdx []int
}

// DatabasePicker is a fuzzy-search overlay for selecting a database.
// It has two modes like the connections picker:
//   - Filter mode (default): typing filters the list, esc → normal mode.
//   - Normal mode: j/k navigate, N creates, D drops, / re-enters filter.
type DatabasePicker struct {
	databases []string
	filter    string
	cursor    int
	scrollRow int
	visible   bool
	width     int
	height    int
	filtering bool
	// mustChoose indicates that the user connected without a database; cancelling
	// returns to the connection list instead of staying in the workspace.
	mustChoose bool
}

// NewDatabasePicker creates a new picker.
func NewDatabasePicker() DatabasePicker {
	return DatabasePicker{}
}

// Show populates and displays the picker in filter mode.
func (p *DatabasePicker) Show(databases []string, mustChoose bool) {
	p.databases = databases
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
	p.visible = true
	p.mustChoose = mustChoose
	p.filtering = true
}

// Hide hides the picker.
func (p *DatabasePicker) Hide() {
	p.visible = false
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
	p.filtering = false
}

// IsVisible returns whether the picker is shown.
func (p DatabasePicker) IsVisible() bool {
	return p.visible
}

// MustChoose returns true when the picker was opened on connect (no DB selected).
func (p DatabasePicker) MustChoose() bool {
	return p.mustChoose
}

// Filtering returns whether the picker is in filter mode.
func (p DatabasePicker) Filtering() bool { return p.filtering }

// StartFiltering enters filter mode.
func (p *DatabasePicker) StartFiltering() {
	p.filtering = true
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
}

// StopFiltering exits filter mode, clearing the filter.
func (p *DatabasePicker) StopFiltering() {
	p.filtering = false
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
}

// SetSize sets the dimensions of the picker.
func (p *DatabasePicker) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// filteredDatabases returns the fuzzy-filtered, sorted list of databases.
func (p DatabasePicker) filteredDatabases() []dbItem {
	if p.filter == "" {
		sorted := make([]string, len(p.databases))
		copy(sorted, p.databases)
		sort.Strings(sorted)
		items := make([]dbItem, len(sorted))
		for i, d := range sorted {
			items[i] = dbItem{name: d}
		}
		return items
	}
	ranked := fuzzyRank(p.filter, p.databases,
		func(d string) string { return d },
		func(a, b fuzzyResult[string]) bool { return a.Item < b.Item })
	items := make([]dbItem, len(ranked))
	for i, r := range ranked {
		items[i] = dbItem{name: r.Item, matchIdx: r.MatchIdx}
	}
	return items
}

// SelectedDatabase returns the database at the cursor (from the filtered list).
func (p DatabasePicker) SelectedDatabase() string {
	items := p.filteredDatabases()
	if len(items) == 0 || p.cursor < 0 || p.cursor >= len(items) {
		return ""
	}
	return items[p.cursor].name
}

// CursorUp moves the selection up.
func (p *DatabasePicker) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.adjustScroll()
	}
}

// CursorDown moves the selection down.
func (p *DatabasePicker) CursorDown() {
	items := p.filteredDatabases()
	if p.cursor < len(items)-1 {
		p.cursor++
		p.adjustScroll()
	}
}

// SetCursor sets the cursor position directly.
func (p *DatabasePicker) SetCursor(i int) {
	items := p.filteredDatabases()
	if len(items) == 0 {
		p.cursor = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(items) {
		i = len(items) - 1
	}
	p.cursor = i
	p.adjustScroll()
}

// ScrollRow returns the current scroll offset (for mouse coordinate mapping).
func (p DatabasePicker) ScrollRow() int {
	return p.scrollRow
}

func (p *DatabasePicker) adjustScroll() {
	maxVisible := p.maxVisibleItems()
	if p.cursor < p.scrollRow {
		p.scrollRow = p.cursor
	}
	if p.cursor >= p.scrollRow+maxVisible {
		p.scrollRow = p.cursor - maxVisible + 1
	}
}

func (p DatabasePicker) maxVisibleItems() int {
	max := p.height - 3 // border(2) + prompt(1)
	if max < 1 {
		max = 1
	}
	return max
}

// FilterAddChar appends a character to the filter.
func (p *DatabasePicker) FilterAddChar(ch string) {
	p.filter += ch
	p.cursor = 0
	p.scrollRow = 0
}

// FilterBackspace removes the last character from the filter.
func (p *DatabasePicker) FilterBackspace() {
	if len(p.filter) > 0 {
		p.filter = p.filter[:len(p.filter)-1]
		p.cursor = 0
		p.scrollRow = 0
	}
}

// View renders the picker overlay.
func (p DatabasePicker) View() string {
	if !p.visible {
		return ""
	}

	items := p.filteredDatabases()

	prompt := renderPalettePrompt(p.filter, p.filtering)

	maxVisible := p.maxVisibleItems()

	end := p.scrollRow + maxVisible
	if end > len(items) {
		end = len(items)
	}

	var rows []string
	for i := p.scrollRow; i < end; i++ {
		item := items[i]
		name := item.name
		if p.filtering && p.filter != "" {
			name = highlightMatches(item.name, item.matchIdx)
		}
		rowStyle := lipgloss.NewStyle().Foreground(colorFg).Padding(0, 1, 0, 2)
		if i == p.cursor {
			rowStyle = rowStyle.Bold(true)
		}
		rows = append(rows, rowStyle.Render(name))
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

	content := lipgloss.JoinVertical(lipgloss.Left,
		prompt,
		listStyled,
	)

	panel := lipgloss.NewStyle().
		Width(p.width - 2).
		Height(p.height - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)

	return panel
}
