package ui

// isFocusable reports whether a panel is present and can receive focus.
// The sidebar, tab bar, editor, and results are always present; the inspector
// and assistant are only focusable when open (they share the right-hand slot).
func (m Model) isFocusable(f Focus) bool {
	switch f {
	case FocusInspector:
		return m.inspector.IsVisible()
	case FocusAssistant:
		return m.assistant.IsVisible()
	}
	return true
}

func (m Model) cycleFocus() Model {
	next := m.focus
	for i := 0; i <= int(FocusAssistant); i++ {
		next++
		if next > FocusAssistant {
			next = FocusConnections
		}
		if m.isFocusable(next) {
			break
		}
	}
	m.focus = next
	m.applyFocus()
	return m
}

func (m Model) cycleFocusBack() Model {
	next := m.focus
	for i := 0; i <= int(FocusAssistant); i++ {
		next--
		if next < FocusConnections {
			next = FocusAssistant
		}
		if m.isFocusable(next) {
			break
		}
	}
	m.focus = next
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
	case FocusAssistant:
		// Landing on the chat panel enters compose mode so typing works at once.
		m.assistant.StartCompose()
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
			// The right-hand slot holds at most one of inspector / assistant.
			if m.assistant.IsVisible() {
				m.focus = FocusAssistant
			} else if m.inspector.IsVisible() {
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
			if m.assistant.IsVisible() {
				m.focus = FocusAssistant
			} else if m.inspector.IsVisible() {
				m.focus = FocusInspector
			}
		}
	case FocusInspector:
		if direction == "ctrl+h" {
			m.focus = FocusResults
		}
	case FocusAssistant:
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

	// Size the help overlay on the persisted model. The view methods
	// (viewConnections/viewWorkspace) are value receivers, so their
	// m.help.SetSize calls only sized a throwaway copy — leaving help's
	// stored width/height at 0. That made maxOff() (used by the scroll
	// handlers during Update) compute a different clamp than View, so j/k
	// could drift past the real bottom and then have to "burn" back.
	m.help.SetSize(m.width, m.height-1)

	if m.state == stateConnections {
		contentW, listH := m.connListContentDims()
		m.connList.SetSize(contentW, listH)
		return m
	}

	if m.state == stateAddConnection {
		innerW, contentH := popupContentSize(m.height)
		m.connForm.SetSize(innerW, contentH)
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
		editorHeight = m.height - statusHeight - tabBarHeight - borderOverhead - 12
		if editorHeight < 8 {
			editorHeight = 8
		}
	}

	inspectorVisible := m.inspector.IsVisible()
	assistantVisible := m.assistant.IsVisible()

	rightWidth := m.width - sidebarWidth - borderOverhead
	// The inspector and assistant share the right-hand slot (mutually
	// exclusive), so subtract whichever (if any) is open.
	rightSlotWidth := 0
	if inspectorVisible {
		rightSlotWidth = InspectorWidth
	} else if assistantVisible {
		rightSlotWidth = AssistantWidth
	}
	if rightSlotWidth > 0 {
		rightWidth -= rightSlotWidth
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
	if assistantVisible {
		viewHeight := tabBarHeight + editorHeight + resultsHeight
		m.assistant.SetSize(AssistantWidth-borderOverhead, viewHeight)
	}
}
