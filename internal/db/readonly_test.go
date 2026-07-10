package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsWriteQuery(t *testing.T) {
	cases := []struct {
		q    string
		want bool
	}{
		// reads
		{"SELECT 1", false},
		{"select * from t", false},
		{"  (SELECT 1)", false},
		{"WITH x AS (SELECT 1) SELECT * FROM x", false},
		{"SHOW TABLES", false},
		{"EXPLAIN SELECT * FROM t", false},
		{"-- comment\nSELECT 1", false},
		{"/* leading */ SELECT 1", false},

		// writes (DML)
		{"INSERT INTO t VALUES (1)", true},
		{"insert into t values (1)", true},
		{"UPDATE t SET a=1", true},
		{"DELETE FROM t", true},
		{"REPLACE INTO t VALUES (1)", true},
		{"MERGE INTO t USING s ON t.id=s.id WHEN MATCHED THEN UPDATE SET a=1", true},

		// writes (DDL / admin)
		{"CREATE TABLE t (a INT)", true},
		{"DROP TABLE t", true},
		{"ALTER TABLE t ADD COLUMN a INT", true},
		{"TRUNCATE TABLE t", true},
		{"GRANT SELECT ON t TO bob", true},
		{"REVOKE SELECT ON t FROM bob", true},
		{"VACUUM", true},
		{"REINDEX", true},
		{"PRAGMA query_only = OFF", true}, // must be blocked: would bypass guard

		// CTE write
		{"WITH x AS (SELECT 1) INSERT INTO t SELECT * FROM x", true},

		// word-boundary: UPDATED_AT is a column, not UPDATE
		{"SELECT UPDATED_AT FROM t", false},
	}
	for _, c := range cases {
		got := isWriteQuery(c.q)
		if got != c.want {
			t.Errorf("isWriteQuery(%q) = %v, want %v", c.q, got, c.want)
		}
	}
}

func TestRejectWriteIfReadOnly(t *testing.T) {
	cfg := ConnectionConfig{ReadOnly: true}
	if err := rejectWriteIfReadOnly(cfg, "SELECT 1"); err != nil {
		t.Errorf("read rejected in read-only mode: %v", err)
	}
	if err := rejectWriteIfReadOnly(cfg, "DELETE FROM t"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("write not rejected in read-only mode; got %v", err)
	}
	// Read-only off: nothing rejected.
	cfg.ReadOnly = false
	if err := rejectWriteIfReadOnly(cfg, "DELETE FROM t"); err != nil {
		t.Errorf("write rejected when not read-only: %v", err)
	}
}

// setupReadOnlySQLite opens a pre-populated SQLite file, then reopens it with
// ReadOnly=true so the guard and engine-level query_only pragma are both active.
func setupReadOnlySQLite(t *testing.T) *SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ro.db")

	// Seed with a normal connection.
	seed := NewSQLite(ConnectionConfig{Driver: DriverSQLite, Database: dbPath})
	if err := seed.Connect(); err != nil {
		t.Fatalf("seed connect: %v", err)
	}
	if _, err := seed.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := seed.Exec(`INSERT INTO t (v) VALUES ('a'), ('b')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("seed close: %v", err)
	}

	// Reopen read-only.
	s := NewSQLite(ConnectionConfig{Driver: DriverSQLite, Database: dbPath, ReadOnly: true})
	if err := s.Connect(); err != nil {
		t.Fatalf("readonly connect: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
		os.Remove(dbPath)
	})
	return s
}

func TestSQLiteReadOnlyRejectsWrites(t *testing.T) {
	s := setupReadOnlySQLite(t)

	// Exec on a write is rejected by the guard with ErrReadOnly.
	if _, err := s.Exec(`INSERT INTO t (v) VALUES ('c')`); !errors.Is(err, ErrReadOnly) {
		t.Errorf("INSERT Exec: want ErrReadOnly, got %v", err)
	}
	if _, err := s.Exec(`UPDATE t SET v='x'`); !errors.Is(err, ErrReadOnly) {
		t.Errorf("UPDATE Exec: want ErrReadOnly, got %v", err)
	}
	if _, err := s.Exec(`DROP TABLE t`); !errors.Is(err, ErrReadOnly) {
		t.Errorf("DROP Exec: want ErrReadOnly, got %v", err)
	}

	// A write through the query-box path (Execute) is also rejected.
	if _, err := s.ExecuteContext(context.Background(), `DELETE FROM t`); !errors.Is(err, ErrReadOnly) {
		t.Errorf("DELETE Execute: want ErrReadOnly, got %v", err)
	}
}

func TestSQLiteReadOnlyAllowsReads(t *testing.T) {
	s := setupReadOnlySQLite(t)

	res, err := s.Execute(`SELECT v FROM t ORDER BY v`)
	if err != nil {
		t.Fatalf("SELECT rejected in read-only mode: %v", err)
	}
	if len(res.Rows) != 2 || res.Rows[0][0] != "a" {
		t.Errorf("unexpected rows: %v", res.Rows)
	}
}

func TestSQLiteReadOnlyBlocksTxnAndSession(t *testing.T) {
	s := setupReadOnlySQLite(t)

	if _, err := s.Begin(); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Begin: want ErrReadOnly, got %v", err)
	}
	if _, err := s.Session(); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Session: want ErrReadOnly, got %v", err)
	}
}

// TestSQLiteReadOnlyEngineEnforced verifies the engine-level query_only pragma
// catches a write even if the application guard were bypassed: we reach past
// the interface and run the INSERT on the underlying *sql.DB directly. It must
// still fail (SQLite refuses the write).
func TestSQLiteReadOnlyEngineEnforced(t *testing.T) {
	s := setupReadOnlySQLite(t)
	_, err := s.db.Exec(`INSERT INTO t (v) VALUES ('direct')`)
	if err == nil {
		t.Error("direct INSERT succeeded despite query_only engine enforcement")
	}
}
