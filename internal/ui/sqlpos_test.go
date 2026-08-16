package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rsiota/creel/internal/db"
)

func TestJumpToQueryPosSingleStatement(t *testing.T) {
	e := NewQueryEditor()
	e.SetValue("SELECT * FORM users")
	if !e.JumpToQueryPos("SELECT * FORM users", 0, 9) {
		t.Fatal("jump failed")
	}
	line, col := e.CursorScreenPos()
	if line != 0 || col != 9 {
		t.Errorf("cursor = %d,%d want 0,9", line, col)
	}
}

func TestJumpToQueryPosMultiStatement(t *testing.T) {
	e := NewQueryEditor()
	e.SetValue("SELECT 1;\nSELECT * FORM users;\nSELECT 2;")
	stmt := "SELECT * FORM users"
	if !e.JumpToQueryPos(stmt, 0, 9) {
		t.Fatal("jump failed")
	}
	line, col := e.CursorScreenPos()
	if line != 1 || col != 9 {
		t.Errorf("cursor = %d,%d want 1,9", line, col)
	}
}

func TestMaybeJumpToQueryErrorMySQL(t *testing.T) {
	m := &Model{
		editor:  NewQueryEditor(),
		results: NewResultsTable(),
		focus:   FocusResults,
	}
	m.editor.SetValue("SELECT *\nFORM users")
	err := fmt.Errorf("query error: %w", &mysql.MySQLError{
		Number:  1064,
		Message: "You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near 'FORM users' at line 2",
	})
	m.maybeJumpToQueryError(err, m.editor.Value(), m.editor.Value())
	line, col := m.editor.CursorScreenPos()
	if line != 1 {
		t.Errorf("line = %d, want 1", line)
	}
	if col != 0 {
		t.Errorf("col = %d, want 0", col)
	}
	if m.focus != FocusEditor {
		t.Errorf("focus = %v, want editor", m.focus)
	}
	if !strings.Contains(m.schemaMsg, "jumped") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestMaybeJumpToQueryErrorPostgresWrapped(t *testing.T) {
	user := "SELECT * FORM users"
	exec := db.PageWrapPrefix + user + ") AS _creel_page LIMIT 51 OFFSET 0"
	m := &Model{editor: NewQueryEditor(), results: NewResultsTable()}
	m.editor.SetValue(user)
	innerOff := len(db.PageWrapPrefix) + 9
	err := &pgconn.PgError{
		Message:  `syntax error at or near "FORM"`,
		Position: int32(innerOff + 1),
	}
	m.maybeJumpToQueryError(err, user, exec)
	_, col := m.editor.CursorScreenPos()
	if col != 9 {
		t.Errorf("col = %d, want 9 (schemaMsg=%q)", col, m.schemaMsg)
	}
}

func TestMaybeJumpToQueryErrorMySQLWrapped(t *testing.T) {
	user := "SELECT * FORM users"
	exec := db.PageWrapPrefix + user + ") AS _creel_page LIMIT 51 OFFSET 0"
	m := &Model{editor: NewQueryEditor(), results: NewResultsTable(), focus: FocusResults}
	m.editor.SetValue(user)
	err := &mysql.MySQLError{
		Number: 1064,
		Message: "You have an error in your SQL syntax; check the manual that " +
			"corresponds to your MySQL server version for the right syntax to " +
			"use near 'FORM users) AS _creel_page LIMIT 51 OFFSET 0' at line 1",
	}
	m.maybeJumpToQueryError(fmt.Errorf("query error: %w", err), user, exec)
	line, col := m.editor.CursorScreenPos()
	if line != 0 || col != 9 {
		t.Errorf("cursor = %d,%d want 0,9 (schemaMsg=%q)", line, col, m.schemaMsg)
	}
}

func TestMaybeJumpIgnoresNonSyntax(t *testing.T) {
	m := &Model{editor: NewQueryEditor(), results: NewResultsTable(), focus: FocusResults}
	m.editor.SetValue("SELECT * FROM users")
	m.maybeJumpToQueryError(fmt.Errorf("no such table: users"), m.editor.Value(), m.editor.Value())
	if m.focus != FocusResults {
		t.Error("should not steal focus for non-syntax errors")
	}
	if m.schemaMsg != "" {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}
