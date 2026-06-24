package bookmarks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Bookmark represents a single saved query.
type Bookmark struct {
	Query   string    `json:"query"`
	SavedAt time.Time `json:"saved_at"`
}

// Store manages bookmarks per connection, persisted to disk as JSON.
type Store struct {
	mu    sync.Mutex
	dir   string
	cache map[string][]Bookmark
}

// NewStore creates a bookmark store rooted at the given config directory.
func NewStore(configDir string) *Store {
	return &Store{
		dir:   filepath.Join(configDir, "bookmarks"),
		cache: make(map[string][]Bookmark),
	}
}

func (s *Store) pathFor(connection string) string {
	safe := sanitizeFilename(connection)
	return filepath.Join(s.dir, safe+".json")
}

func (s *Store) load(connection string) ([]Bookmark, error) {
	if cached, ok := s.cache[connection]; ok {
		return cached, nil
	}

	path := s.pathFor(connection)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Bookmark{}, nil
		}
		return nil, err
	}

	var entries []Bookmark
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	s.cache[connection] = entries
	return entries, nil
}

func (s *Store) save(connection string, entries []Bookmark) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.pathFor(connection), data, 0600)
}

// Add saves a query as a bookmark for the given connection.
// Returns ErrDuplicate if the query already exists.
func (s *Store) Add(connection, query string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load(connection)
	if err != nil {
		return err
	}

	for _, b := range entries {
		if b.Query == query {
			return ErrDuplicate
		}
	}

	entries = append(entries, Bookmark{
		Query:   query,
		SavedAt: time.Now(),
	})

	s.cache[connection] = entries
	return s.save(connection, entries)
}

// Get returns all bookmarks for the given connection.
func (s *Store) Get(connection string) ([]Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(connection)
}

// RemoveAt deletes the bookmark at the given index.
func (s *Store) RemoveAt(connection string, index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load(connection)
	if err != nil {
		return err
	}

	if index < 0 || index >= len(entries) {
		return nil
	}

	entries = append(entries[:index], entries[index+1:]...)
	s.cache[connection] = entries
	return s.save(connection, entries)
}

// Clear removes all bookmarks for a connection.
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

// ErrDuplicate is returned when a query is already bookmarked.
var ErrDuplicate = errDuplicate{}

type errDuplicate struct{}

func (errDuplicate) Error() string { return "query already bookmarked" }
