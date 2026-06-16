package ui

import (
	"fmt"
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
type DatabasePicker struct {
	databases []string
	filter    string
	cursor    int
	scrollRow int
	visible   bool
	width     int
	height    int
	// mustChoose indicates that the user connected without a database; cancelling
	// returns to the connection list instead of staying in the workspace.
	mustChoose bool
}

// NewDatabasePicker creates a new picker.
func NewDatabasePicker() DatabasePicker {
	return DatabasePicker{}
}

// Show populates and displays the picker.
func (p *DatabasePicker) Show(databases []string, mustChoose bool) {
	p.databases = databases
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
	p.visible = true
	p.mustChoose = mustChoose
}

// Hide hides the picker.
func (p *DatabasePicker) Hide() {
	p.visible = false
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
}

// IsVisible returns whether the picker is shown.
func (p DatabasePicker) IsVisible() bool {
	return p.visible
}

// MustChoose returns true when the picker was opened on connect (no DB selected).
func (p DatabasePicker) MustChoose() bool {
	return p.mustChoose
}

// SetSize sets the dimensions of the picker.
func (p *DatabasePicker) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// filteredDatabases returns the fuzzy-filtered, sorted list of databases.
func (p DatabasePicker) filteredDatabases() []dbItem {
	type scored struct {
		item  dbItem
		score int
	}
	var results []scored
	for _, d := range p.databases {
		idx, score := fuzzyMatch(p.filter, d)
		if idx != nil || p.filter == "" {
			results = append(results, scored{dbItem{name: d, matchIdx: idx}, score})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score < results[j].score
		}
		return results[i].item.name < results[j].item.name
	})
	items := make([]dbItem, len(results))
	for i, r := range results {
		items[i] = r.item
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

func (p *DatabasePicker) adjustScroll() {
	maxVisible := p.height - 5
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

	title := titleStyle.Render("Select Database")

	maxVisible := p.height - 4
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
		if p.filter != "" {
			name = highlightMatches(item.name, item.matchIdx)
		}

		marker := " "
		if i == p.cursor {
			marker = lipgloss.NewStyle().Foreground(colorPrimary).Render("▶")
		}

		line := fmt.Sprintf("%s  %s", marker, name)
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

	// Pin list to a fixed height so the filter line sits at the bottom,
	// matching the sidebar layout.
	listStyled := lipgloss.NewStyle().
		Height(maxVisible).
		Render(strings.Join(rows, "\n"))

	filterLine := lipgloss.NewStyle().Foreground(colorPrimary).Render("/"+p.filter) +
		lipgloss.NewStyle().Foreground(colorAccent).Render("▏")

	scrollInfo := ""
	if len(items) > maxVisible {
		scrollInfo = mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", p.scrollRow+1, end, len(items)))
	}

	footer := filterLine + "  " + scrollInfo

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		listStyled,
		footer,
	)

	panel := lipgloss.NewStyle().
		Width(p.width - 2).
		Height(p.height - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Render(content)

	return panel
}
