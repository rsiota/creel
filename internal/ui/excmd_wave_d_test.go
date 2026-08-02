package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/version"
)

func TestExNew(t *testing.T) {
	m := &Model{editor: NewQueryEditor()}
	m.editor.SetValue("SELECT 1;")
	m.runExCommand("new")
	if m.editor.Value() != "" {
		t.Errorf(":new left editor %q", m.editor.Value())
	}
	if !strings.Contains(m.schemaMsg, "new buffer") {
		t.Errorf(":new msg -> %q", m.schemaMsg)
	}
}

func TestExVersion(t *testing.T) {
	m := &Model{}
	m.runExCommand("version")
	want := version.String()
	if m.schemaMsg != want {
		t.Errorf(":version -> %q, want %q", m.schemaMsg, want)
	}
	if !strings.HasPrefix(m.schemaMsg, "creel ") && m.schemaMsg != "creel (devel)" && m.schemaMsg != "creel (unknown)" {
		t.Errorf(":version unexpected form %q", m.schemaMsg)
	}
}

func TestExPlanAlias(t *testing.T) {
	p := exLookup("plan")
	e := exLookup("explain")
	if p == nil || e == nil {
		t.Fatal("exLookup(plan/explain) = nil")
	}
	if p.verbs[0] != e.verbs[0] || p.verbs[0] != "explain" {
		t.Errorf("plan/explain canonical = %q / %q, want explain", p.verbs[0], e.verbs[0])
	}
}

func TestTouchRecentTable(t *testing.T) {
	m := &Model{}
	m.touchRecentTable("users")
	m.touchRecentTable("orders")
	m.touchRecentTable("users") // bump to front
	if len(m.recentTables) != 2 || m.recentTables[0] != "users" || m.recentTables[1] != "orders" {
		t.Errorf("MRU = %v, want [users orders]", m.recentTables)
	}
}

func TestExRecent(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()

	t.Run("not connected", func(t *testing.T) {
		m := &Model{}
		m.runExCommand("recent")
		if !strings.Contains(m.schemaMsg, "not connected") {
			t.Errorf(":recent -> %q", m.schemaMsg)
		}
	})
	t.Run("empty", func(t *testing.T) {
		m := &Model{connection: conn, tables: []string{"users"}}
		m.runExCommand("recent")
		if !strings.Contains(m.schemaMsg, "no recent") {
			t.Errorf(":recent empty -> %q", m.schemaMsg)
		}
	})
	t.Run("lists and opens by rank", func(t *testing.T) {
		m := &Model{
			connection:   conn,
			tables:       []string{"users", "orders"},
			recentTables: []string{"orders", "users"},
			editor:       NewQueryEditor(),
			results:      NewResultsTable(),
		}
		cmd := m.runExCommand("recent")
		if cmd == nil {
			t.Fatalf(":recent list -> %q", m.schemaMsg)
		}
		lrm, ok := cmd().(lookupResultMsg)
		if !ok {
			t.Fatalf("expected lookupResultMsg")
		}
		if len(lrm.result.Rows) != 2 || lrm.result.Rows[0][0] != "orders" {
			t.Errorf("recent rows = %v", lrm.result.Rows)
		}

		cmd = m.runExCommand("recent 1")
		if cmd == nil {
			t.Fatalf(":recent 1 -> %q", m.schemaMsg)
		}
		if !strings.Contains(m.editor.Value(), "SELECT * FROM orders") {
			t.Errorf("editor = %q, want orders", m.editor.Value())
		}
	})
	t.Run("drops gone tables", func(t *testing.T) {
		m := &Model{
			connection:   conn,
			tables:       []string{"users"},
			recentTables: []string{"gone", "users"},
		}
		got := m.liveRecentTables()
		if len(got) != 1 || got[0] != "users" {
			t.Errorf("liveRecentTables = %v, want [users]", got)
		}
	})
}

func TestOpenTableRecordsRecent(t *testing.T) {
	m := &Model{
		tables:  []string{"users"},
		editor:  NewQueryEditor(),
		results: NewResultsTable(),
	}
	m.openTable("users")
	if len(m.recentTables) != 1 || m.recentTables[0] != "users" {
		t.Errorf("after openTable, recent = %v", m.recentTables)
	}
	if !strings.Contains(m.editor.Value(), "SELECT * FROM users") {
		t.Errorf("editor = %q", m.editor.Value())
	}
}

func TestVersionString(t *testing.T) {
	s := version.String()
	if !strings.HasPrefix(s, "creel") {
		t.Errorf("version.String() = %q", s)
	}
}
