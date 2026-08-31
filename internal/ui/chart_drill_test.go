package ui

import (
	"strings"
	"testing"
)

func TestChartBarFilterEquals(t *testing.T) {
	got := chartBarFilter("status", "TEXT", chartBar{label: "open"}, nil, chartKindBar)
	if got != "status = 'open'" {
		t.Errorf("got %q", got)
	}
}

func TestChartBarFilterNull(t *testing.T) {
	got := chartBarFilter("name", "TEXT", chartBar{label: "(null)"}, nil, chartKindBar)
	if got != "(name IS NULL OR name = '')" {
		t.Errorf("got %q", got)
	}
	got = chartBarFilter("n", "INTEGER", chartBar{label: "(null)"}, nil, chartKindBar)
	if got != "n IS NULL" {
		t.Errorf("got %q", got)
	}
}

func TestChartBarFilterOther(t *testing.T) {
	all := make([]chartBar, chartBarLimit+2)
	for i := range all {
		all[i] = chartBar{label: string(rune('a' + i)), n: 1}
	}
	got := chartBarFilter("k", "TEXT", chartBar{label: otherBarLabel}, all, chartKindBar)
	if !strings.Contains(got, " IN (") || !strings.Contains(got, "'u'") {
		t.Errorf("other filter = %q", got)
	}
	if strings.Contains(got, "'a'") {
		t.Errorf("other filter should not include the head: %q", got)
	}
}

func TestChartBarFilterHistRange(t *testing.T) {
	all := []chartBar{
		{label: "0–5"},
		{label: "5–10"},
	}
	got := chartBarFilter("n", "INTEGER", all[0], all, chartKindHist)
	if got != "n >= 0 AND n < 5" {
		t.Errorf("first bin = %q", got)
	}
	got = chartBarFilter("n", "INTEGER", all[1], all, chartKindHist)
	if got != "n >= 5 AND n <= 10" {
		t.Errorf("last bin = %q", got)
	}
}

func TestDrillChartBarKeepsMatchingRows(t *testing.T) {
	m := filterTestModel(t)
	m.chartPanel.ShowBar("freq · msg", []chartBar{
		{label: "a", value: 1, n: 1},
		{label: "b", value: 1, n: 1},
	}, 0, barAggCount)
	m.chartPanel.filterCol = "msg"
	m.chartPanel.cursor = 0
	cmd := m.drillChartBar()
	if cmd == nil {
		t.Fatal("expected a re-query cmd")
	}
	if m.chartPanel.IsVisible() {
		t.Fatal("chart should close")
	}
	if len(m.filters) != 1 || m.filters[0] != "msg = 'a'" {
		t.Errorf("filters = %v, want [msg = 'a']", m.filters)
	}
	if !strings.Contains(m.lastQuery, "msg = 'a'") {
		t.Errorf("lastQuery = %q", m.lastQuery)
	}
}

func TestDrillChartBarNeedsFilterableQuery(t *testing.T) {
	m := &Model{results: NewResultsTable(), chartPanel: NewChartPanel(), baseQuery: "SELECT 1"}
	m.chartPanel.ShowBar("bar · a", []chartBar{{label: "x", value: 1, n: 1}}, 0, barAggCount)
	m.chartPanel.filterCol = "a"
	m.drillChartBar()
	if !strings.Contains(m.schemaMsg, "can't filter") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestDrillChartBarIgnoresLine(t *testing.T) {
	m := filterTestModel(t)
	m.chartPanel.ShowLine("line · a × b", []chartPoint{{x: 1, y: 2}}, 0)
	m.drillChartBar()
	if !strings.Contains(m.schemaMsg, "bar") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
	if !m.chartPanel.IsVisible() {
		t.Fatal("line chart should stay open")
	}
}
