package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/bookmarks"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
	"github.com/ruben/gsql/internal/history"
)

// confirmDestructive() defaults to true (safe) when the setting is unset, and
// honours an explicit on/off value.
func TestConfirmDestructiveSetting(t *testing.T) {
	m := NewModel(&config.Config{})
	if !m.confirmDestructive() {
		t.Error("nil ConfirmDestructive should confirm (default safe)")
	}

	on := true
	m.settings.ConfirmDestructive = &on
	if !m.confirmDestructive() {
		t.Error("ConfirmDestructive=true should confirm")
	}

	off := false
	m.settings.ConfirmDestructive = &off
	if m.confirmDestructive() {
		t.Error("ConfirmDestructive=false should skip confirmation")
	}
}

// newConfirmTestModel builds a workspace model backed by a sqlite database
// with a single populated "users" table, plus the given confirm_destructive
// setting.
func newConfirmTestModel(t *testing.T, confirm *bool) (Model, *db.Connection, string) {
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
	if _, err := sqlite.DB().Exec(`INSERT INTO users (id, name) VALUES (1, 'alice'), (2, 'bob')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	m := NewModel(&config.Config{Settings: config.Settings{ConfirmDestructive: confirm}})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = sqlite
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.tables = []string{"users"}
	return m, sqlite, dbPath
}

// With confirm_destructive off, pressing T truncates immediately — no dialog,
// and the returned command is the async truncate.
func TestTruncateSkipsConfirmWhenDisabled(t *testing.T) {
	off := false
	m, sqlite, _ := newConfirmTestModel(t, &off)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = updated.(Model)
	if m.truncateConfirm != "" {
		t.Fatalf("expected no confirm dialog, got %q", m.truncateConfirm)
	}
	if cmd == nil {
		t.Fatal("expected immediate truncate command")
	}
	msg := cmd().(truncateResultMsg)
	if msg.err != nil {
		t.Fatalf("truncate failed: %v", msg.err)
	}

	result, err := sqlite.DB().Execute(`SELECT COUNT(*) FROM users`)
	if err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if result.Rows[0][0] != "0" {
		t.Fatalf("expected empty table after skip-confirm truncate, got %v", result.Rows)
	}
}

// With confirm_destructive on (default), T opens the typed-name dialog and
// runs nothing yet.
func TestTruncateConfirmsByDefault(t *testing.T) {
	m, _, _ := newConfirmTestModel(t, nil)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m = updated.(Model)
	if m.truncateConfirm != "users" {
		t.Fatalf("expected confirm dialog staged, got %q", m.truncateConfirm)
	}
	if cmd != nil {
		t.Fatal("expected no immediate command while confirming")
	}
}

// With confirm_destructive off, pressing D drops the table immediately via the
// shared execDropTable helper — no typed-name dialog.
func TestDropTableSkipsConfirmWhenDisabled(t *testing.T) {
	off := false
	m, sqlite, _ := newConfirmTestModel(t, &off)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = updated.(Model)
	if m.dropTableConfirm != "" || m.dropTableInput != "" {
		t.Fatalf("expected no confirm dialog, got %q/%q", m.dropTableConfirm, m.dropTableInput)
	}
	if cmd == nil {
		t.Fatal("expected immediate drop command")
	}
	msg := cmd().(schemaResultMsg)
	if msg.err != nil {
		t.Fatalf("drop table failed: %v", msg.err)
	}
	if msg.action != db.SchemaDropTable {
		t.Fatalf("action = %v, want SchemaDropTable", msg.action)
	}

	tables, err := sqlite.DB().Tables()
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	for _, name := range tables {
		if name == "users" {
			t.Fatal("expected users table dropped, still present")
		}
	}
}

// Clearing history and bookmarks runs inline (no async command) when
// confirmation is disabled. The stores are emptied and the confirm flags are
// never raised.
func TestClearHistoryAndBookmarksSkipConfirm(t *testing.T) {
	off := false
	m, _, _ := newConfirmTestModel(t, &off)
	connName := m.connection.Config().Name
	m.historyStore = history.NewStore(t.TempDir())
	m.bookmarkStore = bookmarks.NewStore(t.TempDir())

	// Seed both stores for the connection name.
	if err := m.historyStore.Record(connName, "SELECT 1", 0, true); err != nil {
		t.Fatalf("record history: %v", err)
	}
	if err := m.bookmarkStore.Add(connName, "SELECT 2"); err != nil {
		t.Fatalf("add bookmark: %v", err)
	}

	// History panel visible: pressing D clears without prompting.
	m.history.Toggle()
	if !m.history.IsVisible() {
		t.Fatal("history panel should be visible")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = updated.(Model)
	if m.clearHistoryConfirm {
		t.Fatal("expected no confirm flag, dialog was staged")
	}
	if cmd != nil {
		t.Fatal("expected no async command for inline clear")
	}
	if got, _ := m.historyStore.Get(connName); len(got) != 0 {
		t.Fatalf("history not cleared, got %d records", len(got))
	}

	// Bookmarks panel visible: pressing D clears without prompting. Hide the
	// history panel first so it doesn't intercept the same key.
	m.history.Toggle()
	m.bookmarks.Toggle()
	if !m.bookmarks.IsVisible() {
		t.Fatal("bookmarks panel should be visible")
	}
	// Reseed bookmarks since history clear path doesn't touch them, but keep
	// the assertion self-contained.
	if err := m.bookmarkStore.Add(connName, "SELECT 3"); err != nil {
		t.Fatalf("re-add bookmark: %v", err)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = updated.(Model)
	if m.clearBookmarksConfirm {
		t.Fatal("expected no confirm flag, dialog was staged")
	}
	if cmd != nil {
		t.Fatal("expected no async command for inline clear")
	}
	if got, _ := m.bookmarkStore.Get(connName); len(got) != 0 {
		t.Fatalf("bookmarks not cleared, got %d entries", len(got))
	}
}
