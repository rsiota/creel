package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
