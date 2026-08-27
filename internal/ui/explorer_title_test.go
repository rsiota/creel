package ui

import (
	"strings"
	"testing"
)

func TestRowLabelWithTitle(t *testing.T) {
	cols := []string{"id", "name", "email"}
	vals := map[string]string{"id": "91", "name": "Alice Smith", "email": "a@x.com"}
	got := rowLabel(cols, vals, []string{"id"}, 0)
	if got != "#91  Alice Smith" {
		t.Errorf("rowLabel = %q, want %#v", got, "#91  Alice Smith")
	}
}

func TestRowLabelPrefersNameOverEmail(t *testing.T) {
	cols := []string{"id", "email", "name"}
	vals := map[string]string{"id": "1", "email": "a@x.com", "name": "Bob"}
	got := rowLabel(cols, vals, []string{"id"}, 0)
	if got != "#1  Bob" {
		t.Errorf("rowLabel = %q, want #1  Bob", got)
	}
}

func TestRowLabelSuffixName(t *testing.T) {
	cols := []string{"id", "product_name", "qty"}
	vals := map[string]string{"id": "501", "product_name": "MacBook Pro", "qty": "1"}
	got := rowLabel(cols, vals, []string{"id"}, 0)
	if got != "#501  MacBook Pro" {
		t.Errorf("rowLabel = %q, want #501  MacBook Pro", got)
	}
}

func TestRowLabelNoTitleKeepsID(t *testing.T) {
	cols := []string{"id", "user_id", "total"}
	vals := map[string]string{"id": "10", "user_id": "1", "total": "99"}
	got := rowLabel(cols, vals, []string{"id"}, 0)
	if got != "#10" {
		t.Errorf("rowLabel = %q, want #10 (no title for numeric/FK cols)", got)
	}
}

func TestRowLabelSkipsOpaqueUUID(t *testing.T) {
	cols := []string{"id", "name"}
	vals := map[string]string{
		"id":   "1",
		"name": "550e8400-e29b-41d4-a716-446655440000",
	}
	got := rowLabel(cols, vals, []string{"id"}, 0)
	if got != "#1" {
		t.Errorf("rowLabel = %q, want #1 (UUID title rejected)", got)
	}
}

func TestRowLabelTruncatesLongTitle(t *testing.T) {
	long := strings.Repeat("x", 80)
	cols := []string{"id", "title"}
	vals := map[string]string{"id": "2", "title": long}
	got := rowLabel(cols, vals, []string{"id"}, 0)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("rowLabel = %q, want truncated with …", got)
	}
	if runeLen(got) > len("#2  ")+explorerTitleMax {
		t.Errorf("rowLabel too long: %d runes", runeLen(got))
	}
}

func TestRowLabelSkipsMetadataColumns(t *testing.T) {
	cols := []string{"id", "updated_at", "token"}
	vals := map[string]string{
		"id":         "3",
		"updated_at": "2026-01-01",
		"token":      "secret-value",
	}
	got := rowLabel(cols, vals, []string{"id"}, 0)
	if got != "#3" {
		t.Errorf("rowLabel = %q, want #3", got)
	}
}

func TestRowLabelFallbackTextColumn(t *testing.T) {
	cols := []string{"id", "notes"}
	vals := map[string]string{"id": "4", "notes": "ship overnight"}
	got := rowLabel(cols, vals, []string{"id"}, 0)
	if got != "#4  ship overnight" {
		t.Errorf("rowLabel = %q, want #4  ship overnight", got)
	}
}

func TestPkLabelWithTitle(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "Alice"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})
	got := pkLabel(r)
	if got != " · #1  Alice" {
		t.Errorf("pkLabel = %q, want %q", got, " · #1  Alice")
	}
}

func TestExpandEdgeLoadsChildRowsWithTitle(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE companies (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE employees (id INTEGER PRIMARY KEY, company_id INTEGER, full_name TEXT, FOREIGN KEY (company_id) REFERENCES companies(id))`,
		`INSERT INTO companies VALUES (1,'Acme')`,
		`INSERT INTO employees VALUES (7,1,'Ada Lovelace'),(8,1,'Grace Hopper')`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"companies", "employees"}}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Acme"}}, "")
	m.results.SetEditable("companies", []string{"id"})

	root := m.loadExplorer()().(explorerLoadedMsg).root
	if !strings.Contains(root.label, "Acme") {
		t.Errorf("root.label = %q, want Acme title", root.label)
	}
	m.explorer.applyRoot(root, 0)
	edge := findEdge(root, "employees")
	if edge == nil {
		t.Fatal("no employees edge")
	}
	msg := m.loadExplorerChildren(edge)().(explorerChildrenMsg)
	if msg.err != nil {
		t.Fatalf("expand: %v", msg.err)
	}
	rows := nonSynth(msg.children)
	if len(rows) != 2 {
		t.Fatalf("expected 2 employees, got %d", len(rows))
	}
	labels := map[string]bool{}
	for _, r := range rows {
		labels[r.label] = true
	}
	if !labels["#7  Ada Lovelace"] || !labels["#8  Grace Hopper"] {
		t.Errorf("labels = %v, want titled PK labels", labels)
	}
}
