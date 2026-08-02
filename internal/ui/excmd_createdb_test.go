package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

// newDriverConfigConn builds a *db.Connection for a driver WITHOUT connecting,
// so executor guard logic (which reads Config().Driver before any dial) can be
// exercised without a live database server. Calling the returned cmd would
// dial; the tests below don't.
func newDriverConfigConn(t *testing.T, driver db.Driver, database string) *db.Connection {
	t.Helper()
	conn, err := db.New(db.ConnectionConfig{Driver: driver, Database: database})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	return conn
}

func TestExCreateDatabase(t *testing.T) {
	t.Run("not connected", func(t *testing.T) {
		m := &Model{}
		m.runExCommand("createdb foo")
		if !strings.Contains(m.schemaMsg, "not connected") {
			t.Errorf(":createdb -> %q", m.schemaMsg)
		}
	})
	t.Run("sqlite unsupported", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn}
		m.runExCommand("createdb foo")
		if !strings.Contains(m.schemaMsg, "not supported for sqlite") {
			t.Errorf(":createdb sqlite -> %q", m.schemaMsg)
		}
	})
	t.Run("read-only", func(t *testing.T) {
		conn := newDriverConfigConn(t, db.DriverMySQL, "")
		m := &Model{connection: conn, forceReadOnly: true}
		m.runExCommand("createdb foo")
		if !strings.Contains(m.schemaMsg, "read-only") {
			t.Errorf(":createdb readonly -> %q", m.schemaMsg)
		}
	})
	t.Run("missing name", func(t *testing.T) {
		conn := newDriverConfigConn(t, db.DriverMySQL, "")
		m := &Model{connection: conn}
		m.runExCommand("createdb")
		if !strings.Contains(m.schemaMsg, "needs a database name") {
			t.Errorf(":createdb bare -> %q", m.schemaMsg)
		}
	})
	t.Run("dispatches create", func(t *testing.T) {
		// No live server: don't call cmd(). A non-nil return means every guard
		// passed and execCreateDatabase built the CREATE DATABASE SQL.
		conn := newDriverConfigConn(t, db.DriverMySQL, "")
		m := &Model{connection: conn}
		cmd := m.runExCommand("createdb appdb")
		if cmd == nil {
			t.Fatalf(":createdb appdb -> %q", m.schemaMsg)
		}
	})
}

func TestExDropDatabase(t *testing.T) {
	t.Run("not connected", func(t *testing.T) {
		m := &Model{}
		m.runExCommand("dropdb foo")
		if !strings.Contains(m.schemaMsg, "not connected") {
			t.Errorf(":dropdb -> %q", m.schemaMsg)
		}
	})
	t.Run("sqlite unsupported", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn}
		m.runExCommand("dropdb foo")
		if !strings.Contains(m.schemaMsg, "not supported for sqlite") {
			t.Errorf(":dropdb sqlite -> %q", m.schemaMsg)
		}
	})
	t.Run("read-only", func(t *testing.T) {
		conn := newDriverConfigConn(t, db.DriverMySQL, "")
		m := &Model{connection: conn, forceReadOnly: true}
		m.runExCommand("dropdb! appdb")
		if !strings.Contains(m.schemaMsg, "read-only") {
			t.Errorf(":dropdb readonly -> %q", m.schemaMsg)
		}
	})
	t.Run("stages typed confirm", func(t *testing.T) {
		yes := true
		conn := newDriverConfigConn(t, db.DriverMySQL, "")
		m := &Model{connection: conn, settings: config.Settings{ConfirmDestructive: &yes}}
		cmd := m.runExCommand("dropdb appdb")
		if cmd != nil {
			t.Fatal("expected nil cmd while confirm is staged")
		}
		if m.dropDBConfirm != "appdb" {
			t.Errorf("dropDBConfirm = %q, want appdb", m.dropDBConfirm)
		}
	})
	t.Run("defaults to current database", func(t *testing.T) {
		yes := true
		conn := newDriverConfigConn(t, db.DriverMySQL, "currdb")
		m := &Model{connection: conn, settings: config.Settings{ConfirmDestructive: &yes}}
		m.runExCommand("dropdb")
		if m.dropDBConfirm != "currdb" {
			t.Errorf("dropDBConfirm = %q, want currdb (the current database)", m.dropDBConfirm)
		}
	})
	t.Run("force skips confirm", func(t *testing.T) {
		yes := true
		conn := newDriverConfigConn(t, db.DriverMySQL, "")
		m := &Model{connection: conn, settings: config.Settings{ConfirmDestructive: &yes}}
		cmd := m.runExCommand("dropdb! appdb")
		if cmd == nil {
			t.Fatalf(":dropdb! -> %q", m.schemaMsg)
		}
		if m.dropDBConfirm != "" {
			t.Error("force should not stage a confirmation")
		}
		// Don't call cmd(): it would dial a real MySQL server.
	})
}
