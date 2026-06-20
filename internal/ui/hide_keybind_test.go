package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
)

func TestHideColumnKeybinding(t *testing.T) {
	m := newResultsWorkspaceModel()

	// Cursor starts at col 0 ("id"). Move right to "name" (col 1) and hide it.
	m = press(m, keyRunes('l'))
	m = press(m, keyRunes('H'))

	if !m.results.IsColumnHidden(1) {
		t.Fatal("expected column 1 hidden after H")
	}
	// Cursor must have moved off the hidden column.
	if m.results.IsColumnHidden(m.results.CursorCol()) {
		t.Errorf("cursor left on hidden column %d", m.results.CursorCol())
	}
}

func TestShowAllColumnsKeybinding(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.results.HideColumn(1)
	m.results.HideColumn(2)
	if m.results.HiddenCount() != 2 {
		t.Fatalf("setup: expected 2 hidden, got %d", m.results.HiddenCount())
	}

	// g H restores all columns.
	m = press(m, keyRunes('g'))
	m = press(m, keyRunes('H'))

	if m.results.HiddenCount() != 0 {
		t.Errorf("after g H, expected 0 hidden, got %d", m.results.HiddenCount())
	}
}

func TestColumnVisibilityOverlayOpenApplyCancel(t *testing.T) {
	m := newResultsWorkspaceModel()

	// 'v' opens the overlay.
	m = press(m, keyRunes('v'))
	if !m.columnPicker.IsVisible() {
		t.Fatal("expected column picker visible after 'v'")
	}

	// Hide the first column via space, then apply with enter.
	m = press(m, keyRunes(' '))
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.columnPicker.IsVisible() {
		t.Error("picker should close after enter")
	}
	if !m.results.IsColumnHidden(0) {
		t.Error("column 0 should be hidden after applying selection")
	}
}

func TestColumnVisibilityOverlayCancelDiscards(t *testing.T) {
	m := newResultsWorkspaceModel()

	m = press(m, keyRunes('v'))
	m = press(m, keyRunes(' '))             // toggle (local only)
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc}) // cancel

	if m.results.HiddenCount() != 0 {
		t.Errorf("esc should discard changes; got %d hidden", m.results.HiddenCount())
	}
}

func newResultsWorkspaceModel() Model {
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state = stateWorkspace
	m.focus = FocusResults
	m.results.SetSize(80, 20)
	m.results.SetResult(
		[]string{"id", "name", "email", "age", "city"},
		[][]string{
			{"1", "alice", "alice@test.com", "30", "NYC"},
			{"2", "bob", "bob@test.com", "25", "LA"},
		},
		"2 rows",
	)
	m.results.SetEditable("users", []string{"id"})
	return m
}

func press(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func keyRunes(r ...rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: r}
}
