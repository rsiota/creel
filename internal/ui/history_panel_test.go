package ui

import (
	"testing"
	"time"

	"github.com/ruben/gsql/internal/history"
)

// TestHistoryPanelSlowestSort confirms ToggleSort reorders the visible list by
// elapsed descending while preserving the displayed rank (origIdx+1) that
// :rerun indexes by — it is only a view reordering.
func TestHistoryPanelSlowestSort(t *testing.T) {
	p := NewHistoryPanel()
	p.SetEntries([]history.Entry{
		{Query: "fast", Elapsed: 10 * time.Millisecond}, // rank 1 (oldest)
		{Query: "slow", Elapsed: 5 * time.Second},       // rank 2
		{Query: "mid", Elapsed: 200 * time.Millisecond}, // rank 3 (most recent)
	})

	// Most-recent-first (default): mid, slow, fast. SetEntries reversed the
	// input, so origIdx 0 = most recent (the displayed rank 1).
	got := p.filteredEntries()
	if got[0].entry.Query != "mid" || got[1].entry.Query != "slow" || got[2].entry.Query != "fast" {
		t.Fatalf("recent order: %s %s %s", got[0].entry.Query, got[1].entry.Query, got[2].entry.Query)
	}
	if got[0].origIdx != 0 { // mid is most recent → rank 1
		t.Errorf("mid origIdx = %d, want 0", got[0].origIdx)
	}

	p.ToggleSort()
	if !p.SortBySlowest() {
		t.Fatal("expected sort by slowest after toggle")
	}
	got = p.filteredEntries()
	// Slowest-first: slow(5s), mid(200ms), fast(10ms).
	if got[0].entry.Query != "slow" || got[1].entry.Query != "mid" || got[2].entry.Query != "fast" {
		t.Fatalf("slowest order: %s %s %s", got[0].entry.Query, got[1].entry.Query, got[2].entry.Query)
	}
	// Ranks are unchanged by the reordering: slow was rank 2 (origIdx 1).
	if got[0].origIdx != 1 {
		t.Errorf("slow origIdx = %d, want 1", got[0].origIdx)
	}
}

// TestHistoryPanelSlowestSortZeroSinks confirms untracked (zero-elapsed) entries
// sort to the bottom in slowest mode, since they carry no timing information.
func TestHistoryPanelSlowestSortZeroSinks(t *testing.T) {
	p := NewHistoryPanel()
	p.SetEntries([]history.Entry{
		{Query: "legacy", Elapsed: 0}, // no timing
		{Query: "timed", Elapsed: 100 * time.Millisecond},
	})
	p.ToggleSort()
	got := p.filteredEntries()
	if got[0].entry.Query != "timed" || got[1].entry.Query != "legacy" {
		t.Fatalf("zero should sink: %s %s", got[0].entry.Query, got[1].entry.Query)
	}
}
