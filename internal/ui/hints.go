package ui

import "strings"

// hintList returns the compact hint strings for the current panel or active
// modal overlay. Each element is a single hint group (e.g. "h/j/k/l",
// "enter", "ctrl+s"). It is a thin wrapper over resolveHints, which also
// exposes the registry section so a pressed key's description can be looked
// up (see hintDescription).
func (m Model) hintList() []string {
	_, hints := m.resolveHints()
	return hints
}

// hintSection returns the registry section title backing the current panel or
// modal's hints, or "" for inline-only states (the hint still shows, but the
// pressed key has no description to display).
func (m Model) hintSection() string {
	section, _ := m.resolveHints()
	return section
}

// hintDescription returns the registry description for a matched hint key in
// the current context, or "" if the active panel/modal has no registry section
// or the key isn't documented there. matchedKey is the individual key string
// returned by matchHint (e.g. "j", "ctrl+s"). It is matched against each
// binding's Tokens rather than its Hint string: the Hint is a display
// abbreviation whose "/" split is ambiguous (e.g. "/" splits to ["",""]),
// whereas Tokens are the actual dispatch keys.
func (m Model) hintDescription(matchedKey string) string {
	section := m.hintSection()
	if section == "" {
		return ""
	}
	for _, sec := range registry() {
		if sec.Title != section {
			continue
		}
		for _, b := range sec.Items {
			for _, t := range b.Tokens {
				if t == matchedKey {
					return b.Desc
				}
			}
		}
	}
	return ""
}

// resolveHints returns the registry section title and the hint list for the
// current panel or active modal overlay. section is "" for inline-only states.
// Each hint element is a single hint group (e.g. "h/j/k/l", "enter", "ctrl+s");
// hints for section-backed states come from the registry (single source of
// truth).
//
// The order matters: most specific/modal states are checked first, since they
// stack on top of other overlays (e.g. createDBActive on top of dbPicker).
func (m Model) resolveHints() (section string, hints []string) {
	switch {
	// Help overlay is the most modal surface — check first.
	case m.help.IsVisible():
		return "Help", hintsForSection("Help")

	// Dialogs stacked on top of pickers — check next.
	case m.createDBActive:
		return "", []string{"enter", "esc"}
	case m.dropDBConfirm != "" || m.dropTableConfirm != "":
		return "", []string{"enter", "esc"}
	case m.deleteConnConfirm != "" || m.deleteProviderConfirm != "":
		return "", []string{"y", "n", "esc"}

	// Full-screen overlays.
	case m.importPrompt.IsVisible():
		return "Import Prompt", hintsForSection("Import Prompt")
	case m.exportPicker.IsVisible():
		return "Export Picker", hintsForSection("Export Picker")
	case m.exportOverlay.IsVisible():
		return "Export Dialog", hintsForSection("Export Dialog")
	case m.columnPicker.IsVisible():
		return "Column Picker", hintsForSection("Column Picker")
	case m.filterPicker.IsVisible():
		return "Filter Picker", hintsForSection("Filter Picker")
	case m.dbPicker.IsVisible():
		if m.dbPicker.Filtering() {
			return "", []string{"j/k", "enter", "esc"}
		}
		return "Database Picker", hintsForSection("Database Picker")
	case m.history.IsVisible():
		return "History Panel", hintsForSection("History Panel")
	case m.bookmarks.IsVisible():
		return "Bookmarks Panel", hintsForSection("Bookmarks Panel")
	case m.crossSearch.IsVisible():
		return "Cross-Table Search", hintsForSection("Cross-Table Search")
	case m.explainPanel.IsVisible():
		return "Explain Panel", hintsForSection("Explain Panel")
	case m.lookupPanel.IsVisible():
		return "Lookup Panel", hintsForSection("Lookup Panel")
	case m.erdPanel.searching:
		return "", []string{"tab", "enter", "esc"}
	case m.erdPanel.IsVisible():
		// Graph view shows the spatial-nav keys (j/k/h/l, space, enter) plus the
		// "/" jump-to-table search; the Mermaid source view only scrolls with
		// j/k, so its hint set is smaller.
		if m.erdPanel.merm {
			return "ERD", []string{"j/k", "enter", "m", "g/G", "ctrl+d/ctrl+u", "y", "esc"}
		}
		return "ERD", []string{"j/k/h/l", "space", "enter", "zz", "zc/zo/za", "zM/zR", "/", "p", "m", "g/G", "ctrl+d/ctrl+u", "y", "esc"}
	case m.explorer.IsVisible():
		return "Relationship Explorer", hintsForSection("Relationship Explorer")
	case m.providerPicker.IsVisible():
		return "AI Provider", hintsForSection("AI Provider")
	case m.providerForm.IsVisible():
		if m.providerForm.IsEditing() {
			return "", []string{"enter", "esc"}
		}
		if m.providerForm.ActiveIsChoice() {
			return "", []string{"j/k", "h/l", "enter", "ctrl+t", "esc"}
		}
		return "", []string{"j/k", "e", "enter", "ctrl+t", "esc"}
	case m.modelBrowser.IsVisible():
		return "Model Browser", hintsForSection("Model Browser")
	case m.tableDesigner.IsVisible():
		return "Table Designer", hintsForSection("Table Designer")
	case m.schemaEditor.IsVisible():
		return "Schema Editor", hintsForSection("Schema Editor")
	case m.cellEdit.IsVisible():
		return "Cell Editor", hintsForSection("Cell Editor")
	case m.addColumnForm.IsVisible() || m.tableRenameForm.IsVisible():
		return "Add Column / Rename Table", hintsForSection("Add Column / Rename Table")
	case m.searching:
		return "", []string{"enter", "esc"}

	// Inspector sub-states.
	case m.focus == FocusInspector && (m.inspector.IsEditing() || m.inspector.IsInserting()):
		return "", []string{"enter", "esc"}

	default:
		switch {
		case m.state == stateAddConnection:
			if m.connForm.editing {
				return "", []string{"enter", "esc"}
			}
			if m.connForm.ActiveIsChoice() {
				return "", []string{"j/k", "h/l", "enter", "ctrl+t", "esc"}
			}
			return "", []string{"j/k", "e", "enter", "ctrl+t", "esc"}
		case m.state == stateConnections:
			if m.connList.IsFiltering() {
				return "", []string{"j/k", "enter", "esc"}
			}
			return "Connections", hintsForSection("Connections")
		case m.focus == FocusEditor:
			return "Editor (Vim)", hintsForSection("Editor (Vim)")
		case m.focus == FocusResults:
			return "Results", hintsForSection("Results")
		case m.focus == FocusConnections:
			return "Sidebar (Tables)", hintsForSection("Sidebar (Tables)")
		case m.focus == FocusInspector:
			return "Inspector", hintsForSection("Inspector")
		case m.focus == FocusAssistant:
			// Compose (insert) mode has a small, distinct key set; browse mode
			// uses the full assistant hint set (i/a/o, M, c, j/k, esc…).
			if m.assistant.IsComposing() {
				return "", []string{"enter", "esc"}
			}
			return "Assistant", hintsForSection("Assistant")
		case m.focus == FocusTabBar:
			return "Tab Bar", hintsForSection("Tab Bar")
		}
	}
	return "", nil
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
