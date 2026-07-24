package ui

import (
	"path/filepath"
	"testing"

	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

// newNamedSQLiteConn opens a fresh SQLite database with a configured Name so
// its Config() can key a session. (newSQLiteTestConn leaves Name empty.)
func newNamedSQLiteConn(t *testing.T, name string) *db.Connection {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	conn, err := db.New(db.ConnectionConfig{Name: name, Driver: db.DriverSQLite, Database: path})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return conn
}

// TestSessionSaveRestoreRoundTrip verifies that saveSession captures the
// workspace (tabs + editor buffers + active tab) and restoreSession brings it
// back in a fresh model keyed on the same connection+database.
func TestSessionSaveRestoreRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	conn := newNamedSQLiteConn(t, "testconn")

	m := NewModel(&config.Config{})
	m.connection = conn

	// Two tabs of editor content; the second is active.
	m.editor.SetValue("SELECT * FROM users;")
	m.saveTabState()
	m.addTab("orders", "SELECT * FROM orders;") // makes "orders" active
	m.saveSession()

	// A brand-new model (simulating a later launch) restores from disk.
	m2 := NewModel(&config.Config{})
	m2.connection = conn
	m2.restoreSession()

	if len(m2.resultsTabs) != 2 {
		t.Fatalf("restored %d tabs, want 2", len(m2.resultsTabs))
	}
	got := m2.editor.Value()
	if got != "SELECT * FROM orders;" {
		t.Errorf("active editor = %q, want 'SELECT * FROM orders;'", got)
	}
	if m2.resultsTabs[0].EditorQuery != "SELECT * FROM users;" {
		t.Errorf("tab 0 editor = %q", m2.resultsTabs[0].EditorQuery)
	}
	// The "orders" tab (index 1) should be the restored active one.
	if m2.activeTabID != m2.resultsTabs[1].ID {
		t.Errorf("active tab = %d, want orders (%d)", m2.activeTabID, m2.resultsTabs[1].ID)
	}
}

// TestSessionRestoreNoopWithoutSession confirms reconnecting with no prior
// session leaves the default single "New Query" tab untouched.
func TestSessionRestoreNoopWithoutSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	conn := newNamedSQLiteConn(t, "fresh")
	m := NewModel(&config.Config{})
	m.connection = conn
	m.restoreSession()

	if len(m.resultsTabs) != 1 || m.resultsTabs[0].Title != "New Query" {
		t.Errorf("default tab disturbed: %+v", m.resultsTabs)
	}
	if m.editor.Value() != "" {
		t.Errorf("editor = %q, want empty", m.editor.Value())
	}
}

// TestSessionKeyedByConnectionAndDatabase verifies that state saved under one
// connection+database is not restored under a different database — the pair,
// not the connection alone, is the key.
func TestSessionKeyedByConnectionAndDatabase(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	connA := newNamedSQLiteConn(t, "conn")
	m := NewModel(&config.Config{})
	m.connection = connA
	m.editor.SetValue("db-A-query")
	m.saveTabState()
	m.saveSession()

	// Different connection (so a different database path) must not pick it up.
	connB := newNamedSQLiteConn(t, "conn")
	m2 := NewModel(&config.Config{})
	m2.connection = connB
	m2.restoreSession()

	if m2.editor.Value() != "" {
		t.Errorf("cross-database restore leaked %q", m2.editor.Value())
	}
}

// TestSaveSessionNoopWithoutConnection ensures saveSession does nothing (and
// does not panic) when no connection is open — e.g. quitting from the
// connection picker.
func TestSaveSessionNoopWithoutConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{})
	m.saveSession() // must not panic / must not write

	// Nothing to assert beyond not panicking; confirm no session dir created.
	st, err := m.sessionStore.Load("anything", "anydb")
	if err != nil || st.HasContent() {
		t.Errorf("expected no persisted session, got %+v (%v)", st, err)
	}
}

// TestRestoreSkippedAfterStartupFile verifies that a `gsql -f` loaded buffer
// is not clobbered by a restored session on the first connect, while a later
// connect (switching connections) restores normally.
func TestRestoreSkippedAfterStartupFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	conn := newNamedSQLiteConn(t, "withfile")

	// Seed a saved session.
	m := NewModel(&config.Config{})
	m.connection = conn
	m.editor.SetValue("SAVED SESSION QUERY")
	m.saveTabState()
	m.saveSession()

	// A fresh launch with -f: the file content must win on first connect.
	m2 := NewModel(&config.Config{})
	if _, err := m2.loadStartupFile("n/a"); err == nil {
		t.Skip("loadStartupFile unexpectedly read a file")
	}
	// Simulate the -f load directly (loadStartupFile reads a real file).
	m2.editor.SetValue("STARTUP FILE CONTENT")
	m2.startupFileLoaded = true
	m2.connection = conn
	m2.restoreSession()
	if m2.editor.Value() != "STARTUP FILE CONTENT" {
		t.Errorf("startup file clobbered by session: %q", m2.editor.Value())
	}
	// The one-shot flag is consumed, so a second restore now applies.
	m2.restoreSession()
	if m2.editor.Value() != "SAVED SESSION QUERY" {
		t.Errorf("second restore should apply session: %q", m2.editor.Value())
	}
}
func TestBeginQuitPersistsSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	conn := newNamedSQLiteConn(t, "quitconn")

	m := NewModel(&config.Config{})
	m.connection = conn
	m.editor.SetValue("SELECT 1;")
	m.saveTabState()
	m.beginQuit()

	if !m.quitting {
		t.Error("beginQuit did not set quitting")
	}
	m2 := NewModel(&config.Config{})
	m2.connection = conn
	m2.restoreSession()
	if m2.editor.Value() != "SELECT 1;" {
		t.Errorf("session not persisted by beginQuit: %q", m2.editor.Value())
	}
}
