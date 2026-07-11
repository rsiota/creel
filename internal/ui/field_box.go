package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// linesPerField is the vertical space each bordered field occupies:
// label line + top border + value line + bottom border. It is shared by
// the inspector and the connection form so both render fields identically.
const linesPerField = 4

// fieldBoxBorder returns the border style for a field box: the primary colour
// when focused, the table-grid colour otherwise.
func fieldBoxBorder(focused bool) lipgloss.Style {
	if focused {
		return lipgloss.NewStyle().Foreground(colorPrimary)
	}
	return lipgloss.NewStyle().Foreground(colorBorder)
}

// renderFieldBox renders a single field as a linesPerField-line block: a label
// line (labelStr left-aligned, markerStr right-aligned), a top border, one or
// more value lines wrapping valueContent, and a bottom border.
//
// contentW is the total content width available; the value-box interior is
// contentW-4 columns. labelStr and markerStr are already-styled (markerStr may
// be ""). valueContent is one or more newline-joined, already-styled strings,
// each exactly (contentW-4) columns wide, that become the boxed value line(s).
// This is the single source of truth used by both the inspector and the
// connection form so their field rendering never drifts.
func renderFieldBox(labelStr, markerStr, valueContent string, contentW int, focused bool) string {
	valueWidth := contentW - 4
	if valueWidth < 1 {
		valueWidth = 1
	}
	borderWidth := valueWidth + 2
	bs := fieldBoxBorder(focused)

	// Label line: " " + label + pad + marker + " ", padded to contentW.
	labelW := lipgloss.Width(labelStr)
	markerW := lipgloss.Width(markerStr)
	pad := contentW - 2 - labelW - markerW
	if pad < 1 {
		pad = 1
	}
	labelLine := " " + labelStr + strings.Repeat(" ", pad) + markerStr + " "

	top := bs.Render("┌" + strings.Repeat("─", borderWidth) + "┐")
	bot := bs.Render("└" + strings.Repeat("─", borderWidth) + "┘")

	var mids []string
	for _, line := range strings.Split(valueContent, "\n") {
		mids = append(mids, bs.Render("│ ")+line+bs.Render(" │"))
	}
	return labelLine + "\n" + top + "\n" + strings.Join(mids, "\n") + "\n" + bot
}
