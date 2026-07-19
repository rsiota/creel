package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/ai"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

func TestCtrlFToggleDispatch(t *testing.T) {
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	m := NewModel(&config.Config{})
	mm0, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm0.(Model)
	m.connection = conn
	m.state = stateWorkspace
	m.focus = FocusResults

	mm, _ := m.updateWorkspace(tea.KeyMsg{Type: tea.KeyCtrlF, Runes: []rune{'f'}, Alt: false})
	m = mm.(Model)
	t.Logf("after ctrl+f: assistant.IsVisible=%v focus=%v", m.assistant.IsVisible(), m.focus)
	if !m.assistant.IsVisible() {
		t.Error("ctrl+f did not show the assistant panel")
	}
	if m.focus != FocusAssistant {
		t.Errorf("focus = %v, want FocusAssistant", m.focus)
	}

	// The panel title was removed; assert the panel rendered by checking a
	// transcript marker appears after adding a message.
	m.assistant.AppendUser("hello")
	view := m.viewWorkspace()
	if !strings.Contains(stripANSI(view), "YOU:") {
		t.Errorf("rendered workspace does not show the assistant transcript")
	}
}

// TestQDoesNotQuitWhileAssistantFocused ensures a 'q' typed in the compose
// box (or pressed while browsing the panel) never quits the app — it must be
// routed to the panel instead. Guards against the global q-to-quit handler.
func TestQDoesNotQuitWhileAssistantFocused(t *testing.T) {
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	fresh := func() Model {
		m := NewModel(&config.Config{})
		mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
		m = mm.(Model)
		m.connection = conn
		m.state = stateWorkspace
		return m
	}

	qKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}

	for _, composing := range []bool{true, false} {
		m := fresh()
		m.assistant.Show()
		m.focus = FocusAssistant
		if composing {
			m.assistant.StartCompose()
		} else {
			m.assistant.CancelCompose()
		}

		mm, cmd := m.updateWorkspace(qKey)
		m = mm.(Model)
		if m.quitting {
			t.Errorf("composing=%v: 'q' quit the app while assistant focused", composing)
		}
		if cmd != nil {
			if _, ok := cmd().(tea.QuitMsg); ok {
				t.Errorf("composing=%v: 'q' returned a quit command", composing)
			}
		}
		// In compose mode the 'q' should land in the input; in browse mode it
		// should close the panel.
		if composing && m.assistant.InputValue() != "q" {
			t.Errorf("compose: 'q' not routed to input (got %q)", m.assistant.InputValue())
		}
		if !composing && m.assistant.IsVisible() {
			t.Errorf("browse: 'q' did not close the panel")
		}
	}
}

// TestEscLeavesComposeMode is a regression guard: the global workspace esc
// handler used to swallow esc with a catch-all return, so pressing esc while
// composing a question never reached the panel's compose handler — the user
// was stuck in insert mode. esc must leave compose (back to browse) and keep
// the panel open; a second esc (in browse) closes the panel.
func TestEscLeavesComposeMode(t *testing.T) {
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.connection = conn
	m.state = stateWorkspace
	m.assistant.Show()
	m.focus = FocusAssistant
	m.assistant.StartCompose()
	m.assistant.input.SetValue("draft question")
	if !m.assistant.IsComposing() {
		t.Fatal("setup: assistant should be in compose mode")
	}

	// First esc: leave compose mode, panel stays open, draft cleared.
	mm2, _ := m.updateWorkspace(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm2.(Model)
	if m.assistant.IsComposing() {
		t.Error("esc did not leave compose mode (still composing)")
	}
	if !m.assistant.IsVisible() {
		t.Error("esc in compose mode closed the panel (should only leave compose)")
	}
	if m.assistant.InputValue() != "" {
		t.Errorf("esc in compose mode did not clear the draft: %q", m.assistant.InputValue())
	}

	// Second esc (now in browse mode): closes the panel.
	mm3, cmd := m.updateWorkspace(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm3.(Model)
	if m.assistant.IsVisible() {
		t.Error("esc in browse mode did not close the panel")
	}
	if cmd == nil {
		t.Error("esc in browse mode should emit a close command")
	} else if _, ok := cmd().(closeAssistantMsg); !ok {
		t.Errorf("esc in browse mode emitted %T, want closeAssistantMsg", cmd())
	}
}

// TestTabCyclesThroughAssistant verifies that with the assistant open (and
// inspector closed), Tab reaches the assistant and Shift+Tab reaches it from
// the other direction — i.e. it isn't skipped over.
func TestTabCyclesThroughAssistant(t *testing.T) {
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	fresh := func() Model {
		m := NewModel(&config.Config{})
		mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
		m = mm.(Model)
		m.connection = conn
		m.state = stateWorkspace
		m.assistant.Show() // assistant open, inspector closed
		return m
	}

	// Tab forward from Results must reach Assistant (not skip it for the wrap).
	// Walk a full forward cycle from Results and collect the panels visited.
	visited := []Focus{}
	cur := fresh()
	cur.focus = FocusResults
	for i := 0; i < 7; i++ {
		cur = cur.cycleFocus()
		visited = append(visited, cur.focus)
	}
	// From Results, tabbing should hit Assistant before wrapping to Connections.
	foundAssistant := false
	for _, f := range visited[:5] { // first 5 tabs
		if f == FocusAssistant {
			foundAssistant = true
		}
	}
	if !foundAssistant {
		t.Errorf("forward tab never reached Assistant; visited %v", visited)
	}

	// Shift-tab backward from Connections must reach Assistant (last focusable).
	cur = fresh()
	cur.focus = FocusConnections
	cur = cur.cycleFocusBack()
	if cur.focus != FocusAssistant {
		t.Errorf("shift-tab from Connections = %v, want FocusAssistant", cur.focus)
	}
}

// stripANSI removes ANSI escape sequences so we can assert on visible text.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestModelPicker covers cursor placement, wrap-around navigation, unknown
// models, and that enter at the app level commits the selection to m.aiModel.
func TestModelPicker(t *testing.T) {
	p := NewModelPicker()
	p.Show("glm-4.5-air") // cursor on the reasoning model (index 1)
	if p.Selected() != "glm-4.5-air" {
		t.Fatalf("cursor not on shown model: %q", p.Selected())
	}
	p.Down()
	if p.Selected() != "glm-4.5" { // index 2
		t.Errorf("after Down: %q, want glm-4.5", p.Selected())
	}
	p.Up() // → 1
	p.Up() // → 0
	p.Up() // wrap → last
	if p.Selected() != aiModelOptions[len(aiModelOptions)-1].id {
		t.Errorf("wrap Up: %q, want %q", p.Selected(), aiModelOptions[len(aiModelOptions)-1].id)
	}
	p.Down() // wrap → first
	if p.Selected() != aiModelOptions[0].id {
		t.Errorf("wrap Down: %q, want %q", p.Selected(), aiModelOptions[0].id)
	}

	// Unknown model falls through to the first option.
	p2 := NewModelPicker()
	p2.Show("does-not-exist")
	if p2.Selected() != aiModelOptions[0].id {
		t.Errorf("unknown model cursor: %q", p2.Selected())
	}

	// App-level: enter commits the picker's selection to m.aiModel.
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.connection = conn
	m.state = stateWorkspace
	m.modelPicker.Show("")
	m.modelPicker.Down() // glm-4.5-air
	if m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); m3 != nil {
		m = m3.(Model)
	}
	if m.aiModel != "glm-4.5-air" {
		t.Errorf("after enter: aiModel = %q, want glm-4.5-air", m.aiModel)
	}
	if m.modelPicker.IsVisible() {
		t.Errorf("picker still visible after enter")
	}
}

// TestAIConfigModelOverride verifies the picker choice (m.aiModel) is preferred
// over the env-derived model, and that with no choice the env/default is used.
func TestAIConfigModelOverride(t *testing.T) {
	t.Setenv("GSQL_AI_API_KEY", "k")
	m := NewModel(&config.Config{})
	if got := m.aiConfig().Model; got != ai.DefaultModel {
		t.Errorf("default model = %q, want %q", got, ai.DefaultModel)
	}
	m.aiModel = "glm-4.5-air"
	if got := m.aiConfig().Model; got != "glm-4.5-air" {
		t.Errorf("overridden model = %q, want glm-4.5-air", got)
	}
	if m.effectiveAIModel() != "glm-4.5-air" {
		t.Errorf("effectiveAIModel = %q, want glm-4.5-air", m.effectiveAIModel())
	}
}

// TestAssistantDefaultsToBrowse verifies that focusing the assistant leaves it
// in browse mode (compose is transient), so `M` opens the model picker rather
// than typing into the compose box — the regression this guards against.
func TestAssistantDefaultsToBrowse(t *testing.T) {
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.connection = conn
	m.state = stateWorkspace
	m.assistant.Show()
	m.focus = FocusAssistant
	m.applyFocus()

	if m.assistant.IsComposing() {
		t.Fatalf("assistant should be in browse mode after focus, not compose")
	}

	// `M` from browse opens the model picker (returns an openModelPickerMsg).
	m2, cmd := m.updateWorkspace(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	m = m2.(Model)
	if cmd == nil {
		t.Fatal("M in browse returned no command (should open the picker)")
	}
	if m3, _ := m.Update(cmd()); m3 != nil { // run openModelPickerMsg
		m = m3.(Model)
	}
	if !m.modelPicker.IsVisible() {
		t.Errorf("model picker not visible after M")
	}
	if v := m.assistant.InputValue(); v != "" {
		t.Errorf("M leaked into compose input: %q", v)
	}
}

// TestAIStreamChunkFlow drives the streaming handlers offline: chunk msgs grow
// the panel's live preview and re-issue the drain, then the terminal
// aiResultMsg finalizes the turn and clears the stream buffer.
func TestAIStreamChunkFlow(t *testing.T) {
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	m := NewModel(&config.Config{})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = mm.(Model)
	m.connection = conn
	m.state = stateWorkspace
	m.assistant.Show()
	m.aiStream = make(chan tea.Msg, 4) // non-nil so the handler keeps draining

	// Two chunks grow the live preview.
	for _, d := range []string{"SELECT", " 1"} {
		mm, cmd := m.Update(aiStreamChunkMsg{content: d})
		m = mm.(Model)
		if cmd == nil {
			t.Errorf("chunk handler returned nil cmd (should keep draining)")
		}
	}
	if got := m.assistant.streamText; got != "SELECT 1" {
		t.Errorf("streamText after chunks = %q, want %q", got, "SELECT 1")
	}

	// Terminal result finalizes the turn and clears the preview.
	mm, _ = m.Update(aiResultMsg{reply: "SELECT 1", sql: "SELECT 1", toPanel: true})
	m = mm.(Model)
	if m.assistant.streamText != "" {
		t.Errorf("streamText not cleared after finalize = %q", m.assistant.streamText)
	}
	if m.aiStream != nil {
		t.Errorf("aiStream not cleared after finalize")
	}
	if !m.assistant.HasTurns() {
		t.Errorf("finalize did not append the assistant turn")
	}
	if got := m.assistant.LatestSQL(); got != "SELECT 1" {
		t.Errorf("LatestSQL = %q, want SELECT 1", got)
	}
}
