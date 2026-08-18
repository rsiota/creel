package ui

import (
	"testing"
)

func testCompleteCatalog() []completionItem {
	return []completionItem{
		{text: "SELECT", kind: kindKeyword},
		{text: "FROM", kind: kindKeyword},
		{text: "WHERE", kind: kindKeyword},
		{text: "JOIN", kind: kindKeyword},
		{text: "users", kind: kindTable},
		{text: "orders", kind: kindTable},
		{text: "id", kind: kindColumn, table: "users"},
		{text: "email", kind: kindColumn, table: "users"},
		{text: "id", kind: kindColumn, table: "orders"},
		{text: "user_id", kind: kindColumn, table: "orders"},
		{text: "total", kind: kindColumn, table: "orders"},
	}
}

func TestSQLCompleteScopeFromTable(t *testing.T) {
	known := knownTablesFrom(testCompleteCatalog())
	scope := sqlCompleteScopeFrom("SELECT * FROM ", known)
	if scope.want != wantTable {
		t.Errorf("want = %v, want wantTable", scope.want)
	}
}

func TestSQLCompleteScopeFromWhereColumns(t *testing.T) {
	known := knownTablesFrom(testCompleteCatalog())
	scope := sqlCompleteScopeFrom("SELECT * FROM users WHERE ", known)
	if scope.want != wantColumn {
		t.Fatalf("want = %v, want wantColumn", scope.want)
	}
	if len(scope.tables) != 1 || scope.tables[0] != "users" {
		t.Fatalf("tables = %v, want [users]", scope.tables)
	}
}

func TestSQLCompleteScopeJoinAndAlias(t *testing.T) {
	known := knownTablesFrom(testCompleteCatalog())
	scope := sqlCompleteScopeFrom("SELECT * FROM users u JOIN orders o ON ", known)
	if scope.want != wantColumn {
		t.Fatalf("want = %v, want wantColumn", scope.want)
	}
	if len(scope.tables) != 2 {
		t.Fatalf("tables = %v, want users and orders", scope.tables)
	}
	if scope.aliases["u"] != "users" || scope.aliases["o"] != "orders" {
		t.Errorf("aliases = %v", scope.aliases)
	}

	q := sqlCompleteScopeFrom("SELECT * FROM users u WHERE u.", known)
	if q.qualifier != "u" || q.want != wantColumn {
		t.Fatalf("qualifier scope = %+v", q)
	}
	if len(q.tables) != 1 || q.tables[0] != "users" {
		t.Errorf("qualified tables = %v, want [users]", q.tables)
	}
}

func TestSQLCompleteFilterWhereHidesOtherTable(t *testing.T) {
	all := testCompleteCatalog()
	scope := sqlCompleteScopeFrom("SELECT * FROM users WHERE ", knownTablesFrom(all))
	got := scope.filter(all)
	names := map[string]bool{}
	for _, it := range got {
		if it.kind == kindColumn {
			names[it.text] = true
		}
		if it.kind == kindTable {
			t.Errorf("did not expect table %q in WHERE completion", it.text)
		}
	}
	if !names["email"] || !names["id"] {
		t.Errorf("missing users columns: %v", names)
	}
	if names["total"] || names["user_id"] {
		t.Errorf("orders columns leaked into users WHERE: %v", names)
	}
}

func TestCompletionWhereOnlyCurrentTableColumns(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()
	e.SetCandidates(testCompleteCatalog())
	e.textarea.InsertString("SELECT * FROM users WHERE e")
	e.StartCompletion()
	if !e.CompletionVisible() {
		t.Fatal("expected completion visible")
	}
	for _, c := range e.completion.candidates {
		if c.kind == kindColumn && c.text != "email" {
			t.Errorf("unexpected column %q (table %q)", c.text, c.table)
		}
		if c.kind == kindTable {
			t.Errorf("unexpected table %q after WHERE", c.text)
		}
	}
	found := false
	for _, c := range e.completion.candidates {
		if c.text == "email" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected email among WHERE candidates")
	}
}

func TestCompletionFromSuggestsTablesNotColumns(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.vimMode = VimInsert
	e.Focus()
	e.SetCandidates(testCompleteCatalog())
	e.textarea.InsertString("SELECT * FROM u")
	e.StartCompletion()
	for _, c := range e.completion.candidates {
		if c.kind == kindColumn {
			t.Errorf("column %q offered after FROM", c.text)
		}
	}
	found := false
	for _, c := range e.completion.candidates {
		if c.text == "users" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected users table after FROM")
	}
}
