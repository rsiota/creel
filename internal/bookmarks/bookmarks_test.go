package bookmarks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAddAndGet(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add("mydb", "SELECT 1")
	s.Add("mydb", "SELECT 2")

	entries, err := s.Get("mydb")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(entries))
	}
	if entries[0].Query != "SELECT 1" {
		t.Errorf("expected first bookmark 'SELECT 1', got '%s'", entries[0].Query)
	}
}

func TestDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add("db", "SELECT 1")
	err := s.Add("db", "SELECT 1")
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("expected ErrDuplicate, got %v", err)
	}

	entries, _ := s.Get("db")
	if len(entries) != 1 {
		t.Errorf("expected 1 bookmark (no dupes), got %d", len(entries))
	}
}

func TestPerConnection(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add("db1", "SELECT 1")
	s.Add("db2", "SELECT 2")

	e1, _ := s.Get("db1")
	e2, _ := s.Get("db2")

	if len(e1) != 1 || e1[0].Query != "SELECT 1" {
		t.Error("db1 bookmarks incorrect")
	}
	if len(e2) != 1 || e2[0].Query != "SELECT 2" {
		t.Error("db2 bookmarks incorrect")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add("mydb", "SELECT 1")
	s.Add("mydb", "SELECT 2")

	s2 := NewStore(dir)
	entries, err := s2.Get("mydb")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 bookmarks after reload, got %d", len(entries))
	}
}

func TestRemoveAt(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add("db", "SELECT 1")
	s.Add("db", "SELECT 2")
	s.Add("db", "SELECT 3")

	s.RemoveAt("db", 1)

	entries, _ := s.Get("db")
	if len(entries) != 2 {
		t.Fatalf("expected 2 bookmarks after removal, got %d", len(entries))
	}
	if entries[1].Query != "SELECT 3" {
		t.Errorf("expected remaining 'SELECT 3', got '%s'", entries[1].Query)
	}
}

func TestRemoveAtOutOfBounds(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add("db", "SELECT 1")
	s.RemoveAt("db", 5)

	entries, _ := s.Get("db")
	if len(entries) != 1 {
		t.Errorf("expected 1 bookmark (no-op removal), got %d", len(entries))
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add("db", "SELECT 1")
	s.Clear("db")

	entries, _ := s.Get("db")
	if len(entries) != 0 {
		t.Errorf("expected 0 bookmarks after clear, got %d", len(entries))
	}
}

func TestFilePersisted(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Add("my-connection", "SELECT 1")

	path := filepath.Join(dir, "bookmarks", "my-connection.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected bookmarks file to exist on disk")
	}
}
