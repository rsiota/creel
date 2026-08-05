package ui

import (
	"strings"
	"testing"
)

// TestSqlEscape covers the NULL-vs-empty-string distinction: only the "NULL"
// sentinel maps to SQL NULL; a genuine empty string becomes ” so it
// round-trips through :copyinsert / copy-as-INSERT without corruption.
func TestSqlEscape(t *testing.T) {
	cases := []struct {
		val, typ, want string
	}{
		// NULL sentinel stays NULL.
		{"NULL", "", "NULL"},
		{"NULL", "TEXT", "NULL"},
		// A genuine empty string is now '' (not NULL) so it round-trips.
		{"", "", "''"},
		{"", "TEXT", "''"},
		// Text is single-quoted with embedded quotes doubled.
		{"alice", "TEXT", "'alice'"},
		{"o'brien", "TEXT", "'o''brien'"},
		// Numeric types pass bare (NULL still wins above).
		{"42", "INTEGER", "42"},
		{"3.14", "REAL", "3.14"},
		// Unknown type: a bare number is left unquoted; anything else quoted.
		{"42", "", "42"},
		{"abc", "", "'abc'"},
	}
	for _, c := range cases {
		if got := sqlEscape(c.val, c.typ); got != c.want {
			t.Errorf("sqlEscape(%q, %q) = %q, want %q", c.val, c.typ, got, c.want)
		}
	}
}

// TestCopyAsInsertEmptyString is the regression test for the data-corruption
// bug: sqlEscape previously coerced "" -> NULL, so copying a row whose value
// was a real empty string produced NULL in the INSERT.
func TestCopyAsInsertEmptyString(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "note"},
		[][]string{{"1", ""}, {"2", "NULL"}, {"3", "hi"}},
		"3 rows",
	)
	r.SetEditable("t", []string{"id"})

	sql, count := r.CopyAsInsert()
	if count != 3 {
		t.Fatalf("expected 3 rows, got %d", count)
	}
	// The empty-string value (row 1) must render as '' — the real NULL
	// (row 2) stays NULL. Before the fix the empty string was coerced to
	// NULL, so the dump held two NULLs.
	if !strings.Contains(sql, "(1, '')") {
		t.Errorf("empty string should render as '', got:\n%s", sql)
	}
	if got := strings.Count(sql, "NULL"); got != 1 {
		t.Errorf("expected exactly 1 NULL (the real one), got %d:\n%s", got, sql)
	}
}

func TestCopyAsDelimited(t *testing.T) {
	setup := func() ResultsTable {
		r := NewResultsTable()
		r.SetSize(80, 20)
		r.SetResult(
			[]string{"id", "name"},
			[][]string{{"1", "alice"}, {"2", "bob"}, {"3", "carol"}},
			"3 rows",
		)
		r.SetEditable("users", []string{"id"})
		return r
	}

	t.Run("cursor row defaults to TSV", func(t *testing.T) {
		r := setup()
		r.SetCursor(1, 0) // cursor on row 1 ("2", "bob")
		out, n := r.CopyAsDelimited(fmtTSV)
		if n != 1 {
			t.Fatalf("expected 1 row, got %d", n)
		}
		want := "id\tname\n2\tbob"
		if strings.TrimRight(out, "\n") != want {
			t.Errorf("TSV = %q, want %q", out, want)
		}
	})

	t.Run("marked rows are copied instead", func(t *testing.T) {
		r := setup()
		r.SetCursor(0, 0)
		r.ToggleMark() // row 0
		r.CursorDown()
		r.ToggleMark() // row 1
		out, n := r.CopyAsDelimited(fmtTSV)
		if n != 2 {
			t.Fatalf("expected 2 marked rows, got %d", n)
		}
		if !strings.Contains(out, "alice") || !strings.Contains(out, "bob") {
			t.Errorf("marked-row TSV missing rows: %q", out)
		}
		if strings.Contains(out, "carol") {
			t.Errorf("unmarked row leaked into copy: %q", out)
		}
	})

	t.Run("csv format", func(t *testing.T) {
		r := setup()
		r.SetCursor(0, 0)
		out, n := r.CopyAsDelimited(fmtCSV)
		if n != 1 {
			t.Fatalf("expected 1 row, got %d", n)
		}
		if !strings.Contains(out, "id,name") || !strings.Contains(out, "1,alice") {
			t.Errorf("CSV output unexpected: %q", out)
		}
	})

	t.Run("hidden columns are excluded", func(t *testing.T) {
		r := setup()
		r.SetHiddenColumns([]string{"name"})
		r.SetCursor(0, 0)
		out, _ := r.CopyAsDelimited(fmtTSV)
		if strings.Contains(out, "name") || strings.Contains(out, "alice") {
			t.Errorf("hidden column leaked into copy: %q", out)
		}
		if !strings.Contains(out, "id") {
			t.Errorf("visible column dropped: %q", out)
		}
	})

	t.Run("nothing to copy", func(t *testing.T) {
		r := NewResultsTable() // no result set
		if _, n := r.CopyAsDelimited(fmtTSV); n != 0 {
			t.Errorf("empty table should copy 0 rows, got %d", n)
		}
	})
}
