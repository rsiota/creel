package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/config"
)

func TestToggleColumnMarkOrdered(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"name", "amount", "extra"}, [][]string{
		{"alice", "10", "x"},
		{"bob", "20", "y"},
	}, "")

	r.SetCursor(0, 0)
	if !r.ToggleColumnMark() {
		t.Fatal("first mark should succeed")
	}
	r.SetCursor(0, 1)
	if !r.ToggleColumnMark() {
		t.Fatal("second mark should succeed")
	}
	got := r.MarkedColumns()
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("marked = %v, want [0 1]", got)
	}
	if r.ColumnMarkOrdinal(0) != 1 || r.ColumnMarkOrdinal(1) != 2 {
		t.Fatalf("ordinals = %d/%d, want 1/2", r.ColumnMarkOrdinal(0), r.ColumnMarkOrdinal(1))
	}

	r.SetCursor(0, 2)
	if r.ToggleColumnMark() {
		t.Fatal("third mark should fail")
	}
	if r.ColumnMarkCount() != 2 {
		t.Fatalf("count = %d after failed third mark", r.ColumnMarkCount())
	}

	// Toggle off the first mark.
	r.SetCursor(0, 0)
	if !r.ToggleColumnMark() {
		t.Fatal("unmark should succeed")
	}
	got = r.MarkedColumns()
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("after unmark = %v, want [1]", got)
	}
}

func TestColumnMarksClearedOnSetResult(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"a", "b"}, [][]string{{"1", "2"}}, "")
	r.SetCursor(0, 0)
	r.ToggleColumnMark()
	r.SetResult([]string{"a", "b"}, [][]string{{"3", "4"}}, "")
	if r.ColumnMarkCount() != 0 {
		t.Fatalf("marks survived SetResult: %v", r.MarkedColumns())
	}
}

func TestClearAllMarks(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}}, "")
	r.SetEditable("t", []string{"id"})
	r.ToggleMark()
	r.SetCursor(0, 1)
	r.ToggleColumnMark()
	r.ClearAllMarks()
	if r.MarkCount() != 0 || r.ColumnMarkCount() != 0 {
		t.Fatalf("expected empty marks, row=%d col=%d", r.MarkCount(), r.ColumnMarkCount())
	}
}

func TestBuildBarSeriesSkipsNonNumeric(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"name", "n"}, [][]string{
		{"alice", "10"},
		{"bob", "NULL"},
		{"cara", "x"},
		{"dan", "3.5"},
		{"erin", "-1"},
	}, "")
	bars, skipped := buildBarSeries(r, 0, 1, barAggSum)
	if len(bars) != 2 {
		t.Fatalf("bars = %d, want 2: %+v", len(bars), bars)
	}
	if bars[0].label != "alice" || bars[0].value != 10 {
		t.Errorf("bar0 = %+v", bars[0])
	}
	if bars[1].label != "dan" || bars[1].value != 3.5 {
		t.Errorf("bar1 = %+v", bars[1])
	}
	if skipped != 3 {
		t.Errorf("skipped = %d, want 3", skipped)
	}
}

func TestBuildBarSeriesAggregates(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"name", "n"}, [][]string{
		{"alice", "10"},
		{"bob", "5"},
		{"alice", "20"},
		{"bob", "NULL"},
	}, "")

	sum, skipped := buildBarSeries(r, 0, 1, barAggSum)
	if skipped != 1 {
		t.Errorf("sum skipped = %d, want 1", skipped)
	}
	if len(sum) != 2 {
		t.Fatalf("sum bars = %d, want 2: %+v", len(sum), sum)
	}
	if sum[0].label != "alice" || sum[0].value != 30 {
		t.Errorf("sum[0] = %+v, want alice 30", sum[0])
	}
	if sum[1].label != "bob" || sum[1].value != 5 {
		t.Errorf("sum[1] = %+v, want bob 5", sum[1])
	}

	avg, _ := buildBarSeries(r, 0, 1, barAggAvg)
	if len(avg) != 2 || avg[0].label != "alice" || avg[0].value != 15 {
		t.Errorf("avg = %+v, want alice 15 first", avg)
	}

	cnt, skipped := buildBarSeries(r, 0, 1, barAggCount)
	if skipped != 0 {
		t.Errorf("count skipped = %d, want 0", skipped)
	}
	if len(cnt) != 2 {
		t.Fatalf("count bars = %d, want 2", len(cnt))
	}
	if cnt[0].label != "alice" || cnt[0].value != 2 {
		t.Errorf("count[0] = %+v, want alice 2", cnt[0])
	}
	if cnt[1].label != "bob" || cnt[1].value != 2 {
		t.Errorf("count[1] = %+v, want bob 2", cnt[1])
	}
}

func TestBuildBarSeriesCapsOther(t *testing.T) {
	rows := make([][]string, chartBarLimit+5)
	for i := range rows {
		rows[i] = []string{fmt.Sprintf("L%02d", i), fmt.Sprintf("%d", i+1)}
	}
	r := NewResultsTable()
	r.SetResult([]string{"name", "n"}, rows, "")

	bars, skipped := buildBarSeries(r, 0, 1, barAggSum)
	if skipped != 0 {
		t.Errorf("skipped = %d", skipped)
	}
	if len(bars) != chartBarLimit+5 {
		t.Fatalf("uncapped bars = %d, want %d", len(bars), chartBarLimit+5)
	}
	folded := foldChartBars(bars, false, barAggSum)
	if len(folded) != chartBarLimit+1 {
		t.Fatalf("folded = %d, want %d", len(folded), chartBarLimit+1)
	}
	if folded[0].label != fmt.Sprintf("L%02d", chartBarLimit+4) {
		t.Errorf("largest = %q, want L%02d", folded[0].label, chartBarLimit+4)
	}
	other := folded[len(folded)-1]
	if other.label != otherBarLabel {
		t.Fatalf("last = %q, want %s", other.label, otherBarLabel)
	}
	// Folded labels L00..L04 → 1+2+3+4+5 = 15
	if other.value != 15 {
		t.Errorf("(other) = %v, want 15", other.value)
	}
	all := foldChartBars(bars, true, barAggSum)
	if len(all) != len(bars) {
		t.Fatalf("expanded = %d, want %d", len(all), len(bars))
	}
}

func TestExBarFromMarks(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"user", "hits"}, [][]string{
		{"alice", "5"},
		{"bob", "12"},
	}, "")
	m.results.SetCursor(0, 0)
	m.results.ToggleColumnMark()
	m.results.SetCursor(0, 1)
	m.results.ToggleColumnMark()

	if cmd := m.exBar(nil); cmd != nil {
		t.Fatal("exBar should not return a cmd")
	}
	if !m.chartPanel.IsVisible() {
		t.Fatal("chart should be visible")
	}
	if len(m.chartPanel.bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(m.chartPanel.bars))
	}
	if !strings.Contains(m.chartPanel.title, "user") || !strings.Contains(m.chartPanel.title, "hits") {
		t.Errorf("title = %q", m.chartPanel.title)
	}
	if !strings.Contains(m.chartPanel.title, "sum") {
		t.Errorf("title missing default agg: %q", m.chartPanel.title)
	}
}

func TestExBarFromArgs(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"user", "hits"}, [][]string{{"alice", "5"}}, "")
	if cmd := m.exBar([]string{"user", "hits"}); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if !m.chartPanel.IsVisible() {
		t.Fatal("chart should be visible")
	}

	m.chartPanel.Hide()
	if cmd := m.exBar([]string{"user", "hits", "avg"}); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if !strings.Contains(m.chartPanel.title, "avg") {
		t.Errorf("title = %q, want avg", m.chartPanel.title)
	}
}

func TestExBarCountFromMarks(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"user", "hits"}, [][]string{
		{"alice", "5"},
		{"alice", "1"},
		{"bob", "x"},
	}, "")
	m.results.SetCursor(0, 0)
	m.results.ToggleColumnMark()
	m.results.SetCursor(0, 1)
	m.results.ToggleColumnMark()

	if cmd := m.exBar([]string{"count"}); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if len(m.chartPanel.bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(m.chartPanel.bars))
	}
	if m.chartPanel.bars[0].label != "alice" || m.chartPanel.bars[0].value != 2 {
		t.Errorf("bar0 = %+v, want alice 2", m.chartPanel.bars[0])
	}
}

func TestExBarUnknownAggregate(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"a", "b"}, [][]string{{"x", "1"}}, "")
	m.exBar([]string{"a", "b", "median"})
	if !strings.Contains(m.schemaMsg, "unknown aggregate") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
	if m.chartPanel.IsVisible() {
		t.Fatal("chart should stay hidden")
	}
}

func TestExBarErrors(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.exBar(nil)
	if m.schemaMsg == "" {
		t.Fatal("expected error with no results")
	}

	m.schemaMsg = ""
	m.results.SetResult([]string{"a", "b"}, [][]string{{"x", "y"}}, "")
	m.exBar(nil)
	if !strings.Contains(m.schemaMsg, "mark 2") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	m.schemaMsg = ""
	m.exBar([]string{"missing", "b"})
	if !strings.Contains(m.schemaMsg, "no such column") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	m.schemaMsg = ""
	m.exBar([]string{"a", "b"})
	if !strings.Contains(m.schemaMsg, "no numeric") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
	if m.chartPanel.IsVisible() {
		t.Fatal("chart should stay hidden on error")
	}
}

func TestChartPanelViewExactHeight(t *testing.T) {
	c := NewChartPanel()
	c.SetSize(60, 12)
	c.ShowBar("bar · a × b", []chartBar{
		{label: "alice", value: 10, n: 1},
		{label: "bob", value: 5, n: 1},
	}, 0, barAggSum)
	out := c.View()
	if got := lipgloss.Height(out); got != 12 {
		t.Errorf("height = %d, want 12\n%s", got, out)
	}
	plain := stripANSI(out)
	if strings.Contains(plain, "bar ·") {
		t.Errorf("title should not appear in the panel:\n%s", out)
	}
	if !strings.Contains(plain, "alice") {
		t.Errorf("missing alice:\n%s", out)
	}
	if !strings.Contains(out, "█") {
		t.Errorf("missing bar glyphs:\n%s", out)
	}
}

func TestChartPanelFillsViewport(t *testing.T) {
	bars := make([]chartBar, 20)
	for i := range bars {
		bars[i] = chartBar{label: fmt.Sprintf("r%d", i), value: float64(i + 1)}
	}
	c := NewChartPanel()
	c.SetSize(60, 12) // contentHeight = 10
	c.ShowBar("bar · a × b", bars, 0, barAggSum)
	if c.viewport() != 10 {
		t.Errorf("viewport = %d, want 10 (full content height)", c.viewport())
	}
	lines := c.bodyLines(c.contentWidth())
	if len(lines) != 10 {
		t.Errorf("body lines = %d, want 10", len(lines))
	}
	if !strings.Contains(stripANSI(lines[0]), "r0") {
		t.Errorf("first line should be a bar, got %q", stripANSI(lines[0]))
	}
	if !strings.Contains(stripANSI(lines[9]), "r9") {
		t.Errorf("last line should be a bar, got %q", stripANSI(lines[9]))
	}

	c.ShowBar("bar · a × b", bars, 3, barAggSum)
	if c.viewport() != 9 {
		t.Errorf("viewport with footer = %d, want 9", c.viewport())
	}
	lines = c.bodyLines(c.contentWidth())
	if len(lines) != 10 {
		t.Errorf("body with footer = %d, want 10", len(lines))
	}
	if !strings.Contains(stripANSI(lines[9]), "skipped") {
		t.Errorf("last line should be skipped note, got %q", stripANSI(lines[9]))
	}
}

func TestChartPanelUnfoldOther(t *testing.T) {
	bars := make([]chartBar, chartBarLimit+3)
	for i := range bars {
		bars[i] = chartBar{label: fmt.Sprintf("r%d", i), value: float64(100 - i), n: 1}
	}
	c := NewChartPanel()
	c.SetSize(60, 16)
	c.ShowBar("bar · a × b", bars, 0, barAggSum)

	vis := c.visibleBars()
	if len(vis) != chartBarLimit+1 {
		t.Fatalf("folded visible = %d, want %d", len(vis), chartBarLimit+1)
	}
	if vis[len(vis)-1].label != otherBarLabel {
		t.Fatalf("last folded = %q", vis[len(vis)-1].label)
	}

	c = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if !c.expanded {
		t.Fatal("o should expand")
	}
	vis = c.visibleBars()
	if len(vis) != len(bars) {
		t.Fatalf("expanded visible = %d, want %d", len(vis), len(bars))
	}
	for _, b := range vis {
		if b.label == otherBarLabel {
			t.Fatal("expanded view should not include (other)")
		}
	}
	if c.cursor != chartBarLimit {
		t.Errorf("cursor after unfold = %d, want %d (first new bar)", c.cursor, chartBarLimit)
	}

	c = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if c.expanded {
		t.Fatal("o should fold again")
	}
	if c.cursor != chartBarLimit {
		t.Errorf("cursor after fold = %d, want %d ((other))", c.cursor, chartBarLimit)
	}
}

func TestChartPanelCursorMovesWithoutScrolling(t *testing.T) {
	bars := make([]chartBar, 20)
	for i := range bars {
		bars[i] = chartBar{label: fmt.Sprintf("r%d", i), value: float64(i + 1), n: 1}
	}
	c := NewChartPanel()
	c.SetSize(60, 12) // contentHeight = 10
	c.ShowBar("bar · a × b", bars, 0, barAggSum)
	if c.viewport() != 10 {
		t.Fatalf("viewport = %d, want 10", c.viewport())
	}

	for i := 0; i < 9; i++ {
		c = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	if c.cursor != 9 {
		t.Errorf("cursor = %d, want 9", c.cursor)
	}
	if c.scroll != 0 {
		t.Errorf("scroll = %d, want 0 (chart should stay put)", c.scroll)
	}
	lines := c.bodyLines(c.contentWidth())
	if !strings.Contains(stripANSI(lines[0]), "r0") {
		t.Errorf("top bar scrolled away: %q", stripANSI(lines[0]))
	}

	c = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if c.cursor != 10 {
		t.Errorf("cursor = %d, want 10", c.cursor)
	}
	if c.scroll != 1 {
		t.Errorf("scroll = %d, want 1 (follow cursor off the bottom)", c.scroll)
	}
}

func TestChartPanelSizedByLayout(t *testing.T) {
	m := &Model{state: stateWorkspace, width: 80, height: 24, editor: NewQueryEditor()}
	*m = m.updateLayout()
	if m.chartPanel.height == 0 {
		t.Fatal("chartPanel.height is 0 after updateLayout — SetSize in View does not persist")
	}
	if vh := m.chartPanel.viewport(); vh < 2 {
		t.Errorf("chart viewport = %d after layout, want >=2 (a 1-line viewport scrolls on every move)", vh)
	}
}

func TestChartPanelClosesOnEsc(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.results.SetResult([]string{"a", "b"}, [][]string{{"x", "1"}}, "")
	m.chartPanel.ShowBar("bar · a × b", []chartBar{{label: "x", value: 1, n: 1}}, 0, barAggSum)

	mm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm2.(Model)
	if m.chartPanel.IsVisible() {
		t.Fatal("esc should close chart")
	}
}
