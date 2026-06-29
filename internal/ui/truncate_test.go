package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

func TestBuildTruncateQuery(t *testing.T) {
	tests := []struct {
		driver db.Driver
		table  string
		want   string
	}{
		{db.DriverSQLite, "users", "DELETE FROM users"},
		{db.DriverMySQL, "users", "TRUNCATE TABLE users"},
	}
	for _, tc := range tests {
		got := buildTruncateQuery(tc.driver, tc.table)
		if got != tc.want {
			t.Errorf("buildTruncateQuery(%q, %q) = %q, want %q", tc.driver, tc.table, got, tc.want)
		}
	}
}

func TestTruncateTableFlow(t *testing.T) {
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
	if _, err := sqlite.DB().Exec(`INSERT INTO users (id, name) VALUES (1, 'alice'), (2, 'bob')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = sqlite
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.tables = []string{"users"}

	// Press T on the selected table — should open confirmation.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = updated.(Model)
	if m.truncateConfirm != "users" {
		t.Fatalf("expected truncateConfirm=users, got %q", m.truncateConfirm)
	}

	// Cancel with esc.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.truncateConfirm != "" {
		t.Fatalf("expected truncateConfirm cleared after cancel, got %q", m.truncateConfirm)
	}

	// Confirm truncate.
	m.truncateConfirm = "users"
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.truncateConfirm != "" {
		t.Fatalf("expected truncateConfirm cleared after confirm, got %q", m.truncateConfirm)
	}
	if cmd == nil {
		t.Fatal("expected async truncate command")
	}

	msg := cmd().(truncateResultMsg)
	if msg.err != nil {
		t.Fatalf("truncate failed: %v", msg.err)
	}
	if msg.table != "users" {
		t.Fatalf("truncate table = %q, want users", msg.table)
	}

	result, err := sqlite.DB().Execute(`SELECT COUNT(*) FROM users`)
	if err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "0" {
		t.Fatalf("expected empty table, got %v", result.Rows)
	}

	// Result handler should show success message.
	m.lastQuery = "SELECT * FROM users LIMIT 100"
	m.baseQuery = m.lastQuery
	updated, cmd = m.Update(msg)
	m = updated.(Model)
	if !strings.Contains(m.truncateMsg, "truncated users") {
		t.Fatalf("truncateMsg = %q, want success message", m.truncateMsg)
	}
	if cmd == nil {
		t.Fatal("expected page refresh after truncating visible table")
	}
}

func TestTruncateIgnoresColumnRows(t *testing.T) {
	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.tables = []string{"users"}
	m.expanded["users"] = []db.Column{{Name: "id", Type: "INTEGER"}}
	m.sidebarCursor = 1 // column row under expanded users

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = updated.(Model)
	if m.truncateConfirm != "" {
		t.Fatalf("expected no truncate confirm on column row, got %q", m.truncateConfirm)
	}
}
