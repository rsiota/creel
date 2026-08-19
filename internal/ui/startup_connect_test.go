package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestConnectWithConfig_SQLiteOpensWorkspace(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "startup.db")
	s := db.NewSQLite(db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath})
	if err := s.Connect(); err != nil {
		t.Fatalf("setup connect: %v", err)
	}
	if _, err := s.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	s.Close()

	m := NewModel(&config.Config{})
	cmd := m.connectWithConfig(db.ConnectionConfig{
		Driver:   db.DriverSQLite,
		Database: dbPath,
	})
	if m.connError != "" {
		t.Fatalf("connError = %q", m.connError)
	}
	if m.state != stateWorkspace {
		t.Fatalf("state = %v, want workspace", m.state)
	}
	if m.connection == nil {
		t.Fatal("connection is nil")
	}
	t.Cleanup(func() {
		if m.connection != nil {
			m.connection.Close()
		}
	})
	if m.connection.Config().Name != "startup.db" {
		t.Errorf("Name = %q, want basename startup.db", m.connection.Config().Name)
	}
	found := false
	for _, name := range m.tables {
		if name == "t" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("tables = %v, want to include t", m.tables)
	}
	// Follow-up cmds (editor focus + prefetch) should be returned for Init.
	if cmd == nil {
		t.Error("expected non-nil startup cmd batch")
	}
}

func TestConnectWithConfig_MissingFileSetsError(t *testing.T) {
	m := NewModel(&config.Config{})
	m.connectWithConfig(db.ConnectionConfig{
		Driver:   db.DriverSQLite,
		Database: filepath.Join(t.TempDir(), "no", "such", "dir", "x.db"),
	})
	if m.connError == "" {
		t.Fatal("expected connError for missing parent directory")
	}
	if !strings.Contains(m.connError, "does not exist") {
		t.Fatalf("connError = %q, want a missing-file hint", m.connError)
	}
	if m.state != stateConnections {
		t.Errorf("state = %v, want connections on failure", m.state)
	}
	if m.connection != nil {
		m.connection.Close()
		t.Error("connection should remain nil on failure")
	}
}

func TestConnectFailureShowsInStatusBar(t *testing.T) {
	m := NewModel(&config.Config{})
	m.state = stateConnections
	m.connError = "MySQL is not running or not accepting connections on that host and port — dial tcp: connection refused"
	got := m.statusMessage()
	if got == "" {
		t.Fatal("expected status message for connError")
	}
	if !strings.Contains(got, "MySQL is not running") {
		t.Fatalf("statusMessage = %q", got)
	}
}
