// Package session persists per-connection workspace state — the open tabs,
// their editor buffers, and which tab is active — so reopening a connection
// restores where the user left off. State is keyed by (connection, database)
// and stored as JSON under <configDir>/sessions/.
//
// It intentionally mirrors internal/history's shape (per-key JSON files, a
// mutex-guarded in-memory cache, a shared sanitize helper) but is kept as a
// separate package because it tracks UI workspace state rather than a query
// log.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Tab is the persisted content of one editor tab. Only the parts worth
// restoring are kept: the tab title, the editor buffer, and the last executed
// statement. Result rows, cursors, and filters are intentionally not
// persisted — the buffer is restored verbatim and the user re-runs it
// (mirroring the gsql -f startup flag), which avoids stale data and
// side-effecting writes on reconnect.
type Tab struct {
	Title     string `json:"title,omitempty"`
	Editor    string `json:"editor,omitempty"`     // editor buffer content
	LastQuery string `json:"last_query,omitempty"` // last executed statement ("" if none)
}

// State is the persisted workspace state for a single (connection, database).
type State struct {
	Tabs   []Tab `json:"tabs"`
	Active int   `json:"active"` // index into Tabs; 0 when unset
}

// HasContent reports whether s carries anything worth restoring. A session
// made up only of blank tabs (no editor content, no executed query) is treated
// as empty so reconnecting keeps the default single "New Query" tab.
func (s State) HasContent() bool {
	for _, t := range s.Tabs {
		if t.Editor != "" || t.LastQuery != "" {
			return true
		}
	}
	return false
}

// Store persists workspace State per (connection, database) as JSON files. It
// is safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	dir   string
	cache map[string]State
}

// NewStore creates a session store rooted at configDir. Session files live
// under <configDir>/sessions/.
func NewStore(configDir string) *Store {
	return &Store{
		dir:   filepath.Join(configDir, "sessions"),
		cache: make(map[string]State),
	}
}

// Save persists the workspace state for the given (connection, database),
// creating the sessions directory on first use.
func (s *Store) Save(connection, database string, st State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.pathFor(connection, database), data, 0o600); err != nil {
		return err
	}
	s.cache[cacheKey(connection, database)] = st
	return nil
}

// Load returns the workspace state for the given (connection, database). A
// missing file is not an error — an empty State is returned, leaving the
// caller's default workspace untouched.
func (s *Store) Load(connection, database string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := cacheKey(connection, database)
	if cached, ok := s.cache[key]; ok {
		return cached, nil
	}

	data, err := os.ReadFile(s.pathFor(connection, database))
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	s.cache[key] = st
	return st, nil
}

// Clear removes the persisted state for a (connection, database). A missing
// file is not an error.
func (s *Store) Clear(connection, database string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.cache, cacheKey(connection, database))
	if err := os.Remove(s.pathFor(connection, database)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// cacheKey is the in-memory map key (raw, unsanitized) for a connection +
// database pair.
func cacheKey(connection, database string) string {
	return connection + "\x00" + database
}

// pathFor returns the on-disk file for a connection + database pair. Each part
// is sanitized independently so the separator between them survives, keeping
// distinct pairs from colliding on disk.
func (s *Store) pathFor(connection, database string) string {
	name := sanitize(connection) + "__" + sanitize(database)
	return filepath.Join(s.dir, name+".json")
}

// sanitize reduces a name to the safe filename charset used elsewhere for
// per-connection files (history, bookmarks), so connection/database names with
// spaces, slashes, or dots map deterministically. Identical to history's
// helper to keep file naming consistent across stores.
func sanitize(s string) string {
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
