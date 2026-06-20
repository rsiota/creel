package db

import "testing"

func TestSQLiteAddColumn(t *testing.T) {
	s := setupTestSQLite(t)

	sql, err := BuildAddColumnSQL(DriverSQLite, "users", ColumnDef{
		Name: "nickname",
		Type: "TEXT",
	}, []string{"id", "name", "email"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Exec(sql); err != nil {
		t.Fatalf("add column: %v", err)
	}

	cols, err := s.TableSchema("users")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cols {
		if c.Name == "nickname" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected nickname column, got %v", cols)
	}
}

func TestSQLiteRenameColumn(t *testing.T) {
	s := setupTestSQLite(t)

	sql, err := BuildRenameColumnSQL(DriverSQLite, "users", "email", "email_addr", []string{"id", "name", "email"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Exec(sql); err != nil {
		t.Fatalf("rename: %v", err)
	}

	cols, err := s.TableSchema("users")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cols {
		if c.Name == "email_addr" {
			found = true
		}
		if c.Name == "email" {
			t.Fatal("old column name still present")
		}
	}
	if !found {
		t.Fatalf("expected email_addr, got %v", cols)
	}
}

func TestSQLiteRenameTable(t *testing.T) {
	s := setupTestSQLite(t)

	sql, err := BuildRenameTableSQL(DriverSQLite, "users", "accounts", []string{"users", "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Exec(sql); err != nil {
		t.Fatalf("rename table: %v", err)
	}

	tables, err := s.Tables()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range tables {
		if name == "accounts" {
			found = true
		}
		if name == "users" {
			t.Fatal("old table name still present")
		}
	}
	if !found {
		t.Fatalf("expected accounts table, got %v", tables)
	}
}
