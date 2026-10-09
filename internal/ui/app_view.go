package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// refreshCompletionCandidates rebuilds the editor's candidate list from
// keywords, schemas, tables (active + cached cross-schema), and columns.
func (m *Model) refreshCompletionCandidates() {
	var candidates []completionItem

	for _, kw := range sqlKeywords {
		candidates = append(candidates, completionItem{text: kw, kind: kindKeyword})
	}
	for _, s := range m.schemaNames {
		candidates = append(candidates, completionItem{text: s, kind: kindSchema})
	}
	for _, t := range m.tables {
		candidates = append(candidates, completionItem{text: t, kind: kindTable})
	}
	for schema, tables := range m.schemaTableCache {
		for _, t := range tables {
			candidates = append(candidates, completionItem{text: t, kind: kindTable, schema: schema})
		}
	}

	for table, cols := range m.columnCache {
		for _, c := range cols {
			candidates = append(candidates, completionItem{text: c.Name, kind: kindColumn, table: table})
		}
	}

	m.editor.SetActiveSchema(m.currentSchemaName())
	m.editor.SetCandidates(candidates)
}

// View renders the entire application.
func (m Model) View() string {
	if m.viewCached && m.viewBuf != nil && *m.viewBuf != "" {
		return *m.viewBuf
	}
	s := m.buildView()
	if m.viewBuf != nil {
		*m.viewBuf = s
	}
	return s
}

func (m Model) buildView() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return "Loading..."
	}

	if m.state == stateAddConnection {
		return m.paintBg(m.viewAddConnection())
	}

	if m.state == stateConnections {
		return m.paintBg(m.viewConnections())
	}

	// Database picker: same shell as the connection list / form.
	if m.dbPicker.IsVisible() {
		pw, ph := popupOuterSize(m.height)
		m.dbPicker.SetSize(pw, ph)
		pickerPanel := m.dbPicker.View()
		view := lipgloss.Place(m.width, m.height-1,
			lipgloss.Center, lipgloss.Center,
			pickerPanel,
			canvasPlaceOptions(m.canvasBackground())...)

		// Overlay create-database dialog on top of the picker if active.
		if m.createDBActive {
			dialog := renderInputDialogBare("Create new database", m.createDBInput, m.createDBErr)
			dw := lipgloss.Width(dialog)
			dh := lipgloss.Height(dialog)
			view = placeOverlay(view, dialog, (m.width-dw)/2, (m.height-1-dh)/2)
		}

		// Overlay drop-database confirmation on top of the picker if active.
		if m.dropDBConfirm != "" {
			dialog := renderTypedConfirmDialogBare(
				"Drop database "+m.dropDBConfirm+"?",
				m.dropDBConfirm,
				m.dropDBInput,
			)
			view = placeOverlay(view, dialog, (m.width-lipgloss.Width(dialog))/2, (m.height-1-lipgloss.Height(dialog))/2)
		}

		// Append status bar.
		connName := ""
		if m.connection != nil {
			connName = m.connection.Config().Name
		}
		statusBar := lipgloss.NewStyle().
			Width(m.width).
			Height(1).
			Foreground(colorMuted).
			Background(colorStatusBarBg).
			Render(" " + m.statusBar(connName))
		return m.paintBg(lipgloss.JoinVertical(lipgloss.Left, view, statusBar))
	}

	return m.paintBg(m.viewWorkspace())
}

// connFormPopupDims returns the screen bounds of the add/edit connection form
// popup, matching viewAddConnection's centering and dynamic height.
func (m Model) connFormPopupDims() (panelW, panelH, panelX, panelY int) {
	popupW, _ := popupDim()
	const borderOverhead = 2
	innerW, capH := popupContentSize(m.height)
	m.connForm.SetSize(innerW, capH)
	contentH := m.connForm.effectiveHeight()
	popupH := contentH + borderOverhead
	panelW = popupW
	panelH = popupH
	panelX = (m.width - panelW) / 2
	panelY = (m.height - 1 - panelH) / 2
	return panelW, panelH, panelX, panelY
}

// connListPopupDims returns the (width, height) of the connection-list popup.
// The footprint matches the connection form and database picker (see
// popupOuterSize) so transitions between those screens stay calm. The list
// scrolls internally when there are more connections than fit.
func (m Model) connListPopupDims() (w, h int) {
	return popupOuterSize(m.height)
}

// connListContentDims returns the content width and list-area height the
// connection list should be sized to, derived from connListPopupDims. Shared
// by layout.go (which sizes the real model) and viewConnections (render) so the
// scroll math and what is drawn always agree.
func (m Model) connListContentDims() (contentW, listH int) {
	pw, ph := m.connListPopupDims()
	panelW := pw - 2   // border
	panelH := ph - 2   // border
	listH = panelH - 2 // prompt + scroll-info (chrome)
	if listH < linesPerField {
		listH = linesPerField
	}
	contentW = panelW - 2 // Padding(0,1) → 2 cols
	return contentW, listH
}

func (m Model) viewConnections() string {
	popupW, popupH := m.connListPopupDims()
	borderOverhead := 2

	panelW := popupW - borderOverhead
	panelH := popupH - borderOverhead

	prompt := m.connList.Prompt()
	contentW, listH := m.connListContentDims()
	m.connList.SetSize(contentW, listH)
	m.connList.SetPadBackground(m.canvasBackground())

	listStyled := m.connList.View()
	// Tabs sit above the filter prompt so the fuzzy line stays next to the list.
	parts := make([]string, 0, 4)
	if tabs := m.connList.GroupTabBar(); tabs != "" {
		parts = append(parts, tabs)
	}
	parts = append(parts, prompt, listStyled, m.connList.ScrollInfo())

	panelStyle := lipgloss.NewStyle().
		Width(panelW).
		Height(panelH).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1)
	if bg := m.canvasBackground(); string(bg) != "" {
		panelStyle = panelStyle.Background(bg)
	}
	connPanel := panelStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left, parts...),
	)

	// Space-filled canvas so paintBg covers the whole screen behind the popup.
	view := lipgloss.Place(m.width, m.height-1,
		lipgloss.Center, lipgloss.Center,
		connPanel,
		canvasPlaceOptions(m.canvasBackground())...)

	// Overlay help panel if visible (sized to leave the status bar showing).
	if m.help.IsVisible() {
		m.help.SetSize(m.width, m.height-1)
		view = m.help.View()
	}

	// Overlay connection-deletion confirmation if pending.
	if m.deleteConnConfirm != "" {
		prompt := fmt.Sprintf("Delete connection %s?\nIts keychain secrets are also removed.", m.deleteConnConfirm)
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Append status bar.
	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(1).
		Foreground(colorMuted).
		Background(colorStatusBarBg).
		Render(" " + m.statusBar(""))
	return lipgloss.JoinVertical(lipgloss.Left, view, statusBar)
}

func (m Model) viewAddConnection() string {
	// Sizing is shared with layout.go via popupContentSize so the scroll model
	// and rendering agree. The form uses the fixed shell height (6-field
	// footprint) so it matches the connection list and database picker.
	popupW, _ := popupDim() // width stays fixed
	borderOverhead := 2
	innerW, capH := popupContentSize(m.height)
	m.connForm.SetSize(innerW, capH)
	contentH := m.connForm.effectiveHeight()
	popupH := contentH + borderOverhead

	formPanel := lipgloss.NewStyle().
		Width(popupW-borderOverhead).
		Height(popupH-borderOverhead).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(m.connForm.View())

	panelW := lipgloss.Width(formPanel)
	panelH := lipgloss.Height(formPanel)
	panelX := (m.width - panelW) / 2
	panelY := (m.height - 1 - panelH) / 2

	// Center the popup in the area above the status bar, then append the
	// status bar so the keybinding hints (enter / ctrl+t / esc) are visible.
	placed := lipgloss.Place(m.width, m.height-1,
		lipgloss.Center, lipgloss.Center,
		formPanel,
		lipgloss.WithWhitespaceChars(" "))

	if comp := m.connForm.CompletionView(); comp != "" {
		if row := m.connForm.completionLineOffset(); row >= 0 {
			placed = placeOverlay(placed, comp, panelX+4, panelY+1+row)
		}
	}
	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(1).
		Foreground(colorMuted).
		Background(colorStatusBarBg).
		Render(" " + m.statusBar(""))
	return lipgloss.JoinVertical(lipgloss.Left, placed, statusBar)
}

func (m Model) viewWorkspace() string {
	g := m.workspaceGeom()
	sidebarWidth := g.SidebarWidth
	slotWidth := g.RightSlotW
	statusHeight := g.StatusH
	borderOverhead := g.BorderOH
	editorHeight := g.EditorHeight
	resultsHeight := g.ResultsHeight
	rightWidth := g.RightWidth

	// Build the content area (tabs are inside the editor panel).
	var contentPanel string
	if m.tableDesigner.IsVisible() {
		designerHeight := editorHeight + resultsHeight
		m.tableDesigner.SetSize(rightWidth-borderOverhead, designerHeight-borderOverhead)
		contentPanel = lipgloss.NewStyle().
			Width(rightWidth).
			Height(designerHeight).
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Render(m.tableDesigner.View())
	} else if m.schemaEditor.IsVisible() {
		editorH := editorHeight + resultsHeight
		m.schemaEditor.SetSize(rightWidth-borderOverhead, editorH-borderOverhead)
		contentPanel = lipgloss.NewStyle().
			Width(rightWidth).
			Height(editorH).
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Render(m.schemaEditor.View())
	} else {
		var resultsPanel string
		if m.chartPanel.IsVisible() {
			resultsPanel = m.chartPanel.View()
		} else if m.queryRunning && !m.backendSearching {
			// Show an animated spinner while the query executes.
			frame := spinnerFrames[m.querySpinner%len(spinnerFrames)]
			elapsed := time.Since(m.queryStart).Round(time.Millisecond)
			content := lipgloss.NewStyle().Foreground(colorPrimary).Render(frame) +
				"  " + mutedStyle.Render(fmt.Sprintf("running query… %s", elapsed)) +
				"  " + lipgloss.NewStyle().Foreground(colorMuted).Render(m.cancelHint())
			resultsPanel = lipgloss.NewStyle().
				Width(rightWidth).
				Height(resultsHeight).
				Border(panelBorder()).
				BorderForeground(m.borderForFocus(FocusResults)).
				Align(lipgloss.Center, lipgloss.Center).
				Render(content)
		} else {
			// When the table has results it draws its own border, merging
			// seamlessly with the panel frame. Otherwise fall back to a
			// standard rounded border around the placeholder message.
			hasTable := m.results.HasResult() && m.results.NumCols() > 0
			m.results.SetBorderColor(m.borderForFocus(FocusResults))

			var resultsStyle lipgloss.Style
			if hasTable {
				resultsStyle = lipgloss.NewStyle().
					Width(rightWidth + borderOverhead).
					Height(resultsHeight + borderOverhead)
			} else {
				resultsStyle = lipgloss.NewStyle().
					Width(rightWidth).
					Height(resultsHeight).
					Border(panelBorder()).
					BorderForeground(m.borderForFocus(FocusResults))
			}
			resultsPanel = resultsStyle.Render(func() string {
				m.results.SetSort(m.sortCol, m.sortDir)
				// The prompt no longer lives inside this panel; the table fills
				// it at full height. CmdHeight in workspaceGeom already reserved
				// the row the bottom command line occupies.
				m.results.SetSize(rightWidth+borderOverhead, resultsHeight+borderOverhead)
				return m.results.View()
			}())
		}

		if m.editorVisible {
			editorPanel := lipgloss.NewStyle().
				Width(rightWidth).
				Height(editorHeight - borderOverhead).
				Border(panelBorder()).
				BorderForeground(m.borderForFocus(FocusEditor)).
				Render(lipgloss.JoinVertical(lipgloss.Left,
					m.tabBar.View(),
					lipgloss.NewStyle().Foreground(colorBorder).
						Render(strings.Repeat("─", rightWidth)),
					m.editor.View(),
				))
			contentPanel = lipgloss.JoinVertical(lipgloss.Left,
				editorPanel,
				resultsPanel,
			)
		} else {
			contentPanel = resultsPanel
		}
	}

	rightPanel := contentPanel

	// Build the right-hand slot panel: the inspector, assistant, and docked
	// relationship explorer are mutually exclusive, so at most one is rendered.
	var slotPanel string
	if m.inspector.IsVisible() {
		slotContentHeight := lipgloss.Height(rightPanel) - borderOverhead
		if slotContentHeight < 3 {
			slotContentHeight = 3
		}
		m.inspector.SetSize(slotWidth-borderOverhead, slotContentHeight)
		slotPanel = lipgloss.NewStyle().
			Width(slotWidth - borderOverhead).
			Height(slotContentHeight).
			Border(panelBorder()).
			BorderForeground(m.borderForFocus(FocusInspector)).
			Render(m.inspector.View(m.inspectorResults()))
	} else if m.assistant.IsVisible() {
		slotContentHeight := lipgloss.Height(rightPanel) - borderOverhead
		if slotContentHeight < 3 {
			slotContentHeight = 3
		}
		m.assistant.SetSize(slotWidth-borderOverhead, slotContentHeight)
		m.assistant.spinner = m.querySpinner // keep the pending spinner in sync
		m.assistant.SetModel(m.effectiveAIModel())
		slotPanel = lipgloss.NewStyle().
			Width(slotWidth - borderOverhead).
			Height(slotContentHeight).
			Border(panelBorder()).
			BorderForeground(m.borderForFocus(FocusAssistant)).
			Render(m.assistant.View())
	} else if m.explorer.IsVisible() && m.explorer.docked {
		// The explorer's View() already draws its own border, so place it
		// directly (no second border) sized to the full slot. Its border color
		// mirrors focus, like the inspector/assistant.
		slotH := lipgloss.Height(rightPanel)
		m.explorer.focused = m.focus == FocusExplorer
		m.explorer.SetSize(slotWidth, slotH)
		slotPanel = m.explorer.View()
	}

	// Sidebar content height = right panel height minus sidebar's own borders.
	sidebarContentHeight := lipgloss.Height(rightPanel) - borderOverhead
	if sidebarContentHeight < 3 {
		sidebarContentHeight = 3
	}

	// Reserve 1 line for bottom bar (search/scroll info).
	tableAreaHeight := sidebarContentHeight - 1
	if tableAreaHeight < 1 {
		tableAreaHeight = 1
	}
	maxVisible := tableAreaHeight

	items := m.sidebarItems()

	// Scroll window: cursor-centered for keyboard nav, frozen when the view was
	// anchored by a mouse click. The shared helper is also used by the mouse
	// handler so a click always maps to the rendered item.
	start := m.sidebarRenderedStart()
	end := start + maxVisible
	if end > len(items) {
		end = len(items)
	}
	m.sidebarScroll = start

	sidebarContentWidth := sidebarWidth - borderOverhead

	tableList := strings.Builder{}
	for i := start; i < end; i++ {
		item := items[i]
		isCursor := m.focus == FocusConnections && i == m.sidebarCursor

		var line string
		// Tables under a schema header are nested two spaces; columns must
		// use the same nest so they stay aligned with the table name (after
		// the expand glyph), not with the glyph itself.
		nest := ""
		if item.schema != "" {
			nest = "  "
		}
		if item.isColumn {
			indent := nest + "   " // icon (1) + space + name-start
			colStyle := lipgloss.NewStyle().Foreground(colorLabel)
			if isCursor {
				colStyle = lipgloss.NewStyle().Foreground(colorBg).Background(colorPrimary).Bold(true)
			}
			colName := colStyle.Render(item.text)
			colType := mutedStyle.Render(item.colType)
			line = indent + colName + " " + colType
		} else if item.isSchema {
			style := mutedStyle
			expandIcon := icons.collapsed
			if m.isSchemaSectionExpanded(item.text) {
				expandIcon = icons.expanded
			}
			if isCursor {
				style = selectedStyle
			}
			label := item.text
			if item.text == m.currentSchemaName() {
				label = "* " + item.text
			}
			line = style.Render(expandIcon + " " + label)
		} else {
			style := normalStyle
			expandIcon := icons.collapsed
			expandKey := item.text
			if item.schema != "" && item.schema != m.currentSchemaName() {
				expandKey = item.schema + "." + item.text
			}
			if _, ok := m.expanded[expandKey]; ok {
				expandIcon = icons.expanded
			}
			if isCursor && !m.sidebarFiltering {
				style = selectedStyle
			}
			tableName := item.text
			if isCursor && m.sidebarFiltering {
				line = selectedStyle.Render(nest + expandIcon + " " + item.text)
			} else {
				if m.sidebarFiltering {
					tableName = highlightMatches(item.text, item.matchIdx)
				}
				line = style.Render(nest + expandIcon + " " + tableName)
			}
			if item.isView {
				line += " " + mutedStyle.Render("view")
			}
		}

		// Truncate to sidebar width — strip ANSI codes for measurement,
		// then truncate the rendered string.
		line = truncateSidebarLine(line, sidebarContentWidth)
		tableList.WriteString(line)
		tableList.WriteString("\n")
	}
	if len(items) == 0 {
		if m.sidebarFiltering {
			tableList.WriteString(mutedStyle.Render("  (no matches)"))
		} else {
			tableList.WriteString(mutedStyle.Render("  (no tables)"))
		}
	}

	scrollInfo := ""
	if m.sidebarFiltering {
		scrollInfo = renderPalettePrompt(m.sidebarFilter, true)
	} else if len(items) > maxVisible {
		scrollInfo = mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", start+1, end, len(items)))
	}

	tableListStyled := lipgloss.NewStyle().
		Height(tableAreaHeight).
		Render(strings.TrimRight(tableList.String(), "\n"))

	sidebar := lipgloss.NewStyle().
		Width(sidebarWidth - borderOverhead).
		Height(sidebarContentHeight).
		Border(panelBorder()).
		BorderForeground(m.borderForFocus(FocusConnections)).
		Render(
			lipgloss.JoinVertical(lipgloss.Left, tableListStyled, scrollInfo),
		)

	connName := ""
	if m.connection != nil {
		connName = m.connection.Config().Name
	}

	statusBar := lipgloss.NewStyle().
		Width(m.width).
		Height(statusHeight).
		Foreground(colorMuted).
		Background(colorStatusBarBg).
		Render(" " + m.statusBar(connName))

	var workspace string
	if m.inspector.IsVisible() || m.assistant.IsVisible() || (m.explorer.IsVisible() && m.explorer.docked) {
		if m.sidebarVisible && sidebarWidth > 0 {
			workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel, slotPanel)
		} else {
			workspace = lipgloss.JoinHorizontal(lipgloss.Top, rightPanel, slotPanel)
		}
	} else if m.sidebarVisible && sidebarWidth > 0 {
		workspace = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPanel)
	} else {
		workspace = rightPanel
	}

	// Dim the workspace panels behind long-lived editing overlays.
	// The status bar is kept undimmed so hints remain clearly visible.
	if m.cellEdit.IsVisible() || m.history.IsVisible() || m.bookmarks.IsVisible() || m.crossSearch.IsVisible() || m.explainPanel.IsVisible() || m.diffPanel.IsVisible() || m.lookupPanel.IsVisible() {
		workspace = dimBackground(workspace)
	}

	// Stack workspace, status bar, and the bottom command line. The command
	// line is omitted entirely when no prompt is active, so it adds no height
	// at rest (cmdHeight is 0 then, keeping the layout identical to before).
	layers := []string{workspace, statusBar}
	if cmd := m.commandLine(); cmd != "" {
		layers = append(layers, cmd)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, layers...)

	// Overlay history panel if visible
	if m.history.IsVisible() {
		m.history.SetSize(m.width*65/100, (m.height-1)*65/100)
		histPanel := m.history.View()
		panelW := lipgloss.Width(histPanel)
		panelH := lipgloss.Height(histPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, histPanel, panelX, panelY)
	}

	// Overlay bookmarks panel if visible
	if m.bookmarks.IsVisible() {
		m.bookmarks.SetSize(m.width*65/100, (m.height-1)*65/100)
		bmPanel := m.bookmarks.View()
		panelW := lipgloss.Width(bmPanel)
		panelH := lipgloss.Height(bmPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, bmPanel, panelX, panelY)
	}

	// Overlay cross-search panel if visible
	if m.crossSearch.IsVisible() {
		m.crossSearch.SetSize(m.width*65/100, (m.height-1)*65/100)
		csPanel := m.crossSearch.View()
		panelW := lipgloss.Width(csPanel)
		panelH := lipgloss.Height(csPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, csPanel, panelX, panelY)
	}

	// Overlay explain panel if visible
	if m.explainPanel.IsVisible() {
		m.explainPanel.SetSize(m.width*70/100, (m.height-1)*70/100)
		explainPanelView := m.explainPanel.View()
		panelW := lipgloss.Width(explainPanelView)
		panelH := lipgloss.Height(explainPanelView)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, explainPanelView, panelX, panelY)
	}

	// Overlay result-set diff panel if visible
	if m.diffPanel.IsVisible() {
		m.diffPanel.SetSize(m.width*80/100, (m.height-1)*75/100)
		diffPanelView := m.diffPanel.View()
		panelW := lipgloss.Width(diffPanelView)
		panelH := lipgloss.Height(diffPanelView)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, diffPanelView, panelX, panelY)
	}

	// Overlay lookup panel if visible
	if m.lookupPanel.IsVisible() {
		m.lookupPanel.SetSize(m.width*70/100, (m.height-1)*70/100)
		lookupPanelView := m.lookupPanel.View()
		panelW := lipgloss.Width(lookupPanelView)
		panelH := lipgloss.Height(lookupPanelView)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, lookupPanelView, panelX, panelY)
	}

	// Overlay ERD panel if visible — fills the whole workspace area (above the
	// status line), edge to edge, with no frame so the diagram gets maximum room.
	if m.erdPanel.IsVisible() {
		m.erdPanel.SetSize(m.width, m.height-1)
		view = placeOverlay(view, m.erdPanel.View(), 0, 0)
	}

	// Overlay filter picker if visible
	if m.filterPicker.IsVisible() {
		pw, ph := popupDim()
		m.filterPicker.SetSize(pw, ph)
		filterPanel := m.filterPicker.View()
		panelW := lipgloss.Width(filterPanel)
		panelH := lipgloss.Height(filterPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, filterPanel, panelX, panelY)
	}

	// Overlay column-visibility picker if visible
	if m.columnPicker.IsVisible() {
		pw, ph := popupDim()
		m.columnPicker.SetSize(pw, ph)
		colPanel := m.columnPicker.View()
		panelW := lipgloss.Width(colPanel)
		panelH := lipgloss.Height(colPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, colPanel, panelX, panelY)
	}

	// Overlay discard confirmation dialog if visible
	if m.discardConfirm {
		dialog := renderConfirmDialogBare("Discard all unsaved changes?")
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay truncate confirmation dialog if visible
	if m.truncateConfirm != "" {
		prompt := fmt.Sprintf("Truncate table %s?\nAll rows will be permanently deleted.", m.truncateConfirm)
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay kill-session confirmation dialog if visible
	if m.killConfirm != "" {
		prompt := fmt.Sprintf("Kill session %s?\nActive queries on that connection will be aborted.", m.killConfirm)
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay EXPLAIN ANALYZE confirmation — the statement will actually run.
	if m.explainAnalyzeConfirm {
		prompt := "EXPLAIN ANALYZE runs the statement (including any writes).\nContinue?"
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay drop-table typed confirmation dialog if visible.
	if m.dropTableConfirm != "" {
		prompt := fmt.Sprintf("Drop table %s?\nThis permanently deletes the table, data, and indexes.", m.dropTableConfirm)
		dialog := renderTypedConfirmDialog(prompt, m.dropTableConfirm, m.dropTableInput, 52, 0)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay drop-database typed confirmation dialog if visible. Sized to
	// match the database picker so it replaces it cleanly, not a smaller box
	// floating on top.
	if m.dropDBConfirm != "" {
		prompt := fmt.Sprintf("Drop database %s?\nThis permanently deletes every table and all data in the database.", m.dropDBConfirm)
		pw, ph := popupDim()
		dialog := renderTypedConfirmDialog(prompt, m.dropDBConfirm, m.dropDBInput, pw, ph)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay create-database name input dialog if visible.
	if m.createDBActive {
		pw, ph := popupDim()
		dialog := renderInputDialog("Create new database", m.createDBInput, m.createDBErr, pw, ph)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay row deletion confirmation dialog if visible.
	if m.deleteRowsConfirmTable != "" {
		prompt := fmt.Sprintf("Delete %d row%s from %s?\nThis cannot be undone.", m.deleteRowsConfirmCount, pluralIf(m.deleteRowsConfirmCount != 1, "s"), m.deleteRowsConfirmTable)
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	if m.schemaConfirmSQL != "" {
		prompt := fmt.Sprintf("Drop column on %s?\nThis permanently removes the column and its data.", m.schemaConfirmTable)
		dialog := renderSQLConfirmDialog(prompt, m.schemaConfirmSQL)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay clear-history confirmation dialog if visible.
	if m.clearHistoryConfirm {
		dialog := renderConfirmDialogBare("Clear all query history?\nThis cannot be undone.")
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay clear-bookmarks confirmation dialog if visible.
	if m.clearBookmarksConfirm {
		dialog := renderConfirmDialogBare("Clear all bookmarks?\nThis cannot be undone.")
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay add-column form.
	if m.addColumnForm.IsVisible() {
		popupW := 58
		borderOverhead := 2
		padding := 4
		innerW := popupW - borderOverhead - padding
		m.addColumnForm.SetMaxWidth(innerW)
		formPanel := lipgloss.NewStyle().
			Width(popupW-borderOverhead).
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Render(m.addColumnForm.View())
		panelW := lipgloss.Width(formPanel)
		panelH := lipgloss.Height(formPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, formPanel, panelX, panelY)
	}

	// Overlay table rename form.
	if m.tableRenameForm.IsVisible() {
		popupW := 58
		borderOverhead := 2
		padding := 4
		innerW := popupW - borderOverhead - padding
		m.tableRenameForm.SetMaxWidth(innerW)
		content := m.tableRenameForm.View()
		formPanel := lipgloss.NewStyle().
			Width(popupW-borderOverhead).
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Render(content)
		panelW := lipgloss.Width(formPanel)
		panelH := lipgloss.Height(formPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, formPanel, panelX, panelY)
	}

	// Overlay cell-edit popup (expanded editor for truncated cells).
	if m.cellEdit.IsVisible() {
		// Size the popup to ~65% of the screen, matching history/bookmarks.
		availW := m.width * 65 / 100
		availH := (m.height - 1) * 65 / 100
		// Subtract the cell editor's fixed overhead: label (1) + inner border
		// top/bottom (2) + outer rounded border top/bottom (2) = 5 rows.
		m.cellEdit.SetMaxSize(availW, availH-5)
		panel := lipgloss.NewStyle().
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Render(m.cellEdit.View())
		panelW := lipgloss.Width(panel)
		panelH := lipgloss.Height(panel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, panel, panelX, panelY)
	}

	// Overlay completion popup if visible. Anchor below the cursor, then clamp
	// into the workspace (above status/cmd, left of the right slot) so a long
	// list or a cursor near the edge does not clip off-screen.
	if m.editor.CompletionVisible() {
		cursorLine, cursorCol := m.editor.CursorScreenPos()
		popup := m.editor.CompletionView()
		popupW := lipgloss.Width(popup)
		popupH := lipgloss.Height(popup)
		// Editor panel: top border (1) + tab bar (1) + separator (1) → content.
		const editorContentTop = 1 + 1 + 1
		cursorTop := editorContentTop + cursorLine
		popupX := sidebarWidth + 2 + cursorCol
		popupY := cursorTop + 1 // one row below the cursor line
		maxW := g.EditorRight
		if maxW <= 0 {
			maxW = m.width
		}
		maxH := m.height - statusHeight - g.CmdHeight
		popupX, popupY = fitCompletionPopup(popupX, popupY, cursorTop, popupW, popupH, maxW, maxH)
		view = placeOverlay(view, popup, popupX, popupY)
	}

	// Overlay export picker if visible
	if m.exportPicker.IsVisible() {
		pw, ph := popupDim()
		m.exportPicker.SetSize(pw, ph)
		exportPanel := m.exportPicker.View()
		panelW := lipgloss.Width(exportPanel)
		panelH := lipgloss.Height(exportPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, exportPanel, panelX, panelY)
	}

	// Overlay backup picker (:backup) if visible — wider for size columns.
	if m.backupPicker.IsVisible() {
		pw, ph := backupPickerDim(m.width, m.height)
		m.backupPicker.SetSize(pw, ph)
		backupPanel := m.backupPicker.View()
		panelW := lipgloss.Width(backupPanel)
		panelH := lipgloss.Height(backupPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, backupPanel, panelX, panelY)
	}

	// Overlay export dialog (g X) if visible
	if m.exportOverlay.IsVisible() {
		pw, ph := exportOverlayDim(m.width, m.height)
		m.exportOverlay.SetSize(pw, ph)
		exportPanel := m.exportOverlay.View()
		panelW := lipgloss.Width(exportPanel)
		panelH := lipgloss.Height(exportPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, exportPanel, panelX, panelY)
	}

	// Overlay theme picker (g c) if visible
	if m.themePicker.IsVisible() {
		themePanel := m.themePicker.View()
		panelW := lipgloss.Width(themePanel)
		panelH := lipgloss.Height(themePanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, themePanel, panelX, panelY)
	}

	// Overlay provider picker (M) if visible
	if m.providerPicker.IsVisible() {
		providerPanel := m.providerPicker.View()
		panelW := lipgloss.Width(providerPanel)
		panelH := lipgloss.Height(providerPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, providerPanel, panelX, panelY)
	}

	// Overlay provider-deletion confirmation (stacked over the picker) if pending.
	if m.deleteProviderConfirm != "" {
		prompt := fmt.Sprintf("Delete provider %s?\nIts keychain API key is also removed.", m.deleteProviderConfirm)
		dialog := renderConfirmDialogBare(prompt)
		dlgW := lipgloss.Width(dialog)
		dlgH := lipgloss.Height(dialog)
		dlgX := (m.width - dlgW) / 2
		dlgY := (m.height - 1 - dlgH) / 2
		view = placeOverlay(view, dialog, dlgX, dlgY)
	}

	// Overlay model browser (m) if visible
	if m.modelBrowser.IsVisible() {
		modelPanel := m.modelBrowser.View()
		panelW := lipgloss.Width(modelPanel)
		panelH := lipgloss.Height(modelPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, modelPanel, panelX, panelY)
	}

	// Overlay provider form (n/e from the `M` picker) if visible. Sized like
	// the connection form popup (fixed popupDim width, content-tall height)
	// so it renders identically to the other bordered-field form.
	if m.providerForm.IsVisible() {
		popupW, _ := popupDim()
		borderOverhead := 2
		innerW, _ := popupContentSize(m.height)
		m.providerForm.SetSize(innerW)
		formPanel := lipgloss.NewStyle().
			Width(popupW-borderOverhead).
			Height(m.providerForm.effectiveHeight()).
			Border(panelBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1).
			Render(m.providerForm.View())
		panelW := lipgloss.Width(formPanel)
		panelH := lipgloss.Height(formPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, formPanel, panelX, panelY)
	}

	// Overlay import prompt if visible
	if m.importPrompt.IsVisible() {
		importPanel := m.importPrompt.View()
		panelW := lipgloss.Width(importPanel)
		panelH := lipgloss.Height(importPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, importPanel, panelX, panelY)

		// Overlay completion dropdown as a floating popup below the input line.
		// The input line is the 5th visual row (0-indexed: border, padding,
		// title, blank, input = row 4), so the dropdown starts at row 5.
		if comp := m.importPrompt.CompletionView(); comp != "" {
			view = placeOverlay(view, comp, panelX+9, panelY+5)
		}
	}

	// Overlay command palette if visible
	if m.palette.IsVisible() {
		pw, ph := palettePopupDim()
		palPanel := m.palette.View(pw, ph)
		panelW := lipgloss.Width(palPanel)
		panelH := lipgloss.Height(palPanel)
		panelX := (m.width - panelW) / 2
		panelY := (m.height - 1 - panelH) / 2
		view = placeOverlay(view, palPanel, panelX, panelY)
	}

	// Overlay ":" verb-completion popup directly above the command line.
	if m.ex.visible {
		if popup := m.ex.completionView(m.width); popup != "" {
			ph := lipgloss.Height(popup)
			view = placeOverlay(view, popup, 1, m.height-1-ph)
		}
	}

	// Overlay help panel if visible, leaving the status bar visible below it
	// (help fills the top height-1 rows; the status bar is the last row).
	if m.help.IsVisible() {
		m.help.SetSize(m.width, m.height-1)
		view = placeOverlay(view, m.help.View(), 0, 0)
	}

	return view
}
