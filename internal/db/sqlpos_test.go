package db

import (
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestLocateQueryErrorPostgresPosition(t *testing.T) {
	q := "SELECT * FORM users"
	// Position 1-based at F of FORM (index 9 in "SELECT * FORM..." = byte 9, pos 10)
	err := fmt.Errorf("query error: %w", &pgconn.PgError{
		Severity: "ERROR",
		Message:  `syntax error at or near "FORM"`,
		Position: 10,
	})
	pos := LocateQueryError(err, q, q)
	if !pos.OK {
		t.Fatal("expected OK")
	}
	if pos.Line != 0 || pos.Col != 9 {
		t.Errorf("got line=%d col=%d, want 0,9", pos.Line, pos.Col)
	}
	if pos.Token != "FORM" {
		t.Errorf("token = %q", pos.Token)
	}
}

func TestLocateQueryErrorPostgresWrapped(t *testing.T) {
	user := "SELECT * FORM users"
	exec := PageWrapPrefix + user + ") AS _creel_page LIMIT 51 OFFSET 0"
	// Position points at FORM inside the wrapped query.
	innerOff := len(PageWrapPrefix) + 9 // byte offset of F
	err := &pgconn.PgError{
		Message:  `syntax error at or near "FORM"`,
		Position: int32(innerOff + 1),
	}
	pos := LocateQueryError(err, user, exec)
	if !pos.OK || pos.Col != 9 {
		t.Fatalf("wrapped pos = %+v, want col 9", pos)
	}
}

func TestLocateQueryErrorMySQLLine(t *testing.T) {
	q := "SELECT *\nFORM users"
	err := &mysql.MySQLError{
		Number:  1064,
		Message: "You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near 'FORM users' at line 2",
	}
	pos := LocateQueryError(fmt.Errorf("query error: %w", err), q, q)
	if !pos.OK {
		t.Fatal("expected OK")
	}
	if pos.Line != 1 {
		t.Errorf("line = %d, want 1", pos.Line)
	}
	if pos.Token != "FORM users" {
		t.Errorf("token = %q", pos.Token)
	}
	if pos.Col != 0 { // "FORM users" starts at col 0 on line 2
		t.Errorf("col = %d, want 0", pos.Col)
	}
}

func TestLocateQueryErrorSQLiteNear(t *testing.T) {
	q := "SELECT * FORM users"
	err := fmt.Errorf(`near "FORM": syntax error`)
	pos := LocateQueryError(err, q, q)
	if !pos.OK || pos.Col != 9 {
		t.Fatalf("got %+v", pos)
	}
}

func TestLocateQueryErrorMySQLWrappedNear(t *testing.T) {
	// Pagination wrap leaks into MySQL's near-snippet; without cleanup the
	// token never matches and the caret used to land on SELECT (col 0).
	user := "SELECT * FORM users"
	exec := PageWrapPrefix + user + ") AS _creel_page LIMIT 51 OFFSET 0"
	err := &mysql.MySQLError{
		Number: 1064,
		Message: "You have an error in your SQL syntax; check the manual that " +
			"corresponds to your MySQL server version for the right syntax to " +
			"use near 'FORM users) AS _creel_page LIMIT 51 OFFSET 0' at line 1",
	}
	pos := LocateQueryError(fmt.Errorf("query error: %w", err), user, exec)
	if !pos.OK {
		t.Fatal("expected OK")
	}
	if pos.Col != 9 {
		t.Fatalf("col = %d, want 9 (pos=%+v)", pos.Col, pos)
	}
	if pos.Token != "FORM users" {
		t.Errorf("token = %q, want %q", pos.Token, "FORM users")
	}
}

func TestLocateQueryErrorNoMatch(t *testing.T) {
	pos := LocateQueryError(fmt.Errorf("permission denied for table users"), "SELECT 1", "SELECT 1")
	if pos.OK {
		t.Fatalf("expected not OK, got %+v", pos)
	}
}

func TestOffsetToLineCol(t *testing.T) {
	line, col := offsetToLineCol("ab\ncafé", 3) // at 'c'
	if line != 1 || col != 0 {
		t.Errorf("got %d,%d", line, col)
	}
	_, col = offsetToLineCol("ab\ncafé", len("ab\ncaf")) // at é
	if col != 3 {
		t.Errorf("col = %d, want 3", col)
	}
}
