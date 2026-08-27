package ui

import (
	"strings"

	"github.com/rsiota/creel/internal/session"
)

// sessionKey returns the (connection, database) pair the current workspace is
// bound to — the key under which its state is persisted. ok is false when no
// connection is open, so callers can no-op without keying on empty strings.
func (m *Model) sessionKey() (conn, database string, ok bool) {
	if m.connection == nil {
		return "", "", false
	}
	cfg := m.connection.Config()
	return cfg.Name, cfg.Database, true
}

// saveSession persists the current workspace (open tabs, their editor buffers,
// the active tab, remembered column widths, ERD card positions, panel layout
// sizes, and which right-slot panel was open) for the active
// connection+database. It is best-effort: write errors are ignored, matching
// the other persistence sites. Called at connection/database teardown and on
// quit so reopening a connection restores where the user left off.
func (m *Model) saveSession() {
	conn, database, ok := m.sessionKey()
	if !ok || m.sessionStore == nil {
		return
	}
	// Flush the active editor into its ResultsTab so the buffer under the
	// cursor is captured alongside the inactive tabs.
	m.saveTabState()
	m.snapshotERDPositions()

	st := session.State{
		ColWidths:    m.colWidthMem,
		ERDPositions: m.erdPosMem,
		Layout:       m.snapshotSessionLayout(),
		Panels:       &session.Panels{Right: m.sessionRightPanel()},
	}
	for i, t := range m.resultsTabs {
		st.Tabs = append(st.Tabs, session.Tab{
			Title:     t.Title,
			Editor:    t.EditorQuery,
			LastQuery: t.LastQuery,
		})
		if t.ID == m.activeTabID {
			st.Active = i
		}
	}
	_ = m.sessionStore.Save(conn, database, st)
}

// snapshotSessionLayout captures the current split sizes. Zeros are omitted
// via omitempty so defaults stay unset until the user actually resizes.
func (m *Model) snapshotSessionLayout() *session.Layout {
	l := &session.Layout{
		SidebarWidth:    m.sidebarSplitW,
		EditorHeight:    m.editorSplitH,
		RightSlotWidth:  m.rightSlotSplitW,
		EditorMaximized: m.editorMaximized,
	}
	if l.SidebarWidth == 0 && l.EditorHeight == 0 && l.RightSlotWidth == 0 && !l.EditorMaximized {
		return nil
	}
	return l
}

// sessionRightPanel returns which right-slot panel is currently open.
func (m *Model) sessionRightPanel() string {
	switch {
	case m.inspector.IsVisible():
		return session.RightInspector
	case m.assistant.IsVisible():
		return session.RightAssistant
	case m.explorer.IsVisible() && m.explorer.docked:
		return session.RightExplorer
	default:
		return session.RightNone
	}
}

// restoreSession rebuilds the workspace from the persisted session for the
// active connection+database. With no session (or blank tabs) the default
// single "New Query" tab is left in place. It does not re-run any query — the
// editor buffers are restored verbatim and the user runs them with ctrl+e,
// mirroring the creel -f startup flag (avoids stale data and side-effecting
// writes on reconnect).
//
// Column widths, ERD positions, layout, and panels are always reloaded when a
// session file exists, even if the tabs themselves are blank (HasContent is
// false). The returned flag is true when Panels was present in the session so
// callers can skip the settings.InspectorOpen fallback.
func (m *Model) restoreSession() (panelsRestored bool) {
	conn, database, ok := m.sessionKey()
	if !ok || m.sessionStore == nil {
		return false
	}
	// An explicit startup file (-f) takes precedence over the saved session
	// for the first connect only; later connects (e.g. switching connections)
	// restore normally.
	if m.startupFileLoaded {
		m.startupFileLoaded = false
		return false
	}
	st, err := m.sessionStore.Load(conn, database)
	if err != nil {
		return false
	}
	m.colWidthMem = cloneColWidthMem(st.ColWidths)
	m.erdPosMem = cloneERDPosMem(st.ERDPositions)
	if st.Layout != nil {
		m.applySessionLayout(*st.Layout)
	}
	if st.Panels != nil {
		m.applySessionPanels(*st.Panels)
		panelsRestored = true
	}

	if !st.HasContent() {
		return panelsRestored
	}

	tabs := make([]*ResultsTab, 0, len(st.Tabs))
	for _, t := range st.Tabs {
		tab := NewResultsTab(m.nextTabID, firstNonEmpty(t.Title, "New Query"))
		m.nextTabID++
		tab.EditorQuery = t.Editor
		tab.LastQuery = t.LastQuery
		tabs = append(tabs, tab)
	}

	m.resultsTabs = tabs
	active := st.Active
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	m.activeTabID = tabs[active].ID
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	// Pull the active tab's editor buffer into m.editor so the first frame
	// shows it.
	m.restoreTabState()
	m.editor.CancelCompletion()
	return panelsRestored
}

// applySessionLayout restores split sizes from a saved session. Values are
// stored raw; workspaceGeom clamps them against the current terminal size.
func (m *Model) applySessionLayout(l session.Layout) {
	if l.SidebarWidth > 0 {
		m.sidebarSplitW = l.SidebarWidth
	}
	if l.EditorHeight > 0 {
		m.editorSplitH = l.EditorHeight
	}
	if l.RightSlotWidth > 0 {
		m.rightSlotSplitW = l.RightSlotWidth
	}
	m.editorMaximized = l.EditorMaximized
}

// applySessionPanels opens or closes the right-slot panel to match a saved
// session. Does not steal focus — the connect path still focuses the editor.
func (m *Model) applySessionPanels(p session.Panels) {
	m.inspector.Hide()
	m.assistant.Hide()
	m.explorer.Hide()
	switch p.Right {
	case session.RightInspector:
		m.inspector.Show()
	case session.RightAssistant:
		m.assistant.Show()
	case session.RightExplorer:
		m.explorer.ShowDocked()
	}
}

// beginQuit persists the current session and marks the app as quitting. Every
// quit path (ctrl+c/ctrl+q/q/:q/:qa/:wq/esc) funnels through here so the
// workspace is captured exactly once before the program exits.
func (m *Model) beginQuit() {
	m.saveSession()
	m.quitting = true
}

// syncColWidthMemory applies remembered widths for the current source table
// onto the results grid (so a short page does not shrink columns), then folds
// the resulting widths back into memory (so a wider page grows them). No-op
// for custom queries with no backing table.
func (m *Model) syncColWidthMemory() {
	table := m.results.SourceTable()
	if table == "" {
		return
	}
	table = m.canonicalTableName(table)
	m.results.ApplyRememberedWidths(m.colWidthsFor(table))
	m.mergeColWidths(table, m.results.SnapshotWidths())
}

// colWidthsFor returns the remembered width map for a table, or nil.
func (m *Model) colWidthsFor(table string) map[string]int {
	if m.colWidthMem == nil {
		return nil
	}
	if w, ok := m.colWidthMem[table]; ok {
		return w
	}
	// Case-insensitive fallback for drivers that fold identifiers.
	for name, w := range m.colWidthMem {
		if strings.EqualFold(name, table) {
			return w
		}
	}
	return nil
}

// mergeColWidths raises remembered widths for table to at least the values in
// widths (per column name). New columns are added; existing ones only grow.
func (m *Model) mergeColWidths(table string, widths map[string]int) {
	if table == "" || len(widths) == 0 {
		return
	}
	if m.colWidthMem == nil {
		m.colWidthMem = make(map[string]map[string]int)
	}
	cur, ok := m.colWidthMem[table]
	if !ok {
		// Reuse an EqualFold match's map when the stored key casing differs.
		for name, w := range m.colWidthMem {
			if strings.EqualFold(name, table) {
				cur = w
				ok = true
				table = name
				break
			}
		}
	}
	if !ok {
		cur = make(map[string]int, len(widths))
		m.colWidthMem[table] = cur
	}
	for col, w := range widths {
		if w <= 0 {
			continue
		}
		// Prefer matching an existing column key case-insensitively so we
		// don't sprout duplicate entries when casing drifts.
		key := col
		for existing := range cur {
			if strings.EqualFold(existing, col) {
				key = existing
				break
			}
		}
		if w > cur[key] {
			cur[key] = w
		}
	}
}

// renameColWidthTable moves remembered widths when a table is renamed.
func (m *Model) renameColWidthTable(oldName, newName string) {
	if m.colWidthMem == nil || oldName == "" || newName == "" {
		return
	}
	for name, w := range m.colWidthMem {
		if strings.EqualFold(name, oldName) {
			delete(m.colWidthMem, name)
			m.colWidthMem[newName] = w
			return
		}
	}
}

// canonicalTableName returns the sidebar spelling of table when known.
func (m *Model) canonicalTableName(table string) string {
	for _, t := range m.tables {
		if strings.EqualFold(t, table) {
			return t
		}
	}
	return table
}

// cloneColWidthMem deep-copies a col-width map so mutating memory cannot
// alias the session store's cached State.
func cloneColWidthMem(src map[string]map[string]int) map[string]map[string]int {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]map[string]int, len(src))
	for table, cols := range src {
		cp := make(map[string]int, len(cols))
		for c, w := range cols {
			cp[c] = w
		}
		out[table] = cp
	}
	return out
}

// erdScopeAll is the ERDPositions outer key for a whole-schema layout
// (layout.focus == ""). Neighbourhood layouts use the focused table name so
// a drag in one view cannot land on the other — ranks differ.
const erdScopeAll = "*"

func erdScopeKey(focus string) string {
	if strings.TrimSpace(focus) == "" {
		return erdScopeAll
	}
	return strings.TrimSpace(focus)
}

// snapshotERDPositions copies the current ERD layout's card origins into
// erdPosMem for that layout's scope. No-op while a drag is in flight (cancel
// must restore the last committed positions, not an intermediate frame) or
// when there is no layout.
func (m *Model) snapshotERDPositions() {
	if m.erdPanel.layout == nil || m.erdPanel.dragCard != "" {
		return
	}
	pos := snapshotERDLayout(m.erdPanel.layout)
	if len(pos) == 0 {
		return
	}
	if m.erdPosMem == nil {
		m.erdPosMem = make(map[string]map[string]session.ERDPos)
	}
	m.erdPosMem[erdScopeKey(m.erdPanel.layout.focus)] = pos
}

// applyRememberedERDPositions overlays saved x/y for layout's scope onto its
// cards and re-routes arrows when anything moved. Unknown tables keep the
// freshly computed origin so a schema that grew still places new cards.
func (m *Model) applyRememberedERDPositions(layout *erdLayout) {
	if layout == nil || len(m.erdPosMem) == 0 {
		return
	}
	scope := erdScopeKey(layout.focus)
	saved := m.erdPosMem[scope]
	if saved == nil {
		for name, pos := range m.erdPosMem {
			if strings.EqualFold(name, scope) {
				saved = pos
				break
			}
		}
	}
	applyERDPositions(layout, saved)
}

// renameERDPosTable moves remembered ERD coordinates when a table is renamed:
// both the neighbourhood scope key and every inner card name.
func (m *Model) renameERDPosTable(oldName, newName string) {
	if m.erdPosMem == nil || oldName == "" || newName == "" {
		return
	}
	next := make(map[string]map[string]session.ERDPos, len(m.erdPosMem))
	for scope, tables := range m.erdPosMem {
		newScope := scope
		if strings.EqualFold(scope, oldName) {
			newScope = newName
		}
		inner := make(map[string]session.ERDPos, len(tables))
		for name, pos := range tables {
			key := name
			if strings.EqualFold(name, oldName) {
				key = newName
			}
			inner[key] = pos
		}
		next[newScope] = inner
	}
	m.erdPosMem = next
}

// cloneERDPosMem deep-copies an ERD-position map so mutating memory cannot
// alias the session store's cached State.
func cloneERDPosMem(src map[string]map[string]session.ERDPos) map[string]map[string]session.ERDPos {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]map[string]session.ERDPos, len(src))
	for scope, tables := range src {
		cp := make(map[string]session.ERDPos, len(tables))
		for name, pos := range tables {
			cp[name] = pos
		}
		out[scope] = cp
	}
	return out
}

// snapshotERDLayout records each card's logical origin. Returns nil when the
// layout is empty so callers can skip writing an empty scope.
func snapshotERDLayout(layout *erdLayout) map[string]session.ERDPos {
	if layout == nil || len(layout.cards) == 0 {
		return nil
	}
	out := make(map[string]session.ERDPos, len(layout.cards))
	for _, c := range layout.cards {
		if c == nil || c.name == "" {
			continue
		}
		out[c.name] = session.ERDPos{X: c.x, Y: c.y}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applyERDPositions writes saved origins onto matching cards (case-insensitive
// table names) and re-routes arrows if anything actually moved.
func applyERDPositions(layout *erdLayout, saved map[string]session.ERDPos) {
	if layout == nil || len(saved) == 0 {
		return
	}
	moved := false
	for _, c := range layout.cards {
		if c == nil {
			continue
		}
		p, ok := saved[c.name]
		if !ok {
			for name, pos := range saved {
				if strings.EqualFold(name, c.name) {
					p, ok = pos, true
					break
				}
			}
		}
		if !ok || (c.x == p.X && c.y == p.Y) {
			continue
		}
		c.x, c.y = p.X, p.Y
		moved = true
	}
	if moved {
		rerouteArrows(layout)
	}
}
