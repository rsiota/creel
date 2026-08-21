package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestIsDatetimeColumnName(t *testing.T) {
	yes := []string{"created_at", "updated_at", "deleted_at", "order_date", "ship_time", "timestamp", "DATE"}
	for _, n := range yes {
		if !isDatetimeColumnName(n) {
			t.Errorf("%q should be a datetime column name", n)
		}
	}
	no := []string{"id", "status", "email", "total", "attention", "ration"}
	for _, n := range no {
		if isDatetimeColumnName(n) {
			t.Errorf("%q should not be a datetime column name", n)
		}
	}
}

func TestCompactTimestamp(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"2026-08-21 14:32:05", "2026-08-21 14:32", true},
		{"2026-08-21T14:32:05Z", "2026-08-21 14:32", true},
		{"2026-08-21T14:32:05.123456+00:00", "2026-08-21 14:32", true},
		{"2026-08-21", "2026-08-21", true},
		{"14:32:05", "14:32", true},
		{"NULL", "", false},
		{"", "", false},
		{"not a time", "", false},
		{"pending", "", false},
	}
	for _, tc := range cases {
		got, ok := compactTimestamp(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("compactTimestamp(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestResultsTableCompactsCreatedAt(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(100, 12)
	r.SetResult(
		[]string{"id", "created_at", "note"},
		[][]string{{"1", "2026-08-21 14:32:05.123456", "hi"}},
		"1 row",
	)
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "created_at": "TEXT", "note": "TEXT"})
	r.SetCursor(0, 0)

	view := ansi.Strip(r.View())
	if strings.Contains(view, "14:32:05") {
		t.Errorf("raw seconds should be dropped from display:\n%s", view)
	}
	if !strings.Contains(view, "2026-08-21 14:32") {
		t.Errorf("expected compact datetime:\n%s", view)
	}
	// Raw value preserved for yank / edit.
	if got := r.RowValue(0, 1); got != "2026-08-21 14:32:05.123456" {
		t.Errorf("raw created_at = %q", got)
	}
}

func TestResultsTableDatetimeColWidthUsesCompact(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"created_at"},
		[][]string{{"2026-08-21 14:32:05.123456789"}},
		"1 row",
	)
	w := r.ColWidth(0)
	compactW := runeLen("2026-08-21 14:32")
	if w < compactW {
		t.Fatalf("width %d < compact %d", w, compactW)
	}
	// Should not size to the full fractional string.
	if w >= runeLen("2026-08-21 14:32:05.123456789") {
		t.Errorf("width %d still sized to raw fractional timestamp", w)
	}
}

func TestIsCellTruncatedUsesCompactDatetime(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"created_at"},
		[][]string{{"2026-08-21 14:32:05.123456789"}},
		"1 row",
	)
	if r.IsCellTruncated(0, 0) {
		t.Error("compact form fits column; should not report truncated")
	}
}
