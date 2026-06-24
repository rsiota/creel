package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpBinding is a single key/description pair shown in the help overlay.
type helpBinding struct {
	key  string
	desc string
}

// helpSection groups related bindings under a heading.
type helpSection struct {
	title    string
	bindings []helpBinding
}

// HelpPanel renders a full-screen overlay listing every keybinding,
// grouped by context. Toggled with "?".
type HelpPanel struct {
	visible bool
	width   int
	height  int
}

// NewHelpPanel creates a hidden help panel.
func NewHelpPanel() HelpPanel {
	return HelpPanel{}
}

// Toggle shows or hides the help panel.
func (h *HelpPanel) Toggle() { h.visible = !h.visible }

// Show forces the panel visible.
func (h *HelpPanel) Show() { h.visible = true }

// Hide forces the panel hidden.
func (h *HelpPanel) Hide() { h.visible = false }

// IsVisible reports whether the help panel is shown.
func (h HelpPanel) IsVisible() bool { return h.visible }

// SetSize stores the terminal dimensions for centering/scaling the overlay.
func (h *HelpPanel) SetSize(width, height int) {
	h.width = width
	h.height = height
}

// helpSections returns the static list of keybinding groups.
// Keep descriptions short so the panel fits on small terminals.
func helpSections() []helpSection {
	return []helpSection{
		{
			title: "Global",
			bindings: []helpBinding{
				{"ctrl+e / \\", "run query"},
				{"ctrl+r", "clear editor"},
				{"ctrl+t", "switch connection"},
				{"ctrl+b", "browse databases (MySQL)"},
				{"ctrl+y", "query history"},
				{"ctrl+g", "bookmarks"},
				{"B", "bookmark current query"},
				{"ctrl+o", "toggle inspector"},
				{"ctrl+h/j/k/l", "move focus"},
				{"tab / shift+tab", "cycle focus"},
				{"ctrl+d / ctrl+u", "next / prev page"},
				{"?", "toggle this help"},
				{"q / ctrl+q / ctrl+c", "quit (not while editing)"},
			},
		},
		{
			title: "Sidebar (Tables)",
			bindings: []helpBinding{
				{"j/k, ↑/↓", "move"},
				{"g g / G", "top / bottom"},
				{"space", "expand columns"},
				{"enter / s", "select * from table"},
				{"d", "edit schema (grid)"},
				{"a", "add column"},
				{"r", "rename table"},
				{"T", "truncate table"},
				{"D", "drop table"},
				{"N", "new table (grid editor)"},
				{"X", "export database"},
				{"I", "import SQL dump"},
				{"/", "filter tables"},
			},
		},
		{
			title: "Schema Editor",
			bindings: []helpBinding{
				{"h/j/k/l", "move cell"},
				{"e / i", "edit cell"},
				{"o", "add column"},
				{"enter", "apply change"},
				{"dd", "drop column"},
				{"esc", "done"},
			},
		},
		{
			title: "Export Picker",
			bindings: []helpBinding{
				{"j/k", "move"},
				{"space", "toggle table"},
				{"a", "select all"},
				{"n", "select none"},
				{"enter", "export"},
				{"esc", "cancel"},
			},
		},
		{
			title: "History Panel",
			bindings: []helpBinding{
				{"j/k", "move"},
				{"enter", "load query into editor"},
				{"b", "bookmark selected query"},
				{"D", "clear history"},
				{"esc", "close"},
			},
		},
		{
			title: "Bookmarks Panel",
			bindings: []helpBinding{
				{"j/k", "move"},
				{"enter", "load query into editor"},
				{"d", "delete bookmark"},
				{"D", "clear bookmarks"},
				{"esc", "close"},
			},
		},
		{
			title: "Table Designer",
			bindings: []helpBinding{
				{"h/j/k/l", "move cell"},
				{"e / i", "edit cell"},
				{"o / O", "add row below / above"},
				{"dd", "remove row"},
				{"enter", "create table"},
				{"esc", "cancel"},
			},
		},
		{
			title: "Editor (Vim)",
			bindings: []helpBinding{
				{"i/a/o/A/O", "insert mode"},
				{"esc", "normal mode"},
				{"h/j/k/l, w/b", "move"},
				{"x / dd / dw / D", "delete"},
				{"y / p", "yank / paste"},
				{"ctrl+n", "autocomplete"},
			},
		},
		{
			title: "Results",
			bindings: []helpBinding{
				{"h/j/k/l", "move cursor"},
				{"g g / G", "top / bottom"},
				{"y y", "copy cell"},
				{"g d", "follow foreign key"},
				{"g b", "go back"},
				{"/", "filter column values"},
				{"*", "keep rows equal to cursor cell"},
				{"!", "hide rows equal to cursor cell"},
				{"space", "toggle row mark"},
				{"F", "filter by marked rows"},
				{"C", "clear marks"},
				{"dd", "delete marked or cursor row"},
				{"V", "visual mode (select range)"},
				{"u", "undo last filter"},
				{"c", "clear filters"},
				{"o", "sort column"},
				{"g s", "column stats"},
				{":", "jump to column"},
				{"H", "hide column"},
				{"g H", "show all columns"},
				{"v", "column visibility"},
				{"g /", "regex search"},
				{"n / N", "next / prev match"},
				{"x", "export to CSV"},
				{"e", "edit cell"},
				{"ctrl+s", "save edits"},
				{"A", "insert new row"},
				{"D", "discard edits"},
			},
		},
		{
			title: "Inspector",
			bindings: []helpBinding{
				{"j/k", "move field"},
				{"/", "filter fields"},
				{"e", "edit field"},
				{"ctrl+s", "save"},
				{"ctrl+o", "close"},
			},
		},
	}
}

// View renders the help overlay, sized to fit the terminal.
func (h HelpPanel) View() string {
	if !h.visible {
		return ""
	}

	sections := helpSections()

	// Compute column width: longest key in each section, padded.
	keyWidth := 0
	for _, s := range sections {
		for _, b := range s.bindings {
			if len(b.key) > keyWidth {
				keyWidth = len(b.key)
			}
		}
	}

	// sectionHeight returns the rendered line count of a single section
	// (title + one line per binding).
	sectionHeight := func(s helpSection) int { return 1 + len(s.bindings) }

	// Available content height inside the panel (terminal minus border(2),
	// padding(2), header(1), blank(1), footer(1), blank(1)).
	availH := h.height - 8
	if availH < 20 {
		availH = 20
	}

	// Greedily distribute sections across columns so no column exceeds
	// availH. This avoids the tall Results section dominating one column.
	var columns [][]helpSection
	colHeights := []int{0}
	for _, s := range sections {
		sh := sectionHeight(s) + 2 // section + blank separator
		// Find the shortest column that can still fit this section.
		bestCol := 0
		for ci := range colHeights {
			if colHeights[ci] < colHeights[bestCol] {
				bestCol = ci
			}
		}
		// If the shortest column is full and this section won't fit, start a
		// new column (unless we're already on the last one).
		if colHeights[bestCol]+sh > availH && colHeights[bestCol] > 0 {
			columns = append(columns, nil)
			colHeights = append(colHeights, 0)
			bestCol = len(colHeights) - 1
		}
		if bestCol >= len(columns) {
			columns = append(columns, nil)
		}
		columns[bestCol] = append(columns[bestCol], s)
		colHeights[bestCol] += sh
	}

	renderCol := func(cols []helpSection) string {
		var b strings.Builder
		for i, s := range cols {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(titleStyle.Render(s.title))
			b.WriteString("\n")
			for _, bd := range s.bindings {
				key := lipgloss.NewStyle().Foreground(colorLabel).Render(bd.key)
				pad := strings.Repeat(" ", keyWidth-len(bd.key))
				desc := mutedStyle.Render(bd.desc)
				b.WriteString("  " + key + pad + "  " + desc + "\n")
			}
		}
		return b.String()
	}

	// Join all columns horizontally.
	colStrs := make([]string, len(columns))
	for i, c := range columns {
		colStrs[i] = renderCol(c)
	}
	seps := make([]string, len(colStrs)-1)
	for i := range seps {
		seps[i] = "    "
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, joinInterleaved(colStrs, seps)...)

	header := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Render("Keybindings")
	footer := mutedStyle.Render("press ? or esc to close")

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		body,
		"",
		footer,
	)

	// Constrain to terminal size.
	maxW := h.width - 4
	if maxW < 40 {
		maxW = 40
	}
	maxH := h.height - 2
	if maxH < 10 {
		maxH = 10
	}
	panel := lipgloss.NewStyle().
		Width(maxW).
		Height(maxH).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(h.width, h.height,
		lipgloss.Center, lipgloss.Center,
		panel,
		lipgloss.WithWhitespaceChars(" "),
	)
}

// joinInterleaved interleaves items and separators into a single slice,
// e.g. [a,b,c] + [s1,s2] → [a,s1,b,s2,c].
func joinInterleaved(items, seps []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items)+len(seps))
	for i, item := range items {
		result = append(result, item)
		if i < len(seps) {
			result = append(result, seps[i])
		}
	}
	return result
}
