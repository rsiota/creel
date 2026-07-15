package ui

import (
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/db"
)

func TestExUsesLive(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE superusers (id INTEGER)`,
		`CREATE VIEW active_users AS SELECT * FROM users`,                  // matches
		`CREATE VIEW v_super AS SELECT * FROM superusers`,                  // must NOT match (substring guard)
		`CREATE TRIGGER trg_ins AFTER INSERT ON users BEGIN SELECT 1; END`, // matches
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	m := &Model{
		connection: conn,
		results:    NewResultsTable(),
		editor:     NewQueryEditor(),
		tables:     []string{"users", "superusers"},
	}

	cmd := m.runExCommand("uses users")
	if cmd == nil {
		t.Fatalf(":uses returned nil cmd: %q", m.schemaMsg)
	}
	msg, ok := cmd().(lookupResultMsg)
	if !ok {
		t.Fatalf("expected lookupResultMsg, got %T", cmd())
	}
	if msg.err != nil {
		t.Fatalf("uses fetch error: %v", msg.err)
	}
	if !strings.Contains(msg.title, "users") {
		t.Errorf("title %q should mention users", msg.title)
	}
	// Result columns are [Type, Name]; row[0]=kind, row[1]=name.
	names := map[string]string{} // name -> kind
	for _, row := range msg.result.Rows {
		if len(row) < 2 {
			t.Fatalf("malformed row: %v", row)
		}
		names[row[1]] = row[0]
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 usages (active_users, trg_ins), got %d: %+v", len(names), names)
	}
	if names["active_users"] != "view" {
		t.Errorf("active_users: want view, got %q", names["active_users"])
	}
	if names["trg_ins"] != "trigger" {
		t.Errorf("trg_ins: want trigger, got %q", names["trg_ins"])
	}
	if _, ok := names["v_super"]; ok {
		t.Error("v_super matched 'users' — word-boundary guard failed")
	}

	// Handler path: opening the panel makes it visible.
	m.lookupPanel.Show(msg.title, msg.result)
	if !m.lookupPanel.IsVisible() {
		t.Fatal("lookup panel should be visible after Show")
	}
}

func TestExUsesNoCurrentTable(t *testing.T) {
	m := &Model{connection: &db.Connection{}, results: NewResultsTable()}
	m.runExCommand("uses")
	if !strings.Contains(m.schemaMsg, "no current table") {
		t.Errorf(":uses (no arg) -> %q", m.schemaMsg)
	}
}
