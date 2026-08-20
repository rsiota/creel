package recent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTouchOrderAndCap(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	for _, name := range []string{"a", "b", "c", "a"} {
		if err := s.Touch(name); err != nil {
			t.Fatalf("Touch(%q): %v", name, err)
		}
	}
	got, err := s.Names()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "c", "b"}
	if len(got) != len(want) {
		t.Fatalf("Names=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names=%v, want %v", got, want)
		}
	}
	last, err := s.Last()
	if err != nil || last != "a" {
		t.Fatalf("Last=%q err=%v, want a", last, err)
	}

	s2 := NewStore(dir)
	got, err = s2.Names()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "a" {
		t.Fatalf("reloaded Names=%v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "recent.json")); err != nil {
		t.Fatalf("recent.json missing: %v", err)
	}
}

func TestRemove(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.Touch("a")
	_ = s.Touch("b")
	if err := s.Remove("a"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Names()
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("after Remove: %v", got)
	}
}

func TestCapAtMax(t *testing.T) {
	s := NewStore(t.TempDir())
	for i := 0; i < maxRecent+5; i++ {
		_ = s.Touch(string(rune('a'+i%26)) + string(rune('0'+i/26)))
	}
	got, _ := s.Names()
	if len(got) != maxRecent {
		t.Fatalf("len=%d, want %d", len(got), maxRecent)
	}
}
