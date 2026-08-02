package ui

import (
	"strings"
	"testing"

	"github.com/ruben/creel/internal/config"
	"github.com/ruben/creel/internal/db"
)

func TestExAddColumn(t *testing.T) {
	t.Run("not connected", func(t *testing.T) {
		m := &Model{}
		m.runExCommand("addcolumn users email TEXT")
		if !strings.Contains(m.schemaMsg, "not connected") {
			t.Errorf(":addcolumn -> %q", m.schemaMsg)
		}
	})
	t.Run("read-only", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, tables: []string{"users"}, results: NewResultsTable(), forceReadOnly: true}
		m.runExCommand("addcolumn users email TEXT")
		if !strings.Contains(m.schemaMsg, "read-only") {
			t.Errorf(":addcolumn readonly -> %q", m.schemaMsg)
		}
	})
	t.Run("opens form with one arg", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, tables: []string{"users"}, results: NewResultsTable(), width: 120, height: 40}
		m.runExCommand("addcolumn users")
		if !m.addColumnForm.IsVisible() {
			t.Fatalf("expected add-column form (%q)", m.schemaMsg)
		}
		if m.addColumnForm.Table() != "users" {
			t.Errorf("form table = %q", m.addColumnForm.Table())
		}
	})
	t.Run("bare opens form for current table", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{
			connection: conn, tables: []string{"users"}, results: NewResultsTable(),
			focus: FocusResults, width: 120, height: 40,
		}
		m.results.SetEditable("users", []string{"id"})
		m.sidebarCursor = 0 // users
		m.runExCommand("addcolumn")
		if !m.addColumnForm.IsVisible() {
			t.Fatalf("expected add-column form (%q)", m.schemaMsg)
		}
		if m.addColumnForm.Table() != "users" {
			t.Errorf("form table = %q, want users", m.addColumnForm.Table())
		}
	})
	t.Run("two args usage error", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, tables: []string{"users"}, results: NewResultsTable()}
		cmd := m.runExCommand("addcolumn users email")
		if cmd != nil {
			t.Fatal("expected nil cmd for the ambiguous 2-arg form")
		}
		if !strings.Contains(m.schemaMsg, "usage:") {
			t.Errorf(":addcolumn users email -> %q", m.schemaMsg)
		}
	})
	t.Run("no such table", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, tables: []string{"users"}, results: NewResultsTable()}
		m.runExCommand("addcolumn missing email TEXT")
		if !strings.Contains(m.schemaMsg, "no such table") {
			t.Errorf(":addcolumn missing -> %q", m.schemaMsg)
		}
	})
}

// TestExAddColumnDirect runs ALTER TABLE ADD COLUMN against a real SQLite
// database (the direct path reuses execSchemaDDL, same as the form).
func TestExAddColumnDirect(t *testing.T) {
	setup := func(t *testing.T) (*Model, *db.Connection) {
		conn := newSQLiteTestConn(t)
		if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY)`); err != nil {
			t.Fatal(err)
		}
		return &Model{connection: conn, tables: []string{"users"}, results: NewResultsTable()}, conn
	}
	hasCol := func(t *testing.T, conn *db.Connection, table, col string) bool {
		cols, err := conn.DB().TableSchema(table)
		if err != nil {
			t.Fatalf("TableSchema: %v", err)
		}
		for _, c := range cols {
			if c.Name == col {
				return true
			}
		}
		return false
	}

	t.Run("adds nullable column", func(t *testing.T) {
		m, conn := setup(t)
		defer conn.Close()
		cmd := m.runExCommand("addcolumn users email TEXT")
		if cmd == nil {
			t.Fatalf(":addcolumn -> %q", m.schemaMsg)
		}
		msg := cmd().(schemaResultMsg)
		if msg.err != nil {
			t.Fatalf("add column failed: %v", msg.err)
		}
		if !hasCol(t, conn, "users", "email") {
			t.Error("email column was not added")
		}
	})
	t.Run("not null without default rejected up front", func(t *testing.T) {
		m, conn := setup(t)
		defer conn.Close()
		cmd := m.runExCommand("addcolumn users flag INTEGER no")
		if cmd != nil {
			t.Fatal("expected nil cmd; NOT NULL without default must be rejected before exec")
		}
		if !strings.Contains(m.schemaMsg, "default") {
			t.Errorf(":addcolumn notnull -> %q", m.schemaMsg)
		}
		if hasCol(t, conn, "users", "flag") {
			t.Error("flag column should not have been added")
		}
	})
	t.Run("not null with default adds", func(t *testing.T) {
		m, conn := setup(t)
		defer conn.Close()
		cmd := m.runExCommand("addcolumn users score INTEGER no 0")
		if cmd == nil {
			t.Fatalf(":addcolumn -> %q", m.schemaMsg)
		}
		msg := cmd().(schemaResultMsg)
		if msg.err != nil {
			t.Fatalf("add column failed: %v", msg.err)
		}
		if !hasCol(t, conn, "users", "score") {
			t.Error("score column was not added")
		}
	})
	t.Run("bad nullable value rejected", func(t *testing.T) {
		m, conn := setup(t)
		defer conn.Close()
		cmd := m.runExCommand("addcolumn users email TEXT maybe")
		if cmd != nil {
			t.Fatal("expected nil cmd for a bad nullable value")
		}
		if !strings.Contains(m.schemaMsg, "nullable") {
			t.Errorf(":addcolumn bad nullable -> %q", m.schemaMsg)
		}
	})
}

// Guard the read-only / txn blocking for the direct path alongside the other
// DDL read-only tests.
func TestExAddColumnReadOnly(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	yes := true
	m := &Model{
		connection:    conn,
		tables:        []string{"users"},
		results:       NewResultsTable(),
		forceReadOnly: true,
		settings:      config.Settings{ConfirmDestructive: &yes},
	}
	m.runExCommand("addcolumn users email TEXT")
	if !strings.Contains(m.schemaMsg, "read-only") {
		t.Errorf(":addcolumn readonly -> %q", m.schemaMsg)
	}
}
