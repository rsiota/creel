package ui

import (
	"testing"

	"github.com/ruben/gsql/internal/db"
)

func TestParseSimpleSelectTable(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`SELECT * FROM users`, "users"},
		{`SELECT * FROM users;`, "users"},
		{`SELECT * FROM users LIMIT 100`, "users"},
		{`SELECT * FROM users LIMIT 100;`, "users"},
		{`SELECT * FROM users ORDER BY id`, "users"},
		{`SELECT id, name FROM users`, "users"},
		{`SELECT id, name FROM users;`, "users"},
		{`SELECT * FROM "my table"`, "my table"},
		{"SELECT * FROM `backtick`", "backtick"},
		// Should return empty (not editable)
		{`SELECT * FROM users WHERE id = 1`, "users"},
		{`SELECT * FROM users WHERE first_name = 'Ruben'`, "users"},
		{`SELECT * FROM users WHERE id = 1 ORDER BY name`, "users"},
		{`SELECT * FROM users JOIN orders ON users.id = orders.user_id`, ""},
		{`SELECT * FROM users GROUP BY name`, ""},
		{`SELECT * FROM (SELECT * FROM users)`, ""},
		{`SELECT * FROM users, orders`, ""},
		{`INSERT INTO users VALUES (1)`, ""},
		{`WITH cte AS (SELECT * FROM users) SELECT * FROM cte`, ""},
		{``, ""},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			got := parseSimpleSelectTable(tc.query)
			if got != tc.want {
				t.Errorf("parseSimpleSelectTable(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestResultsTableEditState(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name", "email"},
		[][]string{
			{"1", "alice", "alice@test.com"},
			{"2", "bob", "bob@test.com"},
			{"3", "carol", "carol@test.com"},
		},
		"3 rows",
	)

	// Before editable
	if r.IsEditable() {
		t.Error("should not be editable before SetEditable")
	}

	// Make editable
	r.SetEditable("users", []string{"id"})
	if !r.IsEditable() {
		t.Error("should be editable after SetEditable")
	}
	if !r.HasPrimaryKey() {
		t.Error("should have primary key after SetEditable with pk cols")
	}

	rNoPK := NewResultsTable()
	rNoPK.SetEditable("notes", nil)
	if !rNoPK.IsEditable() {
		t.Error("should be editable without primary key")
	}
	if rNoPK.HasPrimaryKey() {
		t.Error("should not report primary key when none configured")
	}

	// Cursor starts at (0,0)
	if r.cursorRow != 0 || r.cursorCol != 0 {
		t.Errorf("cursor should be at (0,0), got (%d,%d)", r.cursorRow, r.cursorCol)
	}

	// Move cursor down and right
	r.CursorDown()
	r.CursorDown()
	if r.cursorRow != 2 {
		t.Errorf("cursorRow should be 2, got %d", r.cursorRow)
	}

	// Can't go past last row
	r.CursorDown()
	if r.cursorRow != 2 {
		t.Errorf("cursorRow should still be 2, got %d", r.cursorRow)
	}

	r.CursorRight()
	r.CursorRight()
	if r.cursorCol != 2 {
		t.Errorf("cursorCol should be 2, got %d", r.cursorCol)
	}
	r.CursorRight()
	if r.cursorCol != 2 {
		t.Errorf("cursorCol should still be 2, got %d", r.cursorCol)
	}

	// Start editing — should load current cell value
	r.StartEdit()
	if !r.IsEditing() {
		t.Error("should be editing after StartEdit")
	}
	if r.editInput.Value() != "carol@test.com" {
		t.Errorf("edit input should be 'carol@test.com', got %q", r.editInput.Value())
	}

	// Cancel edit
	r.CancelEdit()
	if r.IsEditing() {
		t.Error("should not be editing after CancelEdit")
	}

	// Edit and commit
	r.StartEdit()
	r.editInput.SetValue("new@email.com")
	r.CommitEdit()
	if r.IsEditing() {
		t.Error("should not be editing after CommitEdit")
	}

	// Check dirty cell
	if !r.HasDirtyCells() {
		t.Error("should have dirty cells after commit")
	}
	val := r.RowValue(2, 2)
	if val != "new@email.com" {
		t.Errorf("RowValue(2,2) should be 'new@email.com', got %q", val)
	}

	// Original value should still be there for non-dirty cells
	val = r.RowValue(0, 1)
	if val != "alice" {
		t.Errorf("RowValue(0,1) should be 'alice', got %q", val)
	}

	// DirtyCells should return the edit
	edits := r.DirtyCells()
	if len(edits) != 1 {
		t.Fatalf("expected 1 dirty cell, got %d", len(edits))
	}
	if edits[0].Row != 2 || edits[0].Col != 2 || edits[0].NewValue != "new@email.com" {
		t.Errorf("unexpected edit: %+v", edits[0])
	}

	// ConfirmSaved should clear dirty cells
	r.ConfirmSaved()
	if r.HasDirtyCells() {
		t.Error("should not have dirty cells after ConfirmSaved")
	}
}

func TestResultsTableNullEdit(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "NULL"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})

	// Move cursor to the NULL cell (row 0, col 1)
	r.CursorRight()

	// Editing a NULL cell should start with empty buffer
	r.StartEdit()
	if r.editInput.Value() != "" {
		t.Errorf("editing NULL cell should start empty, got %q", r.editInput.Value())
	}
	r.editInput.SetValue("now set")
	r.CommitEdit()

	edits := r.DirtyCells()
	if len(edits) != 1 || edits[0].NewValue != "now set" {
		t.Errorf("unexpected edit: %+v", edits)
	}
}

func TestCursorCellValue(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "name"},
		[][]string{
			{"1", "alice"},
			{"2", "bob"},
		},
		"2 rows",
	)

	if got := r.CursorCellValue(); got != "1" {
		t.Fatalf("at (0,0) want %q, got %q", "1", got)
	}

	r.CursorDown()
	r.CursorRight()
	if got := r.CursorCellValue(); got != "bob" {
		t.Fatalf("at (1,1) want %q, got %q", "bob", got)
	}

	r.SetDirtyCell(1, 1, "edited")
	if got := r.CursorCellValue(); got != "edited" {
		t.Fatalf("dirty cell want %q, got %q", "edited", got)
	}
}

func TestCopyFeedback(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}},
		"1 row",
	)
	r.cursorRow = 0
	r.cursorCol = 1

	r.StartCopyFeedback()
	if !r.copied {
		t.Fatal("expected copied confirmation")
	}
	if !r.isCopyFlashCell(0, 1) || !r.copyFlashOn {
		t.Fatal("expected flashing cell to start highlighted")
	}

	for i := 0; i < copyFlashTickCount-1; i++ {
		if !r.AdvanceCopyFlash() {
			t.Fatalf("expected more flash ticks at step %d", i)
		}
	}
	if r.AdvanceCopyFlash() {
		t.Fatal("expected flash to finish after all ticks")
	}
	if r.copyFlashActive || r.isCopyFlashCell(0, 1) {
		t.Fatal("expected flash to stop after all ticks")
	}

	r.StartCopyFeedback()
	r.ClearCopiedMessage()
	if r.copied {
		t.Fatal("expected copied confirmation to clear")
	}
}

func TestResultsForeignKeys(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "dept_id", "name"},
		[][]string{{"1", "10", "alice"}},
		"1 row",
	)
	r.SetForeignKeys("employees", []db.ForeignKey{
		{Column: "dept_id", RefTable: "departments", RefColumn: "id"},
	})
	r.cursorCol = 1

	if !r.HasForeignKeys() {
		t.Fatal("expected foreign keys")
	}
	if !r.IsNavigableForeignKey(0, 1) {
		t.Fatal("dept_id should be navigable")
	}
	if r.IsNavigableForeignKey(0, 0) {
		t.Fatal("id should not be a foreign key cell")
	}

	fk, ok := r.ForeignKeyAtCursor()
	if !ok || fk.RefTable != "departments" {
		t.Fatalf("unexpected FK at cursor: %+v", fk)
	}
}
