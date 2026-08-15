package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestBuildLineSeriesSortsAndSkips(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"t", "v"}, [][]string{
		{"3", "30"},
		{"1", "NULL"},
		{"2", "20"},
		{"x", "5"},
		{"1", "10"},
	}, "")
	pts, skipped := buildLineSeries(r, 0, 1)
	if skipped != 2 {
		t.Errorf("skipped = %d, want 2", skipped)
	}
	if len(pts) != 3 {
		t.Fatalf("points = %d, want 3: %+v", len(pts), pts)
	}
	if pts[0].x != 1 || pts[0].y != 10 {
		t.Errorf("pts[0] = %+v, want x=1 y=10", pts[0])
	}
	if pts[1].x != 2 || pts[2].x != 3 {
		t.Errorf("order = %v %v, want x=2 then 3", pts[1], pts[2])
	}
}

func TestExLineFromArgs(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"t", "v"}, [][]string{{"1", "4"}, {"2", "8"}}, "")
	if cmd := m.exLine([]string{"t", "v"}, false); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if !m.chartPanel.IsVisible() || m.chartPanel.kind != chartKindLine {
		t.Fatal("line chart should be visible")
	}
	if len(m.chartPanel.points) != 2 {
		t.Fatalf("points = %d, want 2", len(m.chartPanel.points))
	}
}

func TestExScatterFromArgs(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"t", "v"}, [][]string{{"1", "4"}, {"2", "8"}}, "")
	if cmd := m.exScatter([]string{"t", "v"}, false); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if !m.chartPanel.IsVisible() || m.chartPanel.kind != chartKindScatter {
		t.Fatal("scatter chart should be visible")
	}
	if len(m.chartPanel.points) != 2 {
		t.Fatalf("points = %d, want 2", len(m.chartPanel.points))
	}
	if !strings.Contains(m.chartPanel.title, "scatter") {
		t.Errorf("title = %q", m.chartPanel.title)
	}
}

func TestExLineFromMarks(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"t", "v"}, [][]string{{"1", "4"}}, "")
	m.results.SetCursor(0, 0)
	m.results.ToggleColumnMark()
	m.results.SetCursor(0, 1)
	m.results.ToggleColumnMark()
	if cmd := m.exLine(nil, false); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if m.chartPanel.kind != chartKindLine {
		t.Fatal("expected line kind")
	}
}

func TestExLineErrors(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.exLine(nil, false)
	if m.schemaMsg == "" {
		t.Fatal("expected error with no results")
	}
	m.schemaMsg = ""
	m.results.SetResult([]string{"a", "b"}, [][]string{{"x", "y"}}, "")
	m.exLine(nil, false)
	if !strings.Contains(m.schemaMsg, "mark 2") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
	m.schemaMsg = ""
	m.exLine([]string{"a", "b"}, false)
	if !strings.Contains(m.schemaMsg, "no numeric or datetime") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestParseChartScalarDatetimeNotYear(t *testing.T) {
	got, ok := parseChartScalar("2026-01-07")
	if !ok {
		t.Fatal("date-only should parse")
	}
	want := float64(time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC).Unix())
	if got != want {
		t.Errorf("x = %v, want unix %v (not year 2026)", got, want)
	}
	if got == 2026 {
		t.Fatal("date-only must not parse as year 2026")
	}
}

func TestParseChartScalarNumeric(t *testing.T) {
	got, ok := parseChartScalar("3.5")
	if !ok || got != 3.5 {
		t.Errorf("got %v ok=%v, want 3.5", got, ok)
	}
	got, ok = parseChartScalar("1767744000")
	if !ok || got != 1767744000 {
		t.Errorf("unix epoch string = %v ok=%v", got, ok)
	}
	if _, ok := parseChartScalar("not-a-value"); ok {
		t.Fatal("text should not parse")
	}
}

func TestBuildLineSeriesDatetimeSortsAndKeepsLabel(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"created_at", "amount"}, [][]string{
		{"2026-01-09", "30"},
		{"2026-01-07T00:00:00Z", "10"},
		{"2026-01-08 12:00:00", "20"},
		{"nope", "1"},
	}, "")
	pts, skipped := buildLineSeries(r, 0, 1)
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if len(pts) != 3 {
		t.Fatalf("points = %d, want 3: %+v", len(pts), pts)
	}
	if pts[0].xLabel != "2026-01-07T00:00:00Z" || pts[0].y != 10 {
		t.Errorf("pts[0] = %+v", pts[0])
	}
	if pts[1].xLabel != "2026-01-08 12:00:00" || pts[2].xLabel != "2026-01-09" {
		t.Errorf("labels = %q %q", pts[1].xLabel, pts[2].xLabel)
	}
	if !(pts[0].x < pts[1].x && pts[1].x < pts[2].x) {
		t.Errorf("x order = %v %v %v", pts[0].x, pts[1].x, pts[2].x)
	}
}

func TestLineChartViewExactHeight(t *testing.T) {
	c := NewChartPanel()
	c.SetSize(60, 12)
	c.ShowLine("line · t × v", []chartPoint{
		{x: 1, y: 1, xLabel: "1"},
		{x: 2, y: 4, xLabel: "2"},
		{x: 3, y: 2, xLabel: "3"},
	}, 0)
	out := c.View()
	if got := lipgloss.Height(out); got != 12 {
		t.Errorf("height = %d, want 12\n%s", got, out)
	}
	plain := stripANSI(out)
	if !containsBraille(plain) {
		t.Errorf("missing Braille stroke:\n%s", out)
	}
	if strings.Contains(plain, "●") {
		t.Errorf("legacy point marker leaked:\n%s", out)
	}
	if strings.Contains(plain, "bar ·") {
		t.Errorf("bar title leaked:\n%s", out)
	}
	if !strings.Contains(plain, "─") || !strings.Contains(plain, "│") {
		t.Errorf("missing dim crosshair:\n%s", out)
	}
}

func TestLineChartPadding(t *testing.T) {
	c := NewChartPanel()
	c.SetSize(80, 24)
	c.ShowLine("line", []chartPoint{
		{x: 1, y: 1, xLabel: "1"},
		{x: 2, y: 4, xLabel: "2"},
	}, 0)
	lines := strings.Split(stripANSI(c.View()), "\n")
	if len(lines) < 4 {
		t.Fatalf("too few lines:\n%s", c.View())
	}
	inner := lines[1 : len(lines)-1]
	trim := func(s string) string {
		s = strings.TrimPrefix(s, "│")
		return strings.TrimSuffix(s, "│")
	}
	blank := func(s string) bool { return strings.TrimSpace(trim(s)) == "" }
	top := 0
	for _, ln := range inner {
		if !blank(ln) {
			break
		}
		top++
	}
	bottom := 0
	for i := len(inner) - 1; i >= 0; i-- {
		if !blank(inner[i]) {
			break
		}
		bottom++
	}
	if top != lineChartPad || bottom != lineChartPad {
		t.Errorf("pad top/bottom = %d/%d, want %d\n%s", top, bottom, lineChartPad, c.View())
	}
	var axisRow string
	for _, ln := range inner {
		if strings.Contains(ln, "┤") {
			axisRow = trim(ln)
			break
		}
	}
	if axisRow == "" {
		t.Fatalf("no axis row:\n%s", c.View())
	}
	if got := len(axisRow) - len(strings.TrimLeft(axisRow, " ")); got < lineChartPad {
		t.Errorf("left pad = %d, want >= %d in %q", got, lineChartPad, axisRow)
	}
	if got := len(axisRow) - len(strings.TrimRight(axisRow, " ")); got < lineChartPadRight {
		t.Errorf("right pad = %d, want >= %d in %q", got, lineChartPadRight, axisRow)
	}
}

func TestOverlayCrosshair(t *testing.T) {
	grid := make([][]rune, 5)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", 8))
	}
	dot := brailleBase + 0x01
	grid[2][6] = dot
	overlayCrosshair(grid, 3, 1)
	if grid[1][3] != '┼' {
		t.Fatalf("intersection = %q, want ┼", string(grid[1][3]))
	}
	if grid[1][0] != '─' {
		t.Fatalf("horizontal = %q, want ─", string(grid[1][0]))
	}
	if grid[4][3] != '│' {
		t.Fatalf("vertical = %q, want │", string(grid[4][3]))
	}
	if grid[2][6] != dot {
		t.Fatalf("series cell overwritten: %q", string(grid[2][6]))
	}

	grid[1][3] = brailleBase + 0xFF
	overlayCrosshair(grid, 3, 1)
	if grid[1][3] != brailleBase+0xFF {
		t.Fatalf("intersection series overwritten: %q", string(grid[1][3]))
	}
}

func TestBrailleConnectsDistantPoints(t *testing.T) {
	grid := rasterLineBraille(16, 8, []chartPoint{
		{x: 0, y: 0},
		{x: 10, y: 10},
	}, 0, 10, 0, 10)
	filled := 0
	for _, row := range grid {
		for _, ch := range row {
			if ch != ' ' {
				if ch < brailleBase || ch > brailleBase+0xFF {
					t.Fatalf("non-Braille cell %q", string(ch))
				}
				filled++
			}
		}
	}
	if filled < 8 {
		t.Fatalf("filled cells = %d, want a continuous stroke across the plot", filled)
	}
}

func TestScatterDoesNotConnectDistantPoints(t *testing.T) {
	pts := []chartPoint{{x: 0, y: 0}, {x: 10, y: 10}}
	line := rasterLineBraille(16, 8, pts, 0, 10, 0, 10)
	scatter := rasterScatterBraille(16, 8, pts, 0, 10, 0, 10)
	lineN, scatterN := 0, 0
	for y := range line {
		for x, ch := range line[y] {
			if ch != ' ' {
				lineN++
			}
			if scatter[y][x] != ' ' {
				scatterN++
			}
		}
	}
	if scatterN < 1 {
		t.Fatal("scatter should plot at least one cell")
	}
	if scatterN >= lineN {
		t.Fatalf("scatter filled %d, line filled %d; scatter should not stroke between points", scatterN, lineN)
	}
}

func containsBraille(s string) bool {
	for _, r := range s {
		if r > brailleBase && r <= brailleBase+0xFF {
			return true
		}
	}
	return false
}

func TestLineChartCursorWalksPoints(t *testing.T) {
	c := NewChartPanel()
	c.SetSize(60, 12)
	c.ShowLine("line", []chartPoint{
		{x: 1, y: 1, xLabel: "1"},
		{x: 2, y: 2, xLabel: "2"},
		{x: 3, y: 3, xLabel: "3"},
	}, 0)
	c = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if c.cursor != 1 {
		t.Errorf("cursor = %d, want 1", c.cursor)
	}
	c = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if c.cursor != 0 {
		t.Errorf("cursor = %d, want 0", c.cursor)
	}
}

func TestLineChartEnterRelayoutsWithoutCommandLine(t *testing.T) {
	m := &Model{
		state:      stateWorkspace,
		width:      80,
		height:     24,
		editor:     NewQueryEditor(),
		results:    NewResultsTable(),
		chartPanel: NewChartPanel(),
	}
	m.results.SetResult([]string{"t", "v"}, [][]string{{"1", "4"}, {"2", "8"}}, "")
	*m = m.updateLayout()
	fullH := m.chartPanel.height

	m.ex.Open()
	m.ex.input = "line t v"
	m.layoutWorkspace()
	if m.chartPanel.height >= fullH {
		t.Fatalf("command line should shrink the chart: got %d, full %d", m.chartPanel.height, fullH)
	}

	got, _ := m.handleExKey(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := got.(Model)
	if m2.ex.IsVisible() {
		t.Fatal("command line should be hidden after :line")
	}
	if !m2.chartPanel.IsVisible() {
		t.Fatal("line chart should be visible")
	}
	if m2.chartPanel.height != fullH {
		t.Errorf("chart height = %d, want %d so j/k does not grow the panel", m2.chartPanel.height, fullH)
	}
}
