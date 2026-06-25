package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPaletteOpenClose(t *testing.T) {
	var p palette
	if p.IsVisible() {
		t.Fatal("palette should start hidden")
	}
	p.Open()
	if !p.IsVisible() {
		t.Fatal("palette should be visible after Open")
	}
	if p.input != "" {
		t.Fatal("input should be empty on open")
	}
	if len(p.items) == 0 {
		t.Fatal("palette should have items built from registry")
	}
	if len(p.filtered) != len(p.items) {
		t.Fatalf("filtered should equal items on open (%d vs %d)", len(p.filtered), len(p.items))
	}
	p.Hide()
	if p.IsVisible() {
		t.Fatal("palette should be hidden after Hide")
	}
}

func TestPaletteFuzzyFilter(t *testing.T) {
	var p palette
	p.Open()

	// Type "export" — should match export-related bindings.
	simulateTyping(&p, "export")
	if len(p.filtered) == 0 {
		t.Fatal("expected matches for 'export'")
	}
	found := false
	for _, it := range p.filtered {
		if strings.Contains(strings.ToLower(it.desc), "export") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no filtered item mentions 'export'")
	}

	// Nonsense filter should produce no matches.
	p.input = ""
	p.refilter()
	simulateTyping(&p, "zzzzz")
	if len(p.filtered) != 0 {
		t.Fatalf("expected no matches for nonsense, got %d", len(p.filtered))
	}
}

func TestPaletteNavigation(t *testing.T) {
	var p palette
	p.Open()
	initial := p.cursor

	p = updatePalette(t, p, tea.KeyMsg{Type: tea.KeyDown})
	if p.cursor != initial+1 {
		t.Fatalf("cursor should be %d after down, got %d", initial+1, p.cursor)
	}

	p = updatePalette(t, p, tea.KeyMsg{Type: tea.KeyUp})
	if p.cursor != initial {
		t.Fatalf("cursor should be %d after up, got %d", initial, p.cursor)
	}

	// Wrap from top to bottom.
	p.cursor = 0
	p = updatePalette(t, p, tea.KeyMsg{Type: tea.KeyUp})
	if p.cursor != len(p.filtered)-1 {
		t.Fatalf("cursor should wrap to %d, got %d", len(p.filtered)-1, p.cursor)
	}
}

func TestPaletteEscape(t *testing.T) {
	var p palette
	p.Open()
	simulateTyping(&p, "hello")
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.IsVisible() {
		t.Fatal("palette should be hidden after esc")
	}
	if cmd != nil {
		t.Fatal("cmd should be nil on esc")
	}
}

func TestPaletteEnterExecutableSingleKey(t *testing.T) {
	var p palette
	p.Open()

	// Find a single-token binding (e.g. "ctrl+r" — clear editor).
	target := -1
	for i, it := range p.items {
		if it.token != "" && it.token == "ctrl+r" {
			target = i
			break
		}
	}
	if target == -1 {
		t.Fatal("could not find ctrl+r binding in palette items")
	}

	// Navigate to it.
	p.cursor = target
	token := p.selectedToken()
	if token != "ctrl+r" {
		t.Fatalf("expected token 'ctrl+r', got %q", token)
	}

	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.IsVisible() {
		t.Fatal("palette should be hidden after enter")
	}
	if cmd == nil {
		t.Fatal("cmd should be non-nil to replay the key")
	}
}

func TestPaletteEnterNonExecutable(t *testing.T) {
	var p palette
	p.Open()

	// Find a multi-token binding (not executable).
	target := -1
	for i, it := range p.items {
		if it.token == "" {
			target = i
			break
		}
	}
	if target == -1 {
		t.Fatal("could not find a non-executable binding")
	}

	p.cursor = target
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.IsVisible() {
		t.Fatal("palette should be hidden after enter")
	}
	if cmd != nil {
		t.Fatal("cmd should be nil for non-executable binding")
	}
}

func TestPaletteView(t *testing.T) {
	var p palette
	p.Open()
	out := p.View(120)
	if out == "" {
		t.Fatal("palette view should not be empty when visible")
	}
	if !strings.Contains(out, "❯") {
		t.Error("palette view should contain prompt")
	}

	p.Hide()
	if p.View(120) != "" {
		t.Fatal("palette view should be empty when hidden")
	}
}

func TestPaletteDoublePressNotExecutable(t *testing.T) {
	// The "dd" and "y y" chords use a pending-boolean mechanism — a single
	// keypress won't trigger them, so they must not be marked executable.
	var p palette
	p.Open()
	for _, it := range p.items {
		if it.display == "dd" && it.token != "" {
			t.Errorf("dd binding should not be executable, got token %q", it.token)
		}
		if it.display == "y y" && it.token != "" {
			t.Errorf("y y binding should not be executable, got token %q", it.token)
		}
	}
}

func TestSynthesizeKeyMsg(t *testing.T) {
	tests := []struct {
		token string
		want  string
	}{
		{"j", "j"},
		{"X", "X"},
		{"?", "?"},
		{"enter", "enter"},
		{"esc", "esc"},
		{"tab", "tab"},
		{"ctrl+r", "ctrl+r"},
		{"ctrl+e", "ctrl+e"},
		{"ctrl+c", "ctrl+c"},
		{"/", "/"},
		{"\\", "\\"},
		{"!", "!"},
		{"*", "*"},
	}
	for _, tt := range tests {
		kmsg, ok := synthesizeKeyMsg(tt.token)
		if !ok {
			t.Errorf("synthesizeKeyMsg(%q) returned ok=false", tt.token)
			continue
		}
		if got := kmsg.String(); got != tt.want {
			t.Errorf("synthesizeKeyMsg(%q).String() = %q, want %q", tt.token, got, tt.want)
		}
	}

	// Empty token should fail.
	if _, ok := synthesizeKeyMsg(""); ok {
		t.Error(`synthesizeKeyMsg("") should return ok=false`)
	}
}

// simulateTyping feeds printable characters into the palette via Update.
func simulateTyping(p *palette, s string) {
	for _, ch := range s {
		next, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		*p = next
	}
}

// updatePalette is a test helper that asserts Update succeeds and returns
// the updated palette.
func updatePalette(t *testing.T, p palette, msg tea.KeyMsg) palette {
	t.Helper()
	next, _ := p.Update(msg)
	return next
}
