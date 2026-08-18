package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestExPieFromCursor(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"user", "hits"}, [][]string{
		{"alice", "1"},
		{"bob", "2"},
		{"alice", "3"},
	}, "")
	m.results.SetCursor(0, 0)

	if cmd := m.exPie(nil, false); cmd != nil {
		t.Fatal("page :pie should not return a cmd")
	}
	if m.chartPanel.kind != chartKindPie {
		t.Fatalf("kind = %v, want pie", m.chartPanel.kind)
	}
	if m.chartPanel.title != "pie · user" {
		t.Errorf("title = %q, want pie · user", m.chartPanel.title)
	}
	if len(m.chartPanel.bars) != 2 || m.chartPanel.bars[0].label != "alice" || m.chartPanel.bars[0].value != 2 {
		t.Fatalf("bars = %+v", m.chartPanel.bars)
	}
	m.chartPanel.SetSize(40, 14)
	view := ansi.Strip(m.chartPanel.View())
	if !strings.Contains(view, "alice") || !strings.Contains(view, "bob") {
		t.Errorf("pie view missing legend labels:\n%s", view)
	}
}

func TestExPieBangChartsAllRows(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	m.lastQuery = "SELECT * FROM users"
	if cmd := m.exPie([]string{"name"}, false); cmd != nil {
		t.Fatal("page :pie should not return a cmd")
	}
	if len(m.chartPanel.bars) != 1 {
		t.Fatalf("page pie bars = %d, want 1", len(m.chartPanel.bars))
	}
	m.chartPanel.Hide()
	applyChartCmd(t, &m, m.exPie([]string{"name"}, true))
	if len(m.chartPanel.bars) != 3 {
		t.Fatalf("bang pie bars = %d, want 3", len(m.chartPanel.bars))
	}
	if !strings.Contains(m.chartPanel.title, "pie · name") || !strings.Contains(m.chartPanel.title, " · all") {
		t.Errorf("title = %q", m.chartPanel.title)
	}
}

func TestExPieDrillKeepsRows(t *testing.T) {
	m := filterTestModel(t)
	m.chartPanel.ShowPie("pie · msg", []chartBar{
		{label: "open", value: 2, n: 2},
		{label: "done", value: 1, n: 1},
	}, 0)
	m.chartPanel.filterCol = "msg"
	m.chartPanel.cursor = 0
	cmd := m.drillChartBar()
	if cmd == nil {
		t.Fatal("drill returned nil cmd")
	}
	if m.chartPanel.IsVisible() {
		t.Fatal("chart should close after drill")
	}
	if len(m.filters) != 1 || m.filters[0] != "msg = 'open'" {
		t.Fatalf("filters = %v", m.filters)
	}
}

func TestPieSliceAngles(t *testing.T) {
	bars := []chartBar{{value: 25}, {value: 75}}
	angles := pieSliceAngles(bars, 100)
	if len(angles) != 3 {
		t.Fatalf("angles = %d, want 3 bounds", len(angles))
	}
	if angles[0] != 0 {
		t.Errorf("start = %v", angles[0])
	}
	if angles[1] <= 0 || angles[2] <= angles[1] {
		t.Errorf("bounds = %v", angles)
	}
}

func TestRasterPieSlicesFillsCircle(t *testing.T) {
	bars := []chartBar{{value: 1}, {value: 1}}
	fillBits, _, slices := rasterPieSlices(8, 8, bars, 2)
	filled := 0
	for y, row := range fillBits {
		for x, b := range row {
			if b > 0 {
				filled++
			}
			if b > 0 && slices[y][x] < 0 {
				t.Errorf("filled cell (%d,%d) missing slice index", x, y)
			}
		}
	}
	if filled < 20 {
		t.Errorf("filled cells = %d, want a reasonable pie", filled)
	}
}

func TestPieRowWidths(t *testing.T) {
	c := NewChartPanel()
	c.ShowPie("pie · x", []chartBar{
		{label: "alpha", value: 40},
		{label: "beta", value: 30},
		{label: "gamma", value: 20},
		{label: "delta", value: 10},
	}, 0)
	c.SetSize(60, 16)
	inner := c.contentWidth()
	vis := c.visibleBars()
	legend := c.pieLegendLines(vis, pieTotal(vis))
	legendW := pieLegendWidth(legend)
	pieColW := inner - legendW - 1
	plotH := c.contentHeight()
	if plotH > pieColW/2 {
		plotH = pieColW / 2
	}
	lines := c.renderPieGrid(pieColW, plotH, vis, pieColW)
	for i, ln := range lines {
		w := lipgloss.Width(ln)
		if w != pieColW {
			t.Errorf("row %d: width=%d want pieColW=%d (inner=%d legendW=%d)", i, w, pieColW, inner, legendW)
		}
	}
}

func TestPieSelectedSliceFilled(t *testing.T) {
	bars := []chartBar{{value: 1}, {value: 1}, {value: 1}}

	countBraille := func(cursor int) int {
		c := ChartPanel{cursor: cursor}
		lines := c.renderPieGrid(12, 12, bars, 12)
		n := 0
		for _, ln := range lines {
			for _, r := range ansi.Strip(ln) {
				if r != ' ' {
					n++
				}
			}
		}
		return n
	}

	unselected := countBraille(-1)
	selected := countBraille(0)
	if selected <= unselected {
		t.Fatalf("selected slice should add fill: unselected=%d selected=%d", unselected, selected)
	}
}

func TestPieLegendAlignment(t *testing.T) {
	c := NewChartPanel()
	c.kind = chartKindPie
	c.ShowPie("pie · x", []chartBar{
		{label: "alpha", value: 40},
		{label: "beta", value: 30},
		{label: "gamma", value: 20},
		{label: "delta", value: 10},
	}, 0)
	c.SetSize(60, 16)
	inner := c.contentWidth()
	availH := c.contentHeight()
	vis := c.visibleBars()
	total := pieTotal(vis)
	legend := c.pieLegendLines(vis, total)
	legendW := pieLegendWidth(legend)
	legendH := len(legend)
	wantCol := inner - pieLegendPadRight - legendW
	wantFirstRow := availH - pieLegendPadBottom - legendH

	lines := c.pieBodyLines(inner)
	var legendCols []int
	var legendRows []int
	for row, ln := range lines {
		stripped := ansi.Strip(ln)
		idx := strings.Index(stripped, "●")
		if idx < 0 {
			continue
		}
		legendCols = append(legendCols, lipgloss.Width(stripped[:idx]))
		legendRows = append(legendRows, row)
	}
	if len(legendCols) < 2 {
		t.Fatalf("expected legend rows, got %d", len(legendCols))
	}
	for i, col := range legendCols {
		if col != wantCol {
			t.Fatalf("legend column drift on row %d: col=%d want=%d cols=%v", i, col, wantCol, legendCols)
		}
	}
	if legendRows[0] != wantFirstRow {
		t.Fatalf("legend starts at row %d, want %d (bottom padded)", legendRows[0], wantFirstRow)
	}
	for i := 1; i < len(legendRows); i++ {
		if legendRows[i] != legendRows[i-1]+1 {
			t.Fatalf("legend rows not contiguous: %v", legendRows)
		}
	}
}
