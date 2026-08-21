package ui

import (
	"strings"
	"time"

	"github.com/rsiota/creel/internal/db"
)

// Compact absolute timestamps in the results grid (display only).
// Copy / edit / export keep the raw cell value.

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

// isDatetimeDisplayCol reports whether column col should use compact timestamp
// rendering when its values parse as dates/times.
func (r ResultsTable) isDatetimeDisplayCol(col int) bool {
	if db.IsDateTimeType(r.columnType(col)) {
		return true
	}
	return isDatetimeColumnName(r.ColumnName(col))
}

// compactTimestamp shortens a parseable timestamp for the grid:
//   datetime → "2006-01-02 15:04" (drop seconds / fraction / zone)
//   date     → "2006-01-02"
//   time     → "15:04"
// Unparseable / NULL values return false so the raw string is shown.
func compactTimestamp(val string) (string, bool) {
	s := strings.TrimSpace(val)
	if s == "" || strings.EqualFold(s, "null") {
		return "", false
	}
	t, ok := parseChartTime(s)
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

// datetimeCellDisplay returns the truncated compact form when val parses as a
// timestamp; otherwise false and the caller keeps the raw cell.
func datetimeCellDisplay(val string, width int) (string, bool) {
	c, ok := compactTimestamp(val)
	if !ok {
		return "", false
	}
	return truncateCell(c, width), true
}

// datetimeDisplayWidth is the rune width used when sizing columns for a
// datetime cell (compact form when parseable).
func datetimeDisplayWidth(val string) int {
	if c, ok := compactTimestamp(val); ok {
		return runeLen(c)
	}
	return runeLen(val)
}
