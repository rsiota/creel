package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/rsiota/creel/internal/db"
)

// Relative / compact timestamps in the results grid (display only).
// Copy / edit / export keep the raw cell value.
//
// Recent values show as relative ("2h ago", "yesterday"); older ones fall
// back to the compact absolute form. Time-only values stay "15:04".

// datetimeNow is the clock for relative display. Tests override it.
var datetimeNow = time.Now

// relativeHorizon is how far back/forward a value may be and still render
// relatively before falling back to the compact absolute form.
const relativeHorizon = 7 * 24 * time.Hour

// isDatetimeColumnName reports whether a column name typically holds a
// date/time even when the driver type is TEXT (common for SQLite).
func isDatetimeColumnName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	switch n {
	case "created", "updated", "deleted", "timestamp", "datetime", "date", "time",
		"createdat", "updatedat", "deletedat", "ts":
		return true
	}
	return strings.HasSuffix(n, "_at") ||
		strings.HasSuffix(n, "_on") ||
		strings.HasSuffix(n, "_date") ||
		strings.HasSuffix(n, "_time") ||
		strings.HasSuffix(n, "_timestamp") ||
		strings.HasSuffix(n, "_datetime")
}

// isDatetimeDisplayCol reports whether column col should use compact/relative
// timestamp rendering when its values parse as dates/times.
func (r ResultsTable) isDatetimeDisplayCol(col int) bool {
	if db.IsDateTimeType(r.columnType(col)) {
		return true
	}
	return isDatetimeColumnName(r.ColumnName(col))
}

// parseDatetimeCell parses a cell for display. Zone-less values are treated as
// local wall time so "2h ago" matches what the user typed into the DB.
func parseDatetimeCell(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range chartTimeLayouts {
		if datetimeLayoutHasZone(layout) {
			if t, err := time.Parse(layout, s); err == nil {
				return t, true
			}
			continue
		}
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func datetimeLayoutHasZone(layout string) bool {
	return strings.Contains(layout, "Z07") || layout == time.RFC3339 || layout == time.RFC3339Nano
}

// formatDatetimeDisplay returns the grid display form for a parseable
// timestamp (relative when recent, compact absolute otherwise).
func formatDatetimeDisplay(val string, now time.Time) (string, bool) {
	s := strings.TrimSpace(val)
	if s == "" || strings.EqualFold(s, "null") {
		return "", false
	}
	t, ok := parseDatetimeCell(s)
	if !ok {
		return "", false
	}
	switch {
	case looksLikeDateOnly(s):
		return formatRelativeDate(t, now), true
	case looksLikeTimeOnly(s):
		return t.Format("15:04"), true
	default:
		return formatRelativeDateTime(t, now), true
	}
}

// compactTimestamp is the absolute fallback (and kept for tests / callers that
// want the non-relative form).
func compactTimestamp(val string) (string, bool) {
	s := strings.TrimSpace(val)
	if s == "" || strings.EqualFold(s, "null") {
		return "", false
	}
	t, ok := parseDatetimeCell(s)
	if !ok {
		return "", false
	}
	switch {
	case looksLikeDateOnly(s):
		return t.Format("2006-01-02"), true
	case looksLikeTimeOnly(s):
		return t.Format("15:04"), true
	default:
		return t.Format("2006-01-02 15:04"), true
	}
}

func formatRelativeDateTime(t, now time.Time) string {
	d := now.Sub(t)
	abs := d
	if abs < 0 {
		abs = -abs
	}
	if abs >= relativeHorizon {
		return t.Format("2006-01-02 15:04")
	}
	if abs < time.Minute {
		return "just now"
	}
	label := relativeUnit(abs)
	if d < 0 {
		return "in " + label
	}
	return label + " ago"
}

func formatRelativeDate(t, now time.Time) string {
	tDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	n := now.In(t.Location())
	nDay := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, t.Location())
	days := int(nDay.Sub(tDay).Hours() / 24)
	switch days {
	case 0:
		return "today"
	case 1:
		return "yesterday"
	case -1:
		return "tomorrow"
	}
	if days > 1 && days < 7 {
		return fmt.Sprintf("%dd ago", days)
	}
	if days < -1 && days > -7 {
		return fmt.Sprintf("in %dd", -days)
	}
	return t.Format("2006-01-02")
}

func relativeUnit(d time.Duration) string {
	if d < time.Hour {
		m := int((d + time.Minute/2) / time.Minute)
		if m < 1 {
			m = 1
		}
		if m >= 60 {
			return "1h"
		}
		return fmt.Sprintf("%dm", m)
	}
	if d < 24*time.Hour {
		h := int((d + time.Hour/2) / time.Hour)
		if h < 1 {
			h = 1
		}
		if h >= 24 {
			return "1d"
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int((d + 12*time.Hour) / (24 * time.Hour))
	if days < 1 {
		days = 1
	}
	return fmt.Sprintf("%dd", days)
}

func looksLikeDateOnly(s string) bool {
	// YYYY-MM-DD with nothing else (allow optional trailing Z already stripped by trim).
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func looksLikeTimeOnly(s string) bool {
	if strings.ContainsAny(s, "-T ") {
		return false
	}
	return strings.Contains(s, ":")
}

// datetimeCellDisplay returns the relative/compact form when val parses as a
// timestamp; otherwise false and the caller keeps the raw cell.
func datetimeCellDisplay(val string, width int) (string, bool) {
	c, ok := formatDatetimeDisplay(val, datetimeNow())
	if !ok {
		return "", false
	}
	return truncateCell(c, width), true
}

// datetimeDisplayWidth is the rune width used when sizing columns for a
// datetime cell (display form when parseable).
func datetimeDisplayWidth(val string) int {
	if c, ok := formatDatetimeDisplay(val, datetimeNow()); ok {
		return runeLen(c)
	}
	return runeLen(val)
}
