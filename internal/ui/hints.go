package ui

import "strings"

// hintList returns the list of compact hint strings for the current panel or
// active modal overlay. Each element is a single hint group (e.g. "h/j/k/l",
// "enter", "ctrl+s"). The hints are derived from the registry (single source
// of truth) for sections that have one; pickers without a registry section
// fall back to inline hint lists.
func (m Model) hintList() []string {
	switch {
	case m.exportPicker.IsVisible():
		return hintsForSection("Export Picker")
	case m.columnPicker.IsVisible():
		return []string{"j/k", "space", "a", "n", "enter", "esc"}
	case m.filterPicker.IsVisible():
		return []string{"j/k", "space", "enter", "esc"}
	case m.dbPicker.IsVisible():
		return []string{"j/k", "enter", "esc"}
	case m.history.IsVisible():
		return hintsForSection("History Panel")
	case m.bookmarks.IsVisible():
		return hintsForSection("Bookmarks Panel")
	case m.searching:
		return []string{"enter", "esc"}
	default:
		switch m.focus {
		case FocusEditor:
			return hintsForSection("Editor (Vim)")
		case FocusResults:
			return hintsForSection("Results")
		case FocusConnections:
			return hintsForSection("Sidebar (Tables)")
		case FocusInspector:
			return hintsForSection("Inspector")
		}
	}
	return nil
}

// contextHints returns the hint list joined with "/" for display.
func (m Model) contextHints() string {
	hints := m.hintList()
	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, "/")
}

// matchHint returns the individual key within a hint group that matches the
// pressed key, or "" if none. Each hint group is split on "/" to extract
// individual keys (e.g. "h/j/k/l" yields h, j, k, l). Multi-word keys like
// "ctrl+s" and "enter" are matched as-is. A literal space key is normalized
// to "space" to match the display hint.
func matchHint(hints []string, key string) string {
	if key == " " {
		key = "space"
	}
	for _, h := range hints {
		for _, k := range strings.Split(h, "/") {
			if k == key {
				return k
			}
		}
	}
	return ""
}

// hintsForSection collects all non-empty Hint strings from the registry
// section with the given title.
func hintsForSection(title string) []string {
	for _, sec := range registry() {
		if sec.Title == title {
			var hints []string
			for _, b := range sec.Items {
				if b.Hint != "" {
					hints = append(hints, b.Hint)
				}
			}
			return hints
		}
	}
	return nil
}
