package ui

import "strings"

// hintList returns the list of compact hint strings for the current panel or
// active modal overlay. Each element is a single hint group (e.g. "h/j/k/l",
// "enter", "ctrl+s"). The hints are derived from the registry (single source
// of truth) for sections that have one; pickers without a registry section
// fall back to inline hint lists.
//
// The order matters: most specific/modal states are checked first, since they
// stack on top of other overlays (e.g. createDBActive on top of dbPicker).
func (m Model) hintList() []string {
	switch {
	// Dialogs stacked on top of pickers — check first.
	case m.createDBActive:
		return []string{"enter", "esc"}
	case m.dropDBConfirm != "" || m.dropTableConfirm != "":
		return []string{"enter", "esc"}

	// Full-screen overlays.
	case m.importPrompt.IsVisible():
		return hintsForSection("Import Prompt")
	case m.exportPicker.IsVisible():
		return hintsForSection("Export Picker")
	case m.columnPicker.IsVisible():
		return hintsForSection("Column Picker")
	case m.filterPicker.IsVisible():
		return hintsForSection("Filter Picker")
	case m.dbPicker.IsVisible():
		if m.dbPicker.Filtering() {
			return []string{"j/k", "enter", "esc"}
		}
		return hintsForSection("Database Picker")
	case m.history.IsVisible():
		return hintsForSection("History Panel")
	case m.bookmarks.IsVisible():
		return hintsForSection("Bookmarks Panel")
	case m.crossSearch.IsVisible():
		return []string{"enter", "j/k", "esc"}
	case m.explainPanel.IsVisible():
		return []string{"j/k", "esc"}
	case m.tableDesigner.IsVisible():
		return hintsForSection("Table Designer")
	case m.schemaEditor.IsVisible():
		return hintsForSection("Schema Editor")
	case m.cellEdit.IsVisible():
		return hintsForSection("Cell Editor")
	case m.addColumnForm.IsVisible() || m.tableRenameForm.IsVisible():
		return hintsForSection("Add Column / Rename Table")
	case m.searching:
		return []string{"enter", "esc"}

	// Inspector sub-states.
	case m.focus == FocusInspector && (m.inspector.IsEditing() || m.inspector.IsInserting()):
		return []string{"enter", "esc"}

	default:
		switch {
		case m.state == stateAddConnection:
			return []string{"tab", "enter", "ctrl+t", "esc"}
		case m.state == stateConnections:
			if m.connList.IsFiltering() {
				return []string{"j/k", "enter", "esc"}
			}
			return []string{"j/k", "enter", "n", "e", "d", "/", "esc"}
		case m.focus == FocusEditor:
			return hintsForSection("Editor (Vim)")
		case m.focus == FocusResults:
			return hintsForSection("Results")
		case m.focus == FocusConnections:
			return hintsForSection("Sidebar (Tables)")
		case m.focus == FocusInspector:
			return hintsForSection("Inspector")
		case m.focus == FocusTabBar:
			return []string{"h/l", "t", "enter"}
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
		if h == key {
			return h
		}
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
