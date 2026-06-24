package db

import (
	"testing"
)

func TestSplitSingleStatement(t *testing.T) {
	stmts := SplitStatements("SELECT 1")
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if stmts[0].Text != "SELECT 1" {
		t.Errorf("expected 'SELECT 1', got '%s'", stmts[0].Text)
	}
}

func TestSplitMultipleStatements(t *testing.T) {
	stmts := SplitStatements("SELECT 1; SELECT 2; SELECT 3;")
	if len(stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(stmts))
	}
	expected := []string{"SELECT 1", "SELECT 2", "SELECT 3"}
	for i, want := range expected {
		if stmts[i].Text != want {
			t.Errorf("statement %d: expected '%s', got '%s'", i, want, stmts[i].Text)
		}
	}
}

func TestSplitTrailingSemicolon(t *testing.T) {
	stmts := SplitStatements("SELECT 1;")
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
}

func TestSplitEmptyStatements(t *testing.T) {
	stmts := SplitStatements("SELECT 1;; ;SELECT 2")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (empty ones skipped), got %d", len(stmts))
	}
	if stmts[0].Text != "SELECT 1" {
		t.Errorf("expected 'SELECT 1', got '%s'", stmts[0].Text)
	}
	if stmts[1].Text != "SELECT 2" {
		t.Errorf("expected 'SELECT 2', got '%s'", stmts[1].Text)
	}
}

func TestSplitMultilineStatement(t *testing.T) {
	input := "SELECT *\nFROM users\nWHERE id = 1;"
	stmts := SplitStatements(input)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if stmts[0].Text != "SELECT *\nFROM users\nWHERE id = 1" {
		t.Errorf("unexpected text: '%s'", stmts[0].Text)
	}
}

func TestSplitSemicolonInSingleQuotes(t *testing.T) {
	stmts := SplitStatements("SELECT 'hello; world'; SELECT 2")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	if stmts[0].Text != "SELECT 'hello; world'" {
		t.Errorf("expected semicolon inside quotes preserved, got '%s'", stmts[0].Text)
	}
}

func TestSplitEscapedSingleQuote(t *testing.T) {
	stmts := SplitStatements("INSERT INTO t VALUES ('it''s ok'); SELECT 2")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	if stmts[0].Text != "INSERT INTO t VALUES ('it''s ok')" {
		t.Errorf("unexpected text: '%s'", stmts[0].Text)
	}
}

func TestSplitSemicolonInDoubleQuotes(t *testing.T) {
	stmts := SplitStatements(`SELECT "col;name" FROM t; SELECT 2`)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
}

func TestSplitLineComment(t *testing.T) {
	stmts := SplitStatements("SELECT 1 -- this is a comment\n; SELECT 2")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
}

func TestSplitBlockComment(t *testing.T) {
	stmts := SplitStatements("SELECT /* comment ; here */ 1; SELECT 2")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	if stmts[0].Text != "SELECT /* comment ; here */ 1" {
		t.Errorf("unexpected text: '%s'", stmts[0].Text)
	}
}

func TestSplitNoSemicolonTrailing(t *testing.T) {
	stmts := SplitStatements("SELECT 1; SELECT 2")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	if stmts[1].Text != "SELECT 2" {
		t.Errorf("expected 'SELECT 2', got '%s'", stmts[1].Text)
	}
}

func TestSplitOffsets(t *testing.T) {
	sql := "SELECT 1; SELECT 2"
	stmts := SplitStatements(sql)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	// Verify offsets point to valid positions in the original string.
	for _, s := range stmts {
		if s.Start < 0 || s.End >= len([]rune(sql)) {
			t.Errorf("offset out of range: start=%d end=%d", s.Start, s.End)
		}
	}
}

func TestSplitEmpty(t *testing.T) {
	stmts := SplitStatements("")
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements, got %d", len(stmts))
	}
}

func TestSplitWhitespaceOnly(t *testing.T) {
	stmts := SplitStatements("   \n  \t  ")
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements, got %d", len(stmts))
	}
}
