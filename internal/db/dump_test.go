package db

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestSQLiteDB returns a clean, empty SQLite database backed by a temp file.
func newTestSQLiteDB(t *testing.T) *SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := ConnectionConfig{Driver: DriverSQLite, Database: dbPath}
	s := NewSQLite(cfg)
	if err := s.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
		os.Remove(dbPath)
	})
	return s
}

func TestDumpSQL_BasicTable(t *testing.T) {
	s := newTestSQLiteDB(t)
	_, err := s.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Exec(`INSERT INTO users (id, name, email) VALUES (1, 'alice', 'alice@test.com')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Exec(`INSERT INTO users (id, name, email) VALUES (2, 'bob', 'bob@test.com')`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := DumpTables(&buf, s, DriverSQLite, "test", []string{"users"}, FormatSQL); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// Must contain expected structural elements.
	// CREATE TABLE comes from sqlite_master (the original DDL), not a
	// reconstructed statement — identifiers are unquoted as written.
	for _, want := range []string{
		"-- creel SQL dump",
		`DROP TABLE IF EXISTS "users";`,
		`CREATE TABLE users (`,
		`id INTEGER PRIMARY KEY`,
		`name TEXT NOT NULL`,
		`email TEXT`,
		`INSERT INTO "users" ("id", "name", "email") VALUES`,
		`(1, 'alice', 'alice@test.com')`,
		`(2, 'bob', 'bob@test.com')`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dump:\n%s", want, out)
		}
	}

	// The dump must no longer contain these wrapper statements.
	for _, banned := range []string{"BEGIN;", "COMMIT;", "FOREIGN_KEY_CHECKS", "SET NAMES"} {
		if strings.Contains(out, banned) {
			t.Errorf("unexpected %q in dump:\n%s", banned, out)
		}
	}
}

func TestDumpSQL_EmptyTable(t *testing.T) {
	s := newTestSQLiteDB(t)
	_, err := s.Exec(`CREATE TABLE empty (id INTEGER PRIMARY KEY, label TEXT)`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := DumpTables(&buf, s, DriverSQLite, "test", []string{"empty"}, FormatSQL); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if strings.Contains(out, "INSERT INTO") {
		t.Errorf("empty table should not produce INSERTs:\n%s", out)
	}
	if !strings.Contains(out, `CREATE TABLE empty`) {
		t.Errorf("missing CREATE TABLE:\n%s", out)
	}
}

func TestDumpSQL_NullValues(t *testing.T) {
	s := newTestSQLiteDB(t)
	_, err := s.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, note TEXT)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Exec(`INSERT INTO items (id, note) VALUES (1, NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Exec(`INSERT INTO items (id, note) VALUES (2, 'hello')`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := DumpTables(&buf, s, DriverSQLite, "test", []string{"items"}, FormatSQL); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "(1, NULL)") {
		t.Errorf("NULL value not emitted correctly:\n%s", out)
	}
	if !strings.Contains(out, "(2, 'hello')") {
		t.Errorf("non-NULL value not emitted correctly:\n%s", out)
	}
}

func TestDumpSQL_SpecialChars(t *testing.T) {
	s := newTestSQLiteDB(t)
	_, err := s.Exec(`CREATE TABLE stuff (id INTEGER PRIMARY KEY, val TEXT)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Exec(`INSERT INTO stuff (id, val) VALUES (1, 'it''s a test')`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := DumpTables(&buf, s, DriverSQLite, "test", []string{"stuff"}, FormatSQL); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// Single quotes must be doubled in the output.
	if !strings.Contains(out, "'it''s a test'") {
		t.Errorf("single quote not escaped:\n%s", out)
	}
}

func TestDumpSQL_MultipleTables(t *testing.T) {
	s := newTestSQLiteDB(t)
	_, err := s.Exec(`CREATE TABLE alpha (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Exec(`CREATE TABLE beta (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := DumpTables(&buf, s, DriverSQLite, "test", []string{"alpha", "beta"}, FormatSQL); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	alphaIdx := strings.Index(out, `Table: alpha`)
	betaIdx := strings.Index(out, `Table: beta`)
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatalf("missing table sections:\n%s", out)
	}
	if alphaIdx > betaIdx {
		t.Error("tables not in requested order")
	}
}

func TestDumpSQL_UnsupportedFormat(t *testing.T) {
	s := newTestSQLiteDB(t)
	err := DumpTables(&bytes.Buffer{}, s, DriverSQLite, "test", []string{"x"}, Format("xml"))
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestDumpSQL_Roundtrip(t *testing.T) {
	src := newTestSQLiteDB(t)

	// Schema with various features: PK, NOT NULL, DEFAULT, FK.
	_, err := src.Exec(`CREATE TABLE departments (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`CREATE TABLE employees (
		id INTEGER PRIMARY KEY,
		dept_id INTEGER,
		name TEXT NOT NULL,
		note TEXT,
		FOREIGN KEY (dept_id) REFERENCES departments(id)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`INSERT INTO departments (id, name) VALUES (1, 'Eng'), (2, 'Sales')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`INSERT INTO employees (id, dept_id, name, note) VALUES
		(1, 1, 'Alice', 'lead'),
		(2, 1, 'Bob', NULL),
		(3, 2, 'Carol', 'it''s great')`)
	if err != nil {
		t.Fatal(err)
	}

	// Dump the source (departments must come before employees for FK order).
	var buf bytes.Buffer
	if err := DumpTables(&buf, src, DriverSQLite, "test", []string{"departments", "employees"}, FormatSQL); err != nil {
		t.Fatal(err)
	}

	// Restore into a fresh database.
	dst := newTestSQLiteDB(t)
	for _, stmt := range splitSQLStatements(buf.String()) {
		if _, err := dst.Exec(stmt); err != nil {
			t.Fatalf("restore statement failed: %v\nSQL: %s", err, stmt)
		}
	}

	// Verify departments.
	dRes, err := dst.Execute(`SELECT * FROM departments ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(dRes.Rows) != 2 {
		t.Fatalf("departments: expected 2 rows, got %d", len(dRes.Rows))
	}
	if dRes.Rows[0][1] != "Eng" || dRes.Rows[1][1] != "Sales" {
		t.Errorf("departments data mismatch: %v", dRes.Rows)
	}

	// Verify employees including the NULL and escaped quote.
	eRes, err := dst.Execute(`SELECT * FROM employees ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(eRes.Rows) != 3 {
		t.Fatalf("employees: expected 3 rows, got %d", len(eRes.Rows))
	}
	// Row 2 (Bob) should have NULL note.
	if eRes.Rows[1][3] != "NULL" {
		t.Errorf("expected NULL note for Bob, got %q", eRes.Rows[1][3])
	}
	// Row 3 (Carol) should have the unescaped quote.
	if eRes.Rows[2][3] != "it's great" {
		t.Errorf("expected unescaped quote, got %q", eRes.Rows[2][3])
	}

	// Verify FK survived the roundtrip.
	fks, err := dst.ForeignKeys("employees")
	if err != nil {
		t.Fatal(err)
	}
	if len(fks) != 1 || fks[0].Column != "dept_id" || fks[0].RefTable != "departments" {
		t.Errorf("FK mismatch after roundtrip: %v", fks)
	}
}

func TestDumpSQL_ReimportExistingTables(t *testing.T) {
	src := newTestSQLiteDB(t)
	_, err := src.Exec(`CREATE TABLE departments (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`CREATE TABLE employees (
		id INTEGER PRIMARY KEY,
		dept_id INTEGER,
		name TEXT NOT NULL,
		FOREIGN KEY (dept_id) REFERENCES departments(id)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`INSERT INTO departments (id, name) VALUES (1, 'Eng')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`INSERT INTO employees (id, dept_id, name) VALUES (1, 1, 'Alice')`)
	if err != nil {
		t.Fatal(err)
	}

	// Dump the source.
	var buf bytes.Buffer
	if err := DumpTables(&buf, src, DriverSQLite, "test", []string{"departments", "employees"}, FormatSQL); err != nil {
		t.Fatal(err)
	}

	// Reimport into a database that ALREADY has these tables with different data.
	// This simulates the user's scenario of re-running the dump.
	dst := newTestSQLiteDB(t)
	for _, stmt := range splitSQLStatements(buf.String()) {
		if _, err := dst.Exec(stmt); err != nil {
			t.Fatalf("first import failed: %v\nSQL: %s", err, stmt)
		}
	}

	// Now reimport AGAIN into the same dst database (tables already exist).
	for _, stmt := range splitSQLStatements(buf.String()) {
		if _, err := dst.Exec(stmt); err != nil {
			t.Fatalf("second import (reimport) failed: %v\nSQL: %s", err, stmt)
		}
	}

	// Verify data wasn't duplicated.
	eRes, err := dst.Execute(`SELECT * FROM employees ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(eRes.Rows) != 1 || eRes.Rows[0][2] != "Alice" {
		t.Errorf("expected single Alice row after reimport, got: %v", eRes.Rows)
	}
}

func TestDumpSQL_BatchInsert(t *testing.T) {
	s := newTestSQLiteDB(t)
	_, err := s.Exec(`CREATE TABLE nums (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatal(err)
	}
	// Insert 250 rows — should produce 3 INSERT statements (100, 100, 50).
	for i := 1; i <= 250; i++ {
		if _, err := s.Exec(`INSERT INTO nums (id) VALUES (?)`, i); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := DumpTables(&buf, s, DriverSQLite, "test", []string{"nums"}, FormatSQL); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	count := strings.Count(out, "INSERT INTO")
	if count != 3 {
		t.Errorf("expected 3 INSERT statements for 250 rows (batch=100), got %d", count)
	}
}

func TestFormatSQLValue(t *testing.T) {
	tests := []struct {
		value   string
		colType string
		want    string
	}{
		{"NULL", "TEXT", "NULL"},
		{"NULL", "INTEGER", "NULL"},
		{"42", "INTEGER", "42"},
		{"3.14", "REAL", "3.14"},
		{"hello", "TEXT", "'hello'"},
		{"it's", "TEXT", "'it''s'"},
		{"", "TEXT", "''"},
		{"true", "BOOLEAN", "true"},
		// Datetime types normalize ISO-8601 (as emitted by parseTime) to a
		// MySQL/SQLite-compatible 'YYYY-MM-DD HH:MM:SS' literal.
		{"2026-05-08T18:38:00Z", "TIMESTAMP", "'2026-05-08 18:38:00'"},
		{"2026-05-08T18:38:00.123Z", "DATETIME", "'2026-05-08 18:38:00'"},
		// A non-ISO value on a datetime column is left as-is (already plain).
		{"2026-05-08 18:38:00", "DATETIME", "'2026-05-08 18:38:00'"},
		// ISO-ish value on a non-datetime column is NOT reformatted.
		{"2026-05-08T18:38:00Z", "TEXT", "'2026-05-08T18:38:00Z'"},
	}
	for _, tc := range tests {
		got := formatSQLValue(tc.value, tc.colType, DriverSQLite)
		if got != tc.want {
			t.Errorf("formatSQLValue(%q, %q) = %q, want %q", tc.value, tc.colType, got, tc.want)
		}
	}
}

func TestIsDateTimeType(t *testing.T) {
	dateTime := []string{"TIMESTAMP", "timestamp", "DATETIME", "DATETIME(6)", "DATE", "TIME"}
	for _, ty := range dateTime {
		if !IsDateTimeType(ty) {
			t.Errorf("IsDateTimeType(%q) = false, want true", ty)
		}
	}
	other := []string{"TEXT", "VARCHAR(255)", "INT", "", "BLOB", "YEAR"}
	for _, ty := range other {
		if IsDateTimeType(ty) {
			t.Errorf("isDateTimeType(%q) = true, want false", ty)
		}
	}
}

func TestIsNumericType(t *testing.T) {
	numeric := []string{
		"INT", "integer", "BIGINT", "DECIMAL(10,2)", "REAL", "FLOAT", "BOOLEAN", "bool", "TINYINT",
		"BIGINT UNSIGNED", "bigint unsigned", "INT UNSIGNED", "int unsigned",
		"TINYINT UNSIGNED", "SMALLINT UNSIGNED", "MEDIUMINT UNSIGNED",
		"BIGINT(20) UNSIGNED", "INT(11) UNSIGNED", "DECIMAL(10,2) UNSIGNED",
		"unsigned big int", "UNSIGNED BIGINT",
	}
	for _, ty := range numeric {
		if !isNumericType(ty) {
			t.Errorf("isNumericType(%q) = false, want true", ty)
		}
	}
	nonNumeric := []string{"TEXT", "VARCHAR(255)", "BLOB", "", "DATE", "JSON"}
	for _, ty := range nonNumeric {
		if isNumericType(ty) {
			t.Errorf("isNumericType(%q) = true, want false", ty)
		}
	}
}

func TestDumpSQL_PreservesTableConstraints(t *testing.T) {
	s := newTestSQLiteDB(t)
	_, err := s.Exec(`CREATE TABLE items (
		id INTEGER PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		n INTEGER CHECK (n > 0),
		parent_id INTEGER,
		FOREIGN KEY (parent_id) REFERENCES items(id) ON DELETE CASCADE
	)`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := DumpTables(&buf, s, DriverSQLite, "test", []string{"items"}, FormatSQL); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"UNIQUE",
		"CHECK (n > 0)",
		"ON DELETE CASCADE",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dump dropped %q:\n%s", want, out)
		}
	}
}

func TestTableDefinition_SQLite(t *testing.T) {
	s := newTestSQLiteDB(t)
	_, err := s.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT UNIQUE)`)
	if err != nil {
		t.Fatal(err)
	}
	ddl, err := s.TableDefinition("t")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "CREATE TABLE t") {
		t.Errorf("TableDefinition missing CREATE TABLE: %q", ddl)
	}
	if !strings.Contains(ddl, "UNIQUE") {
		t.Errorf("TableDefinition dropped UNIQUE: %q", ddl)
	}
	if strings.HasSuffix(ddl, ";") {
		t.Errorf("TableDefinition should not end with semicolon: %q", ddl)
	}

	missing, err := s.TableDefinition("no_such_table")
	if err != nil {
		t.Fatal(err)
	}
	if missing != "" {
		t.Errorf("missing table: got %q, want empty", missing)
	}
}

func TestFormatSQLValue_MySQLEscapes(t *testing.T) {
	got := formatSQLValue(`App\Models\Auth\User`, "VARCHAR", DriverMySQL)
	want := "'" + `App\\Models\\Auth\\User` + "'"
	if got != want {
		t.Errorf("namespace: got %q want %q", got, want)
	}

	if got := formatSQLValue("it's", "TEXT", DriverMySQL); got != "'"+`it\'s`+"'" {
		t.Errorf("apostrophe: %s", got)
	}

	if got := formatSQLValue("line1\nline2", "TEXT", DriverMySQL); got != "'"+`line1\nline2`+"'" {
		t.Errorf("newline: %s", got)
	}

	if got := formatSQLValue(`foo\`, "TEXT", DriverMySQL); got != "'"+`foo\\`+"'" {
		t.Errorf("trailing backslash: %s", got)
	}

	href := formatSQLValue(`href="shops"'`, "TEXT", DriverMySQL)
	if !strings.Contains(href, `\"`) || !strings.Contains(href, `\'`) {
		t.Errorf("href quotes not escaped: %s", href)
	}
}

func TestFormatSQLValue_UnsignedIntsUnquoted(t *testing.T) {
	got := formatSQLValue("87", "BIGINT UNSIGNED", DriverMySQL)
	if got != "87" {
		t.Errorf("formatSQLValue(87, BIGINT UNSIGNED) = %q, want 87", got)
	}
}

// splitSQLStatements splits a SQL dump into individual executable statements.
// It strips comment lines (-- ...) and tracks single-quote string state so
// semicolons inside string literals are ignored. This is a test helper for
// the roundtrip test and is not a general-purpose SQL parser.
func splitSQLStatements(dump string) []string {
	var lines []string
	for _, line := range strings.Split(dump, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		lines = append(lines, line)
	}
	joined := strings.Join(lines, "\n")

	var stmts []string
	var current strings.Builder
	inString := false
	for i := 0; i < len(joined); i++ {
		c := joined[i]
		if c == '\'' {
			if i+1 < len(joined) && joined[i+1] == '\'' {
				current.WriteByte('\'')
				current.WriteByte('\'')
				i++
				continue
			}
			inString = !inString
		}
		if c == ';' && !inString {
			stmt := strings.TrimSpace(current.String())
			// Skip transaction wrappers and MySQL-only statements that SQLite
			// (used as the restore target in these tests) cannot execute.
			skip := stmt == "" || stmt == "BEGIN" || stmt == "COMMIT"
			skip = skip || strings.HasPrefix(stmt, "SET NAMES") ||
				strings.HasPrefix(stmt, "LOCK TABLES") ||
				stmt == "UNLOCK TABLES"
			if !skip {
				stmts = append(stmts, stmt)
			}
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	return stmts
}
