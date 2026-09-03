package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func setupInspectorFKModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateWorkspace
	m.focus = FocusInspector
	m.inspector.visible = true
	m.results.SetResult(
		[]string{"id", "dept_id", "name"},
		[][]string{
			{"1", "10", "alice"},
			{"2", "NULL", "bob"},
		},
		"2 rows",
	)
	m.results.SetEditable("employees", []string{"id"})
	m.results.SetForeignKeys("employees", []db.ForeignKey{
		{Column: "dept_id", RefTable: "departments", RefColumn: "id"},
	})
	m.results.SetCursor(0, 0) // grid cursor on id — inspector must not use this for g d
	m.layoutWorkspace()
	return m
}

func TestInspectorShowsFKTargetMarker(t *testing.T) {
	m := setupInspectorFKModel(t)
	m.inspector.cursorField = 1 // dept_id
	view := ansiStrip.ReplaceAllString(m.inspector.View(m.results), "")
	if !strings.Contains(view, "→ departments.id") {
		t.Fatalf("expected FK target marker, view:\n%s", view)
	}
	// Non-FK field still shows a type, not a false arrow.
	m.inspector.cursorField = 2
	view = ansiStrip.ReplaceAllString(m.inspector.View(m.results), "")
	if strings.Count(view, "→ departments.id") != 1 {
		t.Fatalf("FK marker should appear once (on dept_id only):\n%s", view)
	}
}

func TestInspectorGDFollowsFocusedField(t *testing.T) {
	m := setupInspectorFKModel(t)
	m.inspector.cursorField = 1 // dept_id (value 10), not grid cursor col 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	if !m.inspector.pendingG {
		t.Fatal("expected pendingG after g")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("g d should return followForeignKey command")
	}
	q := m.editor.Value()
	if !strings.Contains(q, "departments") || !strings.Contains(q, "10") {
		t.Fatalf("editor query = %q, want SELECT on departments for id=10", q)
	}
}

func TestInspectorGDIgnoresNonFKField(t *testing.T) {
	m := setupInspectorFKModel(t)
	m.inspector.cursorField = 2 // name
	m.results.SetCursor(0, 1)   // grid is on the FK — must not follow that

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("g d on non-FK inspector field should no-op")
	}
	if m.editor.Value() != "" && strings.Contains(m.editor.Value(), "departments") {
		t.Fatalf("should not have followed grid FK, editor=%q", m.editor.Value())
	}
}

func TestInspectorGDSkipsNullFK(t *testing.T) {
	m := setupInspectorFKModel(t)
	m.results.SetCursor(1, 0)
	m.inspector.cursorField = 1 // bob's NULL dept_id

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatal("g d on NULL FK should no-op")
	}
}

func TestExFollowUsesInspectorField(t *testing.T) {
	m := setupInspectorFKModel(t)
	m.inspector.cursorField = 1
	m.results.SetCursor(0, 0)

	cmd := m.exFollow()
	if cmd == nil {
		t.Fatal("exFollow should follow inspector FK field")
	}
	if !strings.Contains(m.editor.Value(), "departments") {
		t.Fatalf("editor = %q, want departments query", m.editor.Value())
	}
}

func TestInspectorUAndGBGoBack(t *testing.T) {
	m := setupInspectorFKModel(t)
	m.editor.SetValue("SELECT * FROM departments WHERE id = '10';")
	m.lastQuery = "SELECT * FROM departments WHERE id = '10';"
	m.queryStack = []queryStackEntry{{
		query: "SELECT * FROM employees;",
		page:  0,
	}}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("u should go back")
	}
	if len(m.queryStack) != 0 {
		t.Fatalf("stack len = %d, want 0 after u", len(m.queryStack))
	}
	if !strings.Contains(m.editor.Value(), "employees") {
		t.Fatalf("editor = %q, want restored employees query", m.editor.Value())
	}
	// goBackQuery arms queryRunning when building its cmd; clear it so the
	// next keystrokes aren't swallowed the way an in-flight query would.
	m.queryRunning = false
	m.queryCancel = nil

	// Push again, then g b.
	m.queryStack = []queryStackEntry{{query: "SELECT * FROM employees;", page: 0}}
	m.editor.SetValue("SELECT * FROM departments WHERE id = '10';")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("g b should go back")
	}
	if len(m.queryStack) != 0 {
		t.Fatalf("stack len = %d after g b, want 0", len(m.queryStack))
	}
}

func TestInspectorGoBackEmptyStack(t *testing.T) {
	m := setupInspectorFKModel(t)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updated.(Model)
	if m.schemaMsg != "nothing to go back to" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
	if len(m.queryStack) != 0 {
		t.Fatalf("stack should stay empty, len=%d", len(m.queryStack))
	}
}
