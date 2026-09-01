package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// doubleClickInterval is the maximum gap between two left-clicks on the same
// results cell that is interpreted as a double-click (entering inline edit).
const doubleClickInterval = 500 * time.Millisecond

// connFormWheelInterval collapses a macOS / trackpad wheel notch (often 3
// rapid MouseWheel events) into a single field step so the form cursor moves
// one field at a time rather than jumping three.
const connFormWheelInterval = 80 * time.Millisecond

// handleConnectionFormMouse routes mouse events on the add/edit connection form.
func (m Model) handleConnectionFormMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	panelW, panelH, panelX, panelY := m.connFormPopupDims()

	if msg.X < panelX || msg.X >= panelX+panelW || msg.Y < panelY || msg.Y >= panelY+panelH {
		return m, nil
	}

	switch msg.Type {
	case tea.MouseWheelUp, tea.MouseWheelDown:
		// Debounce: one field step per notch, not per raw event.
		if !m.lastConnFormWheelTime.IsZero() &&
			time.Since(m.lastConnFormWheelTime) < connFormWheelInterval {
			return m, nil
		}
		m.lastConnFormWheelTime = time.Now()
		if m.connForm.editing {
			m.connForm.fields[m.connForm.activeField()].Blur()
			m.connForm.editing = false
			m.connForm.pathComp.clear()
		}
		if msg.Type == tea.MouseWheelUp {
			m.connForm.moveActive(-1)
		} else {
			m.connForm.moveActive(1)
		}
		return m, nil
	case tea.MouseLeft:
		// Content starts below the panel's top border.
		fi := m.connForm.ClickField(msg.Y - panelY - 1)
		if fi < 0 {
			return m, nil
		}

		if !m.connForm.editing &&
			!m.lastConnFormClickTime.IsZero() &&
			time.Since(m.lastConnFormClickTime) <= doubleClickInterval &&
			m.lastConnFormClickField == fi {
			m.lastConnFormClickTime = time.Time{}
			cmd := m.connForm.StartFieldEdit()
			return m, cmd
		}
		m.lastConnFormClickTime = time.Now()
		m.lastConnFormClickField = fi
		return m, nil
	}
	return m, nil
}

// handleConnectionsMouse routes mouse events on the connection list screen.
func (m Model) handleConnectionsMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Type != tea.MouseLeft {
		return m, nil
	}

	// Connection popup is dynamically sized and centered (see connListPopupDims).
	pw, ph := m.connListPopupDims()
	panelX := (m.width - pw) / 2
	panelY := (m.height - 1 - ph) / 2

	// Inside the border: border(1) + prompt(1) = offset 2 to the first row.
	listY := msg.Y - panelY - 2
	if listY < 0 || msg.X < panelX || msg.X >= panelX+pw {
		return m, nil
	}
	idx := m.connList.YToRow(listY)
	if idx < 0 {
		return m, nil
	}
	m.connList.SetCursor(idx)
	// Clicking a group header folds/unfolds it; clicking a connection selects
	// and connects (matching the keyboard enter behaviour).
	if m.connList.CursorOnGroupHeader() {
		m.connList.ToggleGroupAtCursor()
		return m, nil
	}
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

	// Ignore mouse when inline editors or text-input modes are active. The
	// schema editor and table designer are handled separately below (they
	// replace the editor/results panels and own content-area clicks).
	if m.ex.visible || m.searching || m.backendSearching ||
		m.cellEdit.IsVisible() {
		m.splitDragging = false
		m.sidebarDragging = false
		m.rightSlotDragging = false
		m.colResizeDragging = false
		return m, nil
	}

	// ── Panel split drags ─────────────────────────────────────
	// Action-first (same shape as ERD drag): motion while a button is held
	// arrives as Type=MouseLeft + Action=Motion, so a Type switch would
	// re-enter the press path. See docs/tui-mouse.md.
	if m.splitDragging {
		return m.handleSplitDrag(msg)
	}
	if m.sidebarDragging {
		return m.handleSidebarDrag(msg)
	}
	if m.rightSlotDragging {
		return m.handleRightSlotDrag(msg)
	}
	if m.colResizeDragging {
		return m.handleColResizeDrag(msg)
	}

	g := m.workspaceGeom()
	sidebarWidth := g.SidebarWidth
	editorRight := g.EditorRight
	resultsTop := g.ResultsTop
	resultsBottom := g.ResultsBottom

	// Press on a panel seam starts a resize drag. Horizontal seams before
	// the editor/results seam so T-junctions prefer horizontal resize.
	if msg.Type == tea.MouseLeft && msg.Action != tea.MouseActionMotion &&
		m.onSidebarSplit(msg.X, msg.Y, g) {
		return m.beginSidebarDrag(msg.X, g)
	}
	if msg.Type == tea.MouseLeft && msg.Action != tea.MouseActionMotion &&
		m.onRightSlotSplit(msg.X, msg.Y, g) {
		return m.beginRightSlotDrag(msg.X, g)
	}
	// Schema editor and table designer replace the editor/results split.
	if !m.schemaEditor.IsVisible() && !m.tableDesigner.IsVisible() &&
		msg.Type == tea.MouseLeft && msg.Action != tea.MouseActionMotion &&
		m.onEditorResultsSplit(msg.X, msg.Y, g) {
		return m.beginSplitDrag(msg.Y, g)
	}

	// ── Structure view (schema editor) ────────────────────────
	// When open it replaces the editor+results content area. Route clicks inside
	// its panel to it; sidebar (X<sidebarWidth) and inspector (X>=editorRight)
	// clicks fall through to their own handlers.
	if m.schemaEditor.IsVisible() && msg.X >= sidebarWidth && msg.X < editorRight && msg.Y < resultsBottom {
		return m.handleSchemaEditorMouse(msg)
	}
	// ── Table designer ────────────────────────────────────────
	// Same content-area takeover as the schema editor.
	if m.tableDesigner.IsVisible() && msg.X >= sidebarWidth && msg.X < editorRight && msg.Y < resultsBottom {
		return m.handleTableDesignerMouse(msg)
	}

	// ── Inspector (right column) ──────────────────────────────
	if m.inspector.IsVisible() && msg.X >= editorRight && msg.Y < resultsBottom {
		if msg.Type != tea.MouseLeft {
			return m, nil
		}
		m.focus = FocusInspector
		m.applyFocus()

		// Map the click to a field. The inspector's top border is at Y=0,
		// so content starts at Y=1.
		col := m.inspector.ClickField(msg.Y-1, m.inspectorResults())

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

	// ── Assistant (right column) ──────────────────────────────
	// Shares the inspector's slot; a click anywhere in it focuses the panel
	// and enters compose mode. The wheel scrolls the transcript.
	if m.assistant.IsVisible() && msg.X >= editorRight && msg.Y < resultsBottom {
		switch msg.Type {
		case tea.MouseWheelUp:
			m.focus = FocusAssistant
			m.assistant.scrollUp(3)
			return m, nil
		case tea.MouseWheelDown:
			m.focus = FocusAssistant
			m.assistant.scrollDown(3)
			return m, nil
		case tea.MouseLeft:
			m.focus = FocusAssistant
			m.assistant.StartCompose()
			m.applyFocus()
			return m, nil
		}
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
			return m, m.openTable(item.text)
		}
		return m, nil
	}

	// ── Tab bar (inside editor panel) ─────────────────────────
	// Tab text sits at Y=1 (below the editor's top border).
	if m.editorVisible && msg.Type == tea.MouseLeft && msg.Y == 1 && msg.X >= sidebarWidth && msg.X < editorRight {
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
	if m.editorVisible && msg.Y < resultsTop && msg.X >= sidebarWidth && msg.X < editorRight {
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

	// Scroll wheel — accumulate into wheelAccum and apply on wheelTickMsg
	// (one scroll per tick), not one scroll per event. The Magic Mouse /
	// trackpad emit hundreds of momentum wheel events per swipe; applying
	// synchronously here lets the event rate outrun the renderer and the grid
	// appears to keep scrolling after the gesture ends. Coalescing makes each
	// event O(1) with an unchanged View, so the flood drains without renders.
	if msg.Type == tea.MouseWheelUp {
		m.wheelAccum--
		m.viewCached = true // view-neutral: nothing on screen changed yet
		if !m.wheelTickPending {
			m.wheelTickPending = true
			return m, scheduleWheelTick()
		}
		return m, nil
	}
	if msg.Type == tea.MouseWheelDown {
		m.wheelAccum++
		m.viewCached = true // view-neutral: nothing on screen changed yet
		if !m.wheelTickPending {
			m.wheelTickPending = true
			return m, scheduleWheelTick()
		}
		return m, nil
	}

	if msg.Type != tea.MouseLeft || !m.results.HasResult() || m.results.NumCols() == 0 {
		return m, nil
	}

	// Left-click on header row → resize if on a column separator, else sort.
	headerY := resultsTop + 1 // border (0), header (1)
	if msg.Y == headerY {
		relX := msg.X - sidebarWidth
		if col := m.results.ColumnSepAtX(relX); col >= 0 &&
			msg.Action != tea.MouseActionMotion {
			return m.beginColResizeDrag(col, msg.X)
		}
		colIdx := m.results.ColumnAtX(relX)
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
			// Resolve an in-flight inline edit before moving the cursor. Without
			// this, clicking another cell while editing left edit mode active
			// with the original cell's text buffer, which then rendered (in the
			// edit colour) over the newly clicked cell. Commit the edit — mirroring
			// Enter — so it's staged on its own cell, then move. Clicking the cell
			// already being edited is left alone so a stray click there doesn't
			// drop out of the textinput.
			if m.results.IsEditing() && (rowIdx != m.results.CursorRow() || colIdx != m.results.CursorCol()) {
				m.results.CommitEdit()
			}
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

// onEditorResultsSplit reports whether (x,y) sits on the seam between the
// editor and results panels (editor bottom border or results top border),
// within the centre column.
func (m Model) onEditorResultsSplit(x, y int, g workspaceGeom) bool {
	if !m.editorVisible || g.EditorHeight <= 0 {
		return false
	}
	if x < g.SidebarWidth || x >= g.EditorRight {
		return false
	}
	return y == g.ResultsTop-1 || y == g.ResultsTop
}

// onSidebarSplit reports whether (x,y) sits on the seam between the sidebar
// and the centre column (sidebar right border or centre left border).
func (m Model) onSidebarSplit(x, y int, g workspaceGeom) bool {
	if !m.sidebarVisible || g.SidebarWidth <= 0 {
		return false
	}
	if y < 0 || y > g.ResultsBottom {
		return false
	}
	return x == g.SidebarWidth-1 || x == g.SidebarWidth
}

// onRightSlotSplit reports whether (x,y) sits on the seam between the centre
// column and the right slot (centre right border or right-slot left border).
func (m Model) onRightSlotSplit(x, y int, g workspaceGeom) bool {
	if g.RightSlotW <= 0 {
		return false
	}
	if y < 0 || y > g.ResultsBottom {
		return false
	}
	return x == g.EditorRight-1 || x == g.EditorRight
}

// beginSplitDrag starts an editor↔results resize. Dragging while maximized
// exits maximize and adopts the current visual height so the divider doesn't
// jump.
func (m Model) beginSplitDrag(y int, g workspaceGeom) (tea.Model, tea.Cmd) {
	if m.editorMaximized {
		m.editorMaximized = false
		m.editorSplitH = g.EditorHeight
		g = m.workspaceGeom()
	}
	m.splitDragging = true
	m.splitDragOff = y - g.ResultsTop
	return m.applySplitDragY(y)
}

// handleSplitDrag continues or ends an in-flight editor↔results resize.
func (m Model) handleSplitDrag(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionMotion ||
		(msg.Type == tea.MouseLeft && msg.Action != tea.MouseActionRelease) {
		return m.applySplitDragY(msg.Y)
	}
	if msg.Type == tea.MouseRelease || msg.Action == tea.MouseActionRelease {
		m.splitDragging = false
		return m, nil
	}
	return m, nil
}

// applySplitDragY sets editorSplitH so the results top edge tracks the cursor
// (minus the press offset), clamps, and relayouts.
func (m Model) applySplitDragY(y int) (tea.Model, tea.Cmd) {
	m.editorSplitH = y - m.splitDragOff
	m.layoutWorkspace()
	return m, nil
}

// beginSidebarDrag starts a sidebar↔centre resize.
func (m Model) beginSidebarDrag(x int, g workspaceGeom) (tea.Model, tea.Cmd) {
	m.sidebarDragging = true
	m.sidebarDragOff = x - g.SidebarWidth
	return m.applySidebarDragX(x)
}

// handleSidebarDrag continues or ends an in-flight sidebar resize.
func (m Model) handleSidebarDrag(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionMotion ||
		(msg.Type == tea.MouseLeft && msg.Action != tea.MouseActionRelease) {
		return m.applySidebarDragX(msg.X)
	}
	if msg.Type == tea.MouseRelease || msg.Action == tea.MouseActionRelease {
		m.sidebarDragging = false
		return m, nil
	}
	return m, nil
}

// applySidebarDragX sets sidebarSplitW so the centre column's left edge tracks
// the cursor (minus the press offset), clamps, and relayouts.
func (m Model) applySidebarDragX(x int) (tea.Model, tea.Cmd) {
	m.sidebarSplitW = x - m.sidebarDragOff
	m.layoutWorkspace()
	return m, nil
}

// beginRightSlotDrag starts a centre↔right-slot resize.
func (m Model) beginRightSlotDrag(x int, g workspaceGeom) (tea.Model, tea.Cmd) {
	m.rightSlotDragging = true
	m.rightSlotDragOff = x - g.EditorRight
	return m.applyRightSlotDragX(x)
}

// handleRightSlotDrag continues or ends an in-flight right-slot resize.
func (m Model) handleRightSlotDrag(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionMotion ||
		(msg.Type == tea.MouseLeft && msg.Action != tea.MouseActionRelease) {
		return m.applyRightSlotDragX(msg.X)
	}
	if msg.Type == tea.MouseRelease || msg.Action == tea.MouseActionRelease {
		m.rightSlotDragging = false
		return m, nil
	}
	return m, nil
}

// applyRightSlotDragX sets rightSlotSplitW so the centre column's right edge
// tracks the cursor (minus the press offset), clamps, and relayouts.
func (m Model) applyRightSlotDragX(x int) (tea.Model, tea.Cmd) {
	m.rightSlotSplitW = m.width - x + m.rightSlotDragOff
	m.layoutWorkspace()
	return m, nil
}

// beginColResizeDrag starts a results header-separator resize for col.
func (m Model) beginColResizeDrag(col, x int) (tea.Model, tea.Cmd) {
	w := m.results.ColWidth(col)
	if w <= 0 {
		return m, nil
	}
	m.colResizeDragging = true
	m.colResizeCol = col
	m.colResizeStartX = x
	m.colResizeStartW = w
	m.focus = FocusResults
	m.applyFocus()
	return m, nil
}

// handleColResizeDrag continues or ends an in-flight header-separator resize.
func (m Model) handleColResizeDrag(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionMotion ||
		(msg.Type == tea.MouseLeft && msg.Action != tea.MouseActionRelease) {
		return m.applyColResizeDragX(msg.X)
	}
	if msg.Type == tea.MouseRelease || msg.Action == tea.MouseActionRelease {
		m.colResizeDragging = false
		return m, nil
	}
	return m, nil
}

// applyColResizeDragX sets the dragged column's width from the cursor delta
// since press and persists a session override when the grid has a source table.
func (m Model) applyColResizeDragX(x int) (tea.Model, tea.Cmd) {
	w := m.colResizeStartW + (x - m.colResizeStartX)
	if !m.results.SetColWidth(m.colResizeCol, w) {
		return m, nil
	}
	table := m.results.SourceTable()
	if table == "" {
		return m, nil
	}
	table = m.canonicalTableName(table)
	col := m.results.ColumnName(m.colResizeCol)
	m.setColOverride(table, col, m.results.ColWidth(m.colResizeCol))
	return m, nil
}

// handleTableDesignerMouse routes mouse events to the table designer, which
// replaces the editor/results panels when open. Screen coordinates are
// translated to the designer's content-relative grid (0-based, inside the
// panel's rounded border): content origin is (sidebarWidth+1, 1).
func (m Model) handleTableDesignerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	g := m.workspaceGeom()
	panelH := g.EditorHeight + g.ResultsHeight

	x := msg.X - g.SidebarWidth - 1
	y := msg.Y - 1
	contentW := g.RightWidth - g.BorderOH
	contentH := panelH - g.BorderOH
	// Keep the designer's size/column widths fresh for click mapping. SetSize is
	// otherwise applied only during View (a value-receiver), so the model that
	// handles mouse events would carry stale colWidths/height and the
	// click→cell mapping would drift.
	m.tableDesigner.SetSize(contentW, contentH)
	if x < 0 || x >= contentW || y < 0 || y >= contentH {
		return m, nil
	}

	switch msg.Type {
	case tea.MouseWheelUp:
		m.tableDesigner.Wheel(-1)
		return m, nil
	case tea.MouseWheelDown:
		m.tableDesigner.Wheel(1)
		return m, nil
	case tea.MouseLeft:
		return m, m.tableDesigner.Click(x, y)
	}
	return m, nil
}

// handleSchemaEditorMouse routes mouse events to the structure-view editor,
// which replaces the editor/results panels when open. Screen coordinates are
// translated to the editor's content-relative grid (0-based, inside the
// panel's rounded border): the panel's top-left content cell is at screen
// (sidebarWidth+1, 1).
func (m Model) handleSchemaEditorMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	g := m.workspaceGeom()
	panelH := g.EditorHeight + g.ResultsHeight // matches viewWorkspace's editorH

	// Content-relative coords (inside the border).
	x := msg.X - g.SidebarWidth - 1
	y := msg.Y - 1
	contentW := g.RightWidth - g.BorderOH
	contentH := panelH - g.BorderOH
	// Keep the editor's size/column widths fresh for click mapping (SetSize is
	// otherwise applied only during View, a value-receiver, so the model that
	// handles mouse events would carry stale colWidths/height).
	m.schemaEditor.SetSize(contentW, contentH)
	if x < 0 || x >= contentW || y < 0 || y >= contentH {
		return m, nil // outside the editor's content area
	}

	switch msg.Type {
	case tea.MouseWheelUp:
		m.schemaEditor.Wheel(-1)
		return m, nil
	case tea.MouseWheelDown:
		m.schemaEditor.Wheel(1)
		return m, nil
	case tea.MouseLeft:
		m.schemaEditor.Click(x, y)
		return m, nil
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

	// Help is modal and owns mouse input via handleHelpMouse — never dismiss
	// it from the workspace click-outside path (that path must not run while
	// help is open; if it did, a stray click would wipe a fullscreen overlay).
	if m.help.IsVisible() {
		return false
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

// helpWheelLines is how many lines a mouse-wheel notch scrolls the help overlay.
const helpWheelLines = 3

// handleHelpMouse routes mouse events to the modal help overlay: the wheel
// scrolls the active page; other events (clicks, motion) are ignored so they
// neither dismiss nor navigate it — except left-clicks on the tab bar.
func (m Model) handleHelpMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Prefer Button over deprecated Type so wheel events still scroll when a
	// terminal reports them without a matching Type (or with a release action).
	ev := tea.MouseEvent(msg)
	if ev.IsWheel() {
		switch ev.Button {
		case tea.MouseButtonWheelUp:
			m.help.ScrollBy(-helpWheelLines)
		case tea.MouseButtonWheelDown:
			m.help.ScrollBy(helpWheelLines)
		}
		return m, nil
	}
	switch msg.Type {
	case tea.MouseWheelUp:
		m.help.ScrollBy(-helpWheelLines)
	case tea.MouseWheelDown:
		m.help.ScrollBy(helpWheelLines)
	case tea.MouseLeft:
		// Ignore drag motion; only presses switch tabs.
		if msg.Action == tea.MouseActionMotion {
			return m, nil
		}
		// Click a tab label to switch pages. The panel is centred (panelLeft
		// from the width) and starts at the top row, so the tab bar sits at a
		// fixed screen row.
		if msg.Y == helpTabRow {
			if p := helpTabAt(helpPanelLeft(m.width), msg.X); p >= 0 {
				m.help.SetPage(p)
			}
		}
	}
	return m, nil
}

// handleERDMouse routes mouse events to the ERD panel overlay. The panel
// fills the workspace, so while it's open it intercepts every mouse event —
// this also stops clicks from reaching the workspace panels hidden behind it.
// Coordinate translation (screen → canvas) and hit-testing live on ERDPanel.
//
// The wheel scrolls the diagram (shift+wheel, or a terminal's native horizontal
// wheel, pans sideways). Left-clicks on cards toggle a highlight (or, on a
// non-root card's header — cued by ⤢ — re-focus the ERD on that table's
// neighbourhood), and a double-click browses the table (SELECT *, same as
// Enter). A click-and-drag
// on a card body moves the card freely: the press is recorded as pending, the
// first motion event promotes it to a drag (the card tracks the cursor while
// arrows re-route around it live), and release drops it. A press with no motion
// is still a click, so drag never steals a click. Esc cancels an in-flight drag.
// A mini-map in the bottom-right (when the diagram overflows) steals clicks
// from cards underneath; press and drag on it pan the viewport.
func (m Model) handleERDMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Left-button drag motion is reported as Type=MouseLeft + Action=Motion
	// (bubbletea's backward-compat mapping; Type=MouseMotion is button-less
	// hover, which needs WithMouseAllMotion). Route on Action first, otherwise
	// every drag motion re-enters the MouseLeft press handler and the drag
	// never starts.
	if msg.Action == tea.MouseActionMotion {
		// Button-less motion is hover (Type=MouseMotion); button-held motion is
		// a drag (Type keeps the button, e.g. MouseLeft). Split them here — see
		// docs/tui-mouse.md — so hover drives the tooltip and drag drives the
		// card move, neither disturbing the other.
		if msg.Type == tea.MouseMotion {
			return m.handleERDMouseHover(msg)
		}
		// Mini-map pan-drag: click and drag are the same action, so pan on
		// every motion while the press started on the overlay. An in-flight
		// card drag keeps going even if the cursor crosses the map.
		if m.erdPanel.minimapDrag {
			m.erdPanel = m.erdPanel.minimapPan(msg.X, msg.Y)
			return m, nil
		}
		if m.erdPanel.dragPending == "" && m.erdPanel.dragCard == "" {
			return m, nil
		}
		cx, cy, ok := m.erdPanel.contentToCanvasUnbounded(msg.X, msg.Y)
		if !ok {
			return m, nil
		}
		if m.erdPanel.dragCard == "" {
			var promoted bool
			m.erdPanel, promoted = m.erdPanel.dragPromote(cx, cy)
			if !promoted {
				return m, nil
			}
			m.lastERDClickTime = time.Time{}
			m.lastERDClickCard = ""
		}
		m.erdPanel = m.erdPanel.dragMove(cx, cy)
		return m, nil
	}
	switch msg.Type {
	case tea.MouseWheelUp:
		if msg.Shift {
			m.erdPanel = m.erdPanel.Wheel(0, -1)
		} else {
			m.erdPanel = m.erdPanel.Wheel(-1, 0)
		}
	case tea.MouseWheelDown:
		if msg.Shift {
			m.erdPanel = m.erdPanel.Wheel(0, 1)
		} else {
			m.erdPanel = m.erdPanel.Wheel(1, 0)
		}
	case tea.MouseWheelLeft:
		m.erdPanel = m.erdPanel.Wheel(0, -1)
	case tea.MouseWheelRight:
		m.erdPanel = m.erdPanel.Wheel(0, 1)
	case tea.MouseRelease:
		if m.erdPanel.minimapDrag {
			m.erdPanel.minimapDrag = false
			return m, nil
		}
		// Drop an active drag; or, if a press never promoted, run the deferred
		// click logic (double-click re-centre / single-click highlight).
		if m.erdPanel.dragCard != "" {
			m.erdPanel = m.erdPanel.dragCommit()
			m.snapshotERDPositions()
			return m, nil
		}
		if m.erdPanel.dragPending != "" {
			clicked := m.erdPanel.cardNamed(m.erdPanel.dragPending)
			m.erdPanel.dragPending = ""
			return m.runERDCardClick(clicked)
		}
	case tea.MouseLeft:
		if m.erdPanel.minimapContains(msg.X, msg.Y) {
			m.erdPanel.minimapDrag = true
			m.erdPanel = m.erdPanel.minimapPan(msg.X, msg.Y)
			return m, nil
		}
		cx, cy, ok := m.erdPanel.contentToCanvas(msg.X, msg.Y)
		// A click on a non-root card's header (the whole title row, cued by the
		// ⤢ glyph) takes precedence and re-focuses the ERD on that table's
		// neighbourhood. It acts on press (headers are not draggable).
		if ok {
			if target := m.erdPanel.drillInCard(cx, cy); target != nil {
				m.lastERDClickTime = time.Time{}
				cmd := m.openERD(target.name)
				if root := m.erdPanel.cardNamed(m.erdPanel.layout.focus); root != nil {
					m.erdPanel = m.erdPanel.centerOnCard(root)
				}
				return m, cmd
			}
		}
		var clicked *gcard
		if ok {
			clicked = m.erdPanel.cardAt(cx, cy)
		}
		// Empty space: clear the highlight on press (unchanged).
		if clicked == nil {
			m.lastERDClickTime = time.Time{}
			m.lastERDClickCard = ""
			m.erdPanel = m.erdPanel.toggleHighlight(nil)
			return m, nil
		}
		// Card body: record a pending drag. The click (highlight/recentre) is
		// deferred to release so a drag doesn't toggle highlight mid-move.
		m.erdPanel = m.erdPanel.dragBeginPress(clicked, cx, cy)
	}
	return m, nil
}

// handleERDMouseHover handles button-less mouse motion (hover) over the ERD:
// it sets hoverCard to the table under the cursor ("" over empty space or in
// the Mermaid view) so View overlays its tooltip. The set is throttled by card
// identity — when the cursor stays over the same card, nothing changes and the
// diff renderer skips — and is cleared on any viewport-changing input elsewhere
// (key, wheel, drag). This is the only handler that distinguishes button-less
// motion from drag motion; see docs/tui-mouse.md.
func (m Model) handleERDMouseHover(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	var hovered string
	if !m.erdPanel.merm && !m.erdPanel.minimapContains(msg.X, msg.Y) {
		if cx, cy, ok := m.erdPanel.contentToCanvas(msg.X, msg.Y); ok {
			if c := m.erdPanel.cardAt(cx, cy); c != nil {
				hovered = c.name
			}
		}
	}
	if m.erdPanel.hoverCard != hovered {
		m.erdPanel.hoverCard = hovered
	}
	return m, nil
}

// runERDCardClick runs the deferred body-click logic for an ERD card on mouse
// release: a double-click (same card within doubleClickInterval) browses the
// table (SELECT *, same as Enter); otherwise a single click toggles its
// highlight. Extracted from the MouseRelease path so the press→drag→release
// flow can call it for a click that never promoted to a drag. clicked may be
// nil (then nothing fires — the empty-space clear already happened on press).
func (m Model) runERDCardClick(clicked *gcard) (Model, tea.Cmd) {
	if clicked == nil {
		return m, nil
	}
	now := time.Now()
	if !m.lastERDClickTime.IsZero() &&
		now.Sub(m.lastERDClickTime) <= doubleClickInterval &&
		m.lastERDClickCard == clicked.name {
		m.lastERDClickTime = time.Time{}
		m.erdPanel = m.erdPanel.setFocus(clicked.name)
		return m.erdEnter()
	}
	// Single-click → toggle highlight.
	m.lastERDClickTime = now
	m.lastERDClickCard = clicked.name
	m.erdPanel = m.erdPanel.toggleHighlight(clicked)
	return m, nil
}
