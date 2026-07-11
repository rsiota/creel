package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// doubleClickInterval is the maximum gap between two left-clicks on the same
// results cell that is interpreted as a double-click (entering inline edit).
const doubleClickInterval = 500 * time.Millisecond

// handleConnectionsMouse routes mouse events on the connection list screen.
func (m Model) handleConnectionsMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Type != tea.MouseLeft {
		return m, nil
	}

	// Connection popup is centered at popupDim() = 71×19.
	pw, ph := popupDim()
	panelX := (m.width - pw) / 2
	panelY := (m.height - 1 - ph) / 2

	// Inside the border: border(1) + prompt(1) = offset 2 for first entry.
	// Each entry renders as 2 lines (name + detail).
	listY := msg.Y - panelY - 2
	if listY < 0 || msg.X < panelX || msg.X >= panelX+pw {
		return m, nil
	}
	idx := listY / 2
	items := m.connList.VisibleItemsForMouse()
	if idx < 0 || idx >= len(items) {
		return m, nil
	}
	m.connList.SetCursor(idx)
	if m.connList.IsFiltering() {
		m.connList.CommitFilter()
	}
	return m, m.connectToDB()
}

// handleDatabasePickerMouse routes mouse events on the database picker overlay.
func (m Model) handleDatabasePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Scroll wheel — scroll database list.
	if msg.Type == tea.MouseWheelUp {
		m.dbPicker.CursorUp()
		return m, nil
	}
	if msg.Type == tea.MouseWheelDown {
		m.dbPicker.CursorDown()
		return m, nil
	}
	if msg.Type != tea.MouseLeft {
		return m, nil
	}

	// Database picker is centered at popupDim() = 71×19.
	pw, ph := popupDim()
	panelX := (m.width - pw) / 2
	panelY := (m.height - 1 - ph) / 2

	// Inside the border: border(1) + prompt(1) = offset 2 for first entry.
	// Each entry renders as 1 line.
	listY := msg.Y - panelY - 2
	if listY < 0 || msg.X < panelX || msg.X >= panelX+pw {
		return m, nil
	}
	idx := m.dbPicker.ScrollRow() + listY
	m.dbPicker.SetCursor(idx)

	// Verify the cursor landed on a real item.
	name := m.dbPicker.SelectedDatabase()
	if name == "" {
		return m, nil
	}
	if m.dbPicker.Filtering() {
		m.dbPicker.StopFiltering()
	}
	m.dbPicker.Hide()
	return m, m.selectDatabase(name)
}

// handleWorkspaceMouse routes mouse events to the appropriate panel.
func (m Model) handleWorkspaceMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Click-outside-to-dismiss for overlay panels (left-click only).
	if msg.Type == tea.MouseLeft {
		if dismissed := m.dismissOverlayOnOutsideClick(msg); dismissed {
			return m, nil
		}
	}

	// Database picker overlay.
	if m.dbPicker.IsVisible() {
		return m.handleDatabasePickerMouse(msg)
	}

	// Ignore mouse when inline editors or text-input modes are active.
	if m.tableDesigner.IsVisible() || m.schemaEditor.IsVisible() ||
		m.columnJumping || m.searching || m.backendSearching ||
		m.cellEdit.IsVisible() {
		return m, nil
	}

	sidebarWidth := 30
	editorHeight := 12
	if m.editorMaximized {
		editorHeight = m.height - 1 - 2 - 12
		if editorHeight < 8 {
			editorHeight = 8
		}
	}

	// Right edge of the editor/results area; the inspector sits beyond it.
	editorRight := m.width
	if m.inspector.IsVisible() {
		editorRight = m.width - InspectorWidth
	}

	resultsTop := editorHeight
	resultsBottom := m.height - 2

	// ── Inspector (right column) ──────────────────────────────
	if m.inspector.IsVisible() && msg.X >= editorRight && msg.Y < resultsBottom {
		if msg.Type != tea.MouseLeft {
			return m, nil
		}
		m.focus = FocusInspector
		m.applyFocus()

		// Map the click to a field. The inspector's top border is at Y=0,
		// so content starts at Y=1.
		col := m.inspector.ClickField(msg.Y-1, m.results)

		// Double-click on the same field within doubleClickInterval →
		// enter field edit mode (mirrors the "e"/"i" key binding).
		if col >= 0 &&
			!m.inspector.IsEditing() &&
			!m.lastInspectorClickTime.IsZero() &&
			time.Since(m.lastInspectorClickTime) <= doubleClickInterval &&
			m.lastInspectorClickCol == col {
			m.lastInspectorClickTime = time.Time{}
			return m, m.startInspectorFieldEdit()
		}
		m.lastInspectorClickTime = time.Now()
		m.lastInspectorClickCol = col
		return m, nil
	}

	// ── Sidebar (left column) ─────────────────────────────────
	// A click anywhere in the sidebar — border, empty space, or a table —
	// focuses it so its frame is highlighted.
	if msg.X < sidebarWidth && msg.Y < resultsBottom {
		switch msg.Type {
		case tea.MouseWheelUp:
			m = m.scrollSidebar(-1)
			return m, nil
		case tea.MouseWheelDown:
			m = m.scrollSidebar(1)
			return m, nil
		case tea.MouseLeft:
			m.focus = FocusConnections
			m.applyFocus()
			items := m.sidebarItems()
			if len(items) == 0 {
				return m, nil
			}
			start := m.sidebarRenderedStart()
			idx := start + msg.Y - 1 // -1 for top border
			if idx < 0 || idx >= len(items) {
				return m, nil
			}
			m.sidebarCursor = idx
			m.sidebarScroll = start // freeze the view so the clicked table stays put
			m.sidebarViewAnchored = true
			item := &items[idx]
			if item.isColumn {
				return m, nil
			}
			m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s;", item.text))
			return m, m.executeQuery()
		}
		return m, nil
	}

	// ── Tab bar (inside editor panel) ─────────────────────────
	// Tab text sits at Y=1 (below the editor's top border).
	if msg.Type == tea.MouseLeft && msg.Y == 1 && msg.X >= sidebarWidth && msg.X < editorRight {
		m.focus = FocusTabBar
		m.applyFocus()
		relX := msg.X - sidebarWidth - 1 // -1 for editor's left border
		result := m.tabBar.ClickAt(relX)
		if result >= 0 {
			m.setActiveTab(result)
		} else if result == -1 {
			query := m.editor.Value()
			m.addTab(generateTabTitle(query), query)
		}
		return m, nil
	}

	// ── Editor (top-right) ────────────────────────────────────
	// Any click within the editor panel (border, separator, or text area)
	// that is not the tab bar focuses the editor. The underlying textarea has
	// no mouse support, so the cursor stays where it was — focus is what
	// matters here.
	if msg.Y < resultsTop && msg.X >= sidebarWidth && msg.X < editorRight {
		if msg.Type == tea.MouseLeft {
			m.focus = FocusEditor
			m.applyFocus()
			return m, m.editor.Focus()
		}
		return m, nil
	}

	// ── Results panel (bottom-right) ──────────────────────────
	if msg.Y < resultsTop || msg.Y > resultsBottom {
		return m, nil
	}

	// Any left-click within the results panel focuses it — whether on a cell,
	// the header, or empty space.
	if msg.Type == tea.MouseLeft {
		m.focus = FocusResults
		m.resultsPendingG = false
		m.resultsPendingY = false
		m.resultsPendingD = false
		m.applyFocus()
	}

	// Scroll wheel — scroll results vertically.
	if msg.Type == tea.MouseWheelUp {
		m.results.ScrollUp()
		return m, nil
	}
	if msg.Type == tea.MouseWheelDown {
		m.results.ScrollDown()
		return m, nil
	}

	if msg.Type != tea.MouseLeft || !m.results.HasResult() || m.results.NumCols() == 0 {
		return m, nil
	}

	// Left-click on header row → sort by that column.
	headerY := resultsTop + 1 // border (0), header (1)
	if msg.Y == headerY {
		colIdx := m.results.ColumnAtX(msg.X - sidebarWidth)
		if colIdx >= 0 {
			colName := m.results.ColumnName(colIdx)
			if colName != "" {
				return m, m.sortByColName(colName)
			}
		}
		return m, nil
	}

	// Left-click on a data row → move cursor to that cell. Data starts two
	// rows below the header: the header itself, then the header separator.
	dataRow := msg.Y - headerY - 2
	if dataRow >= 0 {
		rowIdx := m.results.ScrollRow() + dataRow
		colIdx := m.results.ColumnAtX(msg.X - sidebarWidth)
		if rowIdx >= 0 && rowIdx < m.results.NumRows() && colIdx >= 0 {
			m.results.SetCursor(rowIdx, colIdx)

			// Double-click on the same cell within doubleClickInterval →
			// enter inline edit mode (mirrors the "e"/"i" key binding).
			cell := cellRef{row: rowIdx, col: colIdx}
			if !m.results.IsEditing() &&
				!m.lastResultsClickTime.IsZero() &&
				time.Since(m.lastResultsClickTime) <= doubleClickInterval &&
				m.lastResultsClickCell == cell {
				m.lastResultsClickTime = time.Time{}
				return m, m.startResultsCellEdit()
			}
			m.lastResultsClickTime = time.Now()
			m.lastResultsClickCell = cell
		}
	}

	return m, nil
}

// dismissOverlayOnOutsideClick closes the topmost visible overlay if the
// mouse click lands outside its bounds. Returns true if an overlay was dismissed.
func (m *Model) dismissOverlayOnOutsideClick(msg tea.MouseMsg) bool {
	// Helper: compute centered overlay bounds.
	centeredRect := func(panelW, panelH int) (int, int, int, int) {
		x := (m.width - panelW) / 2
		y := (m.height - 1 - panelH) / 2
		return x, y, panelW, panelH
	}
	outside := func(x, y, w, h int) bool {
		return msg.X < x || msg.X >= x+w || msg.Y < y || msg.Y >= y+h
	}

	// Help panel is fullscreen — any click dismisses.
	if m.help.IsVisible() {
		m.help.Hide()
		return true
	}

	// 65% centered overlays.
	if m.history.IsVisible() {
		w := m.width * 65 / 100
		h := (m.height - 1) * 65 / 100
		x, y, pw, ph := centeredRect(w, h)
		if outside(x, y, pw, ph) {
			m.history.Toggle()
			return true
		}
		return false // click inside — let it live
	}
	if m.bookmarks.IsVisible() {
		w := m.width * 65 / 100
		h := (m.height - 1) * 65 / 100
		x, y, pw, ph := centeredRect(w, h)
		if outside(x, y, pw, ph) {
			m.bookmarks.Toggle()
			return true
		}
		return false
	}
	if m.crossSearch.IsVisible() {
		w := m.width * 65 / 100
		h := (m.height - 1) * 65 / 100
		x, y, pw, ph := centeredRect(w, h)
		if outside(x, y, pw, ph) {
			m.crossSearch.Hide()
			return true
		}
		return false
	}

	// popupDim() overlays (71×19).
	pw, ph := popupDim()
	px, py, _, _ := centeredRect(pw, ph)
	if m.palette.IsVisible() {
		if outside(px, py, pw, ph) {
			m.palette.Hide()
			return true
		}
		return false
	}
	if m.filterPicker.IsVisible() {
		if outside(px, py, pw, ph) {
			m.filterPicker.Hide()
			return true
		}
		return false
	}
	if m.columnPicker.IsVisible() {
		if outside(px, py, pw, ph) {
			m.columnPicker.Hide()
			return true
		}
		return false
	}
	if m.exportPicker.IsVisible() {
		if outside(px, py, pw, ph) {
			m.exportPicker.Hide()
			return true
		}
		return false
	}

	return false
}
