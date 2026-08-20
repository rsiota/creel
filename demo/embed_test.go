package demo

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEnsureCreatesAndReuses(t *testing.T) {
	dir := t.TempDir()
	path, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "demo", dbFileName)
	if path != want {
		t.Fatalf("path=%q, want %q", path, want)
	}
	st, err := os.Stat(path)
	if err != nil || st.Size() == 0 {
		t.Fatalf("demo db missing or empty: %v", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("users table missing")
	}

	// Second call must reuse the file (same path, no error).
	path2, err := Ensure(dir)
	if err != nil || path2 != path {
		t.Fatalf("reuse: path=%q err=%v", path2, err)
	}
}

func TestResolvePathPrefersCwdDemo(t *testing.T) {
	tmp := t.TempDir()
	demoDir := filepath.Join(tmp, "demo")
	if err := os.MkdirAll(demoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(demoDir, dbFileName)
	if err := os.WriteFile(local, []byte("not-a-real-db-but-nonzero"), 0o600); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	path, err := ResolvePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	absLocal, err := filepath.Abs(local)
	if err != nil {
		t.Fatal(err)
	}
	absGot, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	// macOS may report /var vs /private/var; compare via EvalSymlinks.
	if resolved, err := filepath.EvalSymlinks(absLocal); err == nil {
		absLocal = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absGot); err == nil {
		absGot = resolved
	}
	if absGot != absLocal {
		t.Fatalf("ResolvePath=%q, want cwd demo %q", absGot, absLocal)
	}
}
