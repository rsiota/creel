package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Status-column tint (prototype): columns whose names look like a status/state
// enum get a semantic foreground color (and a leading ● when there is room).
// Raw cell values are unchanged for copy/edit/export — this is display only.

// isStatusColumnName reports whether a result column should get status styling.
// Kept narrow on purpose: status / state / *_status / *_state.
func isStatusColumnName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	switch n {
	case "status", "state":
		return true
	}
	return strings.HasSuffix(n, "_status") || strings.HasSuffix(n, "_state")
}

// statusValueFg maps a status/state cell value to a theme color. Unknown
// values return false so the cell stays the default foreground.
func statusValueFg(val string) (lipgloss.Color, bool) {
	v := strings.ToLower(strings.TrimSpace(val))
	if v == "" || v == "null" {
		return "", false
	}
	// Normalize separators so "in_progress" / "in-progress" / "in progress" match.
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")

	switch v {
	// Success / done
	case "delivered", "completed", "complete", "paid", "captured", "success",
		"successful", "active", "done", "ok", "approved", "fulfilled":
		return colorStatusOK, true

	// In transit / underway (not terminal)
	case "shipped", "processing", "process", "in_progress", "inprogress",
		"queued", "submitted":
		return colorStatusInfo, true

	// Attention / waiting
	case "pending", "unpaid", "authorized", "open", "new":
		return colorStatusWarn, true

	// Failure / stopped
	case "cancelled", "canceled", "failed", "failure", "refunded", "rejected",
		"inactive", "error", "declined", "void", "expired":
		return colorStatusBad, true

	// Quiet / draft
	case "draft", "unknown", "n_a", "na", "none":
		return colorStatusQuiet, true
	}
	return "", false
}

// statusCellDisplay returns the display string for a status cell (optional ●
// prefix) and whether a semantic color applies. width is the column content
// width used by truncateCell.
func statusCellDisplay(val string, width int) (display string, fg lipgloss.Color, ok bool) {
	fg, ok = statusValueFg(val)
	if !ok {
		return truncateCell(val, width), "", false
	}
	const mark = "● "
	// Prefer the mark when it still fits with the full value; otherwise color only.
	if width >= runeLen(mark)+runeLen(val) {
		return truncateCell(mark+val, width), fg, true
	}
	return truncateCell(val, width), fg, true
}
