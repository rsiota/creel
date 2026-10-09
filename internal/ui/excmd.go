package ui

import (
	"fmt"
	"runtime"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
// key is consumed while the ex line is visible. enter runs the input when the
// verb is exact or uniquely resolved (`:go` → goto); it completes the
// highlighted popup row when the token is still ambiguous (`:g` → goto/grep).
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
// popup row instead of running the current input. Exact aliases (":w") and
// unique prefixes / unique fuzzy verbs (":go", ":gto", ":th") run immediately.
// Ambiguous prefixes (":g" → goto vs grep), partial argument tokens, and
// browsing a list after a trailing space (empty partial — same as Tab)
// complete first so a second Enter executes the finished line.
func (m *Model) exEnterShouldComplete() bool {
	if len(m.ex.comp) == 0 || strings.TrimSpace(m.ex.input) == "" {
		return false
	}
	if m.ex.argMode {
		verb, _ := verbPrefix(m.ex.input)
		rest := strings.TrimLeft(m.ex.input[len(verb):], " \t")
		_, partial := splitArgsPartial(rest)
		cand := m.ex.selectedCompItem().candidate
		if cand == "" {
			return false
		}
		// Empty partial: cursor sits after a space with the popup open
		// (`:theme `, `:set `, `:e ~/Downloads/`). Accept the highlighted row
		// instead of running the unfinished line — same as Tab.
		if partial == "" {
			return true
		}
		return !strings.EqualFold(partial, cand)
	}
	verb, hasSpace := verbPrefix(m.ex.input)
	if hasSpace {
		return false
	}
	return exResolve(strings.ToLower(strings.TrimSuffix(verb, "!"))) == nil
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
// input. Completion applies only to the verb (before any space). Prefix
// matches come first (so :g stays goto/grep); if none match, fuzzy
// subsequence matching is used so :gto still finds goto. One row per
// command even if several of its aliases match. The popup is hidden once
// the typed verb is an exact, unambiguous canonical match.
func (ex *exCmd) recomputeCompletion() {
	ex.comp = ex.comp[:0]
	ex.argMode = false // verb mode
	ex.selIdx = 0
	verb, hasSpace := verbPrefix(ex.input)
	if hasSpace {
		return
	}
	needle := strings.ToLower(verb)
	if needle == "" {
		for _, s := range exCommands() {
			ex.comp = append(ex.comp, exCompItem{
				verb:  s.verbs[0],
				usage: s.usage,
				desc:  s.desc,
			})
		}
		return
	}

	prefixHits, fuzzyHits := matchExVerbs(needle)
	hits := prefixHits
	if len(hits) == 0 {
		hits = fuzzyHits
	}
	for _, s := range hits {
		ex.comp = append(ex.comp, exCompItem{
			verb:  s.verbs[0],
			usage: s.usage,
			desc:  s.desc,
		})
	}
	if len(ex.comp) == 1 && ex.comp[0].verb == needle {
		ex.comp = nil
	}
}

// matchExVerbs splits commands into prefix matches and fuzzy (subsequence)
// matches for needle. Each command appears at most once, ranked by the best
// alias score (lower is better).
func matchExVerbs(needle string) (prefixHits, fuzzyHits []exCmdSpec) {
	type scored struct {
		spec  exCmdSpec
		score int
	}
	var prefix, fuzzy []scored
	for _, s := range exCommands() {
		var bestPrefix, bestFuzzy int
		hasPrefix, hasFuzzy := false, false
		for _, v := range s.verbs {
			lv := strings.ToLower(v)
			if strings.HasPrefix(lv, needle) {
				_, score := fuzzyMatch(needle, v)
				if !hasPrefix || score < bestPrefix {
					bestPrefix = score
					hasPrefix = true
				}
				continue
			}
			idx, score := fuzzyMatch(needle, v)
			if idx != nil && (!hasFuzzy || score < bestFuzzy) {
				bestFuzzy = score
				hasFuzzy = true
			}
		}
		if hasPrefix {
			prefix = append(prefix, scored{s, bestPrefix})
		} else if hasFuzzy {
			fuzzy = append(fuzzy, scored{s, bestFuzzy})
		}
	}
	sort.SliceStable(prefix, func(i, j int) bool { return prefix[i].score < prefix[j].score })
	sort.SliceStable(fuzzy, func(i, j int) bool { return fuzzy[i].score < fuzzy[j].score })
	for _, h := range prefix {
		prefixHits = append(prefixHits, h.spec)
	}
	for _, h := range fuzzy {
		fuzzyHits = append(fuzzyHits, h.spec)
	}
	return prefixHits, fuzzyHits
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
	// Unique prefixes resolve here too so `:go users` can complete tables.
	lookup := strings.TrimSuffix(verb, "!")
	spec := exResolve(lookup)
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
// rune (except inside single quotes, matching POSIX-ish behaviour). On
// Windows a backslash is a path separator, so it is only an escape before
// ", ', or another \ — otherwise C:\Users\... would be eaten.
func splitShellFields(s string) []string {
	var fields []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\' && i+1 < len(runes) && !inSingle:
			next := runes[i+1]
			if runtime.GOOS == "windows" && next != '"' && next != '\'' && next != '\\' {
				cur.WriteRune(r)
				continue
			}
			cur.WriteRune(next)
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
	// best-matching column. This runs *before* unique-prefix resolve so a
	// common column like "id" is not stolen by :indexes.
	if m.focus == FocusResults && m.results.NumCols() > 0 && len(args) == 0 {
		if idx := bestColumnMatch(m.results.columns, input); idx >= 0 {
			m.results.SetCursor(m.results.CursorRow(), idx)
			return nil
		}
	}
	if spec := uniqueExMatch(verb); spec != nil {
		return spec.run(m, args, force)
	}
	m.schemaMsg = fmt.Sprintf("E492: not a command: %s", input)
	return nil
}
