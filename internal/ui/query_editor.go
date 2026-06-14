package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// VimMode represents the current vim editing mode.
type VimMode int

const (
	VimNormal VimMode = iota
	VimInsert
)

// vimPending tracks pending operator state in normal mode (e.g. 'd' waiting for 'd').
type vimPending int

const (
	vimPendingNone vimPending = iota
	vimPendingD   // 'd' pressed, waiting for motion or 'd'
	vimPendingG   // 'g' pressed, waiting for 'g'
)

// QueryEditor wraps a textarea with vim-style modal editing.
type QueryEditor struct {
	textarea textarea.Model
	width    int
	height   int

	vimMode VimMode
	pending vimPending
	yank    string
}

// NewQueryEditor creates a new SQL query editor with vim mode.
func NewQueryEditor() QueryEditor {
	ta := textarea.New()
	ta.Placeholder = "SELECT * FROM ..."
	ta.Prompt = " "
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(5)

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorFg)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorPrimary)
	ta.BlurredStyle = ta.FocusedStyle

	return QueryEditor{
		textarea: ta,
		vimMode:  VimNormal,
	}
}

// Value returns the current SQL text.
func (e QueryEditor) Value() string {
	return e.textarea.Value()
}

// SetValue replaces the editor contents.
func (e *QueryEditor) SetValue(s string) {
	e.textarea.SetValue(s)
}

// VimMode returns the current vim mode.
func (e QueryEditor) VimMode() VimMode {
	return e.vimMode
}

// VimModeStr returns a human-readable mode name.
func (e QueryEditor) VimModeStr() string {
	if e.vimMode == VimNormal {
		return "NORMAL"
	}
	return "INSERT"
}

// Focus gives keyboard focus to the editor.
func (e *QueryEditor) Focus() tea.Cmd {
	return e.textarea.Focus()
}

// Blur removes keyboard focus from the editor.
func (e *QueryEditor) Blur() {
	e.textarea.Blur()
}

// Focused returns whether the editor currently has focus.
func (e QueryEditor) Focused() bool {
	return e.textarea.Focused()
}

// Reset clears the editor contents.
func (e *QueryEditor) Reset() {
	e.textarea.Reset()
}

// Update handles messages for the query editor.
func (e QueryEditor) Update(msg tea.Msg) (QueryEditor, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if e.vimMode == VimNormal {
			return e.handleNormalMode(keyMsg)
		}
		return e.handleInsertMode(keyMsg)
	}

	var cmd tea.Cmd
	e.textarea, cmd = e.textarea.Update(msg)
	return e, cmd
}

func (e QueryEditor) handleInsertMode(msg tea.KeyMsg) (QueryEditor, tea.Cmd) {
	if msg.String() == "esc" {
		e.vimMode = VimNormal
		e.sendKey("left")
		return e, nil
	}

	var cmd tea.Cmd
	e.textarea, cmd = e.textarea.Update(msg)
	return e, cmd
}

func (e QueryEditor) handleNormalMode(msg tea.KeyMsg) (QueryEditor, tea.Cmd) {
	key := msg.String()

	// Handle pending operator states
	if e.pending == vimPendingD {
		e.pending = vimPendingNone
		switch key {
		case "d":
			e.yank = e.currentLineText()
			e.sendKey("home")
			e.sendKey("ctrl+k")
			if e.textarea.LineCount() > 1 {
				e.sendKey("backspace")
			}
		case "w":
			e.sendKey("alt+d")
		}
		return e, nil
	}
	if e.pending == vimPendingG {
		e.pending = vimPendingNone
		if key == "g" {
			e.sendKey("ctrl+home")
		}
		return e, nil
	}

	switch key {
	// Movement
	case "h":
		e.sendKey("left")
	case "l":
		e.sendKey("right")
	case "j":
		e.sendKey("down")
	case "k":
		e.sendKey("up")
	case "0":
		e.sendKey("home")
	case "$":
		e.sendKey("end")
	case "w":
		e.sendKey("alt+right")
	case "b":
		e.sendKey("alt+left")
	case "G":
		e.sendKey("ctrl+end")
	case "g":
		e.pending = vimPendingG
	case "ctrl+d":
		for i := 0; i < 5; i++ {
			e.sendKey("down")
		}
	case "ctrl+u":
		for i := 0; i < 5; i++ {
			e.sendKey("up")
		}

	// Enter insert mode
	case "i":
		e.vimMode = VimInsert
	case "a":
		e.sendKey("right")
		e.vimMode = VimInsert
	case "A":
		e.sendKey("end")
		e.vimMode = VimInsert
	case "I":
		e.sendKey("home")
		e.vimMode = VimInsert
	case "o":
		e.sendKey("end")
		e.sendKey("enter")
		e.vimMode = VimInsert
	case "O":
		e.sendKey("home")
		e.sendKey("enter")
		e.sendKey("up")
		e.vimMode = VimInsert

	// Delete operations
	case "x":
		e.sendKey("delete")
	case "d":
		e.pending = vimPendingD
	case "D":
		e.yank = e.currentLineText()
		e.sendKey("ctrl+k")

	// Yank and paste
	case "y":
		e.yank = e.currentLineText()
	case "p":
		if e.yank != "" {
			e.sendKey("end")
			e.sendKey("enter")
			e.textarea.InsertString(e.yank)
		}

	// Delete character under cursor and enter insert mode
	case "c":
		e.sendKey("ctrl+k")
		e.vimMode = VimInsert
	case "C":
		e.sendKey("ctrl+k")
		e.vimMode = VimInsert

	// Misc
	case "enter":
		e.sendKey("down")
		e.sendKey("home")
	}

	return e, nil
}

// sendKey dispatches a synthetic key event to the textarea.
func (e *QueryEditor) sendKey(keyStr string) {
	msg := tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(keyStr),
	}
	// Map well-known key names to their proper type
	switch keyStr {
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "home":
		msg = tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		msg = tea.KeyMsg{Type: tea.KeyEnd}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	case "delete":
		msg = tea.KeyMsg{Type: tea.KeyDelete}
	case "alt+right":
		msg = tea.KeyMsg{Type: tea.KeyRight, Alt: true}
	case "alt+left":
		msg = tea.KeyMsg{Type: tea.KeyLeft, Alt: true}
	case "ctrl+home":
		msg = tea.KeyMsg{Type: tea.KeyCtrlHome}
	case "ctrl+end":
		msg = tea.KeyMsg{Type: tea.KeyCtrlEnd}
	case "ctrl+k":
		msg = tea.KeyMsg{Type: tea.KeyCtrlK}
	case "ctrl+d":
		msg = tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		msg = tea.KeyMsg{Type: tea.KeyCtrlU}
	case "alt+d":
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true}
	}

	e.textarea, _ = e.textarea.Update(msg)
}

func (e QueryEditor) currentLineText() string {
	lines := strings.Split(e.Value(), "\n")
	if len(lines) == 0 {
		return ""
	}
	idx := e.textarea.Line()
	if idx >= len(lines) {
		return ""
	}
	return lines[idx]
}

// View renders the query editor.
func (e QueryEditor) View() string {
	return e.textarea.View()
}

// SetSize sets the dimensions of the editor.
func (e *QueryEditor) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.textarea.SetWidth(width - 2)
	e.textarea.SetHeight(height)
}

// CursorUp moves the cursor up.
func (e *QueryEditor) CursorUp() {
	e.textarea.CursorUp()
}

// CursorDown moves the cursor down.
func (e *QueryEditor) CursorDown() {
	e.textarea.CursorDown()
}

// FormatQuery returns the query with surrounding whitespace trimmed.
func (e QueryEditor) FormatQuery() string {
	return strings.TrimSpace(e.Value())
}

// HelpText returns keybinding hints for the editor.
func (e QueryEditor) HelpText() string {
	if e.vimMode == VimNormal {
		return fmt.Sprintf("%s run  %s clear  %s/%s move  %s insert",
			mutedStyle.Render("Ctrl+J"),
			mutedStyle.Render("Ctrl+R"),
			mutedStyle.Render("h/j/k/l"),
			mutedStyle.Render("w/b"),
			mutedStyle.Render("i/a/o"),
		)
	}
	return fmt.Sprintf("%s run  %s clear  %s normal mode",
		mutedStyle.Render("Ctrl+J"),
		mutedStyle.Render("Ctrl+R"),
		mutedStyle.Render("Esc"),
	)
}
