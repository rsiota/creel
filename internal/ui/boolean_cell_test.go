package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/rsiota/creel/internal/db"
)

func TestIsBooleanColumnName(t *testing.T) {
	yes := []string{"is_active", "has_access", "can_edit", "should_notify", "was_sent",
		"enabled", "active", "locked", "verified", "published", "user_enabled", "user_flag",
		"email_preference", "sms_preference"}
	for _, n := range yes {
		if !isBooleanColumnName(n) {
			t.Errorf("%q should be a boolean column name", n)
		}
	}
	no := []string{"id", "status", "email", "total", "flag", "flagged_by", "active_users", "deleted_at", "preference"}
	for _, n := range no {
		if isBooleanColumnName(n) {
			t.Errorf("%q should not be a boolean column name", n)
		}
	}
}

func TestParseBoolCell(t *testing.T) {
	cases := []struct {
		in   string
		want bool
		ok   bool
	}{
		{"1", true, true},
		{"0", false, true},
		{"true", true, true},
		{"FALSE", false, true},
		{"t", true, true},
		{"f", false, true},
		{"yes", true, true},
		{"no", false, true},
		{"on", true, true},
		{"off", false, true},
		{"NULL", false, false},
		{"", false, false},
		{"2", false, false},
		{"maybe", false, false},
	}
	for _, tc := range cases {
		got, ok := parseBoolCell(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseBoolCell(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestBooleanCellDisplay(t *testing.T) {
	disp, ok := booleanCellDisplay("1", 10)
	if !ok || !strings.HasPrefix(strings.TrimRight(disp, " "), boolGlyphTrue) {
		t.Errorf("true display = %q ok=%v", disp, ok)
	}
	disp, ok = booleanCellDisplay("0", 10)
	if !ok || !strings.HasPrefix(strings.TrimRight(disp, " "), boolGlyphFalse) {
		t.Errorf("false display = %q ok=%v", disp, ok)
	}
	if _, ok := booleanCellDisplay("2", 10); ok {
		t.Error("non-boolean value should not display as glyph")
	}
}

func TestResultsTableBooleanGlyphs(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	r := NewResultsTable()
	r.SetSize(80, 12)
	r.SetResult(
		[]string{"id", "is_active", "enabled", "email_preference"},
		[][]string{{"1", "1", "true", "1"}, {"2", "0", "false", "0"}},
		"2 rows",
	)
	r.SetColumnTypes(map[string]string{
		"id": "INTEGER", "is_active": "TINYINT", "enabled": "BOOLEAN", "email_preference": "TINYINT",
	})
	// Schema keeps tinyint(1); query ColumnTypes often lose the (1).
	r.SetTableColumns([]db.TableColumnInfo{
		{Name: "id", Type: "int"},
		{Name: "is_active", Type: "tinyint(1)"},
		{Name: "enabled", Type: "tinyint(1)"},
		{Name: "email_preference", Type: "tinyint(1)"},
	})
	r.SetCursor(0, 0)

	view := ansi.Strip(r.View())
	if !strings.Contains(view, boolGlyphTrue) {
		t.Errorf("expected true glyph in view:\n%s", view)
	}
	if !strings.Contains(view, boolGlyphFalse) {
		t.Errorf("expected false glyph in view:\n%s", view)
	}
	// Raw values preserved for yank / edit.
	if got := r.RowValue(0, 1); got != "1" {
		t.Errorf("raw is_active = %q, want 1", got)
	}
	if got := r.RowValue(1, 2); got != "false" {
		t.Errorf("raw enabled = %q, want false", got)
	}
	if got := r.RowValue(0, 3); got != "1" {
		t.Errorf("raw email_preference = %q, want 1", got)
	}

	// Non-boolean column must keep 1/0 as numbers.
	r2 := NewResultsTable()
	r2.SetSize(80, 12)
	r2.SetResult([]string{"id", "count"}, [][]string{{"1", "1"}}, "1 row")
	r2.SetColumnTypes(map[string]string{"id": "INTEGER", "count": "INTEGER"})
	r2.SetCursor(0, 0)
	v2 := ansi.Strip(r2.View())
	if strings.Contains(v2, boolGlyphTrue) || strings.Contains(v2, boolGlyphFalse) {
		t.Errorf("numeric column must not get boolean glyphs:\n%s", v2)
	}
}

func TestResultsTableBooleanFromTinyInt1Schema(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(100, 12)
	r.SetResult(
		[]string{"id", "sms_preference"},
		[][]string{{"1", "1"}},
		"1 row",
	)
	// Bare TINYINT from the driver — only schema tinyint(1) marks it boolean.
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "sms_preference": "TINYINT"})
	r.SetTableColumns([]db.TableColumnInfo{
		{Name: "id", Type: "int"},
		{Name: "sms_preference", Type: "tinyint(1)"},
	})
	r.SetCursor(0, 0)

	view := ansi.Strip(r.View())
	if !strings.Contains(view, boolGlyphTrue) {
		t.Errorf("tinyint(1) schema column should show glyph:\n%s", view)
	}
}

func TestResultsTableBooleanSoftColor(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	r := NewResultsTable()
	r.SetSize(80, 12)
	r.SetResult(
		[]string{"id", "is_active"},
		[][]string{{"1", "1"}, {"2", "0"}},
		"2 rows",
	)
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "is_active": "BOOLEAN"})
	r.SetCursor(0, 0)

	view := r.View()
	softOnCursor := sgrPrefix(lipgloss.NewStyle().Foreground(colorBool).Background(colorCursorRow))
	if !strings.Contains(view, softOnCursor) {
		t.Errorf("expected soft bool tint on cursor row; snippet:\n%s", view[:min(400, len(view))])
	}
	softAlone := sgrPrefix(lipgloss.NewStyle().Foreground(colorBool))
	softStripe := sgrPrefix(lipgloss.NewStyle().Foreground(colorBool).Background(colorStripe))
	if !strings.Contains(view, softAlone) && !strings.Contains(view, softStripe) {
		t.Error("expected soft bool tint on non-cursor boolean cell")
	}
	okPrefix := sgrPrefix(lipgloss.NewStyle().Foreground(colorStatusOK).Background(colorCursorRow))
	if strings.Contains(view, okPrefix) {
		t.Error("boolean must not use status-ok/green tint")
	}
}
