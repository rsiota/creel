package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	cases := []struct {
		in   string
		want string // "" means "expect an error or skip (home-dependent)"
	}{
		{"foo/bar", "foo/bar"},
		{"/abs/path", "/abs/path"},
		{"foo/../bar", "bar"}, // filepath.Clean collapses ..
	}
	for _, c := range cases {
		got, err := expandTilde(filepath.Clean(c.in))
		if err != nil {
			t.Errorf("expandTilde(%q) error: %v", c.in, err)
			continue
		}
		// Only assert the non-~ cases (the ~ cases depend on $HOME).
		if c.want != "" && got != c.want {
			t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// ~/x resolves under the home directory.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got, err := expandTilde("~/x.sql")
	if err != nil {
		t.Fatalf("expandTilde(~/x.sql) error: %v", err)
	}
	if got != filepath.Join(home, "x.sql") {
		t.Errorf("expandTilde(~/x.sql) = %q, want %q", got, filepath.Join(home, "x.sql"))
	}
}

func TestExWriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.sql")
	m := &Model{editor: NewQueryEditor()}
	m.editor.SetValue("SELECT 1;\nSELECT 2;\n")

	m.runExCommand("w " + path)
	if !strings.Contains(m.schemaMsg, "wrote") || !strings.Contains(m.schemaMsg, "2 lines") {
		t.Errorf(":w <file> -> %q", m.schemaMsg)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != "SELECT 1;\nSELECT 2;\n" {
		t.Errorf("file content = %q", string(got))
	}
}

func TestExWriteFileOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.sql")
	if err := os.WriteFile(path, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{editor: NewQueryEditor()}
	m.editor.SetValue("NEW")
	m.runExCommand("w " + path)
	got, _ := os.ReadFile(path)
	if string(got) != "NEW" {
		t.Errorf(":w overwrites: got %q, want NEW", string(got))
	}
}

func TestExEditFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in.sql")
	want := "SELECT * FROM users;\n-- comment\n"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{editor: NewQueryEditor()}
	m.editor.SetValue("throwaway")

	m.runExCommand("e " + path)
	if !strings.Contains(m.schemaMsg, "loaded") || !strings.Contains(m.schemaMsg, "2 lines") {
		t.Errorf(":e <file> -> %q", m.schemaMsg)
	}
	if m.editor.Value() != want {
		t.Errorf("editor buffer after :e = %q, want %q", m.editor.Value(), want)
	}
}

func TestExEditFileMissing(t *testing.T) {
	m := &Model{editor: NewQueryEditor()}
	m.runExCommand("e " + filepath.Join(t.TempDir(), "nope.sql"))
	if !strings.Contains(m.schemaMsg, "read failed") {
		t.Errorf(":e missing file -> %q", m.schemaMsg)
	}
}

func TestExEditNeedsPath(t *testing.T) {
	m := &Model{}
	m.runExCommand("e")
	if !strings.Contains(m.schemaMsg, "file path") {
		t.Errorf(":e with no arg -> %q", m.schemaMsg)
	}
}

// The no-argument `:w` path (save cell edits) is covered by TestExWrite in
// excmd_test.go; the argument path (write buffer to file) is covered above.

func TestLineCount(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"one", 1},
		{"one\ntwo", 2},
		{"one\ntwo\n", 2}, // trailing newline does not add a line
		{"a\nb\nc\n", 3},
	}
	for _, c := range cases {
		if got := lineCount(c.in); got != c.want {
			t.Errorf("lineCount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// loadStartupFile is the `gsql -f` startup loader (the non-interactive
// counterpart of :e). It shares :e's ~ expansion and relative-path handling;
// unlike :e it returns the error rather than parking it in schemaMsg, so Run
// can fail fast on a missing/unreadable file.
func TestLoadStartupFile(t *testing.T) {
	content := "SELECT 1;\nSELECT 2;\n"
	path := filepath.Join(t.TempDir(), "script.sql")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{editor: NewQueryEditor()}
	expanded, err := m.loadStartupFile(path)
	if err != nil {
		t.Fatalf("loadStartupFile: %v", err)
	}
	if m.editor.Value() != content {
		t.Errorf("editor = %q, want %q", m.editor.Value(), content)
	}
	if expanded != path {
		t.Errorf("expanded = %q, want %q", expanded, path)
	}

	// Missing file → error (Run fails fast on this).
	m2 := &Model{editor: NewQueryEditor()}
	if _, err := m2.loadStartupFile(filepath.Join(t.TempDir(), "nope.sql")); err == nil {
		t.Error("loadStartupFile on a missing file should return an error")
	}
}
