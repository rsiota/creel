package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// modalListWidth is the exterior cell width shared by the assistant-panel
// modal pickers (provider picker, model browser). It matches the small confirm
// pickers (truncate/drop table) so they line up on screen.
const modalListWidth = 46

// renderModalList renders the standard assistant-panel list frame: a rounded,
// primary-bordered box with the cursor row highlighted like the ctrl+p command
// palette (full-width primary background, inverted text, "❯" chevron). Rows
// longer than the inner width are truncated with "…" (via truncateCell, which
// also pads to the full width so the highlight fills the row). At most maxRows
// rows are shown, centred on the cursor, so long lists (a provider's /models
// response) stay on screen. A cursor of -1 renders no highlighted row, used for
// the model browser's non-selectable loading/error states.
func renderModalList(rows []string, cursor, width, maxRows int) string {
	inner := width - 2 // Padding(0,1) accounts for the 1-cell side padding
	if maxRows <= 0 || maxRows > len(rows) {
		maxRows = len(rows)
	}
	// Window the rows so the cursor stays roughly centred; clamped to bounds.
	start := cursor - maxRows/2
	if start < 0 {
		start = 0
	}
	if end := start + maxRows; end > len(rows) {
		end = len(rows)
		start = end - maxRows
		if start < 0 {
			start = 0
		}
	}
	out := make([]string, 0, maxRows)
	for i := start; i < len(rows) && len(out) < maxRows; i++ {
		if i == cursor {
			row := truncateCell("❯ "+rows[i], inner)
			out = append(out, lipgloss.NewStyle().
				Background(colorPrimary).Foreground(colorBg).Render(row))
		} else {
			row := truncateCell("  "+rows[i], inner)
			out = append(out, lipgloss.NewStyle().Foreground(colorFg).Render(row))
		}
	}
	body := strings.Join(out, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Width(width).
		Render(body)
}
