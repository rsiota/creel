package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// chartBar is one horizontal bar: a label axis value and a numeric magnitude.
type chartBar struct {
	label string
	value float64
	n     int // row count in this group; used to fold avg into (other)
}

// ChartPanel renders a simple chart (currently horizontal bars) in the
// results-panel slot. Esc/q closes it and restores the grid.
type ChartPanel struct {
	visible  bool
	kind     chartKind
	title    string // e.g. "bar · users × amount · sum"
	bars     []chartBar
	points   []chartPoint
	agg      barAgg
	expanded bool // true = show every grouped bar; false = top 20 + (other)
	skipped  int  // rows skipped (NULL / non-numeric values)
	cursor   int  // index into visibleBars / points
	scroll   int
	width    int
	height   int
}

type chartKind int

const (
	chartKindBar chartKind = iota
	chartKindLine
	chartKindHist
)

// NewChartPanel returns a hidden chart panel.
func NewChartPanel() ChartPanel { return ChartPanel{} }

// IsVisible reports whether the chart is shown in place of the results grid.
func (c ChartPanel) IsVisible() bool { return c.visible }

// ShowBar populates a horizontal bar chart and makes the panel visible.
func (c *ChartPanel) ShowBar(title string, bars []chartBar, skipped int, agg barAgg) {
	c.visible = true
	c.kind = chartKindBar
	c.title = title
	c.bars = bars
	c.points = nil
	c.agg = agg
	c.expanded = false
	c.skipped = skipped
	c.cursor = 0
	c.scroll = 0
}

// ShowLine populates a line chart and makes the panel visible.
func (c *ChartPanel) ShowLine(title string, points []chartPoint, skipped int) {
	c.visible = true
	c.kind = chartKindLine
	c.title = title
	c.bars = nil
	c.points = points
	c.expanded = false
	c.skipped = skipped
	c.cursor = 0
	c.scroll = 0
}

// ShowHist populates a histogram (horizontal bars, never folded) and makes
// the panel visible.
func (c *ChartPanel) ShowHist(title string, bars []chartBar, skipped int) {
	c.ShowBar(title, bars, skipped, barAggCount)
	c.kind = chartKindHist
	c.expanded = true
}

// Hide closes the chart panel.
func (c *ChartPanel) Hide() { c.visible = false }

// SetSize sets the exterior panel dimensions (including border), matching the
// results slot so the chart drops in without shifting the workspace.
func (c *ChartPanel) SetSize(w, h int) { c.width = w; c.height = h }

func (c ChartPanel) contentHeight() int {
	h := c.height - borderOverhead
	if h < 1 {
		h = 1
	}
	return h
}

func (c ChartPanel) contentWidth() int {
	w := c.width - borderOverhead
	if w < 10 {
		w = 10
	}
	return w
}

// Update moves the cursor and unfolds bar (other). The chart stays put until
// the cursor walks off the edge of the viewport, then it follows by one row.
func (c ChartPanel) Update(msg tea.KeyMsg) ChartPanel {
	n := c.seriesLen()
	vh := c.viewport()
	clamp := func() {
		if c.cursor >= n {
			c.cursor = n - 1
		}
		if c.cursor < 0 {
			c.cursor = 0
		}
	}
	switch msg.String() {
	case "o":
		if c.kind == chartKindBar && len(c.bars) > chartBarLimit {
			was := c.expanded
			c.expanded = !c.expanded
			n = c.seriesLen()
			if c.expanded && !was && n > chartBarLimit {
				c.cursor = chartBarLimit
			}
			clamp()
			c.adjustScroll(vh, n)
		}
	case "j", "down", "l", "right":
		if c.cursor < n-1 {
			c.cursor++
			c.adjustScroll(vh, n)
		}
	case "k", "up", "h", "left":
		if c.cursor > 0 {
			c.cursor--
			c.adjustScroll(vh, n)
		}
	case "g":
		c.cursor = 0
		c.scroll = 0
	case "G":
		c.cursor = n - 1
		c.adjustScroll(vh, n)
	case "ctrl+d":
		c.cursor += vh / 2
		clamp()
		c.adjustScroll(vh, n)
	case "ctrl+u":
		c.cursor -= vh / 2
		clamp()
		c.adjustScroll(vh, n)
	}
	return c
}

func (c *ChartPanel) adjustScroll(vh, n int) {
	if vh <= 0 {
		return
	}
	if c.cursor < c.scroll {
		c.scroll = c.cursor
	}
	if c.cursor >= c.scroll+vh {
		c.scroll = c.cursor - vh + 1
	}
	maxScroll := n - vh
	if maxScroll < 0 {
		maxScroll = 0
	}
	if c.scroll > maxScroll {
		c.scroll = maxScroll
	}
	if c.scroll < 0 {
		c.scroll = 0
	}
}

func (c ChartPanel) seriesLen() int {
	if c.kind == chartKindLine {
		return len(c.points)
	}
	return len(c.visibleBars())
}

func (c ChartPanel) visibleBars() []chartBar {
	return foldChartBars(c.bars, c.expanded, c.agg)
}

// viewport is how many bar rows fit. A skipped-row note, when present,
// takes the last content line; otherwise bars fill the panel.
func (c ChartPanel) viewport() int {
	vh := c.contentHeight() - c.footerLines()
	if vh < 1 {
		vh = 1
	}
	return vh
}

func (c ChartPanel) footerLines() int {
	if c.skipped > 0 {
		return 1
	}
	return 0
}

// View renders the bordered chart panel.
func (c ChartPanel) View() string {
	inner := c.contentWidth()
	lines := c.bodyLines(inner)
	for len(lines) < c.contentHeight() {
		lines = append(lines, "")
	}
	if len(lines) > c.contentHeight() {
		lines = lines[:c.contentHeight()]
	}
	return lipgloss.NewStyle().
		Width(inner).
		Height(c.contentHeight()).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Render(strings.Join(lines, "\n"))
}

func (c ChartPanel) bodyLines(inner int) []string {
	if c.kind == chartKindLine {
		return c.lineBodyLines(inner)
	}
	return c.barBodyLines(inner)
}

func (c ChartPanel) barBodyLines(inner int) []string {
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	barStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	labelStyle := lipgloss.NewStyle().Foreground(colorFg)
	valStyle := lipgloss.NewStyle().Foreground(colorLabel)

	if len(c.bars) == 0 {
		return []string{muted.Render(truncateCell("no numeric values to chart", inner))}
	}

	vis := c.visibleBars()
	maxVal := 0.0
	for _, b := range vis {
		if b.value > maxVal {
			maxVal = b.value
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	labelW := 0
	for _, b := range vis {
		if w := lipgloss.Width(b.label); w > labelW {
			labelW = w
		}
	}
	if labelW > inner/3 {
		labelW = inner / 3
	}
	if labelW < 4 {
		labelW = 4
	}

	// " label │████…  value"
	valSample := formatChartValue(maxVal)
	valW := lipgloss.Width(valSample)
	if valW < 4 {
		valW = 4
	}
	// spaces: 1 before label area ends + " │" + 1 after bar + value
	barW := inner - labelW - valW - 4
	if barW < 4 {
		barW = 4
	}

	var out []string
	vh := c.viewport()
	start := c.scroll
	end := start + vh
	if end > len(vis) {
		end = len(vis)
	}
	if start > end {
		start = end
	}
	for i, b := range vis[start:end] {
		filled := 0
		if maxVal > 0 {
			filled = int(math.Round(float64(barW) * b.value / maxVal))
		}
		if filled < 0 {
			filled = 0
		}
		if filled > barW {
			filled = barW
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)
		label := truncateCell(b.label, labelW)
		val := padLeft(formatChartValue(b.value), valW)
		selected := start+i == c.cursor
		ls, ms, bs, vs := labelStyle, muted, barStyle, valStyle
		if selected {
			ls = ls.Background(colorCursorRow)
			ms = ms.Background(colorCursorRow)
			bs = bs.Background(colorCursorRow)
			vs = vs.Background(colorCursorRow)
		}
		line := ls.Render(label) + ms.Render(" │") + bs.Render(bar) + vs.Render(" "+val)
		if selected {
			if pad := inner - lipgloss.Width(line); pad > 0 {
				line += lipgloss.NewStyle().Background(colorCursorRow).Render(strings.Repeat(" ", pad))
			}
		}
		out = append(out, line)
	}

	if c.skipped > 0 {
		out = append(out, muted.Render(truncateCell(
			fmt.Sprintf("skipped %d non-numeric/NULL row%s", c.skipped, pluralIf(c.skipped != 1, "s")),
			inner)))
	}
	return out
}

func formatChartValue(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e12 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.4g", v)
}

func padLeft(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}

// barAgg is how :bar collapses duplicate labels into one bar.
type barAgg int

const (
	barAggSum barAgg = iota
	barAggCount
	barAggAvg
)

func (a barAgg) String() string {
	switch a {
	case barAggCount:
		return "count"
	case barAggAvg:
		return "avg"
	default:
		return "sum"
	}
}

func parseBarAgg(s string) (barAgg, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sum":
		return barAggSum, true
	case "count":
		return barAggCount, true
	case "avg", "average", "mean":
		return barAggAvg, true
	}
	return 0, false
}

const (
	chartBarLimit = 20
	otherBarLabel = "(other)"
)

type barBucket struct {
	label string
	sum   float64
	n     int
}

func (b barBucket) value(agg barAgg) float64 {
	switch agg {
	case barAggCount:
		return float64(b.n)
	case barAggAvg:
		if b.n == 0 {
			return 0
		}
		return b.sum / float64(b.n)
	default:
		return b.sum
	}
}

// buildBarSeries walks the current result page into aggregated chart bars.
// Duplicate labels are grouped; sum/avg skip NULL, non-numeric, and negative
// values (counted in skipped). count includes every row. Bars are sorted by
// value descending and capped at chartBarLimit, with the rest folded into
// "(other)".
func buildBarSeries(r ResultsTable, labelCol, valueCol int, agg barAgg) (bars []chartBar, skipped int) {
	idx := map[string]int{}
	var buckets []barBucket

	n := r.NumRows()
	for i := 0; i < n; i++ {
		label := r.RowValue(i, labelCol)
		if label == "" || label == "NULL" {
			label = "(null)"
		}
		if agg == barAggCount {
			if j, ok := idx[label]; ok {
				buckets[j].n++
			} else {
				idx[label] = len(buckets)
				buckets = append(buckets, barBucket{label: label, n: 1})
			}
			continue
		}
		raw := r.RowValue(i, valueCol)
		if raw == "" || raw == "NULL" {
			skipped++
			continue
		}
		v, ok := parseFloat(raw)
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			skipped++
			continue
		}
		if j, ok := idx[label]; ok {
			buckets[j].sum += v
			buckets[j].n++
		} else {
			idx[label] = len(buckets)
			buckets = append(buckets, barBucket{label: label, sum: v, n: 1})
		}
	}
	if len(buckets) == 0 {
		return nil, skipped
	}

	sort.SliceStable(buckets, func(i, j int) bool {
		vi, vj := buckets[i].value(agg), buckets[j].value(agg)
		if vi != vj {
			return vi > vj
		}
		return buckets[i].label < buckets[j].label
	})

	bars = make([]chartBar, len(buckets))
	for i, b := range buckets {
		bars[i] = chartBar{label: b.label, value: b.value(agg), n: b.n}
	}
	return bars, skipped
}

// foldChartBars keeps the top chartBarLimit bars and collapses the rest into
// "(other)", unless expanded or the set already fits.
func foldChartBars(bars []chartBar, expanded bool, agg barAgg) []chartBar {
	if expanded || len(bars) <= chartBarLimit {
		return bars
	}
	head := bars[:chartBarLimit]
	var other chartBar
	other.label = otherBarLabel
	var n int
	var acc float64
	for _, b := range bars[chartBarLimit:] {
		n += b.n
		if agg == barAggAvg {
			acc += b.value * float64(b.n)
		} else {
			acc += b.value
		}
	}
	other.n = n
	if agg == barAggAvg {
		if n > 0 {
			other.value = acc / float64(n)
		}
	} else {
		other.value = acc
	}
	out := make([]chartBar, 0, chartBarLimit+1)
	out = append(out, head...)
	return append(out, other)
}
