package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestComputeWatchDelta(t *testing.T) {
	prev := [][]string{{"a", "1"}, {"b", "2"}}
	next := [][]string{{"a", "1"}, {"c", "3"}, {"b", "2"}}
	delta := computeWatchDelta(prev, next)
	if delta[0] || !delta[1] || delta[2] {
		t.Fatalf("delta=%v, want only index 1", delta)
	}
	if computeWatchDelta(nil, next) != nil {
		t.Fatal("first snapshot should not highlight")
	}
	if computeWatchDelta(prev, prev) != nil {
		t.Fatal("identical pages should not highlight")
	}
}

func TestWatchDeltaTintsResults(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"n"}, [][]string{{"1"}, {"2"}}, "")
	r.SetWatchDelta(map[int]bool{1: true})
	if !r.IsWatchDeltaRow(1) || r.IsWatchDeltaRow(0) {
		t.Fatal("watch delta flags wrong")
	}
	r.SetResult([]string{"n"}, [][]string{{"1"}}, "")
	if r.IsWatchDeltaRow(0) || r.IsWatchDeltaRow(1) {
		t.Fatal("SetResult should clear watch delta")
	}
}

func TestRedrawLastChartOnRefresh(t *testing.T) {
	m := NewModel(&config.Config{})
	m.results.SetResult(
		[]string{"user", "hits"},
		[][]string{{"alice", "1"}, {"bob", "2"}},
		"",
	)
	cmd := m.exBar([]string{"user", "hits", "sum"}, false)
	if cmd != nil {
		t.Fatal("page :bar should be sync")
	}
	if !m.chartPanel.IsVisible() || !m.lastChartOK {
		t.Fatal("expected chart remembered")
	}
	oldTitle := m.chartPanel.title

	// Simulate a watch refresh with updated values.
	msg := queryExecutedMsg{
		query:  "SELECT user, hits FROM t",
		result: db.Result{Columns: []db.Column{{Name: "user"}, {Name: "hits"}}, Rows: [][]string{{"alice", "10"}, {"bob", "2"}, {"carol", "5"}}},
		page:   0,
		pageSize: 200,
	}
	m.watchActive = true
	m.watchPrevRows = [][]string{{"alice", "1"}, {"bob", "2"}}
	mm, _ := m.Update(msg)
	m = mm.(Model)

	if !m.chartPanel.IsVisible() {
		t.Fatal("chart should stay open across refresh")
	}
	if m.chartPanel.title != oldTitle && !strings.Contains(m.chartPanel.title, "user") {
		t.Fatalf("title=%q", m.chartPanel.title)
	}
	// alice's hits changed and carol is new — those fingerprints differ.
	if !m.results.IsWatchDeltaRow(0) { // alice 10 is new content
		t.Fatalf("expected delta on changed alice row; delta=%v rows=%v", m.results.watchDelta, m.results.rows)
	}
	if !m.results.IsWatchDeltaRow(2) { // carol
		t.Fatalf("expected delta on new carol row; delta=%v", m.results.watchDelta)
	}
	if m.results.IsWatchDeltaRow(1) { // bob unchanged
		t.Fatal("bob should not be highlighted")
	}
}

func TestManualQueryClosesChartWhenNotCharting(t *testing.T) {
	m := NewModel(&config.Config{})
	m.results.SetResult([]string{"a"}, [][]string{{"1"}}, "")
	_ = m.exBar([]string{"a"}, false)
	if !m.chartPanel.IsVisible() {
		t.Fatal("setup")
	}
	m.chartPanel.Hide()
	m.lastChartOK = true // still remembered, but panel closed
	msg := queryExecutedMsg{
		result:   db.Result{Columns: []db.Column{{Name: "a"}}, Rows: [][]string{{"2"}}},
		pageSize: 200,
	}
	mm, _ := m.Update(msg)
	m = mm.(Model)
	if m.chartPanel.IsVisible() {
		t.Fatal("closed chart should stay closed on next query")
	}
}
