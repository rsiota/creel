package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

// QueryEditor wraps a VimBuffer with SQL completion and syntax-highlighted view.
type QueryEditor struct {
	buf         VimBuffer
	width       int
	height      int
	viewYOffset int
	completion  completion
}

// NewQueryEditor creates a new SQL query editor with vim mode.
func NewQueryEditor() QueryEditor {
	return QueryEditor{
		buf: NewVimBuffer(VimBufferConfig{
			Prompt:            " ",
			InitialMode:       VimNormal,
			EnableVisualLine:  true,
			EnableFormatEqual: true,
			StaticCursor:      true,
		}),
	}
}

// Value returns the current SQL text.
func (e QueryEditor) Value() string {
	return e.buf.Value()
}

// SetValue replaces the editor contents. If the text actually changes it
// pushes an undo checkpoint so AI / :e / openTable / history drops are
// reversible with `u`.
func (e *QueryEditor) SetValue(s string) {
	e.buf.SetValue(s)
}

func (e *QueryEditor) setValueRaw(s string) {
	e.buf.setValueRaw(s)
}

// CapturingKeys reports whether the editor should swallow keys that the
// workspace would otherwise treat as global (q, :, ctrl+p, …): insert mode
// and an open `/` search prompt.
func (e QueryEditor) CapturingKeys() bool {
	return e.buf.CapturingKeys()
}

func (e QueryEditor) IsSearching() bool { return e.buf.IsSearching() }
func (e QueryEditor) IsVisual() bool    { return e.buf.IsVisual() }
func (e QueryEditor) VimMode() VimMode  { return e.buf.Mode() }
func (e QueryEditor) Yank() string      { return e.buf.Yank() }

// VimModeStr returns a human-readable mode name.
func (e QueryEditor) VimModeStr() string {
	return e.buf.ModeStr()
}

func (e *QueryEditor) Focus() tea.Cmd { return e.buf.Focus() }
func (e *QueryEditor) Blur()          { e.buf.Blur() }
func (e QueryEditor) Focused() bool   { return e.buf.Focused() }

func (e *QueryEditor) Reset() {
	e.buf.Reset()
}

// Update handles messages for the query editor.
func (e QueryEditor) Update(msg tea.Msg) (QueryEditor, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if e.completion.visible && e.buf.Mode() == VimInsert && !e.buf.IsSearching() && !e.buf.IsVisual() {
			return e.handleInsertCompletion(keyMsg)
		}
	}
	var cmd tea.Cmd
	e.buf, cmd = e.buf.Update(msg)
	e.syncViewOffset()
	if keyMsg, ok := msg.(tea.KeyMsg); ok && e.buf.Mode() == VimInsert && !e.buf.IsSearching() {
		if keyMsg.Type == tea.KeyRunes || keyMsg.String() == "backspace" {
			e.tryAutoTrigger()
		}
	}
	return e, cmd
}

func (e QueryEditor) handleInsertCompletion(msg tea.KeyMsg) (QueryEditor, tea.Cmd) {
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
	if msg.Type == tea.KeyRunes || msg.String() == "backspace" {
		e.buf, _ = e.buf.Update(msg)
		e.tryAutoTrigger()
		return e, nil
	}
	e.CancelCompletion()
	e.buf, _ = e.buf.Update(msg)
	return e, nil
}

func (e QueryEditor) snap() editorSnap {
	line, col := e.cursorLineCol()
	return editorSnap{value: e.Value(), line: line, col: col}
}

func (e *QueryEditor) pushUndo() {
	e.buf.pushUndo()
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
	qOff := lineColToByteOffset(query, line, col)
	if qOff > len(query) {
		qOff = len(query)
	}
	e.buf.moveToByteOffset(base + qOff)
	return true
}

// findQueryBaseOffset returns the byte offset in buf where query begins, or -1.
func findQueryBaseOffset(buf, query string) int {
	for _, s := range db.SplitStatements(buf) {
		trimmed := strings.TrimSpace(strings.TrimRight(s.Text, ";"))
		if trimmed != query && s.Text != query {
			continue
		}
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

// View renders the query editor with SQL syntax highlighting.
func (e *QueryEditor) View() string {
	e.syncViewOffset()
	return e.highlightedView()
}

// SetSize sets the dimensions of the editor.
func (e *QueryEditor) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.buf.SetWidth(width - 2)
	e.buf.SetHeight(height)
	e.syncViewOffset()
}

func (e *QueryEditor) CursorUp()   { e.buf.CursorUp() }
func (e *QueryEditor) CursorDown() { e.buf.CursorDown() }

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

	cursorLine, cursorCol := e.cursorLineCol()
	lines := strings.Split(value, "\n")
	offset := 0
	for i := 0; i < cursorLine && i < len(lines); i++ {
		offset += len([]rune(lines[i])) + 1
	}
	offset += cursorCol

	for _, s := range stmts {
		if offset >= s.Start && offset <= s.End+1 {
			return s.Text
		}
	}
	return stmts[len(stmts)-1].Text
}

// --- Completion ---

func (e *QueryEditor) SetCandidates(candidates []completionItem) {
	e.completion.allCandidates = candidates
}

func (e *QueryEditor) SetActiveSchema(schema string) {
	e.completion.activeSchema = schema
}

func (e QueryEditor) CompletionVisible() bool {
	return e.completion.visible
}

func (e *QueryEditor) StartCompletion() {
	partial, wordStart := e.wordBeforeCursor()
	scope := e.completionScope()
	e.completion.partial = partial
	e.completion.wordStart = wordStart
	e.completion.candidates = filterCandidates(scope.filter(e.completion.allCandidates), partial, scope.want)
	e.completion.selected = 0
	if len(e.completion.candidates) > 0 {
		e.completion.visible = true
	}
}

func (e *QueryEditor) tryAutoTrigger() {
	partial, wordStart := e.wordBeforeCursor()
	scope := e.completionScope()
	if len(partial) < minAutoTriggerChars && !scope.hasTrailingQualifier() {
		e.completion.visible = false
		return
	}
	e.completion.partial = partial
	e.completion.wordStart = wordStart
	e.completion.candidates = filterCandidates(scope.filter(e.completion.allCandidates), partial, scope.want)
	e.completion.selected = 0
	e.completion.visible = len(e.completion.candidates) > 0
}

func (e *QueryEditor) AcceptCompletion() {
	if !e.completion.visible || len(e.completion.candidates) == 0 {
		return
	}
	candidate := e.completion.candidates[e.completion.selected].text

	_, col := e.cursorLineCol()
	charsToDelete := col - e.completion.wordStart

	for i := 0; i < charsToDelete; i++ {
		e.buf.sendKey("left")
	}
	for i := 0; i < charsToDelete; i++ {
		e.buf.sendKey("delete")
	}
	e.buf.InsertString(candidate)
	e.syncViewOffset()

	e.completion.visible = false
}

func (e *QueryEditor) CancelCompletion() {
	e.completion.visible = false
}

func (e *QueryEditor) MoveCompletion(delta int) {
	e.completion.move(delta)
}

func (e *QueryEditor) RefilterCompletion() {
	partial, wordStart := e.wordBeforeCursor()
	scope := e.completionScope()
	e.completion.partial = partial
	e.completion.wordStart = wordStart
	e.completion.candidates = filterCandidates(scope.filter(e.completion.allCandidates), partial, scope.want)
	e.completion.selected = 0
	if len(e.completion.candidates) == 0 {
		e.completion.visible = false
	}
}

func (e QueryEditor) completionScope() sqlCompleteScope {
	all := e.completion.allCandidates
	return sqlCompleteScopeFromQuery(
		e.textBeforePartial(),
		e.StatementAtCursor(),
		knownTablesFrom(all),
		knownSchemasFrom(all),
		e.completion.activeSchema,
	)
}

func (e QueryEditor) textBeforePartial() string {
	text := e.buf.textBeforeCursor()
	partial, _ := e.wordBeforeCursor()
	if partial != "" && strings.HasSuffix(text, partial) {
		return strings.TrimSuffix(text, partial)
	}
	return text
}

func (e QueryEditor) CompletionView() string {
	return e.completion.renderCompletion()
}

func (e QueryEditor) CursorScreenPos() (line int, col int) {
	return e.cursorLineCol()
}

func (e QueryEditor) cursorLineCol() (int, int) {
	return e.buf.CursorLineCol()
}

func (e QueryEditor) wordBeforeCursor() (word string, startCol int) {
	return e.buf.wordBeforeCursor()
}

func (e QueryEditor) visualRange() (lo, hi int) {
	return e.buf.visualRange()
}

func (e QueryEditor) searchPrompt(width int) string {
	return e.buf.searchPrompt(width)
}
