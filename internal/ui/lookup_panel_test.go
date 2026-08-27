package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestLookupPanelCursorHighlight(t *testing.T) {
	var p LookupPanel
	p.SetSize(60, 12)
	p.Show("Table sizes", db.Result{
		Columns: []db.Column{{Name: "Table"}, {Name: "Rows"}, {Name: "Disk"}},
		Rows: [][]string{
			{"events", "100", "4.0KB"},
			{"users", "3", "4.0KB"},
		},
	}, []string{"events", "users"})

	view := p.View()
	sel := sgrPrefix(lipgloss.NewStyle().Background(colorPrimary).Foreground(colorBg))
	if !strings.Contains(view, sel) {
		t.Fatalf("selected row missing highlight %q\nview:\n%s", sel, view)
	}
	if !strings.Contains(view, "events") {
		t.Fatalf("expected events in view:\n%s", view)
	}
}

func TestLookupPanelCursorSkipsHeader(t *testing.T) {
	var p LookupPanel
	p.SetSize(60, 12)
	p.Show("Tables", db.Result{
		Columns: []db.Column{{Name: "Name"}},
		Rows:    [][]string{{"users"}, {"orders"}},
	}, []string{"users", "orders"})

	if p.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0 (first data row)", p.cursor)
	}
	if got := p.SelectedJump(); got != "users" {
		t.Fatalf("SelectedJump = %q, want users", got)
	}

	p = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if p.cursor != 1 {
		t.Fatalf("after j, cursor = %d, want 1", p.cursor)
	}
	if got := p.SelectedJump(); got != "orders" {
		t.Fatalf("SelectedJump = %q, want orders", got)
	}

	p = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if p.cursor != 0 {
		t.Fatalf("after k, cursor = %d, want 0", p.cursor)
	}
}

func TestLookupPanelEnterOpensTable(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Exec(`CREATE TABLE events (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	m := NewModel(&config.Config{})
	m.connection = conn
	m.tables = []string{"events", "users"}
	m.state = stateWorkspace
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	cmd := m.exSizes()
	if cmd == nil {
		t.Fatal("exSizes returned nil")
	}
	msg := cmd().(lookupResultMsg)
	if msg.err != nil {
		t.Fatalf("sizes: %v", msg.err)
	}
	updated, _ := m.Update(msg)
	m = updated.(Model)
	if !m.lookupPanel.IsVisible() {
		t.Fatal("lookup panel should be open")
	}

	// Move to second row if needed, then Enter.
	m.lookupPanel = m.lookupPanel.Update(tea.KeyMsg{Type: tea.KeyDown})
	jump := m.lookupPanel.SelectedJump()
	if jump == "" {
		t.Fatal("expected a jump target")
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.lookupPanel.IsVisible() {
		t.Fatal("lookup panel should close on Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should return openTable command")
	}
	// openTable sets the editor then executeQuery; editor should name the table.
	q := m.editor.Value()
	if !strings.Contains(strings.ToLower(q), strings.ToLower(jump)) {
		t.Fatalf("editor query = %q, want SELECT involving %s", q, jump)
	}
}

func TestLookupPanelEnterNoJumpIsNoop(t *testing.T) {
	m := NewModel(&config.Config{})
	m.state = stateWorkspace
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.lookupPanel.Show("Peek", db.Result{
		Columns: []db.Column{{Name: "Field"}, {Name: "Value"}},
		Rows:    [][]string{{"rows", "3"}},
	}, nil)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !m.lookupPanel.IsVisible() {
		t.Fatal("panel should stay open when row is not jumpable")
	}
	if cmd != nil {
		t.Fatal("Enter with no jump should not run a command")
	}
}

func TestExSizesIncludesJumps(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Exec(`CREATE TABLE events (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	m := &Model{connection: conn}
	msg := m.exSizes()().(lookupResultMsg)
	if len(msg.jumps) != len(msg.result.Rows) {
		t.Fatalf("jumps=%d rows=%d", len(msg.jumps), len(msg.result.Rows))
	}
	if msg.jumps[0] != msg.result.Rows[0][0] {
		t.Fatalf("jump %q != table %q", msg.jumps[0], msg.result.Rows[0][0])
	}
}
