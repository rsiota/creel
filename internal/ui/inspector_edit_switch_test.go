package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func setupInspectorEditModel(t *testing.T) Model {
	t.Helper()
	cfg := &config.Config{}
	m := NewModel(cfg)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
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
	m.layoutWorkspace()
	return m
}

// Moving the field cursor with j while editing must commit the in-flight
// value onto its own column and leave edit mode, so the newly focused field
// shows its own value — not the shared textinput buffer.
func TestInspectorJWhileEditingCommitsAndMoves(t *testing.T) {
	m := setupInspectorEditModel(t)
	m.inspector.cursorField = 1 // name
	m.inspector.StartFieldEdit(m.results)
	m.inspector.editInput.SetValue("alice-edited")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)

	if m.inspector.IsEditing() {
		t.Fatal("j while editing should end edit mode")
	}
	if m.inspector.cursorField != 2 {
		t.Fatalf("cursorField=%d, want 2 (email)", m.inspector.cursorField)
	}
	if !m.results.IsDirty(0, 1) || m.results.RowValue(0, 1) != "alice-edited" {
		t.Fatalf("name edit should be staged, got %q dirty=%v",
			m.results.RowValue(0, 1), m.results.IsDirty(0, 1))
	}
	view := m.inspector.View(m.results)
	plain := ansiStrip.ReplaceAllString(view, "")
	if !strings.Contains(plain, "alice@test.com") {
		t.Fatalf("email field should show its own value:\n%s", plain)
	}
	// Focused field must not be rendering the shared edit buffer.
	if m.inspector.IsEditing() {
		t.Fatal("should not still be editing after j")
	}
}

func TestInspectorClickOtherFieldWhileEditingCommits(t *testing.T) {
	m := setupInspectorEditModel(t)
	x := m.width - 10

	yName := inspectorFieldScreenY(t, m, "name")
	yEmail := inspectorFieldScreenY(t, m, "email")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yName})
	m = out.(Model)
	m.inspector.StartFieldEdit(m.results)
	m.inspector.editInput.SetValue("from-name")

	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)

	if m.inspector.IsEditing() {
		t.Fatal("clicking another field should end edit mode")
	}
	if m.inspector.cursorField != 2 {
		t.Fatalf("cursorField=%d, want 2 (email)", m.inspector.cursorField)
	}
	if m.results.RowValue(0, 1) != "from-name" {
		t.Fatalf("name edit should be staged, got %q", m.results.RowValue(0, 1))
	}
}

func TestInspectorClickOtherFieldUnchangedDoesNotDirty(t *testing.T) {
	m := setupInspectorEditModel(t)
	x := m.width - 10

	yName := inspectorFieldScreenY(t, m, "name")
	yEmail := inspectorFieldScreenY(t, m, "email")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yName})
	m = out.(Model)
	m.inspector.StartFieldEdit(m.results)
	// Leave the value untouched, then click away.
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)

	if m.inspector.IsEditing() {
		t.Fatal("click-away should end edit mode")
	}
	if m.results.IsDirty(0, 1) {
		t.Fatal("unchanged field must not be marked dirty")
	}
	if m.results.RowValue(0, 1) != "alice" {
		t.Fatalf("name = %q, want alice", m.results.RowValue(0, 1))
	}
}

func TestInspectorClickSameFieldWhileEditingKeepsEdit(t *testing.T) {
	m := setupInspectorEditModel(t)
	x := m.width - 10
	yEmail := inspectorFieldScreenY(t, m, "email")

	out, _ := m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)
	m.inspector.StartFieldEdit(m.results)
	m.inspector.editInput.SetValue("still-typing")

	// Same-field click should not commit (mirrors results-grid behaviour).
	m.lastInspectorClickTime = time.Time{} // avoid double-click path
	out, _ = m.handleWorkspaceMouse(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: yEmail})
	m = out.(Model)

	if !m.inspector.IsEditing() {
		t.Fatal("clicking the field being edited should keep edit mode")
	}
	if m.inspector.editInput.Value() != "still-typing" {
		t.Fatalf("edit buffer = %q, want still-typing", m.inspector.editInput.Value())
	}
}
