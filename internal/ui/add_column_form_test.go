package ui

import (
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestAddColumnFormSubmit(t *testing.T) {
	f := NewAddColumnForm()
	f.Show("users", db.DriverSQLite, []string{"id", "name"})

	f.fields[acFieldName].SetValue("nickname")
	f.fields[acFieldType].SetValue("TEXT")
	f.fields[acFieldNullable].SetValue("yes")
	f.fields[acFieldDefault].SetValue("")

	sql, errMsg := f.Submit()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	want := `ALTER TABLE "users" ADD COLUMN "nickname" TEXT`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestAddColumnFormSubmitNotNullRequiresDefault(t *testing.T) {
	f := NewAddColumnForm()
	f.Show("users", db.DriverSQLite, nil)
	f.fields[acFieldName].SetValue("score")
	f.fields[acFieldType].SetValue("INTEGER")
	f.fields[acFieldNullable].SetValue("no")
	f.fields[acFieldDefault].SetValue("")

	_, errMsg := f.Submit()
	if errMsg == "" {
		t.Fatal("expected validation error")
	}
}

func TestAddColumnFormSubmitDuplicateName(t *testing.T) {
	f := NewAddColumnForm()
	f.Show("users", db.DriverSQLite, []string{"email"})
	f.fields[acFieldName].SetValue("email")
	f.fields[acFieldType].SetValue("TEXT")

	_, errMsg := f.Submit()
	if errMsg == "" {
		t.Fatal("expected duplicate column error")
	}
}
