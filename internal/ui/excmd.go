package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rsiota/creel/internal/db"
	"github.com/rsiota/creel/internal/version"
)

// exCmd is the vim-style ":" command line: a modal prompt at the bottom of the
// workspace. Type a command (with optional arguments), enter runs it, esc
// cancels, ↑/↓ recalls history. Unknown input in the results view falls back
// to a column jump, preserving the legacy ":" behaviour.
type exCmd struct {
	visible   bool
	input     string
	hist      []string     // command history, most-recent last
	histIdx   int          // recall cursor; len(hist) == "fresh input"
	comp      []exCompItem // popup candidates; empty = no popup
	argMode   bool         // true: comp holds argument candidates (Tab completes last token)
	selIdx    int          // popup selection cursor (0 = top/Tab target); valid when len(comp) > 0
	recalling bool         // up/down is walking history; cleared by typing so it returns to popup nav
}

// exCompItem is one row in the ":" completion popup. In verb mode verb/desc
// are set (the command being completed); in argument mode candidate is set
// (the argument value being completed).
type exCompItem struct {
	verb      string // verb mode: canonical verb inserted by Tab
	candidate string // arg mode: argument value inserted by Tab
	usage     string // invocation form, e.g. ":w [file]"
	desc      string
}

// handleExKey routes keys to the open ":" command line. It is modal: every
// key is consumed while the ex line is visible. enter runs the input (or
// completes the highlighted popup row when the typed token is still partial);
// esc cancels; ↑/↓ recalls history.
func (m *Model) handleExKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.ex.Hide()
		m.layoutWorkspace()
		return *m, nil
	case "enter":
		if m.exEnterShouldComplete() {
			m.applyExSelection()
			return *m, nil
		}
		input := strings.TrimSpace(m.ex.input)
		m.ex.input = ""
		m.ex.visible = false
		m.ex.comp = nil
		m.ex.argMode = false
		if input == "" {
			m.layoutWorkspace()
			return *m, nil
		}
		m.ex.hist = append(m.ex.hist, input)
		m.ex.histIdx = len(m.ex.hist)
		cmd := m.runExCommand(input)
		m.layoutWorkspace()
		return *m, cmd
	case "up", "down":
		// With the popup visible and not mid-history, up/down move its
		// selection (mirroring the command palette). Otherwise they walk
		// history. A "recalling" flag keeps a history walk going even when a
		// recalled value would itself show a popup, and typing clears it so
		// up/down returns to popup navigation. The input!="" check keeps
		// ":<up>" (fresh prompt) recalling the last command, vim-style.
		if !m.ex.recalling && len(m.ex.comp) > 0 && m.ex.input != "" {
			if msg.String() == "up" {
				m.ex.moveSel(-1)
			} else {
				m.ex.moveSel(1)
			}
			return *m, nil
		}
		m.ex.recalling = true
		if msg.String() == "up" {
			m.ex.recall(-1)
		} else {
			m.ex.recall(1)
		}
		m.recomputeExCompletion()
		return *m, nil
	case "tab":
		m.applyExSelection()
		return *m, nil
	case "backspace":
		if len(m.ex.input) > 0 {
			r := []rune(m.ex.input)
			m.ex.input = string(r[:len(r)-1])
			m.ex.recalling = false
			m.recomputeExCompletion()
		}
		return *m, nil
	}
	if ch, ok := keyFilterChar(msg); ok {
		m.ex.input += ch
		m.ex.recalling = false
		m.recomputeExCompletion()
	}
	return *m, nil
}

// applyExSelection fills the input from the highlighted popup row (Tab/Enter).
func (m *Model) applyExSelection() {
	if len(m.ex.comp) == 0 {
		return
	}
	item := m.ex.selectedCompItem()
	if m.ex.argMode {
		m.ex.input = applyArgCompletion(m.ex.input, item.candidate)
	} else {
		m.ex.input = item.verb
	}
	m.ex.recalling = false
	m.recomputeExCompletion()
}

// exEnterShouldComplete reports whether Enter should accept the highlighted
// popup row instead of running the current input. Valid command aliases (e.g.
// ":w") still run immediately; partial verbs and partial argument tokens
// complete first so a second Enter executes the finished line.
func (m *Model) exEnterShouldComplete() bool {
	if len(m.ex.comp) == 0 || strings.TrimSpace(m.ex.input) == "" {
		return false
	}
	if m.ex.argMode {
		verb, _ := verbPrefix(m.ex.input)
		rest := strings.TrimLeft(m.ex.input[len(verb):], " \t")
		_, partial := splitArgsPartial(rest)
		if partial == "" {
			return false
		}
		return !strings.EqualFold(partial, m.ex.selectedCompItem().candidate)
	}
	verb, hasSpace := verbPrefix(m.ex.input)
	if hasSpace {
		return false
	}
	return exLookup(strings.ToLower(strings.TrimSuffix(verb, "!"))) == nil
}

func (ex exCmd) selectedCompItem() exCompItem {
	sel := ex.selIdx
	if sel < 0 || sel >= len(ex.comp) {
		sel = 0
	}
	return ex.comp[sel]
}

// Open shows the ex command line with an empty buffer and seeds the
// verb-completion popup with every command, so ":" alone is discoverable.
func (ex *exCmd) Open() {
	ex.visible = true
	ex.input = ""
	ex.histIdx = len(ex.hist)
	ex.recalling = false
	ex.recomputeCompletion()
}

// Hide closes the ex command line and drops any completion popup.
func (ex *exCmd) Hide() {
	ex.visible = false
	ex.comp = nil
	ex.argMode = false
	ex.selIdx = 0
	ex.recalling = false
}

// IsVisible reports whether the ex command line is shown.
func (ex exCmd) IsVisible() bool { return ex.visible }

// verbPrefix returns the command verb being typed (the run of input before the
// first space/tab) and whether a separator follows it — i.e. whether the
// cursor has moved past the verb into arguments, where verb completion no
// longer applies.
func verbPrefix(input string) (verb string, hasSpace bool) {
	for i, r := range input {
		if r == ' ' || r == '\t' {
			return input[:i], true
		}
	}
	return input, false
}

// recomputeCompletion refreshes the verb-completion list from the current
// input. Completion applies only to the verb (before any space); matches are
// prefix-based and case-insensitive, one row per command even if several of
// its aliases match. The popup is hidden once the typed verb is an exact,
// unambiguous canonical match (the user has fully specified the command).
func (ex *exCmd) recomputeCompletion() {
	ex.comp = ex.comp[:0]
	ex.argMode = false // verb mode
	ex.selIdx = 0
	verb, hasSpace := verbPrefix(ex.input)
	if hasSpace {
		return
	}
	needle := strings.ToLower(verb)
	for _, s := range exCommands() {
		for _, v := range s.verbs {
			if strings.HasPrefix(v, needle) {
				ex.comp = append(ex.comp, exCompItem{
					verb:  s.verbs[0],
					usage: s.usage,
					desc:  s.desc,
				})
				break
			}
		}
	}
	if len(ex.comp) == 1 && ex.comp[0].verb == needle && needle != "" {
		ex.comp = nil
	}
}

// recomputeExCompletion is the Model-level entry point for the ":" popup. Verb
// completion (before any space) is delegated to recomputeCompletion; argument
// completion (once the cursor is past the verb) needs Model data, so it runs
// here. The two modes set exCmd.argMode so rendering and Tab know what a row
// represents.
func (m *Model) recomputeExCompletion() {
	verb, hasSpace := verbPrefix(m.ex.input)
	if !hasSpace {
		m.ex.recomputeCompletion()
		return
	}
	m.ex.comp = m.ex.comp[:0]
	m.ex.argMode = true
	m.ex.selIdx = 0
	// A trailing "!" (force) on the verb must be stripped, mirroring parseExLine.
	lookup := strings.TrimSuffix(verb, "!")
	spec := exLookup(lookup)
	if spec == nil || spec.complete == nil {
		return
	}
	rest := strings.TrimLeft(m.ex.input[len(verb):], " \t")
	args, partial := splitArgsPartial(rest)
	cands := spec.complete(m, args, partial)
	for _, c := range rankStrings(partial, cands) {
		m.ex.comp = append(m.ex.comp, exCompItem{candidate: c})
	}
}

// splitArgsPartial splits the text after the verb into completed arguments and
// the token currently being typed (partial, "" when the cursor sits in the gap
// between arguments). Quoting follows splitShellFields; a partially-typed
// quoted argument yields its raw content, which is fine in practice since
// table/theme names are single words.
func splitArgsPartial(rest string) (args []string, partial string) {
	if rest == "" {
		return nil, ""
	}
	fields := splitShellFields(rest)
	last := rest[len(rest)-1]
	if last == ' ' || last == '\t' {
		return fields, ""
	}
	if len(fields) == 0 {
		return nil, ""
	}
	return fields[:len(fields)-1], fields[len(fields)-1]
}

// rankStrings fuzzy-ranks items by query (best match first); an empty query
// returns all items sorted alphabetically for stable display.
func rankStrings(query string, items []string) []string {
	if len(items) == 0 {
		return nil
	}
	if query == "" {
		out := append([]string(nil), items...)
		sort.Strings(out)
		return out
	}
	ranked := fuzzyRank(query, items, func(s string) string { return s }, nil)
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.Item
	}
	return out
}

// applyArgCompletion returns input with its last whitespace-delimited token
// replaced by candidate — what Tab does in argument mode.
func applyArgCompletion(input, candidate string) string {
	if idx := strings.LastIndex(input, " "); idx >= 0 {
		return input[:idx+1] + candidate
	}
	return candidate
}

// exCompletionWidth is the fixed content width of the verb-completion popup:
// descriptions truncate to fit so the box is a stable rectangle that doesn't
// grow or shrink as the filtered list changes. exCompletionCmdW caps the
// command-name column so a future very long verb can't force the popup wide;
// both are easy knobs to tune. (The full argument syntax lives in :help, not
// the popup — showing it here padded short commands with a lot of empty
// space.)
const (
	exCompletionWidth = 60
	exCompletionCmdW  = 16
)

// completionView renders the verb-completion popup (one row per candidate)
// for display directly above the ":" prompt, or "" when nothing applies. The
// selected row (initially the top match) is the Tab target. Rendering mirrors
// the palette (Ctrl+P): blue
// command names, grey descriptions, and a solid highlight bar on the Tab
// target. Each row shows the command name (":verb") and its short description
// — the full invocation form (with arguments) is left to :help, so the command
// column stays narrow and descriptions aren't pushed far to the right. The
// popup is a fixed-width rectangle: the command column is pinned to the global
// max command-name width (capped) and descriptions truncate with "…", so
// neither the box nor the columns jitter as you type.
func (ex exCmd) completionView(maxW int) string {
	if !ex.visible || len(ex.comp) == 0 {
		return ""
	}
	if ex.argMode {
		return ex.argCompletionView(maxW)
	}
	const maxRows = 9
	items, localSel := exPopupWindow(ex.comp, ex.selIdx, maxRows)
	// Stable command column: the global max ":verb" width (capped), computed
	// from the full command set rather than the current filter so it never
	// shifts as you type.
	cmdW := 0
	for _, s := range exCommands() {
		if w := runeLen(":" + s.verbs[0]); w > cmdW {
			cmdW = w
		}
	}
	if cmdW > exCompletionCmdW {
		cmdW = exCompletionCmdW
	}
	descW := exCompletionWidth - cmdW - 4 // 4 = 2 leading + 2 gap
	if descW < 8 {
		descW = 8
	}
	// fit truncates s to w (with "…") then right-pads to exactly w, so every
	// row is the same width.
	fit := func(s string, w int) string {
		t := truncateRunes(s, w)
		return t + strings.Repeat(" ", w-runeLen(t))
	}
	var lines []string
	for i, it := range items {
		cmd := fit(":"+it.verb, cmdW)
		desc := fit(it.desc, descW)
		var row string
		if i == localSel {
			// Tab target: a solid highlight bar, mirroring the palette's
			// selected row (bg colorPrimary, fg colorBg, "❯" marker).
			row = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBg).
				Render("❯ " + cmd + "  " + desc)
		} else {
			cmdStr := lipgloss.NewStyle().Foreground(colorPrimary).Render(cmd)
			descStr := lipgloss.NewStyle().Foreground(colorLabel).Render(desc)
			row = "  " + cmdStr + "  " + descStr
		}
		lines = append(lines, row)
	}
	return lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorBorder).
		Render(strings.Join(lines, "\n"))
}

// argCompletionView renders the argument-candidate popup: a single column of
// candidate names with the selected row highlighted (the Tab target), mirroring the verb
// popup's styling. One column is enough — the candidate is the whole value.
// Unlike the verb popup (a fixed rectangle), the column sizes to its content
// and is capped only by the available terminal width (maxW): a long file path
// is the useful information, so it gets as much room as the terminal allows
// instead of being cropped to a fixed 16 cells.
func (ex exCmd) argCompletionView(maxW int) string {
	const maxRows = 9
	items, localSel := exPopupWindow(ex.comp, ex.selIdx, maxRows)
	// Column width is the max over ALL candidates (not just the visible
	// window) so the box width stays stable as the window scrolls.
	colW := 0
	for _, it := range ex.comp {
		if w := runeLen(it.candidate); w > colW {
			colW = w
		}
	}
	// The popup sits at column 1; each row is a 2-cell prefix ("❯ " or "  ")
	// plus the candidate, wrapped by a 2-cell border, so the candidate column
	// reaches the right edge at width-5 (one more kept as a margin). Unknown
	// width (0, e.g. unsized) falls back to a default; a pathologically narrow
	// terminal still shows a few characters.
	const overhead = 6
	avail := 60
	if maxW > 0 {
		avail = maxW - overhead
		if avail < 8 {
			avail = 8
		}
	}
	if colW > avail {
		colW = avail
	}
	fit := func(s string, w int) string {
		t := truncateRunes(s, w)
		return t + strings.Repeat(" ", w-runeLen(t))
	}
	var lines []string
	for i, it := range items {
		cell := fit(it.candidate, colW)
		if i == localSel {
			lines = append(lines, lipgloss.NewStyle().
				Background(colorPrimary).Foreground(colorBg).
				Render("❯ "+cell))
		} else {
			lines = append(lines, lipgloss.NewStyle().
				Foreground(colorPrimary).Render("  "+cell))
		}
	}
	return lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorBorder).
		Render(strings.Join(lines, "\n"))
}

// moveSel moves the popup selection cursor by delta, wrapping around
// (mirroring the command palette). Callers guard len(comp) > 0.
func (ex *exCmd) moveSel(delta int) {
	n := len(ex.comp)
	if n == 0 {
		ex.selIdx = 0
		return
	}
	ex.selIdx = (ex.selIdx + delta + n) % n
}

// exPopupWindow returns the slice of popup rows to render and the index of the
// selected row within that slice, keeping selIdx visible with a sliding window
// when there are more candidates than maxRows (mirroring the command palette:
// the window advances only once the cursor reaches its bottom edge).
func exPopupWindow(comp []exCompItem, selIdx, maxRows int) (items []exCompItem, localSel int) {
	n := len(comp)
	if n == 0 {
		return nil, 0
	}
	if selIdx < 0 {
		selIdx = 0
	}
	if selIdx >= n {
		selIdx = n - 1
	}
	start := 0
	if selIdx >= maxRows {
		start = selIdx - maxRows + 1
	}
	end := start + maxRows
	if end > n {
		end = n
	}
	return comp[start:end], selIdx - start
}

// recall steps through command history (delta < 0 = older, > 0 = newer).
func (ex *exCmd) recall(delta int) {
	n := len(ex.hist)
	if n == 0 {
		return
	}
	ex.histIdx += delta
	if ex.histIdx < 0 {
		ex.histIdx = 0
	}
	if ex.histIdx > n {
		ex.histIdx = n
	}
	if ex.histIdx < n {
		ex.input = ex.hist[ex.histIdx]
	} else {
		ex.input = ""
	}
}

// View renders the prompt: ":" plus the current input and a trailing underline
// cursor (an overlay cell, so it never shifts the text).
func (ex exCmd) View() string {
	if !ex.visible {
		return ""
	}
	return lipgloss.NewStyle().Foreground(colorPrimary).Render(":"+ex.input) +
		lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" ")
}

// parseExLine splits a ":" command line into a lowercased verb and its
// arguments, honoring single/double-quoted substrings so an argument may
// contain spaces. A trailing "!" on the verb (e.g. "q!") is returned as
// force=true and stripped from the verb. The original (untrimmed) fields are
// not preserved; callers needing the raw identifier (the column-jump fallback)
// use the original input string.
func parseExLine(input string) (verb string, args []string, force bool) {
	fields := splitShellFields(input)
	if len(fields) == 0 {
		return "", nil, false
	}
	verb = strings.ToLower(fields[0])
	if strings.HasSuffix(verb, "!") {
		force = true
		verb = strings.TrimSuffix(verb, "!")
	}
	return verb, fields[1:], force
}

// splitShellFields is a small shell-like field splitter: whitespace separates
// fields, single/double quotes group a field, and a backslash escapes the next
// rune (except inside single quotes, matching POSIX-ish behaviour).
func splitShellFields(s string) []string {
	var fields []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\' && i+1 < len(runes) && !inSingle:
			cur.WriteRune(runes[i+1])
			i++
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	return fields
}

// runExCommand parses and executes a ":" command line, returning any async
// command. Known verbs are dispatched through the ex command registry
// (exCommands in excmd_registry.go); an unknown bare identifier in the
// results view falls back to a column jump, preserving the legacy ":"
// behaviour, and anything else is reported as E492. Errors and short feedback
// are set via the transient status-bar message (m.schemaMsg).
func (m *Model) runExCommand(input string) tea.Cmd {
	verb, args, force := parseExLine(input)
	if spec := exLookup(verb); spec != nil {
		return spec.run(m, args, force)
	}
	// Legacy fallback: in the results view a bare identifier jumps to the
	// best-matching column. Use the original input so column-name case is
	// preserved.
	if m.focus == FocusResults && m.results.NumCols() > 0 && len(args) == 0 {
		if idx := bestColumnMatch(m.results.columns, input); idx >= 0 {
			m.results.SetCursor(m.results.CursorRow(), idx)
			return nil
		}
	}
	m.schemaMsg = fmt.Sprintf("E492: not a command: %s", input)
	return nil
}

// exWrite commits staged cell edits (:w).
func (m *Model) exWrite(force bool) tea.Cmd {
	_ = force // :w! is accepted for parity; saves don't prompt otherwise
	if !m.results.HasDirtyCells() {
		m.schemaMsg = "no changes to save"
		return nil
	}
	return m.saveEdits()
}

// exQuit closes the active tab (:q), or quits the app when it is the last tab
// — mirroring vim, where :q on the final window exits. (The q / ctrl+q keys
// quit unconditionally; :q reaches the same path once no tabs remain.) Unsaved
// edits block unless forced (:q!).
func (m *Model) exQuit(force bool) tea.Cmd {
	if !force && m.results.HasDirtyCells() {
		m.schemaMsg = "unsaved changes — use :q! to discard"
		return nil
	}
	if len(m.resultsTabs) <= 1 {
		m.beginQuit()
		return tea.Quit
	}
	m.closeTab(m.activeTabID)
	return nil
}

// exQuitAll quits the app after closing every tab (:qa). Unsaved edits in any
// tab block unless forced (:qa!). Dirty state is checked across all tabs via
// saveTabState so inactive tabs are included.
func (m *Model) exQuitAll(force bool) tea.Cmd {
	m.saveTabState()
	if !force {
		for _, tab := range m.resultsTabs {
			if tab.Results.HasDirtyCells() {
				m.schemaMsg = "unsaved changes — use :qa! to discard"
				return nil
			}
		}
	}
	m.beginQuit()
	return tea.Quit
}

// exRun executes the statement under the cursor (:run / :r). Shares
// executeQuery with ctrl+e / \.
func (m *Model) exRun() tea.Cmd {
	if m.editor.StatementAtCursor() == "" {
		m.schemaMsg = "nothing to run"
		return nil
	}
	return m.executeQuery()
}

// exTabNew opens a new results tab with the current editor contents, matching
// the bare `t` key in results/connections.
func (m *Model) exTabNew() tea.Cmd {
	query := m.editor.Value()
	m.addTab(generateTabTitle(query), query)
	return nil
}

// exTabClose closes the active tab (:tabclose). Unlike :q, the last tab is
// refused (vim :tabclose) — use :q to quit. Unsaved edits block unless forced.
func (m *Model) exTabClose(force bool) tea.Cmd {
	if len(m.resultsTabs) <= 1 {
		m.schemaMsg = "cannot close the last tab — use :q to quit"
		return nil
	}
	if !force && m.results.HasDirtyCells() {
		m.schemaMsg = "unsaved changes — use :tabclose! to discard"
		return nil
	}
	m.closeTab(m.activeTabID)
	return nil
}

// exTabNext activates the next tab (cyclic), sharing TabBar.NextTab with g t.
func (m *Model) exTabNext() tea.Cmd {
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	if nextID := m.tabBar.NextTab(); nextID >= 0 {
		m.setActiveTab(nextID)
	}
	return nil
}

// exTabPrev activates the previous tab (cyclic), sharing TabBar.PrevTab with g T.
func (m *Model) exTabPrev() tea.Cmd {
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	if prevID := m.tabBar.PrevTab(); prevID >= 0 {
		m.setActiveTab(prevID)
	}
	return nil
}

// exTabs lists open tabs in the status bar, marking the active one with [].
func (m *Model) exTabs() tea.Cmd {
	if len(m.resultsTabs) == 0 {
		m.schemaMsg = "no tabs"
		return nil
	}
	parts := make([]string, 0, len(m.resultsTabs))
	for i, tab := range m.resultsTabs {
		title := tab.Title
		if title == "" {
			title = "untitled"
		}
		label := fmt.Sprintf("%d:%s", i+1, title)
		if tab.ID == m.activeTabID {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	m.schemaMsg = strings.Join(parts, "  ")
	return nil
}

// exCopy copies the cell under the cursor to the clipboard (:copy). Shares
// copyCursorCell with the yy chord.
func (m *Model) exCopy() tea.Cmd {
	return m.copyCursorCell()
}

// exDiscard discards staged cell edits (:discard), sharing discardResultsEdits
// with the results D key. Stages the y/enter confirmation when
// confirm_destructive is on. Gives feedback when there is nothing to discard
// (the key stays silent).
func (m *Model) exDiscard(force bool) tea.Cmd {
	if m.discardResultsEdits(force) {
		return nil
	}
	if !m.results.IsEditable() {
		m.schemaMsg = "nothing to discard — results not editable"
	} else {
		m.schemaMsg = "no changes to discard"
	}
	return nil
}

// exClone duplicates the marked rows or the cursor row (:clone), sharing
// cloneRows with the results P key. Gives feedback for the no-op cases the key
// swallows silently (no connection / nothing editable).
func (m *Model) exClone() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if !m.results.IsEditable() || m.results.NumRows() == 0 {
		m.schemaMsg = "no editable rows to clone"
		return nil
	}
	return m.cloneRows()
}

// exFollow follows the foreign key under the cursor (:follow), sharing
// followForeignKey with the g d chord. Gives feedback when there is no FK on
// the current column (the key silently no-ops).
func (m *Model) exFollow() tea.Cmd {
	if m.results.NumCols() == 0 {
		m.schemaMsg = "no results to navigate"
		return nil
	}
	if _, ok := m.results.ForeignKeyAtCursor(); !ok {
		m.schemaMsg = "no foreign key on this column"
		return nil
	}
	return m.followForeignKey()
}

// exBack returns to the previous query in the navigation stack (:back),
// sharing goBackQuery with the g b chord.
func (m *Model) exBack() tea.Cmd {
	if len(m.queryStack) == 0 {
		m.schemaMsg = "nowhere to go back to"
		return nil
	}
	return m.goBackQuery()
}

// exKeep keeps only rows equal to the cursor cell (:keep); exHide is its
// inverse. Both share quickFilterCell with the * / ! keys.
func (m *Model) exKeep() tea.Cmd { return m.exQuickFilter(false) }
func (m *Model) exHide() tea.Cmd { return m.exQuickFilter(true) }

func (m *Model) exQuickFilter(negate bool) tea.Cmd {
	if !m.canFilter() || m.results.NumRows() == 0 {
		m.schemaMsg = "no rows to filter"
		return nil
	}
	if m.results.CursorCellValue() == "" {
		m.schemaMsg = "cursor cell is empty — move to a value first"
		return nil
	}
	return m.quickFilterCell(negate)
}

// exUndo removes the last filter (:undo); exUnfilter clears all of them
// (:unfilter). Both share undoFilter / clearFilters with the u / c keys.
func (m *Model) exUndo() tea.Cmd {
	if len(m.filters) == 0 {
		m.schemaMsg = "no filters to undo"
		return nil
	}
	return m.undoFilter()
}

func (m *Model) exUnfilter() tea.Cmd {
	if len(m.filters) == 0 {
		m.schemaMsg = "no filters to clear"
		return nil
	}
	return m.clearFilters()
}

// exCopyInsert copies the current result rows as INSERT statements to the
// clipboard (:copyinsert), sharing copyRowsAsInsert with the Y key.
func (m *Model) exCopyInsert() tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no rows to copy"
		return nil
	}
	if cmd := m.copyRowsAsInsert(); cmd != nil {
		return cmd
	}
	m.schemaMsg = "nothing to copy"
	return nil
}

// exCopyRow copies the marked rows (or the cursor row when none are marked)
// to the clipboard as TSV by default, or a specified format
// (:copyrow csv|tsv|md|json|jsonl), sharing copyRowsDelimited.
func (m *Model) exCopyRow(args []string) tea.Cmd {
	format := fmtTSV
	if len(args) > 0 {
		f, ok := parseExportFormat(args[0])
		if !ok {
			m.schemaMsg = ":copyrow format must be one of: csv, json, jsonl, md, tsv"
			return nil
		}
		format = f
	}
	return m.copyRowsDelimited(format)
}

// exRegex applies a regex search to the current page (:regex <pattern>),
// sharing applySearch with the g/ search mode. Patterns may contain spaces.
func (m *Model) exRegex(args []string) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no rows to search"
		return nil
	}
	pattern := strings.Join(args, " ")
	if strings.TrimSpace(pattern) == "" {
		m.schemaMsg = ":regex needs a pattern"
		return nil
	}
	m.applySearch(pattern)
	return nil
}

// exHideColumn hides a column (:hidecolumn [col]). Defaults to the column
// under the cursor; with a name it hides that column. Mirrors the H key.
func (m *Model) exHideColumn(args []string) tea.Cmd {
	if m.results.NumCols() == 0 {
		m.schemaMsg = "no columns to hide"
		return nil
	}
	col := m.results.CursorCol()
	if len(args) > 0 {
		found := -1
		for i := 0; i < m.results.NumCols(); i++ {
			if strings.EqualFold(m.results.ColumnName(i), args[0]) {
				found = i
				break
			}
		}
		if found < 0 {
			m.schemaMsg = fmt.Sprintf("no such column: %s", args[0])
			return nil
		}
		col = found
	}
	if !m.results.HideColumn(col) {
		m.schemaMsg = "column already hidden or is the last visible column"
		return nil
	}
	return nil
}

// exShowColumns reveals all hidden columns (:showcolumns), mirroring g H.
func (m *Model) exShowColumns() tea.Cmd {
	m.results.ShowAllColumns()
	return nil
}

// exNew clears the editor to an empty scratch buffer (:new). Does not open a
// new tab — use :tabnew for that.
func (m *Model) exNew() tea.Cmd {
	m.editor.SetValue("")
	m.schemaMsg = "new buffer"
	return m.editor.Focus()
}

// exVersion prints the build version in the status bar (:version).
func (m *Model) exVersion() tea.Cmd {
	m.schemaMsg = version.String()
	return nil
}

// exRecent lists or re-opens recently touched tables (:recent [n|name]).
// Tables are recorded by openTable (:goto, sidebar enter, mouse). Bare lists
// them in the lookup overlay; a number opens by MRU rank (1 = most recent);
// a name opens if it appears in the recent list.
func (m *Model) exRecent(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	names := m.liveRecentTables()
	if len(names) == 0 {
		m.schemaMsg = "no recent tables"
		return nil
	}
	if len(args) == 0 {
		return m.exListNames("Recent", names)
	}
	arg := args[0]
	if n, err := strconv.Atoi(arg); err == nil {
		if n < 1 || n > len(names) {
			m.schemaMsg = fmt.Sprintf("recent rank out of range (1-%d)", len(names))
			return nil
		}
		return m.openTable(names[n-1])
	}
	name := resolveNameInList(arg, names)
	if name == "" {
		m.schemaMsg = fmt.Sprintf("not in recent: %s", arg)
		return nil
	}
	return m.openTable(name)
}

// exTruncate empties a table (:truncate [table]), defaulting to the current
// table. Shares execTruncate with the sidebar T key. Stages the enter/esc
// confirm dialog when confirm_destructive is on, unless forced (:truncate!).
func (m *Model) exTruncate(name string, force bool) tea.Cmd {
	table := m.resolveDDLTableArg(name)
	if table == "" {
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: truncate disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	if !force && m.confirmDestructive() {
		m.truncateConfirm = table
		return nil
	}
	return m.execTruncate(table)
}

// exDrop drops a table (:drop [table]), defaulting to the current table.
// Shares execDropTable with the sidebar D key. Stages the typed-name confirm
// when confirm_destructive is on, unless forced (:drop!).
func (m *Model) exDrop(name string, force bool) tea.Cmd {
	table := m.resolveDDLTableArg(name)
	if table == "" {
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: drop disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	if !force && m.confirmDestructive() {
		m.dropTableConfirm = table
		m.dropTableInput = ""
		return nil
	}
	return m.execDropTable(table)
}

// exRename renames a table (:rename [old] [new]). With no args (or one), opens
// the rename form for the current/named table (same as sidebar r). With two
// args, renames non-interactively via BuildRenameTableSQL.
func (m *Model) exRename(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: rename disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	switch len(args) {
	case 0:
		table := m.resolveDDLTableArg("")
		if table == "" {
			return nil
		}
		return m.openTableRenameForm(table)
	case 1:
		table := m.resolveDDLTableArg(args[0])
		if table == "" {
			return nil
		}
		return m.openTableRenameForm(table)
	default:
		old := m.resolveTableName(args[0])
		if old == "" {
			m.schemaMsg = fmt.Sprintf("no such table: %s", args[0])
			return nil
		}
		newName := args[1]
		sql, err := db.BuildRenameTableSQL(m.connection.Config().Driver, old, newName, m.tables)
		if err != nil {
			m.schemaMsg = err.Error()
			return nil
		}
		return m.execSchemaDDL(old, sql, db.SchemaRenameTable, newName)
	}
}

// exCreate opens the inline table designer (:create), sharing openCreateTableForm
// with the sidebar N key.
func (m *Model) exCreate() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: create table disabled"
		return nil
	}
	return m.openCreateTableForm()
}

// exAddColumn adds a column to a table (:addcolumn ...), sharing
// openAddColumnFormForTable + execSchemaDDL(SchemaAddColumn) with the sidebar a
// key. With zero or one argument it opens the form for the current/named table
// (like :rename); with three or more — <table> <name> <type> [nullable]
// [default] — it runs ALTER TABLE ADD COLUMN directly. SQL types containing
// spaces (e.g. Postgres "double precision") should use the form, since the
// type is one shell field here.
func (m *Model) exAddColumn(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: add column disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	// 0-1 args: open the form for the current / named table (default-to-current,
	// preferring the sidebar cursor like the a key).
	if len(args) <= 1 {
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		table := m.resolveDDLTableArg(name)
		if table == "" {
			return nil
		}
		return m.openAddColumnFormForTable(table)
	}
	// Two args is ambiguous (table + name, no type) — ask for the type.
	if len(args) < 3 {
		m.schemaMsg = "usage: :addcolumn <table> <name> <type> [nullable] [default]"
		return nil
	}
	// Direct: <table> <name> <type> [nullable] [default].
	table := m.resolveDDLTableArg(args[0])
	if table == "" {
		return nil
	}
	col := db.ColumnDef{
		Name: args[1],
		Type: args[2],
	}
	if len(args) >= 4 {
		nullable, errMsg := parseNullable(args[3])
		if errMsg != "" {
			m.schemaMsg = errMsg
			return nil
		}
		col.NotNull = !nullable
	}
	if len(args) >= 5 && strings.TrimSpace(args[4]) != "" {
		col.HasDefault = true
		col.Default = args[4]
	}
	cols, err := m.connection.DB().TableSchema(table)
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	existing := make([]string, len(cols))
	for i, c := range cols {
		existing[i] = c.Name
	}
	sql, err := db.BuildAddColumnSQL(m.connection.Config().Driver, table, col, existing)
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	return m.execSchemaDDL(table, sql, db.SchemaAddColumn, "")
}

// resolveNameInList picks an entry by EqualFold exact match, then substring.
// Returns "" if nothing matches.
func resolveNameInList(query string, names []string) string {
	if query == "" {
		return ""
	}
	for _, n := range names {
		if strings.EqualFold(n, query) {
			return n
		}
	}
	needle := strings.ToLower(query)
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), needle) {
			return n
		}
	}
	return ""
}

// resolveConnectionName resolves a user-typed connection name against config.
func (m *Model) resolveConnectionName(query string) string {
	if m.config == nil {
		return ""
	}
	names := make([]string, 0, len(m.config.Connections))
	for _, c := range m.config.Connections {
		names = append(names, c.Name)
	}
	return resolveNameInList(query, names)
}

// exConnections opens the connection list (:connections), sharing
// showConnectionList with ctrl+t.
func (m *Model) exConnections() tea.Cmd {
	return m.showConnectionList()
}

// exConnect switches to a named connection (:connect [name] / :c). With no
// argument it opens the connection list like :connections.
func (m *Model) exConnect(args []string) tea.Cmd {
	if len(args) == 0 {
		return m.showConnectionList()
	}
	if m.config == nil || len(m.config.Connections) == 0 {
		m.schemaMsg = "no connections configured"
		return nil
	}
	resolved := m.resolveConnectionName(args[0])
	if resolved == "" {
		m.schemaMsg = fmt.Sprintf("no such connection: %s", args[0])
		return nil
	}
	m.connError = ""
	cmd := m.connectByName(resolved)
	if m.connError != "" {
		m.schemaMsg = m.connError
		m.connError = ""
		return nil
	}
	if !m.dbPicker.IsVisible() {
		m.schemaMsg = "connected: " + resolved
	}
	return cmd
}

// exReconnect rebuilds the active MySQL/Postgres connection (and SSH tunnel)
// in place so a dropped session does not kick the user back to the picker.
func (m *Model) exReconnect() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if !m.needsKeepAlive() {
		m.schemaMsg = "reconnect is for MySQL/Postgres connections"
		return nil
	}
	if m.reconnecting {
		m.schemaMsg = "already reconnecting…"
		return nil
	}
	m.reconnectRetry = false
	return m.reconnectInPlace()
}

// exDB lists or switches databases (:db / :use [database]). Bare opens the
// picker (ctrl+b); with an argument switches directly. SQLite gets an explicit
// message rather than a silent no-op.
func (m *Model) exDB(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	driver := m.connection.Config().Driver
	if driver != db.DriverMySQL && driver != db.DriverPostgres {
		m.schemaMsg = "switching databases is not supported for " + string(driver)
		return nil
	}
	if len(args) == 0 {
		return m.openDatabasePicker(false)
	}
	dbs, err := m.connection.DB().Databases()
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	name := resolveNameInList(args[0], dbs)
	if name == "" {
		m.schemaMsg = fmt.Sprintf("no such database: %s", args[0])
		return nil
	}
	m.connError = ""
	cmd := m.selectDatabase(name)
	if m.connError != "" {
		m.schemaMsg = m.connError
		m.connError = ""
		return nil
	}
	m.schemaMsg = "database: " + name
	return cmd
}

// exSchema lists or switches schemas (:schema [name]). MySQL equates schema
// with database and delegates to :db. Postgres lists/switches search_path.
// SQLite is unsupported.
func (m *Model) exSchema(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	switch m.connection.Config().Driver {
	case db.DriverMySQL:
		return m.exDB(args)
	case db.DriverSQLite:
		m.schemaMsg = "schemas are not supported for sqlite"
		return nil
	case db.DriverPostgres:
		schemas, err := m.connection.Schemas()
		if err != nil {
			m.schemaMsg = err.Error()
			return nil
		}
		if len(args) == 0 {
			if len(schemas) == 0 {
				m.schemaMsg = "no schemas"
				return nil
			}
			cur := m.connection.Config().Schema
			parts := make([]string, 0, len(schemas))
			for _, s := range schemas {
				if cur != "" && strings.EqualFold(s, cur) {
					parts = append(parts, "["+s+"]")
				} else {
					parts = append(parts, s)
				}
			}
			m.schemaMsg = strings.Join(parts, "  ")
			return nil
		}
		name := resolveNameInList(args[0], schemas)
		if name == "" {
			m.schemaMsg = fmt.Sprintf("no such schema: %s", args[0])
			return nil
		}
		m.connError = ""
		cmd := m.selectSchema(name)
		if m.connError != "" {
			m.schemaMsg = m.connError
			m.connError = ""
			return nil
		}
		m.schemaMsg = "schema: " + name
		return cmd
	default:
		m.schemaMsg = "schemas are not supported for " + string(m.connection.Config().Driver)
		return nil
	}
}

// exCreateDatabase creates a database (:createdb <name>), sharing
// execCreateDatabase with the db-picker N key. MySQL/Postgres only; SQLite is
// unsupported (a single file is the database). DDL, so blocked in read-only
// mode and while a transaction is open (it would implicitly commit on
// MySQL/Postgres).
func (m *Model) exCreateDatabase(name string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	driver := m.connection.Config().Driver
	if driver != db.DriverMySQL && driver != db.DriverPostgres {
		m.schemaMsg = "create database is not supported for " + string(driver)
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: create database disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		m.schemaMsg = ":createdb needs a database name"
		return nil
	}
	return m.execCreateDatabase(name)
}

// exDropDatabase drops a database (:dropdb[!] [name]), sharing execDropDatabase
// with the db-picker D key. Defaults to the current database when no name is
// given. Stages the typed-name confirmation when confirm_destructive is on,
// unless forced (:dropdb!). MySQL/Postgres only.
func (m *Model) exDropDatabase(name string, force bool) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	driver := m.connection.Config().Driver
	if driver != db.DriverMySQL && driver != db.DriverPostgres {
		m.schemaMsg = "drop database is not supported for " + string(driver)
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: drop database disabled"
		return nil
	}
	if m.txnBlocksWrite() {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = m.connection.Config().Database
	}
	if name == "" {
		m.schemaMsg = ":dropdb needs a database name"
		return nil
	}
	if !force && m.confirmDestructive() {
		m.dropDBConfirm = name
		m.dropDBInput = ""
		return nil
	}
	return m.execDropDatabase(name)
}

// maxRecentTables caps the in-memory MRU list backing :recent.
const maxRecentTables = 20

// touchRecentTable records name at the front of the MRU list (deduped).
func (m *Model) touchRecentTable(name string) {
	if name == "" {
		return
	}
	out := make([]string, 0, len(m.recentTables)+1)
	out = append(out, name)
	for _, t := range m.recentTables {
		if !strings.EqualFold(t, name) {
			out = append(out, t)
		}
	}
	if len(out) > maxRecentTables {
		out = out[:maxRecentTables]
	}
	m.recentTables = out
}

// openTable browses a table via SELECT * FROM — shared by :goto, sidebar
// enter, mouse open, and :recent <n>. Records the table in the MRU list.
func (m *Model) openTable(name string) tea.Cmd {
	m.touchRecentTable(name)
	m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s;", name))
	return m.executeQuery()
}

// liveRecentTables returns recent table names that still exist in m.tables,
// preserving MRU order.
func (m Model) liveRecentTables() []string {
	if len(m.recentTables) == 0 {
		return nil
	}
	alive := make(map[string]string, len(m.tables)) // lower → canonical
	for _, t := range m.tables {
		alive[strings.ToLower(t)] = t
	}
	var out []string
	for _, t := range m.recentTables {
		if canon, ok := alive[strings.ToLower(t)]; ok {
			out = append(out, canon)
		}
	}
	return out
}

// exGoto opens a table by name (:goto users): exact (case-insensitive) match
// first, then a substring fallback, then runs SELECT * FROM <table>.
func (m *Model) exGoto(name string) tea.Cmd {
	items := m.sidebarItems()
	target := -1
	for i, it := range items {
		if !it.isColumn && strings.EqualFold(it.text, name) {
			target = i
			break
		}
	}
	if target < 0 {
		needle := strings.ToLower(name)
		for i, it := range items {
			if !it.isColumn && strings.Contains(strings.ToLower(it.text), needle) {
				target = i
				break
			}
		}
	}
	if target < 0 {
		m.schemaMsg = fmt.Sprintf("no such table: %s", name)
		return nil
	}
	m.sidebarCursor = target
	m.sidebarViewAnchored = false
	return m.openTable(items[target].text)
}

// exBegin starts a manual transaction (:begin [isolation]). While it is
// active, statements run from the editor execute on the tx — so SELECTs see
// the tx's own uncommitted writes — and :commit / :rollback finish it. Cell
// edits / inserts / deletes / DDL are refused for the duration (they use their
// own autocommit path and would commit outside the tx). Refused while
// read-only, while a query is in flight, or when a transaction is already open.
//
// Isolation is optional: `:begin`, `:begin serializable`, `:begin repeatable
// read`, `:begin read committed`, `:begin read uncommitted` (also hyphenated
// or short forms: rr, rc, s, ru). SQLite is effectively always serializable.
//
// begin/commit/rollback run synchronously: they're rare, explicit actions
// that transfer no row data, and doing them inline (rather than as a goroutine
// command) keeps the tx lifecycle single-threaded. The queryRunning guard
// ensures no in-flight query goroutine is touching the tx when they run.
func (m *Model) exBegin(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: transactions disabled"
		return nil
	}
	if m.tx != nil {
		m.schemaMsg = "transaction already in progress — use :commit or :rollback"
		return nil
	}
	if m.queryRunning {
		m.schemaMsg = "wait for the running query to finish"
		return nil
	}
	level, err := db.ParseIsolation(strings.Join(args, " "))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	tx, err := m.connection.DB().Begin(level)
	if err != nil {
		m.schemaMsg = "begin failed: " + err.Error()
		return nil
	}
	m.tx = tx
	m.txIsolation = level
	if level == db.IsolationDefault {
		m.schemaMsg = "transaction started — :commit or :rollback to finish"
	} else {
		m.schemaMsg = fmt.Sprintf("transaction started (%s) — :commit or :rollback to finish", level)
	}
	return nil
}

// exCommit commits the active manual transaction (:commit). The displayed
// results are left as-is; re-run a SELECT (or ctrl+r) to see the committed
// state. We deliberately do NOT auto-re-run the statement under the cursor —
// after a write that would re-execute it outside the tx (a double apply).
func (m *Model) exCommit() tea.Cmd {
	if m.tx == nil {
		m.schemaMsg = "no transaction in progress"
		return nil
	}
	if m.queryRunning {
		m.schemaMsg = "wait for the running query to finish"
		return nil
	}
	if err := m.tx.Commit(); err != nil {
		m.schemaMsg = "commit failed: " + err.Error()
		m.tx = nil // don't reuse a (likely) dead tx
		m.txIsolation = db.IsolationDefault
		return nil
	}
	m.tx = nil
	m.txIsolation = db.IsolationDefault
	m.schemaMsg = "transaction committed"
	return nil
}

// exRollback discards the active manual transaction (:rollback).
func (m *Model) exRollback() tea.Cmd {
	if m.tx == nil {
		m.schemaMsg = "no transaction in progress"
		return nil
	}
	if m.queryRunning {
		m.schemaMsg = "wait for the running query to finish"
		return nil
	}
	if err := m.tx.Rollback(); err != nil {
		m.schemaMsg = "rollback failed: " + err.Error()
		m.tx = nil
		m.txIsolation = db.IsolationDefault
		return nil
	}
	m.tx = nil
	m.txIsolation = db.IsolationDefault
	m.schemaMsg = "transaction rolled back"
	return nil
}

// txnBlocksWrite reports whether a manual transaction is active and, if so,
// sets a transient status-bar message explaining why the write was refused.
// Cell edits, inserts, deletes, and DDL each use their own autocommit path; if
// allowed during a transaction they'd commit outside it, surprising the user
// (and on MySQL/PG, DDL would implicitly commit the whole tx). Blocking them
// keeps transaction semantics honest.
func (m *Model) txnBlocksWrite() bool {
	if m.tx != nil {
		m.schemaMsg = "transaction active — :commit or :rollback before editing"
		return true
	}
	return false
}

// exEditFile loads a .sql (or any text) file into the editor (:e <file>),
// replacing the current buffer — vim's :edit. The contents are not executed;
// run them from the editor as usual (statements are split at run time). ~ is
// expanded; relative paths resolve against the working directory.
func (m *Model) exEditFile(path string) tea.Cmd {
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	content, err := os.ReadFile(expanded)
	if err != nil {
		m.schemaMsg = "read failed: " + err.Error()
		return nil
	}
	m.editor.SetValue(string(content))
	m.schemaMsg = fmt.Sprintf("loaded %s (%d lines)", expanded, lineCount(string(content)))
	return nil
}

// loadStartupFile reads a .sql file into the editor for the `creel -f` startup
// flag — the non-interactive counterpart of :e. It expands ~ and resolves
// relative paths against the working directory (same as :e), returning the
// expanded path on success or the read error otherwise. Run fails fast on the
// error (a missing/unreadable file is almost always a typo); the loaded script
// is not executed — the user reviews it in the editor and runs it (ctrl+e).
func (m *Model) loadStartupFile(path string) (string, error) {
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(expanded)
	if err != nil {
		return expanded, err
	}
	m.editor.SetValue(string(content))
	// A startup file is an explicit request to review this buffer, so the
	// first connect should not clobber it with a restored session.
	m.startupFileLoaded = true
	return expanded, nil
}

// exSession manages the saved workspace session for the current connection +
// database (:session clear | :session save | :session). "clear" drops the
// persisted snapshot so the next reconnect starts fresh — the live workspace
// is untouched, and it re-saves on the next quit/teardown, so it is not gated
// behind confirm_destructive. "save" snapshots now (without quitting); bare
// reports whether a session is stored and how many tabs it holds.
func (m *Model) exSession(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	conn, database, ok := m.sessionKey()
	if !ok || m.sessionStore == nil {
		m.schemaMsg = "no active session"
		return nil
	}
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch sub {
	case "clear", "off", "reset", "delete":
		if err := m.sessionStore.Clear(conn, database); err != nil {
			m.schemaMsg = "session: " + err.Error()
			return nil
		}
		m.colWidthMem = nil
		m.erdPosMem = nil
		m.schemaMsg = "session cleared — reconnect will start fresh"
		return nil
	case "save":
		m.saveSession()
		m.schemaMsg = "session saved"
		return nil
	case "", "status", "show":
		st, err := m.sessionStore.Load(conn, database)
		if err != nil {
			m.schemaMsg = "session: " + err.Error()
			return nil
		}
		if !st.HasContent() {
			m.schemaMsg = "no saved session"
		} else {
			active := st.Active
			if active < 0 || active >= len(st.Tabs) {
				active = 0
			}
			m.schemaMsg = fmt.Sprintf("session: %d tab(s) saved, active %d", len(st.Tabs), active+1)
		}
		return nil
	}
	m.schemaMsg = "usage: :session [clear|save]"
	return nil
}

// exWriteFile writes the editor buffer to a file (:w <file>) — vim's :write.
// It overwrites an existing file (use a versioned name if you need to keep the
// old one). ~ is expanded; relative paths resolve against the working dir.
func (m *Model) exWriteFile(path string) tea.Cmd {
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	content := m.editor.Value()
	if err := os.WriteFile(expanded, []byte(content), 0o644); err != nil {
		m.schemaMsg = "write failed: " + err.Error()
		return nil
	}
	m.schemaMsg = fmt.Sprintf("wrote %s (%d lines)", expanded, lineCount(content))
	return nil
}

// exSaveBlob writes the binary value under the results cursor to a file
// (:saveblob <file>). Binary cells are scanned as []byte and shown as
// "<BLOB …>" placeholders; this is how you recover the raw bytes.
func (m *Model) exSaveBlob(path string) tea.Cmd {
	row, col := m.results.CursorRow(), m.results.CursorCol()
	data, ok := m.results.BlobData(row, col)
	if !ok {
		m.schemaMsg = "cursor cell is not a binary value"
		return nil
	}
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	if err := os.WriteFile(expanded, data, 0o644); err != nil {
		m.schemaMsg = "write failed: " + err.Error()
		return nil
	}
	m.schemaMsg = fmt.Sprintf("wrote %s to %s", db.FormatByteSize(len(data)), expanded)
	return nil
}

// exExport writes the current result set to ~/Downloads in the given format
// (:export <fmt> [cols...]) — a non-interactive shortcut over the g X export
// dialog. <fmt> is one of csv, json, jsonl, md, tsv (case-insensitive; "markdown"
// and "json lines" are accepted). Optional trailing arguments name the columns
// to export (comma-separated within one arg or across args, e.g.
// `:export csv name,email`); when omitted, all columns are exported. The row
// scope defaults sensibly (marked rows if any, else whole table, else page);
// use the g X dialog to choose scope explicitly. It reuses exportResults, so
// feedback flows through the same export status message.
func (m *Model) exExport(args []string) tea.Cmd {
	if len(args) == 0 {
		m.schemaMsg = ":export needs a format: csv, json, jsonl, md, tsv"
		return nil
	}
	format, ok := parseExportFormat(args[0])
	if !ok {
		m.schemaMsg = ":export needs a format: csv, json, jsonl, md, tsv"
		return nil
	}
	var cols []string
	for _, a := range args[1:] {
		for _, c := range strings.Split(a, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				cols = append(cols, c)
			}
		}
	}
	return m.exportResults(format, cols, m.defaultExportScope())
}

// resolveTableName finds the canonical (sidebar) name for a table the user
// typed: an exact case-insensitive match first, then a substring fallback.
// Returns "" if nothing matches. Shared by ex commands that take a table arg.
func (m Model) resolveTableName(name string) string {
	items := m.sidebarItems()
	for _, it := range items {
		if !it.isColumn && strings.EqualFold(it.text, name) {
			return it.text
		}
	}
	needle := strings.ToLower(name)
	for _, it := range items {
		if !it.isColumn && strings.Contains(strings.ToLower(it.text), needle) {
			return it.text
		}
	}
	return ""
}

// resolveTableArg resolves an optional table argument for an ex command, shared
// by :refs and :uses (and future table-targeted lookups). With no name it
// falls back to the current table — the focused sidebar selection or the
// results' source table (default-to-current-object convention, #15). With a
// name it resolves case-insensitively against the sidebar (exact match, then
// substring). On failure it sets schemaMsg and returns "".
func (m *Model) resolveTableArg(name string) string {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return ""
	}
	if name == "" {
		if t := m.currentTable(); t != "" {
			return t
		}
		m.schemaMsg = "no current table — name one: e.g. :refs <table>"
		return ""
	}
	if resolved := m.resolveTableName(name); resolved != "" {
		return resolved
	}
	m.schemaMsg = fmt.Sprintf("no such table: %s", name)
	return ""
}

// resolveDDLTableArg resolves an optional table for sidebar-mirrored DDL
// (:truncate / :drop / :rename). Unlike resolveTableArg, a bare command prefers
// the sidebar cursor (sidebarSelectedTable) — matching the T/D/r keys — even
// when results still show a different SourceTable or focus is not the sidebar.
func (m *Model) resolveDDLTableArg(name string) string {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return ""
	}
	if name != "" {
		if resolved := m.resolveTableName(name); resolved != "" {
			return resolved
		}
		m.schemaMsg = fmt.Sprintf("no such table: %s", name)
		return ""
	}
	if t := m.sidebarSelectedTable(); t != "" {
		return t
	}
	if t := m.currentTable(); t != "" {
		return t
	}
	m.schemaMsg = "no current table — name one"
	return ""
}

// exRefs lists the foreign keys referencing a table (:refs <table>) — the
// reverse of g d. When the results grid is backing the same table and a row is
// focused, it additionally shows a per-referrer Count column computed against
// that row's value ("Orders (14)"), turning the relationship list into a live,
// countable inbound view — the first step toward the row-explorer fan-out.
// The lookup runs async and opens in the lookup overlay panel. It reads
// connection metadata, so it is unaffected by (and does not block on) an
// active transaction.
func (m *Model) exRefs(name string) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	conn := m.connection
	driver := conn.Config().Driver

	// Capture the focused row's column→value map on the main goroutine so the
	// async closure can compute per-referrer counts. Counts are only shown when
	// the results grid is backing this same table with a focused row.
	rowVals, label, scoped := m.focusedRowValues(table)

	return func() tea.Msg {
		refs, err := conn.DB().ReferencingForeignKeys(table)
		if err != nil {
			return lookupResultMsg{err: err}
		}

		cols := []db.Column{{Name: "Table"}, {Name: "Column"}, {Name: "References"}}
		if scoped {
			cols = append(cols, db.Column{Name: "Count"})
		}
		rows := make([][]string, len(refs))

		if scoped {
			// Fan out count queries concurrently — database/sql pools
			// connections and is safe for parallel use, and each goroutine
			// writes a distinct slice index, so the slice stays race-free.
			var wg sync.WaitGroup
			for i, r := range refs {
				wg.Add(1)
				go func(i int, r db.Referrer) {
					defer wg.Done()
					rows[i] = []string{r.Table, r.Column, table + "." + r.RefColumn, countReferrer(conn, driver, r, rowVals)}
				}(i, r)
			}
			wg.Wait()
		} else {
			for i, r := range refs {
				rows[i] = []string{r.Table, r.Column, table + "." + r.RefColumn}
			}
		}

		return lookupResultMsg{
			title:  "References to " + table + label,
			result: db.Result{Columns: cols, Rows: rows},
		}
	}
}

// focusedRowValues returns the focused results row as a lowercased
// column-name → value map, plus a short " · pk=val" label identifying which
// row the counts are scoped to. ok is false when the grid is not backing
// `table` (or has no focused row), in which case :refs omits the Count column
// and behaves as before. Must run on the main goroutine — it reads live UI
// state.
func (m *Model) focusedRowValues(table string) (vals map[string]string, label string, ok bool) {
	r := m.results
	if !r.HasResult() {
		return nil, "", false
	}
	if src := r.SourceTable(); src == "" || !strings.EqualFold(src, table) {
		return nil, "", false
	}
	row := r.CursorRow()
	if row < 0 || row >= r.NumRows() {
		return nil, "", false
	}
	vals = make(map[string]string, r.NumCols())
	for c := 0; c < r.NumCols(); c++ {
		vals[strings.ToLower(r.ColumnName(c))] = r.RowValue(row, c)
	}
	return vals, pkLabel(r), true
}

// pkLabel builds a " · #val" suffix identifying the focused row, preferring
// the primary key tuple and falling back to the first non-empty cell when
// there is no PK. Empty when no usable value is found.
func pkLabel(r ResultsTable) string {
	if tup := r.CursorPKTuple(); len(tup) > 0 {
		parts := make([]string, 0, len(tup))
		for _, v := range tup {
			if v == "" || v == "NULL" {
				continue
			}
			parts = append(parts, v)
		}
		if len(parts) > 0 {
			return " · #" + strings.Join(parts, ", ")
		}
	}
	for c := 0; c < r.NumCols(); c++ {
		if v := r.RowValue(r.CursorRow(), c); v != "" && v != "NULL" {
			return " · " + r.ColumnName(c) + "=" + v
		}
	}
	return ""
}

// countReferrer returns how many rows in the child table reference the focused
// parent row via this FK. "-" means nothing to match (the parent value is
// absent/NULL), "?" means the count query failed (the row still renders). Uses
// the same string-escaping convention as g d's buildForeignKeyQuery.
func countReferrer(conn *db.Connection, driver db.Driver, ref db.Referrer, rowVals map[string]string) string {
	val, ok := rowVals[strings.ToLower(ref.RefColumn)]
	if !ok || val == "" || val == "NULL" {
		return "-"
	}
	return countRelated(conn, driver, ref.Table, ref.Column, val)
}

// countRelated returns how many rows in `table` have `col` equal to `val`, as
// a string. "?" means the count query failed. Shared by :refs' per-referrer
// counts and the relationship explorer's per-edge counts.
func countRelated(conn *db.Connection, driver db.Driver, table, col, val string) string {
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = %s",
		quoteIdentD(driver, table), quoteIdentD(driver, col), quoteSQLString(val))
	res, err := conn.DB().Execute(q)
	if err != nil {
		return "?"
	}
	if len(res.Rows) == 0 || len(res.Rows[0]) == 0 {
		return "0"
	}
	return res.Rows[0][0]
}

// openDockedExplorer toggles the relationship explorer as a right-slot panel
// (the "inspector-tab" variant): non-modal, sharing the slot with the
// inspector/assistant, and cursor-driven — it re-roots to the focused results
// row as the cursor moves. Invoked by `g r` and `:explore`.
func (m *Model) openDockedExplorer() tea.Cmd {
	if m.explorer.IsVisible() && m.explorer.docked {
		m.closeDockedExplorer()
		return nil
	}
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	// The right slot holds one panel: close inspector/assistant.
	m.inspector.Hide()
	m.assistant.Hide()
	m.explorer.ShowDocked()
	m.explorer.markLoading()
	m.focus = FocusExplorer
	m.layoutWorkspace()
	m.applyFocus()
	return m.loadExplorer()
}

// closeDockedExplorer hides the docked explorer panel and returns focus to the
// results grid.
func (m *Model) closeDockedExplorer() {
	m.explorer.Hide()
	if m.focus == FocusExplorer {
		m.focus = FocusResults
	}
	m.layoutWorkspace()
	m.applyFocus()
}

// explorerAnchor is the identity of the results row the explorer would root at
// (source table + PK tuple), or "" when there is nothing to anchor to. Used to
// detect cursor moves so the docked panel can re-root without redundant loads.
func (m Model) explorerAnchor() string {
	r := m.results
	if !r.HasResult() {
		return ""
	}
	src := r.SourceTable()
	if src == "" {
		return ""
	}
	tup := r.CursorPKTuple()
	if len(tup) == 0 {
		return ""
	}
	return src + "|" + strings.Join(tup, ",")
}

// maybeReloadDockedExplorer re-roots the docked explorer when the results
// cursor has landed on a different row. It does not markLoading, so the
// previous tree stays visible until the new root arrives (no flicker on every
// cursor move).
func (m *Model) maybeReloadDockedExplorer() tea.Cmd {
	if !m.explorer.IsVisible() {
		return nil
	}
	cur := m.explorerAnchor()
	if cur == "" || cur == m.explorer.anchor {
		m.syncExplorerFKHighlight()
		return nil
	}
	return m.loadExplorer()
}

// syncExplorerFKHighlight marks the explorer edge that matches the results
// cursor's FK column, and — when the grid has focus — moves the explorer
// cursor onto it. Called after a row re-root and on every results cursor move
// that doesn't change the row.
func (m *Model) syncExplorerFKHighlight() {
	col, ref := "", ""
	if fk, ok := m.results.ForeignKeyAtCursor(); ok {
		col, ref = fk.Column, fk.RefTable
	}
	m.explorer.HighlightLinked(col, ref, m.focus == FocusResults)
}

// loadExplorer builds the explorer tree root from the focused results row and
// loads its first-level edges (with counts), returning explorerLoadedMsg. It
// powers openDockedExplorer and the auto-refresh after Enter/back (wired in
// app.go on queryExecutedMsg). When there is nothing to explore it returns a
// message with an emptyMsg so the panel shows a reason rather than a blank box.
func (m *Model) loadExplorer() tea.Cmd {
	r := m.results
	depth := len(m.queryStack)
	if m.connection == nil {
		return explorerMsg(explorerLoadedMsg{depth: depth, emptyMsg: "not connected"})
	}
	if !r.HasResult() {
		return explorerMsg(explorerLoadedMsg{depth: depth, emptyMsg: "no results — run a query first"})
	}
	src := r.SourceTable()
	if src == "" {
		return explorerMsg(explorerLoadedMsg{depth: depth, emptyMsg: "current results are not a single table — browse a table to explore"})
	}
	rowVals, label, ok := m.focusedRowValues(src)
	if !ok {
		return explorerMsg(explorerLoadedMsg{depth: depth, emptyMsg: "no focused row — select a row to explore its relationships"})
	}
	conn := m.connection
	driver := conn.Config().Driver
	return func() tea.Msg {
		root := &expNode{kind: nodeRow, table: src, rowVals: rowVals, label: label, expanded: true}
		edges, err := loadRowEdges(conn, driver, src, rowVals, nil)
		if err != nil {
			return explorerLoadedMsg{depth: depth, err: err}
		}
		for _, e := range edges {
			e.parent = root
			e.depth = 1
		}
		root.children = edges
		if len(edges) == 0 {
			root.children = []*expNode{synthNode(src+" has no relationships", root)}
		}
		return explorerLoadedMsg{root: root, depth: depth}
	}
}

// explorerMsg wraps a pre-resolved explorerLoadedMsg as a no-op command, for
// the synchronous empty/error paths of loadExplorer.
func explorerMsg(msg explorerLoadedMsg) tea.Cmd {
	return func() tea.Msg { return msg }
}

// loadExplorerChildren lazily loads a node's children, returning
// explorerChildrenMsg: for an edge node it loads the related rows (capped at
// explorerChildLimit); for a row node it loads that row's edges. Depth-capped
// nodes get a synthetic marker instead of a load.
func (m *Model) loadExplorerChildren(node *expNode) tea.Cmd {
	if node == nil {
		return nil
	}
	if node.depth >= maxExplorerDepth {
		return func() tea.Msg {
			return explorerChildrenMsg{parent: node, children: []*expNode{synthNode("(depth limit reached)", node)}}
		}
	}
	conn := m.connection
	driver := conn.Config().Driver
	if node.isEdge() {
		val := node.filterVal
		full := edgeDrillQuery(driver, node.edge, val)
		q := full + fmt.Sprintf(" LIMIT %d", explorerChildLimit+1)
		tbl := node.edge.targetTable
		// Collect every row already on the path from the root to this edge so
		// we can suppress children that would re-enter it. FK graphs cycle
		// (users → orders → users), and without this the drill-down re-shows
		// the row you started at, ad infinitum (until maxExplorerDepth).
		ancestors := ancestorIdentities(node)
		return func() tea.Msg {
			res, err := conn.DB().Execute(q)
			if err != nil {
				return explorerChildrenMsg{parent: node, err: err}
			}
			pkCols, _ := conn.DB().PrimaryKeys(tbl)
			rows := buildChildRowNodes(res, tbl, pkCols, driver)
			// Drop child rows that are already ancestors (cycle break).
			cycled := 0
			if len(ancestors) > 0 {
				kept := rows[:0]
				for _, r := range rows {
					if ancestors[rowIdentity(tbl, r.rowVals)] {
						cycled++
						continue
					}
					kept = append(kept, r)
				}
				rows = kept
			}
			if len(res.Rows) > explorerChildLimit {
				more := synthNode(fmt.Sprintf("(+%d more — enter to open in grid)", len(res.Rows)-explorerChildLimit), node)
				more.drillQuery = full // full, unlimited set
				if len(rows) > explorerChildLimit {
					rows = append(rows[:explorerChildLimit], more)
				} else {
					rows = append(rows, more)
				}
			}
			switch {
			case len(rows) == 0 && cycled > 0:
				return explorerChildrenMsg{parent: node, fold: true}
			case len(rows) == 0:
				rows = []*expNode{synthNode("(no rows)", node)}
			}
			return explorerChildrenMsg{parent: node, children: rows}
		}
	}
	// row node: load its edges, omitting outbound edges that loop back to a row
	// already on the path (so e.g. a child row's FK to its parent isn't shown).
	ancestors := ancestorRowNodes(node)
	return func() tea.Msg {
		edges, err := loadRowEdges(conn, driver, node.table, node.rowVals, ancestors)
		if err != nil {
			return explorerChildrenMsg{parent: node, err: err}
		}
		if len(edges) == 0 {
			// Nothing to list (no FKs, or every edge was a back-edge): fold the
			// node back up rather than show a "(no relationships)" marker.
			return explorerChildrenMsg{parent: node, fold: true}
		}
		return explorerChildrenMsg{parent: node, children: edges}
	}
}

// loadRowEdges fetches a row's outbound + inbound FK edges and resolves a live
// per-edge count, returning edge nodes ready to attach as a row's children.
// The count fan-out is concurrent (one goroutine per edge, distinct index).
func loadRowEdges(conn *db.Connection, driver db.Driver, table string, rowVals map[string]string, ancestors []*expNode) ([]*expNode, error) {
	out, errO := conn.DB().ForeignKeys(table)
	in, errI := conn.DB().ReferencingForeignKeys(table)
	if errO != nil && errI != nil {
		return nil, errO
	}
	edges := make([]*expNode, 0, len(out)+len(in))
	for _, fk := range out {
		if errO != nil {
			continue
		}
		val := rowVals[strings.ToLower(fk.Column)]
		e := &expNode{
			kind:      nodeEdge,
			edge:      relNode{dir: relOutbound, targetTable: fk.RefTable, targetColumn: fk.RefColumn, sourceColumn: fk.Column},
			filterVal: val,
		}
		e.drillQuery = edgeDrillQuery(driver, e.edge, val)
		edges = append(edges, e)
	}
	for _, rf := range in {
		if errI != nil {
			continue
		}
		val := rowVals[strings.ToLower(rf.RefColumn)]
		e := &expNode{
			kind:      nodeEdge,
			edge:      relNode{dir: relInbound, targetTable: rf.Table, targetColumn: rf.Column, sourceColumn: rf.RefColumn},
			filterVal: val,
		}
		e.drillQuery = edgeDrillQuery(driver, e.edge, val)
		edges = append(edges, e)
	}
	var wg sync.WaitGroup
	for i := range edges {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			val := edges[i].filterVal
			if val == "" || val == "NULL" {
				edges[i].edge.count = "-"
				return
			}
			edges[i].edge.count = countRelated(conn, driver, edges[i].edge.targetTable, edges[i].edge.targetColumn, val)
		}(i)
	}
	wg.Wait()

	// Keep only edges worth showing:
	//   - outbound: positive count, and not a back-edge to an ancestor
	//   - inbound: any resolved numeric count, including 0 — empty child
	//     relations stay visible so "insert related" (A) has a target
	kept := edges[:0]
	for _, e := range edges {
		if !isNumericCount(e.edge.count) {
			continue
		}
		c, _ := strconv.Atoi(e.edge.count)
		if e.edge.dir == relOutbound {
			if c <= 0 {
				continue
			}
			if isOutboundBackEdge(e, ancestors) {
				continue
			}
		}
		// inbound: keep even when c == 0
		kept = append(kept, e)
	}
	return kept, nil
}

// isOutboundBackEdge reports whether an outbound edge's target row is already
// an ancestor on the path from the root — i.e. expanding it would only loop
// back to a row already shown. Outbound FK targets are unique (a PK or unique
// column), so a matching ancestor means there is nothing new to drill into.
func isOutboundBackEdge(e *expNode, ancestors []*expNode) bool {
	col := strings.ToLower(e.edge.targetColumn)
	for _, a := range ancestors {
		if a.table == e.edge.targetTable && a.rowVals[col] == e.filterVal {
			return true
		}
	}
	return false
}

// edgeDrillQuery builds the SELECT to open an edge's full related set in the
// grid (Enter on an edge node). An absent value falls back to an unfiltered
// browse of the target table.
func edgeDrillQuery(driver db.Driver, edge relNode, val string) string {
	tbl := quoteIdentD(driver, edge.targetTable)
	col := quoteIdentD(driver, edge.targetColumn)
	if val == "" || val == "NULL" {
		return fmt.Sprintf("SELECT * FROM %s", tbl)
	}
	return fmt.Sprintf("SELECT * FROM %s WHERE %s = %s", tbl, col, quoteSQLString(val))
}

// buildChildRowNodes turns a SELECT result set into row nodes, one per row,
// each carrying its column→value map, a display label, and a drill query to
// open that exact row in the grid on Enter.
func buildChildRowNodes(res db.Result, table string, pkCols []string, driver db.Driver) []*expNode {
	cols := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		cols[i] = c.Name
	}
	nodes := make([]*expNode, 0, len(res.Rows))
	for ri, row := range res.Rows {
		vals := make(map[string]string, len(cols))
		for i := range cols {
			if i < len(row) {
				vals[strings.ToLower(cols[i])] = row[i]
			}
		}
		nodes = append(nodes, &expNode{
			kind:       nodeRow,
			table:      table,
			rowVals:    vals,
			label:      rowLabel(cols, vals, pkCols, ri),
			drillQuery: rowDrillQuery(driver, table, pkCols, vals),
		})
	}
	return nodes
}

// rowIdentity returns a canonical key for one row, used only to detect FK
// cycles in the explorer tree: a child row that matches a row already on the
// path from the root is suppressed so drilling never re-shows the row you
// started from. Two rows are "the same" exactly when every cell matches.
func rowIdentity(table string, vals map[string]string) string {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(table)
	b.WriteByte('|')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(vals[k])
	}
	return b.String()
}

// ancestorIdentities collects the row identities along the path from the root
// down to (but excluding) the given edge node — every row the explorer has
// already expanded through to reach this edge. Used to break FK cycles.
func ancestorIdentities(edge *expNode) map[string]bool {
	set := map[string]bool{}
	for p := edge.parent; p != nil; p = p.parent {
		if p.kind == nodeRow && !p.synthetic && p.rowVals != nil {
			set[rowIdentity(p.table, p.rowVals)] = true
		}
	}
	return set
}

// ancestorRowNodes returns the row nodes on the path from the root down to (but
// excluding) the given node, nearest-first. Used to omit outbound edges that
// would loop back to a row already shown.
func ancestorRowNodes(node *expNode) []*expNode {
	var out []*expNode
	for p := node.parent; p != nil; p = p.parent {
		if p.kind == nodeRow && !p.synthetic && p.rowVals != nil {
			out = append(out, p)
		}
	}
	return out
}

// rowLabel builds a human-friendly identity for a child row: the PK tuple
// ("#1001") if available, else the first non-empty cell, else a positional
// fallback rendered as "#N".
func rowLabel(cols []string, vals map[string]string, pkCols []string, idx int) string {
	if len(pkCols) > 0 {
		parts := make([]string, 0, len(pkCols))
		ok := true
		for _, pk := range pkCols {
			v := vals[strings.ToLower(pk)]
			if v == "" || v == "NULL" {
				ok = false
				break
			}
			parts = append(parts, v)
		}
		if ok && len(parts) > 0 {
			return "#" + strings.Join(parts, ", ")
		}
	}
	for _, c := range cols {
		v := vals[strings.ToLower(c)]
		if v != "" && v != "NULL" {
			return c + "=" + v
		}
	}
	return fmt.Sprintf("#%d", idx)
}

// rowDrillQuery builds the SELECT to open one exact row in the grid on Enter.
// Returns "" when the table has no PK (Enter is then disabled for that node).
func rowDrillQuery(driver db.Driver, table string, pkCols []string, vals map[string]string) string {
	if len(pkCols) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pkCols))
	for _, pk := range pkCols {
		v := vals[strings.ToLower(pk)]
		if v == "" || v == "NULL" {
			return "" // incomplete PK — can't address the row uniquely
		}
		parts = append(parts, fmt.Sprintf("%s = %s", quoteIdentD(driver, pk), quoteSQLString(v)))
	}
	return fmt.Sprintf("SELECT * FROM %s WHERE %s", quoteIdentD(driver, table), strings.Join(parts, " AND "))
}

// exUses lists the objects (views, functions, procedures, triggers) that
// reference a table in their definitions (:uses <table>) — a textual
// dependency scan, complementary to :refs. Same conventions as exRefs:
// default to the current table, async, transaction-unaffected.
func (m *Model) exUses(name string) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		uses, err := conn.DB().Uses(table)
		if err != nil {
			return lookupResultMsg{err: err}
		}
		cols := []db.Column{{Name: "Type"}, {Name: "Name"}}
		rows := make([][]string, 0, len(uses))
		for _, u := range uses {
			rows = append(rows, []string{u.Kind, u.Name})
		}
		return lookupResultMsg{
			title:  fmt.Sprintf("Objects using %s", table),
			result: db.Result{Columns: cols, Rows: rows},
		}
	}
}

// exDescribe opens the structure view for a table (:describe [table]),
// defaulting to the current table. It reuses resolveTableArg (so it works
// unqualified on the focused/last-queried table) and points the sidebar at
// the resolved table so openSchemaPanel — which reads the sidebar selection —
// targets it. The d key does the same for the sidebar cursor; this adds a
// name-addressable path.
func (m *Model) exDescribe(name string) tea.Cmd {
	return m.exOpenStructureTab(name, seTabColumns)
}

// exOpenStructureTab opens the structure panel on a specific tab for a table
// (:columns / :indexes / :fk / :constraints / :describe). Shares openSchemaPanel
// with the d key and :describe.
func (m *Model) exOpenStructureTab(name string, tab int) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	m.syncSidebarCursorToTable(table)
	cmd := m.openSchemaPanel()
	if m.schemaEditor.IsVisible() {
		m.schemaEditor.SetActiveTab(tab)
	}
	return cmd
}

// exListNames shows a single-column lookup overlay of names (tables, views,
// schemas). Shared by :tables / :views / :schemas.
func (m *Model) exListNames(title string, names []string) tea.Cmd {
	cols := []db.Column{{Name: "Name"}}
	rows := make([][]string, 0, len(names))
	for _, n := range names {
		rows = append(rows, []string{n})
	}
	return func() tea.Msg {
		return lookupResultMsg{
			title:  title,
			result: db.Result{Columns: cols, Rows: rows},
		}
	}
}

// exTables lists base tables in the lookup overlay (:tables / :dt). Views are
// excluded when Views() succeeds so the two list verbs stay distinct.
func (m *Model) exTables() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	viewSet := map[string]bool{}
	if views, err := m.connection.DB().Views(); err == nil {
		for _, v := range views {
			viewSet[v] = true
		}
	}
	names := make([]string, 0, len(m.tables))
	for _, t := range m.tables {
		if !viewSet[t] {
			names = append(names, t)
		}
	}
	if len(names) == 0 {
		m.schemaMsg = "no tables"
		return nil
	}
	return m.exListNames("Tables", names)
}

// exViews lists views in the lookup overlay (:views / :dv).
func (m *Model) exViews() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	views, err := m.connection.DB().Views()
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	if len(views) == 0 {
		m.schemaMsg = "no views"
		return nil
	}
	return m.exListNames("Views", views)
}

// exSchemasList lists schemas/namespaces in the lookup overlay (:schemas).
// Distinct from :schema [name], which switches (or status-lists) the active
// schema. SQLite is unsupported.
func (m *Model) exSchemasList() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	schemas, err := m.connection.Schemas()
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	if len(schemas) == 0 {
		m.schemaMsg = "no schemas"
		return nil
	}
	return m.exListNames("Schemas", schemas)
}

// searchHit is one row in the :search / :find lookup overlay.
type searchHit struct {
	kind   string // "table", "view", "column"
	name   string
	parent string // table for columns; empty otherwise
}

// exSearch fuzzy-finds tables, views, and columns by name (:search / :find).
// Uses cached sidebar metadata (m.tables + columnCache); distinct from the
// results g / regex and from cross-search (cell values).
func (m *Model) exSearch(needle string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if needle == "" {
		m.schemaMsg = ":search needs a name"
		return nil
	}

	var hits []searchHit
	viewSet := map[string]bool{}
	if views, err := m.connection.DB().Views(); err == nil {
		for _, v := range views {
			viewSet[v] = true
		}
	}

	for _, t := range m.tables {
		kind := "table"
		if viewSet[t] {
			kind = "view"
		}
		hits = append(hits, searchHit{kind: kind, name: t})
	}
	for table, cols := range m.columnCache {
		for _, c := range cols {
			hits = append(hits, searchHit{kind: "column", name: c.Name, parent: table})
		}
	}

	ranked := fuzzyRank(needle, hits, func(h searchHit) string {
		if h.parent != "" {
			return h.parent + "." + h.name
		}
		return h.name
	}, nil)
	if len(ranked) == 0 {
		m.schemaMsg = fmt.Sprintf("no matches for %q", needle)
		return nil
	}

	cols := []db.Column{{Name: "Kind"}, {Name: "Name"}, {Name: "Parent"}}
	rows := make([][]string, 0, len(ranked))
	for _, r := range ranked {
		rows = append(rows, []string{r.Item.kind, r.Item.name, r.Item.parent})
	}
	title := fmt.Sprintf("Search: %s", needle)
	return func() tea.Msg {
		return lookupResultMsg{
			title:  title,
			result: db.Result{Columns: cols, Rows: rows},
		}
	}
}

// exStats shows summary statistics for a column (:stats [column]), defaulting
// to the cursor column. With an argument it first moves the cursor to that
// column (case-insensitive exact match), so fetchColumnStats — which reads the
// cursor column — summarizes it.
func (m *Model) exStats(arg string) tea.Cmd {
	if m.results.NumRows() == 0 || m.connection == nil {
		m.schemaMsg = "no results to summarize"
		return nil
	}
	if arg != "" {
		idx := -1
		for i := 0; i < m.results.NumCols(); i++ {
			if strings.EqualFold(m.results.ColumnName(i), arg) {
				idx = i
				break
			}
		}
		if idx < 0 {
			m.schemaMsg = fmt.Sprintf("no such column: %s", arg)
			return nil
		}
		m.results.SetCursor(m.results.CursorRow(), idx)
	}
	return m.fetchColumnStats()
}

// exBar opens a horizontal bar chart in the results slot. Columns come from
// args (`:bar label value [sum|count|avg]`) or from ordered column marks (M).
// One column (named or a single mark) counts distinct values. Duplicate
// labels are grouped. The current page supplies the rows unless force
// (`:bar!`) re-runs lastQuery without the page LIMIT.
func (m *Model) exBar(args []string, force bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}

	labelCol, valueCol, agg, err := m.resolveBarColumns(args)
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	label := m.results.ColumnName(labelCol)
	value := m.results.ColumnName(valueCol)
	title := fmt.Sprintf("bar · %s × %s · %s", label, value, agg)
	emptyErr := "no numeric values in " + value
	if agg == barAggCount && labelCol == valueCol {
		title = fmt.Sprintf("bar · %s · count", label)
		emptyErr = "no values to chart"
	}
	return m.runChart(chartSpec{
		kind:     chartKindBar,
		agg:      agg,
		colNames: []string{label, value},
		title:    title,
		emptyErr: emptyErr,
	}, force)
}

// exFreq opens a frequency bar chart of one column (count of each distinct
// column. `:freq!` re-runs lastQuery without the page LIMIT.
func (m *Model) exFreq(args []string, force bool) tea.Cmd {
	return m.exFreqLike(args, force, chartKindBar, "freq")
}

// exPie opens a pie chart of one column (same counts as :freq). The column
// comes from args, a single column mark, or the cursor column. `:pie!`
// re-runs lastQuery without the page LIMIT.
func (m *Model) exPie(args []string, force bool) tea.Cmd {
	return m.exFreqLike(args, force, chartKindPie, "pie")
}

func (m *Model) exFreqLike(args []string, force bool, kind chartKind, prefix string) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	col, err := m.resolveFreqColumn(args)
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	name := m.results.ColumnName(col)
	return m.runChart(chartSpec{
		kind:     kind,
		agg:      barAggCount,
		colNames: []string{name, name},
		title:    fmt.Sprintf("%s · %s", prefix, name),
		emptyErr: "no values to chart",
	}, force)
}

// exLine opens a line chart in the results slot. Columns come from args
// (`:line x y`) or from the two ordered column marks (M: x, then y).
// `:line!` re-runs lastQuery without the page LIMIT.
func (m *Model) exLine(args []string, force bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	xCol, yCol, err := m.resolveXYColumns(args, "line")
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	xName := m.results.ColumnName(xCol)
	yName := m.results.ColumnName(yCol)
	title := fmt.Sprintf("line · %s × %s", xName, yName)
	return m.runChart(chartSpec{
		kind:     chartKindLine,
		colNames: []string{xName, yName},
		title:    title,
		emptyErr: "no numeric or datetime x/y pairs in " + xName + " × " + yName,
	}, force)
}

// exScatter opens a scatter chart in the results slot. Columns come from
// args (`:scatter x y`) or from the two ordered column marks (M: x, then y).
// `:scatter!` re-runs lastQuery without the page LIMIT.
func (m *Model) exScatter(args []string, force bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	xCol, yCol, err := m.resolveXYColumns(args, "scatter")
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	xName := m.results.ColumnName(xCol)
	yName := m.results.ColumnName(yCol)
	title := fmt.Sprintf("scatter · %s × %s", xName, yName)
	return m.runChart(chartSpec{
		kind:     chartKindScatter,
		colNames: []string{xName, yName},
		title:    title,
		emptyErr: "no numeric or datetime x/y pairs in " + xName + " × " + yName,
	}, force)
}

// exHist opens a histogram of one numeric column. The column comes from
// args, a single column mark, or the cursor column. bins is optional
// (Sturges, clamped 8–20). `:hist!` re-runs lastQuery without the page LIMIT.
func (m *Model) exHist(args []string, force bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	col, bins, err := m.resolveHistColumn(args)
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	name := m.results.ColumnName(col)
	title := fmt.Sprintf("hist · %s", name)
	if bins > 0 {
		title += fmt.Sprintf(" · %d bins", bins)
	}
	return m.runChart(chartSpec{
		kind:     chartKindHist,
		colNames: []string{name},
		bins:     bins,
		title:    title,
		emptyErr: "no numeric values in " + name,
	}, force)
}

// resolveXYColumns picks x/y column indices from :line / :scatter args or
// from ordered column marks.
func (m *Model) resolveXYColumns(args []string, verb string) (xCol, yCol int, err string) {
	find := m.resultColumnIndex
	switch len(args) {
	case 0:
		marked := m.results.MarkedColumns()
		if len(marked) != 2 {
			return 0, 0, fmt.Sprintf("mark 2 columns with M (x, then y), or :%s <x> <y>", verb)
		}
		return marked[0], marked[1], ""
	case 1:
		return 0, 0, fmt.Sprintf("usage: :%s <x> <y> (or mark 2 columns with M)", verb)
	default:
		xCol = find(args[0])
		if xCol < 0 {
			return 0, 0, fmt.Sprintf("no such column: %s", args[0])
		}
		yCol = find(args[1])
		if yCol < 0 {
			return 0, 0, fmt.Sprintf("no such column: %s", args[1])
		}
		return xCol, yCol, ""
	}
}

// resolveHistColumn picks a numeric column and an optional bin count from
// :hist args. With no column name, a single M mark wins, else the cursor
// column (same as :stats). A lone numeric arg that is not a column name is
// the bin count.
func (m *Model) resolveHistColumn(args []string) (col, bins int, err string) {
	defaultCol := func() (int, string) {
		marked := m.results.MarkedColumns()
		switch len(marked) {
		case 0:
			return m.results.CursorCol(), ""
		case 1:
			return marked[0], ""
		default:
			return 0, "mark 1 column with M, or :hist <column> [bins]"
		}
	}
	parseBins := func(s string) (int, string) {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 {
			return 0, fmt.Sprintf("invalid bin count: %s", s)
		}
		if n > 100 {
			n = 100
		}
		return n, ""
	}

	switch len(args) {
	case 0:
		col, err = defaultCol()
		return col, 0, err
	case 1:
		if idx := m.resultColumnIndex(args[0]); idx >= 0 {
			return idx, 0, ""
		}
		if _, e := strconv.Atoi(args[0]); e == nil {
			n, err := parseBins(args[0])
			if err != "" {
				return 0, 0, err
			}
			col, err = defaultCol()
			return col, n, err
		}
		return 0, 0, fmt.Sprintf("no such column: %s", args[0])
	default:
		col = m.resultColumnIndex(args[0])
		if col < 0 {
			return 0, 0, fmt.Sprintf("no such column: %s", args[0])
		}
		n, err := parseBins(args[1])
		return col, n, err
	}
}

// resolveFreqColumn picks the column for :freq / :pie. With no name, a single M mark
// wins, else the cursor column (same as :hist / :stats).
func (m *Model) resolveFreqColumn(args []string) (col int, err string) {
	defaultCol := func() (int, string) {
		marked := m.results.MarkedColumns()
		switch len(marked) {
		case 0:
			return m.results.CursorCol(), ""
		case 1:
			return marked[0], ""
		default:
			return 0, "mark 1 column with M, or :freq/:pie <column>"
		}
	}
	switch len(args) {
	case 0:
		return defaultCol()
	case 1:
		if idx := m.resultColumnIndex(args[0]); idx >= 0 {
			return idx, ""
		}
		return 0, fmt.Sprintf("no such column: %s", args[0])
	default:
		return 0, "usage: :freq [column] (or :pie [column])"
	}
}

// resolveBarColumns picks label/value column indices and an aggregate from
// :bar args or from ordered column marks. A single column (named or marked)
// is a frequency count. Returns a user-facing error string on failure.
func (m *Model) resolveBarColumns(args []string) (labelCol, valueCol int, agg barAgg, err string) {
	find := m.resultColumnIndex
	freq := func(col int) (int, int, barAgg, string) {
		return col, col, barAggCount, ""
	}
	fromMarks := func(a barAgg, explicit bool) (int, int, barAgg, string) {
		marked := m.results.MarkedColumns()
		switch len(marked) {
		case 1:
			if !explicit || a == barAggCount {
				return freq(marked[0])
			}
			return 0, 0, 0, "mark 2 columns with M (label, then value), or :bar <label> <value> [sum|count|avg]"
		case 2:
			return marked[0], marked[1], a, ""
		default:
			return 0, 0, 0, "mark 2 columns with M (label, then value), :bar <label> to count, or :bar <label> <value> [sum|count|avg]"
		}
	}
	fromNames := func(label, value string, a barAgg) (int, int, barAgg, string) {
		lc := find(label)
		if lc < 0 {
			return 0, 0, 0, fmt.Sprintf("no such column: %s", label)
		}
		vc := find(value)
		if vc < 0 {
			return 0, 0, 0, fmt.Sprintf("no such column: %s", value)
		}
		return lc, vc, a, ""
	}

	switch len(args) {
	case 0:
		return fromMarks(barAggSum, false)
	case 1:
		if a, ok := parseBarAgg(args[0]); ok {
			return fromMarks(a, true)
		}
		if idx := find(args[0]); idx >= 0 {
			return freq(idx)
		}
		return 0, 0, 0, fmt.Sprintf("no such column: %s", args[0])
	case 2:
		// `:bar status count` is a one-column frequency unless `count` is
		// also a real column (then it stays label × value).
		if a, ok := parseBarAgg(args[1]); ok && find(args[1]) < 0 {
			lc := find(args[0])
			if lc < 0 {
				return 0, 0, 0, fmt.Sprintf("no such column: %s", args[0])
			}
			if a != barAggCount {
				return 0, 0, 0, "one-column :bar only supports count (or pass a value column)"
			}
			return freq(lc)
		}
		return fromNames(args[0], args[1], barAggSum)
	default:
		a, ok := parseBarAgg(args[2])
		if !ok {
			return 0, 0, 0, fmt.Sprintf("unknown aggregate: %s (try sum, count, or avg)", args[2])
		}
		return fromNames(args[0], args[1], a)
	}
}

// exTheme switches to a named theme (:theme <name>), applying it live and
// persisting the choice — the non-interactive counterpart of the g c picker.
// The name must match a known theme (case-insensitive).
func (m *Model) exTheme(name string) tea.Cmd {
	resolved := ""
	for _, t := range themeNames() {
		if strings.EqualFold(t, name) {
			resolved = t
			break
		}
	}
	if resolved == "" {
		m.schemaMsg = fmt.Sprintf("no such theme: %s", name)
		return nil
	}
	m.settings.Theme = resolved
	if m.config != nil {
		m.config.Settings.Theme = resolved
		_ = m.config.Save()
	}
	applyPalette(paletteForTheme(resolved))
	m.schemaMsg = "theme: " + resolved
	return nil
}

// exIcons switches the tree expand/collapse glyph set (:icons <unicode|nerdfont>),
// applying it live and persisting the choice to config — the same shape as
// exTheme. "unicode" (or "default") restores the portable triangles and
// clears the stored value (so it omits from YAML); "nerdfont" uses Nerd Font
// angle chevrons (U+F105/U+F107), which need a Nerd Font in the terminal.
func (m *Model) exIcons(name string) tea.Cmd {
	resolved, ok := resolveIconSet(name)
	if !ok {
		m.schemaMsg = fmt.Sprintf("unknown icon set: %s (try :icons unicode or :icons nerdfont)", name)
		return nil
	}
	m.settings.Icons = resolved
	if m.config != nil {
		m.config.Settings.Icons = resolved
		_ = m.config.Save()
	}
	applyIcons(resolved)
	label := resolved
	if label == "" {
		label = "unicode"
	}
	m.schemaMsg = "icons: " + label
	return nil
}

// exCount runs SELECT count(*) FROM <table> (:count [table]), defaulting to
// the current table. It reuses the :goto pattern (set the editor, run the
// statement) so the count lands in the results panel like any query.
func (m *Model) exCount(name string) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	m.editor.SetValue(fmt.Sprintf("SELECT count(*) FROM %s;", table))
	return m.executeQuery()
}

// defaultSampleSize is the row cap :sample uses — a small, fast peek distinct
// from :goto, which opens the table for paged browsing at the full page size.
const defaultSampleSize = 10

// exSample peeks at the first rows of a table (:sample [table] / :head),
// defaulting to the current table. A small fixed limit keeps it a quick glance
// rather than a full first page (:goto already does that).
func (m *Model) exSample(name string) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s LIMIT %d;", table, defaultSampleSize))
	return m.executeQuery()
}

// bookmarkCurrentQuery adds the editor's current query to the bookmarks for
// the active connection. It is the shared action behind the B key and
// :bookmark, reporting the outcome via the transient bookmark status message.
// (The B key keeps its own insert-mode guard; this helper is the pure action,
// safe to call from the modal ex line where the editor isn't receiving keys.)
func (m *Model) bookmarkCurrentQuery() {
	q := m.editor.FormatQuery()
	if q == "" || m.connection == nil || m.bookmarkStore == nil {
		return
	}
	if err := m.bookmarkStore.Add(m.connection.Config().Name, q); err == nil {
		m.bookmarkMsg = "bookmarked"
	} else {
		m.bookmarkMsg = "already bookmarked"
	}
}

// exImport runs an async SQL import from a file (:import <file>) — the
// non-interactive counterpart of the I key. It expands ~ (the shared
// expandTilde, the same expansion the import prompt applies) and hands the
// resolved path to execImportSQL, so progress and the final result flow
// through the same import status messages as the interactive path.
func (m *Model) exImport(path string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	return m.execImportSQL(expanded)
}

// exRerun loads and runs a past query by history rank (:rerun <n>), where n=1
// is the most recent — the same most-recent-first order the history panel shows
// (and numbers). The history store lists entries oldest→newest, so the nth most
// recent is entries[len-n]. It reuses the :goto pattern (set the editor, run
// the statement).
func (m *Model) exRerun(arg string) tea.Cmd {
	if m.connection == nil || m.historyStore == nil {
		m.schemaMsg = "no history available"
		return nil
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		m.schemaMsg = ":rerun needs a positive number (1 = most recent)"
		return nil
	}
	entries, err := m.historyStore.Get(m.connection.Config().Name)
	if err != nil || len(entries) == 0 {
		m.schemaMsg = "no history yet"
		return nil
	}
	idx := len(entries) - n
	if idx < 0 {
		m.schemaMsg = fmt.Sprintf("history has only %d entries", len(entries))
		return nil
	}
	m.editor.SetValue(entries[idx].Query)
	return m.executeQuery()
}

// defaultWatchInterval is the refresh period :watch uses when given no
// argument. minWatchInterval guards against a refresh loop that would thrash
// the database and starve the UI. defaultTailInterval is faster — tailing is
// meant to feel live.
const (
	defaultWatchInterval = 5 * time.Second
	defaultTailInterval  = 2 * time.Second
	minWatchInterval     = time.Second
)

// watchTick schedules the next :watch refresh, carrying the active generation
// so the handler can ignore a tick from a superseded (restarted/stopped) watch.
func watchTick(d time.Duration, gen uint64) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return watchTickMsg{gen: gen}
	})
}

// parseWatchInterval accepts either a bare integer (seconds, e.g. "3") or a Go
// duration string ("3s", "1m", "500ms"). It rejects anything below the minimum
// so a typo can't spin a sub-second refresh loop.
func parseWatchInterval(s string) (time.Duration, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		d := time.Duration(n) * time.Second
		if d < minWatchInterval {
			return 0, false
		}
		return d, true
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d < minWatchInterval {
			return 0, false
		}
		return d, true
	}
	return 0, false
}

// exWatch toggles periodic re-execution of the last query (:watch [n] /
// :watch off). With no argument it refreshes every defaultWatchInterval; a bare
// integer is seconds and a Go duration ("3s", "1m") is accepted too.
// "off"/"stop"/"0" stops an active watch. Each refresh re-runs m.lastQuery at
// the current page/filters (a live refresh of the view in focus), so running a
// different query makes the watch follow it — the status bar's WATCH indicator
// keeps that visible. Starting a watch bumps watchGen so any prior tick chain
// dies instead of doubling the rate.
func (m *Model) exWatch(arg string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	a := strings.TrimSpace(arg)
	switch strings.ToLower(a) {
	case "off", "stop", "0":
		m.stopBackgroundRefresh()
		return nil
	}
	interval := defaultWatchInterval
	if a != "" {
		d, ok := parseWatchInterval(a)
		if !ok {
			m.schemaMsg = ":watch interval must be like 3 or 3s or 1m (min 1s)"
			return nil
		}
		interval = d
	}
	if m.lastQuery == "" {
		m.schemaMsg = "nothing to watch — run a query first"
		return nil
	}
	m.watchGen++
	m.watchActive = true
	m.watchInterval = interval
	m.watchMode = "watch"
	m.schemaMsg = fmt.Sprintf("watching every %s — :watch off to stop", humanDuration(interval))
	// Refresh immediately, then arm the next tick.
	return tea.Batch(m.runPageQuery(), watchTick(interval, m.watchGen))
}

// handleWatchTick processes a watch refresh. It ignores stale ticks (from a
// superseded/stopped watch), self-terminates when there's nothing left to
// watch, and otherwise refreshes the current view — unless a query is already
// in flight (to avoid cancel-thrashing when the interval is shorter than the
// query) — before rescheduling. Extracted from Update so the logic is testable
// without driving the whole Update dispatch.
func (m Model) handleWatchTick(msg watchTickMsg) (Model, tea.Cmd) {
	if !m.watchActive || msg.gen != m.watchGen {
		return m, nil
	}
	if m.lastQuery == "" || m.connection == nil {
		m.watchActive = false
		return m, nil
	}
	cmds := []tea.Cmd{watchTick(m.watchInterval, m.watchGen)}
	if !m.queryRunning {
		cmds = append(cmds, m.runPageQuery())
	}
	return m, tea.Batch(cmds...)
}

// stopBackgroundRefresh stops an active :watch or :tail, reporting which was
// running. Shared by "off"/"stop"/"0" on both verbs so either cancels either.
func (m *Model) stopBackgroundRefresh() {
	if !m.watchActive {
		m.schemaMsg = "no active watch or tail"
		return
	}
	kind := "watch"
	if m.watchMode == "tail" {
		kind = "tail"
	}
	m.watchActive = false
	m.watchPrevRows = nil
	m.schemaMsg = kind + " stopped"
}

// exTail streams the newest rows of a table (:tail [table] [n]) — the
// append-only/event-table companion to :watch. It resolves the table
// (defaulting to the current one), builds a newest-first query ordered by the
// primary key when there's a single-column PK (the common case for event
// tables; otherwise unordered), and re-runs it on a timer, reusing the :watch
// machinery. Each refresh resets the cursor to the top (newest) row, so new
// rows stream in at the head. "off"/"stop"/"0" cancels the refresh (same as
// :watch off). An optional second argument is the interval in seconds or a Go
// duration ("3", "3s", "1m").
func (m *Model) exTail(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if len(args) > 0 {
		switch strings.ToLower(strings.TrimSpace(args[0])) {
		case "off", "stop", "0":
			m.stopBackgroundRefresh()
			return nil
		}
	}
	table := ""
	interval := defaultTailInterval
	if len(args) > 0 {
		table = args[0]
	}
	if len(args) > 1 {
		d, ok := parseWatchInterval(args[1])
		if !ok {
			m.schemaMsg = ":tail interval must be like 3 or 3s or 1m (min 1s)"
			return nil
		}
		interval = d
	}
	table = m.resolveTableArg(table)
	if table == "" {
		return nil
	}
	driver := m.connection.Config().Driver
	q := "SELECT * FROM " + quoteIdentD(driver, table)
	// Order newest-first by the PK when there's a single-column one (the usual
	// shape of an append-only table). Composite PKs are left unordered rather
	// than guessing an ordering.
	if pks, err := m.connection.DB().PrimaryKeys(table); err == nil && len(pks) == 1 {
		q += " ORDER BY " + quoteIdentD(driver, pks[0]) + " DESC"
	}
	m.lastQuery = q
	m.baseQuery = q
	m.filters = nil
	m.sortCol = ""
	m.sortDir = ""
	m.page = 0
	m.queryStack = nil
	m.totalRows = 0
	m.totalRowsSet = false
	m.watchMode = "tail"
	m.watchGen++
	m.watchActive = true
	m.watchInterval = interval
	m.schemaMsg = fmt.Sprintf("tailing %s every %s — :tail off to stop", table, humanDuration(interval))
	return tea.Batch(m.runPageQuery(), watchTick(interval, m.watchGen))
}

// quoteIdentD quotes a SQL identifier for the given driver (double quotes for
// SQLite/Postgres, backticks for MySQL), matching db.quoteIdent. The ui
// package's other quoteIdent is double-quote-only; this driver-aware variant
// matters for :tail's ORDER BY, where MySQL would otherwise treat "col" as a
// string literal and silently ignore the ordering.
func quoteIdentD(driver db.Driver, name string) string {
	switch driver {
	case db.DriverSQLite, db.DriverPostgres:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	default:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
}

// exLimit changes the results page size (:limit <n> / :limit off). The new size
// is applied immediately by re-running the current query at page 0 — the old
// page position is meaningless under a different size. "off"/"default" restores
// the configured default; bare ":limit" reports the current size. Minimum 1;
// there's no upper cap (a huge limit just asks the DB for more rows).
func (m *Model) exLimit(arg string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	a := strings.TrimSpace(arg)
	switch strings.ToLower(a) {
	case "off", "default":
		m.pageSize = defaultPageSize
		m.page = 0
		m.schemaMsg = fmt.Sprintf("page size reset to %d (default)", m.pageSize)
		return m.rerunForLimit()
	case "":
		dft := ""
		if m.pageSize == defaultPageSize {
			dft = " (default)"
		}
		m.schemaMsg = fmt.Sprintf("page size: %d%s", m.pageSize, dft)
		return nil
	}
	n, err := strconv.Atoi(a)
	if err != nil || n < 1 {
		m.schemaMsg = ":limit needs a positive number (or off)"
		return nil
	}
	m.pageSize = n
	m.page = 0
	m.schemaMsg = fmt.Sprintf("page size set to %d", n)
	return m.rerunForLimit()
}

// rerunForLimit re-runs the current query at page 0 to apply a new page size,
// or returns nil if no query has been run yet.
func (m *Model) rerunForLimit() tea.Cmd {
	if m.lastQuery == "" {
		return nil
	}
	return m.runPageQuery()
}

// exTiming toggles showing the last query's elapsed time in the status bar
// (:timing / :timing on / :timing off). The duration is captured on every
// query completion regardless; this only controls whether it's displayed.
func (m *Model) exTiming(arg string) tea.Cmd {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on":
		m.showTiming = true
	case "off":
		m.showTiming = false
	case "":
		m.showTiming = !m.showTiming
	default:
		m.schemaMsg = ":timing takes on, off, or nothing"
		return nil
	}
	if m.showTiming {
		m.schemaMsg = "timing on"
	} else {
		m.schemaMsg = "timing off"
	}
	return nil
}

// exPeek shows a one-glance summary of a table (:peek [table]) — row count,
// column count, primary key, and the column list — in the lookup overlay. It
// defaults to the current table and runs async (a COUNT(*) plus schema/PK
// introspection), complementing :describe (full structure) and :count (just the
// number).
func (m *Model) exPeek(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	conn := m.connection
	qt := quoteIdentD(conn.Config().Driver, table)
	return func() tea.Msg {
		schema, err := conn.DB().TableSchema(table)
		if err != nil {
			return lookupResultMsg{err: err}
		}
		pks, _ := conn.DB().PrimaryKeys(table)
		// Row count is best-effort: a failure just shows an em dash.
		count := "—"
		if res, err := conn.DB().Execute("SELECT count(*) FROM " + qt); err == nil && len(res.Rows) > 0 && len(res.Rows[0]) > 0 {
			count = res.Rows[0][0]
		}
		names := make([]string, len(schema))
		for i, c := range schema {
			names[i] = c.Name
		}
		pk := "—"
		if len(pks) > 0 {
			pk = strings.Join(pks, ", ")
		}
		rows := [][]string{
			{"rows", count},
			{"columns", strconv.Itoa(len(schema))},
			{"primary key", pk},
			{"column names", strings.Join(names, ", ")},
		}
		return lookupResultMsg{
			title:  fmt.Sprintf("Peek: %s", table),
			result: db.Result{Columns: []db.Column{{Name: "Field"}, {Name: "Value"}}, Rows: rows},
		}
	}
}

// filterExprRE parses a :filter expression: a column, a comparison operator,
// and a value. It tolerates spaces around the operator and accepts the compact
// form ("col=val") as well as the spaced one ("col = val"); the value is the
// remainder of the line, so it may contain spaces.
var filterExprRE = regexp.MustCompile(`^(\w+)\s*(=|!=|>=|<=|>|<|~)\s*(.+)$`)

// parseFilterExpr splits a :filter expression into column, operator, value.
func parseFilterExpr(s string) (col, op, value string, ok bool) {
	m := filterExprRE.FindStringSubmatch(s)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], strings.TrimSpace(m[3]), true
}

// buildFilterFragment renders a :filter expression as a WHERE fragment, type-
// quoting the value via formatFilterValue (the same helper the */! cell filters
// use) so strings are quoted and numbers left bare. Operators are literal SQL
// semantics; ~ is a convenience for a substring LIKE.
func buildFilterFragment(col, op, value, dbType string) string {
	switch op {
	case "~":
		esc := strings.ReplaceAll(value, "'", "''")
		return fmt.Sprintf("%s LIKE '%%%s%%'", col, esc)
	case "=", "!=", ">", "<", ">=", "<=":
		return fmt.Sprintf("%s %s %s", col, op, formatFilterValue(value, dbType))
	}
	return ""
}

// exFilter applies, clears, or lists quick filters from the : line
// (:filter <col><op><value> | :filter off | :filter). It wires the : line into
// the m.filters infra shared by the */! cell filters, the value picker, and
// filterByMarks: the fragment is appended (replacing any existing filter on the
// same column), applyFilteredQuery rebuilds lastQuery, and the query re-runs at
// page 0. The expression is structured rather than raw so the value is type-
// quoted; ops are = != > < >= <= and ~ (LIKE substring). Requires a simple
// single-table SELECT (canFilter), since the filter layer rebuilds from the
// base table. "off"/"clear" drops all filters; bare ":filter" lists them.
func (m *Model) exFilter(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	joined := strings.TrimSpace(strings.Join(args, " "))
	switch strings.ToLower(joined) {
	case "off", "clear":
		if len(m.filters) == 0 {
			m.schemaMsg = "no active filters"
			return nil
		}
		m.schemaMsg = fmt.Sprintf("cleared %d filter%s", len(m.filters), pluralIf(len(m.filters) != 1, "s"))
		return m.clearFilters()
	case "":
		if len(m.filters) == 0 {
			m.schemaMsg = "no active filters"
		} else {
			short := make([]string, len(m.filters))
			for i, f := range m.filters {
				short[i] = compactFilter(f)
			}
			m.schemaMsg = "filters: " + strings.Join(short, "  ")
		}
		return nil
	}
	if !m.canFilter() {
		m.schemaMsg = "filtering needs a simple table query (SELECT * FROM <table>)"
		return nil
	}
	col, op, value, ok := parseFilterExpr(joined)
	if !ok {
		m.schemaMsg = "usage: :filter <col> <op> <value>  (ops: = != > < >= <= ~)"
		return nil
	}
	idx := -1
	for i := 0; i < m.results.NumCols(); i++ {
		if strings.EqualFold(m.results.ColumnName(i), col) {
			idx = i
			break
		}
	}
	if idx < 0 {
		m.schemaMsg = fmt.Sprintf("no such column: %s", col)
		return nil
	}
	frag := buildFilterFragment(col, op, value, m.results.ColumnType(idx))
	if frag == "" {
		m.schemaMsg = "unsupported operator: " + op
		return nil
	}
	m.filters = removeColumnFilters(m.filters, col)
	m.filters = append(m.filters, frag)
	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	m.schemaMsg = "filtered: " + compactFilter(frag)
	return m.runPageQuery()
}

// lineCount returns the number of lines in s (a trailing newline does not add
// an extra line), matching how editors report buffer size.
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
