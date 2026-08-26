package ui

import (
	"strings"
	"testing"
)

func TestExSetNull(t *testing.T) {
	setup := func() *Model {
		m := &Model{results: NewResultsTable()}
		m.results.SetResult(
			[]string{"id", "created_at", "note"},
			[][]string{{"1", "2026-01-01 12:00:00", "hello"}},
			"1 row",
		)
		m.results.SetColumnTypes(map[string]string{
			"id":         "INTEGER",
			"created_at": "DATETIME",
			"note":       "TEXT",
		})
		m.results.SetEditable("events", []string{"id"})
		m.connection = newSQLiteTestConn(t)
		return m
	}

	t.Run("stages NULL on cursor cell", func(t *testing.T) {
		m := setup()
		m.results.SetCursor(0, 1)
		m.runExCommand("setnull")
		if got := m.results.RowValue(0, 1); got != "NULL" {
			t.Fatalf("cell = %q, want NULL", got)
		}
		if !strings.Contains(m.schemaMsg, "staged NULL on created_at") {
			t.Errorf("message = %q", m.schemaMsg)
		}
	})

	t.Run("named column on cursor row", func(t *testing.T) {
		m := setup()
		m.results.SetCursor(0, 0)
		m.runExCommand("setnull note")
		if got := m.results.RowValue(0, 2); got != "NULL" {
			t.Fatalf("note = %q, want NULL", got)
		}
		if !strings.Contains(m.schemaMsg, "staged NULL on note") {
			t.Errorf("message = %q", m.schemaMsg)
		}
	})

	t.Run("already NULL", func(t *testing.T) {
		m := setup()
		m.results.SetResult(
			[]string{"id", "created_at"},
			[][]string{{"1", "NULL"}},
			"1 row",
		)
		m.results.SetColumnTypes(map[string]string{
			"id":         "INTEGER",
			"created_at": "DATETIME",
		})
		m.results.SetEditable("events", []string{"id"})
		m.results.SetCursor(0, 1)
		m.runExCommand("setnull")
		if !strings.Contains(m.schemaMsg, "already NULL") {
			t.Errorf("message = %q", m.schemaMsg)
		}
		if m.results.HasDirtyCells() {
			t.Error("should not stage a dirty cell when already NULL")
		}
	})

	t.Run("primary key rejected", func(t *testing.T) {
		m := setup()
		m.results.SetCursor(0, 0)
		m.runExCommand("setnull id")
		if !strings.Contains(m.schemaMsg, "primary key") {
			t.Errorf("message = %q", m.schemaMsg)
		}
	})

	t.Run("unknown column", func(t *testing.T) {
		m := setup()
		m.runExCommand("setnull missing")
		if !strings.Contains(m.schemaMsg, "no such column") {
			t.Errorf("message = %q", m.schemaMsg)
		}
	})

	t.Run("not editable", func(t *testing.T) {
		m := &Model{results: NewResultsTable(), connection: newSQLiteTestConn(t)}
		m.results.SetResult(
			[]string{"id"},
			[][]string{{"1"}},
			"1 row",
		)
		m.runExCommand("setnull")
		if !strings.Contains(m.schemaMsg, "not editable") {
			t.Errorf("message = %q", m.schemaMsg)
		}
	})
}

func TestExSetNullSave(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Exec(`CREATE TABLE events (
		id INTEGER PRIMARY KEY,
		created_at DATETIME
	)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := conn.DB().Exec(`INSERT INTO events (id, created_at) VALUES (1, '2026-01-01 12:00:00')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	m := &Model{results: NewResultsTable(), connection: conn}
	m.results.SetResult(
		[]string{"id", "created_at"},
		[][]string{{"1", "2026-01-01 12:00:00"}},
		"1 row",
	)
	m.results.SetColumnTypes(map[string]string{
		"id":         "INTEGER",
		"created_at": "DATETIME",
	})
	m.results.SetEditable("events", []string{"id"})
	m.results.SetCursor(0, 1)

	m.runExCommand("setnull")
	cmd := m.saveEdits()
	if cmd == nil {
		t.Fatal("saveEdits returned nil")
	}
	msg := cmd().(saveResultMsg)
	if msg.err != nil {
		t.Fatalf("save failed: %v", msg.err)
	}

	res, err := conn.DB().Execute(`SELECT created_at FROM events WHERE id = 1`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0][0] != "NULL" {
		t.Fatalf("created_at = %#v, want NULL", res.Rows)
	}
}
