package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func setupInspectorColSyncModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusInspector
	m.inspector.visible = true
	m.results.SetResult(
		[]string{"id", "name", "email"},
		[][]string{{"1", "alice", "alice@test.com"}},
		"1 row",
	)
	m.results.SetEditable("users", []string{"id"})
	m.results.SetCursor(0, 0)
	m.layoutWorkspace()
	return m
}

func TestInspectorJKMovesResultsColumn(t *testing.T) {
	m := setupInspectorColSyncModel(t)
	if m.results.CursorCol() != 0 {
		t.Fatalf("start col = %d, want 0", m.results.CursorCol())
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.inspector.cursorField != 1 {
		t.Fatalf("cursorField=%d, want 1", m.inspector.cursorField)
	}
	if m.results.CursorCol() != 1 {
		t.Fatalf("results col=%d, want 1 (name)", m.results.CursorCol())
	}
	if m.focus != FocusInspector {
		t.Fatalf("focus=%v, want FocusInspector (must not steal focus)", m.focus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.results.CursorCol() != 2 {
		t.Fatalf("results col=%d, want 2 (email)", m.results.CursorCol())
	}
}

func TestResultsHLMovesInspectorField(t *testing.T) {
	m := setupInspectorColSyncModel(t)
	m.focus = FocusResults
	m.results.SetCursor(0, 0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)
	if m.results.CursorCol() != 1 {
		t.Fatalf("results col=%d, want 1", m.results.CursorCol())
	}
	if m.inspector.cursorField != 1 {
		t.Fatalf("inspector field=%d, want 1", m.inspector.cursorField)
	}
	if m.focus != FocusResults {
		t.Fatalf("focus=%v, want FocusResults", m.focus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)
	if m.inspector.cursorField != 2 {
		t.Fatalf("inspector field=%d, want 2", m.inspector.cursorField)
	}
}

func TestInspectorSyncSkipsHiddenGridColumn(t *testing.T) {
	m := setupInspectorColSyncModel(t)
	if !m.results.HideColumn(1) {
		t.Fatal("hide name")
	}
	m.results.SetCursor(0, 0)
	m.inspector.cursorField = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.inspector.cursorField != 1 {
		t.Fatalf("inspector should still land on hidden name, field=%d", m.inspector.cursorField)
	}
	if m.results.CursorCol() != 0 {
		t.Fatalf("grid col=%d, want 0 (hidden name must not take the grid cursor)", m.results.CursorCol())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.results.CursorCol() != 2 {
		t.Fatalf("grid col=%d, want 2 (email is visible)", m.results.CursorCol())
	}
}

func TestInspectorInsertDoesNotSyncGridColumn(t *testing.T) {
	m := setupInspectorColSyncModel(t)
	m.inspector.StartInsert()
	m.results.SetCursor(0, 0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.inspector.cursorField != 1 {
		t.Fatalf("cursorField=%d, want 1", m.inspector.cursorField)
	}
	if m.results.CursorCol() != 0 {
		t.Fatalf("grid col=%d, want 0 during insert", m.results.CursorCol())
	}
}

func TestToggleInspectorLandsOnGridColumn(t *testing.T) {
	m := setupInspectorColSyncModel(t)
	m.inspector.Hide()
	m.focus = FocusResults
	m.results.SetCursor(0, 2)

	m.toggleInspector()
	if m.focus != FocusInspector {
		t.Fatalf("focus=%v, want FocusInspector", m.focus)
	}
	if m.inspector.cursorField != 2 {
		t.Fatalf("inspector field=%d, want 2 (email, matching grid)", m.inspector.cursorField)
	}
}

func TestInspectorClickSyncsGridColumn(t *testing.T) {
	m := setupInspectorColSyncModel(t)
	m.focus = FocusResults
	x := m.width - 10
	yEmail := inspectorFieldScreenY(t, m, "email")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)
	if m.inspector.cursorField != 2 {
		t.Fatalf("cursorField=%d, want 2", m.inspector.cursorField)
	}
	if m.results.CursorCol() != 2 {
		t.Fatalf("results col=%d, want 2", m.results.CursorCol())
	}
}
