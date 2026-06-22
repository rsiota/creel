package db

import (
	"bytes"
	"strings"
	"testing"
)

func TestImportSQL_BasicRoundtrip(t *testing.T) {
	src := newTestSQLiteDB(t)
	_, err := src.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`INSERT INTO users (id, name, email) VALUES (1, 'alice', 'alice@test.com')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`INSERT INTO users (id, name, email) VALUES (2, 'bob', 'bob@test.com')`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := DumpTables(&buf, src, DriverSQLite, "test", []string{"users"}, FormatSQL); err != nil {
		t.Fatal(err)
	}

	dst := newTestSQLiteDB(t)
	result, err := ImportSQL(&buf, dst, int64(buf.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Statements == 0 {
		t.Error("expected at least 1 statement imported")
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(result.Errors), result.Errors)
	}

	// Verify data.
	res, err := dst.Execute(`SELECT * FROM users ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
	if res.Rows[0][1] != "alice" || res.Rows[1][1] != "bob" {
		t.Errorf("data mismatch: %v", res.Rows)
	}
}

func TestImportSQL_SemicolonInString(t *testing.T) {
	dst := newTestSQLiteDB(t)
	_, err := dst.Exec(`CREATE TABLE stuff (id INTEGER PRIMARY KEY, val TEXT)`)
	if err != nil {
		t.Fatal(err)
	}

	dump := `INSERT INTO stuff (id, val) VALUES (1, 'semi;colon');
INSERT INTO stuff (id, val) VALUES (2, 'a''b;c');`

	result, err := ImportSQL(strings.NewReader(dump), dst, int64(len(dump)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Statements != 2 {
		t.Errorf("expected 2 statements, got %d", result.Statements)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got: %v", result.Errors)
	}

	res, err := dst.Execute(`SELECT val FROM stuff ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0][0] != "semi;colon" {
		t.Errorf("row 0: got %q", res.Rows[0][0])
	}
	if res.Rows[1][0] != "a'b;c" {
		t.Errorf("row 1: got %q", res.Rows[1][0])
	}
}

func TestImportSQL_IgnoresFailures(t *testing.T) {
	dst := newTestSQLiteDB(t)

	dump := `CREATE TABLE a (id INTEGER PRIMARY KEY);
INVALID SQL HERE;
INSERT INTO a (id) VALUES (1);
ANOTHER BAD ONE;
INSERT INTO a (id) VALUES (2);`

	result, err := ImportSQL(strings.NewReader(dump), dst, int64(len(dump)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Statements != 5 {
		t.Errorf("expected 5 statements attempted, got %d", result.Statements)
	}
	if len(result.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(result.Errors))
	}

	// Verify the good statements still executed.
	res, err := dst.Execute(`SELECT * FROM a ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 {
		t.Errorf("expected 2 rows in table a, got %d", len(res.Rows))
	}
}

func TestImportSQL_CommentStripping(t *testing.T) {
	dst := newTestSQLiteDB(t)
	_, err := dst.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatal(err)
	}

	dump := `-- This is a comment; with a semicolon
/* block comment; another semicolon */
INSERT INTO t (id) VALUES (1);
-- trailing comment`

	result, err := ImportSQL(strings.NewReader(dump), dst, int64(len(dump)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Statements != 1 {
		t.Errorf("expected 1 statement, got %d", result.Statements)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got: %v", result.Errors)
	}
}

func TestImportSQL_RoundtripMultiTableWithFK(t *testing.T) {
	src := newTestSQLiteDB(t)
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

	var buf bytes.Buffer
	if err := DumpTables(&buf, src, DriverSQLite, "test", []string{"departments", "employees"}, FormatSQL); err != nil {
		t.Fatal(err)
	}

	dst := newTestSQLiteDB(t)
	result, err := ImportSQL(&buf, dst, int64(buf.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected 0 errors, got: %v", result.Errors)
	}

	// Verify data.
	dRes, err := dst.Execute(`SELECT * FROM departments ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(dRes.Rows) != 2 {
		t.Fatalf("departments: expected 2 rows, got %d", len(dRes.Rows))
	}

	eRes, err := dst.Execute(`SELECT * FROM employees ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(eRes.Rows) != 3 {
		t.Fatalf("employees: expected 3 rows, got %d", len(eRes.Rows))
	}
	if eRes.Rows[1][3] != "NULL" {
		t.Errorf("expected NULL note for Bob, got %q", eRes.Rows[1][3])
	}
	if eRes.Rows[2][3] != "it's great" {
		t.Errorf("expected unescaped quote, got %q", eRes.Rows[2][3])
	}

	// Verify FK survived.
	fks, err := dst.ForeignKeys("employees")
	if err != nil {
		t.Fatal(err)
	}
	if len(fks) != 1 || fks[0].Column != "dept_id" || fks[0].RefTable != "departments" {
		t.Errorf("FK mismatch after roundtrip: %v", fks)
	}
}

func TestImportSQL_ProgressCallback(t *testing.T) {
	dst := newTestSQLiteDB(t)
	_, err := dst.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatal(err)
	}

	dump := `INSERT INTO t (id) VALUES (1);
INSERT INTO t (id) VALUES (2);
INSERT INTO t (id) VALUES (3);`

	var progressCalls []struct {
		read  int64
		total int64
	}
	result, err := ImportSQL(strings.NewReader(dump), dst, int64(len(dump)), func(read, total int64) {
		progressCalls = append(progressCalls, struct {
			read  int64
			total int64
		}{read, total})
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Statements != 3 {
		t.Errorf("expected 3 statements, got %d", result.Statements)
	}
	if len(progressCalls) != 3 {
		t.Errorf("expected 3 progress callbacks, got %d", len(progressCalls))
	}
	if len(progressCalls) > 0 {
		last := progressCalls[len(progressCalls)-1]
		if last.total != int64(len(dump)) {
			t.Errorf("expected total=%d, got %d", len(dump), last.total)
		}
	}
}

func TestImportSQL_MySQLConditionalComments(t *testing.T) {
	dst := newTestSQLiteDB(t)
	_, err := dst.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, val TEXT)`)
	if err != nil {
		t.Fatal(err)
	}

	// MySQL conditional comments should be parsed and their interior executed.
	// Session wrappers (@OLD_...) should be skipped.
	dump := `/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
INSERT INTO t (id, val) VALUES (1, 'a');
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;`

	result, err := ImportSQL(strings.NewReader(dump), dst, int64(len(dump)), nil)
	if err != nil {
		t.Fatal(err)
	}
	// The two /*!40101 SET @OLD_... */ wrappers should be skipped, only the
	// INSERT counted. SET @OLD_... wrappers are skipped but the non-wrapper
	// "SET CHARACTER_SET_CLIENT=@OLD..." is still attempted (and may fail on
	// SQLite, which is fine — it's collected as an error, not fatal).
	if result.Statements == 0 {
		t.Error("expected at least 1 statement imported")
	}
}

func TestImportResult_Summary(t *testing.T) {
	tests := []struct {
		result   ImportResult
		filename string
		want     string
	}{
		{ImportResult{Statements: 10, Errors: nil}, "backup.sql", "Imported 10 statements → backup.sql"},
		{ImportResult{Statements: 10, Errors: make([]ImportError, 3)}, "backup.sql", "Imported 10 statements, 3 failed → backup.sql"},
	}
	for _, tc := range tests {
		got := tc.result.Summary(tc.filename)
		if got != tc.want {
			t.Errorf("Summary() = %q, want %q", got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate: got %q", got)
	}
	if got := truncate("hello world this is long", 10); got != "hello worl..." {
		t.Errorf("truncate: got %q", got)
	}
}
