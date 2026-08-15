package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func applyChartCmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected async chart command")
	}
	msg := cmd()
	ready, ok := msg.(chartReadyMsg)
	if !ok {
		t.Fatalf("got %T: %v", msg, msg)
	}
	updated, _ := m.Update(ready)
	*m = updated.(Model)
}

func TestExBarPageDoesNotRequery(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	m.lastQuery = "SELECT * FROM users"
	if cmd := m.exBar([]string{"name", "id"}, false); cmd != nil {
		t.Fatal("page :bar should not return a cmd")
	}
	if len(m.chartPanel.bars) != 1 {
		t.Fatalf("page bars = %d, want 1", len(m.chartPanel.bars))
	}
	if strings.Contains(m.chartPanel.title, " · all") {
		t.Errorf("page title should not say all: %q", m.chartPanel.title)
	}
}

func TestExBarBangChartsAllRows(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	m.lastQuery = "SELECT * FROM users"
	applyChartCmd(t, &m, m.exBar([]string{"name", "id"}, true))
	if !m.chartPanel.IsVisible() {
		t.Fatal("chart should be visible")
	}
	if len(m.chartPanel.bars) != 3 {
		t.Fatalf("bang bars = %d, want 3 (all rows)", len(m.chartPanel.bars))
	}
	if !strings.Contains(m.chartPanel.title, " · all") {
		t.Errorf("title = %q, want · all", m.chartPanel.title)
	}
}

func TestExHistBangChartsAllRows(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	m.lastQuery = "SELECT * FROM users"
	if cmd := m.exHist([]string{"id", "4"}, false); cmd != nil {
		t.Fatal("page :hist should not return a cmd")
	}
	if m.chartPanel.kind != chartKindHist {
		t.Fatal("expected hist")
	}
	if len(m.chartPanel.bars) != 1 {
		t.Fatalf("page hist bars = %d, want 1 (single value)", len(m.chartPanel.bars))
	}

	m.chartPanel.Hide()
	applyChartCmd(t, &m, m.exHist([]string{"id", "4"}, true))
	if m.chartPanel.kind != chartKindHist {
		t.Fatal("expected hist after bang")
	}
	if len(m.chartPanel.bars) != 4 {
		t.Fatalf("bang hist bars = %d, want 4 bins", len(m.chartPanel.bars))
	}
	var n int
	for _, b := range m.chartPanel.bars {
		n += b.n
	}
	if n != 3 {
		t.Errorf("binned rows = %d, want 3", n)
	}
}

func TestExBarBangNeedsQuery(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel()}
	m.results.SetResult([]string{"a", "b"}, [][]string{{"x", "1"}}, "")
	if cmd := m.exBar([]string{"a", "b"}, true); cmd != nil {
		t.Fatal("bang without connection should not return a cmd")
	}
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	m.schemaMsg = ""
	wired, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	m.connection = wired.connection
	if cmd := m.exBar([]string{"a", "b"}, true); cmd != nil {
		t.Fatal("bang without lastQuery should not return a cmd")
	}
	if !strings.Contains(m.schemaMsg, "no query") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExLineBangChartsAllRows(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	m.lastQuery = "SELECT * FROM users"
	if cmd := m.exLine([]string{"id", "id"}, false); cmd != nil {
		t.Fatal("page :line should not return a cmd")
	}
	if len(m.chartPanel.points) != 1 {
		t.Fatalf("page points = %d, want 1", len(m.chartPanel.points))
	}
	m.chartPanel.Hide()
	applyChartCmd(t, &m, m.exLine([]string{"id", "id"}, true))
	if len(m.chartPanel.points) != 3 {
		t.Fatalf("bang points = %d, want 3", len(m.chartPanel.points))
	}
	if !strings.Contains(m.chartPanel.title, " · all") {
		t.Errorf("title = %q", m.chartPanel.title)
	}
}

func TestRunExCommandBarBang(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	m.lastQuery = "SELECT * FROM users"
	cmd := m.runExCommand("bar! name id")
	applyChartCmd(t, &m, cmd)
	if len(m.chartPanel.bars) != 3 {
		t.Fatalf("bars = %d, want 3", len(m.chartPanel.bars))
	}
}

func TestExFreqBangChartsAllRows(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	m.lastQuery = "SELECT * FROM users"
	if cmd := m.exFreq([]string{"name"}, false); cmd != nil {
		t.Fatal("page :freq should not return a cmd")
	}
	if len(m.chartPanel.bars) != 1 || m.chartPanel.bars[0].label != "Ada" {
		t.Fatalf("page freq = %+v, want Ada", m.chartPanel.bars)
	}
	m.chartPanel.Hide()
	applyChartCmd(t, &m, m.exFreq([]string{"name"}, true))
	if len(m.chartPanel.bars) != 3 {
		t.Fatalf("bang freq bars = %d, want 3", len(m.chartPanel.bars))
	}
	if !strings.Contains(m.chartPanel.title, "freq · name") || !strings.Contains(m.chartPanel.title, " · all") {
		t.Errorf("title = %q", m.chartPanel.title)
	}
}
