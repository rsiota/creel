package ui

import (
	"strings"
	"testing"
)

func TestExDiagnoseNotConnected(t *testing.T) {
	m := &Model{}
	if cmd := m.runExCommand("diagnose"); cmd != nil {
		t.Fatal("expected nil")
	}
	if m.schemaMsg != "not connected" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExDiagnoseNoStatement(t *testing.T) {
	m := &Model{connection: newSQLiteTestConn(t), editor: NewQueryEditor()}
	if cmd := m.runExCommand("diag"); cmd != nil {
		t.Fatal("expected nil")
	}
	if !strings.Contains(m.schemaMsg, "no statement") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExDiagnoseSQLiteScan(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users VALUES (1,'a'),(2,'b')`); err != nil {
		t.Fatal(err)
	}
	m := &Model{connection: conn, editor: NewQueryEditor()}
	m.editor.SetValue("SELECT * FROM users WHERE name = 'a'")
	cmd := m.runExCommand("diagnose")
	if cmd == nil {
		t.Fatalf("expected cmd, schemaMsg=%q", m.schemaMsg)
	}
	msg, ok := cmd().(diagnoseResultMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if len(msg.result.Rows) == 0 {
		t.Fatal("expected findings")
	}
	joined := ""
	for _, row := range msg.result.Rows {
		joined += strings.Join(row, " ") + "\n"
	}
	if !strings.Contains(joined, "users") && !strings.Contains(joined, "No obvious") {
		t.Fatalf("unexpected findings:\n%s", joined)
	}
	if msg.query == "" || msg.planText == "" {
		t.Fatalf("expected cached plan fields on msg")
	}
}
