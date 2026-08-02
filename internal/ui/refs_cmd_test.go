package ui

import (
	"strings"
	"testing"

	"github.com/ruben/creel/internal/db"
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
	if !strings.Contains(m.schemaMsg, "no current table") {
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

	cmd := m.runExCommand("refs departments")
	if cmd == nil {
		t.Fatalf(":refs returned nil cmd: %q", m.schemaMsg)
	}
	msg, ok := cmd().(lookupResultMsg)
	if !ok {
		t.Fatalf("expected lookupResultMsg, got %T", cmd())
	}
	if msg.err != nil {
		t.Fatalf("refs fetch error: %v", msg.err)
	}
	if !strings.Contains(msg.title, "departments") {
		t.Errorf("title %q should mention departments", msg.title)
	}
	if len(msg.result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(msg.result.Rows), msg.result.Rows)
	}
	refTables := map[string]bool{}
	for _, row := range msg.result.Rows {
		refTables[row[0]] = true // first column = child table
	}
	if !refTables["employees"] || !refTables["budgets"] {
		t.Errorf("expected employees+budgets, got %v", refTables)
	}

	// Handler path: opening the panel makes it visible with the rows.
	m.lookupPanel.Show(msg.title, msg.result)
	if !m.lookupPanel.IsVisible() {
		t.Fatal("lookup panel should be visible after Show")
	}

	// Default-to-current-table: no arg, with the results source set.
	m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
	m.results.SetEditable("departments", []string{"id"})
	if cmd2 := m.runExCommand("refs"); cmd2 == nil {
		t.Fatalf(":refs (no arg, current=departments) returned nil: %q", m.schemaMsg)
	}
}

func TestExRefsScopedCounts(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE departments (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE employees (id INTEGER PRIMARY KEY, dept_id INTEGER, FOREIGN KEY (dept_id) REFERENCES departments(id))`,
		`CREATE TABLE budgets (id INTEGER PRIMARY KEY, dept_id INTEGER, FOREIGN KEY (dept_id) REFERENCES departments(id))`,
		`INSERT INTO departments (id, name) VALUES (1,'Eng'),(2,'Sales')`,
		`INSERT INTO employees (id, dept_id) VALUES (10,1),(11,1),(12,2)`,
		`INSERT INTO budgets (id, dept_id) VALUES (100,1),(101,2)`,
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

	// Focus dept id=1 in the results grid so counts are row-scoped.
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Eng"}}, "")
	m.results.SetEditable("departments", []string{"id"})

	msg, ok := m.runExCommand("refs departments")().(lookupResultMsg)
	if !ok {
		t.Fatalf("expected lookupResultMsg, got %T", m.runExCommand("refs departments"))
	}
	if msg.err != nil {
		t.Fatalf("refs fetch error: %v", msg.err)
	}

	// Title should identify the scoped row.
	if !strings.Contains(msg.title, "#1") {
		t.Errorf("title %q should be row-scoped (#1)", msg.title)
	}
	// A Count column should be present.
	hasCount := false
	for _, c := range msg.result.Columns {
		if c.Name == "Count" {
			hasCount = true
		}
	}
	if !hasCount {
		t.Fatalf("expected Count column, got %+v", msg.result.Columns)
	}

	// Map child table -> its count string and verify dept 1 fan-out.
	counts := map[string]string{}
	for _, row := range msg.result.Rows {
		// row = [Table, Column, References, Count]
		counts[row[0]] = row[len(row)-1]
	}
	if counts["employees"] != "2" {
		t.Errorf("employees count = %q, want 2", counts["employees"])
	}
	if counts["budgets"] != "1" {
		t.Errorf("budgets count = %q, want 1", counts["budgets"])
	}
}

func TestExRefsUnscopedNoCountColumn(t *testing.T) {
	// When the grid is not backing the queried table, no Count column appears
	// (backward-compatible fallback to the original behaviour).
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE departments (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE employees (id INTEGER PRIMARY KEY, dept_id INTEGER, FOREIGN KEY (dept_id) REFERENCES departments(id))`,
		`INSERT INTO departments VALUES (1,'Eng')`,
		`INSERT INTO employees VALUES (10,1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{
		connection: conn,
		results:    NewResultsTable(), // no result loaded -> not scoped
		editor:     NewQueryEditor(),
		tables:     []string{"departments", "employees"},
	}

	msg, ok := m.runExCommand("refs departments")().(lookupResultMsg)
	if !ok {
		t.Fatalf("expected lookupResultMsg, got %T", m.runExCommand("refs departments"))
	}
	if msg.err != nil {
		t.Fatalf("refs fetch error: %v", msg.err)
	}
	for _, c := range msg.result.Columns {
		if c.Name == "Count" {
			t.Errorf("unscoped :refs should not have a Count column, got %+v", msg.result.Columns)
		}
	}
	if !strings.Contains(msg.title, "departments") || strings.Contains(msg.title, "#") {
		t.Errorf("unscoped title %q should not be row-scoped", msg.title)
	}
}

func TestLookupPanelEmpty(t *testing.T) {
	var p LookupPanel
	p.Show("References to users", db.Result{})
	if !p.IsVisible() {
		t.Fatal("panel should be visible")
	}
	// An empty result yields just the header line (title + count 0).
	if got := len(p.lines()); got != 1 {
		t.Fatalf("empty lookup panel lines = %d, want 1", got)
	}
}
