package ui

import (
	"strings"
	"testing"
	"time"

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

func TestFormatDatetimeDisplayRelative(t *testing.T) {
	now := time.Date(2026, 8, 30, 18, 0, 0, 0, time.Local)
	cases := []struct {
		in, want string
	}{
		{now.Add(-30 * time.Second).Format("2006-01-02 15:04:05"), "just now"},
		{now.Add(-5 * time.Minute).Format("2006-01-02 15:04:05"), "5m ago"},
		{now.Add(-3 * time.Hour).Format("2006-01-02 15:04:05"), "3h ago"},
		{now.Add(-2 * 24 * time.Hour).Format("2006-01-02 15:04:05"), "2d ago"},
		{now.Add(45 * time.Minute).Format("2006-01-02 15:04:05"), "in 45m"},
		{now.Add(-10 * 24 * time.Hour).Format("2006-01-02 15:04:05"), now.Add(-10 * 24 * time.Hour).Format("2006-01-02 15:04")},
		{now.Format("2006-01-02"), "today"},
		{now.Add(-24 * time.Hour).Format("2006-01-02"), "yesterday"},
		{now.Add(24 * time.Hour).Format("2006-01-02"), "tomorrow"},
		{now.Add(-3 * 24 * time.Hour).Format("2006-01-02"), "3d ago"},
		{"2020-01-15", "2020-01-15"},
		{"14:32:05", "14:32"},
	}
	for _, tc := range cases {
		got, ok := formatDatetimeDisplay(tc.in, now)
		if !ok || got != tc.want {
			t.Errorf("formatDatetimeDisplay(%q) = (%q, %v), want (%q, true)", tc.in, got, ok, tc.want)
		}
	}
}

func TestResultsTableShowsRelativeCreatedAt(t *testing.T) {
	now := time.Date(2026, 8, 30, 18, 0, 0, 0, time.Local)
	prev := datetimeNow
	datetimeNow = func() time.Time { return now }
	t.Cleanup(func() { datetimeNow = prev })

	raw := now.Add(-2 * time.Hour).Format("2006-01-02 15:04:05") + ".123456"
	r := NewResultsTable()
	r.SetSize(100, 12)
	r.SetResult(
		[]string{"id", "created_at", "note"},
		[][]string{{"1", raw, "hi"}},
		"1 row",
	)
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "created_at": "TEXT", "note": "TEXT"})
	r.SetCursor(0, 0)

	view := ansi.Strip(r.View())
	if strings.Contains(view, "15:04:05") || strings.Contains(view, raw) {
		t.Errorf("raw fractional timestamp should not appear:\n%s", view)
	}
	if !strings.Contains(view, "2h ago") {
		t.Errorf("expected relative datetime:\n%s", view)
	}
	if got := r.RowValue(0, 1); got != raw {
		t.Errorf("raw created_at = %q", got)
	}
}

func TestResultsTableDatetimeColWidthUsesDisplay(t *testing.T) {
	now := time.Date(2026, 8, 30, 18, 0, 0, 0, time.Local)
	prev := datetimeNow
	datetimeNow = func() time.Time { return now }
	t.Cleanup(func() { datetimeNow = prev })

	raw := now.Add(-2 * time.Hour).Format("2006-01-02 15:04:05") + ".123456789"
	r := NewResultsTable()
	r.SetResult(
		[]string{"created_at"},
		[][]string{{raw}},
		"1 row",
	)
	w := r.ColWidth(0)
	dispW := runeLen("2h ago")
	if w < dispW {
		t.Fatalf("width %d < display %d", w, dispW)
	}
	if w >= runeLen(raw) {
		t.Errorf("width %d still sized to raw fractional timestamp", w)
	}
}

func TestIsCellTruncatedUsesDatetimeDisplay(t *testing.T) {
	now := time.Date(2026, 8, 30, 18, 0, 0, 0, time.Local)
	prev := datetimeNow
	datetimeNow = func() time.Time { return now }
	t.Cleanup(func() { datetimeNow = prev })

	r := NewResultsTable()
	r.SetResult(
		[]string{"created_at"},
		[][]string{{now.Add(-2 * time.Hour).Format("2006-01-02 15:04:05") + ".123456789"}},
		"1 row",
	)
	if r.IsCellTruncated(0, 0) {
		t.Error("relative form fits column; should not report truncated")
	}
}
