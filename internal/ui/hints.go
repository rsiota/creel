package ui

import "strings"

// contextHints returns a compact, slash-separated string of the most useful
// keybindings for the current panel or active modal overlay. The hints are
// derived from the registry (single source of truth) for sections that have
// one; pickers without a registry section fall back to inline hint lists.
func (m Model) contextHints() string {
	var hints []string

	switch {
	case m.exportPicker.IsVisible():
		hints = hintsForSection("Export Picker")
	case m.columnPicker.IsVisible():
		hints = []string{"j/k", "space", "a", "n", "enter", "esc"}
	case m.filterPicker.IsVisible():
		hints = []string{"j/k", "space", "enter", "esc"}
	case m.dbPicker.IsVisible():
		hints = []string{"j/k", "enter", "esc"}
	case m.history.IsVisible():
		hints = hintsForSection("History Panel")
	case m.bookmarks.IsVisible():
		hints = hintsForSection("Bookmarks Panel")
	case m.searching:
		hints = []string{"enter", "esc"}
	default:
		switch m.focus {
		case FocusEditor:
			hints = hintsForSection("Editor (Vim)")
		case FocusResults:
			hints = hintsForSection("Results")
		case FocusConnections:
			hints = hintsForSection("Sidebar (Tables)")
		case FocusInspector:
			hints = hintsForSection("Inspector")
		}
	}

	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, "/")
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
