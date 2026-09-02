package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/recent"
)

func TestLoadConnectionsSelectsRecent(t *testing.T) {
	dir := t.TempDir()
	store := recent.NewStore(dir)
	if err := store.Touch("alpha"); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch("beta"); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Connections: []config.ConnectionConfig{
		{Name: "alpha", Driver: "sqlite", Database: "/tmp/a.db"},
		{Name: "beta", Driver: "sqlite", Database: "/tmp/b.db"},
		{Name: "gamma", Driver: "sqlite", Database: "/tmp/c.db"},
	}}
	m := NewModel(cfg)
	m.recentStore = store
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 40})
	m = mm.(Model)
	m.loadConnections()
	cw, lh := m.connListContentDims()
	m.connList.SetSize(cw, lh)

	if got := m.connList.SelectedName(); got != "beta" {
		t.Fatalf("SelectedName=%q, want beta (most recent)", got)
	}
	view := stripAnsiConn(m.connList.View())
	if strings.Contains(view, "recent") {
		t.Errorf("picker should not show a recent badge:\n%s", view)
	}
	if !strings.Contains(view, "beta") {
		t.Errorf("expected selected connection name in view:\n%s", view)
	}
}

func TestOpenDemoDatabaseFromEmptyList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	m := NewModel(&config.Config{})
	m.recentStore = recent.NewStore(filepath.Join(dir, "creel"))
	m.loadConnections()
	if !m.connList.SelectedIsDemo() {
		t.Fatal("expected demo invitation on empty list")
	}

	cmd := (&m).connectToDB()
	if m.connError != "" {
		t.Fatalf("connect error: %s", m.connError)
	}
	if m.state != stateWorkspace {
		t.Fatalf("state=%v, want workspace", m.state)
	}
	if m.connection == nil {
		t.Fatal("expected open connection")
	}
	// Ad-hoc demo uses the file basename as the connection name.
	if got := m.connection.Config().Name; got != "creel-demo.db" {
		t.Fatalf("connection name=%q, want creel-demo.db", got)
	}
	if cmd != nil {
		_ = cmd // focus / prefetch / keep-alive; not needed for the assert
	}
}
