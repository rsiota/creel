package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/db"
)

// VimMode represents the current vim editing mode.
type VimMode int

const (
	VimNormal VimMode = iota
	VimInsert
)

const maxEditorUndo = 80

// vimPending tracks pending operator state in normal mode (e.g. 'd' waiting for 'd').
type vimPending int

const (
	vimPendingNone  vimPending = iota
	vimPendingD                // 'd' pressed, waiting for motion or 'd'
	vimPendingG                // 'g' pressed, waiting for 'g'
	vimPendingEqual            // '=' pressed, waiting for '='
)

// editorSnap is one undo/redo checkpoint: buffer text plus cursor.
type editorSnap struct {
	value string
	line  int
	col   int
}

// QueryEditor wraps a textarea with vim-style modal editing.
type QueryEditor struct {
	textarea    textarea.Model
	width       int
	height      int
	viewYOffset int

	vimMode VimMode
	pending vimPending
	yank    string

	completion completion

	undo []editorSnap
	redo []editorSnap
	// insertBase is the buffer from the moment insert mode was entered. On
	// esc, if the text changed, it becomes one undo unit (vim-style).
	insertBase *editorSnap

	searching    bool
	searchQuery  string
	searchFocus  string // last confirmed / query; n/N replay this
	searchOffset int    // last match rune offset; n/N step from here

	visualLine   bool
	visualAnchor int // logical line where V was pressed
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

// SetValue replaces the editor contents. If the text actually changes it
// pushes an undo checkpoint so AI / :e / openTable / history drops are
// reversible with `u`.
func (e *QueryEditor) SetValue(s string) {
	if e.Value() == s {
		return
	}
	e.pushUndo()
	e.setValueRaw(s)
}

func (e *QueryEditor) setValueRaw(s string) {
	e.textarea.SetValue(s)
	e.visualLine = false
}

// CapturingKeys reports whether the editor should swallow keys that the
// workspace would otherwise treat as global (q, :, ctrl+p, …): insert mode
// and an open `/` search prompt.
func (e QueryEditor) CapturingKeys() bool {
	return e.vimMode == VimInsert || e.searching || e.visualLine
}

// IsSearching reports whether the `/` prompt is open.
func (e QueryEditor) IsSearching() bool { return e.searching }

// IsVisual reports whether visual-line mode is active.
func (e QueryEditor) IsVisual() bool { return e.visualLine }

// VimMode returns the current vim mode.
func (e QueryEditor) VimMode() VimMode {
	return e.vimMode
}

// VimModeStr returns a human-readable mode name.
func (e QueryEditor) VimModeStr() string {
	switch {
	case e.searching:
		return "SEARCH"
	case e.visualLine:
		return "V-LINE"
	case e.vimMode == VimNormal:
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
	if e.Value() != "" {
		e.pushUndo()
	}
	e.textarea.Reset()
	e.visualLine = false
	e.searching = false
}

// Update handles messages for the query editor.
func (e QueryEditor) Update(msg tea.Msg) (QueryEditor, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if e.searching {
			return e.handleSearch(keyMsg)
		}
		if e.visualLine {
			return e.handleVisualLine(keyMsg)
		}
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
		e.commitInsertUndo()
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
			e.pushUndo()
			e.yank = e.currentLineText()
			e.sendKey("home")
			e.sendKey("ctrl+k")
			if e.textarea.LineCount() > 1 {
				e.sendKey("backspace")
			}
		case "w":
			e.pushUndo()
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
	if e.pending == vimPendingEqual {
		e.pending = vimPendingNone
		if key == "=" {
			e.pushUndo()
			formatted := formatSQL(e.textarea.Value())
			e.setValueRaw(formatted)
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
	case "=":
		e.pending = vimPendingEqual
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
		e.beginInsert()
	case "a":
		e.sendKey("right")
		e.beginInsert()
	case "A":
		e.sendKey("end")
		e.beginInsert()
	case "I":
		e.sendKey("home")
		e.beginInsert()
	case "o":
		e.beginInsert()
		e.sendKey("end")
		e.sendKey("enter")
	case "O":
		e.beginInsert()
		e.sendKey("home")
		e.sendKey("enter")
		e.sendKey("up")

	// Delete operations
	case "x":
		e.pushUndo()
		e.sendKey("delete")
	case "d":
		e.pending = vimPendingD
	case "D":
		e.pushUndo()
		e.yank = e.currentLineText()
		e.sendKey("ctrl+k")

	// Yank and paste
	case "y":
		e.yank = e.currentLineText()
	case "p":
		if e.yank != "" {
			e.pushUndo()
			e.sendKey("end")
			e.sendKey("enter")
			e.textarea.InsertString(e.yank)
		}

	// Delete character under cursor and enter insert mode
	case "c":
		e.beginInsert()
		e.sendKey("ctrl+k")
	case "C":
		e.beginInsert()
		e.sendKey("ctrl+k")

	// Undo / redo. U is redo so we don't steal global ctrl+r (refresh).
	case "u":
		e.undoOnce()
	case "U":
		e.redoOnce()

	// Buffer search
	case "/":
		e.searching = true
		e.searchQuery = ""
	case "n":
		e.jumpSearch(1)
	case "N":
		e.jumpSearch(-1)

	// Visual line
	case "V":
		e.visualLine = true
		e.visualAnchor = e.textarea.Line()

	// Misc
	case "enter":
		e.sendKey("down")
		e.sendKey("home")
	}

	return e, nil
}

func (e QueryEditor) snap() editorSnap {
	line, col := e.cursorLineCol()
	return editorSnap{value: e.Value(), line: line, col: col}
}

func (e *QueryEditor) pushSnap(s editorSnap) {
	e.undo = append(e.undo, s)
	if len(e.undo) > maxEditorUndo {
		e.undo = e.undo[len(e.undo)-maxEditorUndo:]
	}
	e.redo = nil
}

func (e *QueryEditor) pushUndo() {
	e.pushSnap(e.snap())
}

func (e *QueryEditor) restoreSnap(s editorSnap) {
	e.setValueRaw(s.value)
	e.restoreCursor(s.line, s.col)
}

func (e *QueryEditor) restoreCursor(line, col int) {
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}
	// Prefer the textarea's line/column API over synthetic keys: ctrl+home is
	// not always honoured, which left the caret at EOF after SetValue.
	for e.textarea.Line() > 0 {
		e.textarea.CursorUp()
	}
	for i := 0; i < line; i++ {
		if e.textarea.Line() >= e.textarea.LineCount()-1 {
			break
		}
		e.textarea.CursorDown()
	}
	e.textarea.SetCursor(col)
}

func (e *QueryEditor) beginInsert() {
	s := e.snap()
	e.insertBase = &s
	e.vimMode = VimInsert
}

func (e *QueryEditor) commitInsertUndo() {
	if e.insertBase == nil {
		return
	}
	if e.Value() != e.insertBase.value {
		e.pushSnap(*e.insertBase)
	}
	e.insertBase = nil
}

func (e *QueryEditor) undoOnce() {
	if len(e.undo) == 0 {
		return
	}
	cur := e.snap()
	last := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.redo = append(e.redo, cur)
	e.restoreSnap(last)
}

func (e *QueryEditor) redoOnce() {
	if len(e.redo) == 0 {
		return
	}
	cur := e.snap()
	last := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.undo = append(e.undo, cur)
	e.restoreSnap(last)
}

func (e QueryEditor) visualRange() (lo, hi int) {
	cur := e.textarea.Line()
	lo, hi = e.visualAnchor, cur
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func (e QueryEditor) handleSearch(msg tea.KeyMsg) (QueryEditor, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		e.searching = false
		e.searchQuery = ""
		return e, nil
	case "enter":
		e.searchFocus = e.searchQuery
		e.searching = false
		e.searchQuery = ""
		e.searchOffset = e.cursorOffset() - 1
		e.jumpSearch(1)
		return e, nil
	case "backspace":
		runes := []rune(e.searchQuery)
		if len(runes) > 0 {
			e.searchQuery = string(runes[:len(runes)-1])
		}
		return e, nil
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && msg.Runes[0] >= 0x20 {
		e.searchQuery += string(msg.Runes[0])
	}
	return e, nil
}

func (e QueryEditor) handleVisualLine(msg tea.KeyMsg) (QueryEditor, tea.Cmd) {
	key := msg.String()
	if e.pending == vimPendingG {
		e.pending = vimPendingNone
		if key == "g" {
			e.sendKey("ctrl+home")
		}
		return e, nil
	}
	switch key {
	case "esc", "ctrl+c", "v":
		e.visualLine = false
	case "j", "down":
		e.sendKey("down")
	case "k", "up":
		e.sendKey("up")
	case "G":
		e.sendKey("ctrl+end")
	case "g":
		e.pending = vimPendingG
	case "y":
		e.yankVisual()
		e.visualLine = false
	case "d", "x":
		e.deleteVisual()
		e.visualLine = false
	}
	return e, nil
}

func (e *QueryEditor) yankVisual() {
	lo, hi := e.visualRange()
	lines := strings.Split(e.Value(), "\n")
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	if lo > hi || len(lines) == 0 {
		return
	}
	e.yank = strings.Join(lines[lo:hi+1], "\n")
}

func (e *QueryEditor) deleteVisual() {
	e.yankVisual()
	e.pushUndo()
	lo, hi := e.visualRange()
	lines := strings.Split(e.Value(), "\n")
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	newLines := append(append([]string{}, lines[:lo]...), lines[hi+1:]...)
	e.setValueRaw(strings.Join(newLines, "\n"))
	e.restoreCursor(lo, 0)
}

func (e QueryEditor) cursorOffset() int {
	line, col := e.cursorLineCol()
	lines := strings.Split(e.Value(), "\n")
	off := 0
	for i := 0; i < line && i < len(lines); i++ {
		off += len([]rune(lines[i])) + 1
	}
	off += col
	return off
}

func (e *QueryEditor) jumpSearch(dir int) {
	q := e.searchFocus
	if q == "" {
		return
	}
	lower := strings.ToLower(e.Value())
	needle := strings.ToLower(q)
	if needle == "" {
		return
	}
	var starts []int
	for i := 0; ; {
		j := strings.Index(lower[i:], needle)
		if j < 0 {
			break
		}
		starts = append(starts, i+j)
		i += j + 1
		if i >= len(lower) {
			break
		}
	}
	if len(starts) == 0 {
		return
	}
	cur := e.searchOffset
	pick := starts[0]
	if dir >= 0 {
		for _, s := range starts {
			if s > cur {
				pick = s
				break
			}
		}
	} else {
		pick = starts[len(starts)-1]
		for i := len(starts) - 1; i >= 0; i-- {
			if starts[i] < cur {
				pick = starts[i]
				break
			}
		}
	}
	e.searchOffset = pick
	e.moveToByteOffset(pick)
}

func (e *QueryEditor) moveToByteOffset(off int) {
	val := e.Value()
	if off < 0 {
		off = 0
	}
	if off > len(val) {
		off = len(val)
	}
	prefix := val[:off]
	line := strings.Count(prefix, "\n")
	col := 0
	if i := strings.LastIndex(prefix, "\n"); i >= 0 {
		col = len([]rune(prefix[i+1:]))
	} else {
		col = len([]rune(prefix))
	}
	e.restoreCursor(line, col)
}

// JumpToQueryPos moves the cursor to line/col (0-based) within query, which
// must appear as a statement (or substring) in the editor buffer. Used to land
// on a driver-reported syntax error. Returns false if query cannot be found.
func (e *QueryEditor) JumpToQueryPos(query string, line, col int) bool {
	query = strings.TrimSpace(strings.TrimRight(query, ";"))
	if query == "" {
		return false
	}
	buf := e.Value()
	base := findQueryBaseOffset(buf, query)
	if base < 0 {
		return false
	}
	// Convert line/col within query to a byte offset within query, then into buf.
	qOff := lineColToByteOffset(query, line, col)
	if qOff > len(query) {
		qOff = len(query)
	}
	e.moveToByteOffset(base + qOff)
	return true
}

// findQueryBaseOffset returns the byte offset in buf where query begins, or -1.
func findQueryBaseOffset(buf, query string) int {
	for _, s := range db.SplitStatements(buf) {
		trimmed := strings.TrimSpace(strings.TrimRight(s.Text, ";"))
		if trimmed != query && s.Text != query {
			continue
		}
		// s.Start/End are rune indexes into buf; locate Text inside that span.
		runes := []rune(buf)
		if s.Start < 0 || s.Start >= len(runes) {
			break
		}
		end := s.End + 1
		if end > len(runes) {
			end = len(runes)
		}
		segment := string(runes[s.Start:end])
		rel := strings.Index(segment, s.Text)
		if rel < 0 {
			rel = strings.Index(segment, query)
		}
		if rel < 0 {
			continue
		}
		return len(string(runes[:s.Start])) + len(segment[:rel])
	}
	if i := strings.Index(buf, query); i >= 0 {
		return i
	}
	return -1
}

func lineColToByteOffset(s string, line, col int) int {
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}
	off := 0
	for i := 0; i < line; i++ {
		n := strings.IndexByte(s[off:], '\n')
		if n < 0 {
			return len(s)
		}
		off += n + 1
	}
	rest := s[off:]
	runes := []rune(rest)
	if col > len(runes) {
		col = len(runes)
	}
	return off + len(string(runes[:col]))
}

func (e QueryEditor) searchPrompt(width int) string {
	q := e.searchQuery
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Render("/"))
	if q == "" {
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" "))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Render(q))
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" "))
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
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
	e.completion.candidates = filterCandidates(e.contextualCandidates(), partial)
	e.completion.selected = 0
	if len(e.completion.candidates) > 0 {
		e.completion.visible = true
	}
}

// tryAutoTrigger shows the popup if the current word is long enough and has matches.
func (e *QueryEditor) tryAutoTrigger() {
	partial, wordStart := e.wordBeforeCursor()
	scope := sqlCompleteScopeFrom(e.textBeforePartial(), knownTablesFrom(e.completion.allCandidates))
	if len(partial) < minAutoTriggerChars && scope.qualifier == "" {
		e.completion.visible = false
		return
	}
	e.completion.partial = partial
	e.completion.wordStart = wordStart
	e.completion.candidates = filterCandidates(scope.filter(e.completion.allCandidates), partial)
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
	e.completion.candidates = filterCandidates(e.contextualCandidates(), partial)
	e.completion.selected = 0
	if len(e.completion.candidates) == 0 {
		e.completion.visible = false
	}
}

// contextualCandidates restricts the catalog to tables or columns based on the
// SQL to the left of the token being typed (FROM/JOIN → tables, WHERE/ON →
// columns of those tables).
func (e QueryEditor) contextualCandidates() []completionItem {
	scope := sqlCompleteScopeFrom(e.textBeforePartial(), knownTablesFrom(e.completion.allCandidates))
	return scope.filter(e.completion.allCandidates)
}

func (e QueryEditor) textBeforeCursor() string {
	value := e.Value()
	cursorLine, cursorCol := e.cursorLineCol()
	lines := strings.Split(value, "\n")
	var b strings.Builder
	for i := 0; i < cursorLine && i < len(lines); i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}
	if cursorLine >= 0 && cursorLine < len(lines) {
		runes := []rune(lines[cursorLine])
		if cursorCol > len(runes) {
			cursorCol = len(runes)
		}
		b.WriteString(string(runes[:cursorCol]))
	}
	return b.String()
}

func (e QueryEditor) textBeforePartial() string {
	text := e.textBeforeCursor()
	partial, _ := e.wordBeforeCursor()
	if partial != "" && strings.HasSuffix(text, partial) {
		return strings.TrimSuffix(text, partial)
	}
	return text
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
