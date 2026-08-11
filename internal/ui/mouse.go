package ui

import (
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
		return m, nil
	}

	// ── Editor↔results split drag ─────────────────────────────
	// Action-first (same shape as ERD drag): motion while a button is held
	// arrives as Type=MouseLeft + Action=Motion, so a Type switch would
	// re-enter the press path. See docs/tui-mouse.md.
	if m.splitDragging {
		return m.handleSplitDrag(msg)
	}

	g := m.workspaceGeom()
	sidebarWidth := g.SidebarWidth
	editorRight := g.EditorRight
	resultsTop := g.ResultsTop
	resultsBottom := g.ResultsBottom

	// Press on the editor/results seam starts a resize drag. Schema editor
	// and table designer replace the split, so they skip this.
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
				// If the inspector is open, a double-click on a grid cell
				// signals intent to edit the cell directly: close the
				// inspector first. Leave it alone when it's mid-edit or
				// mid-insert so in-progress work isn't discarded.
				if m.inspector.IsVisible() && !m.inspector.IsEditing() && !m.inspector.IsInserting() {
					m.inspector.Hide()
					if m.focus == FocusInspector {
						m.focus = FocusResults
					}
					m.layoutWorkspace()
					m.applyFocus()
				}
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
	if x < g.SidebarWidth || x >= g.EditorRight {
		return false
	}
	return y == g.ResultsTop-1 || y == g.ResultsTop
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

// helpWheelLines is how many lines a mouse-wheel notch scrolls the help overlay.
const helpWheelLines = 3

// handleHelpMouse routes mouse events to the modal help overlay: the wheel
// scrolls the active page; other events (clicks, motion) are ignored so they
// neither dismiss nor navigate it.
func (m Model) handleHelpMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.MouseWheelUp:
		m.help.ScrollBy(-helpWheelLines)
	case tea.MouseWheelDown:
		m.help.ScrollBy(helpWheelLines)
	case tea.MouseLeft:
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
// neighbourhood), and a double-click recentres the viewport. A click-and-drag
// on a card body moves the card freely: the press is recorded as pending, the
// first motion event promotes it to a drag (the card tracks the cursor while
// arrows re-route around it live), and release drops it. A press with no motion
// is still a click, so drag never steals a click. Esc cancels an in-flight drag.
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
		// Drop an active drag; or, if a press never promoted, run the deferred
		// click logic (double-click re-centre / single-click highlight).
		if m.erdPanel.dragCard != "" {
			m.erdPanel = m.erdPanel.dragCommit()
			return m, nil
		}
		if m.erdPanel.dragPending != "" {
			clicked := m.erdPanel.cardNamed(m.erdPanel.dragPending)
			m.erdPanel.dragPending = ""
			m = m.runERDCardClick(clicked)
		}
	case tea.MouseLeft:
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
	if !m.erdPanel.merm {
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
// release: a double-click (same card within doubleClickInterval) recentres the
// viewport on it; otherwise a single click toggles its highlight. Extracted
// from the MouseRelease path so the press→drag→release flow can call it for a
// click that never promoted to a drag. clicked may be nil (then nothing fires —
// the empty-space clear already happened on press).
func (m Model) runERDCardClick(clicked *gcard) Model {
	if clicked == nil {
		return m
	}
	now := time.Now()
	if !m.lastERDClickTime.IsZero() &&
		now.Sub(m.lastERDClickTime) <= doubleClickInterval &&
		m.lastERDClickCard == clicked.name {
		// Double-click on a card → re-centre the viewport on it (and move the
		// keyboard focus to it, matching a single click).
		m.lastERDClickTime = time.Time{}
		m.erdPanel = m.erdPanel.setFocus(clicked.name)
		m.erdPanel = m.erdPanel.centerOnCard(clicked)
		return m
	}
	// Single-click → toggle highlight.
	m.lastERDClickTime = now
	m.lastERDClickCard = clicked.name
	m.erdPanel = m.erdPanel.toggleHighlight(clicked)
	return m
}
