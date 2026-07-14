package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpPanel renders a full-screen overlay listing every keybinding,
// grouped by context. Toggled with "?".
//
// The binding data lives in registry() (registry.go) — the single source of
// truth. This file only handles rendering.
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

// View renders the help overlay, sized to fit the terminal.
func (h HelpPanel) View() string {
	if !h.visible {
		return ""
	}

	sections := registry()

	// Compute column width: longest key in each section, padded.
	keyWidth := 0
	for _, s := range sections {
		for _, b := range s.Items {
			if w := runeLen(b.Display); w > keyWidth {
				keyWidth = w
			}
		}
	}

	// sectionHeight returns the rendered line count of a single section
	// (title + one line per binding).
	sectionHeight := func(s Section) int { return 1 + len(s.Items) }

	// Available content height inside the panel (terminal minus border(2),
	// padding(2), header(1), blank(1), footer(1), blank(1)).
	availH := h.height - 8
	if availH < 20 {
		availH = 20
	}

	// Greedily distribute sections across columns so no column exceeds
	// availH. This avoids the tall Results section dominating one column.
	var columns [][]Section
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

	renderCol := func(cols []Section) string {
		var b strings.Builder
		for i, s := range cols {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(titleStyle.Render(s.Title))
			b.WriteString("\n")
			for _, bd := range s.Items {
				key := lipgloss.NewStyle().Foreground(colorLabel).Render(bd.Display)
				pad := strings.Repeat(" ", keyWidth-runeLen(bd.Display))
				desc := lipgloss.NewStyle().Foreground(colorFg).Render(bd.Desc)
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
