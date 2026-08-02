package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		query  string
		target string
		match  bool
	}{
		{"usr", "users", true},
		{"usr", "user_settings", true},
		{"ord", "orders", true},
		{"xyz", "users", false},
		{"", "anything", true},
		{"US", "users", true}, // case-insensitive
		{"uo", "users_orders", true},
	}
	for _, tt := range tests {
		idx, _ := fuzzyMatch(tt.query, tt.target)
		got := idx != nil || tt.query == ""
		if got != tt.match {
			t.Errorf("fuzzyMatch(%q, %q): expected match=%v, got match=%v", tt.query, tt.target, tt.match, got)
		}
	}
}

func TestFuzzyMatchScoring(t *testing.T) {
	// "usr" should match "users" better (earlier, more consecutive) than other tables.
	_, scoreConsec := fuzzyMatch("usr", "users")
	_, scoreGap := fuzzyMatch("usr", "u_s_r_table")
	if scoreConsec >= scoreGap {
		t.Errorf("expected consecutive match to score better: consec=%d, gap=%d", scoreConsec, scoreGap)
	}
}

func TestSidebarFilterMode(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "user_settings", "orders", "products", "logs"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	// Press '/' to enter filter mode.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	if !m.sidebarFiltering {
		t.Fatal("expected sidebarFiltering=true after pressing '/'")
	}

	// Type "usr" — should match users and user_settings.
	for _, r := range "usr" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	items := m.sidebarItems()
	if len(items) != 2 {
		t.Fatalf("expected 2 filtered items, got %d: %+v", len(items), items)
	}
	if items[0].text != "users" {
		t.Errorf("expected best match 'users', got %q", items[0].text)
	}

	// j/k should navigate within filtered results.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.sidebarCursor != 1 {
		t.Errorf("expected sidebarCursor=1 after j in filter, got %d", m.sidebarCursor)
	}
	cur := m.currentSidebarItem()
	if cur == nil || cur.text != "user_settings" {
		t.Errorf("expected cursor on 'user_settings', got %+v", cur)
	}

	// Enter selects the match and exits filter mode; cursor lands on real table.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.sidebarFiltering {
		t.Error("expected filter mode to exit after enter")
	}
	if m.sidebarCursor != 1 { // "user_settings" is index 1 in m.tables
		t.Errorf("expected sidebarCursor=1 (user_settings), got %d", m.sidebarCursor)
	}
	item := m.currentSidebarItem()
	if item == nil || item.text != "user_settings" {
		t.Errorf("expected cursor on 'user_settings' after enter, got %+v", item)
	}
}

func TestSidebarFilterCancel(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "orders"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	// Enter filter mode and type.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updated.(Model)
	if !m.sidebarFiltering {
		t.Fatal("expected to be in filter mode")
	}

	// Esc cancels.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.sidebarFiltering {
		t.Error("expected filter mode to exit after esc")
	}
	if m.sidebarFilter != "" {
		t.Errorf("expected empty filter after cancel, got %q", m.sidebarFilter)
	}
}

func TestSidebarFilterBackspace(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "orders"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	for _, r := range "us" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	if m.sidebarFilter != "us" {
		t.Fatalf("expected filter 'us', got %q", m.sidebarFilter)
	}

	// Backspace removes last character.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if m.sidebarFilter != "u" {
		t.Errorf("expected filter 'u' after backspace, got %q", m.sidebarFilter)
	}
}

func TestSidebarFilterSelectWithExpandedTable(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "user_settings", "orders"}
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.expanded = map[string][]db.Column{
		"users": {
			{Name: "id", Type: "INTEGER"},
			{Name: "name", Type: "TEXT"},
		},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	for _, r := range "settings" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	item := m.currentSidebarItem()
	if item == nil || item.text != "user_settings" {
		t.Fatalf("expected cursor on user_settings after filter select, got %+v", item)
	}
	if item.isColumn {
		t.Fatal("expected a table item, not a column")
	}
}

func TestSidebarFilterNoMatch(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "orders"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	for _, r := range "xyz" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	items := m.sidebarItems()
	if len(items) != 0 {
		t.Errorf("expected 0 matches for 'xyz', got %d", len(items))
	}
}
