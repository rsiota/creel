package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxHistoryPerConnection = 500

// Entry represents a single query history record.
type Entry struct {
	Query   string        `json:"query"`
	RunAt   time.Time     `json:"run_at"`
	Elapsed time.Duration `json:"elapsed"`
	Success bool          `json:"success"`
}

// Store manages query history per connection, persisted to disk as JSON.
type Store struct {
	mu       sync.Mutex
	dir      string
	cache    map[string][]Entry
}

// NewStore creates a history store rooted at the given config directory.
func NewStore(configDir string) *Store {
	return &Store{
		dir:   filepath.Join(configDir, "history"),
		cache: make(map[string][]Entry),
	}
}

func (s *Store) pathFor(connection string) string {
	safe := sanitizeFilename(connection)
	return filepath.Join(s.dir, safe+".json")
}

func (s *Store) load(connection string) ([]Entry, error) {
	if cached, ok := s.cache[connection]; ok {
		return cached, nil
	}

	path := s.pathFor(connection)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	s.cache[connection] = entries
	return entries, nil
}

func (s *Store) save(connection string, entries []Entry) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.pathFor(connection), data, 0600)
}

// Record adds a query to the history for the given connection. The elapsed
// duration is persisted so slow queries can be surfaced later; pass 0 when the
// duration is unknown (e.g. legacy callers). Duplicate consecutive queries are
// skipped.
func (s *Store) Record(connection, query string, elapsed time.Duration, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load(connection)
	if err != nil {
		return err
	}

	// Skip if identical to the most recent entry.
	if len(entries) > 0 && entries[len(entries)-1].Query == query {
		return nil
	}

	entries = append(entries, Entry{
		Query:   query,
		RunAt:   time.Now(),
		Elapsed: elapsed,
		Success: success,
	})

	// Trim to max size.
	if len(entries) > maxHistoryPerConnection {
		entries = entries[len(entries)-maxHistoryPerConnection:]
	}

	s.cache[connection] = entries
	return s.save(connection, entries)
}

// Get returns the query history for the given connection, most recent last.
func (s *Store) Get(connection string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(connection)
}

// Clear removes all history for a connection.
func (s *Store) Clear(connection string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[connection] = nil
	path := s.pathFor(connection)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Search returns entries whose query contains the given substring.
func (s *Store) Search(connection, substr string) ([]Entry, error) {
	entries, err := s.Get(connection)
	if err != nil {
		return nil, err
	}

	var matched []Entry
	for _, e := range entries {
		if containsCI(e.Query, substr) {
			matched = append(matched, e)
		}
	}
	return matched, nil
}

func containsCI(s, substr string) bool {
	return containsFold([]rune(s), []rune(substr))
}

func containsFold(s, substr []rune) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			a := s[i+j]
			b := substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func sanitizeFilename(s string) string {
	var b []rune
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b = append(b, r)
		} else {
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "default"
	}
	return string(b)
}

// FormatTime returns a human-readable timestamp for display.
func FormatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// FormatElapsed renders a query duration compactly: "12.3ms", "1.23s",
// "1m02s". A zero duration (entries recorded before timing was tracked, or a
// caller that passed 0) returns "—" so the column stays aligned.
func FormatElapsed(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
	case d < time.Minute:
		return fmt.Sprintf("%.2fs", d.Seconds())
	default:
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*60
		return fmt.Sprintf("%dm%02ds", m, s)
	}
}

// FormatEntry returns a single-line representation for display.
func FormatEntry(e Entry, idx int) string {
	status := "✓"
	if !e.Success {
		status = "✗"
	}
	return fmt.Sprintf("%s  %s  %s", status, FormatTime(e.RunAt), truncate(e.Query, 60))
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
