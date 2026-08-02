package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/config"
)

func TestHelpPanelRender(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(120, 40)
	out := stripAnsi(h.View())
	if out == "" {
		t.Fatal("help panel rendered empty")
	}
	// First viewport shows the header, the tab bar, and the top section.
	for _, want := range []string{"Keybindings", "Global", "Keys", "Commands"} {
		if !strings.Contains(out, want) {
			t.Errorf("help panel missing %q", want)
		}
	}
	// Scrolling actually moves the viewport: the top section scrolls off.
	for i := 0; i < 3; i++ {
		h.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	if strings.Contains(stripAnsi(h.View()), "Global") {
		t.Error("Global should have scrolled off the top after paging down")
	}
}

func TestHelpPanelToggle(t *testing.T) {
	h := NewHelpPanel()
	if h.IsVisible() {
		t.Fatal("help panel should start hidden")
	}
	h.Show()
	if !h.IsVisible() {
		t.Fatal("help panel should be visible after Show")
	}
	h.Hide()
	if h.IsVisible() {
		t.Fatal("help panel should be hidden after Hide")
	}
	h.Toggle()
	if !h.IsVisible() {
		t.Fatal("help panel should be visible after Toggle")
	}
}

func TestStatusBarRender(t *testing.T) {
	m := NewModel(&config.Config{})
	m.width = 120
	m.height = 40
	out := m.statusBar("test-conn")
	if out == "" {
		t.Fatal("status bar empty")
	}
	if !strings.Contains(out, "?") {
		t.Error("status bar should contain help hint")
	}
}

// Descriptions must align in a single display column even when a key display
// contains multi-byte glyphs (e.g. "↑/↓"). The padding used to be computed
// with len() (bytes), so arrow rows drifted left of the rest.
func TestHelpPanelDescriptionAlignmentWithArrowKeys(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	// Tall terminal so every section lands in a single column (the column
	// balancer splits by height), keeping each binding on its own line.
	h.SetSize(220, 500)
	lines := strings.Split(stripAnsi(h.View()), "\n")

	// prefixWidth is the display width of everything before `desc` on its row.
	prefixWidth := func(desc string) int {
		for _, ln := range lines {
			if idx := strings.Index(ln, desc); idx >= 0 {
				return runeLen(ln[:idx])
			}
		}
		t.Fatalf("description %q not found in help view", desc)
		return -1
	}

	// ASCII key ("t" → "new tab") vs arrow key ("↑/↓" → "navigate completions"):
	// both descriptions must start at the same display column.
	asciiW := prefixWidth("new tab")
	arrowW := prefixWidth("navigate completions")
	if asciiW != arrowW {
		t.Errorf("descriptions misaligned: ascii-key col=%d arrow-key col=%d",
			asciiW, arrowW)
	}
}
