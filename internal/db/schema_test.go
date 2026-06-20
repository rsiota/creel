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
	actions := ColumnSchemaActions(DriverSQLite)
	if len(actions) != 1 || actions[0] != SchemaRenameColumn {
		t.Fatalf("sqlite actions = %v", actions)
	}
}
