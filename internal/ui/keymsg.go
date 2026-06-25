package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// synthesizeKeyMsg constructs a tea.KeyMsg whose String() matches the given
// dispatch token. This lets the command palette "replay" a keybinding by
// feeding a synthetic keypress through the normal dispatch, avoiding the
// need for action closures or a dispatch refactor.
func synthesizeKeyMsg(token string) (tea.KeyMsg, bool) {
	switch token {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}, true
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}, true
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}, true
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}, true
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}, true
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}, true
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}, true
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}, true
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}, true
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}, true
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}, true
	case "delete":
		return tea.KeyMsg{Type: tea.KeyDelete}, true
	}

	// ctrl+<single letter> — maps to KeyType 1–26 (ASCII control codes).
	if strings.HasPrefix(token, "ctrl+") && len(token) == 6 {
		ch := token[5]
		if ch >= 'a' && ch <= 'z' {
			return tea.KeyMsg{Type: tea.KeyType(ch - 'a' + 1)}, true
		}
	}

	// Printable characters: letters, digits, punctuation, space.
	if token != "" {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(token)}, true
	}

	return tea.KeyMsg{}, false
}
