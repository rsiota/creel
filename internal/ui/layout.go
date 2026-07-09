package ui

func (m Model) cycleFocus() Model {
	m.focus++
	if m.focus > FocusInspector {
		m.focus = FocusConnections
	}
	// Skip invisible panels
	if m.focus == FocusInspector && !m.inspector.IsVisible() {
		m.focus = FocusConnections
	}
	m.applyFocus()
	return m
}

func (m Model) cycleFocusBack() Model {
	m.focus--
	if m.focus < FocusConnections {
		m.focus = FocusInspector
	}
	// Skip invisible panels
	if m.focus == FocusInspector && !m.inspector.IsVisible() {
		m.focus = FocusResults
	}
	m.applyFocus()
	return m
}

func (m *Model) applyFocus() {
	m.editor.Blur()
	m.tabBar.Blur()
	switch m.focus {
	case FocusEditor:
		m.editor.Focus()
	case FocusTabBar:
		m.tabBar.Focus()
	}
}

// moveFocus navigates between panels using vim-style directions.
func (m Model) moveFocus(direction string) Model {
	switch m.focus {
	case FocusConnections:
		if direction == "ctrl+l" {
			m.focus = FocusTabBar
		}
	case FocusTabBar:
		switch direction {
		case "ctrl+h":
			m.focus = FocusConnections
		case "ctrl+j":
			m.focus = FocusEditor
		}
	case FocusEditor:
		switch direction {
		case "ctrl+h":
			m.focus = FocusConnections
		case "ctrl+k":
			m.focus = FocusTabBar
		case "ctrl+j":
			m.focus = FocusResults
		case "ctrl+l":
			if m.inspector.IsVisible() {
				m.focus = FocusInspector
			}
		}
	case FocusResults:
		switch direction {
		case "ctrl+h":
			m.focus = FocusConnections
		case "ctrl+k":
			m.focus = FocusEditor
		case "ctrl+l":
			if m.inspector.IsVisible() {
				m.focus = FocusInspector
			}
		}
	case FocusInspector:
		if direction == "ctrl+h" {
			m.focus = FocusResults
		}
	}
	m.applyFocus()
	return m
}

func (m Model) updateLayout() Model {
	if m.width == 0 || m.height == 0 {
		return m
	}

	if m.state == stateConnections {
		return m
	}

	if m.state == stateAddConnection {
		m.connForm.SetSize(m.width, m.height)
		return m
	}

	m.layoutWorkspace()
	return m
}

// layoutWorkspace sizes the workspace panels. Uses pointer receiver so it
// works correctly when called from both value and pointer receiver methods.
func (m *Model) layoutWorkspace() {
	sidebarWidth := 30
	inspectorWidth := InspectorWidth
	statusHeight := 1
	tabBarHeight := 0 // tabs are inside the editor panel
	borderOverhead := 2
	editorHeight := 12

	if m.editorMaximized {
		// Editor takes most of the vertical space, results gets a sliver.
		editorHeight = m.height - statusHeight - tabBarHeight - borderOverhead - 8
		if editorHeight < 8 {
			editorHeight = 8
		}
	}

	inspectorVisible := m.inspector.IsVisible()

	rightWidth := m.width - sidebarWidth - borderOverhead
	if inspectorVisible {
		rightWidth -= inspectorWidth
	}

	resultsHeight := m.height - tabBarHeight - editorHeight - statusHeight - borderOverhead
	if resultsHeight < 3 {
		resultsHeight = 3
	}

	// Sidebar and inspector span the same height as editor + results combined.
	sideContentHeight := m.height - statusHeight - borderOverhead
	if sideContentHeight < 3 {
		sideContentHeight = 3
	}

	editorContentHeight := editorHeight - borderOverhead - 2 // -2 for tab line + separator inside editor
	if editorContentHeight < 1 {
		editorContentHeight = 1
	}

	m.connList.SetSize(sidebarWidth-borderOverhead, sideContentHeight)
	m.editor.SetSize(rightWidth, editorContentHeight)

	m.tabBar.SetSize(rightWidth, 1)

	m.results.SetSize(rightWidth+borderOverhead, resultsHeight+borderOverhead)

	if inspectorVisible {
		viewHeight := tabBarHeight + editorHeight + resultsHeight
		m.inspector.SetSize(inspectorWidth-borderOverhead, viewHeight)
	}
}
