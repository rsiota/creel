package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/secrets"
)

// newProviderTestModel builds a workspace-state model with the config pointing
// at a throwaway directory so Save() never touches the real config file.
func newProviderTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{})
	m.state = stateWorkspace
	return m
}

// Adding a provider via the form appends it and auto-defaults it (first one).
func TestSaveProviderFormAddAutoDefaults(t *testing.T) {
	m := newProviderTestModel(t)
	m.providerForm.Show()
	m.providerForm.fields[pfName].SetValue("openai")
	m.providerForm.fields[pfKey].SetValue("sk-1")
	m.providerForm.fields[pfSecrets].SetValue("plain") // keep plaintext; no keychain needed

	cmd := (&m).saveProviderForm()
	if cmd != nil {
		t.Errorf("expected nil cmd on a clean save, got %v", cmd)
	}
	if got := m.config.GetAIProvider("openai"); got == nil || got.APIKey != "sk-1" {
		t.Errorf("provider not saved: %+v", got)
	}
	if m.config.AI.Default != "openai" {
		t.Errorf("Default = %q, want openai (first provider auto-defaults)", m.config.AI.Default)
	}
	if m.providerForm.IsVisible() {
		t.Error("form should be hidden after save")
	}
	if !m.providerPicker.IsVisible() {
		t.Error("picker should be reopened after save")
	}
}

// A duplicate name is rejected before anything is mutated.
func TestSaveProviderFormRejectsDuplicate(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "old"})

	m.providerForm.Show()
	m.providerForm.fields[pfName].SetValue("openai")
	m.providerForm.fields[pfKey].SetValue("sk-new")

	(&m).saveProviderForm()

	if m.providerForm.IsVisible() == false {
		t.Error("form should stay open on a validation error")
	}
	if m.providerForm.errMsg == "" {
		t.Error("expected a uniqueness error on the form")
	}
	// The existing provider is untouched.
	if got := m.config.GetAIProvider("openai"); got == nil || got.APIKey != "old" {
		t.Errorf("existing provider clobbered: %+v", got)
	}
}

// Editing in place (same name) updates the fields without a uniqueness clash.
func TestSaveProviderFormEditInPlace(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "sk-old", BaseURL: "https://a/v1"})
	m.config.AI.Default = "openai"

	m.providerForm.ShowEdit(config.AIProvider{Name: "openai", APIKey: "sk-old", BaseURL: "https://a/v1"})
	m.providerForm.fields[pfKey].SetValue("sk-rotated")
	m.providerForm.fields[pfSecrets].SetValue("plain")

	(&m).saveProviderForm()

	if got := m.config.GetAIProvider("openai"); got == nil || got.APIKey != "sk-rotated" {
		t.Errorf("provider not updated: %+v", got)
	}
	if m.config.AI.Default != "openai" {
		t.Errorf("Default = %q, want unchanged openai", m.config.AI.Default)
	}
	if n := len(m.config.AI.Providers); n != 1 {
		t.Errorf("provider count = %d, want 1 (edit should not duplicate)", n)
	}
}

// A rename moves the entry and (in keychain mode) leaves no orphan key.
func TestSaveProviderFormRename(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "old", APIKey: "sk-x"})

	m.providerForm.ShowEdit(config.AIProvider{Name: "old", APIKey: "sk-x"})
	m.providerForm.fields[pfName].SetValue("renamed")
	m.providerForm.fields[pfSecrets].SetValue("plain")

	(&m).saveProviderForm()

	if got := m.config.GetAIProvider("old"); got != nil {
		t.Errorf("old name still present: %+v", got)
	}
	if got := m.config.GetAIProvider("renamed"); got == nil {
		t.Error("renamed provider missing")
	}
}

// A keychain-mode rename purges the old key and stores under the new name.
func TestSaveProviderFormRenameRekeys(t *testing.T) {
	if !secrets.Available() {
		t.Skip("OS keychain not available")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	name := "creel-test-rename-before"
	t.Cleanup(func() {
		_ = secrets.DeleteAI(name)
		_ = secrets.DeleteAI("creel-test-rename-after")
	})

	// Seed a keychain entry under the old name.
	ref, err := secrets.StoreAI(name, "sk-seed")
	if err != nil {
		t.Fatalf("seed StoreAI: %v", err)
	}

	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: name, APIKey: ref})
	m.providerForm.ShowEdit(config.AIProvider{Name: name, APIKey: ref})
	m.providerForm.fields[pfName].SetValue("creel-test-rename-after")
	// Keep keychain mode (inferred from the ref) so storeProviderSecret re-keys.

	(&m).saveProviderForm()

	// The old keychain entry must be gone.
	if _, err := secrets.Resolve(ref); err == nil {
		t.Error("old keychain entry should have been purged on rename")
	}
	saved := m.config.GetAIProvider("creel-test-rename-after")
	if saved == nil || !secrets.IsReference(saved.APIKey) {
		t.Fatalf("renamed provider should hold a keychain ref, got %+v", saved)
	}
	got, err := secrets.Resolve(saved.APIKey)
	if err != nil {
		t.Fatalf("Resolve new key: %v", err)
	}
	if got != "sk-seed" {
		t.Errorf("new keychain value = %q, want sk-seed", got)
	}
}

// Deleting a provider removes it and reconciles the default. This exercises
// the executor (deleteProvider) directly; the confirm gate is covered below.
func TestDeleteProviderRemovesAndReconciles(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "sk-1"})
	m.config.AddAIProvider(config.AIProvider{Name: "anthropic", APIKey: "sk-2"})
	m.config.AI.Default = "openai"
	m.providerPicker.Show(m.config.AI.Providers, "openai") // cursor on openai

	(&m).deleteProvider("openai")

	if got := m.config.GetAIProvider("openai"); got != nil {
		t.Error("deleted provider still present")
	}
	if m.config.AI.Default == "openai" {
		t.Error("default should not point at the deleted provider")
	}
}

// Deleting the last provider drops the default entirely.
func TestDeleteProviderLastOneClearsDefault(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "only", APIKey: "sk-1"})
	m.config.AI.Default = "only"
	m.providerPicker.Show(m.config.AI.Providers, "only")

	(&m).deleteProvider("only")

	if len(m.config.AI.Providers) != 0 {
		t.Errorf("providers = %v, want empty", m.config.AI.Providers)
	}
	if m.config.AI.Default != "" {
		t.Errorf("Default = %q, want empty after deleting the last provider", m.config.AI.Default)
	}
}

// By default (confirm_destructive unset → true) `d` stages a y/n prompt
// instead of deleting immediately.
func TestDeleteProviderGatedStagesConfirm(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "sk-1"})
	m.providerPicker.Show(m.config.AI.Providers, "openai")

	cmd := (&m).deleteSelectedProvider()
	if cmd != nil {
		t.Errorf("gated delete should not run yet, got cmd %v", cmd)
	}
	if m.deleteProviderConfirm != "openai" {
		t.Errorf("deleteProviderConfirm = %q, want openai", m.deleteProviderConfirm)
	}
	// Nothing deleted yet.
	if got := m.config.GetAIProvider("openai"); got == nil {
		t.Error("provider deleted before confirmation")
	}
}

// With confirm_destructive: false, `d` deletes immediately (no prompt).
func TestDeleteProviderUngatedDeletesImmediately(t *testing.T) {
	falseVal := false
	m := newProviderTestModel(t)
	m.settings.ConfirmDestructive = &falseVal
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "sk-1"})
	m.providerPicker.Show(m.config.AI.Providers, "openai")

	(&m).deleteSelectedProvider()
	if m.deleteProviderConfirm != "" {
		t.Errorf("deleteProviderConfirm = %q, want empty when ungated", m.deleteProviderConfirm)
	}
	if got := m.config.GetAIProvider("openai"); got != nil {
		t.Error("ungated delete should remove the provider immediately")
	}
}

// The confirm prompt: `y` runs the delete, `n`/`esc` cancel leaving it intact.
func TestDeleteProviderConfirmKeysViaUpdate(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "sk-1"})
	m.providerPicker.Show(m.config.AI.Providers, "openai")

	// Stage the prompt.
	(&m).deleteSelectedProvider()

	// `n` cancels.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	mm := res.(Model)
	if mm.deleteProviderConfirm != "" {
		t.Error("n should clear the confirm flag")
	}
	if got := mm.config.GetAIProvider("openai"); got == nil {
		t.Error("provider should remain after cancelling")
	}

	// Re-stage and confirm with `y`.
	(&mm).deleteSelectedProvider()
	res, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	mm = res.(Model)
	if got := mm.config.GetAIProvider("openai"); got != nil {
		t.Error("y should delete the provider")
	}
}

// `enter` also confirms (mirrors the connection/excmd confirm conventions).
func TestDeleteProviderConfirmEnterKey(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "sk-1"})
	m.providerPicker.Show(m.config.AI.Providers, "openai")
	(&m).deleteSelectedProvider()

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := res.(Model)
	if got := mm.config.GetAIProvider("openai"); got != nil {
		t.Error("enter should confirm the delete")
	}
}

// `n` from the `M` picker opens the add form.
func TestProviderPickerNKeyOpensForm(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "sk-1"})
	m.providerPicker.Show(m.config.AI.Providers, "openai")

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	mm := res.(Model)
	// The form-open message is delivered by the returned cmd; executing it
	// routes to the openProviderFormAddMsg handler which shows the form.
	if cmd == nil {
		t.Fatal("expected a cmd carrying the form-open message")
	}
	msg := cmd()
	if _, ok := msg.(openProviderFormAddMsg); !ok {
		t.Fatalf("cmd msg = %T, want openProviderFormAddMsg", msg)
	}
	res2, _ := mm.Update(msg)
	if !res2.(Model).providerForm.IsVisible() {
		t.Error("form should be visible after the n key")
	}
}

// `e` from the `M` picker carries the selected provider name to pre-fill.
func TestProviderPickerEKeyOpensEdit(t *testing.T) {
	m := newProviderTestModel(t)
	m.config.AddAIProvider(config.AIProvider{Name: "openai", APIKey: "sk-1", BaseURL: "https://a/v1"})
	m.providerPicker.Show(m.config.AI.Providers, "openai")

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	mm := res.(Model)
	msg := cmd()
	em, ok := msg.(openProviderFormEditMsg)
	if !ok {
		t.Fatalf("cmd msg = %T, want openProviderFormEditMsg", msg)
	}
	if em.name != "openai" {
		t.Errorf("edit msg name = %q, want openai", em.name)
	}
	res2, _ := mm.Update(msg)
	form := res2.(Model).providerForm
	if !form.IsVisible() || form.editName != "openai" {
		t.Errorf("form not pre-filled: visible=%v editName=%q", form.IsVisible(), form.editName)
	}
}

// ctrl+t in the form kicks off a probe (testing flag becomes true).
func TestProviderFormCtrlTStartsTest(t *testing.T) {
	m := newProviderTestModel(t)
	m.providerForm.Show()
	m.providerForm.fields[pfKey].SetValue("sk-test")

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	mm := res.(Model)
	if !mm.providerForm.testing {
		t.Error("ctrl+t should set the testing flag")
	}
	if cmd == nil {
		t.Error("ctrl+t should return a probe cmd")
	}
}

// ctrl+t is ignored while a probe is already in flight.
func TestProviderFormCtrlTBlockedWhileTesting(t *testing.T) {
	m := newProviderTestModel(t)
	m.providerForm.Show()
	m.providerForm.fields[pfKey].SetValue("sk-test")
	m.providerForm.testing = true

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	mm := res.(Model)
	if !mm.providerForm.testing {
		t.Error("testing flag should remain true")
	}
	if cmd != nil {
		t.Error("ctrl+t while testing should not start a second probe")
	}
}

// A probe result routes back to the form (field tinting + message).
func TestProviderTestResultRoutesToForm(t *testing.T) {
	m := newProviderTestModel(t)
	m.providerForm.Show()

	// Success path.
	res, _ := m.Update(providerTestResultMsg{err: nil})
	mm := res.(Model)
	if !mm.providerForm.testOK || mm.providerForm.testMsg == "" {
		t.Errorf("expected a success message, got ok=%v msg=%q", mm.providerForm.testOK, mm.providerForm.testMsg)
	}

	// Failure path attributes the error to the key.
	res, _ = m.Update(providerTestResultMsg{err: errStringer("401 Unauthorized")})
	mm = res.(Model)
	if mm.providerForm.testOK {
		t.Error("expected failure on an auth error")
	}
	if mm.providerForm.statusOf(pfKey) != testFail {
		t.Error("auth error should tint the API Key field")
	}
}

// With NO providers configured (the "add my first one" case), `M` must still
// open the picker so `n` is reachable — otherwise the form could never be
// opened. The empty picker renders a placeholder.
func TestMOpensPickerEvenWithNoProviders(t *testing.T) {
	m := newProviderTestModel(t) // empty config: zero providers

	res, cmd := m.Update(openProviderPickerMsg{})
	mm := res.(Model)
	if !mm.providerPicker.IsVisible() {
		t.Fatal("M with no providers should still open the picker")
	}
	_ = cmd

	// `n` from the empty picker opens the add form.
	res, cmd = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	mm = res.(Model)
	if cmd == nil {
		t.Fatal("n should emit an openProviderFormAddMsg")
	}
	if _, ok := cmd().(openProviderFormAddMsg); !ok {
		t.Fatalf("n cmd msg = %T, want openProviderFormAddMsg", cmd())
	}
}

// The empty picker renders a placeholder row (and no selection highlight).
func TestProviderPickerEmptyPlaceholder(t *testing.T) {
	p := NewProviderPicker()
	p.Show(nil, "")
	out := p.View()
	if !strings.Contains(out, "press n to add") {
		t.Errorf("empty picker should show a press-n placeholder, got: %s", out)
	}
	if p.Selected() != "" {
		t.Errorf("empty picker Selected = %q, want empty", p.Selected())
	}
}
