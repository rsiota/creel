package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/creel/internal/config"
)

// newConnListTestModel builds a model on the connection list screen with two
// connections and the cursor on the first. XDG_CONFIG_HOME is redirected so
// Save() never touches the real config file.
func newConnListTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{Connections: []config.ConnectionConfig{
		{Name: "alpha", Driver: "sqlite", Database: ":memory:"},
		{Name: "beta", Driver: "sqlite", Database: ":memory:"},
	}})
	m.state = stateConnections
	(&m).loadConnections()
	m.connList.SetCursor(0)
	// NewModel starts the list in filter mode (the live connection screen
	// does too); cancel it so the n/e/d command keys are reachable, mirroring
	// how a user presses esc first.
	m.connList.CancelFilter()
	return m
}

// By default `d` stages a y/n prompt instead of deleting immediately.
func TestDeleteConnectionGatedStagesConfirm(t *testing.T) {
	m := newConnListTestModel(t)

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)
	if mm.deleteConnConfirm != "alpha" {
		t.Errorf("deleteConnConfirm = %q, want alpha", mm.deleteConnConfirm)
	}
	if got := mm.config.GetConnection("alpha"); got == nil {
		t.Error("alpha deleted before confirmation")
	}
}

// With confirm_destructive: false, `d` deletes immediately.
func TestDeleteConnectionUngatedDeletesImmediately(t *testing.T) {
	falseVal := false
	m := newConnListTestModel(t)
	m.settings.ConfirmDestructive = &falseVal

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)
	if mm.deleteConnConfirm != "" {
		t.Errorf("deleteConnConfirm = %q, want empty when ungated", mm.deleteConnConfirm)
	}
	if got := mm.config.GetConnection("alpha"); got != nil {
		t.Error("ungated d should delete alpha immediately")
	}
}

// `y` confirms the staged delete and removes the connection.
func TestDeleteConnectionConfirmY(t *testing.T) {
	m := newConnListTestModel(t)
	// Stage.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)

	res, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	mm = res.(Model)
	if mm.deleteConnConfirm != "" {
		t.Error("y should clear the confirm flag")
	}
	if got := mm.config.GetConnection("alpha"); got != nil {
		t.Error("y should delete alpha")
	}
}

// `enter` also confirms; `n`/`esc` cancel leaving the connection intact.
func TestDeleteConnectionConfirmCancel(t *testing.T) {
	// enter confirms.
	m := newConnListTestModel(t)
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	res, _ = res.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := res.(Model).config.GetConnection("alpha"); got != nil {
		t.Error("enter should confirm the delete")
	}

	// n cancels.
	m = newConnListTestModel(t)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	res, _ = res.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	mm := res.(Model)
	if mm.deleteConnConfirm != "" {
		t.Error("n should clear the confirm flag")
	}
	if got := mm.config.GetConnection("alpha"); got == nil {
		t.Error("alpha should remain after cancelling")
	}

	// esc cancels too.
	m = newConnListTestModel(t)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	res, _ = res.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm = res.(Model)
	if mm.deleteConnConfirm != "" || mm.config.GetConnection("alpha") == nil {
		t.Error("esc should cancel and keep alpha")
	}
}

// While the confirm prompt is up, navigation keys are swallowed (the dialog is
// modal) — the cursor does not move.
func TestDeleteConnectionConfirmSwallowsKeys(t *testing.T) {
	m := newConnListTestModel(t)
	before := m.connList.SelectedName()
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)

	// j is swallowed (not a confirm key), selection unchanged.
	res, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	mm = res.(Model)
	if got := mm.connList.SelectedName(); got != before {
		t.Errorf("selection moved to %q during confirm; want %q", got, before)
	}
	if mm.deleteConnConfirm == "" {
		t.Error("swallowed key should not dismiss the confirm")
	}
}

// --- group-header stepping (d / e) -----------------------------------------

// findGroupHeader returns the cursor index of a group's header row, or -1.
func findGroupHeader(c ConnectionList, group string) int {
	for i, r := range c.rows() {
		if r.kind == rowGroup && r.group == group {
			return i
		}
	}
	return -1
}

// `d` on a group header steps onto that group's first connection and stages
// the delete confirmation for it (rather than silently no-op'ing).
func TestDeleteFromGroupHeaderStepsToFirstConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{Connections: groupedConns()})
	m.state = stateConnections
	(&m).loadConnections()
	m.connList.CancelFilter()
	m.connList.SetCursor(findGroupHeader(m.connList, "Work")) // Work header
	if m.connList.SelectedName() != "" {
		t.Fatalf("cursor should be on the Work header, got selection %q", m.connList.SelectedName())
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)
	if mm.deleteConnConfirm != "wk-a" {
		t.Errorf("deleteConnConfirm = %q, want wk-a (first Work connection)", mm.deleteConnConfirm)
	}
	if got := mm.connList.SelectedName(); got != "wk-a" {
		t.Errorf("cursor = %q after d on header, want wk-a", got)
	}
	if mm.config.GetConnection("wk-a") == nil {
		t.Error("wk-a should still be present (confirm staged, not yet deleted)")
	}
}

// `e` on a group header steps onto the first connection and opens the edit
// form pre-filled for it.
func TestEditFromGroupHeaderStepsToFirstConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{Connections: groupedConns()})
	m.state = stateConnections
	(&m).loadConnections()
	m.connList.CancelFilter()
	m.connList.SetCursor(findGroupHeader(m.connList, "Personal")) // Personal header

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	mm := res.(Model)
	if mm.connForm.editName != "pers-c" {
		t.Errorf("edit form editName = %q, want pers-c", mm.connForm.editName)
	}
	if mm.connForm.mode != formModeEdit {
		t.Error("expected the edit form to be in edit mode")
	}
}

// `d` on a *collapsed* group's header expands it and then selects the first
// connection (otherwise the target row would be hidden and unreachable).
func TestDeleteFromCollapsedGroupHeaderExpandsAndSteps(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{Connections: groupedConns()})
	m.state = stateConnections
	(&m).loadConnections()
	m.connList.CancelFilter()
	m.connList.SetCursor(findGroupHeader(m.connList, "Work"))
	m.connList.ToggleGroupAtCursor() // collapse Work

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)
	if mm.deleteConnConfirm != "wk-a" {
		t.Errorf("deleteConnConfirm = %q, want wk-a after expanding collapsed group", mm.deleteConnConfirm)
	}
	if got := mm.connList.SelectedName(); got != "wk-a" {
		t.Errorf("cursor = %q, want wk-a", got)
	}
}

// Stepping is a no-op when the cursor is already on a connection (the common
// case): the selection is unchanged and the confirm stages for it as usual.
func TestDeleteFromConnectionDoesNotStep(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{Connections: groupedConns()})
	m.state = stateConnections
	(&m).loadConnections()
	m.connList.CancelFilter()
	m.connList.SetCursor(findGroupHeader(m.connList, "Work") + 2) // wk-b (2nd Work conn)

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)
	if mm.deleteConnConfirm != "wk-b" {
		t.Errorf("deleteConnConfirm = %q, want wk-b", mm.deleteConnConfirm)
	}
}
