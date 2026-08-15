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

const (
	brailleBase       rune = 0x2800
	lineChartPad           = 4
	lineChartPadRight      = 7
)

func clampChartPad(want, avail, minContent int) int {
	max := (avail - minContent) / 2
	if max < 0 {
		return 0
	}
	if want > max {
		return max
	}
	return want
}

func clampChartPadLR(left, right, avail, minContent int) (int, int) {
	extra := avail - minContent
	if extra <= 0 {
		return 0, 0
	}
	if left+right <= extra {
		return left, right
	}
	if left > extra {
		return extra, 0
	}
	return left, extra - left
}

// brailleBits maps a 2×4 cell (subX 0..1, subY 0..3, top-left origin) onto
// Unicode Braille dots 1–8.
var brailleBits = [4][2]byte{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

func (c ChartPanel) lineBodyLines(inner int) []string {
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	numSt := lipgloss.NewStyle().Foreground(colorFg)
	axisSt := lipgloss.NewStyle().Foreground(colorMuted)
	hairSt := lipgloss.NewStyle().Foreground(colorBorder)
	lineSt := lipgloss.NewStyle().Foreground(colorFg)
	selSt := lipgloss.NewStyle().Foreground(colorFg).Background(colorCursorRow)

	if len(c.points) == 0 {
		return []string{muted.Render(truncateCell("no numeric values to chart", inner))}
	}

	availH := c.contentHeight() - c.footerLines()
	padY := clampChartPad(lineChartPad, availH, 4)
	padL, padR := clampChartPadLR(lineChartPad, lineChartPadRight, inner, 10)
	h := availH - 2*padY
	if h < 4 {
		h = 4
	}
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
	plotW := inner - padL - padR - axisGutter
	plotH := h - 2 // rule + x labels
	if plotW < 4 {
		plotW = 4
	}
	if plotH < 2 {
		plotH = 2
	}

	grid := rasterBraille(plotW, plotH, c.points, minX, maxX, minY, maxY, c.kind != chartKindScatter)

	selCol, selRow := -1, -1
	if c.cursor >= 0 && c.cursor < len(c.points) {
		p := c.points[c.cursor]
		px, py := linePixelOf(p.x, p.y, minX, maxX, minY, maxY, plotW*2, plotH*4)
		selCol, selRow = px/2, py/4
		overlayCrosshair(grid, selCol, selRow)
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
		b.WriteString(numSt.Render(label) + axisSt.Render(tick))
		for col, ch := range grid[row] {
			s := string(ch)
			st := hairSt
			switch {
			case col == selCol && row == selRow && isBrailleCell(ch):
				st = selSt
			case isBrailleCell(ch):
				st = lineSt
			}
			b.WriteString(st.Render(s))
		}
		out = append(out, b.String())
	}

	out = append(out, axisSt.Render(padLeft("", yW)+"└"+strings.Repeat("─", plotW)))
	out = append(out, numSt.Render(strings.Repeat(" ", axisGutter)+c.lineXAxis(plotW)))

	leftPad := strings.Repeat(" ", padL)
	body := make([]string, 0, len(out)+2*padY+1)
	for i := 0; i < padY; i++ {
		body = append(body, "")
	}
	for _, line := range out {
		body = append(body, leftPad+line)
	}
	for i := 0; i < padY; i++ {
		body = append(body, "")
	}
	if c.skipped > 0 {
		body = append(body, muted.Render(truncateCell(
			fmt.Sprintf("skipped %d non-numeric/NULL row%s", c.skipped, pluralIf(c.skipped != 1, "s")),
			inner)))
	}
	return body
}

// rasterLineBraille draws a polyline at Braille resolution (2×4 dots per cell).
func rasterLineBraille(plotW, plotH int, pts []chartPoint, minX, maxX, minY, maxY float64) [][]rune {
	return rasterBraille(plotW, plotH, pts, minX, maxX, minY, maxY, true)
}

// rasterScatterBraille plots each sample as a point with no connecting stroke.
func rasterScatterBraille(plotW, plotH int, pts []chartPoint, minX, maxX, minY, maxY float64) [][]rune {
	return rasterBraille(plotW, plotH, pts, minX, maxX, minY, maxY, false)
}

func rasterBraille(plotW, plotH int, pts []chartPoint, minX, maxX, minY, maxY float64, connect bool) [][]rune {
	pixW, pixH := plotW*2, plotH*4
	dots := make([][]byte, plotH)
	for i := range dots {
		dots[i] = make([]byte, plotW)
	}
	set := func(px, py int) {
		if px < 0 || py < 0 || px >= pixW || py >= pixH {
			return
		}
		dots[py/4][px/2] |= brailleBits[py%4][px%2]
	}
	if connect {
		if len(pts) == 1 {
			px, py := linePixelOf(pts[0].x, pts[0].y, minX, maxX, minY, maxY, pixW, pixH)
			set(px, py)
		}
		for i := 1; i < len(pts); i++ {
			x0, y0 := linePixelOf(pts[i-1].x, pts[i-1].y, minX, maxX, minY, maxY, pixW, pixH)
			x1, y1 := linePixelOf(pts[i].x, pts[i].y, minX, maxX, minY, maxY, pixW, pixH)
			strokePixels(set, x0, y0, x1, y1)
		}
	} else {
		for _, p := range pts {
			px, py := linePixelOf(p.x, p.y, minX, maxX, minY, maxY, pixW, pixH)
			set(px, py)
		}
	}
	grid := make([][]rune, plotH)
	for y := range dots {
		grid[y] = make([]rune, plotW)
		for x, bits := range dots[y] {
			if bits == 0 {
				grid[y][x] = ' '
			} else {
				grid[y][x] = brailleBase + rune(bits)
			}
		}
	}
	return grid
}

func linePixelOf(x, y, minX, maxX, minY, maxY float64, pixW, pixH int) (px, py int) {
	px = scalePixel(x, minX, maxX, pixW)
	if pixH <= 1 || maxY == minY {
		return px, pixH / 2
	}
	t := (y - minY) / (maxY - minY)
	py = (pixH - 1) - int(math.Round(t*float64(pixH-1)))
	if py < 0 {
		py = 0
	}
	if py >= pixH {
		py = pixH - 1
	}
	return px, py
}

func scalePixel(v, min, max float64, extent int) int {
	if extent <= 1 || max == min {
		return 0
	}
	p := int(math.Round((v - min) / (max - min) * float64(extent-1)))
	if p < 0 {
		p = 0
	}
	if p >= extent {
		p = extent - 1
	}
	return p
}

func strokePixels(set func(x, y int), x0, y0, x1, y1 int) {
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
		set(x, y)
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

// overlayCrosshair draws a muted vertical/horizontal guide through the
// selected sample. The series itself is left intact.
func overlayCrosshair(grid [][]rune, col, row int) {
	if len(grid) == 0 || col < 0 || row < 0 {
		return
	}
	h, w := len(grid), len(grid[0])
	if col >= w || row >= h {
		return
	}
	for x := 0; x < w; x++ {
		if grid[row][x] == ' ' {
			grid[row][x] = '─'
		}
	}
	for y := 0; y < h; y++ {
		if grid[y][col] == ' ' {
			grid[y][col] = '│'
		}
	}
	switch grid[row][col] {
	case ' ', '─', '│':
		grid[row][col] = '┼'
	}
}

func isBrailleCell(ch rune) bool {
	return ch > brailleBase && ch <= brailleBase+0xFF
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
