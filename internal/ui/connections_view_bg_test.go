package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rsiota/creel/internal/config"
)

func TestViewConnectionsPaintsEveryLine(t *testing.T) {
	applyPalette(themes["git-hub-light-default"])
	cfg := &config.Config{Connections: []config.ConnectionConfig{
		{Name: "local", Driver: "sqlite", Database: "/tmp/a.db"},
		{Name: "staging", Driver: "mysql", Host: "10.0.0.5", Port: 3306, Database: "app"},
		{Name: "prod", Driver: "postgres", Host: "10.0.0.6", Port: 5432, Database: "app"},
		{Name: "dev", Driver: "sqlite", Database: "/tmp/b.db"},
		{Name: "qa", Driver: "sqlite", Database: "/tmp/c.db"},
	}}
	m := NewModel(cfg)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateConnections
	m.loadConnections()

	assertFullWidthPainted := func(t *testing.T, painted string, tw int) {
		t.Helper()
		bg := ansiBgSeq(colorBg)
		if bg == "" {
			t.Fatalf("theme bg %s yields empty ansi seq", colorBg)
		}
		lines := strings.Split(painted, "\n")
		for i, line := range lines {
			w := lipgloss.Width(line)
			if w != tw {
				t.Errorf("line %d width=%d want terminal %d stripped=%q", i, w, tw, ansi.Strip(line))
			}
			if ansi.Strip(line) == "" {
				t.Errorf("line %d has no printable content", i)
			}
		}
	}

	raw := m.buildView()
	assertFullWidthPainted(t, m.paintBg(raw), 120)

	m.connList.MoveCursor(1)
	m.connList.MoveCursor(1)
	m.connList.MoveCursor(1)
	assertFullWidthPainted(t, m.paintBg(m.buildView()), 120)
}

func TestViewConnectionsTransparentSkipsExplicitBg(t *testing.T) {
	applyPalette(themes["git-hub-light-default"])
	cfg := &config.Config{Connections: []config.ConnectionConfig{
		{Name: "local", Driver: "sqlite", Database: "/tmp/a.db"},
	}}
	m := NewModel(cfg)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	m.state = stateConnections
	m.settings.TransparentBackground = true
	m.loadConnections()

	bg := ansiBgSeq(colorBg)
	if bg == "" {
		t.Fatal("theme bg seq empty")
	}
	raw := m.buildView()
	if strings.Contains(raw, bg) {
		t.Errorf("transparent connection picker should not embed theme bg %q", colorBg)
	}
}
