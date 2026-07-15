package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// exCmd is the vim-style ":" command line: a modal prompt at the bottom of the
// workspace. Type a command (with optional arguments), enter runs it, esc
// cancels, ↑/↓ recalls history. Unknown input in the results view falls back
// to a column jump, preserving the legacy ":" behaviour.
type exCmd struct {
	visible bool
	input   string
	hist    []string // command history, most-recent last
	histIdx int      // recall cursor; len(hist) == "fresh input"
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
		if input == "" {
			return *m, nil
		}
		m.ex.hist = append(m.ex.hist, input)
		m.ex.histIdx = len(m.ex.hist)
		return *m, m.runExCommand(input)
	case "up":
		m.ex.recall(-1)
		return *m, nil
	case "down":
		m.ex.recall(1)
		return *m, nil
	case "backspace":
		if len(m.ex.input) > 0 {
			r := []rune(m.ex.input)
			m.ex.input = string(r[:len(r)-1])
		}
		return *m, nil
	}
	if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
		m.ex.input += msg.String()
	}
	return *m, nil
}

// Open shows the ex command line with an empty buffer.
func (ex *exCmd) Open() {
	ex.visible = true
	ex.input = ""
	ex.histIdx = len(ex.hist)
}

// Hide closes the ex command line.
func (ex *exCmd) Hide() { ex.visible = false }

// IsVisible reports whether the ex command line is shown.
func (ex exCmd) IsVisible() bool { return ex.visible }

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
// command. Errors and short feedback are reported via the transient status-bar
// message (m.schemaMsg), mirroring the other action handlers.
func (m *Model) runExCommand(input string) tea.Cmd {
	verb, args, force := parseExLine(input)
	switch verb {
	case "e", "edit":
		if len(args) == 0 {
			m.schemaMsg = ":e needs a file path"
			return nil
		}
		return m.exEditFile(args[0])
	case "w", "write":
		// With a file argument, write the editor buffer to disk (vim :w file);
		// without one, :w saves staged cell edits (the legacy meaning).
		if len(args) > 0 {
			return m.exWriteFile(args[0])
		}
		return m.exWrite(force)
	case "q", "quit":
		return m.exQuit(force)
	case "wq", "x":
		cmd := m.saveEdits() // no-op when there are no dirty cells
		if len(m.resultsTabs) > 1 {
			m.closeTab(m.activeTabID)
		} else {
			m.schemaMsg = "cannot close the last tab"
		}
		return cmd
	case "sort":
		if len(args) == 0 {
			m.schemaMsg = ":sort needs a column name"
			return nil
		}
		return m.sortByColName(args[0])
	case "goto", "gt":
		if len(args) == 0 {
			m.schemaMsg = ":goto needs a table name"
			return nil
		}
		return m.exGoto(args[0])
	case "export":
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		return m.exExport(arg)
	case "begin", "transaction":
		return m.exBegin()
	case "commit":
		return m.exCommit()
	case "rollback":
		return m.exRollback()
	case "help", "h":
		m.help.Show()
		return nil
	default:
		// Legacy ":" behaviour: in the results view a bare identifier jumps
		// to the best-matching column. Use the original input so column-name
		// case is preserved.
		if m.focus == FocusResults && m.results.NumCols() > 0 && len(args) == 0 {
			if idx := bestColumnMatch(m.results.columns, input); idx >= 0 {
				m.results.SetCursor(m.results.CursorRow(), idx)
				return nil
			}
		}
		m.schemaMsg = fmt.Sprintf("E492: not a command: %s", input)
		return nil
	}
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

// exQuit closes the active tab (:q). Unsaved edits block unless forced (:q!).
func (m *Model) exQuit(force bool) tea.Cmd {
	if !force && m.results.HasDirtyCells() {
		m.schemaMsg = "unsaved changes — use :q! to discard"
		return nil
	}
	if len(m.resultsTabs) <= 1 {
		m.schemaMsg = "cannot close the last tab"
		return nil
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
