package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

// newInspectorMouseModel builds a workspace model with the inspector visible
// and editable results, for testing inspector mouse interactions.
func newInspectorMouseModel(t *testing.T) Model {
	t.Helper()
	cfg := &config.Config{}
	m := NewModel(cfg)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusResults
	m.inspector.visible = true
	m.results.SetResult(
		[]string{"id", "user_id", "email", "created_at"},
		[][]string{{"1", "42", "alice@test.com", "2024-01-01"}},
		"1 row",
	)
	m.results.SetEditable("users", []string{"id"})
	m.layoutWorkspace()
	return m
}

// inspectorFieldScreenY returns the absolute screen Y of the inspector field
// label for the given column name, so tests don't hardcode magic offsets.
// The inspector's top border adds 1 to the content-relative line.
func inspectorFieldScreenY(t *testing.T, m Model, colName string) int {
	t.Helper()
	view := ansiStrip.ReplaceAllString(m.inspector.View(m.results), "")
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, colName) {
			return i + 1 // +1 for the inspector's top border
		}
	}
	t.Fatalf("inspector field %q not found", colName)
	return -1
}

// Single click on an inspector field should focus the inspector and move the
// field cursor to the clicked field, without entering edit mode.
func TestInspectorClickFocusesAndMovesFieldCursor(t *testing.T) {
	m := newInspectorMouseModel(t)
	x := m.width - 10 // well inside the inspector column

	yEmail := inspectorFieldScreenY(t, m, "email")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)
	if m.focus != FocusInspector {
		t.Fatalf("focus=%v, want FocusInspector", m.focus)
	}
	if m.inspector.cursorField != 2 {
		t.Errorf("cursorField=%d, want 2 (email)", m.inspector.cursorField)
	}
	if m.inspector.IsEditing() {
		t.Error("single click should not enter edit mode")
	}
}

// Double-click on an inspector field should enter field edit mode.
func TestInspectorDoubleClickEntersEditMode(t *testing.T) {
	m := newInspectorMouseModel(t)
	x := m.width - 10
	yEmail := inspectorFieldScreenY(t, m, "email")

	// First click: focus + move cursor, no edit.
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)
	if m.inspector.IsEditing() {
		t.Fatal("single click should not enter edit mode")
	}

	// Second click on the same field: enters edit mode.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)
	if !m.inspector.IsEditing() {
		t.Errorf("double click should enter field edit mode")
	}
}

// Clicking two different fields in quick succession must not trigger edit mode.
func TestInspectorDoubleClickDifferentFieldsDoesNotEdit(t *testing.T) {
	m := newInspectorMouseModel(t)
	x := m.width - 10
	yEmail := inspectorFieldScreenY(t, m, "email")
	yUser := inspectorFieldScreenY(t, m, "user_id")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yUser})
	m = out.(Model)
	if m.inspector.IsEditing() {
		t.Errorf("clicking two different fields should not enter edit mode")
	}
}

// Double-clicking a primary-key field (which can't be edited) must not enter
// edit mode.
func TestInspectorDoubleClickPKFieldDoesNotEdit(t *testing.T) {
	m := newInspectorMouseModel(t)
	x := m.width - 10
	yID := inspectorFieldScreenY(t, m, "* id")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yID})
	m = out.(Model)
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yID})
	m = out.(Model)
	if m.inspector.IsEditing() {
		t.Errorf("double click on PK field should not enter edit mode")
	}
}

func TestInspectorWheelMovesFieldCursor(t *testing.T) {
	m := newInspectorMouseModel(t)
	x := m.width - 10
	y := inspectorFieldScreenY(t, m, "id")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseWheelDown, X: x, Y: y})
	m = out.(Model)
	if m.focus != FocusInspector {
		t.Fatalf("focus=%v, want FocusInspector", m.focus)
	}
	if m.inspector.cursorField != 1 {
		t.Fatalf("cursorField=%d, want 1 after wheel down", m.inspector.cursorField)
	}
	if m.results.CursorCol() != 1 {
		t.Fatalf("results col=%d, want 1 (column sync)", m.results.CursorCol())
	}
	if m.inspector.IsEditing() {
		t.Error("wheel should not enter edit mode")
	}

	m.lastInspectorWheelTime = time.Time{}
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseWheelUp, X: x, Y: y})
	m = out.(Model)
	if m.inspector.cursorField != 0 {
		t.Fatalf("cursorField=%d, want 0 after wheel up", m.inspector.cursorField)
	}
}

func TestInspectorWheelDebouncesNotch(t *testing.T) {
	m := newInspectorMouseModel(t)
	x := m.width - 10
	y := inspectorFieldScreenY(t, m, "id")

	for i := 0; i < 3; i++ {
		out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseWheelDown, X: x, Y: y})
		m = out.(Model)
	}
	if m.inspector.cursorField != 1 {
		t.Errorf("after 3-event notch: cursorField=%d, want 1", m.inspector.cursorField)
	}

	m.lastInspectorWheelTime = time.Now().Add(-connFormWheelInterval - time.Millisecond)
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseWheelDown, X: x, Y: y})
	m = out.(Model)
	if m.inspector.cursorField != 2 {
		t.Errorf("after debounce window: cursorField=%d, want 2", m.inspector.cursorField)
	}
}

func TestInspectorWheelWhileEditingCommits(t *testing.T) {
	m := newInspectorMouseModel(t)
	m.focus = FocusInspector
	m.inspector.cursorField = 1 // user_id
	m.inspector.StartFieldEdit(m.results)
	m.inspector.editInput.SetValue("99")

	x := m.width - 10
	y := inspectorFieldScreenY(t, m, "user_id")
	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseWheelDown, X: x, Y: y})
	m = out.(Model)

	if m.inspector.IsEditing() {
		t.Fatal("wheel should end edit mode")
	}
	if m.inspector.cursorField != 2 {
		t.Fatalf("cursorField=%d, want 2 (email)", m.inspector.cursorField)
	}
	if m.results.RowValue(0, 1) != "99" {
		t.Fatalf("staged user_id = %q, want 99", m.results.RowValue(0, 1))
	}
}
