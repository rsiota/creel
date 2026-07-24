package ui

import "github.com/ruben/gsql/internal/session"

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
// and the active tab) for the active connection+database. It is best-effort:
// write errors are ignored, matching the other persistence sites. Called at
// connection/database teardown and on quit so reopening a connection restores
// where the user left off.
func (m *Model) saveSession() {
	conn, database, ok := m.sessionKey()
	if !ok || m.sessionStore == nil {
		return
	}
	// Flush the active editor into its ResultsTab so the buffer under the
	// cursor is captured alongside the inactive tabs.
	m.saveTabState()

	st := session.State{}
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

// restoreSession rebuilds the workspace tabs from the persisted session for
// the active connection+database. With no session (or a blank one) the default
// single "New Query" tab is left in place. It does not re-run any query — the
// editor buffers are restored verbatim and the user runs them with ctrl+e,
// mirroring the gsql -f startup flag (avoids stale data and side-effecting
// writes on reconnect).
func (m *Model) restoreSession() {
	conn, database, ok := m.sessionKey()
	if !ok || m.sessionStore == nil {
		return
	}
	// An explicit startup file (-f) takes precedence over the saved session
	// for the first connect only; later connects (e.g. switching connections)
	// restore normally.
	if m.startupFileLoaded {
		m.startupFileLoaded = false
		return
	}
	st, err := m.sessionStore.Load(conn, database)
	if err != nil || !st.HasContent() {
		return
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
}

// beginQuit persists the current session and marks the app as quitting. Every
// quit path (ctrl+c/ctrl+q/q/:q/:qa/:wq/esc) funnels through here so the
// workspace is captured exactly once before the program exits.
func (m *Model) beginQuit() {
	m.saveSession()
	m.quitting = true
}
