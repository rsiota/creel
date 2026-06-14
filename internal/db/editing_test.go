package db

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestSQLite(t *testing.T) *SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := ConnectionConfig{Driver: DriverSQLite, Database: dbPath}
	s := NewSQLite(cfg)
	if err := s.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { s.Close(); os.Remove(dbPath) })

	_, err := s.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = s.Exec(`INSERT INTO users (id, name, email) VALUES (1, 'alice', 'alice@test.com')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = s.Exec(`INSERT INTO users (id, name, email) VALUES (2, 'bob', 'bob@test.com')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return s
}

func TestSQLiteExec(t *testing.T) {
	s := setupTestSQLite(t)

	// UPDATE
	res, err := s.Exec(`UPDATE users SET name = ? WHERE id = ?`, "alice2", 1)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", res.RowsAffected)
	}

	// Verify
	result, err := s.Execute(`SELECT name FROM users WHERE id = 1`)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "alice2" {
		t.Errorf("expected alice2, got %v", result.Rows)
	}

	// INSERT
	res, err = s.Exec(`INSERT INTO users (id, name, email) VALUES (?, ?, ?)`, 3, "carol", "carol@test.com")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", res.RowsAffected)
	}

	// DELETE
	res, err = s.Exec(`DELETE FROM users WHERE id = ?`, 3)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", res.RowsAffected)
	}
}

func TestSQLiteExecParameterized(t *testing.T) {
	s := setupTestSQLite(t)

	// Test that parameterized queries prevent injection
	_, err := s.Exec(`UPDATE users SET name = ? WHERE id = ?`, "'; DROP TABLE users;--", 1)
	if err != nil {
		t.Fatalf("update with special chars: %v", err)
	}

	// Verify table still exists
	_, err = s.Execute(`SELECT * FROM users`)
	if err != nil {
		t.Fatalf("table should still exist after parameterized update: %v", err)
	}
}

func TestSQLitePrimaryKeys(t *testing.T) {
	s := setupTestSQLite(t)

	pks, err := s.PrimaryKeys("users")
	if err != nil {
		t.Fatalf("PrimaryKeys: %v", err)
	}
	if len(pks) != 1 || pks[0] != "id" {
		t.Errorf("expected [id], got %v", pks)
	}

	// Composite PK
	_, err = s.Exec(`CREATE TABLE orders (user_id INTEGER, product_id INTEGER, qty INTEGER, PRIMARY KEY (user_id, product_id))`)
	if err != nil {
		t.Fatalf("create composite pk table: %v", err)
	}
	pks, err = s.PrimaryKeys("orders")
	if err != nil {
		t.Fatalf("PrimaryKeys composite: %v", err)
	}
	if len(pks) != 2 {
		t.Errorf("expected 2 PKs, got %d: %v", len(pks), pks)
	}
}
