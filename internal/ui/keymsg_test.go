package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestKeyFilterChar(t *testing.T) {
	if ch, ok := keyFilterChar(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}); !ok || ch != "a" {
		t.Fatalf("rune: got %q ok=%v", ch, ok)
	}
	if ch, ok := keyFilterChar(tea.KeyMsg{Type: tea.KeySpace}); !ok || ch != " " {
		t.Fatalf("space: got %q ok=%v", ch, ok)
	}
	if _, ok := keyFilterChar(tea.KeyMsg{Type: tea.KeyEnter}); ok {
		t.Fatal("enter should not be a filter char")
	}
}

func TestBackendSearchAcceptsSpace(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state = stateWorkspace
	m.focus = FocusResults
	m.baseQuery = "SELECT * FROM users"
	m.connection = &db.Connection{}
	m.backendSearching = true

	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("john")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("doe")},
	} {
		updated, _ := m.Update(k)
		m = updated.(Model)
	}
	if m.backendSearchInput != "john doe" {
		t.Fatalf("backendSearchInput=%q, want %q", m.backendSearchInput, "john doe")
	}
}

func TestResultsRegexSearchAcceptsSpace(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state = stateWorkspace
	m.focus = FocusResults
	m.searching = true
	m.results.SetResult([]string{"name"}, [][]string{{"John Doe"}}, "1 row")

	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("John")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("Doe")},
	} {
		updated, _ := m.Update(k)
		m = updated.(Model)
	}
	if m.searchQuery != "John Doe" {
		t.Fatalf("searchQuery=%q, want %q", m.searchQuery, "John Doe")
	}
}
