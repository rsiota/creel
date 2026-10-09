package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateFocusedPanel(msg tea.KeyMsg, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	// Dispatch to focused panel
	switch m.focus {
	case FocusTabBar:
		switch msg.String() {
		case "h", "left":
			if prevID := m.tabBar.PrevTab(); prevID >= 0 {
				m.setActiveTab(prevID)
			}
			return m, nil
		case "l", "right":
			if nextID := m.tabBar.NextTab(); nextID >= 0 {
				m.setActiveTab(nextID)
			}
			return m, nil
		case "j", "down", "enter":
			m.focus = FocusEditor
			m.applyFocus()
			return m, nil
		}
		return m, nil
	case FocusEditor:
		// Handle ctrl+arrow for result scrolling while in editor
		switch msg.String() {
		case "ctrl+up":
			m.results.ScrollUp()
			return m, nil
		case "ctrl+down":
			m.results.ScrollDown()
			return m, nil
		case "ctrl+left":
			m.results.ScrollLeft()
			return m, nil
		case "ctrl+right":
			m.results.ScrollRight()
			return m, nil
		case "ctrl+n":
			if m.editor.VimMode() == VimInsert && !m.editor.CompletionVisible() {
				m.editor.StartCompletion()
				return m, nil
			}
		}

		// Command history navigation: up/down arrow in vim normal mode.
		if m.editor.VimMode() == VimNormal && !m.editor.CompletionVisible() {
			switch msg.String() {
			case "up":
				if m.connection != nil && m.historyStore != nil {
					if m.historyNavIdx == -1 {
						entries, err := m.historyStore.Get(m.connection.Config().Name)
						if err != nil || len(entries) == 0 {
							return m, nil
						}
						m.historyNavEntries = make([]string, len(entries))
						for i, e := range entries {
							m.historyNavEntries[i] = e.Query
						}
						m.historyNavSaved = m.editor.Value()
						m.historyNavIdx = len(m.historyNavEntries) - 1
					} else if m.historyNavIdx > 0 {
						m.historyNavIdx--
					} else {
						return m, nil // already at oldest
					}
					m.editor.SetValue(m.historyNavEntries[m.historyNavIdx])
				}
				return m, nil
			case "down":
				if m.historyNavIdx >= 0 {
					if m.historyNavIdx < len(m.historyNavEntries)-1 {
						m.historyNavIdx++
						m.editor.SetValue(m.historyNavEntries[m.historyNavIdx])
					} else {
						m.historyNavIdx = -1
						m.editor.SetValue(m.historyNavSaved)
					}
					return m, nil
				}
			}
			// Reset history navigation on any other key in normal mode.
			if msg.String() != "up" && msg.String() != "down" {
				m.historyNavIdx = -1
			}
		}

		m.editor, cmd = m.editor.Update(msg)
		return m, tea.Batch(cmd, m.ensureSchemaCompletionFetch())
	case FocusResults:
		// Clear dd pending state on any non-'d' key.
		if msg.String() != "d" {
			m.resultsPendingD = false
		}
		// Search mode (g/) intercepts all keys.
		if m.searching {
			updateSearchMatcher := func() {
				if m.searchQuery == "" {
					m.results.SetSearchMatcher(nil)
				} else {
					m.results.SetSearchMatcher(compileSearchPattern(m.searchQuery))
				}
			}
			switch msg.String() {
			case "esc":
				m.searching = false
				m.searchQuery = ""
				m.results.SetSearchMatcher(nil)
				return m, nil
			case "enter":
				query := m.searchQuery
				m.searching = false
				m.searchQuery = ""
				m.applySearch(query)
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					updateSearchMatcher()
				}
				return m, nil
			case "ctrl+c":
				m.searching = false
				m.searchQuery = ""
				m.results.SetSearchMatcher(nil)
				return m, nil
			}
			if ch, ok := keyFilterChar(msg); ok {
				m.searchQuery += ch
				updateSearchMatcher()
				return m, nil
			}
			return m, nil
		}
		// Backend search mode intercepts all keys.
		if m.backendSearching {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.cancelBackendSearch()
				return m, nil
			case "enter":
				m.commitBackendSearch()
				return m, nil
			case "backspace":
				if len(m.backendSearchInput) > 0 {
					m.backendSearchInput = m.backendSearchInput[:len(m.backendSearchInput)-1]
					return m, m.scheduleBackendSearch()
				}
				return m, nil
			}
			if ch, ok := keyFilterChar(msg); ok {
				m.backendSearchInput += ch
				return m, m.scheduleBackendSearch()
			}
			return m, nil
		}
		// Visual mode intercepts movement and commit/cancel keys.
		if m.results.IsVisualMode() {
			switch msg.String() {
			case "esc", "V":
				m.results.ClearVisualMode()
				return m, nil
			case "enter":
				m.commitVisualMarks()
				m.results.ClearVisualMode()
				return m, nil
			case "p":
				// Fill current column across the visual range (dirty only).
				return m, m.fillVisualRange()
			case "j", "down":
				m.results.CursorDown()
				return m, nil
			case "k", "up":
				m.results.CursorUp()
				return m, nil
			case "g":
				m.results.CursorTop()
				return m, nil
			case "G":
				m.results.CursorBottom()
				return m, nil
			case "ctrl+c":
				m.results.ClearVisualMode()
				return m, nil
			}
			return m, nil
		}
		// If currently editing a cell, intercept keys first.
		if m.results.IsEditing() {
			switch msg.String() {
			case "enter":
				m.results.CommitEdit()
				return m, nil
			case "esc":
				m.results.CancelEdit()
				return m, nil
			case "ctrl+c":
				m.results.CancelEdit()
				return m, nil
			}
			// All other keys go to the textinput.
			m.results, cmd = m.results.Update(msg)
			return m, cmd
		}

		// Insert row must work on empty editable tables too.
		if msg.String() == "A" {
			if m.results.IsEditable() && !m.results.HasDirtyCells() && !m.inspector.IsInserting() {
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.startInsert()
				return m, nil
			}
		}

		// g e — explain query plan (works regardless of whether results are loaded).
		if msg.String() == "e" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			return m, m.explainQuery()
		}
		// g E — EXPLAIN ANALYZE (runs the query; confirm_destructive gated).
		if msg.String() == "E" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			return m, m.explainQueryAnalyze()
		}

		// g X — open the export dialog (format + columns + scope). Capital X
		// avoids the g x (close tab) tab-management prefix.
		if msg.String() == "X" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			if m.results.NumRows() > 0 {
				m.exportOverlay.Show(
					m.results.ColumnNames(),
					m.results.SourceTable() != "",
					strings.TrimSpace(m.lastQuery) != "",
					m.results.MarkCount(),
					m.results.NumRows(),
					m.totalRows, m.totalRowsSet,
				)
				m.layoutWorkspace()
			}
			return m, nil
		}

		// g r — relationship explorer: toggles the docked panel, a navigable
		// object-graph view of the focused row's inbound + outbound FK edges
		// with live counts that re-roots as the cursor moves. Same as `:explore`.
		if msg.String() == "r" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			return m, m.openDockedExplorer()
		}

		// g R — static ERD: a Mermaid erDiagram of the current table's FK
		// neighbourhood (or the whole schema when no table is focused), shown in
		// a scrollable panel. Same as `:erd [table]`.
		if msg.String() == "R" && m.resultsPendingG {
			m.resultsPendingG = false
			m.resultsPendingY = false
			return m, m.openERD(m.currentTable())
		}

		// g/G navigation works on empty tables too.
		if msg.String() == "g" && !m.resultsPendingG {
			m.resultsPendingG = true
			return m, nil
		}
		if msg.String() == "G" {
			m.resultsPendingG = false
			m.resultsPendingY = false
			m.results.CursorBottom()
			return m, nil
		}

		// Cell cursor navigation.
		if m.results.NumRows() > 0 {
			switch msg.String() {
			case "up", "k":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorUp()
				return m, nil
			case "down", "j":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorDown()
				return m, nil
			case "left", "h":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorLeft()
				m.syncInspectorFieldFromGrid()
				return m, nil
			case "0":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorFirstCol()
				m.syncInspectorFieldFromGrid()
				return m, nil
			case "$":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorLastCol()
				m.syncInspectorFieldFromGrid()
				return m, nil
			case "b":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					return m, m.goBackQuery()
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorLeft()
				m.syncInspectorFieldFromGrid()
				return m, nil
			case "d":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					m.resultsPendingD = false
					return m, m.followForeignKey()
				}
				if !m.results.IsEditable() || m.results.NumRows() == 0 {
					m.resultsPendingD = false
					return m, nil
				}
				if m.resultsPendingD {
					m.resultsPendingD = false
					return m, m.startDeleteRows()
				}
				m.resultsPendingD = true
				return m, nil
			case "right", "l", "w":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorRight()
				m.syncInspectorFieldFromGrid()
				return m, nil
			case "G":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.results.CursorBottom()
				return m, nil
			case "g":
				m.resultsPendingY = false
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.results.CursorTop()
					return m, nil
				}
				m.resultsPendingG = true
				return m, nil
			case "y":
				m.resultsPendingG = false
				if m.resultsPendingY {
					m.resultsPendingY = false
					return m, m.copyCursorCell()
				}
				m.resultsPendingY = true
				return m, nil
			case "r":
				// y r — copy marked/cursor rows as TSV (same as :copyrow).
				// Completes the pending-Y chord; yy remains copy-cell, g r
				// remains the explorer (handled above when pending-G).
				if m.resultsPendingY {
					m.resultsPendingY = false
					m.resultsPendingG = false
					return m, m.copyRowsDelimited(fmtTSV)
				}
			case "p":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if !m.results.IsEditable() || !m.results.HasPrimaryKey() {
					return m, nil
				}
				// With marks, fill the current column across marked rows
				// (dirty only). Without marks, paste into the cursor cell
				// and save immediately. Allowed even when the inspector is
				// open — focus is still on results here.
				if m.results.MarkCount() > 0 {
					return m, m.fillMarkedRows()
				}
				colName := m.results.ColumnName(m.results.CursorCol())
				if m.results.isPKColumn(colName) {
					return m, nil
				}
				if m.results.IsBlobCell(m.results.CursorRow(), m.results.CursorCol()) {
					m.exportMsg = "binary cell — use :saveblob to export"
					return m, nil
				}
				if clip, ok := readOSClipboard(); ok {
					if clip == "" {
						clip = m.yank
					}
					return m, m.pasteIntoCursorCell(clip)
				}
				if cmd := m.beginClipQuery(clipPasteCell, ""); cmd != nil {
					return m, cmd
				}
				return m, m.pasteIntoCursorCell(m.yank)
			case "s":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					return m, m.fetchColumnStats()
				}
			case "e", "i":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.startResultsCellEdit()
			case "E":
				// Expand opens a multi-line peek of the cell under the cursor.
				// It doubles as a read-only viewer when the results can't be
				// written back (read-only mode, custom queries, PK-less views),
				// so it is intentionally not gated on editability like e/i.
				// Close the inspector first when safe (same as e/i / double-click).
				m.resultsPendingG = false
				m.resultsPendingY = false
				if !m.prepareResultsEdit() {
					return m, nil
				}
				return m, m.openCellEditPopup(m.results.CursorRow(), m.results.CursorCol())
			case "n":
				if m.lastSearch != "" {
					m.resultsPendingG = false
					m.resultsPendingY = false
					match := compileSearchPattern(m.lastSearch)
					if row, col := findNextMatch(m.results, match, false); row >= 0 {
						m.results.SetCursor(row, col)
					}
					return m, nil
				}
			case "N":
				if m.lastSearch != "" {
					m.resultsPendingG = false
					m.resultsPendingY = false
					match := compileSearchPattern(m.lastSearch)
					if row, col := findPrevMatch(m.results, match); row >= 0 {
						m.results.SetCursor(row, col)
					}
					return m, nil
				}
			case "ctrl+s":
				if !m.results.IsEditable() && !m.inspector.IsInserting() {
					break
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.saveChanges()
			case "D":
				if !m.results.IsEditable() {
					break
				}
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.discardResultsEdits(false)
				return m, nil
			case "/":
				if m.resultsPendingG {
					// g/ — client-side regex search on loaded page.
					m.resultsPendingG = false
					m.resultsPendingY = false
					if m.results.NumRows() > 0 {
						m.searching = true
						m.searchQuery = ""
					}
					return m, nil
				}
				// / — backend full-text search across all columns.
				m.resultsPendingY = false
				if m.rejectFilter(false) {
					return m, nil
				}
				m.backendSearching = true
				m.backendSearchInput = ""
				return m, nil
			case "f":
				if m.resultsPendingG {
					m.resultsPendingG = false
					m.resultsPendingY = false
					return m, m.openFilterPicker()
				}
			case "c":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if len(m.filters) > 0 {
					return m, m.clearFilters()
				}
			case "C":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.MarkCount() > 0 || m.results.ColumnMarkCount() > 0 {
					m.results.ClearAllMarks()
				}
				return m, nil
			case "M":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.NumCols() == 0 {
					return m, nil
				}
				if !m.results.ToggleColumnMark() {
					m.schemaMsg = "mark at most 2 columns (label, then value) — then :bar / :line"
				}
				return m, nil
			case "u":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if len(m.filters) > 0 {
					return m, m.undoFilter()
				}
			case "*":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.quickFilterCell(false)
			case "!":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.quickFilterCell(true)
			case " ":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.IsEditable() && m.results.NumRows() > 0 {
					m.results.ToggleMark()
				}
				return m, nil
			case "F":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.filterByMarks()
			case "o":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.toggleSort()
			case "x":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.exportToCSV()
			case "Y":
				m.resultsPendingG = false
				m.resultsPendingY = false
				if cmd := m.copyRowsAsInsert(); cmd != nil {
					return m, cmd
				}
			case "P":
				m.resultsPendingG = false
				m.resultsPendingY = false
				return m, m.cloneRows()
			case "H":
				if m.resultsPendingG {
					// g H — show all columns.
					m.resultsPendingG = false
					m.resultsPendingY = false
					m.results.ShowAllColumns()
					return m, nil
				}
				// H — hide the column under the cursor.
				m.resultsPendingG = false
				m.resultsPendingY = false
				if m.results.NumCols() > 0 {
					m.results.HideColumn(m.results.CursorCol())
					m.syncInspectorFieldFromGrid()
				}
				return m, nil
			case ">":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resizeResultsColumn(colResizeStep)
				return m, nil
			case "<":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resizeResultsColumn(-colResizeStep)
				return m, nil
			case "=":
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resetResultsColumnWidth()
				return m, nil
			case "v":
				// v — open the column-visibility overlay.
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resultsPendingD = false
				if m.results.NumCols() > 0 {
					m.openColumnPicker()
					return m, nil
				}
			case "V":
				// V — enter line-wise visual mode for bulk row marking.
				m.resultsPendingG = false
				m.resultsPendingY = false
				m.resultsPendingD = false
				if m.results.IsEditable() && m.results.NumRows() > 0 {
					m.results.SetVisualMode()
				}
				return m, nil
			}
			m.resultsPendingG = false
			m.resultsPendingY = false
			m.results, cmd = m.results.Update(msg)
			return m, cmd
		}

		m.resultsPendingG = false
		m.resultsPendingY = false
		m.results, cmd = m.results.Update(msg)
	case FocusConnections:
		// Fuzzy filter mode intercepts all keys.
		if m.sidebarFiltering {
			switch msg.String() {
			case "esc":
				m.sidebarFiltering = false
				m.sidebarFilter = ""
				m.sidebarCursor = 0
				return m, nil
			case " ":
				// Exit filter mode and toggle expand on the highlighted table.
				if item := m.currentSidebarItem(); item != nil && item.isTableRow() {
					selected := item.text
					m.sidebarFiltering = false
					m.sidebarFilter = ""
					m.syncSidebarCursorToTable(selected)
					m.toggleExpand()
					return m, nil
				}
				m.sidebarFiltering = false
				m.sidebarFilter = ""
				return m, nil
			case "enter":
				// Select the highlighted match in the full sidebar list.
				if item := m.currentSidebarItem(); item != nil && item.isTableRow() {
					selected := item.text
					m.sidebarFiltering = false
					m.sidebarFilter = ""
					m.syncSidebarCursorToTable(selected)
					return m, nil
				}
				m.sidebarFiltering = false
				m.sidebarFilter = ""
				m.sidebarCursor = 0
				return m, nil
			case "backspace":
				if len(m.sidebarFilter) > 0 {
					m.sidebarFilter = m.sidebarFilter[:len(m.sidebarFilter)-1]
				}
				m.sidebarCursor = 0
				return m, nil
			case "up", "k":
				m = m.scrollSidebar(-1)
				return m, nil
			case "down", "j":
				m = m.scrollSidebar(1)
				return m, nil
			case "ctrl+c":
				m.sidebarFiltering = false
				m.sidebarFilter = ""
				m.sidebarCursor = 0
				return m, nil
			}
			// Printable characters extend the filter.
			if ch, ok := keyFilterChar(msg); ok {
				m.sidebarFilter += ch
				m.sidebarCursor = 0
				return m, nil
			}
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			m.sidebarPendingG = false
			m = m.scrollSidebar(-1)
			return m, nil
		case "down", "j":
			m.sidebarPendingG = false
			m = m.scrollSidebar(1)
			return m, nil
		case "G":
			m.sidebarPendingG = false
			items := m.sidebarItems()
			if len(items) > 0 {
				m.sidebarCursor = len(items) - 1
			}
			return m, nil
		case "g":
			if m.sidebarPendingG {
				m.sidebarPendingG = false
				m.sidebarCursor = 0
				return m, nil
			}
			m.sidebarPendingG = true
			return m, nil
		case " ":
			m.sidebarPendingG = false
			m.toggleExpand()
			return m, nil
		case "l", "right":
			m.sidebarPendingG = false
			m.focus = FocusResults
			m.applyFocus()
			return m, nil
		case "/":
			m.sidebarFiltering = true
			m.sidebarFilter = ""
			m.sidebarCursor = 0
			return m, nil
		case "enter", "s":
			item := m.currentSidebarItem()
			if item != nil {
				return m, m.sidebarActivateItem(item)
			}
		case "d":
			m.sidebarPendingG = false
			if m.sidebarSelectedTable() != "" {
				return m, m.openSchemaPanel()
			}
			return m, nil
		case "T":
			m.sidebarPendingG = false
			item := m.currentSidebarItem()
			if item != nil && item.isTableRow() && (item.schema == "" || item.schema == m.currentSchemaName()) {
				if m.confirmDestructive() {
					m.truncateConfirm = item.text
					return m, nil
				}
				return m, m.execTruncate(item.text)
			}
		case "D":
			m.sidebarPendingG = false
			item := m.currentSidebarItem()
			if item != nil && item.isTableRow() && (item.schema == "" || item.schema == m.currentSchemaName()) {
				if m.confirmDestructive() {
					m.dropTableConfirm = item.text
					m.dropTableInput = ""
					return m, nil
				}
				return m, m.execDropTable(item.text)
			}
		case "r":
			m.sidebarPendingG = false
			item := m.currentSidebarItem()
			if item != nil && item.isTableRow() && (item.schema == "" || item.schema == m.currentSchemaName()) {
				return m, m.openTableRenameForm(item.text)
			}
		case "a":
			m.sidebarPendingG = false
			if m.sidebarSelectedActiveTable() != "" {
				return m, m.openAddColumnForm()
			}
		case "N":
			m.sidebarPendingG = false
			return m, m.openCreateTableForm()
		case "X":
			m.sidebarPendingG = false
			m.exportPicker.Show(m.tables, m.currentTable())
			m.layoutWorkspace()
			return m, nil
		case "I":
			m.sidebarPendingG = false
			m.importPrompt.Show("~/Downloads/")
			return m, nil
		case "S":
			m.sidebarPendingG = false
			if m.connection != nil && len(m.tables) > 0 {
				m.crossSearch.Show()
				m.layoutWorkspace()
				return m, m.editor.Focus()
			}
		}
		m.connList, cmd = m.connList.Update(msg)
	case FocusInspector:
		if m.inspector.IsEditing() {
			switch msg.String() {
			case "enter":
				m.commitInspectorFieldEdit()
				return m, nil
			case "esc", "ctrl+c":
				m.inspector.CancelEdit()
				return m, nil
			case "up", "k":
				// Moving the field cursor must end the in-flight edit first;
				// otherwise the shared textinput keeps rendering on the newly
				// focused field (same failure mode as results-grid click-away).
				m.commitInspectorFieldEdit()
				m.inspector.CursorUp(m.inspectorResults())
				m.syncGridColFromInspector()
				return m, nil
			case "down", "j":
				m.commitInspectorFieldEdit()
				m.inspector.CursorDown(m.inspectorResults())
				m.syncGridColFromInspector()
				return m, nil
			}
			m.inspector, cmd = m.inspector.Update(msg)
			return m, cmd
		}
		if m.inspector.IsFiltering() {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.inspector.CancelFilter()
				m.inspector.cursorField = 0
				return m, nil
			case "enter":
				m.inspector.CommitFilter(m.inspectorResults())
				m.syncGridColFromInspector()
				return m, nil
			case "backspace":
				m.inspector.FilterBackspace()
				return m, nil
			case "up", "k":
				m.inspector.CursorUp(m.inspectorResults())
				m.syncGridColFromInspector()
				return m, nil
			case "down", "j":
				m.inspector.CursorDown(m.inspectorResults())
				m.syncGridColFromInspector()
				return m, nil
			}
			if ch, ok := keyFilterChar(msg); ok {
				m.inspector.FilterAddChar(ch)
				return m, nil
			}
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			m.inspector.pendingG = false
			src := m.inspectorResults()
			if m.inspector.JSONTreeActive() {
				if m.inspector.JSONTreeUp(src) {
					return m, nil
				}
			}
			m.inspector.CursorUp(src)
			m.syncGridColFromInspector()
			return m, nil
		case "down", "j":
			m.inspector.pendingG = false
			src := m.inspectorResults()
			if m.inspector.JSONTreeActive() {
				if m.inspector.JSONTreeDown(src) {
					return m, nil
				}
			}
			m.inspector.CursorDown(src)
			m.syncGridColFromInspector()
			return m, nil
		case "G":
			m.inspector.pendingG = false
			src := m.inspectorResults()
			if m.inspector.JSONTreeActive() {
				m.inspector.JSONTreeBottom(src)
				return m, nil
			}
			m.inspector.CursorBottom(src)
			m.syncGridColFromInspector()
			return m, nil
		case "g":
			if m.inspector.pendingG {
				m.inspector.pendingG = false
				src := m.inspectorResults()
				if m.inspector.JSONTreeActive() {
					m.inspector.JSONTreeTop(src)
					return m, nil
				}
				m.inspector.CursorTop(src)
				m.syncGridColFromInspector()
				return m, nil
			}
			m.inspector.pendingG = true
			return m, nil
		case "d":
			// g d — follow FK on the focused inspector field (same as results).
			if m.inspector.pendingG {
				m.inspector.pendingG = false
				return m, m.followForeignKey()
			}
			return m, nil
		case "b":
			// g b — pop the FK / drill navigation stack (pairs with g d).
			if m.inspector.pendingG {
				m.inspector.pendingG = false
				return m, m.inspectorGoBack()
			}
			return m, nil
		case "u":
			// Same stack-back as the explorer; g b remains the results-paired chord.
			m.inspector.pendingG = false
			return m, m.inspectorGoBack()
		case "/":
			m.inspector.StartFilter()
			return m, nil
		case "o", "enter":
			m.inspector.pendingG = false
			if m.inspector.ToggleJSONFold(m.inspectorResults()) {
				return m, nil
			}
			return m, nil
		case "l", "right":
			m.inspector.pendingG = false
			if m.inspector.JSONTreeExpand(m.inspectorResults()) {
				return m, nil
			}
			return m, nil
		case "h", "left":
			m.inspector.pendingG = false
			if m.inspector.JSONTreeCollapse(m.inspectorResults()) {
				return m, nil
			}
			return m, nil
		case "e", "i":
			m.inspector.pendingG = false
			src := m.inspectorResults()
			col := m.inspector.selectedColumn(src)
			if !m.inspector.IsInserting() && m.results.IsBlobCell(m.results.CursorRow(), col) {
				return m, m.openCellEditPopup(m.results.CursorRow(), col)
			}
			// JSON is view-only in the inspector fold; edit via the E popup.
			if !m.inspector.IsInserting() && m.inspector.FocusedFieldIsJSON(src) {
				return m, m.openCellEditPopup(m.results.CursorRow(), col)
			}
			if !m.inspector.IsInserting() && m.inspector.IsFieldTruncated(src) {
				return m, m.openCellEditPopup(m.results.CursorRow(), col)
			}
			m.inspector.StartFieldEdit(src)
			return m, nil
		case "E":
			m.inspector.pendingG = false
			if !m.inspector.IsInserting() {
				col := m.inspector.selectedColumn(m.inspectorResults())
				return m, m.openCellEditPopup(m.results.CursorRow(), col)
			}
		case "ctrl+s":
			m.inspector.pendingG = false
			if m.results.IsEditable() || m.inspector.IsInserting() {
				return m, m.saveChanges()
			}
			return m, nil
		case "A":
			m.inspector.pendingG = false
			if m.results.IsEditable() && !m.results.HasDirtyCells() && !m.inspector.IsInserting() {
				m.startInsert()
			}
			return m, nil
		case "esc":
			if m.inspector.JSONTreeActive() {
				m.inspector.CollapseJSONTree()
				return m, nil
			}
			if m.inspector.IsInserting() {
				m.inspector.CancelInsert()
				return m, m.maybeRestoreExplorerAfterInsert()
			}
		case "D":
			m.inspector.pendingG = false
			if m.results.HasDirtyCells() {
				if m.confirmDestructive() {
					m.discardConfirm = true
					return m, nil
				}
				m.results.DiscardEdits()
			}
			return m, nil
		}
		return m, nil
	case FocusAssistant:
		// Route all keys to the panel. Global focus movement (ctrl+h/j/k/l)
		// is handled before this switch, so it still works; esc in compose
		// mode just leaves compose, and esc in browse mode closes the panel.
		a, acmd := m.assistant.HandleKey(msg)
		m.assistant = a
		return m, acmd
	case FocusExplorer:
		// Docked relationship-explorer panel. Non-modal: global focus movement
		// (ctrl+h/l) is handled before this switch. Tree nav: j/k move, → expands,
		// ← collapses, Enter re-roots the grid, t opens the node in a new tab,
		// A inserts related, u/g b goes back, r retargets, esc/q close the panel.
		switch msg.String() {
		case "esc", "q":
			m.closeDockedExplorer()
			return m, nil
		case "enter":
			return m, m.explorerActivate()
		case "t":
			return m, m.explorerOpenInTab()
		case "right", "l":
			return m, m.explorerExpand()
		case "left", "h":
			m.explorerCollapse()
			return m, nil
		case "A":
			return m, m.explorerInsertRelated()
		case "u", "backspace":
			if len(m.queryStack) == 0 {
				m.schemaMsg = "nothing to go back to"
				return m, nil
			}
			return m, m.goBackQuery()
		case "b":
			// g b from the explorer — same as results.
			if m.resultsPendingG {
				m.resultsPendingG = false
				if len(m.queryStack) == 0 {
					m.schemaMsg = "nothing to go back to"
					return m, nil
				}
				return m, m.goBackQuery()
			}
		case "g":
			m.resultsPendingG = true
			return m, nil
		case "r":
			m.resultsPendingG = false
			m.explorer.markLoading()
			return m, m.loadExplorer()
		}
		m.resultsPendingG = false
		m.explorer = m.explorer.Update(msg)
		return m, nil
	}
	return m, cmd
}
