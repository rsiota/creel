package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestBuildInsertQuery(t *testing.T) {
	columns := []db.TableColumnInfo{
		{Name: "id", Type: "INTEGER", PrimaryKey: true, AutoIncrement: true, NotNull: true},
		{Name: "name", Type: "TEXT", NotNull: true},
		{Name: "email", Type: "TEXT"},
		{Name: "status", Type: "TEXT", HasDefault: true},
	}

	t.Run("includes provided values and skips auto increment", func(t *testing.T) {
		query, args, err := buildInsertQuery(db.DriverSQLite, "users", columns, map[string]string{
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
		_, _, err := buildInsertQuery(db.DriverSQLite, "users", columns, map[string]string{
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
		_, _, err := buildInsertQuery(db.DriverSQLite, "users", optional, map[string]string{})
		if err == nil || !strings.Contains(err.Error(), "no values to insert") {
			t.Fatalf("expected no values error, got %v", err)
		}
	})

	t.Run("NULL values map to SQL nil args", func(t *testing.T) {
		query, args, err := buildInsertQuery(db.DriverSQLite, "users", columns, map[string]string{
			"name":  "alice",
			"email": "NULL",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if query != "INSERT INTO users (name, email) VALUES (?, ?)" {
			t.Fatalf("query = %q", query)
		}
		if len(args) != 2 || args[0] != "alice" || args[1] != nil {
			t.Fatalf("args = %#v, want [alice, nil]", args)
		}
	})

	t.Run("datetime values are normalized from ISO-8601", func(t *testing.T) {
		dtColumns := []db.TableColumnInfo{
			{Name: "id", Type: "INTEGER", PrimaryKey: true, AutoIncrement: true},
			{Name: "created_at", Type: "DATETIME", NotNull: true},
			{Name: "updated_at", Type: "TIMESTAMP"},
		}
		_, args, err := buildInsertQuery(db.DriverSQLite, "logs", dtColumns, map[string]string{
			"created_at": "2026-01-07T15:04:30Z",
			"updated_at": "2026-01-07T15:04:30Z",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(args) != 2 || args[0] != "2026-01-07 15:04:30" || args[1] != "2026-01-07 15:04:30" {
			t.Fatalf("args = %#v, want normalized datetime strings", args)
		}
	})

	t.Run("empty table name", func(t *testing.T) {
		_, _, err := buildInsertQuery(db.DriverSQLite, "", columns, map[string]string{"name": "alice"})
		if err == nil || !strings.Contains(err.Error(), "no table") {
			t.Fatalf("expected no table error, got %v", err)
		}
	})

	t.Run("postgres uses numbered placeholders", func(t *testing.T) {
		query, _, err := buildInsertQuery(db.DriverPostgres, "users", columns, map[string]string{
			"name":  "alice",
			"email": "alice@test.com",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if query != "INSERT INTO users (name, email) VALUES ($1, $2)" {
			t.Fatalf("query = %q", query)
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
