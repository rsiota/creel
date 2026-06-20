package ui

import (
	"testing"

	"github.com/ruben/gsql/internal/db"
)

func TestColumnEditFormRename(t *testing.T) {
	f := NewColumnEditForm()
	f.Show(db.SchemaRenameColumn, "users", db.DriverSQLite, db.TableColumnInfo{
		Name: "email",
		Type: "TEXT",
	}, []string{"id", "email"})
	f.fields[0].SetValue("email_address")

	sql, errMsg := f.Submit()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	want := `ALTER TABLE "users" RENAME COLUMN "email" TO "email_address"`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestColumnEditFormModifyType(t *testing.T) {
	f := NewColumnEditForm()
	f.Show(db.SchemaModifyType, "users", db.DriverMySQL, db.TableColumnInfo{
		Name:   "bio",
		Type:   "text",
		NotNull: false,
	}, nil)
	f.fields[0].SetValue("VARCHAR(500)")

	sql, errMsg := f.Submit()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	want := "ALTER TABLE `users` MODIFY COLUMN `bio` VARCHAR(500) NULL"
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestGuardColumnActionDropPK(t *testing.T) {
	msg := GuardColumnAction(db.SchemaDropColumn, db.TableColumnInfo{
		Name:       "id",
		PrimaryKey: true,
	})
	if msg == "" {
		t.Fatal("expected guard error for PK drop")
	}
}
