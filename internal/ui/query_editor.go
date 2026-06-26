package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
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
	viewYOffset int

	vimMode VimMode
	pending vimPending
	yank    string

	completion completion
}

// NewQueryEditor creates a new SQL query editor with vim mode.
func NewQueryEditor() QueryEditor {
	ta := textarea.New()
	ta.Prompt = " "
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(5)

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorFg)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorPrimary)
	ta.BlurredStyle = ta.FocusedStyle
	ta.Cursor.SetMode(cursor.CursorStatic)

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
	e.syncViewOffset()
	return e, cmd
}

func (e QueryEditor) handleInsertMode(msg tea.KeyMsg) (QueryEditor, tea.Cmd) {
	// Completion popup key handling
	if e.completion.visible {
		switch msg.String() {
		case "esc":
			e.CancelCompletion()
			return e, nil
		case "tab":
			e.AcceptCompletion()
			return e, nil
		case "enter":
			e.AcceptCompletion()
			return e, nil
		case "up", "ctrl+p":
			e.MoveCompletion(-1)
			return e, nil
		case "down", "ctrl+n":
			e.MoveCompletion(1)
			return e, nil
		}
		// Printable characters: pass to textarea, then re-evaluate popup.
		if msg.Type == tea.KeyRunes || msg.String() == "backspace" {
			e.textarea, _ = e.textarea.Update(msg)
			e.tryAutoTrigger()
			return e, nil
		}
		// Any other key (space, punctuation, arrows): dismiss and pass through.
		e.CancelCompletion()
	}

	if msg.String() == "esc" {
		e.vimMode = VimNormal
		e.sendKey("left")
		return e, nil
	}

	var cmd tea.Cmd
	e.textarea, cmd = e.textarea.Update(msg)

	// Auto-trigger after typing a printable character.
	if msg.Type == tea.KeyRunes {
		e.tryAutoTrigger()
	}

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
	e.syncViewOffset()
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

// View renders the query editor with SQL syntax highlighting.
func (e *QueryEditor) View() string {
	e.syncViewOffset()
	return e.highlightedView()
}

// SetSize sets the dimensions of the editor.
func (e *QueryEditor) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.textarea.SetWidth(width - 2)
	e.textarea.SetHeight(height)
	e.syncViewOffset()
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

// StatementAtCursor returns the SQL statement under the cursor when the
// editor contains multiple statements, or the full buffer if there is only
// one (or none). Returns "" for an empty editor.
func (e QueryEditor) StatementAtCursor() string {
	value := e.Value()
	stmts := db.SplitStatements(value)
	if len(stmts) <= 1 {
		return strings.TrimSpace(value)
	}

	// Compute the cursor's global rune offset.
	cursorLine, cursorCol := e.cursorLineCol()
	lines := strings.Split(value, "\n")
	offset := 0
	for i := 0; i < cursorLine && i < len(lines); i++ {
		offset += len([]rune(lines[i])) + 1 // +1 for \n
	}
	offset += cursorCol

	// Find the statement containing the cursor offset.
	for _, s := range stmts {
		if offset >= s.Start && offset <= s.End+1 {
			return s.Text
		}
	}

	// Cursor is past the last statement's end — return the last one.
	return stmts[len(stmts)-1].Text
}

// --- Completion ---

// SetCandidates stores the full candidate list for auto-completion.
func (e *QueryEditor) SetCandidates(candidates []completionItem) {
	e.completion.allCandidates = candidates
}

// CompletionVisible returns whether the completion popup is shown.
func (e QueryEditor) CompletionVisible() bool {
	return e.completion.visible
}

// StartCompletion forces the popup open (manual trigger via Ctrl+N).
func (e *QueryEditor) StartCompletion() {
	partial, wordStart := e.wordBeforeCursor()
	e.completion.partial = partial
	e.completion.wordStart = wordStart
	e.completion.candidates = filterCandidates(e.completion.allCandidates, partial)
	e.completion.selected = 0
	if len(e.completion.candidates) > 0 {
		e.completion.visible = true
	}
}

// tryAutoTrigger shows the popup if the current word is long enough and has matches.
func (e *QueryEditor) tryAutoTrigger() {
	partial, wordStart := e.wordBeforeCursor()
	if len(partial) < minAutoTriggerChars {
		e.completion.visible = false
		return
	}
	e.completion.partial = partial
	e.completion.wordStart = wordStart
	e.completion.candidates = filterCandidates(e.completion.allCandidates, partial)
	e.completion.selected = 0
	e.completion.visible = len(e.completion.candidates) > 0
}

// AcceptCompletion replaces the partial word with the selected candidate.
func (e *QueryEditor) AcceptCompletion() {
	if !e.completion.visible || len(e.completion.candidates) == 0 {
		return
	}
	candidate := e.completion.candidates[e.completion.selected].text

	// Recompute cursor position (it may have moved since trigger).
	_, col := e.cursorLineCol()
	charsToDelete := col - e.completion.wordStart

	for i := 0; i < charsToDelete; i++ {
		e.sendKey("left")
	}
	for i := 0; i < charsToDelete; i++ {
		e.sendKey("delete")
	}
	e.textarea.InsertString(candidate)

	e.completion.visible = false
}

// CancelCompletion hides the popup without inserting anything.
func (e *QueryEditor) CancelCompletion() {
	e.completion.visible = false
}

// MoveCompletion adjusts the selection by delta.
func (e *QueryEditor) MoveCompletion(delta int) {
	e.completion.move(delta)
}

// RefilterCompletion re-extracts the partial word and re-filters candidates.
// If no candidates remain, the popup is hidden.
func (e *QueryEditor) RefilterCompletion() {
	partial, wordStart := e.wordBeforeCursor()
	e.completion.partial = partial
	e.completion.wordStart = wordStart
	e.completion.candidates = filterCandidates(e.completion.allCandidates, partial)
	e.completion.selected = 0
	if len(e.completion.candidates) == 0 {
		e.completion.visible = false
	}
}

// CompletionView renders the popup box.
func (e QueryEditor) CompletionView() string {
	return e.completion.renderCompletion()
}

// CursorScreenPos returns the cursor's logical line and column.
func (e QueryEditor) CursorScreenPos() (line int, col int) {
	return e.cursorLineCol()
}

// cursorLineCol returns the current logical line index and character column.
func (e QueryEditor) cursorLineCol() (int, int) {
	line := e.textarea.Line()
	li := e.textarea.LineInfo()
	col := li.StartColumn + li.ColumnOffset
	return line, col
}

// wordBeforeCursor returns the word being typed before the cursor and
// the column where it starts.
func (e QueryEditor) wordBeforeCursor() (word string, startCol int) {
	_, col := e.cursorLineCol()
	line := e.currentLineText()
	if col > len([]rune(line)) {
		col = len([]rune(line))
	}
	runes := []rune(line)
	start := col
	for start > 0 && isWordChar(runes[start-1]) {
		start--
	}
	return string(runes[start:col]), start
}
