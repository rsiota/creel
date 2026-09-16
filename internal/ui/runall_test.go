package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func newRunAllTestModel(t *testing.T) (Model, *db.Connection) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(dir, "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.DB().Execute(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state = stateWorkspace
	m.connection = conn
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	return m, conn
}

func drainQueryCmd(t *testing.T, m Model, cmd tea.Cmd) (Model, queryExecutedMsg) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command")
	}
	msg := cmd()
	// tea.Batch returns a batchMsg; unwrap until we find queryExecutedMsg.
	for {
		switch v := msg.(type) {
		case queryExecutedMsg:
			updated, _ := m.Update(v)
			return updated.(Model), v
		case tea.BatchMsg:
			var found queryExecutedMsg
			var ok bool
			for _, c := range v {
				inner := c()
				if q, is := inner.(queryExecutedMsg); is {
					found = q
					ok = true
					break
				}
			}
			if !ok {
				t.Fatalf("batch had no queryExecutedMsg: %#v", v)
			}
			updated, _ := m.Update(found)
			return updated.(Model), found
		default:
			t.Fatalf("unexpected msg type %T", msg)
			return m, queryExecutedMsg{}
		}
	}
}

func TestRunAllExecutesAllStatements(t *testing.T) {
	m, conn := newRunAllTestModel(t)
	m.editor.SetValue("INSERT INTO items (id, name) VALUES (1, 'a');\nINSERT INTO items (id, name) VALUES (2, 'b');\nSELECT name FROM items ORDER BY id;")

	m, msg := drainQueryCmd(t, m, m.exRunAll())
	if msg.err != nil {
		t.Fatalf("err: %v", msg.err)
	}
	if msg.multiTotal != 3 || msg.multiRan != 3 {
		t.Fatalf("multi = %d/%d, want 3/3", msg.multiRan, msg.multiTotal)
	}
	if !strings.Contains(m.schemaMsg, "ran 3 statements") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
	res, err := conn.DB().Execute(`SELECT count(*) FROM items`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0][0] != "2" {
		t.Fatalf("row count = %v", res.Rows[0])
	}
	if m.results.NumRows() != 2 {
		t.Fatalf("grid rows = %d, want 2", m.results.NumRows())
	}
}

func TestRunAllStopsOnError(t *testing.T) {
	m, conn := newRunAllTestModel(t)
	m.editor.SetValue("INSERT INTO items (id, name) VALUES (1, 'a');\nINSERT INTO nope VALUES (1);\nINSERT INTO items (id, name) VALUES (2, 'b');")

	m, msg := drainQueryCmd(t, m, m.exRunAll())
	if msg.err == nil {
		t.Fatal("expected error")
	}
	if msg.multiFail != 2 || msg.multiRan != 1 {
		t.Fatalf("fail=%d ran=%d, want fail=2 ran=1", msg.multiFail, msg.multiRan)
	}
	if !strings.Contains(m.results.Message(), "statement 2/3") {
		t.Fatalf("error = %q", m.results.Message())
	}
	res, err := conn.DB().Execute(`SELECT count(*) FROM items`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0][0] != "1" {
		t.Fatalf("should have stopped after first insert: count=%v", res.Rows[0])
	}
}

func TestSourceFileRunsWithoutReplacingEditor(t *testing.T) {
	m, conn := newRunAllTestModel(t)
	m.editor.SetValue("-- keep me")
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sql")
	if err := os.WriteFile(path, []byte("INSERT INTO items (id, name) VALUES (1, 'from-file');\nSELECT name FROM items;"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, msg := drainQueryCmd(t, m, m.exSource([]string{path}))
	if msg.err != nil {
		t.Fatalf("err: %v", msg.err)
	}
	if m.editor.Value() != "-- keep me" {
		t.Fatalf("editor was replaced: %q", m.editor.Value())
	}
	res, err := conn.DB().Execute(`SELECT name FROM items`)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0][0] != "from-file" {
		t.Fatalf("rows = %v", res.Rows)
	}
}
