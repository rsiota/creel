package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/creel/internal/db"
)

func TestTableDesignerSubmit(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, []string{"users", "orders"})
	d.SetSize(80, 24)

	d.nameField.SetValue("accounts")
	// Pre-filled id row + blank row. Edit the blank row.
	d.rows[1][tdColName] = "name"
	d.rows[1][tdColType] = "TEXT"

	sql, errMsg := d.Submit()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	want := `CREATE TABLE "accounts" (
    "id" INTEGER PRIMARY KEY NOT NULL,
    "name" TEXT
)`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}

func TestTableDesignerSubmitBlankRowsSkipped(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)

	d.nameField.SetValue("t")
	// Keep id row, leave the blank row empty.
	sql, errMsg := d.Submit()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if !strings.Contains(sql, `"id" INTEGER PRIMARY KEY NOT NULL`) {
		t.Fatalf("expected id column in sql, got %q", sql)
	}
	if strings.Count(sql, "\n") != 2 {
		t.Fatalf("expected only id column (2 newlines), got %q", sql)
	}
}

func TestTableDesignerRejectsEmptyName(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)

	_, errMsg := d.Submit()
	if errMsg == "" {
		t.Fatal("expected empty name error")
	}
}

func TestTableDesignerRejectsDuplicateTable(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, []string{"users"})
	d.nameField.SetValue("users")

	_, errMsg := d.Submit()
	if errMsg == "" || !strings.Contains(errMsg, "already exists") {
		t.Fatalf("expected duplicate table error, got %q", errMsg)
	}
}

func TestTableDesignerRejectsDuplicateColumn(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)
	d.nameField.SetValue("t")
	d.rows[1][tdColName] = "id" // same as pre-filled row
	d.rows[1][tdColType] = "TEXT"

	_, errMsg := d.Submit()
	if errMsg == "" || !strings.Contains(errMsg, "duplicated") {
		t.Fatalf("expected duplicate column error, got %q", errMsg)
	}
}

func TestTableDesignerAddRowBelow(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)
	startRows := len(d.rows)

	d, _ = d.Update(pressKey("j")) // leave name field
	d, _ = d.Update(pressKey("o")) // add row below

	if len(d.rows) != startRows+1 {
		t.Fatalf("expected %d rows, got %d", startRows+1, len(d.rows))
	}
	if d.cursorRow != 1 {
		t.Fatalf("expected cursor on row 1, got %d", d.cursorRow)
	}
}

func TestTableDesignerAddRowAbove(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)
	startRows := len(d.rows)

	d, _ = d.Update(pressKey("j")) // leave name field → row 0
	d, _ = d.Update(pressKey("O")) // add row above

	if len(d.rows) != startRows+1 {
		t.Fatalf("expected %d rows, got %d", startRows+1, len(d.rows))
	}
	if d.cursorRow != 0 {
		t.Fatalf("expected cursor stayed at 0, got %d", d.cursorRow)
	}
}

func TestTableDesignerRemoveRow(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)
	// Move to row 1 and dd-remove it.
	d, _ = d.Update(pressKey("j")) // row 0
	d, _ = d.Update(pressKey("j")) // row 1
	startRows := len(d.rows)

	d, _ = d.Update(pressKey("d"))
	d, _ = d.Update(pressKey("d"))

	if len(d.rows) != startRows-1 {
		t.Fatalf("expected %d rows after dd, got %d", startRows-1, len(d.rows))
	}
}

func TestTableDesignerRemoveRowKeepsAtLeastOne(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)
	// Force single row.
	d.rows = d.rows[:1]
	d.cursorRow = 0
	d, _ = d.Update(pressKey("j")) // ensure on grid
	d.cursorRow = 0

	d, _ = d.Update(pressKey("d"))
	d, _ = d.Update(pressKey("d"))

	if len(d.rows) != 1 {
		t.Fatalf("expected 1 row preserved, got %d", len(d.rows))
	}
}

func TestTableDesignerInlineEdit(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)
	d, _ = d.Update(pressKey("j")) // grid, row 0
	d, _ = d.Update(pressKey("l")) // col 1 (type)

	d, _ = d.Update(pressKey("e"))
	if !d.editing {
		t.Fatal("expected editing after 'e'")
	}

	// Clear the pre-filled value and type a new one.
	d.editInput.SetValue("")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("VARCHAR(80)")})

	d, _ = d.Update(pressKey("enter"))
	if d.editing {
		t.Fatal("expected editing=false after enter")
	}
	if d.rows[0][tdColType] != "VARCHAR(80)" {
		t.Fatalf("expected type VARCHAR(80), got %q", d.rows[0][tdColType])
	}
}

func TestTableDesignerEscCancelsEdit(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)
	d, _ = d.Update(pressKey("j"))
	d, _ = d.Update(pressKey("e"))

	d, _ = d.Update(pressKey("esc"))
	if d.editing {
		t.Fatal("expected editing=false after esc")
	}
}

func TestTableDesignerMySQLQuoting(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverMySQL, []string{"users"})
	d.nameField.SetValue("accounts")

	sql, errMsg := d.Submit()
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if !strings.Contains(sql, "`accounts`") {
		t.Fatalf("expected mysql backticks, got %q", sql)
	}
}

func TestTableDesignerNameToGridNavigation(t *testing.T) {
	d := NewTableDesigner()
	d.Show(db.DriverSQLite, nil)

	if !d.focusName {
		t.Fatal("expected focusName=true on show")
	}

	// Down moves to grid.
	d, _ = d.Update(pressKey("j"))
	if d.focusName {
		t.Fatal("expected focusName=false after j")
	}
	if d.cursorRow != 0 || d.cursorCol != 0 {
		t.Fatalf("expected cursor 0,0 got %d,%d", d.cursorRow, d.cursorCol)
	}

	// Up from row 0 returns to name.
	d, _ = d.Update(pressKey("k"))
	if !d.focusName {
		t.Fatal("expected focusName=true after up from row 0")
	}
}

// pressKey builds a tea.KeyMsg for the given key string.
func pressKey(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key), Alt: false}
}
