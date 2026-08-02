package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestExOpenStructureTab(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}

	m := &Model{
		connection: conn,
		tables:     []string{"users"},
		editor:     NewQueryEditor(),
		results:    NewResultsTable(),
		width:      120,
		height:     40,
	}

	cases := []struct {
		cmd  string
		want int
	}{
		{"columns users", seTabColumns},
		{"indexes users", seTabIndexes},
		{"fk users", seTabFK},
		{"constraints users", seTabChecks},
		{"describe users", seTabColumns},
	}
	for _, c := range cases {
		m.schemaEditor.Hide()
		cmd := m.runExCommand(c.cmd)
		if !m.schemaEditor.IsVisible() {
			t.Fatalf("%s: structure panel not open (%q)", c.cmd, m.schemaMsg)
		}
		if got := m.schemaEditor.ActiveTab(); got != c.want {
			t.Errorf("%s: active tab = %d, want %d", c.cmd, got, c.want)
		}
		// Drain the async structure load cmd so it doesn't leak.
		if cmd != nil {
			_ = cmd()
		}
	}
}

func TestExTablesAndViews(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.DB().Exec(`CREATE VIEW active AS SELECT * FROM users`); err != nil {
		t.Fatal(err)
	}

	m := &Model{
		connection: conn,
		tables:     []string{"users", "active"},
		editor:     NewQueryEditor(),
		results:    NewResultsTable(),
	}

	cmd := m.runExCommand("tables")
	if cmd == nil {
		t.Fatalf(":tables -> %q", m.schemaMsg)
	}
	msg := cmd()
	lrm, ok := msg.(lookupResultMsg)
	if !ok {
		t.Fatalf("expected lookupResultMsg, got %T", msg)
	}
	if len(lrm.result.Rows) != 1 || lrm.result.Rows[0][0] != "users" {
		t.Errorf(":tables rows = %v, want [[users]]", lrm.result.Rows)
	}

	cmd = m.runExCommand("dv")
	if cmd == nil {
		t.Fatalf(":dv -> %q", m.schemaMsg)
	}
	msg = cmd()
	lrm, ok = msg.(lookupResultMsg)
	if !ok {
		t.Fatalf("expected lookupResultMsg, got %T", msg)
	}
	if len(lrm.result.Rows) != 1 || lrm.result.Rows[0][0] != "active" {
		t.Errorf(":views rows = %v, want [[active]]", lrm.result.Rows)
	}
}

func TestExTablesNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("tables")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":tables -> %q", m.schemaMsg)
	}
}

func TestExSchemasListSQLite(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{connection: conn}
	m.runExCommand("schemas")
	if !strings.Contains(m.schemaMsg, "not supported") {
		t.Errorf(":schemas on sqlite -> %q", m.schemaMsg)
	}
}

func TestExSearch(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{
		connection: conn,
		tables:     []string{"users", "orders"},
		columnCache: map[string][]db.Column{
			"users":  {{Name: "id"}, {Name: "email"}},
			"orders": {{Name: "id"}, {Name: "user_id"}},
		},
	}

	t.Run("missing arg", func(t *testing.T) {
		m.runExCommand("search")
		if !strings.Contains(m.schemaMsg, "needs a name") {
			t.Errorf(":search bare -> %q", m.schemaMsg)
		}
	})
	t.Run("no match", func(t *testing.T) {
		m.runExCommand("find zzz")
		if !strings.Contains(m.schemaMsg, "no matches") {
			t.Errorf(":find zzz -> %q", m.schemaMsg)
		}
	})
	t.Run("matches table and column", func(t *testing.T) {
		cmd := m.runExCommand("search user")
		if cmd == nil {
			t.Fatalf(":search user -> %q", m.schemaMsg)
		}
		msg := cmd()
		lrm, ok := msg.(lookupResultMsg)
		if !ok {
			t.Fatalf("expected lookupResultMsg, got %T", msg)
		}
		if len(lrm.result.Rows) < 2 {
			t.Fatalf("expected multiple hits, got %v", lrm.result.Rows)
		}
		// Should include the users table and user_id column.
		foundTable, foundCol := false, false
		for _, row := range lrm.result.Rows {
			if row[0] == "table" && row[1] == "users" {
				foundTable = true
			}
			if row[0] == "column" && row[1] == "user_id" {
				foundCol = true
			}
		}
		if !foundTable || !foundCol {
			t.Errorf("hits = %v, want users table and user_id column", lrm.result.Rows)
		}
	})
}
