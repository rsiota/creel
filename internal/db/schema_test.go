package db

import (
	"strings"
	"testing"
)

func TestValidateAddColumn(t *testing.T) {
	tests := []struct {
		name     string
		col      ColumnDef
		existing []string
		wantErr  string
	}{
		{
			name: "valid nullable",
			col:  ColumnDef{Name: "nickname", Type: "TEXT"},
		},
		{
			name: "valid not null with default",
			col:  ColumnDef{Name: "score", Type: "INTEGER", NotNull: true, HasDefault: true, Default: "0"},
		},
		{
			name:    "missing name",
			col:     ColumnDef{Type: "TEXT"},
			wantErr: "name is required",
		},
		{
			name:    "invalid name",
			col:     ColumnDef{Name: "1bad", Type: "TEXT"},
			wantErr: "invalid",
		},
		{
			name:     "duplicate",
			col:      ColumnDef{Name: "email", Type: "TEXT"},
			existing: []string{"id", "email"},
			wantErr:  "already exists",
		},
		{
			name:    "missing type",
			col:     ColumnDef{Name: "bio", Type: "  "},
			wantErr: "type is required",
		},
		{
			name:    "not null without default",
			col:     ColumnDef{Name: "bio", Type: "TEXT", NotNull: true},
			wantErr: "require a default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAddColumn(tc.col, tc.existing)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildAddColumnSQL_SQLite(t *testing.T) {
	sql, err := BuildAddColumnSQL(DriverSQLite, "users", ColumnDef{
		Name: "nickname",
		Type: "TEXT",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER TABLE "users" ADD COLUMN "nickname" TEXT`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}

	sql, err = BuildAddColumnSQL(DriverSQLite, "users", ColumnDef{
		Name:       "active",
		Type:       "INTEGER",
		NotNull:    true,
		HasDefault: true,
		Default:    "1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want = `ALTER TABLE "users" ADD COLUMN "active" INTEGER NOT NULL DEFAULT 1`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}

	sql, err = BuildAddColumnSQL(DriverSQLite, "users", ColumnDef{
		Name:       "label",
		Type:       "TEXT",
		HasDefault: true,
		Default:    "draft",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want = `ALTER TABLE "users" ADD COLUMN "label" TEXT DEFAULT 'draft'`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestBuildAddColumnSQL_MySQL(t *testing.T) {
	sql, err := BuildAddColumnSQL(DriverMySQL, "users", ColumnDef{
		Name: "nickname",
		Type: "VARCHAR(50)",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER TABLE `users` ADD COLUMN `nickname` VARCHAR(50)"
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}

	sql, err = BuildAddColumnSQL(DriverMySQL, "users", ColumnDef{
		Name:       "status",
		Type:       "INT",
		NotNull:    true,
		HasDefault: true,
		Default:    "NULL",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want = "ALTER TABLE `users` ADD COLUMN `status` INT NOT NULL DEFAULT NULL"
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestBuildRenameColumnSQL(t *testing.T) {
	sql, err := BuildRenameColumnSQL(DriverSQLite, "users", "email", "email_addr", []string{"id", "email"})
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER TABLE "users" RENAME COLUMN "email" TO "email_addr"`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestBuildRenameTableSQL(t *testing.T) {
	sql, err := BuildRenameTableSQL(DriverSQLite, "users", "accounts", []string{"users", "orders"})
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER TABLE "users" RENAME TO "accounts"`
	if sql != want {
		t.Fatalf("sqlite sql = %q, want %q", sql, want)
	}

	sql, err = BuildRenameTableSQL(DriverMySQL, "users", "accounts", []string{"users", "orders"})
	if err != nil {
		t.Fatal(err)
	}
	want = "RENAME TABLE `users` TO `accounts`"
	if sql != want {
		t.Fatalf("mysql sql = %q, want %q", sql, want)
	}

	_, err = BuildRenameTableSQL(DriverSQLite, "users", "users", []string{"users"})
	if err == nil {
		t.Fatal("expected error for same name")
	}

	_, err = BuildRenameTableSQL(DriverSQLite, "users", "orders", []string{"users", "orders"})
	if err == nil {
		t.Fatal("expected error for duplicate table")
	}
}

func TestBuildModifyColumnSQL(t *testing.T) {
	sql, err := BuildModifyColumnSQL(DriverMySQL, "users", ColumnDef{
		Name:    "bio",
		Type:    "VARCHAR(500)",
		NotNull: true,
		HasDefault: true,
		Default: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER TABLE `users` MODIFY COLUMN `bio` VARCHAR(500) NOT NULL DEFAULT 'hello'"
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestBuildDropColumnSQL(t *testing.T) {
	_, err := BuildDropColumnSQL(DriverMySQL, "users", "id", TableColumnInfo{
		Name: "id", PrimaryKey: true,
	})
	if err == nil {
		t.Fatal("expected error dropping PK")
	}

	sql, err := BuildDropColumnSQL(DriverMySQL, "users", "nickname", TableColumnInfo{Name: "nickname"})
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER TABLE `users` DROP COLUMN `nickname`"
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestBuildDropTableSQL(t *testing.T) {
	// Empty table name is rejected.
	if _, err := BuildDropTableSQL(DriverSQLite, "  "); err == nil {
		t.Fatal("expected error for empty table name")
	}

	// SQLite uses double-quote identifiers.
	sql, err := BuildDropTableSQL(DriverSQLite, "users")
	if err != nil {
		t.Fatal(err)
	}
	if want := `DROP TABLE "users"`; sql != want {
		t.Fatalf("sqlite sql = %q, want %q", sql, want)
	}

	// MySQL uses backtick identifiers.
	sql, err = BuildDropTableSQL(DriverMySQL, "users")
	if err != nil {
		t.Fatal(err)
	}
	if want := "DROP TABLE `users`"; sql != want {
		t.Fatalf("mysql sql = %q, want %q", sql, want)
	}
}

func TestSchemaActionNeedsConfirm(t *testing.T) {
	if !SchemaActionNeedsConfirm(SchemaDropColumn) {
		t.Fatal("drop column should require confirm")
	}
	if SchemaActionNeedsConfirm(SchemaRenameColumn) {
		t.Fatal("rename should run directly from the form")
	}
}

func TestSchemaSupports(t *testing.T) {
	if !SchemaSupports(DriverSQLite, SchemaAddColumn) {
		t.Fatal("sqlite should support add column")
	}
	if SchemaSupports(DriverSQLite, SchemaModifyType) {
		t.Fatal("sqlite should not support modify type")
	}
	if !SchemaSupports(DriverMySQL, SchemaDropColumn) {
		t.Fatal("mysql should support drop column")
	}
	if !SchemaSupports(DriverSQLite, SchemaCreateTable) {
		t.Fatal("sqlite should support create table")
	}
	if !SchemaSupports(DriverMySQL, SchemaCreateTable) {
		t.Fatal("mysql should support create table")
	}
	if !SchemaSupports(DriverSQLite, SchemaDropTable) {
		t.Fatal("sqlite should support drop table")
	}
	if !SchemaSupports(DriverMySQL, SchemaDropTable) {
		t.Fatal("mysql should support drop table")
	}
	if SchemaActionLabel(SchemaDropTable) != "Drop table" {
		t.Fatal("drop table label mismatch")
	}
	actions := ColumnSchemaActions(DriverSQLite)
	if len(actions) != 1 || actions[0] != SchemaRenameColumn {
		t.Fatalf("sqlite actions = %v", actions)
	}
}

func TestValidateCreateTable(t *testing.T) {
	tests := []struct {
		name      string
		table     string
		cols      []ColumnDef
		existing  []string
		wantErr   string
	}{
		{
			name:  "valid single column",
			table: "accounts",
			cols:  []ColumnDef{{Name: "id", Type: "INTEGER"}},
		},
		{
			name:  "valid not null without default",
			table: "accounts",
			cols:  []ColumnDef{{Name: "id", Type: "INTEGER", NotNull: true}},
		},
		{
			name:  "valid with blank rows skipped",
			table: "accounts",
			cols: []ColumnDef{
				{Name: "id", Type: "INTEGER"},
				{Name: "name", Type: "TEXT"},
				{}, // trailing blank row
			},
		},
		{
			name:    "empty table name",
			table:   "  ",
			cols:    []ColumnDef{{Name: "id", Type: "INTEGER"}},
			wantErr: "name is required",
		},
		{
			name:    "invalid table name",
			table:   "1bad",
			cols:    []ColumnDef{{Name: "id", Type: "INTEGER"}},
			wantErr: "invalid",
		},
		{
			name:     "duplicate table",
			table:    "users",
			cols:     []ColumnDef{{Name: "id", Type: "INTEGER"}},
			existing: []string{"users", "orders"},
			wantErr:  "already exists",
		},
		{
			name:    "no columns",
			table:   "accounts",
			cols:    nil,
			wantErr: "at least one column",
		},
		{
			name:    "only blank rows",
			table:   "accounts",
			cols:    []ColumnDef{{}, {}},
			wantErr: "at least one column",
		},
		{
			name:    "column missing name",
			table:   "accounts",
			cols:    []ColumnDef{{Type: "TEXT"}},
			wantErr: "column name is required",
		},
		{
			name:    "column missing type",
			table:   "accounts",
			cols:    []ColumnDef{{Name: "id"}},
			wantErr: "type is required",
		},
		{
			name:    "duplicate column",
			table:   "accounts",
			cols:    []ColumnDef{{Name: "id", Type: "INT"}, {Name: "ID", Type: "TEXT"}},
			wantErr: "duplicated",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateTable(tc.table, tc.cols, tc.existing)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildCreateTableSQL_SQLite(t *testing.T) {
	sql, err := BuildCreateTableSQL(DriverSQLite, "accounts", []ColumnDef{
		{Name: "id", Type: "INTEGER", NotNull: true},
		{Name: "name", Type: "TEXT"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE TABLE "accounts" (
    "id" INTEGER NOT NULL,
    "name" TEXT
)`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}

	// Defaults and NOT NULL both present, plus a trailing blank row that must
	// be dropped from the output.
	sql, err = BuildCreateTableSQL(DriverSQLite, "accounts", []ColumnDef{
		{Name: "active", Type: "INTEGER", NotNull: true, HasDefault: true, Default: "1"},
		{Name: "label", Type: "TEXT", HasDefault: true, Default: "draft"},
		{},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want = `CREATE TABLE "accounts" (
    "active" INTEGER NOT NULL DEFAULT 1,
    "label" TEXT DEFAULT 'draft'
)`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestBuildCreateTableSQL_MySQL(t *testing.T) {
	sql, err := BuildCreateTableSQL(DriverMySQL, "accounts", []ColumnDef{
		{Name: "id", Type: "INT", NotNull: true},
		{Name: "email", Type: "VARCHAR(255)"},
	}, []string{"users"})
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE TABLE `accounts` (\n    `id` INT NOT NULL,\n    `email` VARCHAR(255)\n)"
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}

	// Duplicate table name is rejected.
	_, err = BuildCreateTableSQL(DriverMySQL, "users", []ColumnDef{{Name: "id", Type: "INT"}}, []string{"users"})
	if err == nil {
		t.Fatal("expected error for duplicate table")
	}
}

func TestFormatDefault(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"NULL", "NULL"},
		{"null", "NULL"},
		{"42", "42"},
		{"3.14", "3.14"},
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{"CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP"},
		{"current_timestamp", "current_timestamp"},
		{"CURRENT_TIMESTAMP(3)", "CURRENT_TIMESTAMP(3)"},
		{"CURRENT_DATE", "CURRENT_DATE"},
		{"now()", "now()"},
		{"uuid()", "uuid()"},
		{"2024-01-01 00:00:00", "'2024-01-01 00:00:00'"},
	}
	for _, tc := range tests {
		got := formatDefault(tc.input)
		if got != tc.want {
			t.Errorf("formatDefault(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
