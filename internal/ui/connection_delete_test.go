package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
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

// --- group tabs (d / e) ----------------------------------------------------

// `d` on a connection in a group tab stages delete for that connection.
func TestDeleteFromGroupedConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{Connections: groupedConns()})
	m.state = stateConnections
	(&m).loadConnections()
	m.connList.CancelFilter()
	m.connList.setGroupTab("Work")
	m.connList.SetCursor(0) // wk-a

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)
	if mm.deleteConnConfirm != "wk-a" {
		t.Errorf("deleteConnConfirm = %q, want wk-a", mm.deleteConnConfirm)
	}
	if mm.config.GetConnection("wk-a") == nil {
		t.Error("wk-a should still be present (confirm staged, not yet deleted)")
	}
}

// `e` on a grouped connection opens the edit form pre-filled for it.
func TestEditFromGroupedConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{Connections: groupedConns()})
	m.state = stateConnections
	(&m).loadConnections()
	m.connList.CancelFilter()
	m.connList.setGroupTab("Personal")
	m.connList.SetCursor(0)

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	mm := res.(Model)
	if mm.connForm.editName != "pers-c" {
		t.Errorf("edit form editName = %q, want pers-c", mm.connForm.editName)
	}
	if mm.connForm.mode != formModeEdit {
		t.Error("expected the edit form to be in edit mode")
	}
}

// Delete targets the connection under the cursor (e.g. second in the Work tab).
func TestDeleteFromSecondGroupedConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{Connections: groupedConns()})
	m.state = stateConnections
	(&m).loadConnections()
	m.connList.CancelFilter()
	m.connList.setGroupTab("Work")
	m.connList.SetCursor(1) // wk-b

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	mm := res.(Model)
	if mm.deleteConnConfirm != "wk-b" {
		t.Errorf("deleteConnConfirm = %q, want wk-b", mm.deleteConnConfirm)
	}
}
