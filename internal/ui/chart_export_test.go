package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestChartTitleSlug(t *testing.T) {
	if got := chartTitleSlug("bar · users × amount · sum"); got != "bar_users_amount_sum" {
		t.Fatalf("got %q", got)
	}
	if got := chartTitleSlug(""); got != "chart" {
		t.Fatalf("empty = %q", got)
	}
}

func TestParseChartExportFormat(t *testing.T) {
	f, ok := parseChartExportFormat("svg")
	if !ok || f != chartExportSVG {
		t.Fatalf("svg = %v %v", f, ok)
	}
	f, ok = parseChartExportFormat("")
	if !ok || f != chartExportTXT {
		t.Fatalf("default = %v %v", f, ok)
	}
	if _, ok := parseChartExportFormat("pdf"); ok {
		t.Fatal("pdf should be rejected")
	}
}

func TestSnapshotTextBar(t *testing.T) {
	c := NewChartPanel()
	c.ShowBar("bar · status · count", []chartBar{
		{label: "ok", value: 10},
		{label: "err", value: 3},
	}, 0, barAggCount)
	c.SetSize(80, 20)
	text := c.SnapshotText()
	if !strings.Contains(text, "bar · status · count") {
		t.Fatalf("missing title:\n%s", text)
	}
	if !strings.Contains(text, "ok") || !strings.Contains(text, "err") {
		t.Fatalf("missing labels:\n%s", text)
	}
	if strings.Contains(text, "\x1b[") {
		t.Fatal("snapshot must be plain (no ANSI)")
	}
}

func TestSnapshotSVGBarAndPie(t *testing.T) {
	c := NewChartPanel()
	c.ShowBar("demo", []chartBar{{label: "a", value: 2}, {label: "b", value: 5}}, 0, barAggSum)
	svg := c.SnapshotSVG()
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "<rect") {
		t.Fatalf("bar svg:\n%s", svg)
	}
	c.ShowPie("pie · status", []chartBar{{label: "ok", value: 7}, {label: "err", value: 3}}, 0)
	svg = c.SnapshotSVG()
	if !strings.Contains(svg, "<path") && !strings.Contains(svg, "<circle") {
		t.Fatalf("pie svg:\n%s", svg)
	}
}

func TestSnapshotSVGLine(t *testing.T) {
	c := NewChartPanel()
	c.ShowLine("line · x × y", []chartPoint{
		{x: 1, y: 2, xLabel: "1"},
		{x: 2, y: 4, xLabel: "2"},
		{x: 3, y: 3, xLabel: "3"},
	}, 0)
	svg := c.SnapshotSVG()
	if !strings.Contains(svg, "<polyline") || !strings.Contains(svg, "<circle") {
		t.Fatalf("line svg:\n%s", svg)
	}
}

func TestExportChartWritesDownloads(t *testing.T) {
	dir := t.TempDir()
	restore := SwapUserDownloadsDir(func() (string, error) { return dir, nil })
	t.Cleanup(restore)

	m := Model{chartPanel: NewChartPanel()}
	m.chartPanel.ShowBar("bar · demo", []chartBar{{label: "a", value: 1}}, 0, barAggCount)
	m.chartPanel.SetSize(60, 12)
	m.exportChart(chartExportTXT)
	if !strings.Contains(m.exportMsg, "exported chart →") {
		t.Fatalf("exportMsg = %q", m.exportMsg)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".txt") {
		t.Fatalf("files = %v", entries)
	}
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "bar · demo") {
		t.Fatalf("body = %s", body)
	}

	m.exportChart(chartExportSVG)
	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var sawSVG bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".svg") {
			sawSVG = true
		}
	}
	if !sawSVG {
		t.Fatalf("expected svg among %v", entries)
	}
}

func TestExportChartRequiresOpenChart(t *testing.T) {
	m := Model{}
	m.exportChart(chartExportTXT)
	if !strings.Contains(m.exportMsg, "no chart open") {
		t.Fatalf("exportMsg = %q", m.exportMsg)
	}
}

func TestExChartExportAndKey(t *testing.T) {
	dir := t.TempDir()
	restore := SwapUserDownloadsDir(func() (string, error) { return dir, nil })
	t.Cleanup(restore)

	m := Model{chartPanel: NewChartPanel(), width: 100, height: 30}
	m.chartPanel.ShowBar("bar · k", []chartBar{{label: "x", value: 9}}, 0, barAggCount)
	m.chartPanel.SetSize(80, 16)

	if cmd := m.runExCommand("chartexport svg"); cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if !strings.Contains(m.exportMsg, ".svg") {
		t.Fatalf("exportMsg = %q", m.exportMsg)
	}

	m.state = stateWorkspace
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if !strings.Contains(m.exportMsg, ".txt") {
		t.Fatalf("x key exportMsg = %q", m.exportMsg)
	}
}

func TestChartExportFilename(t *testing.T) {
	ts := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	got := chartExportFilename("bar · demo", chartExportSVG, ts)
	if got != "creel_chart_bar_demo_20260909_120000.svg" {
		t.Fatalf("got %q", got)
	}
}
