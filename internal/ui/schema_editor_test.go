package ui

import (
	"strings"
	"testing"

	"github.com/ruben/creel/internal/db"
)

func TestSchemaEditorRenameColumn(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverSQLite, []db.TableColumnInfo{
		{Name: "id", Type: "INTEGER", NotNull: true, PrimaryKey: true},
		{Name: "email", Type: "TEXT"},
	})

	// Navigate to email row, name column, edit it.
	e.cursorRow = 1
	e.cursorCol = seColName
	e.startCellEdit()
	e.editInput.SetValue("email_addr")
	e.commitCellEdit()

	sql, action, errMsg := e.PendingEditDDL()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	want := `ALTER TABLE "users" RENAME COLUMN "email" TO "email_addr"`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
	if action != db.SchemaRenameColumn {
		t.Fatalf("action = %v, want SchemaRenameColumn", action)
	}
}

func TestSchemaEditorModifyType(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "email", Type: "VARCHAR(100)"},
	})

	e.cursorRow = 0
	e.cursorCol = seColType
	e.startCellEdit()
	e.editInput.SetValue("VARCHAR(255)")
	e.commitCellEdit()

	sql, action, errMsg := e.PendingEditDDL()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if !strings.Contains(sql, "MODIFY COLUMN `email` VARCHAR(255)") {
		t.Fatalf("sql = %q, want MODIFY COLUMN", sql)
	}
	if action != db.SchemaModifyType {
		t.Fatalf("action = %v, want SchemaModifyType", action)
	}
}

func TestSchemaEditorModifyNullable(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "email", Type: "VARCHAR(255)", NotNull: true},
	})

	e.cursorRow = 0
	e.cursorCol = seColNull
	e.startCellEdit()
	e.editInput.SetValue("NULL")
	e.commitCellEdit()

	sql, action, errMsg := e.PendingEditDDL()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if !strings.Contains(sql, "MODIFY COLUMN") || strings.Contains(sql, "NOT NULL") {
		t.Fatalf("sql = %q, expected nullable MODIFY", sql)
	}
	if action != db.SchemaModifyNullable {
		t.Fatalf("action = %v, want SchemaModifyNullable", action)
	}
}

func TestSchemaEditorModifyDefault(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "status", Type: "VARCHAR(20)"},
	})

	e.cursorRow = 0
	e.cursorCol = seColDefault
	e.startCellEdit()
	e.editInput.SetValue("active")
	e.commitCellEdit()

	sql, action, errMsg := e.PendingEditDDL()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if !strings.Contains(sql, "DEFAULT 'active'") {
		t.Fatalf("sql = %q, expected DEFAULT 'active'", sql)
	}
	if action != db.SchemaModifyDefault {
		t.Fatalf("action = %v, want SchemaModifyDefault", action)
	}
}

func TestSchemaEditorNoChangeProducesNoDDL(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "email", Type: "VARCHAR(255)"},
	})

	// Don't actually change anything.
	e.cursorRow = 0
	e.cursorCol = seColName
	sql, _, _ := e.PendingEditDDL()
	if sql != "" {
		t.Fatalf("expected empty SQL when no change, got %q", sql)
	}
}

func TestSchemaEditorSQLiteTypeReadOnly(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverSQLite, []db.TableColumnInfo{
		{Name: "email", Type: "TEXT"},
	})

	// On SQLite, type is not editable.
	if e.cellEditable(0, seColType) {
		t.Fatal("expected type cell to be read-only on SQLite")
	}
	// Move cursor to type column and try to edit — should be blocked.
	e.cursorCol = seColType
	e.startCellEdit()
	if e.editing {
		t.Fatal("expected editing=false on read-only cell")
	}
	if e.notice == "" {
		t.Fatal("expected a notice explaining the restriction")
	}
}

func TestSchemaEditorSQLiteNameEditable(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverSQLite, []db.TableColumnInfo{
		{Name: "email", Type: "TEXT"},
	})

	if !e.cellEditable(0, seColName) {
		t.Fatal("expected name cell to be editable on SQLite")
	}
}

func TestSchemaEditorAutoIncrementNotEditable(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "id", Type: "INT", AutoIncrement: true, PrimaryKey: true},
	})

	for _, col := range []int{seColName, seColType, seColNull, seColDefault} {
		if e.cellEditable(0, col) {
			t.Fatalf("expected col %d to be read-only for auto-increment", col)
		}
	}
}

func TestSchemaEditorNavigation(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "a", Type: "INT"},
		{Name: "b", Type: "INT"},
		{Name: "c", Type: "INT"},
	})

	if e.cursorRow != 0 || e.cursorCol != 0 {
		t.Fatalf("start at 0,0 got %d,%d", e.cursorRow, e.cursorCol)
	}

	e, _ = e.Update(pressKey("j"))
	if e.cursorRow != 1 {
		t.Fatalf("after j: row=%d, want 1", e.cursorRow)
	}

	e, _ = e.Update(pressKey("l"))
	if e.cursorCol != 1 {
		t.Fatalf("after l: col=%d, want 1", e.cursorCol)
	}

	e, _ = e.Update(pressKey("k"))
	if e.cursorRow != 0 {
		t.Fatalf("after k: row=%d, want 0", e.cursorRow)
	}

	e, _ = e.Update(pressKey("h"))
	if e.cursorCol != 0 {
		t.Fatalf("after h: col=%d, want 0", e.cursorCol)
	}
}

func TestSchemaEditorInlineEditCycle(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "email", Type: "VARCHAR(100)"},
	})

	e, _ = e.Update(pressKey("l")) // move to type col
	e, _ = e.Update(pressKey("e"))
	if !e.editing {
		t.Fatal("expected editing after 'e'")
	}

	e.editInput.SetValue("TEXT")
	e, _ = e.Update(pressKey("enter"))

	if e.editing {
		t.Fatal("expected editing=false after enter")
	}
	if e.rows[0][seColType] != "TEXT" {
		t.Fatalf("expected type TEXT, got %q", e.rows[0][seColType])
	}
}

func TestSchemaEditorEscCancelsEdit(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "email", Type: "VARCHAR(100)"},
	})

	e, _ = e.Update(pressKey("e"))
	e, _ = e.Update(pressKey("esc"))
	if e.editing {
		t.Fatal("expected editing=false after esc")
	}
}

func TestSchemaEditorSetColumnsAfterDDL(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "email", Type: "VARCHAR(100)"},
	})

	// Simulate post-DDL refresh with updated metadata.
	e.SetColumns([]db.TableColumnInfo{
		{Name: "email", Type: "VARCHAR(255)"},
	})
	if e.rows[0][seColType] != "VARCHAR(255)" {
		t.Fatalf("expected refreshed type, got %q", e.rows[0][seColType])
	}
}

func TestSchemaEditorPendingDropColumn(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "id", Type: "INT", PrimaryKey: true},
		{Name: "bio", Type: "TEXT"},
	})

	e.cursorRow = 1
	col, ok := e.PendingDropColumn()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if col.Name != "bio" {
		t.Fatalf("expected bio, got %q", col.Name)
	}
}

func TestSchemaEditorAddRowBelow(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "id", Type: "INT"},
		{Name: "name", Type: "TEXT"},
	})

	startRows := len(e.rows)
	e.cursorRow = 0
	e.addRowBelow()

	if len(e.rows) != startRows+1 {
		t.Fatalf("expected %d rows, got %d", startRows+1, len(e.rows))
	}
	if e.cursorRow != 1 {
		t.Fatalf("expected cursor on row 1, got %d", e.cursorRow)
	}
	if !e.IsNewRow() {
		t.Fatal("expected new row at cursor")
	}
}

func TestSchemaEditorNewRowBuildsAddColumn(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverSQLite, []db.TableColumnInfo{
		{Name: "id", Type: "INTEGER"},
	})

	// Add a new row, fill in the cells.
	e.addRowBelow()
	e.rows[1][seColName] = "email"
	e.rows[1][seColType] = "TEXT"
	e.rows[1][seColNull] = "yes"

	sql, action, errMsg := e.PendingEditDDL()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if !strings.Contains(sql, "ADD COLUMN") {
		t.Fatalf("expected ADD COLUMN, got %q", sql)
	}
	if !strings.Contains(sql, `"email"`) {
		t.Fatalf("expected email in SQL, got %q", sql)
	}
	if action != db.SchemaAddColumn {
		t.Fatalf("action = %v, want SchemaAddColumn", action)
	}
}

func TestSchemaEditorNewRowValidatesRequiredFields(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverSQLite, []db.TableColumnInfo{
		{Name: "id", Type: "INTEGER"},
	})

	e.addRowBelow()
	e.rows[1][seColName] = "email"
	// Type left blank.

	_, _, errMsg := e.PendingEditDDL()
	if errMsg == "" {
		t.Fatal("expected validation error for missing type")
	}
}

func TestSchemaEditorNewRowAllCellsEditable(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverSQLite, []db.TableColumnInfo{
		{Name: "id", Type: "INTEGER"},
	})

	e.addRowBelow()
	for col := 0; col < seColCount; col++ {
		if !e.cellEditable(1, col) {
			t.Fatalf("expected col %d editable on new row (even on SQLite)", col)
		}
	}
}

func TestSchemaEditorNewRowRemovedByDD(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "id", Type: "INT"},
	})

	e.addRowBelow()
	startRows := len(e.rows)

	// dd on a new row removes it locally.
	e, _ = e.Update(pressKey("d"))
	e, _ = e.Update(pressKey("d"))

	if len(e.rows) != startRows-1 {
		t.Fatalf("expected %d rows after dd, got %d", startRows-1, len(e.rows))
	}
}

func TestSchemaEditorPendingDropColumnRejectsNewRow(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "id", Type: "INT"},
	})

	e.addRowBelow()
	_, ok := e.PendingDropColumn()
	if ok {
		t.Fatal("expected ok=false for new row (no confirm needed)")
	}
}
