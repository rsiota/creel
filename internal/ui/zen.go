package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/session"
)

// zenSnapshot captures workspace chrome visibility before entering zen mode.
type zenSnapshot struct {
	sidebarVisible  bool
	editorVisible   bool
	editorMaximized bool
	rightPanel      string
	chartVisible    bool
	focus           Focus
}

// exZen toggles zen mode or exits it explicitly (:zen off).
func (m *Model) exZen(args []string) tea.Cmd {
	if len(args) > 0 && strings.EqualFold(args[0], "off") {
		if m.zenActive {
			m.exitZen()
		}
		return nil
	}
	if err := m.toggleZen(); err != "" {
		m.schemaMsg = err
	}
	return nil
}

// toggleZen enters or leaves zen mode (results-only layout).
func (m *Model) toggleZen() string {
	if m.zenActive {
		m.exitZen()
		return ""
	}
	return m.enterZen()
}

func (m *Model) enterZen() string {
	if msg := m.zenEnterBlockReason(); msg != "" {
		return msg
	}
	m.zenSaved = zenSnapshot{
		sidebarVisible:  m.sidebarVisible,
		editorVisible:   m.editorVisible,
		editorMaximized: m.editorMaximized,
		rightPanel:      m.sessionRightPanel(),
		chartVisible:    m.chartPanel.IsVisible(),
		focus:           m.focus,
	}
	m.zenActive = true

	m.sidebarVisible = false
	m.editorVisible = false
	m.editorMaximized = false
	m.inspector.Hide()
	m.assistant.Hide()
	m.explorer.Hide()
	if m.chartPanel.IsVisible() {
		m.chartPanel.Hide()
	}
	m.focus = FocusResults

	m.layoutWorkspace()
	m.applyFocus()
	return ""
}

func (m *Model) exitZen() {
	if !m.zenActive {
		return
	}
	s := m.zenSaved
	m.zenActive = false

	m.sidebarVisible = s.sidebarVisible
	m.editorVisible = s.editorVisible
	m.editorMaximized = s.editorMaximized
	m.applySessionPanels(session.Panels{Right: s.rightPanel})
	if s.chartVisible {
		m.chartPanel.visible = true
	}

	m.focus = s.focus
	if !m.isFocusable(m.focus) {
		m.focus = FocusResults
	}

	m.layoutWorkspace()
	m.applyFocus()
}

func (m Model) zenEnterBlockReason() string {
	if m.state != stateWorkspace {
		return "zen is only available in the workspace"
	}
	if m.tableDesigner.IsVisible() || m.schemaEditor.IsVisible() {
		return "close the schema editor first"
	}
	if m.results.IsEditing() || m.inspector.IsEditing() || m.inspector.IsInserting() {
		return "finish editing before zen"
	}
	if m.cellEdit.IsVisible() {
		return "close the cell editor first"
	}
	return ""
}

// clearZenState drops zen mode without restoring the pre-zen layout. Called when
// the user changes panel visibility manually while zen is active.
func (m *Model) clearZenState() {
	m.zenActive = false
}
