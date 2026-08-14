package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// chartPoint is one sample on a line chart, sorted by x.
type chartPoint struct {
	x, y   float64
	xLabel string
}

// buildLineSeries walks the current result page into x/y points. Both columns
// must parse as numbers; NULL / non-numeric cells are skipped. Points are
// sorted by x ascending. Negative values are kept (unlike bar charts).
func buildLineSeries(r ResultsTable, xCol, yCol int) (pts []chartPoint, skipped int) {
	n := r.NumRows()
	pts = make([]chartPoint, 0, n)
	for i := 0; i < n; i++ {
		xr := r.RowValue(i, xCol)
		yr := r.RowValue(i, yCol)
		x, xok := parseFloat(xr)
		y, yok := parseFloat(yr)
		if !xok || !yok || math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			skipped++
			continue
		}
		label := strings.TrimSpace(xr)
		if label == "" || label == "NULL" {
			label = formatChartValue(x)
		}
		pts = append(pts, chartPoint{x: x, y: y, xLabel: label})
	}
	sort.SliceStable(pts, func(i, j int) bool {
		if pts[i].x != pts[j].x {
			return pts[i].x < pts[j].x
		}
		return pts[i].y < pts[j].y
	})
	return pts, skipped
}

func (c ChartPanel) lineBodyLines(inner int) []string {
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	axis := lipgloss.NewStyle().Foreground(colorLabel)
	lineSt := lipgloss.NewStyle().Foreground(colorPrimary)
	pointSt := lipgloss.NewStyle().Foreground(colorPrimary)
	selSt := lipgloss.NewStyle().Foreground(colorPrimary).Background(colorCursorRow)

	if len(c.points) == 0 {
		return []string{muted.Render(truncateCell("no numeric values to chart", inner))}
	}

	h := c.contentHeight() - c.footerLines()
	if h < 4 {
		h = 4
	}
	// y-axis labels + " ┤", x-axis rule, x labels
	yW := 0
	minY, maxY := c.points[0].y, c.points[0].y
	minX, maxX := c.points[0].x, c.points[0].x
	for _, p := range c.points {
		if p.y < minY {
			minY = p.y
		}
		if p.y > maxY {
			maxY = p.y
		}
		if p.x < minX {
			minX = p.x
		}
		if p.x > maxX {
			maxX = p.x
		}
	}
	for _, v := range []float64{minY, maxY} {
		if w := lipgloss.Width(formatChartValue(v)); w > yW {
			yW = w
		}
	}
	if yW < 3 {
		yW = 3
	}
	axisGutter := yW + 2 // "┤ " after the number
	plotW := inner - axisGutter
	plotH := h - 2 // rule + x labels
	if plotW < 4 {
		plotW = 4
	}
	if plotH < 2 {
		plotH = 2
	}

	grid := make([][]rune, plotH)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", plotW))
	}

	colOf := func(x float64) int {
		if plotW <= 1 || maxX == minX {
			return 0
		}
		c := int(math.Round((x - minX) / (maxX - minX) * float64(plotW-1)))
		if c < 0 {
			c = 0
		}
		if c >= plotW {
			c = plotW - 1
		}
		return c
	}
	rowOf := func(y float64) int {
		if plotH <= 1 || maxY == minY {
			return plotH / 2
		}
		t := (y - minY) / (maxY - minY)
		r := (plotH - 1) - int(math.Round(t*float64(plotH-1)))
		if r < 0 {
			r = 0
		}
		if r >= plotH {
			r = plotH - 1
		}
		return r
	}

	for i := 1; i < len(c.points); i++ {
		x0, y0 := colOf(c.points[i-1].x), rowOf(c.points[i-1].y)
		x1, y1 := colOf(c.points[i].x), rowOf(c.points[i].y)
		strokeLine(grid, x0, y0, x1, y1)
	}
	for _, p := range c.points {
		grid[rowOf(p.y)][colOf(p.x)] = '●'
	}

	selCol, selRow := -1, -1
	if c.cursor >= 0 && c.cursor < len(c.points) {
		selCol = colOf(c.points[c.cursor].x)
		selRow = rowOf(c.points[c.cursor].y)
	}

	out := make([]string, 0, h)
	for row := 0; row < plotH; row++ {
		label := strings.Repeat(" ", yW)
		tick := "│ "
		switch row {
		case 0:
			label = padLeft(formatChartValue(maxY), yW)
			tick = "┤ "
		case plotH - 1:
			label = padLeft(formatChartValue(minY), yW)
			tick = "┤ "
		case plotH / 2:
			if plotH >= 5 && minY != maxY {
				mid := minY + (maxY-minY)/2
				label = padLeft(formatChartValue(mid), yW)
				tick = "┤ "
			}
		}
		var b strings.Builder
		b.WriteString(axis.Render(label + tick))
		for col, ch := range grid[row] {
			s := string(ch)
			st := muted
			switch {
			case col == selCol && row == selRow && ch == '●':
				st = selSt
			case ch == '●':
				st = pointSt
			case ch != ' ':
				st = lineSt
			}
			if col == selCol && ch == ' ' {
				st = lipgloss.NewStyle().Background(colorCursorRow)
			}
			b.WriteString(st.Render(s))
		}
		out = append(out, b.String())
	}

	out = append(out, axis.Render(padLeft("", yW)+"└"+strings.Repeat("─", plotW)))
	out = append(out, muted.Render(strings.Repeat(" ", axisGutter)+c.lineXAxis(plotW)))

	if c.skipped > 0 {
		out = append(out, muted.Render(truncateCell(
			fmt.Sprintf("skipped %d non-numeric/NULL row%s", c.skipped, pluralIf(c.skipped != 1, "s")),
			inner)))
	}
	return out
}

func strokeLine(grid [][]rune, x0, y0, x1, y1 int) {
	h := len(grid)
	if h == 0 {
		return
	}
	w := len(grid[0])
	dx := absInt(x1 - x0)
	dy := absInt(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	x, y := x0, y0
	for {
		if x >= 0 && x < w && y >= 0 && y < h && grid[y][x] == ' ' {
			var g rune = '─'
			if dx == 0 {
				g = '│'
			} else if dy != 0 {
				// grid y grows downward
				if sx*sy > 0 {
					g = '╲'
				} else {
					g = '╱'
				}
			}
			grid[y][x] = g
		}
		if x == x1 && y == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x += sx
		}
		if e2 < dx {
			err += dx
			y += sy
		}
	}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (c ChartPanel) lineXAxis(plotW int) string {
	left := c.points[0].xLabel
	right := c.points[len(c.points)-1].xLabel
	if c.cursor < 0 || c.cursor >= len(c.points) {
		return spreadEnds(left, right, plotW)
	}
	p := c.points[c.cursor]
	mid := p.xLabel + " " + formatChartValue(p.y)
	need := lipgloss.Width(left) + lipgloss.Width(mid) + lipgloss.Width(right) + 2
	if need > plotW {
		return truncateCell(mid, plotW)
	}
	gap := (plotW - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)) / 2
	if gap < 1 {
		gap = 1
	}
	s := left + strings.Repeat(" ", gap) + mid
	pad := plotW - lipgloss.Width(s) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	s += strings.Repeat(" ", pad) + right
	if lipgloss.Width(s) > plotW {
		return truncateCell(mid, plotW)
	}
	return s
}

func spreadEnds(left, right string, width int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 >= width {
		return truncateCell(left+" "+right, width)
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}
