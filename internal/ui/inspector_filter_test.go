package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/creel/internal/config"
)

func setupInspectorModel(t *testing.T) Model {
	t.Helper()
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state = stateWorkspace
	m.focus = FocusInspector
	m.inspector.visible = true
	m.results.SetResult(
		[]string{"id", "user_id", "email", "created_at"},
		[][]string{{"1", "42", "alice@test.com", "2024-01-01"}},
		"1 row",
	)
	return m
}

func TestInspectorFilterMode(t *testing.T) {
	m := setupInspectorModel(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	if !m.inspector.IsFiltering() {
		t.Fatal("expected inspector filter mode after pressing '/'")
	}

	for _, r := range "mail" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	fields := m.inspector.fieldList(m.results)
	if len(fields) != 1 {
		t.Fatalf("expected 1 filtered field, got %d", len(fields))
	}
	if m.results.ColumnName(fields[0]) != "email" {
		t.Fatalf("expected match on email, got %q", m.results.ColumnName(fields[0]))
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.inspector.IsFiltering() {
		t.Fatal("expected filter mode to exit after enter")
	}
	if m.inspector.cursorField != 2 {
		t.Fatalf("expected cursor on column 2 (email), got %d", m.inspector.cursorField)
	}
}

func TestInspectorFilterCancel(t *testing.T) {
	m := setupInspectorModel(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.inspector.IsFiltering() {
		t.Fatal("expected filter mode to exit after esc")
	}
	if m.inspector.filter != "" {
		t.Fatalf("expected empty filter after cancel, got %q", m.inspector.filter)
	}
}

func TestInspectorFilterNoMatch(t *testing.T) {
	m := setupInspectorModel(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	for _, r := range "xyz" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	fields := m.inspector.fieldList(m.results)
	if len(fields) != 0 {
		t.Fatalf("expected 0 matches for 'xyz', got %d", len(fields))
	}
}
