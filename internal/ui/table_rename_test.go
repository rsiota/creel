package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

func TestTableRenameFormSubmit(t *testing.T) {
	f := NewTableRenameForm()
	f.Show("users", db.DriverSQLite, []string{"users", "orders"})
	f.field.SetValue("accounts")

	sql, errMsg := f.Submit()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	want := `ALTER TABLE "users" RENAME TO "accounts"`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func setupRenameTestSQLite(t *testing.T) *db.Connection {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath}
	sqlite, err := db.New(cfg)
	if err != nil {
		t.Fatalf("new sqlite: %v", err)
	}
	if err := sqlite.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		sqlite.Close()
		os.Remove(dbPath)
	})
	if _, err := sqlite.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return sqlite
}

func TestTableRenameFlow(t *testing.T) {
	sqlite := setupRenameTestSQLite(t)

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = sqlite
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.tables = []string{"users"}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if !m.tableRenameForm.IsVisible() {
		t.Fatal("expected rename form after pressing r")
	}
	if m.tableRenameForm.Table() != "users" {
		t.Fatalf("expected table users, got %q", m.tableRenameForm.Table())
	}

	m.tableRenameForm.field.SetValue("accounts")
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected async rename command")
	}

	msg := cmd().(schemaResultMsg)
	if msg.err != nil {
		t.Fatalf("rename failed: %v", msg.err)
	}
	if msg.newTable != "accounts" {
		t.Fatalf("newTable = %q, want accounts", msg.newTable)
	}

	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.tableRenameForm.IsVisible() {
		t.Fatal("form should close after success")
	}
	found := false
	for _, name := range m.tables {
		if name == "accounts" {
			found = true
		}
		if name == "users" {
			t.Fatal("old table name still in sidebar list")
		}
	}
	if !found {
		t.Fatalf("expected accounts in tables, got %v", m.tables)
	}

	tables, err := sqlite.DB().Tables()
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	found = false
	for _, name := range tables {
		if name == "accounts" {
			found = true
		}
		if name == "users" {
			t.Fatal("old table name still in database")
		}
	}
	if !found {
		t.Fatalf("expected accounts in database, got %v", tables)
	}
}

func TestReplaceSimpleSelectTable(t *testing.T) {
	tests := []struct {
		query   string
		oldName string
		newName string
		want    string
	}{
		{
			query:   "SELECT * FROM users LIMIT 100;",
			oldName: "users",
			newName: "accounts",
			want:    "SELECT * FROM accounts LIMIT 100;",
		},
		{
			query:   `SELECT * FROM "users" LIMIT 100;`,
			oldName: "users",
			newName: "accounts",
			want:    `SELECT * FROM "accounts" LIMIT 100;`,
		},
		{
			query:   "SELECT * FROM orders LIMIT 100;",
			oldName: "users",
			newName: "accounts",
			want:    "SELECT * FROM orders LIMIT 100;",
		},
	}

	for _, tc := range tests {
		got := replaceSimpleSelectTable(tc.query, tc.oldName, tc.newName)
		if got != tc.want {
			t.Errorf("replaceSimpleSelectTable(%q) = %q, want %q", tc.query, got, tc.want)
		}
	}
}

func TestTableRenameIgnoresColumnRows(t *testing.T) {
	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.tables = []string{"users"}
	m.expanded["users"] = []db.Column{{Name: "id", Type: "INTEGER"}}
	m.sidebarCursor = 1 // column row under expanded users

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if m.tableRenameForm.IsVisible() {
		t.Fatal("rename form should not open on a column row")
	}
	if cmd != nil {
		t.Fatal("expected no command for column row")
	}
}
