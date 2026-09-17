package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestExplainAnalyzeSQLiteRejected(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := Model{
		connection: conn,
		editor:     NewQueryEditor(),
		state:      stateWorkspace,
	}
	m.editor.SetValue("SELECT 1")
	cmd := m.explainQueryAnalyze()
	if cmd != nil {
		t.Fatal("SQLite should not start ANALYZE")
	}
	if !strings.Contains(m.schemaMsg, "not supported on SQLite") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExplainAnalyzeReadOnlyBlocksWrites(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.New(db.ConnectionConfig{
		Driver:   db.DriverPostgres, // driver checked before connect for ANALYZE gate
		Database: "x",
		ReadOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Don't Connect — we only need Config() for the gate.
	_ = dir
	m := Model{
		connection:    conn,
		editor:        NewQueryEditor(),
		forceReadOnly: true,
		state:         stateWorkspace,
	}
	m.editor.SetValue("DELETE FROM users")
	cmd := m.explainQueryAnalyze()
	if cmd != nil {
		t.Fatal("read-only write ANALYZE should not run")
	}
	if !strings.Contains(m.schemaMsg, "read-only") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExplainAnalyzeStagesConfirm(t *testing.T) {
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverMySQL, Database: "app", Host: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	m := Model{
		connection: conn,
		editor:     NewQueryEditor(),
		state:      stateWorkspace,
		settings:   config.Settings{}, // confirm_destructive default on
	}
	m.editor.SetValue("SELECT 1")
	cmd := m.explainQueryAnalyze()
	if cmd != nil {
		t.Fatal("should stage confirm, not return a cmd")
	}
	if !m.explainAnalyzeConfirm {
		t.Fatal("expected explainAnalyzeConfirm")
	}

	updated, cmd := m.Update(runeKey('n'))
	mm := updated.(Model)
	if mm.explainAnalyzeConfirm {
		t.Fatal("n should cancel confirm")
	}
	if cmd != nil {
		t.Fatal("cancel should not run ANALYZE")
	}
}

func TestExplainAnalyzeSkipsConfirmWhenDisabled(t *testing.T) {
	off := false
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverPostgres, Database: "app", Host: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	m := Model{
		connection: conn,
		editor:     NewQueryEditor(),
		state:      stateWorkspace,
		settings:   config.Settings{ConfirmDestructive: &off},
	}
	m.editor.SetValue("SELECT 1")
	cmd := m.explainQueryAnalyze()
	if cmd == nil {
		t.Fatal("expected ANALYZE cmd when confirm is off")
	}
	if m.explainAnalyzeConfirm {
		t.Fatal("should not stage confirm")
	}
}

func TestExExplainBangTriggersAnalyze(t *testing.T) {
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverMySQL, Database: "app", Host: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	m := Model{
		connection: conn,
		editor:     NewQueryEditor(),
		state:      stateWorkspace,
	}
	m.editor.SetValue("SELECT 1")
	m.runExCommand("explain!")
	if !m.explainAnalyzeConfirm {
		t.Fatal(":explain! should stage ANALYZE confirm")
	}
}

func TestBuildExplainAnalyzeSQL(t *testing.T) {
	// Unit-check the SQL prefix via explainQueryOpts against a fake-connected
	// model by inspecting the command's message — requires Execute to fail
	// without a server, which still returns explainResultMsg with analyze set.
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	m := Model{connection: conn, editor: NewQueryEditor(), state: stateWorkspace}
	m.editor.SetValue("SELECT 1")
	// Force analyze path on SQLite — opts falls back to QUERY PLAN and clears analyze.
	msg := m.explainQueryOpts(false, "", true)().(explainResultMsg)
	if msg.err != nil {
		t.Fatalf("sqlite fallback explain failed: %v", msg.err)
	}
	if msg.analyze {
		t.Fatal("sqlite ANALYZE fallback should clear analyze flag")
	}
}
