// Package recent persists a most-recently-used list of connection names so the
// connection picker can reopen with the last-used entry selected.
package recent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const maxRecent = 10

// Store keeps an MRU list of connection names under <configDir>/recent.json.
type Store struct {
	mu    sync.Mutex
	path  string
	names []string // most recent first
	loaded bool
}

// NewStore roots the MRU file at configDir/recent.json.
func NewStore(configDir string) *Store {
	return &Store{path: filepath.Join(configDir, "recent.json")}
}

// Touch records name at the front of the MRU (deduped) and persists.
func (s *Store) Touch(name string) error {
	if name == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return err
	}
	out := make([]string, 0, len(s.names)+1)
	out = append(out, name)
	for _, n := range s.names {
		if n == name {
			continue
		}
		out = append(out, n)
	}
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	s.names = out
	return s.saveLocked()
}

// Names returns the MRU list (most recent first). Missing file → empty slice.
func (s *Store) Names() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return nil, err
	}
	out := make([]string, len(s.names))
	copy(out, s.names)
	return out, nil
}

// Last returns the most recent connection name, or "" when none.
func (s *Store) Last() (string, error) {
	names, err := s.Names()
	if err != nil || len(names) == 0 {
		return "", err
	}
	return names[0], nil
}

// Remove drops name from the MRU (e.g. after deleting a connection).
func (s *Store) Remove(name string) error {
	if name == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	out := s.names[:0]
	for _, n := range s.names {
		if n != name {
			out = append(out, n)
		}
	}
	s.names = out
	return s.saveLocked()
}

func (s *Store) loadLocked() error {
	if s.loaded {
		return nil
	}
	s.loaded = true
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.names = nil
			return nil
		}
		return err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	s.names = names
	return nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.names, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
