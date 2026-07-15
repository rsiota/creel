package ui

import (
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/db"
)

func TestExRefsNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("refs users")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":refs not connected -> %q", m.schemaMsg)
	}
}

func TestExRefsNoArgNoCurrentTable(t *testing.T) {
	m := &Model{connection: &db.Connection{}, results: NewResultsTable()}
	m.runExCommand("refs")
	if !strings.Contains(m.schemaMsg, "needs a table name") {
		t.Errorf(":refs (no arg, no current table) -> %q", m.schemaMsg)
	}
}

func TestExRefsNoSuchTable(t *testing.T) {
	m := &Model{
		connection: &db.Connection{},
		results:    NewResultsTable(),
		tables:     []string{"users"},
	}
	m.runExCommand("refs boggy")
	if !strings.Contains(m.schemaMsg, "no such table") {
		t.Errorf(":refs boggy -> %q", m.schemaMsg)
	}
}

func TestExRefsLive(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE departments (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE employees (id INTEGER PRIMARY KEY, dept_id INTEGER, FOREIGN KEY (dept_id) REFERENCES departments(id))`,
		`CREATE TABLE budgets (id INTEGER PRIMARY KEY, dept_id INTEGER, FOREIGN KEY (dept_id) REFERENCES departments(id))`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	m := &Model{
		connection: conn,
		results:    NewResultsTable(),
		editor:     NewQueryEditor(),
		tables:     []string{"departments", "employees", "budgets"},
	}

	// :refs departments → async fetch → refsResultMsg with both referrers.
	cmd := m.runExCommand("refs departments")
	if cmd == nil {
		t.Fatalf(":refs returned nil cmd: %q", m.schemaMsg)
	}
	rr, ok := cmd().(refsResultMsg)
	if !ok {
		t.Fatalf("expected refsResultMsg, got %T", cmd())
	}
	if rr.err != nil {
		t.Fatalf("refs fetch error: %v", rr.err)
	}
	if len(rr.refs) != 2 {
		t.Fatalf("expected 2 referrers, got %d: %+v", len(rr.refs), rr.refs)
	}
	seen := map[string]bool{}
	for _, r := range rr.refs {
		seen[r.Table] = true
	}
	if !seen["employees"] || !seen["budgets"] {
		t.Errorf("expected employees+budgets, got %v", seen)
	}

	// Handler path: opening the panel makes it visible with the rows.
	m.refsPanel.Show(rr.table, rr.refs)
	if !m.refsPanel.IsVisible() {
		t.Fatal("refs panel should be visible after Show")
	}

	// Default-to-current-table: no arg, with the results source set.
	m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
	m.results.SetEditable("departments", []string{"id"})
	if cmd2 := m.runExCommand("refs"); cmd2 == nil {
		t.Fatalf(":refs (no arg, current=departments) returned nil: %q", m.schemaMsg)
	}
}

func TestRefsPanelEmpty(t *testing.T) {
	var p RefsPanel
	p.Show("users", nil)
	if !p.IsVisible() {
		t.Fatal("panel should be visible")
	}
	// lines() yields a single explanatory line for the empty case.
	if got := len(p.lines()); got != 1 {
		t.Fatalf("empty refs panel lines = %d, want 1", got)
	}
}
