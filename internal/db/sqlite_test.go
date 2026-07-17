package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func setupSQLiteTestDB(t *testing.T) *SQLite {
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
	_, err := s.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = s.Exec(`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, total REAL, FOREIGN KEY (user_id) REFERENCES users(id))`)
	if err != nil {
		t.Fatalf("create orders: %v", err)
	}
	_, err = s.Exec(`INSERT INTO users (name, email) VALUES ('alice', 'alice@test.com'), ('bob', 'bob@test.com'), ('carol', NULL)`)
	if err != nil {
		t.Fatalf("insert users: %v", err)
	}
	_, err = s.Exec(`INSERT INTO orders (id, user_id, total) VALUES (1, 1, 99.50), (2, 2, 0.0), (3, 1, 200.0)`)
	if err != nil {
		t.Fatalf("insert orders: %v", err)
	}
	return s
}

func TestSQLiteTables(t *testing.T) {
	s := setupSQLiteTestDB(t)
	tables, err := s.Tables()
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	// sqlite_sequence is auto-created when AUTOINCREMENT is used
	hasUsers, hasOrders := false, false
	for _, t := range tables {
		if t == "users" {
			hasUsers = true
		}
		if t == "orders" {
			hasOrders = true
		}
	}
	if !hasUsers || !hasOrders {
		t.Errorf("expected users and orders in tables, got %v", tables)
	}
}

func TestSQLiteTableSchema(t *testing.T) {
	s := setupSQLiteTestDB(t)
	cols, err := s.TableSchema("users")
	if err != nil {
		t.Fatalf("TableSchema: %v", err)
	}
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
	expectedNames := map[string]bool{"id": false, "name": false, "email": false}
	for _, c := range cols {
		if _, ok := expectedNames[c.Name]; ok {
			expectedNames[c.Name] = true
		}
	}
	for name, found := range expectedNames {
		if !found {
			t.Errorf("column %q not found in schema", name)
		}
	}
}

func TestSQLiteTableSchema_NonExistent(t *testing.T) {
	s := setupSQLiteTestDB(t)
	cols, err := s.TableSchema("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cols) != 0 {
		t.Errorf("expected 0 columns for nonexistent table, got %d", len(cols))
	}
}

func TestSQLitePKs(t *testing.T) {
	s := setupSQLiteTestDB(t)
	pks, err := s.PrimaryKeys("users")
	if err != nil {
		t.Fatalf("PrimaryKeys: %v", err)
	}
	if len(pks) != 1 || pks[0] != "id" {
		t.Errorf("expected ['id'], got %v", pks)
	}
}

func TestSQLitePKs_NoPK(t *testing.T) {
	s := setupSQLiteTestDB(t)
	_, err := s.Exec(`CREATE TABLE notes (text TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	pks, err := s.PrimaryKeys("notes")
	if err != nil {
		t.Fatalf("PrimaryKeys: %v", err)
	}
	if len(pks) != 0 {
		t.Errorf("expected 0 PKs, got %v", pks)
	}
}

func TestSQLiteFKs(t *testing.T) {
	s := setupSQLiteTestDB(t)
	fks, err := s.ForeignKeys("orders")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	if len(fks) != 1 {
		t.Fatalf("expected 1 FK, got %d", len(fks))
	}
	if fks[0].Column != "user_id" || fks[0].RefTable != "users" || fks[0].RefColumn != "id" {
		t.Errorf("unexpected FK: %+v", fks[0])
	}
}

func TestSQLiteFKs_None(t *testing.T) {
	s := setupSQLiteTestDB(t)
	fks, err := s.ForeignKeys("users")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	if len(fks) != 0 {
		t.Errorf("expected 0 FKs, got %d", len(fks))
	}
}

func TestSQLiteTableColumnInfo(t *testing.T) {
	s := setupSQLiteTestDB(t)
	cols, err := s.TableColumnInfo("users")
	if err != nil {
		t.Fatalf("TableColumnInfo: %v", err)
	}
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}

	// Find the 'id' column
	var idCol TableColumnInfo
	for _, c := range cols {
		if c.Name == "id" {
			idCol = c
			break
		}
	}
	if idCol.Name != "id" {
		t.Fatal("id column not found")
	}
	if !idCol.PrimaryKey {
		t.Error("id should be primary key")
	}
	if !idCol.AutoIncrement {
		t.Error("id should be auto-increment (INTEGER PRIMARY KEY)")
	}

	// Find the 'name' column
	var nameCol TableColumnInfo
	for _, c := range cols {
		if c.Name == "name" {
			nameCol = c
			break
		}
	}
	if !nameCol.NotNull {
		t.Error("name should be NOT NULL")
	}
}

func TestSQLiteExecute(t *testing.T) {
	s := setupSQLiteTestDB(t)
	result, err := s.Execute("SELECT * FROM users ORDER BY id")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(result.Columns))
	}
	if len(result.Rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(result.Rows))
	}
	if result.Rows[0][1] != "alice" {
		t.Errorf("first row name should be 'alice', got %q", result.Rows[0][1])
	}
	// Carol has NULL email
	if result.Rows[2][2] != "NULL" {
		t.Errorf("carol's email should be NULL, got %q", result.Rows[2][2])
	}
}

func TestSQLiteExecute_Error(t *testing.T) {
	s := setupSQLiteTestDB(t)
	_, err := s.Execute("SELECT * FROM nonexistent_table")
	if err == nil {
		t.Error("expected error for nonexistent table")
	}
}

func TestSQLiteExecuteContext(t *testing.T) {
	s := setupSQLiteTestDB(t)
	result, err := s.ExecuteContext(context.Background(), "SELECT COUNT(*) FROM users")
	if err != nil {
		t.Fatalf("ExecuteContext: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != "3" {
		t.Errorf("expected count 3, got %q", result.Rows[0][0])
	}
}

func TestSQLiteExecuteContext_Cancelled(t *testing.T) {
	s := setupSQLiteTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err := s.ExecuteContext(ctx, "SELECT * FROM users")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestSQLiteExecInsert(t *testing.T) {
	s := setupSQLiteTestDB(t)
	res, err := s.Exec("INSERT INTO users (name, email) VALUES ('dave', 'dave@test.com')")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", res.RowsAffected)
	}

	// Verify the insert
	result, _ := s.Execute("SELECT name FROM users WHERE email = 'dave@test.com'")
	if len(result.Rows) != 1 || result.Rows[0][0] != "dave" {
		t.Errorf("insert verification failed: %v", result.Rows)
	}
}

func TestSQLiteExec_Update(t *testing.T) {
	s := setupSQLiteTestDB(t)
	res, err := s.Exec("UPDATE users SET email = 'updated@test.com' WHERE name = 'alice'")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", res.RowsAffected)
	}
}

func TestSQLiteExec_Delete(t *testing.T) {
	s := setupSQLiteTestDB(t)
	res, err := s.Exec("DELETE FROM users WHERE name = 'bob'")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", res.RowsAffected)
	}
	// Verify
	result, _ := s.Execute("SELECT COUNT(*) FROM users")
	if result.Rows[0][0] != "2" {
		t.Errorf("expected 2 users after delete, got %q", result.Rows[0][0])
	}
}

func TestSQLiteTableRowCounts(t *testing.T) {
	s := setupSQLiteTestDB(t)
	counts, err := s.TableRowCounts()
	if err != nil {
		t.Fatalf("TableRowCounts: %v", err)
	}
	if counts["users"] != 3 {
		t.Errorf("expected 3 users, got %d", counts["users"])
	}
	if counts["orders"] != 3 {
		t.Errorf("expected 3 orders, got %d", counts["orders"])
	}
}

func TestSQLiteDatabases(t *testing.T) {
	s := setupSQLiteTestDB(t)
	dbs, err := s.Databases()
	if err != nil {
		t.Fatalf("Databases: %v", err)
	}
	if len(dbs) != 1 {
		t.Fatalf("expected 1 database, got %d", len(dbs))
	}
}

func TestSQLiteUseDatabase(t *testing.T) {
	s := setupSQLiteTestDB(t)
	err := s.UseDatabase("other")
	if err == nil {
		t.Error("UseDatabase should error for SQLite")
	}
}

func TestSQLiteSchemas(t *testing.T) {
	s := setupSQLiteTestDB(t)
	if _, err := s.Schemas(); err == nil {
		t.Error("Schemas should error for SQLite")
	}
	if err := s.UseSchema("main"); err == nil {
		t.Error("UseSchema should error for SQLite")
	}
}

func TestSQLiteViews(t *testing.T) {
	s := setupSQLiteTestDB(t)
	if _, err := s.Exec(`CREATE VIEW active_users AS SELECT id, name FROM users`); err != nil {
		t.Fatal(err)
	}
	views, err := s.Views()
	if err != nil {
		t.Fatalf("Views: %v", err)
	}
	if len(views) != 1 || views[0] != "active_users" {
		t.Errorf("Views = %v, want [active_users]", views)
	}
}

func TestSQLiteCloseTwice(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := ConnectionConfig{Driver: DriverSQLite, Database: dbPath}
	s := NewSQLite(cfg)
	if err := s.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("first close: %v", err)
	}
	// Second close should be safe (nil db after first close)
	if err := s.Close(); err != nil {
		t.Errorf("second close should be safe: %v", err)
	}
}
