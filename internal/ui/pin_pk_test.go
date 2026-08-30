package ui

import (
	"strings"
	"testing"
)

func TestPinnedPKStaysVisibleWhenScrolling(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(28, 12) // narrow: only PK + one data column fit
	r.SetResult(
		[]string{"id", "name", "email", "city"},
		[][]string{
			{"1", "alice", "a@test.com", "NYC"},
			{"2", "bob", "b@test.com", "LA"},
		},
		"2 rows",
	)
	r.SetEditable("users", []string{"id"})

	vis := r.visibleColRange()
	if len(vis) < 2 || vis[0] != 0 {
		t.Fatalf("initial visible = %v, want id pinned first", vis)
	}

	// Scroll into later columns.
	r.ScrollRight()
	r.ScrollRight()
	vis = r.visibleColRange()
	if len(vis) == 0 || vis[0] != 0 {
		t.Fatalf("after scroll visible = %v, want id still pinned", vis)
	}
	for _, c := range vis[1:] {
		if c == 0 {
			t.Fatalf("id should not appear twice: %v", vis)
		}
	}
	// Scrolling region should have moved past early unpinned cols.
	if vis[len(vis)-1] < 2 {
		t.Fatalf("expected later columns after scroll, got %v", vis)
	}
}

func TestPinnedPKBoundaryGlyph(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(60, 12)
	r.SetResult(
		[]string{"id", "name", "email"},
		[][]string{{"1", "alice", "a@test.com"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})

	view := r.View()
	if strings.Contains(view, "┃") || strings.Contains(view, "╂") {
		t.Fatalf("pin boundary should use normal separators, not heavy glyphs\n%s", view)
	}
}

func TestMidTablePKNotPinned(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(24, 12)
	r.SetResult(
		[]string{"name", "id", "email"},
		[][]string{{"alice", "1", "a@test.com"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"}) // PK is not a leading column

	if pins := r.pinnedCols(); len(pins) != 0 {
		t.Fatalf("mid-table PK should not pin, got %v", pins)
	}
	r.ScrollRight()
	vis := r.visibleColRange()
	// Without a leading pin, id can scroll out of the narrow window.
	for _, c := range vis {
		if c == 1 {
			// Still visible on this width — force another scroll.
			r.ScrollRight()
			vis = r.visibleColRange()
			break
		}
	}
	for _, c := range vis {
		if c == 1 {
			t.Fatalf("mid-table id should scroll away; visible=%v", vis)
		}
	}
}

func TestCompositeLeadingPKPinned(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(30, 12)
	r.SetResult(
		[]string{"tenant_id", "id", "name", "email"},
		[][]string{{"t1", "1", "alice", "a@test.com"}},
		"1 row",
	)
	r.SetEditable("users", []string{"tenant_id", "id"})

	pins := r.pinnedCols()
	if len(pins) != 2 || pins[0] != 0 || pins[1] != 1 {
		t.Fatalf("pins = %v, want [0 1]", pins)
	}
	r.ScrollRight()
	vis := r.visibleColRange()
	if len(vis) < 2 || vis[0] != 0 || vis[1] != 1 {
		t.Fatalf("composite PK should stay pinned, visible=%v", vis)
	}
}

func TestEnsureCursorVisibleWithPinnedPK(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(28, 12)
	r.SetResult(
		[]string{"id", "c1", "c2", "c3", "c4"},
		[][]string{{"1", "a", "b", "c", "d"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})
	r.SetCursor(0, 4) // far right
	r.ensureCursorVisible()
	vis := r.visibleColRange()
	hasID, hasC4 := false, false
	for _, c := range vis {
		if c == 0 {
			hasID = true
		}
		if c == 4 {
			hasC4 = true
		}
	}
	if !hasID || !hasC4 {
		t.Fatalf("expected id pinned and c4 visible; visible=%v scrollCol=%d", vis, r.scrollCol)
	}
}
