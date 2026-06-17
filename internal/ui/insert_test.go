package ui

import (
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/db"
)

func TestBuildInsertQuery(t *testing.T) {
	columns := []db.TableColumnInfo{
		{Name: "id", Type: "INTEGER", PrimaryKey: true, AutoIncrement: true, NotNull: true},
		{Name: "name", Type: "TEXT", NotNull: true},
		{Name: "email", Type: "TEXT"},
		{Name: "status", Type: "TEXT", HasDefault: true},
	}

	t.Run("includes provided values and skips auto increment", func(t *testing.T) {
		query, args, err := buildInsertQuery("users", columns, map[string]string{
			"name":  "alice",
			"email": "alice@test.com",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if query != "INSERT INTO users (name, email) VALUES (?, ?)" {
			t.Fatalf("query = %q", query)
		}
		if len(args) != 2 || args[0] != "alice" || args[1] != "alice@test.com" {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("required column without value", func(t *testing.T) {
		_, _, err := buildInsertQuery("users", columns, map[string]string{
			"email": "alice@test.com",
		})
		if err == nil || !strings.Contains(err.Error(), `column "name" is required`) {
			t.Fatalf("expected required column error, got %v", err)
		}
	})

	t.Run("no values to insert", func(t *testing.T) {
		optional := []db.TableColumnInfo{
			{Name: "id", Type: "INTEGER", PrimaryKey: true, AutoIncrement: true},
			{Name: "status", Type: "TEXT", HasDefault: true},
		}
		_, _, err := buildInsertQuery("users", optional, map[string]string{})
		if err == nil || !strings.Contains(err.Error(), "no values to insert") {
			t.Fatalf("expected no values error, got %v", err)
		}
	})

	t.Run("empty table name", func(t *testing.T) {
		_, _, err := buildInsertQuery("", columns, map[string]string{"name": "alice"})
		if err == nil || !strings.Contains(err.Error(), "no table") {
			t.Fatalf("expected no table error, got %v", err)
		}
	})
}

func TestInsertValuesByName(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}},
		"1 row",
	)

	out := insertValuesByName(r, map[int]string{1: "bob"})
	if out["name"] != "bob" {
		t.Fatalf("name = %q, want bob", out["name"])
	}
	if _, ok := out["id"]; ok {
		t.Fatal("unexpected id in output")
	}
}

func TestInspectorInsertMode(t *testing.T) {
	ins := NewInspector()
	r := NewResultsTable()
	r.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}}, "1 row")
	r.SetEditable("users", []string{"id"})
	r.SetTableColumns([]db.TableColumnInfo{
		{Name: "id", Type: "INTEGER", PrimaryKey: true, AutoIncrement: true},
		{Name: "name", Type: "TEXT", NotNull: true},
	})

	ins.StartInsert()
	if !ins.IsInserting() {
		t.Fatal("expected insert mode")
	}

	ins.cursorField = 1
	ins.StartFieldEdit(r)
	if !ins.IsEditing() {
		t.Fatal("expected field edit")
	}
	ins.editInput.SetValue("carol")
	col, val, ok := ins.CommitFieldEdit()
	if !ok || col != 1 || val != "carol" {
		t.Fatalf("commit = (%d, %q, %v)", col, val, ok)
	}
	if ins.InsertValues()[1] != "carol" {
		t.Fatalf("insert value = %q", ins.InsertValues()[1])
	}

	ins.cursorField = 0
	ins.StartFieldEdit(r)
	if ins.IsEditing() {
		t.Fatal("auto-increment column should not be editable")
	}

	ins.CancelInsert()
	if ins.IsInserting() {
		t.Fatal("expected insert mode cancelled")
	}
}
