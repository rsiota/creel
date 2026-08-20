package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

// newConnListModel builds a model in the connections state with the given
// connections, sized as the app would on a real terminal.
func newConnListModel(t *testing.T, conns []config.ConnectionConfig, h int) Model {
	t.Helper()
	cfg := &config.Config{Connections: conns}
	m := NewModel(cfg)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: h})
	m = mm.(Model)
	m.loadConnections()
	// Size the real model the way updateLayout does on resize.
	cw, lh := m.connListContentDims()
	m.connList.SetSize(cw, lh)
	return m
}

func makeConns(n int) []config.ConnectionConfig {
	out := make([]config.ConnectionConfig, n)
	for i := range out {
		out[i] = config.ConnectionConfig{
			Name: fmt.Sprintf("conn%d", i), Driver: "sqlite",
			Database: fmt.Sprintf("/tmp/db%d.sqlite", i),
		}
	}
	return out
}

// Each connection renders as an inspector-style field box: a bordered value
// box with the name + driver badge on the label line and the detail inside.
func TestConnectionListRendersFieldBoxes(t *testing.T) {
	m := newConnListModel(t, []config.ConnectionConfig{
		{Name: "local", Driver: "sqlite", Database: "/tmp/app.db"},
		{Name: "staging", Driver: "mysql", Host: "10.0.0.5", Port: 3306, Database: "myapp"},
	}, 40)

	view := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(m.connList.View(), "")
	// Two entries → two field-box top borders.
	if got := strings.Count(view, "┌"); got != 2 {
		t.Errorf("field boxes=%d, want 2\n%s", got, view)
	}
	// Name + driver badge on the label line; detail inside the box.
	for _, want := range []string{"local", "[SQLITE]", "/tmp/app.db", "staging", "[MYSQL]", "10.0.0.5"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in view\n%s", want, view)
		}
	}
}

// The empty state shows a selectable demo invitation instead of a blank list.
func TestConnectionListEmptyState(t *testing.T) {
	m := newConnListModel(t, nil, 30)
	view := stripAnsiConn(m.connList.View())
	if !strings.Contains(view, "Try the demo database") {
		t.Errorf("demo invitation missing: %q", view)
	}
	if !m.connList.SelectedIsDemo() {
		t.Error("cursor should rest on the demo invitation")
	}
	if m.connList.SelectedName() != "" {
		t.Errorf("SelectedName should be empty for demo row, got %q", m.connList.SelectedName())
	}
}

func stripAnsiConn(s string) string {
	return regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(s, "")
}

// The footer shows a connection count when everything fits (no scroll range).
func TestConnectionListCountFooter(t *testing.T) {
	m := newConnListModel(t, makeConns(3), 40)
	if got := m.connList.ScrollInfo(); !strings.Contains(got, "3 connections") {
		t.Errorf("ScrollInfo=%q, want it to contain '3 connections'", got)
	}
}

// The popup grows as connections are added (until capped by the viewport).
func TestConnectionListPopupGrowsWithItems(t *testing.T) {
	m1 := newConnListModel(t, makeConns(1), 60)
	m4 := newConnListModel(t, makeConns(4), 60)
	_, h1 := m1.connListPopupDims()
	_, h4 := m4.connListPopupDims()
	if h4 <= h1 {
		t.Errorf("popup did not grow with items: 1-conn H=%d, 4-conn H=%d", h1, h4)
	}
	// Three extra connections should add exactly 3*linesPerField rows.
	if want := h1 + 3*linesPerField; h4 != want {
		t.Errorf("4-conn H=%d, want %d (1-conn %d + 3*%d)", h4, want, h1, linesPerField)
	}
}

// While filtering, the popup height must stay constant — sized to the total
// connection count, not the shrinking match set — so it doesn't jump as the
// user types. (The list area just shows fewer boxes with breathing room.)
func TestConnectionListPopupHeightConstantWhileFiltering(t *testing.T) {
	m := newConnListModel(t, makeConns(6), 60)
	_, h0 := m.connListPopupDims()

	m.connList.StartFilter()
	// makeConns names are "conn0".."conn5": "1" matches only "conn1".
	m.connList.FilterAddChar("1")
	if got := len(m.connList.visibleItems()); got != 1 {
		t.Fatalf("filter setup: visible=%d, want 1", got)
	}
	_, h1 := m.connListPopupDims()
	if h1 != h0 {
		t.Errorf("popup height changed while filtering: before=%d after=%d (want equal)", h0, h1)
	}
}

// While the cursor stays within the visible window, j/k must NOT scroll — the
// cards stay put and only the focused border moves. Scrolling kicks in only
// once the cursor reaches the bottom edge (like the inspector/form). This is a
// regression test for the value-receiver sizing bug that left connList.height
// at 0 and made every cursor move scroll aggressively.
func TestConnectionListWindowStableUntilEdge(t *testing.T) {
	m := newConnListModel(t, makeConns(6), 22) // short terminal → maxVisible < 6

	maxVisible := m.connList.maxVisibleItems()
	items := len(m.connList.visibleItems())
	if maxVisible >= items {
		t.Fatalf("test setup expects scrolling: maxVisible=%d items=%d", maxVisible, items)
	}

	// Move within the first window: scroll must not change.
	for i := 0; i < maxVisible-1; i++ {
		before := m.connList.scroll
		m.connList.MoveCursor(1)
		if m.connList.scroll != before {
			t.Fatalf("scrolled at cursor=%d (%d->%d): window must stay stable until the bottom edge",
				m.connList.cursor, before, m.connList.scroll)
		}
	}

	// One more move crosses the bottom edge → scroll must advance.
	m.connList.MoveCursor(1)
	if m.connList.scroll == 0 {
		t.Errorf("expected scroll>0 after crossing the bottom edge, got 0")
	}
}
