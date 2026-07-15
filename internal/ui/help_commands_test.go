package ui

import (
	"strings"
	"testing"
)

// TestHelpListsExCommands pins that the "?" overlay folds the ":" command
// registry into its output (title + representative usages), so the help sheet
// is the single place to discover both keybindings and commands.
func TestHelpListsExCommands(t *testing.T) {
	h := NewHelpPanel()
	h.Show()
	h.SetSize(140, 50)
	out := stripAnsi(h.View())
	if !strings.Contains(out, "Commands") {
		t.Error("help overlay should include a Commands section")
	}
	for _, want := range []string{":goto <table>", ":export", ":refs", ":begin"} {
		if !strings.Contains(out, want) {
			t.Errorf("help overlay should list %q", want)
		}
	}
}

// TestCommandsBlockHeight is the layout-reservation arithmetic: two columns,
// so height is 1 (title) + ceil(n/2).
func TestCommandsBlockHeight(t *testing.T) {
	cases := []struct{ n, want int }{
		{0, 0},
		{1, 2},
		{2, 2},
		{3, 3},
		{13, 8},
	}
	for _, c := range cases {
		if got := commandsBlockHeight(c.n); got != c.want {
			t.Errorf("commandsBlockHeight(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}
