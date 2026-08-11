package ui

// Default and clamp sizes for the workspace chrome. Layout, View, and mouse
// hit-testing all read these through workspaceGeom — never re-declare them.
const (
	defaultSidebarWidth = 30
	defaultEditorHeight = 12
	minEditorHeight     = 8
	minResultsHeight    = 3
	minSidebarWidth     = 18
	minCenterWidth      = 40 // editor/results column must stay usable
	workspaceStatusH    = 1
	workspaceBorderOH   = 2
)

// workspaceGeom is the single source of truth for workspace panel geometry.
// layoutWorkspace, viewWorkspace, and mouse hit-tests must all derive from this
// so a resize (or maximize) cannot desync paint from click mapping.
type workspaceGeom struct {
	SidebarWidth  int // left column outer width
	EditorHeight  int // editor panel outer height (incl. borders)
	ResultsHeight int // results panel height as used by viewWorkspace
	ResultsTop    int // screen Y of the first results row (== EditorHeight)
	ResultsBottom int // inclusive last Y of the panel area (above status)
	EditorRight   int // exclusive right X of the centre column
	RightWidth    int // centre-column content width (editor/results style width)
	RightSlotW    int // inspector / assistant / docked-explorer width (0 if closed)
	CmdHeight     int // bottom ":"/"/" line (0 or 1)
	BorderOH      int
	StatusH       int
}

// workspaceGeom computes panel bounds for the current model. Safe to call with
// a zero-sized terminal (returns zeros / clamps); callers that size widgets
// still guard on width/height == 0 in updateLayout.
func (m Model) workspaceGeom() workspaceGeom {
	g := workspaceGeom{
		BorderOH: workspaceBorderOH,
		StatusH:  workspaceStatusH,
	}
	if m.width == 0 || m.height == 0 {
		g.SidebarWidth = defaultSidebarWidth
		g.EditorHeight = defaultEditorHeight
		return g
	}

	if m.ex.visible || m.searching || m.backendSearching {
		g.CmdHeight = 1
	}

	switch {
	case m.inspector.IsVisible():
		g.RightSlotW = InspectorWidth
	case m.assistant.IsVisible():
		g.RightSlotW = AssistantWidth
	case m.explorer.IsVisible() && m.explorer.docked:
		g.RightSlotW = InspectorWidth
	}

	g.SidebarWidth = m.effectiveSidebarWidth(g.RightSlotW)
	g.EditorHeight = m.effectiveEditorHeight(g.CmdHeight)

	g.ResultsHeight = m.height - g.EditorHeight - g.StatusH - g.CmdHeight - g.BorderOH
	if g.ResultsHeight < minResultsHeight {
		g.ResultsHeight = minResultsHeight
	}

	g.ResultsTop = g.EditorHeight
	// Status bar occupies the last row; the optional cmd line sits just above
	// it. Mouse handlers ignore input while the cmd line is open, so the
	// bottom of the clickable panel area is always height-2.
	g.ResultsBottom = m.height - g.StatusH - 1

	g.EditorRight = m.width
	if g.RightSlotW > 0 {
		g.EditorRight = m.width - g.RightSlotW
	}

	g.RightWidth = m.width - g.SidebarWidth - g.BorderOH
	if g.RightSlotW > 0 {
		g.RightWidth -= g.RightSlotW
	}
	return g
}

// effectiveSidebarWidth returns the outer sidebar width from sidebarSplitW
// (or the default), clamped so the centre column keeps minCenterWidth.
func (m Model) effectiveSidebarWidth(rightSlotW int) int {
	maxW := m.width - workspaceBorderOH - rightSlotW - minCenterWidth
	if maxW < minSidebarWidth {
		maxW = minSidebarWidth
	}

	w := m.sidebarSplitW
	if w <= 0 {
		w = defaultSidebarWidth
	}
	if w < minSidebarWidth {
		w = minSidebarWidth
	}
	if w > maxW {
		w = maxW
	}
	return w
}

// effectiveEditorHeight returns the outer editor-panel height, honouring
// maximize and the user-dragged split (editorSplitH), clamped so results keep
// at least minResultsHeight rows.
func (m Model) effectiveEditorHeight(cmdHeight int) int {
	avail := m.height - workspaceStatusH - cmdHeight - workspaceBorderOH
	maxH := avail - minResultsHeight
	if maxH < minEditorHeight {
		maxH = minEditorHeight
	}

	if m.editorMaximized {
		// Most of the vertical space; leave a results sliver (~12 rows of
		// chrome for the bottom panel), matching the pre-geom behaviour.
		h := avail - 12
		if h < minEditorHeight {
			h = minEditorHeight
		}
		if h > maxH {
			h = maxH
		}
		return h
	}

	h := m.editorSplitH
	if h <= 0 {
		h = defaultEditorHeight
	}
	if h < minEditorHeight {
		h = minEditorHeight
	}
	if h > maxH {
		h = maxH
	}
	return h
}

// isFocusable reports whether a panel is present and can receive focus.
// The sidebar, tab bar, editor, and results are always present; the inspector
// and assistant are only focusable when open (they share the right-hand slot).
func (m Model) isFocusable(f Focus) bool {
	switch f {
	case FocusInspector:
		return m.inspector.IsVisible()
	case FocusAssistant:
		return m.assistant.IsVisible()
	case FocusExplorer:
		return m.explorer.IsVisible() && m.explorer.docked
	}
	return true
}

func (m Model) cycleFocus() Model {
	next := m.focus
	for i := 0; i <= int(FocusExplorer); i++ {
		next++
		if next > FocusExplorer {
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
	for i := 0; i <= int(FocusExplorer); i++ {
		next--
		if next < FocusConnections {
			next = FocusExplorer
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
		// Browse mode by default: `i`/`a` enters compose to type a question,
		// `M` opens the model picker, `j`/`k` scrolls. Compose is a transient
		// mode (exited on submit/esc), so browse commands are always reachable.
	}
}

// rightSlotFocus returns the focus target for the right-hand slot — the
// inspector, assistant, or docked explorer, which are mutually exclusive — or
// ok=false when none is open. Shared across moveFocus so the centre column and
// the sidebar agree on what "right" means.
func (m Model) rightSlotFocus() (Focus, bool) {
	switch {
	case m.explorer.IsVisible() && m.explorer.docked:
		return FocusExplorer, true
	case m.assistant.IsVisible():
		return FocusAssistant, true
	case m.inspector.IsVisible():
		return FocusInspector, true
	}
	return 0, false
}

// moveFocus navigates between panels using vim-style directions, treating the
// workspace as three columns: the sidebar (Connections), the centre stack
// (tab bar / editor / results), and the right-hand slot (inspector / assistant
// / docked explorer). ctrl+l/ctrl+h move between columns; ctrl+j/ctrl/k move
// within the centre stack. The editor is the centre hub, so a full traverse is
// Sidebar →(l)→ Editor →(l)→ right slot and the reverse mirrors it —
// navigation never dead-ends at the editor.
func (m Model) moveFocus(direction string) Model {
	right, hasRight := m.rightSlotFocus()
	switch m.focus {
	case FocusConnections:
		// The sidebar spans the full height, so only horizontal moves apply.
		if direction == "ctrl+l" {
			m.focus = FocusEditor
		}
	case FocusTabBar:
		switch direction {
		case "ctrl+h":
			m.focus = FocusConnections
		case "ctrl+j":
			m.focus = FocusEditor
		case "ctrl+l":
			if hasRight {
				m.focus = right
			}
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
			if hasRight {
				m.focus = right
			}
		}
	case FocusResults:
		switch direction {
		case "ctrl+h":
			m.focus = FocusConnections
		case "ctrl+k":
			m.focus = FocusEditor
		case "ctrl+l":
			if hasRight {
				m.focus = right
			}
		}
	case FocusInspector, FocusAssistant, FocusExplorer:
		// The right slot spans the full height; move back left to the editor
		// (the centre hub).
		if direction == "ctrl+h" {
			m.focus = FocusEditor
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
	g := m.workspaceGeom()

	inspectorVisible := m.inspector.IsVisible()
	assistantVisible := m.assistant.IsVisible()
	explorerDocked := m.explorer.IsVisible() && m.explorer.docked

	// Sidebar and inspector span the same height as editor + results combined.
	sideContentHeight := m.height - g.StatusH - g.BorderOH
	if sideContentHeight < 3 {
		sideContentHeight = 3
	}

	editorContentHeight := g.EditorHeight - g.BorderOH - 2 // -2 for tab line + separator
	if editorContentHeight < 1 {
		editorContentHeight = 1
	}

	m.connList.SetSize(g.SidebarWidth-g.BorderOH, sideContentHeight)
	m.editor.SetSize(g.RightWidth, editorContentHeight)

	m.tabBar.SetSize(g.RightWidth, 1)

	m.results.SetSize(g.RightWidth+g.BorderOH, g.ResultsHeight+g.BorderOH)

	if inspectorVisible {
		viewHeight := g.EditorHeight + g.ResultsHeight
		m.inspector.SetSize(InspectorWidth-g.BorderOH, viewHeight)
	}
	if assistantVisible {
		viewHeight := g.EditorHeight + g.ResultsHeight
		m.assistant.SetSize(AssistantWidth-g.BorderOH, viewHeight)
	}
	if explorerDocked {
		// Rendered directly in the slot (View() carries its own border), so the
		// total panel is InspectorWidth × (editor+results+borders).
		viewHeight := g.EditorHeight + g.ResultsHeight + g.BorderOH
		m.explorer.SetSize(InspectorWidth, viewHeight)
	}

	// Modal overlay panels (explain / lookup) share a centered 70% size. They
	// MUST be sized here rather than only in View: View has a value receiver,
	// so a SetSize there mutates a throwaway copy. The docked explorer is sized
	// as a slot panel above, not here.
	overlayW := m.width * 70 / 100
	overlayH := (m.height - 1) * 70 / 100
	m.explainPanel.SetSize(overlayW, overlayH)
	m.lookupPanel.SetSize(overlayW, overlayH)
}
