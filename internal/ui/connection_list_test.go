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

// Each connection renders as a quiet single line: name + muted host/path.
func TestConnectionListRendersQuietRows(t *testing.T) {
	m := newConnListModel(t, []config.ConnectionConfig{
		{Name: "local", Driver: "sqlite", Database: "/tmp/app.db"},
		{Name: "staging", Driver: "mysql", Host: "10.0.0.5", Port: 3306, Database: "myapp"},
	}, 40)

	view := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(m.connList.View(), "")
	if strings.Contains(view, "┌") {
		t.Errorf("connection rows should not use field-box chrome\n%s", view)
	}
	for _, want := range []string{"local", "/tmp/app.db", "staging", "10.0.0.5"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in view\n%s", want, view)
		}
	}
	for _, noise := range []string{"[SQLITE]", "[MYSQL]", "recent"} {
		if strings.Contains(view, noise) {
			t.Errorf("unexpected %q in quiet picker\n%s", noise, view)
		}
	}
}

func TestLoadConnectionsSSHDetailShowsBastionOnly(t *testing.T) {
	cfg := &config.Config{Connections: []config.ConnectionConfig{
		{Name: "tunnel", Driver: "mysql", Host: "127.0.0.1", Port: 3306, Database: "app", SSHHost: "bastion.example"},
	}}
	m := NewModel(cfg)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 40})
	m = mm.(Model)
	m.loadConnections()
	cw, lh := m.connListContentDims()
	m.connList.SetSize(cw, lh)

	view := stripAnsiConn(m.connList.View())
	if !strings.Contains(view, "bastion.example") {
		t.Errorf("expected SSH host in detail:\n%s", view)
	}
	if strings.Contains(view, "127.0.0.1") || strings.Contains(view, "via") {
		t.Errorf("SSH detail should be bastion only, not local host via tunnel:\n%s", view)
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

// The popup uses a fixed shell height (shared with the connection form and
// database picker), so adding connections does not grow the frame — the list
// scrolls instead.
func TestConnectionListPopupFixedShellHeight(t *testing.T) {
	m1 := newConnListModel(t, makeConns(1), 60)
	m8 := newConnListModel(t, makeConns(8), 60)
	_, h1 := m1.connListPopupDims()
	_, h8 := m8.connListPopupDims()
	if h1 != h8 {
		t.Errorf("popup height should be fixed: 1-conn H=%d, 8-conn H=%d", h1, h8)
	}
	_, wantOuter := popupOuterSize(60)
	if h1 != wantOuter {
		t.Errorf("popup H=%d, want shell outer %d", h1, wantOuter)
	}
	// Enough connections to overflow the fixed shell's list area.
	m40 := newConnListModel(t, makeConns(40), 60)
	_, h40 := m40.connListPopupDims()
	if h1 != h40 {
		t.Errorf("popup height should be fixed: 1-conn H=%d, 40-conn H=%d", h1, h40)
	}
	cw, lh := m40.connListContentDims()
	m40.connList.SetSize(cw, lh)
	if m40.connList.maxVisibleItems() >= 40 {
		t.Fatalf("expected scrolling for 40 conns: maxVisible=%d", m40.connList.maxVisibleItems())
	}
}

// Connection list, connection form, and database picker share one outer height.
func TestStartupPopupsShareShellHeight(t *testing.T) {
	const termH = 60
	m := newConnListModel(t, makeConns(2), termH)
	_, listH := m.connListPopupDims()

	iw, formContentH := popupContentSize(termH)
	m.connForm = NewConnectionForm()
	m.connForm.SetSize(iw, formContentH)
	formOuter := m.connForm.effectiveHeight() + 2

	dbW, dbH := popupOuterSize(termH)
	_ = dbW
	if listH != formOuter || listH != dbH {
		t.Errorf("shell heights differ: list=%d form=%d db=%d", listH, formOuter, dbH)
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
	m := newConnListModel(t, makeConns(40), 22) // short terminal → maxVisible < 40

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
