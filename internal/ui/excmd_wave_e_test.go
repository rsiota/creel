package ui

import (
	"strings"
	"testing"

	"github.com/ruben/creel/internal/config"
)

func TestExTruncate(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.DB().Exec(`INSERT INTO users (id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}

	t.Run("not connected", func(t *testing.T) {
		m := &Model{}
		m.runExCommand("truncate users")
		if !strings.Contains(m.schemaMsg, "not connected") {
			t.Errorf(":truncate -> %q", m.schemaMsg)
		}
	})
	t.Run("stages confirm", func(t *testing.T) {
		m := &Model{
			connection: conn,
			tables:     []string{"users"},
			results:    NewResultsTable(),
			settings:   config.Settings{}, // confirm_destructive defaults on
		}
		cmd := m.runExCommand("truncate users")
		if cmd != nil {
			t.Fatal("expected nil cmd while confirm is staged")
		}
		if m.truncateConfirm != "users" {
			t.Errorf("truncateConfirm = %q, want users", m.truncateConfirm)
		}
	})
	t.Run("force skips confirm", func(t *testing.T) {
		m := &Model{
			connection: conn,
			tables:     []string{"users"},
			results:    NewResultsTable(),
			settings:   config.Settings{},
		}
		cmd := m.runExCommand("truncate! users")
		if cmd == nil {
			t.Fatal("expected truncate cmd")
		}
		if m.truncateConfirm != "" {
			t.Error("force should not stage confirm")
		}
		msg := cmd()
		if trm, ok := msg.(truncateResultMsg); !ok || trm.err != nil {
			t.Fatalf("truncate result: %#v", msg)
		}
	})
}

func TestExDrop(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	if _, err := conn.DB().Exec(`CREATE TABLE doomed (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}

	t.Run("stages typed confirm", func(t *testing.T) {
		m := &Model{
			connection: conn,
			tables:     []string{"doomed"},
			results:    NewResultsTable(),
			settings:   config.Settings{},
		}
		cmd := m.runExCommand("drop doomed")
		if cmd != nil {
			t.Fatal("expected nil while confirm staged")
		}
		if m.dropTableConfirm != "doomed" {
			t.Errorf("dropTableConfirm = %q", m.dropTableConfirm)
		}
	})
	t.Run("force drops", func(t *testing.T) {
		m := &Model{
			connection: conn,
			tables:     []string{"doomed"},
			results:    NewResultsTable(),
			settings:   config.Settings{},
		}
		cmd := m.runExCommand("drop! doomed")
		if cmd == nil {
			t.Fatal("expected drop cmd")
		}
		msg := cmd().(schemaResultMsg)
		if msg.err != nil {
			t.Fatalf("drop failed: %v", msg.err)
		}
	})
}

func TestExRename(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.DB().Exec(`CREATE TABLE orders (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}

	t.Run("opens form with one arg", func(t *testing.T) {
		m := &Model{
			connection: conn,
			tables:     []string{"users"},
			results:    NewResultsTable(),
			width:      120,
			height:     40,
		}
		m.runExCommand("rename users")
		if !m.tableRenameForm.IsVisible() {
			t.Fatal("expected rename form")
		}
		if m.tableRenameForm.Table() != "users" {
			t.Errorf("form table = %q", m.tableRenameForm.Table())
		}
	})
	t.Run("bare prefers sidebar over results source", func(t *testing.T) {
		// Regression: :rename used currentTable(), which preferred SourceTable
		// ("users") over the sidebar cursor ("orders") — unlike sidebar r.
		m := &Model{
			connection: conn,
			tables:     []string{"users", "orders"},
			results:    NewResultsTable(),
			focus:      FocusResults, // not the sidebar
			width:      120,
			height:     40,
		}
		m.results.SetEditable("users", []string{"id"})
		m.sidebarCursor = 1 // orders
		m.runExCommand("rename")
		if !m.tableRenameForm.IsVisible() {
			t.Fatalf("expected rename form (%q)", m.schemaMsg)
		}
		if m.tableRenameForm.Table() != "orders" {
			t.Errorf("form table = %q, want orders (sidebar selection)", m.tableRenameForm.Table())
		}
	})
	t.Run("two args renames directly", func(t *testing.T) {
		m := &Model{
			connection: conn,
			tables:     []string{"users"},
			results:    NewResultsTable(),
		}
		cmd := m.runExCommand("rename users accounts")
		if cmd == nil {
			t.Fatalf(":rename -> %q", m.schemaMsg)
		}
		msg := cmd().(schemaResultMsg)
		if msg.err != nil {
			t.Fatalf("rename failed: %v", msg.err)
		}
		if msg.newTable != "accounts" {
			t.Errorf("newTable = %q", msg.newTable)
		}
	})
	t.Run("missing args", func(t *testing.T) {
		m := &Model{connection: conn, results: NewResultsTable()}
		m.runExCommand("rename")
		if !strings.Contains(m.schemaMsg, "no current table") {
			t.Errorf(":rename bare -> %q", m.schemaMsg)
		}
	})
}

func TestExCreate(t *testing.T) {
	t.Run("not connected", func(t *testing.T) {
		m := &Model{}
		m.runExCommand("create")
		if !strings.Contains(m.schemaMsg, "not connected") {
			t.Errorf(":create -> %q", m.schemaMsg)
		}
	})
	t.Run("opens designer", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, tables: nil, width: 120, height: 40}
		m.runExCommand("create")
		if !m.tableDesigner.IsVisible() {
			t.Fatal("expected table designer")
		}
	})
}

func TestExDDLReadOnly(t *testing.T) {
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
	m.runExCommand("truncate! users")
	if !strings.Contains(m.schemaMsg, "read-only") {
		t.Errorf(":truncate readonly -> %q", m.schemaMsg)
	}
	m.runExCommand("drop! users")
	if !strings.Contains(m.schemaMsg, "read-only") {
		t.Errorf(":drop readonly -> %q", m.schemaMsg)
	}
	m.runExCommand("create")
	if !strings.Contains(m.schemaMsg, "read-only") {
		t.Errorf(":create readonly -> %q", m.schemaMsg)
	}
}
