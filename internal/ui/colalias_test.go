package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestDisambiguateColumnNames(t *testing.T) {
	got, changed := disambiguateColumnNames([]string{"id", "name", "id", "id_2"})
	if !changed {
		t.Fatal("expected duplicates to change")
	}
	want := []string{"id", "name", "id_3", "id_2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	got, changed = disambiguateColumnNames([]string{"id", "name"})
	if changed || strings.Join(got, ",") != "id,name" {
		t.Fatalf("unique names changed: %v changed=%v", got, changed)
	}
	got, changed = disambiguateColumnNames([]string{"ID", "id"})
	if !changed || strings.Join(got, ",") != "ID,id_2" {
		t.Fatalf("case fold: got %v", got)
	}
}

func TestColumnLeaf(t *testing.T) {
	if !isStatusColumnName("orders.status") {
		t.Fatal("qualified status column should tint")
	}
	if !isDatetimeColumnName("u.created_at") {
		t.Fatal("qualified created_at should format as a datetime")
	}
	if !isBooleanColumnName("u.is_admin") {
		t.Fatal("qualified is_admin should render as a boolean")
	}
}

func schemaLookup(schema map[string][]string) func(string) ([]string, bool) {
	return func(table string) ([]string, bool) {
		cols, ok := schema[table]
		return cols, ok && len(cols) > 0
	}
}

func TestRewriteSelectAliasesExplicit(t *testing.T) {
	q := "SELECT u.id, u.name, o.id FROM users u JOIN orders o ON u.id = o.user_id"
	got, ok := rewriteSelectAliases(q, db.DriverPostgres, schemaLookup(nil))
	if !ok {
		t.Fatal("expected rewrite")
	}
	want := `SELECT u.id, u.name, o.id AS "o.id" FROM users u JOIN orders o ON u.id = o.user_id`
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if _, ok := rewriteSelectAliases("SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id", db.DriverPostgres, nil); ok {
		t.Fatal("unique names should not be rewritten")
	}
}

func TestRewriteSelectAliasesStar(t *testing.T) {
	schema := map[string][]string{
		"users":  {"id", "name"},
		"orders": {"id", "user_id", "total"},
	}
	q := "SELECT * FROM users JOIN orders ON users.id = orders.user_id"
	got, ok := rewriteSelectAliases(q, db.DriverSQLite, schemaLookup(schema))
	if !ok {
		t.Fatal("expected star rewrite")
	}
	want := `SELECT "users"."id", "users"."name", "orders"."id" AS "orders.id", "orders"."user_id", "orders"."total" FROM users JOIN orders ON users.id = orders.user_id`
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	aliased := "SELECT * FROM users u JOIN orders o ON u.id = o.user_id"
	got, ok = rewriteSelectAliases(aliased, db.DriverSQLite, schemaLookup(schema))
	if !ok {
		t.Fatal("expected aliased star rewrite")
	}
	want = `SELECT "u"."id", "u"."name", "o"."id" AS "o.id", "o"."user_id", "o"."total" FROM users u JOIN orders o ON u.id = o.user_id`
	if got != want {
		t.Fatalf("aliased star got %q\nwant %q", got, want)
	}
}

func TestRewriteSelectAliasesQualStar(t *testing.T) {
	schema := map[string][]string{
		"users":  {"id", "name"},
		"orders": {"id", "total"},
	}
	q := "SELECT u.*, o.* FROM users u JOIN orders o ON u.id = o.user_id"
	got, ok := rewriteSelectAliases(q, db.DriverSQLite, schemaLookup(schema))
	if !ok {
		t.Fatal("expected qual-star rewrite")
	}
	want := `SELECT u.*, "o"."id" AS "o.id", "o"."total" FROM users u JOIN orders o ON u.id = o.user_id`
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestRewriteSelectAliasesDistinctAndLiteral(t *testing.T) {
	q := "SELECT DISTINCT u.id, 'a,b' AS label, o.id FROM users u LEFT JOIN orders o ON u.id = o.user_id WHERE o.id > 1"
	got, ok := rewriteSelectAliases(q, db.DriverMySQL, schemaLookup(nil))
	if !ok {
		t.Fatal("expected rewrite")
	}
	want := "SELECT DISTINCT u.id, 'a,b' AS label, o.id AS `o.id` FROM users u LEFT JOIN orders o ON u.id = o.user_id WHERE o.id > 1"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestRewriteSelectAliasesRefusesUnsafe(t *testing.T) {
	schema := map[string][]string{"users": {"id"}, "orders": {"id"}}
	cases := []string{
		"SELECT * FROM users",
		"SELECT * FROM users JOIN orders ON users.id = orders.user_id NATURAL",
		"SELECT * FROM (SELECT * FROM users) u JOIN orders ON u.id = orders.user_id",
		"SELECT * FROM users NATURAL JOIN orders",
		"SELECT * FROM users u JOIN orders o USING (id)",
		"SELECT count(*), o.id FROM users u JOIN orders o ON u.id = o.user_id",
		"WITH c AS (SELECT 1 AS id) SELECT * FROM c JOIN users ON true",
	}
	for _, q := range cases {
		// NATURAL-only suffix isn't valid; the NATURAL JOIN and subquery cases are.
		if _, ok := rewriteSelectAliases(q, db.DriverSQLite, schemaLookup(schema)); ok {
			t.Errorf("rewrote unsafe query %q", q)
		}
	}
	// Unique star across a join stays as SELECT * — nothing to alias.
	q := "SELECT * FROM users JOIN orders ON users.id = orders.user_id"
	schema = map[string][]string{
		"users":  {"id", "name"},
		"orders": {"order_id", "total"},
	}
	if _, ok := rewriteSelectAliases(q, db.DriverSQLite, schemaLookup(schema)); ok {
		t.Fatal("unique star join should stay untouched")
	}
}

func TestRewriteSelectAliasesSQLiteValues(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(dir, "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	for _, stmt := range []string{
		`CREATE TABLE users (id INTEGER, name TEXT)`,
		`CREATE TABLE orders (id INTEGER, user_id INTEGER, total TEXT)`,
		`INSERT INTO users VALUES (1, 'ada')`,
		`INSERT INTO orders VALUES (9, 1, '10')`,
	} {
		if _, err := conn.DB().Execute(stmt); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(table string) ([]string, bool) {
		cols, err := conn.DB().TableSchema(table)
		if err != nil || len(cols) == 0 {
			return nil, false
		}
		names := make([]string, len(cols))
		for i, col := range cols {
			names[i] = col.Name
		}
		return names, true
	}
	q := `SELECT * FROM users JOIN orders ON users.id = orders.user_id`
	rewritten, ok := rewriteSelectAliases(q, db.DriverSQLite, lookup)
	if !ok {
		t.Fatal("expected rewrite")
	}
	result, err := conn.DB().Execute(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, col := range result.Columns {
		names = append(names, col.Name)
	}
	want := []string{"id", "name", "orders.id", "user_id", "total"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("columns %v, want %v\nsql %s", names, want, rewritten)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("rows %d", len(result.Rows))
	}
	row := result.Rows[0]
	if strings.Join(row, ",") != "1,ada,9,1,10" {
		t.Fatalf("row %v", row)
	}
	wrapped := `SELECT * FROM (` + rewritten + `) AS _creel_filt ORDER BY "orders.id" DESC`
	sorted, err := conn.DB().Execute(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted.Rows) != 1 || sorted.Rows[0][2] != "9" {
		t.Fatalf("wrapped sort row %v", sorted.Rows)
	}
}

func TestFilterUsesAliasedSource(t *testing.T) {
	m := Model{
		baseQuery:  "SELECT * FROM users JOIN orders ON users.id = orders.user_id",
		wrapSource: `SELECT "users"."id", "orders"."id" AS "orders.id" FROM users JOIN orders ON users.id = orders.user_id`,
		filters:    []string{`"orders.id" = '9'`},
		sortCol:    "orders.id",
		sortDir:    "DESC",
	}
	got := m.buildFilteredQuery()
	if !strings.Contains(got, `AS "orders.id"`) || !strings.Contains(got, `ORDER BY orders.id DESC`) {
		t.Fatalf("filtered query %q", got)
	}
	if strings.Contains(got, "SELECT * FROM users JOIN") {
		t.Fatalf("wrapped the original star join: %q", got)
	}
}
