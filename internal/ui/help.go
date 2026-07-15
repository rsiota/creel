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

	// The Commands block (rendered from exCommands) sits below the keybinding
	// columns; reserve its height plus a blank separator so the columns don't
	// crowd it out.
	cmdCount := len(exCommands())
	cmdReserved := commandsBlockHeight(cmdCount) + 1

	// sectionHeight returns the rendered line count of a single section
	// (title + one line per binding).
	sectionHeight := func(s Section) int { return 1 + len(s.Items) }

	// Available content height inside the panel (terminal minus border(2),
	// padding(2), header(1), blank(1), footer(1), blank(1), and the reserved
	// Commands block).
	availH := h.height - 8 - cmdReserved
	if availH < 16 {
		availH = 16
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
	// Constrain to terminal size.
	maxW := h.width - 4
	if maxW < 40 {
		maxW = 40
	}
	cmdBlock := renderCommandsBlock(maxW)

	body := lipgloss.JoinHorizontal(lipgloss.Top, joinInterleaved(colStrs, seps)...)

	header := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Render("Keybindings")
	footer := mutedStyle.Render("press ? or esc to close")

	layers := []string{header, "", body}
	if cmdBlock != "" {
		layers = append(layers, "", cmdBlock)
	}
	layers = append(layers, "", footer)
	content := lipgloss.JoinVertical(lipgloss.Left, layers...)

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

// renderCommandsBlock renders the ":" commands (from exCommands) as a titled
// two-column block for the help overlay: "usage   description" per row. It is
// independent of the keybinding column layout (sized to key displays, far
// narrower than command usages) so neither widens the other. maxWidth bounds
// the block; descs are truncated to fit. Returns "" when there is no room.
func renderCommandsBlock(maxWidth int) string {
	cmds := exCommands()
	if len(cmds) == 0 || maxWidth < 40 {
		return ""
	}
	const descW = 30
	usageW := 0
	for _, c := range cmds {
		if w := runeLen(c.usage); w > usageW {
			usageW = w
		}
	}
	half := (len(cmds) + 1) / 2
	groups := [][]exCmdSpec{cmds[:half], cmds[half:]}
	renderCol := func(items []exCmdSpec) string {
		var b strings.Builder
		for _, c := range items {
			usage := lipgloss.NewStyle().Foreground(colorLabel).Render(c.usage)
			usage += strings.Repeat(" ", usageW-runeLen(c.usage))
			desc := lipgloss.NewStyle().Foreground(colorFg).Render(truncateRunes(c.desc, descW))
			b.WriteString("  " + usage + "  " + desc + "\n")
		}
		return b.String()
	}
	left := renderCol(groups[0])
	right := renderCol(groups[1])
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		joinInterleaved([]string{left, right}, []string{"    "})...)
	title := titleStyle.Render("Commands")
	return lipgloss.JoinVertical(lipgloss.Left, title, body)
}

// truncateRunes clips s to max visible runes with a trailing "…" when clipped.
// exCmdSpec.desc values are plain (unstyled) strings, so rune slicing is safe.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runeLen(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) > max-1 {
		r = r[:max-1]
	}
	return string(r) + "…"
}

// commandsBlockHeight returns the rendered height of renderCommandsBlock for n
// commands (title + ceil(n/2) rows across two columns), used to reserve space.
func commandsBlockHeight(n int) int {
	if n == 0 {
		return 0
	}
	return 1 + (n+1)/2
}
