package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

// newExportTestModel builds a Model wired to a temp SQLite DB with a `users`
// table holding several rows, and editable results backed by that table. The
// page holds only one row so whole-table scope is observably different from the
// in-memory page.
func newExportTestModel(t *testing.T, pageRows [][]string) (Model, *db.Connection) {
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
	if _, err := conn.DB().Execute(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.DB().Execute(`INSERT INTO users (id, name, email) VALUES
		(1,'Ada','ada@x'), (2,'Alan','alan@x'), (3,'Grace','grace@x')`); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = conn
	m.state = stateWorkspace
	m.focus = FocusResults
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	m.results.SetEditable("users", []string{"id"})
	m.results.SetResult([]string{"id", "name", "email"}, pageRows, "page")
	return m, conn
}

// Whole-table scope re-queries without a LIMIT, so all rows are exported even
// though the in-memory page holds just one.
func TestExportResultsWholeTable(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})

	cmd := m.exportResults(fmtCSV, nil, scopeAll)
	if cmd == nil {
		t.Fatal("whole-table export should return an async command")
	}
	msg := cmd()
	done, ok := msg.(exportDoneMsg)
	if !ok {
		t.Fatalf("expected exportDoneMsg, got %T: %v", msg, msg)
	}
	if done.err != nil {
		t.Fatalf("export error: %v", done.err)
	}
	if done.count != 3 {
		t.Errorf("whole-table count=%d, want 3 (all rows)", done.count)
	}
	if !strings.HasSuffix(done.path, ".csv") {
		t.Errorf("path=%s, want .csv suffix", done.path)
	}
}

// Column projection applies to the whole-table re-query: the generated SELECT
// lists only the chosen columns.
func TestExportResultsWholeTableColumnProjection(t *testing.T) {
	m, conn := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})

	// Capture the query the export issues by spying through a wrapper is heavy;
	// instead verify the exported CSV has exactly the requested columns by
	// reading the file back. Two columns: name, email.
	cmd := m.exportResults(fmtCSV, []string{"name", "email"}, scopeAll)
	msg := cmd()
	done, ok := msg.(exportDoneMsg)
	if !ok {
		t.Fatalf("expected exportDoneMsg, got %T", msg)
	}
	if done.err != nil {
		t.Fatalf("export error: %v", done.err)
	}
	b, err := os.ReadFile(done.path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 4 { // header + 3 rows
		t.Fatalf("expected 4 CSV lines, got %d: %q", len(lines), b)
	}
	if lines[0] != "name,email" {
		t.Errorf("header=%q, want name,email", lines[0])
	}
	for _, l := range lines[1:] {
		if strings.Contains(l, ",,") || strings.HasPrefix(l, ",") {
			t.Errorf("row %q has a leading/empty first field (id leaked?)", l)
		}
	}
	_ = conn
}

// Marked-rows scope re-queries only the marked rows, projecting columns.
func TestExportResultsMarkedRows(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	// Mark two rows by PK tuple.
	m.results.markedRows = map[string][]string{
		"1": {"1"},
		"3": {"3"},
	}
	m.results.markedTable = "users"

	cmd := m.exportResults(fmtCSV, nil, scopeMarked)
	if cmd == nil {
		t.Fatal("marked export should return an async command")
	}
	msg := cmd()
	done, ok := msg.(exportDoneMsg)
	if !ok {
		t.Fatalf("expected exportDoneMsg, got %T", msg)
	}
	if done.err != nil {
		t.Fatalf("export error: %v", done.err)
	}
	if done.count != 2 {
		t.Errorf("marked count=%d, want 2", done.count)
	}
}

// scopeMarked with no marks falls back to the current page (in-memory, no
// command), so callers can pass a default scope without prechecking.
func TestExportResultsMarkedFallbackToPage(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})
	cmd := m.exportResults(fmtCSV, nil, scopeMarked)
	if cmd != nil {
		t.Errorf("scopeMarked with no marks should fall back to in-memory page (nil cmd)")
	}
	if !strings.Contains(m.exportMsg, "exported 1 row") {
		t.Errorf("expected 1-row page export, got %q", m.exportMsg)
	}
}

// defaultExportScope prefers marks, then whole table, then page.
func TestDefaultExportScope(t *testing.T) {
	m, _ := newExportTestModel(t, [][]string{{"1", "Ada", "ada@x"}})

	// Editable table, no marks → whole table.
	if got := m.defaultExportScope(); got != scopeAll {
		t.Errorf("editable/no-marks scope=%v, want scopeAll", got)
	}

	// With marks → marked.
	m.results.markedRows = map[string][]string{"1": {"1"}}
	m.results.markedTable = "users"
	if got := m.defaultExportScope(); got != scopeMarked {
		t.Errorf("marked scope=%v, want scopeMarked", got)
	}

	// Non-editable (custom query, no source table) → page.
	m.results.ClearEditable()
	m.results.SetResult([]string{"x"}, [][]string{{"1"}}, "q")
	if got := m.defaultExportScope(); got != scopePage {
		t.Errorf("custom-query scope=%v, want scopePage", got)
	}
}

// resolveExportColumns maps names case-insensitively and preserves result
// order; nil means all columns.
func TestResolveExportColumns(t *testing.T) {
	all := []string{"id", "Name", "Email"}

	// nil → all.
	idx, names := resolveExportColumns(all, nil)
	if len(idx) != 3 || len(names) != 3 {
		t.Fatalf("nil: idx=%v names=%v", idx, names)
	}

	// Subset, out-of-order + different case → request order is preserved
	// (email first, then id).
	idx, names = resolveExportColumns(all, []string{"email", "id"})
	if len(idx) != 2 {
		t.Fatalf("subset: idx=%v", idx)
	}
	if names[0] != "Email" || names[1] != "id" {
		t.Errorf("subset names=%v, want [Email id] (request order)", names)
	}

	// Unknown names dropped.
	idx, _ = resolveExportColumns(all, []string{"id", "bogus"})
	if len(idx) != 1 || idx[0] != 0 {
		t.Errorf("unknown-drop: idx=%v, want [0]", idx)
	}
}

// buildSelectClause returns "*" when all columns are selected (matching the
// previous behaviour) and a quoted list otherwise.
func TestBuildSelectClause(t *testing.T) {
	all := []string{"id", "name"}
	if got := buildSelectClause(db.DriverSQLite, all, all); got != "*" {
		t.Errorf("all columns: got %q, want *", got)
	}
	got := buildSelectClause(db.DriverSQLite, []string{"name"}, all)
	if got != `"name"` {
		t.Errorf("subset: got %q, want \"name\"", got)
	}
	// MySQL uses backticks.
	got = buildSelectClause(db.DriverMySQL, []string{"name", "id"}, all)
	if got != "`name`, `id`" {
		t.Errorf("mysql subset: got %q", got)
	}
}
