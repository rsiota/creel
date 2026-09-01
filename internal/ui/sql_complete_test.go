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
	e.buf.SetMode(VimInsert)
	e.Focus()
	e.SetCandidates(testCompleteCatalog())
	e.buf.InsertString("SELECT * FROM users WHERE e")
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
	e.buf.SetMode(VimInsert)
	e.Focus()
	e.SetCandidates(testCompleteCatalog())
	e.buf.InsertString("SELECT * FROM u")
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

func TestCompletionUpdateSetSuggestsColumns(t *testing.T) {
	all := testCompleteCatalog()
	scope := sqlCompleteScopeFrom("UPDATE users SET ", knownTablesFrom(all))
	if scope.want != wantColumn {
		t.Fatalf("want = %v, want wantColumn", scope.want)
	}
	got := scope.filter(all)
	names := map[string]bool{}
	for _, it := range got {
		if it.kind == kindColumn {
			names[it.text] = true
		}
		if it.kind == kindTable {
			t.Errorf("table %q in SET completion", it.text)
		}
	}
	if !names["email"] || !names["id"] {
		t.Errorf("missing users columns after SET: %v", names)
	}
	if names["total"] {
		t.Errorf("orders column leaked into UPDATE users SET: %v", names)
	}

	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.buf.SetMode(VimInsert)
	e.Focus()
	e.SetCandidates(all)
	e.buf.InsertString("UPDATE users SET e")
	e.StartCompletion()
	found := false
	for _, c := range e.completion.candidates {
		if c.kind == kindColumn && c.text == "email" {
			found = true
		}
		if c.kind == kindColumn && c.table == "orders" {
			t.Errorf("orders column %q in UPDATE users SET", c.text)
		}
	}
	if !found {
		t.Fatal("expected email after UPDATE users SET e")
	}
}

func TestCompletionInsertColumnList(t *testing.T) {
	all := testCompleteCatalog()
	scope := sqlCompleteScopeFrom("INSERT INTO users (", knownTablesFrom(all))
	if scope.want != wantColumn {
		t.Fatalf("want = %v, want wantColumn", scope.want)
	}
	if len(scope.tables) != 1 || scope.tables[0] != "users" {
		t.Fatalf("tables = %v, want [users]", scope.tables)
	}
	got := scope.filter(all)
	for _, it := range got {
		if it.kind == kindTable {
			t.Errorf("table %q in INSERT column list", it.text)
		}
		if it.kind == kindColumn && it.table != "users" {
			t.Errorf("column %q from %s in INSERT INTO users", it.text, it.table)
		}
	}

	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.buf.SetMode(VimInsert)
	e.Focus()
	e.SetCandidates(all)
	e.buf.InsertString("INSERT INTO users (e")
	e.StartCompletion()
	found := false
	for _, c := range e.completion.candidates {
		if c.text == "email" {
			found = true
		}
		if c.kind == kindColumn && c.table != "users" {
			t.Errorf("unexpected column %q (%s)", c.text, c.table)
		}
	}
	if !found {
		t.Fatal("expected email in INSERT INTO users (e")
	}
}

func TestCompletionSelectListUsesFromTables(t *testing.T) {
	all := testCompleteCatalog()
	// Cursor in SELECT list; FROM users appears after the cursor in the statement.
	scope := sqlCompleteScopeFromQuery("SELECT ", "SELECT  FROM users", knownTablesFrom(all), nil, "")
	if scope.want != wantColumn {
		t.Fatalf("want = %v, want wantColumn (SELECT list with FROM users)", scope.want)
	}
	if len(scope.tables) != 1 || scope.tables[0] != "users" {
		t.Fatalf("tables = %v, want [users]", scope.tables)
	}
	got := scope.filter(all)
	names := map[string]bool{}
	for _, it := range got {
		if it.kind == kindColumn {
			names[it.text] = true
		}
		if it.kind == kindTable {
			t.Errorf("table %q in SELECT-list completion once FROM is known", it.text)
		}
	}
	if !names["email"] {
		t.Errorf("expected users columns in SELECT list: %v", names)
	}
	if names["total"] {
		t.Errorf("orders column leaked: %v", names)
	}

	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.buf.SetMode(VimInsert)
	e.Focus()
	e.SetCandidates(all)
	// Build "SELECT  FROM users" then move cursor back into the SELECT list.
	e.buf.InsertString("SELECT  FROM users")
	// Move left past " FROM users" (12 runes) to sit after "SELECT ".
	for i := 0; i < len(" FROM users"); i++ {
		e.buf.sendKey("left")
	}
	e.buf.InsertString("e")
	e.StartCompletion()
	if !e.CompletionVisible() {
		t.Fatal("expected completion in SELECT list")
	}
	found := false
	for _, c := range e.completion.candidates {
		if c.text == "email" {
			found = true
		}
		if c.kind == kindColumn && c.table != "users" {
			t.Errorf("unexpected column %q (%s)", c.text, c.table)
		}
	}
	if !found {
		t.Fatalf("expected email in SELECT list; candidates=%v", candidateTexts(e.completion.candidates))
	}
}

func candidateTexts(items []completionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.text
	}
	return out
}

func TestSQLCompleteSchemaQualifierOffersTables(t *testing.T) {
	all := []completionItem{
		{text: "public", kind: kindSchema},
		{text: "users", kind: kindTable}, // active schema
		{text: "orders", kind: kindTable, schema: "public"},
		{text: "accounts", kind: kindTable, schema: "billing"},
		{text: "id", kind: kindColumn, table: "users"},
	}
	known := knownTablesFrom(all)
	schemas := knownSchemasFrom(all)
	scope := sqlCompleteScopeFromQuery("SELECT * FROM public.", "", known, schemas, "public")
	if scope.want != wantTable {
		t.Fatalf("want = %v, want wantTable", scope.want)
	}
	if scope.schemaFilter != "public" {
		t.Fatalf("schemaFilter = %q, want public", scope.schemaFilter)
	}
	got := scope.filter(all)
	names := map[string]bool{}
	for _, it := range got {
		if it.kind == kindColumn {
			t.Errorf("column %q after schema.", it.text)
		}
		if it.kind == kindSchema {
			t.Errorf("schema %q after schema.", it.text)
		}
		if it.kind == kindTable {
			names[it.text] = true
		}
	}
	if !names["users"] && !names["orders"] {
		t.Errorf("expected public tables, got %v", names)
	}
	if names["accounts"] {
		t.Errorf("billing.accounts leaked into public.: %v", names)
	}
}

func TestSQLCompleteSchemaTableQualifierOffersColumns(t *testing.T) {
	all := []completionItem{
		{text: "public", kind: kindSchema},
		{text: "email", kind: kindColumn, table: "public.users"},
		{text: "total", kind: kindColumn, table: "public.orders"},
		{text: "id", kind: kindColumn, table: "users"},
	}
	scope := sqlCompleteScopeFromQuery("SELECT * FROM public.users WHERE public.users.", "", knownTablesFrom(all), knownSchemasFrom(all), "public")
	if scope.want != wantColumn {
		t.Fatalf("want = %v, want wantColumn", scope.want)
	}
	if len(scope.tables) != 1 || scope.tables[0] != "public.users" {
		t.Fatalf("tables = %v, want [public.users]", scope.tables)
	}
	got := scope.filter(all)
	names := map[string]bool{}
	for _, it := range got {
		if it.kind == kindColumn {
			names[it.text] = true
		}
	}
	if !names["email"] {
		t.Errorf("missing public.users columns: %v", names)
	}
	if names["total"] || names["id"] {
		t.Errorf("other table columns leaked: %v", names)
	}
}

func TestScanSQLCompleteSchemaQualifiedFrom(t *testing.T) {
	all := []completionItem{
		{text: "public", kind: kindSchema},
		{text: "users", kind: kindTable},
	}
	scope, _, _ := scanSQLComplete("SELECT * FROM public.users WHERE ", knownTablesFrom(all), knownSchemasFrom(all))
	if len(scope.tables) != 1 || scope.tables[0] != "public.users" {
		t.Fatalf("tables = %v, want [public.users]", scope.tables)
	}
}

func TestTrailingQualifierParts(t *testing.T) {
	parts := trailingQualifierParts(tokenizeSQL("SELECT * FROM public.users."))
	if len(parts) != 2 || parts[0] != "public" || parts[1] != "users" {
		t.Fatalf("parts = %v, want [public users]", parts)
	}
	parts = trailingQualifierParts(tokenizeSQL("SELECT * FROM u."))
	if len(parts) != 1 || parts[0] != "u" {
		t.Fatalf("parts = %v, want [u]", parts)
	}
}

// TestCompletionSelectListHidesUnscopedColumns verifies that before FROM names
// any tables, the popup offers keywords/tables but not every column in the
// catalog (the previous wantAny behaviour was very noisy after one character).
func TestCompletionSelectListHidesUnscopedColumns(t *testing.T) {
	all := testCompleteCatalog()
	scope := sqlCompleteScopeFrom("SELECT ", knownTablesFrom(all))
	if scope.want != wantAny {
		t.Fatalf("want = %v, want wantAny", scope.want)
	}
	got := scope.filter(all)
	var sawKeyword, sawTable bool
	for _, it := range got {
		switch it.kind {
		case kindColumn:
			t.Errorf("unscoped column %q (%s) offered in SELECT list", it.text, it.table)
		case kindKeyword:
			sawKeyword = true
		case kindTable:
			sawTable = true
		}
	}
	if !sawKeyword {
		t.Error("expected keywords in SELECT-list completion")
	}
	if !sawTable {
		t.Error("expected tables in SELECT-list completion")
	}

	e := NewQueryEditor()
	e.SetSize(80, 10)
	e.buf.SetMode(VimInsert)
	e.Focus()
	e.SetCandidates(all)
	e.buf.InsertString("SELECT e")
	e.StartCompletion()
	if !e.CompletionVisible() {
		t.Fatal("expected completion visible after SELECT e")
	}
	for _, c := range e.completion.candidates {
		if c.kind == kindColumn {
			t.Errorf("column %q offered after SELECT e", c.text)
		}
	}
	found := false
	for _, c := range e.completion.candidates {
		if c.text == "SELECT" || c.kind == kindKeyword {
			found = true
		}
	}
	// "e" should still match keywords like ELSE / END / EXISTS, not email.
	if !found {
		t.Fatal("expected keyword matches after SELECT e")
	}
	for _, c := range e.completion.candidates {
		if c.text == "email" {
			t.Fatal("email column must not appear before FROM")
		}
	}
}
