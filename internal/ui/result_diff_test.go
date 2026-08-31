package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAlignDiffColumns(t *testing.T) {
	union, aIdx, bIdx := alignDiffColumns(
		[]string{"id", "name"},
		[]string{"NAME", "extra"},
	)
	if len(union) != 3 || union[0] != "id" || !strings.EqualFold(union[1], "name") || union[2] != "extra" {
		t.Fatalf("union = %v", union)
	}
	if aIdx[0] != 0 || aIdx[1] != 1 || aIdx[2] != -1 {
		t.Fatalf("aIdx = %v", aIdx)
	}
	if bIdx[0] != -1 || bIdx[1] != 0 || bIdx[2] != 1 {
		t.Fatalf("bIdx = %v", bIdx)
	}
}

func TestComputeResultDiffByIndex(t *testing.T) {
	a := diffSnapshot{
		title: "A",
		cols:  []string{"id", "name"},
		rows: [][]string{
			{"1", "alice"},
			{"2", "bob"},
			{"3", "carol"},
		},
	}
	b := diffSnapshot{
		title: "B",
		cols:  []string{"id", "name"},
		rows: [][]string{
			{"1", "alice"},
			{"2", "bobby"},
			{"4", "dave"},
		},
	}
	d := computeResultDiff(a, b)
	if d.Mode != "row" {
		t.Fatalf("mode = %q, want row", d.Mode)
	}
	if d.Same != 1 || d.Changed != 2 || d.Removed != 0 || d.Added != 0 {
		t.Fatalf("counts same=%d changed=%d removed=%d added=%d", d.Same, d.Changed, d.Removed, d.Added)
	}
}

func TestComputeResultDiffByPK(t *testing.T) {
	a := diffSnapshot{
		title:       "A",
		cols:        []string{"id", "name"},
		rows:        [][]string{{"1", "alice"}, {"2", "bob"}, {"3", "carol"}},
		pkCols:      []string{"id"},
		sourceTable: "users",
	}
	b := diffSnapshot{
		title:       "B",
		cols:        []string{"id", "name"},
		rows:        [][]string{{"2", "bobby"}, {"3", "carol"}, {"4", "dave"}},
		pkCols:      []string{"id"},
		sourceTable: "users",
	}
	d := computeResultDiff(a, b)
	if d.Mode != "pk" {
		t.Fatalf("mode = %q, want pk", d.Mode)
	}
	// 1 removed, 2 changed, 3 same, 4 added
	if d.Removed != 1 || d.Changed != 1 || d.Same != 1 || d.Added != 1 {
		t.Fatalf("counts same=%d changed=%d removed=%d added=%d", d.Same, d.Changed, d.Removed, d.Added)
	}
}

func TestComputeResultDiffColumnUnion(t *testing.T) {
	a := diffSnapshot{title: "A", cols: []string{"id", "a"}, rows: [][]string{{"1", "x"}}}
	b := diffSnapshot{title: "B", cols: []string{"id", "b"}, rows: [][]string{{"1", "y"}}}
	d := computeResultDiff(a, b)
	if len(d.Cols) != 3 {
		t.Fatalf("cols = %v", d.Cols)
	}
	if d.Changed != 1 {
		t.Fatalf("want 1 changed (missing cols differ as —), got %d", d.Changed)
	}
}

func TestDiffPanelShowAndToggle(t *testing.T) {
	d := computeResultDiff(
		diffSnapshot{title: "A", cols: []string{"id"}, rows: [][]string{{"1"}, {"2"}}},
		diffSnapshot{title: "B", cols: []string{"id"}, rows: [][]string{{"1"}, {"3"}}},
	)
	var p DiffPanel
	p.Show(d)
	if !p.IsVisible() {
		t.Fatal("expected visible")
	}
	if !p.changesOnly {
		t.Fatal("default changes-only")
	}
	// changes-only: same row hidden → fewer visible than entries
	if len(p.visibleIdx) >= len(d.Entries) {
		t.Fatalf("visibleIdx=%d entries=%d", len(p.visibleIdx), len(d.Entries))
	}
	p = p.Update(teaKey("a"))
	if p.changesOnly {
		t.Fatal("a should show all rows")
	}
	if len(p.visibleIdx) != len(d.Entries) {
		t.Fatalf("all rows visibleIdx=%d want %d", len(p.visibleIdx), len(d.Entries))
	}
	p.SetSize(80, 24)
	view := p.View()
	if !strings.Contains(view, "diff") {
		t.Fatalf("view missing title:\n%s", view)
	}
}

func teaKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
