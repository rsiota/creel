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
				{"d", "schema panel"},
				{"a", "add column"},
				{"T", "truncate table"},
				{"/", "filter tables"},
			},
		},
		{
			title: "Schema Panel",
			bindings: []helpBinding{
				{"j/k, ↑/↓", "move"},
				{"/", "filter columns"},
				{"a", "add column"},
				{"enter", "column actions"},
				{"esc", "close / back"},
			},
		},
		{
			title: "Column Edit",
			bindings: []helpBinding{
				{"enter", "run"},
				{"esc", "cancel"},
				{"tab", "next field"},
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

	// Build two columns of sections to keep the overlay compact.
	half := (len(sections) + 1) / 2
	left := sections[:half]
	right := sections[half:]

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

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		renderCol(left),
		"    ",
		renderCol(right),
	)

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

	// Constrain to terminal size with a max width.
	maxW := h.width - 6
	if maxW < 40 {
		maxW = 40
	}
	panel := lipgloss.NewStyle().
		Width(maxW).
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
