package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	st := State{
		Tabs: []Tab{
			{Title: "users", Editor: "SELECT * FROM users;", LastQuery: "SELECT * FROM users;"},
			{Title: "New Query", Editor: "-- scratch\n"},
		},
		Active: 0,
		ColWidths: map[string]map[string]int{
			"users": {"email": 28, "name": 12},
		},
	}
	if err := s.Save("Work DB", "appdb", st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A fresh store reads it back from disk (cache must not mask a miss).
	fresh := NewStore(dir)
	got, err := fresh.Load("Work DB", "appdb")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Tabs) != 2 || got.Tabs[0].LastQuery != "SELECT * FROM users;" || got.Active != 0 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.ColWidths["users"]["email"] != 28 || got.ColWidths["users"]["name"] != 12 {
		t.Errorf("col widths round-trip mismatch: %+v", got.ColWidths)
	}
}

func TestStoreLoadMissingIsEmpty(t *testing.T) {
	s := NewStore(t.TempDir())
	got, err := s.Load("nope", "db")
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if got.HasContent() {
		t.Errorf("missing session should be empty, got %+v", got)
	}
}

func TestStoreConnectionAndDatabaseKeyedSeparately(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Same connection, different databases must not collide.
	if err := s.Save("c", "db_a", State{Tabs: []Tab{{Editor: "a"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("c", "db_b", State{Tabs: []Tab{{Editor: "b"}}}); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Load("c", "db_a")
	b, _ := s.Load("c", "db_b")
	if a.Tabs[0].Editor != "a" || b.Tabs[0].Editor != "b" {
		t.Errorf("db keying collided: a=%+v b=%+v", a, b)
	}

	// And two files exist on disk.
	entries, err := os.ReadDir(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 session files, got %d", len(entries))
	}
}

func TestStoreClear(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Save("c", "db", State{Tabs: []Tab{{Editor: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear("c", "db"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	got, _ := s.Load("c", "db")
	if got.HasContent() {
		t.Errorf("after Clear, session should be empty: %+v", got)
	}
}

func TestHasContent(t *testing.T) {
	if (State{}).HasContent() {
		t.Error("empty state should have no content")
	}
	if (State{Tabs: []Tab{{Title: "x"}}}).HasContent() {
		t.Error("tab with only a title should have no content")
	}
	if !(State{Tabs: []Tab{{LastQuery: "SELECT 1"}}}).HasContent() {
		t.Error("tab with a last query should have content")
	}
}
