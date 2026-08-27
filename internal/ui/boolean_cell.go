package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/db"
)

// Boolean-column glyphs (display only). Copy / edit / export keep the raw
// cell value. Uses ●/○ — the same checked/unchecked pair as the export picker.
// Glyphs use colorBool (default fg softened toward bg).
const (
	boolGlyphTrue  = "●"
	boolGlyphFalse = "○"
)

// booleanValueFg reports the soft default-fg tint for a parseable boolean cell.
func booleanValueFg(val string) (lipgloss.Color, bool) {
	if _, ok := parseBoolCell(val); !ok {
		return "", false
	}
	return colorBool, true
}

// isBooleanColumnName reports whether a column name typically holds a boolean
// even when the driver type is INTEGER/TINYINT (common for MySQL BOOLEAN and
// SQLite). Kept narrow on purpose: is_/has_/can_ prefixes and a few exact
// names / suffixes.
func isBooleanColumnName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	switch n {
	case "enabled", "disabled", "active", "locked", "verified",
		"published", "archived", "visible", "flagged", "featured",
		"subscribed", "deleted", "sticky":
		return true
	}
	if strings.HasPrefix(n, "is_") || strings.HasPrefix(n, "has_") ||
		strings.HasPrefix(n, "can_") || strings.HasPrefix(n, "should_") ||
		strings.HasPrefix(n, "was_") {
		return true
	}
	return strings.HasSuffix(n, "_enabled") ||
		strings.HasSuffix(n, "_active") ||
		strings.HasSuffix(n, "_flag") ||
		strings.HasSuffix(n, "_locked") ||
		strings.HasSuffix(n, "_verified") ||
		strings.HasSuffix(n, "_published") ||
		strings.HasSuffix(n, "_visible") ||
		strings.HasSuffix(n, "_archived") ||
		strings.HasSuffix(n, "_preference")
}

// isBooleanDisplayCol reports whether column col should use boolean glyph
// rendering when its value parses as true/false.
func (r ResultsTable) isBooleanDisplayCol(col int) bool {
	if db.IsBooleanType(r.columnType(col)) {
		return true
	}
	// Query ColumnTypes() often returns bare "TINYINT" on MySQL; schema
	// TableColumnInfo keeps "tinyint(1)" from information_schema.
	if info, ok := r.columnInfo(col); ok && db.IsBooleanType(info.Type) {
		return true
	}
	return isBooleanColumnName(r.ColumnName(col))
}

// parseBoolCell maps a raw cell string to a boolean. Only exact, common
// driver spellings match — unknown values return false so the caller keeps
// the raw string (avoids turning a tinyint count of 2 into a glyph).
func parseBoolCell(val string) (bool, bool) {
	v := strings.ToLower(strings.TrimSpace(val))
	if v == "" || v == "null" {
		return false, false
	}
	switch v {
	case "1", "t", "true", "yes", "y", "on":
		return true, true
	case "0", "f", "false", "no", "n", "off":
		return false, true
	}
	return false, false
}

// booleanCellDisplay returns the ●/○ glyph when val parses as a boolean;
// otherwise false so the caller keeps the raw cell.
func booleanCellDisplay(val string, width int) (string, bool) {
	b, ok := parseBoolCell(val)
	if !ok {
		return "", false
	}
	glyph := boolGlyphFalse
	if b {
		glyph = boolGlyphTrue
	}
	return truncateCell(glyph, width), true
}

// booleanDisplayWidth is the rune width used when sizing columns for a
// boolean cell (glyph when parseable).
func booleanDisplayWidth(val string) int {
	if _, ok := parseBoolCell(val); ok {
		return runeLen(boolGlyphTrue)
	}
	return runeLen(val)
}
