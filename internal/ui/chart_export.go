package ui

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// chartExportFormat is txt (Unicode snapshot) or svg.
type chartExportFormat int

const (
	chartExportTXT chartExportFormat = iota
	chartExportSVG
)

func parseChartExportFormat(s string) (chartExportFormat, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "txt", "text", "unicode", "plain":
		return chartExportTXT, true
	case "svg":
		return chartExportSVG, true
	default:
		return 0, false
	}
}

func (f chartExportFormat) ext() string {
	if f == chartExportSVG {
		return "svg"
	}
	return "txt"
}

// SnapshotText returns a plain Unicode snapshot of the chart (no ANSI, no
// panel border). Bar/hist exports include every visible bar (not just the
// scrolled viewport). Line/scatter/pie use a fixed export canvas size.
func (c ChartPanel) SnapshotText() string {
	snap := c.snapshotPanel()
	lines := snap.bodyLines(snap.contentWidth())
	var b strings.Builder
	if snap.title != "" {
		b.WriteString(snap.title)
		b.WriteString("\n\n")
	}
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ansi.Strip(line))
	}
	if b.Len() == 0 || b.String()[b.Len()-1] != '\n' {
		b.WriteByte('\n')
	}
	return b.String()
}

// SnapshotSVG returns a vector rendering of the current chart series.
func (c ChartPanel) SnapshotSVG() string {
	switch c.kind {
	case chartKindLine, chartKindScatter:
		return chartSVGXY(c)
	case chartKindPie:
		return chartSVGPie(c)
	default:
		return chartSVGBars(c)
	}
}

// snapshotPanel prepares a copy sized for full-series text export.
func (c ChartPanel) snapshotPanel() ChartPanel {
	snap := c
	snap.cursor = -1
	snap.scroll = 0
	const minInner = 72
	if snap.contentWidth() < minInner {
		snap.width = minInner + borderOverhead
	}
	switch snap.kind {
	case chartKindBar, chartKindHist:
		n := len(snap.visibleBars())
		if n < 1 {
			n = 1
		}
		snap.height = n + snap.footerLines() + borderOverhead
	case chartKindPie:
		if snap.contentHeight() < 22 {
			snap.height = 22 + borderOverhead
		}
	default:
		if snap.contentHeight() < 18 {
			snap.height = 18 + borderOverhead
		}
	}
	return snap
}

func chartExportFilename(title string, format chartExportFormat, ts time.Time) string {
	slug := chartTitleSlug(title)
	return fmt.Sprintf("creel_chart_%s_%s.%s", slug, ts.Format("20060102_150405"), format.ext())
}

func chartTitleSlug(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "chart"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteByte('_')
			prevDash = true
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return "chart"
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// exportChart writes the open chart to ~/Downloads as txt or svg.
func (m *Model) exportChart(format chartExportFormat) {
	if !m.chartPanel.IsVisible() {
		m.exportMsg = "no chart open — run :bar / :line / :pie first"
		return
	}
	var content string
	switch format {
	case chartExportSVG:
		content = m.chartPanel.SnapshotSVG()
	default:
		content = m.chartPanel.SnapshotText()
	}
	name := chartExportFilename(m.chartPanel.title, format, time.Now())
	path, _, err := writeFile(name, content, 0)
	if err != nil {
		m.exportMsg = fmt.Sprintf("chart export failed: %v", err)
		return
	}
	m.exportMsg = fmt.Sprintf("exported chart → %s", path)
}

// exChartExport handles :chartexport [txt|svg].
func (m *Model) exChartExport(args []string) tea.Cmd {
	format := chartExportTXT
	if len(args) > 0 {
		f, ok := parseChartExportFormat(args[0])
		if !ok {
			m.schemaMsg = ":chartexport needs txt or svg"
			return nil
		}
		format = f
	}
	m.exportChart(format)
	return nil
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&apos;",
	)
	return replacer.Replace(s)
}

func chartSVGBars(c ChartPanel) string {
	vis := c.visibleBars()
	title := c.title
	if title == "" {
		title = "chart"
	}
	if len(vis) == 0 {
		return chartSVGEmpty(title)
	}
	maxVal := 0.0
	for _, b := range vis {
		if b.value > maxVal {
			maxVal = b.value
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	const (
		left   = 140.0
		top    = 48.0
		barH   = 18.0
		gap    = 6.0
		plotW  = 420.0
		right  = 72.0
		bottom = 24.0
	)
	height := top + float64(len(vis))*(barH+gap) + bottom
	width := left + plotW + right
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">
<title>%s</title>
<rect width="100%%" height="100%%" fill="#ffffff"/>
<text x="16" y="28" font-family="ui-sans-serif, system-ui, sans-serif" font-size="14" fill="#111">%s</text>
`, width, height, width, height, xmlEscape(title), xmlEscape(title)))
	for i, bar := range vis {
		y := top + float64(i)*(barH+gap)
		w := plotW * bar.value / maxVal
		if w < 0 {
			w = 0
		}
		b.WriteString(fmt.Sprintf(
			`<text x="%.0f" y="%.0f" text-anchor="end" font-family="ui-sans-serif, system-ui, sans-serif" font-size="11" fill="#333">%s</text>
<rect x="%.0f" y="%.0f" width="%.1f" height="%.0f" fill="#3b82f6" rx="2"/>
<text x="%.0f" y="%.0f" font-family="ui-sans-serif, system-ui, sans-serif" font-size="11" fill="#333">%s</text>
`,
			left-8, y+barH*0.72, xmlEscape(truncateRunes(bar.label, 28)),
			left, y, w, barH,
			left+w+8, y+barH*0.72, xmlEscape(formatChartValue(bar.value)),
		))
	}
	b.WriteString("</svg>\n")
	return b.String()
}

func chartSVGXY(c ChartPanel) string {
	title := c.title
	if title == "" {
		title = "chart"
	}
	pts := c.points
	if len(pts) == 0 {
		return chartSVGEmpty(title)
	}
	minX, maxX := pts[0].x, pts[0].x
	minY, maxY := pts[0].y, pts[0].y
	for _, p := range pts {
		if p.x < minX {
			minX = p.x
		}
		if p.x > maxX {
			maxX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.y > maxY {
			maxY = p.y
		}
	}
	if minX == maxX {
		maxX = minX + 1
	}
	if minY == maxY {
		maxY = minY + 1
	}
	const (
		left   = 56.0
		top    = 48.0
		plotW  = 520.0
		plotH  = 320.0
		right  = 24.0
		bottom = 40.0
	)
	width := left + plotW + right
	height := top + plotH + bottom
	mapX := func(x float64) float64 {
		return left + (x-minX)/(maxX-minX)*plotW
	}
	mapY := func(y float64) float64 {
		return top + plotH - (y-minY)/(maxY-minY)*plotH
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">
<title>%s</title>
<rect width="100%%" height="100%%" fill="#ffffff"/>
<text x="16" y="28" font-family="ui-sans-serif, system-ui, sans-serif" font-size="14" fill="#111">%s</text>
<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" fill="none" stroke="#ccc"/>
`, width, height, width, height, xmlEscape(title), xmlEscape(title), left, top, plotW, plotH))
	b.WriteString(fmt.Sprintf(
		`<text x="%.0f" y="%.0f" font-family="ui-sans-serif, system-ui, sans-serif" font-size="10" fill="#666">%s</text>
<text x="%.0f" y="%.0f" font-family="ui-sans-serif, system-ui, sans-serif" font-size="10" fill="#666">%s</text>
`,
		8.0, top+10, xmlEscape(formatChartValue(maxY)),
		8.0, top+plotH, xmlEscape(formatChartValue(minY)),
	))
	if c.kind == chartKindLine && len(pts) > 1 {
		var poly strings.Builder
		for i, p := range pts {
			if i > 0 {
				poly.WriteByte(' ')
			}
			poly.WriteString(fmt.Sprintf("%.1f,%.1f", mapX(p.x), mapY(p.y)))
		}
		b.WriteString(fmt.Sprintf(`<polyline fill="none" stroke="#3b82f6" stroke-width="2" points="%s"/>`+"\n", poly.String()))
	}
	for _, p := range pts {
		b.WriteString(fmt.Sprintf(
			`<circle cx="%.1f" cy="%.1f" r="3" fill="#3b82f6"/>`+"\n",
			mapX(p.x), mapY(p.y),
		))
	}
	b.WriteString("</svg>\n")
	return b.String()
}

func chartSVGPie(c ChartPanel) string {
	title := c.title
	if title == "" {
		title = "chart"
	}
	vis := c.visibleBars()
	total := 0.0
	for _, bar := range vis {
		total += bar.value
	}
	if len(vis) == 0 || total <= 0 {
		return chartSVGEmpty(title)
	}
	const (
		cx     = 200.0
		cy     = 200.0
		r      = 140.0
		width  = 640.0
		height = 400.0
		legX   = 380.0
		legY   = 60.0
	)
	colors := []string{"#3b82f6", "#8b5cf6", "#22c55e", "#ef4444", "#f59e0b", "#14b8a6"}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">
<title>%s</title>
<rect width="100%%" height="100%%" fill="#ffffff"/>
<text x="16" y="28" font-family="ui-sans-serif, system-ui, sans-serif" font-size="14" fill="#111">%s</text>
`, width, height, width, height, xmlEscape(title), xmlEscape(title)))
	start := -math.Pi / 2
	for i, bar := range vis {
		frac := bar.value / total
		sweep := frac * 2 * math.Pi
		end := start + sweep
		large := 0
		if sweep > math.Pi {
			large = 1
		}
		x1 := cx + r*math.Cos(start)
		y1 := cy + r*math.Sin(start)
		x2 := cx + r*math.Cos(end)
		y2 := cy + r*math.Sin(end)
		color := colors[i%len(colors)]
		if frac >= 1 {
			// Full circle — path arcs can't close a 360° slice.
			b.WriteString(fmt.Sprintf(
				`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`+"\n",
				cx, cy, r, color,
			))
		} else {
			b.WriteString(fmt.Sprintf(
				`<path d="M %.1f %.1f L %.1f %.1f A %.1f %.1f 0 %d 1 %.1f %.1f Z" fill="%s"/>`+"\n",
				cx, cy, x1, y1, r, r, large, x2, y2, color,
			))
		}
		pct := 100 * frac
		b.WriteString(fmt.Sprintf(
			`<rect x="%.0f" y="%.0f" width="12" height="12" fill="%s" rx="2"/>
<text x="%.0f" y="%.0f" font-family="ui-sans-serif, system-ui, sans-serif" font-size="12" fill="#333">%s — %s (%.1f%%)</text>
`,
			legX, legY+float64(i)*22, color,
			legX+20, legY+float64(i)*22+11,
			xmlEscape(truncateRunes(bar.label, 24)),
			xmlEscape(formatChartValue(bar.value)),
			pct,
		))
		start = end
	}
	b.WriteString("</svg>\n")
	return b.String()
}

func chartSVGEmpty(title string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="480" height="120" viewBox="0 0 480 120">
<title>%s</title>
<rect width="100%%" height="100%%" fill="#ffffff"/>
<text x="16" y="28" font-family="ui-sans-serif, system-ui, sans-serif" font-size="14" fill="#111">%s</text>
<text x="16" y="64" font-family="ui-sans-serif, system-ui, sans-serif" font-size="12" fill="#666">no numeric values to chart</text>
</svg>
`, xmlEscape(title), xmlEscape(title))
}
