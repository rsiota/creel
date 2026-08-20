package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

func TestChartPanelAllowsExCommand(t *testing.T) {
	m := NewModel(&config.Config{})
	m.state = stateWorkspace
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	m.results.SetResult([]string{"user", "hits"}, [][]string{{"a", "1"}}, "")
	_ = m.exBar([]string{"user", "hits", "sum"}, false)
	if !m.chartPanel.IsVisible() {
		t.Fatal("expected chart open")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = mm.(Model)
	if !m.ex.IsVisible() {
		t.Fatal(": should open the ex line while the chart is open")
	}
	if !m.chartPanel.IsVisible() {
		t.Fatal("chart should stay open under the ex line")
	}

	// Typing into the ex line must not be swallowed by the chart.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	m = mm.(Model)
	if m.ex.input != "w" {
		t.Fatalf("ex input=%q, want w (chart swallowed the key)", m.ex.input)
	}
}
