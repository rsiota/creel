package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/config"
)

// Show positions the cursor on the active theme, treating empty as the default.
func TestThemePickerShowCursor(t *testing.T) {
	p := NewThemePicker()
	p.Show("")
	if !p.IsVisible() {
		t.Fatal("picker should be visible after Show")
	}
	if p.AppliedAtOpen() != defaultThemeName {
		t.Errorf("AppliedAtOpen = %q, want %q for empty input", p.AppliedAtOpen(), defaultThemeName)
	}
	if p.Selected() != defaultThemeName {
		t.Errorf("cursor = %q, want default %q", p.Selected(), defaultThemeName)
	}

	p.Show("nord")
	if p.AppliedAtOpen() != "nord" {
		t.Errorf("AppliedAtOpen = %q, want nord", p.AppliedAtOpen())
	}
	if p.Selected() != "nord" {
		t.Errorf("cursor = %q, want nord", p.Selected())
	}
}

// Up/Down move the cursor and live-apply the selected palette so the package
// color vars track the cursor.
func TestThemePickerLivePreview(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	p := NewThemePicker()
	p.Show("tokyo-night") // cursor on tokyo-night; Show does not re-apply

	p.Down() // -> gruvbox
	if p.Selected() != "gruvbox" {
		t.Fatalf("after Down, selected = %q, want gruvbox", p.Selected())
	}
	if colorPrimary != gruvboxPalette.primary {
		t.Errorf("Down did not live-apply gruvbox: colorPrimary = %s", colorPrimary)
	}

	p.Down() // -> nord
	if colorPrimary != nordPalette.primary {
		t.Errorf("Down did not live-apply nord: colorPrimary = %s", colorPrimary)
	}

	p.Up() // -> gruvbox
	if colorPrimary != gruvboxPalette.primary {
		t.Errorf("Up did not live-apply gruvbox: colorPrimary = %s", colorPrimary)
	}

	// Clamping: Up past the top stays on the first theme.
	p.Up()
	p.Up()
	if p.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", p.cursor)
	}
}

// Commit hides the picker and returns the selected name; the palette remains
// applied (no re-apply needed by the caller).
func TestThemePickerCommit(t *testing.T) {
	defer applyPalette(defaultPalette)

	p := NewThemePicker()
	p.Show("tokyo-night")
	p.Down() // gruvbox
	name := p.Commit()
	if name != "gruvbox" {
		t.Fatalf("Commit = %q, want gruvbox", name)
	}
	if p.IsVisible() {
		t.Error("picker should be hidden after Commit")
	}
	if colorPrimary != gruvboxPalette.primary {
		t.Error("palette should still be gruvbox after Commit")
	}
}

// Hide hides the picker without touching the active palette.
func TestThemePickerHideDoesNotApply(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	p := NewThemePicker()
	p.Show("tokyo-night")
	p.Hide()
	if p.IsVisible() {
		t.Error("picker should be hidden after Hide")
	}
	if colorPrimary != defaultPalette.primary {
		t.Error("Hide changed the palette; it should be a no-op on colors")
	}
}

// View renders nothing when hidden and a framed panel when visible.
func TestThemePickerView(t *testing.T) {
	p := NewThemePicker()
	if got := p.View(); got != "" {
		t.Errorf("hidden View = %q, want empty", got)
	}
	p.Show("tokyo-night")
	got := p.View()
	if got == "" {
		t.Fatal("visible View should be non-empty")
	}
	// The cursor's theme is visible (Show starts it in the first window).
	if !strings.Contains(got, "tokyo-night") {
		t.Errorf("View missing the selected theme name %q", "tokyo-night")
	}
	// Width and height match the column-visibility picker (opened with v),
	// which uses popupDim.
	popupW, popupH := popupDim()
	if w := lipgloss.Width(got); w != popupW {
		t.Errorf("View width = %d, want %d (popupDim width)", w, popupW)
	}
	if h := lipgloss.Height(got); h != popupH {
		t.Errorf("View height = %d, want %d (popupDim height)", h, popupH)
	}
	// The swatch dots are right-aligned: each theme row's visible content ends
	// with a dot just inside the right border (the border and 1-char padding
	// follow it).
	for _, line := range strings.Split(got, "\n") {
		vis := stripAnsi(line)
		name := ""
		for _, n := range themeNames() {
			if strings.Contains(vis, n) {
				name = n
				break
			}
		}
		if name == "" {
			continue // border line
		}
		vis = strings.TrimRight(vis, " \t")
		vis = strings.TrimSuffix(vis, "│")
		vis = strings.TrimRight(vis, " ")
		if !strings.HasSuffix(vis, "●") {
			t.Errorf("row for %q does not end with a swatch dot: %q", name, line)
		}
	}
}

// TestThemePickerScroll verifies the picker scrolls when the cursor moves
// past the visible window (needed once the catalog exceeds the panel height).
func TestThemePickerScroll(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	p := NewThemePicker()
	p.Show("tokyo-night")
	_, popupH := popupDim()
	maxVisible := popupH - 2

	// The first theme is visible initially.
	if !strings.Contains(stripAnsi(p.View()), "tokyo-night") {
		t.Fatal("initial view should contain the first theme")
	}
	// Move the cursor past the bottom of the window; the view scrolls and the
	// first theme leaves the visible area while the cursor's theme appears.
	for i := 0; i < maxVisible; i++ {
		p.Down()
	}
	view := stripAnsi(p.View())
	if strings.Contains(view, "tokyo-night") {
		t.Errorf("after scrolling past the first window, tokyo-night should be off-screen")
	}
	if !strings.Contains(view, p.Selected()) {
		t.Errorf("scrolled view should contain the cursor's theme %q", p.Selected())
	}
}

// --- integration via Update ---

// newWorkspaceModel returns a Model positioned in the results workspace with
// an empty results table, ready to drive with key messages.
func newWorkspaceModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state = stateWorkspace
	m.focus = FocusResults
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	return m
}

func sendKey(m Model, msg tea.KeyMsg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

// g c opens the theme picker from the workspace.
func TestThemePickerOpenViaGC(t *testing.T) {
	defer applyPalette(defaultPalette)
	m := newWorkspaceModel(t)

	m = sendKey(m, runeKey('g'))
	m = sendKey(m, runeKey('c'))

	if !m.themePicker.IsVisible() {
		t.Fatal("g c should open the theme picker")
	}
	if m.themePicker.Selected() != defaultThemeName {
		t.Errorf("picker cursor = %q, want default %q", m.themePicker.Selected(), defaultThemeName)
	}
}

// j live-previews the next theme; esc reverts the palette to the open-time
// theme and closes the picker.
func TestThemePickerEscReverts(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)
	m := newWorkspaceModel(t)

	m = sendKey(m, runeKey('g'))
	m = sendKey(m, runeKey('c'))
	m = sendKey(m, runeKey('j')) // preview gruvbox

	if colorPrimary != gruvboxPalette.primary {
		t.Fatalf("j should live-preview gruvbox, colorPrimary = %s", colorPrimary)
	}

	m = sendKey(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.themePicker.IsVisible() {
		t.Error("esc should close the picker")
	}
	if colorPrimary != defaultPalette.primary {
		t.Errorf("esc should revert to default, colorPrimary = %s", colorPrimary)
	}
}

// enter persists the previewed theme to settings and the config file.
func TestThemePickerCommitPersists(t *testing.T) {
	defer applyPalette(defaultPalette)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // redirect config write

	m := newWorkspaceModel(t)
	m = sendKey(m, runeKey('g'))
	m = sendKey(m, runeKey('c'))
	m = sendKey(m, runeKey('j')) // preview gruvbox
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.themePicker.IsVisible() {
		t.Error("enter should close the picker")
	}
	if m.settings.Theme != "gruvbox" {
		t.Errorf("settings.Theme = %q, want gruvbox", m.settings.Theme)
	}
	if m.config.Settings.Theme != "gruvbox" {
		t.Errorf("config.Settings.Theme = %q, want gruvbox", m.config.Settings.Theme)
	}

	// The theme should survive a config reload from disk.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if loaded.Settings.Theme != "gruvbox" {
		t.Errorf("reloaded theme = %q, want gruvbox", loaded.Settings.Theme)
	}
}
