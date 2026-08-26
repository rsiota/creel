package db

import "testing"

func TestFormatTableDiskSize(t *testing.T) {
	if got := FormatTableDiskSize(-1); got != "—" {
		t.Errorf("unknown = %q, want —", got)
	}
	if got := FormatTableDiskSize(0); got != "0B" {
		t.Errorf("zero = %q, want 0B", got)
	}
	if got := FormatTableDiskSize(1536); got != "1.5KB" {
		t.Errorf("1536 = %q, want 1.5KB", got)
	}
}

func TestSQLiteTableSizes(t *testing.T) {
	s := setupSQLiteTestDB(t)
	sizes, err := s.TableSizes()
	if err != nil {
		t.Fatalf("TableSizes: %v", err)
	}
	if len(sizes) < 2 {
		t.Fatalf("expected at least users and orders, got %v", sizes)
	}
	byName := make(map[string]TableSize, len(sizes))
	for _, ts := range sizes {
		byName[ts.Name] = ts
	}
	users, ok := byName["users"]
	if !ok {
		t.Fatal("missing users table")
	}
	if users.Rows != 3 {
		t.Errorf("users rows = %d, want 3", users.Rows)
	}
	if users.RowsApprox {
		t.Error("sqlite row counts should be exact")
	}
	if users.DiskBytes <= 0 {
		t.Errorf("users disk = %d, want positive bytes", users.DiskBytes)
	}
	orders := byName["orders"]
	if orders.Rows != 3 {
		t.Errorf("orders rows = %d, want 3", orders.Rows)
	}
}
