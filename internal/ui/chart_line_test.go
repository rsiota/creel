package ui

import (
	"strings"
	"testing"

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
	if cmd := m.exLine([]string{"t", "v"}); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if !m.chartPanel.IsVisible() || m.chartPanel.kind != chartKindLine {
		t.Fatal("line chart should be visible")
	}
	if len(m.chartPanel.points) != 2 {
		t.Fatalf("points = %d, want 2", len(m.chartPanel.points))
	}
}

func TestExLineFromMarks(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"t", "v"}, [][]string{{"1", "4"}}, "")
	m.results.SetCursor(0, 0)
	m.results.ToggleColumnMark()
	m.results.SetCursor(0, 1)
	m.results.ToggleColumnMark()
	if cmd := m.exLine(nil); cmd != nil {
		t.Fatal("unexpected cmd")
	}
	if m.chartPanel.kind != chartKindLine {
		t.Fatal("expected line kind")
	}
}

func TestExLineErrors(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.exLine(nil)
	if m.schemaMsg == "" {
		t.Fatal("expected error with no results")
	}
	m.schemaMsg = ""
	m.results.SetResult([]string{"a", "b"}, [][]string{{"x", "y"}}, "")
	m.exLine(nil)
	if !strings.Contains(m.schemaMsg, "mark 2") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
	m.schemaMsg = ""
	m.exLine([]string{"a", "b"})
	if !strings.Contains(m.schemaMsg, "no numeric") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
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
	if !strings.Contains(plain, "●") {
		t.Errorf("missing points:\n%s", out)
	}
	if strings.Contains(plain, "bar ·") {
		t.Errorf("bar title leaked:\n%s", out)
	}
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
