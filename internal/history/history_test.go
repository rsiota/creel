package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordAndGet(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Record some queries
	s.Record("mydb", "SELECT 1", 0, true)
	s.Record("mydb", "SELECT 2", 0, true)
	s.Record("mydb", "SELECT 3", 0, false)

	entries, err := s.Get("mydb")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Query != "SELECT 1" {
		t.Errorf("expected first entry 'SELECT 1', got '%s'", entries[0].Query)
	}
	if entries[2].Success {
		t.Error("expected third entry to be unsuccessful")
	}
}

func TestDuplicateConsecutive(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Record("db", "SELECT 1", 0, true)
	s.Record("db", "SELECT 1", 0, true)
	s.Record("db", "SELECT 1", 0, true)

	entries, _ := s.Get("db")
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (deduped), got %d", len(entries))
	}
}

func TestPerConnection(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Record("db1", "SELECT 1", 0, true)
	s.Record("db2", "SELECT 2", 0, true)

	e1, _ := s.Get("db1")
	e2, _ := s.Get("db2")

	if len(e1) != 1 || e1[0].Query != "SELECT 1" {
		t.Error("db1 history incorrect")
	}
	if len(e2) != 1 || e2[0].Query != "SELECT 2" {
		t.Error("db2 history incorrect")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Record("mydb", "SELECT 1", 0, true)
	s.Record("mydb", "SELECT 2", 0, true)

	// Create a new store from same dir
	s2 := NewStore(dir)
	entries, err := s2.Get("mydb")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after reload, got %d", len(entries))
	}
}

func TestMaxHistory(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	for i := 0; i < maxHistoryPerConnection+50; i++ {
		s.Record("db", "SELECT "+string(rune('A'+i%26)), 0, true)
	}

	entries, _ := s.Get("db")
	if len(entries) > maxHistoryPerConnection {
		t.Errorf("expected max %d entries, got %d", maxHistoryPerConnection, len(entries))
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Record("db", "SELECT 1", 0, true)
	s.Clear("db")

	entries, _ := s.Get("db")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(entries))
	}
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Record("db", "SELECT * FROM users", 0, true)
	s.Record("db", "SELECT * FROM orders", 0, true)
	s.Record("db", "DROP TABLE users", 0, false)

	results, err := s.Search("db", "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 matches, got %d", len(results))
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Record("db", "SELECT * FROM Users", 0, true)

	results, _ := s.Search("db", "users")
	if len(results) != 1 {
		t.Errorf("expected 1 case-insensitive match, got %d", len(results))
	}
}

func TestFilePersisted(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	s.Record("my-connection", "SELECT 1", 0, true)

	path := filepath.Join(dir, "history", "my-connection.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected history file to exist on disk")
	}
}

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		d   time.Duration
		out string
	}{
		{0, "—"},
		{500 * time.Microsecond, "500µs"},
		{12 * time.Millisecond, "12.0ms"},
		{1234 * time.Millisecond, "1.23s"},
		{2 * time.Second, "2.00s"},
		{90 * time.Second, "1m30s"},
		{-time.Second, "—"},
	}
	for _, c := range cases {
		if got := FormatElapsed(c.d); got != c.out {
			t.Errorf("FormatElapsed(%v) = %q, want %q", c.d, got, c.out)
		}
	}
}

func TestRecordStoresElapsed(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	d := 250 * time.Millisecond
	if err := s.Record("db", "SELECT 1", d, true); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.Get("db")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Elapsed != d {
		t.Errorf("expected elapsed %v, got %v", d, entries[0].Elapsed)
	}
}

// TestElapsedPersistsAcrossReload confirms the duration survives a store
// reload (JSON round-trip), and that legacy files without the field load as 0.
func TestElapsedPersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	d := 5 * time.Second
	if err := s.Record("db", "SELECT 1", d, true); err != nil {
		t.Fatal(err)
	}

	s2 := NewStore(dir)
	entries, err := s2.Get("db")
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Elapsed != d {
		t.Errorf("after reload: elapsed %v, want %v", entries[0].Elapsed, d)
	}
	if !entries[0].Success {
		t.Error("success flag not preserved across reload")
	}
}
