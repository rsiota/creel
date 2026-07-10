package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderBoxTable renders headers and rows as a box-bordered grid whose column
// widths fit the widest cell (or header) in each column. It is the shared
// "table" renderer used by the read-only tabs of the schema editor so that
// indexes, foreign keys, and triggers share the same look as the editable
// columns grid and the results table.
//
// maxWidth caps the total grid width; columns that would overflow are
// truncated. An empty header string still allocates a column (useful for flag
// columns like the indexes' "unique" slot).
func renderBoxTable(headers []string, rows [][]string, maxWidth int) string {
	ncols := len(headers)
	if ncols == 0 {
		return ""
	}

	// Natural column widths from headers and cells.
	widths := make([]int, ncols)
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, r := range rows {
		for i := 0; i < ncols; i++ {
			w := 0
			if i < len(r) {
				w = lipgloss.Width(r[i])
			}
			if w > widths[i] {
				widths[i] = w
			}
		}
	}

	// Each cell is rendered as " " + content + " " (1 space padding each side),
	// so a column's box width = widths[i] + 2. Sum + separators must fit
	// maxWidth when set.
	if maxWidth > 0 {
		renderBoxTableFitWidth(widths, maxWidth)
	}

	border := lipgloss.NewStyle().Foreground(colorBorder)
	headStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)

	pad := func(s string, w int) string {
		if lipgloss.Width(s) >= w {
			return s
		}
		return s + strings.Repeat(" ", w-lipgloss.Width(s))
	}
	trunc := func(s string, w int) string {
		return truncateSidebarLine(s, w) // strips/measures ANSI + ellipsizes
	}

	var b strings.Builder

	// Top border.
	b.WriteString(border.Render("┌"))
	for j := 0; j < ncols; j++ {
		b.WriteString(border.Render(strings.Repeat("─", widths[j]+2)))
		if j < ncols-1 {
			b.WriteString(border.Render("┬"))
		}
	}
	b.WriteString(border.Render("┐"))
	b.WriteString("\n")

	// Header row.
	b.WriteString(border.Render("│"))
	for j := 0; j < ncols; j++ {
		cell := " " + headStyle.Render(pad(headers[j], widths[j])) + " "
		b.WriteString(cell)
		b.WriteString(border.Render("│"))
	}
	b.WriteString("\n")

	// Header/data separator.
	b.WriteString(border.Render("├"))
	for j := 0; j < ncols; j++ {
		b.WriteString(border.Render(strings.Repeat("─", widths[j]+2)))
		if j < ncols-1 {
			b.WriteString(border.Render("┼"))
		}
	}
	b.WriteString(border.Render("┤"))
	b.WriteString("\n")

	// Data rows.
	for _, r := range rows {
		b.WriteString(border.Render("│"))
		for j := 0; j < ncols; j++ {
			val := ""
			if j < len(r) {
				val = r[j]
			}
			val = trunc(val, widths[j])
			rendered := " " + pad(val, widths[j]) + " "
			b.WriteString(rendered)
			b.WriteString(border.Render("│"))
		}
		b.WriteString("\n")
	}

	// Bottom border.
	b.WriteString(border.Render("└"))
	for j := 0; j < ncols; j++ {
		b.WriteString(border.Render(strings.Repeat("─", widths[j]+2)))
		if j < ncols-1 {
			b.WriteString(border.Render("┴"))
		}
	}
	b.WriteString(border.Render("┘"))

	return b.String()
}

// renderBoxTableFitWidth shrinks column widths in place so the rendered grid
// (sum of (width+2) + ncols-1 separators + 2 outer borders) fits maxWidth. It
// trims the widest columns first and preserves a minimum width of 3 per column.
func renderBoxTableFitWidth(widths []int, maxWidth int) {
	ncols := len(widths)
	const minW = 3
	for {
		total := 0
		for _, w := range widths {
			total += w + 2
		}
		total += ncols - 1 // inner separators
		total += 2         // outer borders
		if total <= maxWidth {
			return
		}
		// Find the widest column and shrink it.
		widest := 0
		for i := 1; i < ncols; i++ {
			if widths[i] > widths[widest] {
				widest = i
			}
		}
		if widths[widest] <= minW {
			return // can't shrink further without dropping below the floor
		}
		widths[widest]--
	}
}
