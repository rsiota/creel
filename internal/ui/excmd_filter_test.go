package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/db"
)

// Tests for the v2 ex-command wiring: :filter (the headline — connecting the :
// line to the m.filters infra) and :open/:save (file aliases of :e/:w).

func TestParseFilterExpr(t *testing.T) {
	cases := []struct {
		in           string
		col, op, val string
		ok           bool
	}{
		{"status=active", "status", "=", "active", true},   // compact form
		{"status = active", "status", "=", "active", true}, // spaced form
		{"amount > 100", "amount", ">", "100", true},
		{"x >= 5", "x", ">=", "5", true}, // >= before > in the regex
		{"x <= 5", "x", "<=", "5", true},
		{"name = John Doe", "name", "=", "John Doe", true}, // value keeps spaces
		{"nocol", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, c := range cases {
		col, op, val, ok := parseFilterExpr(c.in)
		if ok != c.ok || (ok && (col != c.col || op != c.op || val != c.val)) {
			t.Errorf("parseFilterExpr(%q) = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
				c.in, col, op, val, ok, c.col, c.op, c.val, c.ok)
		}
	}
}

func TestBuildFilterFragment(t *testing.T) {
	cases := []struct {
		col, op, val, typ, want string
	}{
		{"msg", "=", "hi", "TEXT", "msg = 'hi'"},    // text quoted
		{"level", ">", "3", "INTEGER", "level > 3"}, // numeric bare
		{"level", "!=", "0", "INTEGER", "level != 0"},
		{"msg", "~", "alert", "TEXT", "msg LIKE '%alert%'"},
		{"name", "=", "O'Brien", "TEXT", "name = 'O''Brien'"}, // embedded quote escaped
	}
	for _, c := range cases {
		if got := buildFilterFragment(c.col, c.op, c.val, c.typ); got != c.want {
			t.Errorf("buildFilterFragment(%v) = %q, want %q", c, got, c.want)
		}
	}
}

// filterTestModel builds a model whose results are a simple single-table SELECT
// with typed columns, so :filter's canFilter path and value type-quoting work.
func filterTestModel(t *testing.T) *Model {
	t.Helper()
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Execute("CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, msg TEXT, level INTEGER)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	m := &Model{
		connection: conn,
		tables:     []string{"events"},
		baseQuery:  "SELECT * FROM events",
		editor:     NewQueryEditor(),
		results:    NewResultsTable(),
	}
	m.results.SetResult([]string{"id", "msg", "level"}, [][]string{{"1", "a", "5"}}, "")
	m.results.SetColumnTypes(map[string]string{"id": "INTEGER", "msg": "TEXT", "level": "INTEGER"})
	return m
}

func TestExFilterNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("filter level > 1")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":filter with no connection -> %q", m.schemaMsg)
	}
}

func TestExFilterNeedsSimpleQuery(t *testing.T) {
	m := &Model{
		connection: &db.Connection{},
		baseQuery:  "SELECT * FROM a JOIN b ON a.id = b.id",
		results:    NewResultsTable(),
	}
	m.runExCommand("filter level > 1")
	if !strings.Contains(m.schemaMsg, "simple table query") {
		t.Errorf(":filter on a join -> %q", m.schemaMsg)
	}
}

func TestExFilterBareAndOffWithNone(t *testing.T) {
	m := filterTestModel(t)
	m.runExCommand("filter")
	if !strings.Contains(m.schemaMsg, "no active filters") {
		t.Errorf("bare :filter with none -> %q", m.schemaMsg)
	}
	m.runExCommand("filter off")
	if !strings.Contains(m.schemaMsg, "no active filters") {
		t.Errorf(":filter off with none -> %q", m.schemaMsg)
	}
}

func TestExFilterAppliesAndClears(t *testing.T) {
	m := filterTestModel(t)
	cmd := m.runExCommand("filter level > 3")
	if cmd == nil {
		t.Fatal(":filter level > 3 should re-run the query")
	}
	if len(m.filters) != 1 || m.filters[0] != "level > 3" {
		t.Errorf("filters = %v, want [level > 3]", m.filters)
	}
	if !strings.Contains(m.lastQuery, "WHERE") || !strings.Contains(m.lastQuery, "level > 3") {
		t.Errorf("lastQuery = %q, want it to carry the WHERE", m.lastQuery)
	}
	// bare now lists the active filter
	m.runExCommand("filter")
	if !strings.Contains(m.schemaMsg, "level") {
		t.Errorf("bare :filter should list active filters: %q", m.schemaMsg)
	}
	// off clears and re-runs the base query
	cmd = m.runExCommand("filter off")
	if len(m.filters) != 0 {
		t.Errorf("after :filter off, filters = %v, want empty", m.filters)
	}
	if cmd == nil {
		t.Error(":filter off should re-run the base query")
	}
}

func TestExFilterTypeQuotesText(t *testing.T) {
	m := filterTestModel(t)
	m.runExCommand("filter msg = hello")
	if len(m.filters) != 1 || m.filters[0] != "msg = 'hello'" {
		t.Errorf("text value should be quoted: filters = %v", m.filters)
	}
}

func TestExFilterLike(t *testing.T) {
	m := filterTestModel(t)
	m.runExCommand("filter msg ~ alert")
	if len(m.filters) != 1 || m.filters[0] != "msg LIKE '%alert%'" {
		t.Errorf("LIKE fragment: filters = %v", m.filters)
	}
}

func TestExFilterReplacesColumnFilter(t *testing.T) {
	m := filterTestModel(t)
	m.runExCommand("filter level > 3")
	m.runExCommand("filter level = 5")
	if len(m.filters) != 1 || m.filters[0] != "level = 5" {
		t.Errorf("second :filter on the same column should replace: filters = %v", m.filters)
	}
}

func TestExFilterCompactForm(t *testing.T) {
	m := filterTestModel(t)
	m.runExCommand("filter level>3") // no spaces
	if len(m.filters) != 1 || m.filters[0] != "level > 3" {
		t.Errorf("compact form: filters = %v", m.filters)
	}
}

func TestExFilterUnknownColumn(t *testing.T) {
	m := filterTestModel(t)
	m.runExCommand("filter bogus > 1")
	if !strings.Contains(m.schemaMsg, "no such column") {
		t.Errorf(":filter unknown column -> %q", m.schemaMsg)
	}
}

func TestExFilterBadExpr(t *testing.T) {
	m := filterTestModel(t)
	m.runExCommand("filter nonsense")
	if !strings.Contains(m.schemaMsg, "usage") {
		t.Errorf(":filter bad expr -> %q", m.schemaMsg)
	}
}

// :open / :save are file aliases of :e / :w.

func TestExOpenSaveMissingPath(t *testing.T) {
	m := &Model{editor: NewQueryEditor()}
	m.runExCommand("open")
	if !strings.Contains(m.schemaMsg, "file path") {
		t.Errorf(":open with no arg -> %q", m.schemaMsg)
	}
	m.runExCommand("save")
	if !strings.Contains(m.schemaMsg, "file path") {
		t.Errorf(":save with no arg -> %q", m.schemaMsg)
	}
}

func TestExOpenLoadsLikeEdit(t *testing.T) {
	content := "SELECT 1;\nSELECT 2;\n"
	path := filepath.Join(t.TempDir(), "script.sql")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{editor: NewQueryEditor()}
	m.runExCommand("open " + path)
	if m.editor.Value() != content {
		t.Errorf(":open did not load file: got %q", m.editor.Value())
	}
	if !strings.Contains(m.schemaMsg, "loaded") {
		t.Errorf(":open should confirm via :e's message: %q", m.schemaMsg)
	}
}
