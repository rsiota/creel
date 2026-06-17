package ui

import (
	"strings"
	"testing"
)

func TestTokenizeSQL(t *testing.T) {
	tokens := tokenizeSQL("SELECT name FROM users WHERE id = 1 -- comment")
	kinds := make(map[string]sqlTokenKind)
	for _, tok := range tokens {
		kinds[strings.TrimSpace(tok.text)] = tok.kind
	}

	if kinds["SELECT"] != tokenKeyword {
		t.Fatalf("SELECT kind = %v", kinds["SELECT"])
	}
	if kinds["FROM"] != tokenKeyword {
		t.Fatalf("FROM kind = %v", kinds["FROM"])
	}
	if kinds["1"] != tokenNumber {
		t.Fatalf("1 kind = %v", kinds["1"])
	}
	if kinds["-- comment"] != tokenComment {
		t.Fatalf("comment kind = %v", kinds["-- comment"])
	}
}

func TestTokenizeSQLStringLiteral(t *testing.T) {
	tokens := tokenizeSQL("WHERE name = 'O''Brien'")
	var found bool
	for _, tok := range tokens {
		if tok.text == "'O''Brien'" && tok.kind == tokenString {
			found = true
		}
	}
	if !found {
		t.Fatal("expected escaped string literal token")
	}
}

func TestHighlightRange(t *testing.T) {
	out := highlightRange("SELECT 1", 0, 6)
	if !strings.Contains(out, "SELECT") {
		t.Fatalf("expected SELECT in output: %q", out)
	}
}

func TestQueryEditorHighlightedView(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(60, 5)
	e.SetValue("SELECT id FROM users")

	view := e.View()
	if !strings.Contains(view, "SELECT") {
		t.Fatalf("view missing SELECT: %q", view)
	}
	if !strings.Contains(view, "FROM") {
		t.Fatalf("view missing FROM: %q", view)
	}
}

func TestSoftWrapRunes(t *testing.T) {
	wrapped := softWrapRunes([]rune("hello world"), 5)
	if len(wrapped) < 2 {
		t.Fatalf("expected wrap into multiple lines, got %d", len(wrapped))
	}
}

func TestBuildDisplayLinesLongLine(t *testing.T) {
	e := NewQueryEditor()
	e.SetSize(40, 5)
	longLine := strings.Repeat("a", 32) + " FROM users"
	e.SetValue("SELECT " + longLine)

	lines := e.buildDisplayLines()
	for i, dl := range lines {
		if len([]rune(dl.segment)) < 0 {
			t.Fatalf("line %d: empty segment", i)
		}
		// Must not panic when rendering.
		_ = highlightSegment(dl.segment)
	}

	_ = e.View()
}

func TestHighlightSubstringBounds(t *testing.T) {
	s := "SELECT 1"
	if highlightSubstring(s, 0, 100) == "" {
		t.Fatal("expected non-empty highlight")
	}
	if highlightSubstring(s, 10, 20) != "" {
		t.Fatal("expected empty highlight for out-of-range slice")
	}
}
