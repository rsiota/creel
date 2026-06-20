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
