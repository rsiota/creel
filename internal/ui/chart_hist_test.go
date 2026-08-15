package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDefaultHistBins(t *testing.T) {
	if got := defaultHistBins(1); got != 1 {
		t.Errorf("n=1: %d, want 1", got)
	}
	if got := defaultHistBins(2); got != 8 {
		t.Errorf("n=2: %d, want 8 (Sturges clamped)", got)
	}
	if got := defaultHistBins(256); got != 9 {
		t.Errorf("n=256: %d, want 9", got)
	}
	if got := defaultHistBins(1_000_000); got != 20 {
		t.Errorf("n=1e6: %d, want 20", got)
	}
}

func TestBuildHistSeriesSkipsAndKeepsNegatives(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"n"}, [][]string{
		{"-5"}, {"NULL"}, {"x"}, {"0"}, {"5"},
	}, "")
	bars, skipped := buildHistSeries(r, 0, 4)
	if skipped != 2 {
		t.Errorf("skipped = %d, want 2", skipped)
	}
	if len(bars) != 4 {
		t.Fatalf("bars = %d, want 4: %+v", len(bars), bars)
	}
	var total int
	for _, b := range bars {
		total += b.n
	}
	if total != 3 {
		t.Errorf("binned count = %d, want 3", total)
	}
}

func TestBuildHistSeriesEmptyBins(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"n"}, [][]string{{"0"}, {"100"}}, "")
	bars, skipped := buildHistSeries(r, 0, 4)
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(bars) != 4 {
		t.Fatalf("bars = %d, want 4", len(bars))
	}
	if bars[0].n != 1 || bars[3].n != 1 {
		t.Errorf("ends = %d/%d, want 1/1: %+v", bars[0].n, bars[3].n, bars)
	}
	if bars[1].n != 0 || bars[2].n != 0 {
		t.Errorf("middle bins should be empty: %+v", bars)
	}
}

func TestBuildHistSeriesEqualValues(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"n"}, [][]string{{"3"}, {"3"}, {"3"}}, "")
	bars, skipped := buildHistSeries(r, 0, 8)
	if skipped != 0 || len(bars) != 1 {
		t.Fatalf("bars=%d skipped=%d, want 1 bar", len(bars), skipped)
	}
	if bars[0].value != 3 || bars[0].label != "3" {
		t.Errorf("bar = %+v, want label 3 value 3", bars[0])
	}
}

func TestExHistFromCursorAndBins(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"name", "n"}, [][]string{
		{"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"},
	}, "")
	m.results.SetCursor(0, 1)
	if cmd := m.exHist([]string{"2"}, false); cmd != nil {
		t.Fatal("page hist should not return a cmd")
	}
	if m.chartPanel.kind != chartKindHist {
		t.Fatalf("kind = %v, want hist", m.chartPanel.kind)
	}
	if !m.chartPanel.expanded {
		t.Fatal("hist should not fold into (other)")
	}
	if len(m.chartPanel.bars) != 2 {
		t.Fatalf("bars = %d, want 2 bins", len(m.chartPanel.bars))
	}
	if !strings.Contains(m.chartPanel.title, "n") || !strings.Contains(m.chartPanel.title, "2 bins") {
		t.Errorf("title = %q", m.chartPanel.title)
	}
}

func TestExHistFromMark(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"name", "n"}, [][]string{{"a", "1"}, {"b", "10"}}, "")
	m.results.SetCursor(0, 1)
	m.results.ToggleColumnMark()
	m.results.SetCursor(0, 0)
	if cmd := m.exHist(nil, false); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if m.chartPanel.kind != chartKindHist {
		t.Fatal("expected hist")
	}
	if !strings.Contains(m.chartPanel.title, "n") {
		t.Errorf("title = %q, want marked column n", m.chartPanel.title)
	}
}

func TestExHistErrors(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.exHist(nil, false)
	if m.schemaMsg == "" {
		t.Fatal("expected error with no results")
	}

	m.schemaMsg = ""
	m.results.SetResult([]string{"a", "b"}, [][]string{{"x", "y"}}, "")
	m.results.SetCursor(0, 0)
	m.results.ToggleColumnMark()
	m.results.SetCursor(0, 1)
	m.results.ToggleColumnMark()
	m.exHist(nil, false)
	if !strings.Contains(m.schemaMsg, "mark 1") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	m.schemaMsg = ""
	m.results.ClearColumnMarks()
	m.exHist([]string{"missing"}, false)
	if !strings.Contains(m.schemaMsg, "no such column") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	m.schemaMsg = ""
	m.exHist([]string{"a"}, false)
	if !strings.Contains(m.schemaMsg, "no numeric") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
	if m.chartPanel.IsVisible() {
		t.Fatal("chart should stay hidden on error")
	}

	m.schemaMsg = ""
	m.exHist([]string{"0"}, false)
	if !strings.Contains(m.schemaMsg, "invalid bin count") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestHistDoesNotFold(t *testing.T) {
	bars := make([]chartBar, chartBarLimit+5)
	for i := range bars {
		bars[i] = chartBar{label: "b", value: float64(i + 1), n: 1}
	}
	c := NewChartPanel()
	c.ShowHist("hist · n", bars, 0)
	if c.kind != chartKindHist || !c.expanded {
		t.Fatal("ShowHist should set hist + expanded")
	}
	if got := len(c.visibleBars()); got != len(bars) {
		t.Errorf("visible = %d, want %d (no (other) fold)", got, len(bars))
	}
	c = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if !c.expanded || len(c.visibleBars()) != len(bars) {
		t.Fatal("o should not fold a histogram")
	}
}
