package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// pieSliceColors cycles through theme accents for distinct slices.
var pieSliceColors = []lipgloss.Color{
	colorPrimary,
	colorAccent,
	colorSuccess,
	colorLabel,
	colorFg,
	colorWarn,
}

const pieLegendMinW = 22
const pieLegendPadBottom = 4
const pieLegendPadRight = 9
const pieRadiusFactor = 0.89
const pieSizeScale = 0.92

// ShowPie populates a pie chart (same counts as :freq) and makes the panel visible.
func (c *ChartPanel) ShowPie(title string, bars []chartBar, skipped int) {
	c.ShowBar(title, bars, skipped, barAggCount)
	c.kind = chartKindPie
}

func (c ChartPanel) pieBodyLines(inner int) []string {
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	if len(c.bars) == 0 {
		return []string{muted.Render(truncateCell("no values to chart", inner))}
	}

	vis := c.visibleBars()
	total := pieTotal(vis)
	if total <= 0 {
		return []string{muted.Render(truncateCell("no values to chart", inner))}
	}

	availH := c.contentHeight() - c.footerLines()
	legend := c.pieLegendLines(vis, total)
	legendW := pieLegendWidth(legend)
	legendH := len(legend)

	out := pieComposeLayout(inner, availH, legend, legendW, legendH, func(plotW, plotH int) []string {
		return c.renderPieGrid(plotW, plotH, vis, plotW)
	})

	if c.skipped > 0 {
		note := muted.Render(truncateCell(
			fmt.Sprintf("skipped %d non-numeric/NULL row%s", c.skipped, pluralIf(c.skipped != 1, "s")),
			inner))
		if len(out) >= availH {
			out[availH-1] = note
		} else {
			out = append(out, note)
		}
	}
	return out
}

func pieTotal(bars []chartBar) float64 {
	var t float64
	for _, b := range bars {
		t += b.value
	}
	return t
}

func pieLegendWidth(lines []string) int {
	w := pieLegendMinW
	for _, ln := range lines {
		if lw := lipgloss.Width(ln); lw > w {
			w = lw
		}
	}
	return w
}

// pieComposeLayout places the pie in the remaining area and anchors the legend
// to the bottom-right with fixed padding.
func pieComposeLayout(inner, availH int, legend []string, legendW, legendH int, renderPie func(plotW, plotH int) []string) []string {
	out := make([]string, availH)
	for i := range out {
		out[i] = strings.Repeat(" ", inner)
	}

	legendRow := availH - pieLegendPadBottom - legendH
	legendCol := inner - pieLegendPadRight - legendW
	if legendCol < 0 {
		legendCol = 0
	}
	if legendRow < 0 {
		legendRow = 0
	}

	// Size the pie from the full panel; the legend overlays the bottom-right corner.
	plotH := availH
	plotW := inner / 2
	if plotW < 4 {
		plotW = 4
	}
	if plotW > plotH*2 {
		plotW = plotH * 2
	}
	if plotH > plotW/2 {
		plotH = plotW / 2
	}
	if plotH < 4 {
		plotH = 4
	}
	if plotW > inner {
		plotW = inner
	}
	plotW = int(float64(plotW) * pieSizeScale)
	plotH = int(float64(plotH) * pieSizeScale)
	if plotW < 4 {
		plotW = 4
	}
	if plotH < 4 {
		plotH = 4
	}

	pieLines := renderPie(plotW, plotH)
	pieLeft := (inner - plotW) / 2
	pieTop := (availH - plotH) / 2
	for row, line := range pieLines {
		target := pieTop + row
		if target < 0 || target >= availH {
			continue
		}
		out[target] = overlayLine(out[target], line, pieLeft)
	}

	for i, leg := range legend {
		target := legendRow + i
		if target < 0 || target >= availH {
			continue
		}
		out[target] = overlayLine(out[target], leg, legendCol)
	}
	return out
}

func (c ChartPanel) pieLegendLines(vis []chartBar, total float64) []string {
	markerSt := func(i int) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(pieSliceColor(i))
	}
	labelSt := lipgloss.NewStyle().Foreground(colorFg)
	valSt := lipgloss.NewStyle().Foreground(colorLabel)
	sel := lipgloss.NewStyle().Background(colorCursorRow)

	vh := c.viewport()
	start := c.scroll
	end := start + vh
	if end > len(vis) {
		end = len(vis)
	}
	if start > end {
		start = end
	}

	var out []string
	for i, b := range vis[start:end] {
		idx := start + i
		pct := int(math.Round(b.value / total * 100))
		marker := markerSt(idx).Render("●")
		label := truncateCell(b.label, 12)
		count := formatChartValue(b.value)
		line := marker + " " + labelSt.Render(padRight(label, 12)) + " " +
			valSt.Render(padLeft(count, 6)) + " " +
			valSt.Render(padLeft(fmt.Sprintf("%d%%", pct), 4))
		if idx == c.cursor {
			line = sel.Render(padRight(line, pieLegendMinW+8))
		}
		out = append(out, line)
	}
	return out
}

func pieSliceColor(i int) lipgloss.Color {
	return pieSliceColors[i%len(pieSliceColors)]
}

// pieBrailleFill is the Braille dot mask used for the selected slice.
const pieBrailleFill = 0xFF

func (c ChartPanel) renderPieGrid(plotW, plotH int, vis []chartBar, lineW int) []string {
	total := pieTotal(vis)
	fillBits, borderBits, slices := rasterPieSlices(plotW, plotH, vis, total)

	out := make([]string, 0, plotH)
	for row := 0; row < plotH; row++ {
		var b strings.Builder
		for col := range fillBits[row] {
			idx := slices[row][col]
			selected := idx == c.cursor

			var cellBits byte
			switch {
			case selected && fillBits[row][col] != 0:
				cellBits = fillBits[row][col] & pieBrailleFill
				if cellBits == 0 {
					cellBits = fillBits[row][col]
				}
			case !selected && borderBits[row][col] != 0:
				cellBits = borderBits[row][col]
			default:
				b.WriteString(" ")
				continue
			}

			ch := brailleBase + rune(cellBits)
			st := lipgloss.NewStyle().Foreground(pieSliceColor(idx))
			b.WriteString(st.Render(string(ch)))
		}
		line := b.String()
		if pad := lineW - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out = append(out, line)
	}
	return out
}

// rasterPieSlices returns fill masks, border masks, and a slice index per cell.
// Slices start at 12 o'clock and sweep clockwise.
func rasterPieSlices(cellsW, cellsH int, bars []chartBar, total float64) ([][]byte, [][]byte, [][]int) {
	fillBits := make([][]byte, cellsH)
	borderBits := make([][]byte, cellsH)
	slices := make([][]int, cellsH)
	for y := 0; y < cellsH; y++ {
		fillBits[y] = make([]byte, cellsW)
		borderBits[y] = make([]byte, cellsW)
		slices[y] = make([]int, cellsW)
		for x := 0; x < cellsW; x++ {
			slices[y][x] = -1
		}
	}
	if total <= 0 || len(bars) == 0 {
		return fillBits, borderBits, slices
	}
	angleBounds := pieSliceAngles(bars, total)
	pixW, pixH := cellsW*2, cellsH*4
	cx := float64(pixW) / 2
	cy := float64(pixH) / 2
	r := math.Min(cx, cy) * pieRadiusFactor
	if r < 1 {
		r = 1
	}
	r2 := r * r

	sliceAt := func(ang float64) int {
		for i := 0; i < len(angleBounds)-1; i++ {
			if ang >= angleBounds[i] && ang < angleBounds[i+1] {
				return i
			}
		}
		return len(angleBounds) - 2
	}
	inside := func(px, py int) (ang float64, ok bool) {
		dx := float64(px) + 0.5 - cx
		dy := float64(py) + 0.5 - cy
		if dx*dx+dy*dy > r2 {
			return 0, false
		}
		ang = math.Atan2(-dy, dx)
		if ang < 0 {
			ang += 2 * math.Pi
		}
		return math.Mod(ang+math.Pi/2, 2*math.Pi), true
	}

	for py := 0; py < pixH; py++ {
		for px := 0; px < pixW; px++ {
			ang, ok := inside(px, py)
			if !ok {
				continue
			}
			bit := brailleBits[py%4][px%2]
			cellX, cellY := px/2, py/4
			fillBits[cellY][cellX] |= bit
			slices[cellY][cellX] = sliceAt(ang)

			outerEdge := false
			sliceEdge := false
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nAng, nOk := inside(px+d[0], py+d[1])
				if !nOk {
					outerEdge = true
					continue
				}
				if sliceAt(nAng) != sliceAt(ang) {
					sliceEdge = true
				}
			}
			if outerEdge || sliceEdge {
				borderBits[cellY][cellX] |= bit
			}
		}
	}

	for cy := 0; cy < cellsH; cy++ {
		for cx := 0; cx < cellsW; cx++ {
			if fillBits[cy][cx] == 0 {
				continue
			}
			ang, ok := inside(cx*2+1, cy*4+2)
			if ok {
				slices[cy][cx] = sliceAt(ang)
			}
		}
	}
	return fillBits, borderBits, slices
}

// pieSliceAngles returns cumulative start angles in [0, 2π) for each slice.
func pieSliceAngles(bars []chartBar, total float64) []float64 {
	if total <= 0 || len(bars) == 0 {
		return nil
	}
	out := make([]float64, len(bars)+1)
	acc := 0.0
	for i, b := range bars {
		out[i] = acc
		acc += b.value / total * 2 * math.Pi
	}
	out[len(bars)] = 2 * math.Pi
	return out
}
