package ui

import (
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
	bars, skipped := buildBarSeries(r, 0, 1)
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
		{label: "alice", value: 10},
		{label: "bob", value: 5},
	}, 0)
	out := c.View()
	if got := lipgloss.Height(out); got != 12 {
		t.Errorf("height = %d, want 12\n%s", got, out)
	}
	if !strings.Contains(stripANSI(out), "alice") {
		t.Errorf("missing alice:\n%s", out)
	}
	if !strings.Contains(out, "█") {
		t.Errorf("missing bar glyphs:\n%s", out)
	}
}

func TestChartPanelClosesOnEsc(t *testing.T) {
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.results.SetResult([]string{"a", "b"}, [][]string{{"x", "1"}}, "")
	m.chartPanel.ShowBar("bar · a × b", []chartBar{{label: "x", value: 1}}, 0)

	mm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm2.(Model)
	if m.chartPanel.IsVisible() {
		t.Fatal("esc should close chart")
	}
}
