package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
// border is the lipgloss style whose Foreground is used to draw the box
// borders; callers pass fieldBoxBorder(focused) for the usual focus colouring,
// or a red style to signal a failed test field (connection / provider form).
// Passing test fields keep the usual focus border — only the ✓ marker marks OK.
// This is the single source of truth used by both the inspector and the
// connection form so their field rendering never drifts.
func renderFieldBox(labelStr, markerStr, valueContent string, contentW int, border lipgloss.Style) string {
	if contentW < 5 {
		contentW = 5
	}
	valueWidth := contentW - 4
	if valueWidth < 1 {
		valueWidth = 1
	}
	borderWidth := valueWidth + 2
	bs := border

	// Label line: " " + label + pad + marker + " ", padded to contentW.
	// When the label/marker don't fit (narrow inspector), drop the marker and
	// truncate the label rather than letting pad=1 push past contentW — that
	// overflow widens the workspace by a column and the terminal wraps a line.
	labelW := lipgloss.Width(labelStr)
	markerW := lipgloss.Width(markerStr)
	pad := contentW - 2 - labelW - markerW
	if pad < 1 {
		markerStr = ""
		markerW = 0
		labelBudget := contentW - 2
		if labelBudget < 1 {
			labelBudget = 1
		}
		if labelW > labelBudget {
			labelStr = ansi.Truncate(labelStr, labelBudget, "…")
			labelW = lipgloss.Width(labelStr)
		}
		pad = contentW - 2 - labelW
		if pad < 0 {
			pad = 0
		}
	}
	labelLine := " " + labelStr + strings.Repeat(" ", pad) + markerStr + " "
	if w := lipgloss.Width(labelLine); w < contentW {
		labelLine += strings.Repeat(" ", contentW-w)
	} else if w > contentW {
		labelLine = ansi.Truncate(labelLine, contentW, "…")
	}

	top := bs.Render("┌" + strings.Repeat("─", borderWidth) + "┐")
	bot := bs.Render("└" + strings.Repeat("─", borderWidth) + "┘")

	var mids []string
	for _, line := range strings.Split(valueContent, "\n") {
		mids = append(mids, bs.Render("│ ")+line+bs.Render(" │"))
	}
	return labelLine + "\n" + top + "\n" + strings.Join(mids, "\n") + "\n" + bot
}
