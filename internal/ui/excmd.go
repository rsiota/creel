package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ruben/gsql/internal/db"
)

// exCmd is the vim-style ":" command line: a modal prompt at the bottom of the
// workspace. Type a command (with optional arguments), enter runs it, esc
// cancels, ↑/↓ recalls history. Unknown input in the results view falls back
// to a column jump, preserving the legacy ":" behaviour.
type exCmd struct {
	visible bool
	input   string
	hist    []string     // command history, most-recent last
	histIdx int          // recall cursor; len(hist) == "fresh input"
	comp    []exCompItem // verb-completion candidates; empty = no popup
}

// exCompItem is one row in the ":" verb-completion popup.
type exCompItem struct {
	verb  string // canonical verb inserted by Tab
	usage string // invocation form, e.g. ":w [file]"
	desc  string
}

// handleExKey routes keys to the open ":" command line. It is modal: every
// key is consumed while the ex line is visible. enter parses and runs the
// input (recording it to history); esc cancels; ↑/↓ recalls history.
func (m *Model) handleExKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.ex.Hide()
		return *m, nil
	case "enter":
		input := strings.TrimSpace(m.ex.input)
		m.ex.input = ""
		m.ex.visible = false
		m.ex.comp = nil
		if input == "" {
			return *m, nil
		}
		m.ex.hist = append(m.ex.hist, input)
		m.ex.histIdx = len(m.ex.hist)
		return *m, m.runExCommand(input)
	case "up":
		m.ex.recall(-1)
		m.ex.recomputeCompletion()
		return *m, nil
	case "down":
		m.ex.recall(1)
		m.ex.recomputeCompletion()
		return *m, nil
	case "tab":
		// Complete the verb to the top match (its canonical name). recompute
		// then hides the popup, since the verb is now an exact match.
		if len(m.ex.comp) > 0 {
			m.ex.input = m.ex.comp[0].verb
			m.ex.recomputeCompletion()
		}
		return *m, nil
	case "backspace":
		if len(m.ex.input) > 0 {
			r := []rune(m.ex.input)
			m.ex.input = string(r[:len(r)-1])
			m.ex.recomputeCompletion()
		}
		return *m, nil
	}
	if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
		m.ex.input += msg.String()
		m.ex.recomputeCompletion()
	}
	return *m, nil
}

// Open shows the ex command line with an empty buffer and seeds the
// verb-completion popup with every command, so ":" alone is discoverable.
func (ex *exCmd) Open() {
	ex.visible = true
	ex.input = ""
	ex.histIdx = len(ex.hist)
	ex.recomputeCompletion()
}

// Hide closes the ex command line and drops any completion popup.
func (ex *exCmd) Hide() {
	ex.visible = false
	ex.comp = nil
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

// completionView renders the verb-completion popup (one row per candidate)
// for display directly above the ":" prompt, or "" when nothing applies. The
// first row is the Tab target. Rendering mirrors the palette (Ctrl+P): blue
// commands, grey descriptions, and a solid highlight bar on the Tab target.
// Only the matching and anchoring differ (the command line is a bottom
// prompt, so the popup sits above it rather than at the editor cursor).
func (ex exCmd) completionView() string {
	if !ex.visible || len(ex.comp) == 0 {
		return ""
	}
	const maxRows = 9
	items := ex.comp
	if len(items) > maxRows {
		items = items[:maxRows]
	}
	usageW := 0
	for _, it := range items {
		if w := runeLen(it.usage); w > usageW {
			usageW = w
		}
	}
	var lines []string
	for i, it := range items {
		usage := it.usage + strings.Repeat(" ", usageW-runeLen(it.usage))
		var row string
		if i == 0 {
			// Tab target: a solid highlight bar, mirroring the palette's
			// selected row (bg colorPrimary, fg colorBg, "❯" marker).
			row = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBg).
				Render("❯ " + usage + "  " + it.desc)
		} else {
			usageStr := lipgloss.NewStyle().Foreground(colorPrimary).Render(usage)
			desc := lipgloss.NewStyle().Foreground(colorLabel).Render(it.desc)
			row = "  " + usageStr + "  " + desc
		}
		lines = append(lines, row)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Render(strings.Join(lines, "\n"))
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
		m.quitting = true
		return tea.Quit
	}
	m.closeTab(m.activeTabID)
	return nil
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
	m.editor.SetValue(fmt.Sprintf("SELECT * FROM %s;", items[target].text))
	return m.executeQuery()
}

// exBegin starts a manual transaction (:begin). While it is active, statements
// run from the editor execute on the tx — so SELECTs see the tx's own
// uncommitted writes — and :commit / :rollback finish it. Cell edits / inserts
// / deletes / DDL are refused for the duration (they use their own autocommit
// path and would commit outside the tx). Refused while read-only, while a
// query is in flight, or when a transaction is already open.
//
// begin/commit/rollback run synchronously: they're rare, explicit actions
// that transfer no row data, and doing them inline (rather than as a goroutine
// command) keeps the tx lifecycle single-threaded. The queryRunning guard
// ensures no in-flight query goroutine is touching the tx when they run.
func (m *Model) exBegin() tea.Cmd {
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
	tx, err := m.connection.DB().Begin()
	if err != nil {
		m.schemaMsg = "begin failed: " + err.Error()
		return nil
	}
	m.tx = tx
	m.schemaMsg = "transaction started — :commit or :rollback to finish"
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
		return nil
	}
	m.tx = nil
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
		return nil
	}
	m.tx = nil
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

// loadStartupFile reads a .sql file into the editor for the `gsql -f` startup
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
	return expanded, nil
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

// exExport writes the current result set to ~/Downloads in the given format
// (:export <fmt>) — a non-interactive shortcut over the g X export picker.
// <fmt> is one of csv, json, jsonl, md, tsv (case-insensitive; "markdown" and
// "json lines" are accepted). It reuses exportResults, so marked rows are
// re-queried for complete data exactly as the picker does, and feedback flows
// through the same export status message.
func (m *Model) exExport(arg string) tea.Cmd {
	format, ok := parseExportFormat(arg)
	if !ok {
		m.schemaMsg = ":export needs a format: csv, json, jsonl, md, tsv"
		return nil
	}
	return m.exportResults(format)
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

// exRefs lists the foreign keys referencing a table (:refs <table>) — the
// reverse of g d. The lookup runs async and opens in the lookup overlay panel.
// It reads connection metadata, so it is unaffected by (and does not block on)
// an active transaction.
func (m *Model) exRefs(name string) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		refs, err := conn.DB().ReferencingForeignKeys(table)
		if err != nil {
			return lookupResultMsg{err: err}
		}
		cols := []db.Column{{Name: "Table"}, {Name: "Column"}, {Name: "References"}}
		rows := make([][]string, 0, len(refs))
		for _, r := range refs {
			rows = append(rows, []string{r.Table, r.Column, table + "." + r.RefColumn})
		}
		return lookupResultMsg{
			title:  fmt.Sprintf("References to %s", table),
			result: db.Result{Columns: cols, Rows: rows},
		}
	}
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
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	items := m.sidebarItems()
	for i, it := range items {
		if !it.isColumn && it.text == table {
			m.sidebarCursor = i
			m.sidebarViewAnchored = false
			break
		}
	}
	return m.openSchemaPanel()
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
