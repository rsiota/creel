package ui

import (
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestInsertRowKeybindingOnEmptyTable(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.results.SetResult(
		[]string{"id", "name"},
		nil,
		"0 rows",
	)
	m.results.SetEditable("users", []string{"id"})
	m.results.SetTableColumns([]db.TableColumnInfo{
		{Name: "id", Type: "INTEGER", PrimaryKey: true, AutoIncrement: true},
		{Name: "name", Type: "TEXT", NotNull: true},
	})

	m = press(m, keyRunes('A'))

	if !m.inspector.IsInserting() {
		t.Fatal("expected insert mode after A on empty editable table")
	}
	if m.focus != FocusInspector {
		t.Fatalf("focus = %v, want FocusInspector", m.focus)
	}
}

func TestInsertRowKeybindingWithoutPrimaryKey(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.results.SetResult(
		[]string{"name"},
		nil,
		"0 rows",
	)
	m.results.SetEditable("notes", nil)
	m.results.SetTableColumns([]db.TableColumnInfo{
		{Name: "name", Type: "TEXT", NotNull: true},
	})

	m = press(m, keyRunes('A'))

	if !m.inspector.IsInserting() {
		t.Fatal("expected insert mode after A on table without primary key")
	}
}
