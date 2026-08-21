package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestIsStatusColumnName(t *testing.T) {
	yes := []string{"status", "Status", "STATE", "payment_status", "order_state", "fulfillment_status"}
	for _, n := range yes {
		if !isStatusColumnName(n) {
			t.Errorf("%q should be a status column", n)
		}
	}
	no := []string{"id", "user_id", "total", "name", "status_code", "statement"}
	for _, n := range no {
		if isStatusColumnName(n) {
			t.Errorf("%q should not be a status column", n)
		}
	}
}

func TestStatusValueFg(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	cases := []struct {
		val  string
		want lipgloss.Color
		ok   bool
	}{
		{"delivered", colorStatusOK, true},
		{"DELIVERED", colorStatusOK, true},
		{"shipped", colorStatusInfo, true},
		{"pending", colorStatusWarn, true},
		{"cancelled", colorStatusBad, true},
		{"captured", colorStatusOK, true},
		{"authorized", colorStatusWarn, true},
		{"in-progress", colorStatusInfo, true},
		{"weird", "", false},
		{"NULL", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := statusValueFg(tc.val)
		if ok != tc.ok || (tc.ok && got != tc.want) {
			t.Errorf("statusValueFg(%q) = (%q, %v), want (%q, %v)", tc.val, got, ok, tc.want, tc.ok)
		}
	}
}

func TestStatusCellDisplayMark(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	disp, fg, ok := statusCellDisplay("pending", 20)
	if !ok || fg != colorStatusWarn {
		t.Fatalf("ok=%v fg=%q", ok, fg)
	}
	if !strings.HasPrefix(strings.TrimRight(disp, " "), "● pending") {
		t.Errorf("display = %q, want ● pending…", disp)
	}
	// Narrow column: color only, no mark.
	disp, _, ok = statusCellDisplay("pending", 5)
	if !ok {
		t.Fatal("expected color")
	}
	if strings.Contains(disp, "●") {
		t.Errorf("narrow cell should omit mark, got %q", disp)
	}
}

func TestResultsTableStatusCellColor(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	r := NewResultsTable()
	r.SetSize(80, 12)
	r.SetResult(
		[]string{"id", "status", "total"},
		[][]string{{"1", "delivered", "10"}, {"2", "pending", "5"}, {"3", "cancelled", "0"}},
		"3 rows",
	)
	r.SetCursor(0, 0) // leave status cells non-cursor so semantic fg shows

	view := r.View()
	successPrefix := sgrPrefix(lipgloss.NewStyle().Foreground(colorStatusOK).Background(colorCursorRow))
	// Row 0 is cursor row — status uses soft success on cursor-row bg.
	if !strings.Contains(view, successPrefix) {
		t.Errorf("expected delivered status soft tint on cursor row; view snippet:\n%s", view[:min(400, len(view))])
	}
	warnAlone := sgrPrefix(lipgloss.NewStyle().Foreground(colorStatusWarn))
	errAlone := sgrPrefix(lipgloss.NewStyle().Foreground(colorStatusBad))
	if !strings.Contains(view, warnAlone) && !strings.Contains(view, sgrPrefix(lipgloss.NewStyle().Foreground(colorStatusWarn).Background(colorStripe))) {
		t.Error("expected pending warn tint")
	}
	if !strings.Contains(view, errAlone) && !strings.Contains(view, sgrPrefix(lipgloss.NewStyle().Foreground(colorStatusBad).Background(colorStripe))) {
		t.Error("expected cancelled error tint")
	}
	if !strings.Contains(view, "●") {
		t.Error("expected status mark glyph in view")
	}
	// Non-status column name must not tint "delivered" as status if renamed.
	r2 := NewResultsTable()
	r2.SetSize(80, 12)
	r2.SetResult([]string{"id", "note"}, [][]string{{"1", "delivered"}}, "1 row")
	r2.SetCursor(0, 0)
	v2 := r2.View()
	if strings.Contains(v2, "●") {
		t.Error("non-status column must not get status mark")
	}
}
