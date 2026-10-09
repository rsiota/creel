package ui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/db"
)

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
	m.clearAliasState()
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
// quoted; ops are = != > < >= <= and ~ (LIKE substring). Works on simple
// SELECT * FROM <table> and on other SELECTs via a subquery wrap. "off"/"clear"
// drops all filters; bare ":filter" lists them.
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
		m.schemaMsg = "filtering needs a SELECT with unique column names"
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
