package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
)

func TestTableScrolling(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)

	// Simulate window size
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Simulate tables loaded
	m.tables = []string{"users", "orders", "products", "items", "logs"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	// Press j (down)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)

	if m.tableScroll != 1 {
		t.Errorf("after j: expected tableScroll=1, got %d", m.tableScroll)
	}

	// Press j again
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)

	if m.tableScroll != 2 {
		t.Errorf("after j,j: expected tableScroll=2, got %d", m.tableScroll)
	}

	// Press k (up)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(Model)

	if m.tableScroll != 1 {
		t.Errorf("after j,j,k: expected tableScroll=1, got %d", m.tableScroll)
	}
}

func TestTableScrollClamping(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"a", "b", "c"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	// Scroll past the end
	for i := 0; i < 10; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(Model)
	}
	if m.tableScroll != 2 {
		t.Errorf("expected clamped to 2, got %d", m.tableScroll)
	}

	// Scroll past the beginning
	for i := 0; i < 10; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		m = updated.(Model)
	}
	if m.tableScroll != 0 {
		t.Errorf("expected clamped to 0, got %d", m.tableScroll)
	}
}
