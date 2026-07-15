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

func TestSQLiteForeignKeys(t *testing.T) {
	s := setupTestSQLite(t)

	_, err := s.Exec(`CREATE TABLE departments (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create departments: %v", err)
	}
	_, err = s.Exec(`CREATE TABLE employees (
		id INTEGER PRIMARY KEY,
		dept_id INTEGER,
		name TEXT NOT NULL,
		FOREIGN KEY (dept_id) REFERENCES departments(id)
	)`)
	if err != nil {
		t.Fatalf("create employees: %v", err)
	}

	fks, err := s.ForeignKeys("employees")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	if len(fks) != 1 {
		t.Fatalf("expected 1 FK, got %d", len(fks))
	}
	if fks[0].Column != "dept_id" || fks[0].RefTable != "departments" || fks[0].RefColumn != "id" {
		t.Fatalf("unexpected FK: %+v", fks[0])
	}
}

func TestSQLiteReferencingForeignKeys(t *testing.T) {
	s := setupTestSQLite(t)

	for _, q := range []string{
		`CREATE TABLE departments (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE employees (id INTEGER PRIMARY KEY, dept_id INTEGER, name TEXT, FOREIGN KEY (dept_id) REFERENCES departments(id))`,
		`CREATE TABLE budgets (id INTEGER PRIMARY KEY, dept_id INTEGER, amount REAL, FOREIGN KEY (dept_id) REFERENCES departments(id))`,
		`CREATE TABLE unrelated (id INTEGER PRIMARY KEY)`,
	} {
		if _, err := s.Exec(q); err != nil {
			t.Fatalf("create table: %v\n%s", err, q)
		}
	}

	refs, err := s.ReferencingForeignKeys("departments")
	if err != nil {
		t.Fatalf("ReferencingForeignKeys: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 referrers of departments, got %d: %+v", len(refs), refs)
	}
	seen := map[string]bool{}
	for _, r := range refs {
		if r.Column != "dept_id" || r.RefColumn != "id" {
			t.Errorf("unexpected referrer: %+v", r)
		}
		seen[r.Table] = true
	}
	if !seen["employees"] || !seen["budgets"] {
		t.Errorf("expected employees and budgets; got %v", seen)
	}

	// A table nobody references returns nothing.
	none, err := s.ReferencingForeignKeys("unrelated")
	if err != nil {
		t.Fatalf("ReferencingForeignKeys(unrelated): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 referrers for unrelated, got %d: %+v", len(none), none)
	}
}
