package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// QueryEditor wraps a textarea for SQL input.
type QueryEditor struct {
	textarea textarea.Model
	width    int
	height   int
}

// NewQueryEditor creates a new SQL query editor.
func NewQueryEditor() QueryEditor {
	ta := textarea.New()
	ta.Placeholder = "SELECT * FROM ..."
	ta.Prompt = "│ "
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(5)

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorFg)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorPrimary)
	ta.BlurredStyle = ta.FocusedStyle

	return QueryEditor{textarea: ta}
}

// Value returns the current SQL text.
func (e QueryEditor) Value() string {
	return e.textarea.Value()
}

// SetValue replaces the editor contents.
func (e *QueryEditor) SetValue(s string) {
	e.textarea.SetValue(s)
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
	var cmd tea.Cmd
	e.textarea, cmd = e.textarea.Update(msg)
	return e, cmd
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

// FormatQuery returns the query with surrounding whitespace trimmed,
// collapsing newlines for clean statement execution.
func (e QueryEditor) FormatQuery() string {
	return strings.TrimSpace(e.Value())
}

// HelpText returns keybinding hints for the editor.
func (e QueryEditor) HelpText() string {
	return fmt.Sprintf("%s run query  %s clear  %s/%s navigate",
		mutedStyle.Render("Ctrl+J"),
		mutedStyle.Render("Ctrl+R"),
		mutedStyle.Render("↑"),
		mutedStyle.Render("↓"),
	)
}
