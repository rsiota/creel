package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

func TestTableScrolling(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)

	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "orders", "products", "items", "logs"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	// Press j (down)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.sidebarCursor != 1 {
		t.Errorf("after j: expected sidebarCursor=1, got %d", m.sidebarCursor)
	}

	// Press j again
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.sidebarCursor != 2 {
		t.Errorf("after j,j: expected sidebarCursor=2, got %d", m.sidebarCursor)
	}

	// Press k (up)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(Model)
	if m.sidebarCursor != 1 {
		t.Errorf("after j,j,k: expected sidebarCursor=1, got %d", m.sidebarCursor)
	}
}

func TestTableScrollClamping(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"a", "b", "c"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	for i := 0; i < 10; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(Model)
	}
	if m.sidebarCursor != 2 {
		t.Errorf("expected clamped to 2, got %d", m.sidebarCursor)
	}

	for i := 0; i < 10; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		m = updated.(Model)
	}
	if m.sidebarCursor != 0 {
		t.Errorf("expected clamped to 0, got %d", m.sidebarCursor)
	}
}

func TestExpandCollapseNavigation(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "orders"}
	m.state = stateWorkspace
	m.focus = FocusConnections
	m.expanded = map[string][]db.Column{
		"users": {
			{Name: "id", Type: "INTEGER"},
			{Name: "name", Type: "TEXT"},
		},
	}

	items := m.sidebarItems()
	// users, id, name, orders = 4 items
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}
	if items[0].text != "users" || items[0].isColumn {
		t.Error("item 0 should be table 'users'")
	}
	if items[1].text != "id" || !items[1].isColumn {
		t.Error("item 1 should be column 'id'")
	}
	if items[2].text != "name" || !items[2].isColumn {
		t.Error("item 2 should be column 'name'")
	}
	if items[3].text != "orders" || items[3].isColumn {
		t.Error("item 3 should be table 'orders'")
	}

	// Navigate down through columns
	for i := 0; i < 3; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(Model)
	}
	item := m.currentSidebarItem()
	if item == nil || item.text != "orders" {
		t.Errorf("expected cursor on 'orders', got %+v", item)
	}
}

func TestSelectFromTable(t *testing.T) {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.tables = []string{"users", "orders"}
	m.state = stateWorkspace
	m.focus = FocusConnections

	// Press 's' to select
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(Model)

	if m.editor.Value() != "SELECT * FROM users;" {
		t.Errorf("unexpected query: %s", m.editor.Value())
	}
	if m.focus != FocusConnections {
		t.Error("expected focus to remain on sidebar after select")
	}
}
