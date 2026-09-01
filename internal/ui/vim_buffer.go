package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
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

// VimBufferConfig tunes optional vim features for a textarea buffer.
type VimBufferConfig struct {
	Prompt            string
	InitialMode       VimMode
	EnableVisualLine  bool
	EnableFormatEqual bool // g=g motion only when false; = = waits for = to format SQL
	StaticCursor      bool
}

// VimBuffer is a bubbles textarea with vim normal/insert editing, search, and
// undo. Shared by the SQL query editor and the cell-edit popup.
type VimBuffer struct {
	textarea textarea.Model
	cfg      VimBufferConfig

	mode         VimMode
	pending      vimPending
	yank         string
	undo         []editorSnap
	redo         []editorSnap
	insertBase   *editorSnap
	searching    bool
	searchQuery  string
	searchFocus  string
	searchOffset int
	visualLine   bool
	visualAnchor int

	onChange func()
}

// NewVimBuffer builds a vim-capable textarea with the given options.
func NewVimBuffer(cfg VimBufferConfig) VimBuffer {
	ta := textarea.New()
	ta.Prompt = cfg.Prompt
	ta.ShowLineNumbers = false
	ta.CharLimit = 0

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(colorFg)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorFg)
	if cfg.Prompt != "" {
		ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorPrimary)
	}
	ta.BlurredStyle = ta.FocusedStyle
	if cfg.StaticCursor {
		ta.Cursor.SetMode(cursor.CursorStatic)
	}

	return VimBuffer{
		textarea: ta,
		cfg:      cfg,
		mode:     cfg.InitialMode,
	}
}

// SetOnChange registers a callback after buffer mutations (e.g. sync scroll).
func (b *VimBuffer) SetOnChange(fn func()) {
	b.onChange = fn
}

func (b *VimBuffer) notifyChange() {
	if b.onChange != nil {
		b.onChange()
	}
}

// Value returns the buffer text.
func (b VimBuffer) Value() string {
	return b.textarea.Value()
}

// SetValue replaces the buffer, pushing an undo checkpoint when the text changes.
func (b *VimBuffer) SetValue(s string) {
	if b.Value() == s {
		return
	}
	b.pushUndo()
	b.setValueRaw(s)
}

func (b *VimBuffer) setValueRaw(s string) {
	b.textarea.SetValue(s)
	b.visualLine = false
}

// SetMode switches vim mode (used by tests and reopen hooks).
func (b *VimBuffer) SetMode(m VimMode) {
	b.mode = m
}

// Reset clears the buffer.
func (b *VimBuffer) Reset() {
	if b.Value() != "" {
		b.pushUndo()
	}
	b.textarea.Reset()
	b.visualLine = false
	b.searching = false
}

// CapturingKeys reports whether global workspace keys should be swallowed.
func (b VimBuffer) CapturingKeys() bool {
	return b.mode == VimInsert || b.searching || b.visualLine
}

func (b VimBuffer) IsSearching() bool  { return b.searching }
func (b VimBuffer) IsVisual() bool     { return b.visualLine }
func (b VimBuffer) Mode() VimMode      { return b.mode }
func (b VimBuffer) Yank() string       { return b.yank }

// ModeStr returns a human-readable mode label.
func (b VimBuffer) ModeStr() string {
	switch {
	case b.searching:
		return "SEARCH"
	case b.visualLine:
		return "V-LINE"
	case b.mode == VimNormal:
		return "NORMAL"
	}
	return "INSERT"
}

func (b *VimBuffer) Focus() tea.Cmd  { return b.textarea.Focus() }
func (b *VimBuffer) Blur()           { b.textarea.Blur() }
func (b VimBuffer) Focused() bool    { return b.textarea.Focused() }

func (b *VimBuffer) SetWidth(w int)  { b.textarea.SetWidth(w) }
func (b *VimBuffer) SetHeight(h int) { b.textarea.SetHeight(h) }
func (b *VimBuffer) SetCharLimit(n int) {
	b.textarea.CharLimit = n
}

func (b *VimBuffer) CursorUp()    { b.textarea.CursorUp() }
func (b *VimBuffer) CursorDown()  { b.textarea.CursorDown() }
func (b *VimBuffer) InsertString(s string) {
	b.textarea.InsertString(s)
	b.notifyChange()
}

func (b VimBuffer) Line() int      { return b.textarea.Line() }
func (b VimBuffer) LineCount() int { return b.textarea.LineCount() }
func (b VimBuffer) LineInfo() textarea.LineInfo {
	return b.textarea.LineInfo()
}

func (b *VimBuffer) Textarea() *textarea.Model { return &b.textarea }

// View renders the raw bubbles textarea (callers may post-process the cursor).
func (b VimBuffer) View() string { return b.textarea.View() }

// ConsumeEsc handles esc in modal contexts. When closeOnNormal is true, a
// second esc in normal mode returns shouldClose=true (used by the cell popup).
func (b *VimBuffer) ConsumeEsc(closeOnNormal bool) (handled bool, shouldClose bool) {
	if b.searching {
		b.searching = false
		b.searchQuery = ""
		return true, false
	}
	if b.visualLine {
		b.visualLine = false
		return true, false
	}
	if b.mode == VimInsert {
		b.commitInsertUndo()
		b.mode = VimNormal
		b.sendKey("left")
		return true, false
	}
	if closeOnNormal && b.mode == VimNormal {
		return true, true
	}
	return false, false
}

// Update routes keyboard and paste messages through vim handling.
func (b VimBuffer) Update(msg tea.Msg) (VimBuffer, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if b.searching {
			return b.handleSearch(keyMsg)
		}
		if b.visualLine {
			return b.handleVisualLine(keyMsg)
		}
		if b.mode == VimNormal {
			return b.handleNormalMode(keyMsg)
		}
		return b.handleInsertMode(keyMsg)
	}

	var cmd tea.Cmd
	b.textarea, cmd = b.textarea.Update(msg)
	b.notifyChange()
	return b, cmd
}

func (b VimBuffer) handleInsertMode(msg tea.KeyMsg) (VimBuffer, tea.Cmd) {
	if msg.String() == "esc" {
		b.commitInsertUndo()
		b.mode = VimNormal
		b.sendKey("left")
		return b, nil
	}

	var cmd tea.Cmd
	b.textarea, cmd = b.textarea.Update(msg)
	b.notifyChange()
	return b, cmd
}

func (b VimBuffer) handleNormalMode(msg tea.KeyMsg) (VimBuffer, tea.Cmd) {
	key := msg.String()

	if b.pending == vimPendingD {
		b.pending = vimPendingNone
		switch key {
		case "d":
			b.pushUndo()
			b.yank = b.currentLineText()
			b.sendKey("home")
			b.sendKey("ctrl+k")
			if b.textarea.LineCount() > 1 {
				b.sendKey("backspace")
			}
		case "w":
			b.pushUndo()
			b.sendKey("alt+d")
		}
		return b, nil
	}
	if b.pending == vimPendingG {
		b.pending = vimPendingNone
		if key == "g" {
			b.sendKey("ctrl+home")
		}
		return b, nil
	}
	if b.pending == vimPendingEqual && b.cfg.EnableFormatEqual {
		b.pending = vimPendingNone
		if key == "=" {
			b.pushUndo()
			formatted := formatSQL(b.textarea.Value())
			b.setValueRaw(formatted)
			b.sendKey("ctrl+home")
		}
		return b, nil
	}

	switch key {
	case "h":
		b.sendKey("left")
	case "l":
		b.sendKey("right")
	case "j":
		b.sendKey("down")
	case "k":
		b.sendKey("up")
	case "0":
		b.sendKey("home")
	case "$":
		b.sendKey("end")
	case "w":
		b.sendKey("alt+right")
	case "b":
		b.sendKey("alt+left")
	case "G":
		b.sendKey("ctrl+end")
	case "g":
		b.pending = vimPendingG
	case "=":
		if b.cfg.EnableFormatEqual {
			b.pending = vimPendingEqual
		}
	case "ctrl+d":
		for i := 0; i < 5; i++ {
			b.sendKey("down")
		}
	case "ctrl+u":
		for i := 0; i < 5; i++ {
			b.sendKey("up")
		}
	case "i":
		b.beginInsert()
	case "a":
		b.sendKey("right")
		b.beginInsert()
	case "A":
		b.sendKey("end")
		b.beginInsert()
	case "I":
		b.sendKey("home")
		b.beginInsert()
	case "o":
		b.beginInsert()
		b.sendKey("end")
		b.sendKey("enter")
	case "O":
		b.beginInsert()
		b.sendKey("home")
		b.sendKey("enter")
		b.sendKey("up")
	case "x":
		b.pushUndo()
		b.sendKey("delete")
	case "d":
		b.pending = vimPendingD
	case "D":
		b.pushUndo()
		b.yank = b.currentLineText()
		b.sendKey("ctrl+k")
	case "y":
		b.yank = b.currentLineText()
	case "p":
		if b.yank != "" {
			b.pushUndo()
			b.sendKey("end")
			b.sendKey("enter")
			b.textarea.InsertString(b.yank)
			b.notifyChange()
		}
	case "c":
		b.beginInsert()
		b.sendKey("ctrl+k")
	case "C":
		b.beginInsert()
		b.sendKey("ctrl+k")
	case "u":
		b.undoOnce()
	case "U":
		b.redoOnce()
	case "/":
		b.searching = true
		b.searchQuery = ""
	case "n":
		b.jumpSearch(1)
	case "N":
		b.jumpSearch(-1)
	case "V":
		if b.cfg.EnableVisualLine {
			b.visualLine = true
			b.visualAnchor = b.textarea.Line()
		}
	case "enter":
		b.sendKey("down")
		b.sendKey("home")
	}

	return b, nil
}

func (b VimBuffer) snap() editorSnap {
	line, col := b.CursorLineCol()
	return editorSnap{value: b.Value(), line: line, col: col}
}

func (b *VimBuffer) pushSnap(s editorSnap) {
	b.undo = append(b.undo, s)
	if len(b.undo) > maxEditorUndo {
		b.undo = b.undo[len(b.undo)-maxEditorUndo:]
	}
	b.redo = nil
}

func (b *VimBuffer) pushUndo() {
	b.pushSnap(b.snap())
}

func (b *VimBuffer) restoreSnap(s editorSnap) {
	b.setValueRaw(s.value)
	b.restoreCursor(s.line, s.col)
}

func (b *VimBuffer) restoreCursor(line, col int) {
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}
	for b.textarea.Line() > 0 {
		b.textarea.CursorUp()
	}
	for i := 0; i < line; i++ {
		if b.textarea.Line() >= b.textarea.LineCount()-1 {
			break
		}
		b.textarea.CursorDown()
	}
	b.textarea.SetCursor(col)
	b.notifyChange()
}

func (b *VimBuffer) beginInsert() {
	s := b.snap()
	b.insertBase = &s
	b.mode = VimInsert
}

func (b *VimBuffer) commitInsertUndo() {
	if b.insertBase == nil {
		return
	}
	if b.Value() != b.insertBase.value {
		b.pushSnap(*b.insertBase)
	}
	b.insertBase = nil
}

func (b *VimBuffer) undoOnce() {
	if len(b.undo) == 0 {
		return
	}
	cur := b.snap()
	last := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	b.redo = append(b.redo, cur)
	b.restoreSnap(last)
}

func (b *VimBuffer) redoOnce() {
	if len(b.redo) == 0 {
		return
	}
	cur := b.snap()
	last := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	b.undo = append(b.undo, cur)
	b.restoreSnap(last)
}

func (b VimBuffer) visualRange() (lo, hi int) {
	cur := b.textarea.Line()
	lo, hi = b.visualAnchor, cur
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func (b VimBuffer) handleSearch(msg tea.KeyMsg) (VimBuffer, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		b.searching = false
		b.searchQuery = ""
		return b, nil
	case "enter":
		b.searchFocus = b.searchQuery
		b.searching = false
		b.searchQuery = ""
		b.searchOffset = b.cursorOffset() - 1
		b.jumpSearch(1)
		return b, nil
	case "backspace":
		runes := []rune(b.searchQuery)
		if len(runes) > 0 {
			b.searchQuery = string(runes[:len(runes)-1])
		}
		return b, nil
	}
	if ch, ok := keyFilterChar(msg); ok {
		b.searchQuery += ch
	}
	return b, nil
}

func (b VimBuffer) handleVisualLine(msg tea.KeyMsg) (VimBuffer, tea.Cmd) {
	key := msg.String()
	if b.pending == vimPendingG {
		b.pending = vimPendingNone
		if key == "g" {
			b.sendKey("ctrl+home")
		}
		return b, nil
	}
	switch key {
	case "esc", "ctrl+c", "v":
		b.visualLine = false
	case "j", "down":
		b.sendKey("down")
	case "k", "up":
		b.sendKey("up")
	case "G":
		b.sendKey("ctrl+end")
	case "g":
		b.pending = vimPendingG
	case "y":
		b.yankVisual()
		b.visualLine = false
	case "d", "x":
		b.deleteVisual()
		b.visualLine = false
	}
	return b, nil
}

func (b *VimBuffer) yankVisual() {
	lo, hi := b.visualRange()
	lines := strings.Split(b.Value(), "\n")
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	if lo > hi || len(lines) == 0 {
		return
	}
	b.yank = strings.Join(lines[lo:hi+1], "\n")
}

func (b *VimBuffer) deleteVisual() {
	b.yankVisual()
	b.pushUndo()
	lo, hi := b.visualRange()
	lines := strings.Split(b.Value(), "\n")
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	newLines := append(append([]string{}, lines[:lo]...), lines[hi+1:]...)
	b.setValueRaw(strings.Join(newLines, "\n"))
	b.restoreCursor(lo, 0)
}

func (b VimBuffer) cursorOffset() int {
	line, col := b.CursorLineCol()
	lines := strings.Split(b.Value(), "\n")
	off := 0
	for i := 0; i < line && i < len(lines); i++ {
		off += len([]rune(lines[i])) + 1
	}
	off += col
	return off
}

func (b *VimBuffer) jumpSearch(dir int) {
	q := b.searchFocus
	if q == "" {
		return
	}
	lower := strings.ToLower(b.Value())
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
	cur := b.searchOffset
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
	b.searchOffset = pick
	b.moveToByteOffset(pick)
}

func (b *VimBuffer) moveToByteOffset(off int) {
	val := b.Value()
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
	b.restoreCursor(line, col)
}

func (b VimBuffer) searchPrompt(width int) string {
	q := b.searchQuery
	var out strings.Builder
	out.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Render("/"))
	if q == "" {
		out.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" "))
	} else {
		out.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Render(q))
		out.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" "))
	}
	return lipgloss.NewStyle().Width(width).Render(out.String())
}

func (b *VimBuffer) sendKey(keyStr string) {
	msg := tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(keyStr),
	}
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

	b.textarea, _ = b.textarea.Update(msg)
	b.notifyChange()
}

func (b VimBuffer) currentLineText() string {
	lines := strings.Split(b.Value(), "\n")
	if len(lines) == 0 {
		return ""
	}
	idx := b.textarea.Line()
	if idx >= len(lines) {
		return ""
	}
	return lines[idx]
}

// CursorLineCol returns the logical line index and rune column.
func (b VimBuffer) CursorLineCol() (int, int) {
	line := b.textarea.Line()
	li := b.textarea.LineInfo()
	col := li.StartColumn + li.ColumnOffset
	return line, col
}

func (b VimBuffer) wordBeforeCursor() (word string, startCol int) {
	_, col := b.CursorLineCol()
	line := b.currentLineText()
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

func (b VimBuffer) textBeforeCursor() string {
	value := b.Value()
	cursorLine, cursorCol := b.CursorLineCol()
	lines := strings.Split(value, "\n")
	var out strings.Builder
	for i := 0; i < cursorLine && i < len(lines); i++ {
		out.WriteString(lines[i])
		out.WriteByte('\n')
	}
	if cursorLine >= 0 && cursorLine < len(lines) {
		runes := []rune(lines[cursorLine])
		if cursorCol > len(runes) {
			cursorCol = len(runes)
		}
		out.WriteString(string(runes[:cursorCol]))
	}
	return out.String()
}
