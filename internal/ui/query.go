package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/db"
)

// executeQuery runs the query under the cursor asynchronously with pagination.
// When the editor contains multiple statements, only the one under the cursor
// is executed.
func (m *Model) executeQuery() tea.Cmd {
	query := m.editor.StatementAtCursor()
	if query == "" {
		return nil
	}

	m.lastQuery = query
	m.baseQuery = query
	m.filters = nil
	m.sortCol = ""
	m.sortDir = ""
	m.page = 0
	m.queryStack = nil
	m.totalRows = 0
	m.totalRowsSet = false
	return m.runPageQuery()
}

// explainQuery wraps the statement under the cursor in EXPLAIN and executes it
// asynchronously. The result is displayed in a scrollable overlay panel.
func (m *Model) explainQuery() tea.Cmd {
	if m.connection == nil {
		return nil
	}
	query := m.editor.StatementAtCursor()
	if query == "" {
		return nil
	}

	// Strip trailing semicolons — EXPLAIN must precede a single statement.
	query = strings.TrimRight(strings.TrimSpace(query), ";")
	if query == "" {
		return nil
	}

	driver := m.connection.Config().Driver
	var explainStmt string
	switch driver {
	case db.DriverSQLite:
		explainStmt = "EXPLAIN QUERY PLAN " + query
	case db.DriverPostgres:
		explainStmt = "EXPLAIN " + query
	default: // MySQL
		explainStmt = "EXPLAIN " + query
	}

	conn := m.connection
	ctx, cancel := m.queryContext()
	return func() tea.Msg {
		defer cancel()
		result, err := conn.DB().ExecuteContext(ctx, explainStmt)
		return explainResultMsg{result: result, err: err}
	}
}

// nextPage advances to the next page of results.
func (m *Model) nextPage() tea.Cmd {
	if m.lastQuery == "" {
		return nil
	}
	m.page++
	return m.runPageQuery()
}

// prevPage goes back to the previous page of results.
func (m *Model) prevPage() tea.Cmd {
	if m.lastQuery == "" || m.page == 0 {
		return nil
	}
	m.page--
	return m.runPageQuery()
}

// buildPageMsg constructs the pagination status string using the current
// page position, row count, and total rows (if known).
func (m Model) buildPageMsg(page, pageSize, rowCount int, hasNext bool) string {
	offset := page * pageSize
	if m.totalRowsSet {
		if hasNext {
			return fmt.Sprintf("page %d (rows %d-%d of %s)", page+1, offset+1, offset+rowCount, formatCount(m.totalRows))
		}
		if page > 0 {
			return fmt.Sprintf("page %d (rows %d-%d of %s)", page+1, offset+1, offset+rowCount, formatCount(m.totalRows))
		}
		return fmt.Sprintf("%s rows", formatCount(m.totalRows))
	}
	if page > 0 || hasNext {
		pgInfo := fmt.Sprintf("page %d (rows %d-%d)", page+1, offset+1, offset+rowCount)
		if hasNext {
			pgInfo += " · more available"
		}
		return pgInfo
	}
	return ""
}

// formatCount formats an integer with thousands separators.
func formatCount(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + formatCount(-n)
	}
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}
	return string(result)
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 100 * time.Millisecond

// spinnerTick returns a command that fires a spinnerTickMsg after the spinner
// interval, keeping the UI animated while a query is in flight.
func spinnerTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// queryContext returns a context for an async query. The returned cancel
// func is stored on the model so esc/ctrl+c can cancel immediately; when
// queryTimeout > 0 the context also carries a deadline so a runaway query
// can't hang the TUI. A zero/negative queryTimeout disables the deadline
// (wait indefinitely — esc still cancels).
func (m Model) queryContext() (context.Context, context.CancelFunc) {
	if m.queryTimeout > 0 {
		return context.WithTimeout(context.Background(), m.queryTimeout)
	}
	return context.WithCancel(context.Background())
}

// cancelHint is the in-flight hint shown beside the spinner: esc cancels,
// and when a deadline is set the limit is shown so the user knows it will
// auto-cancel.
func (m Model) cancelHint() string {
	if m.queryTimeout > 0 {
		return fmt.Sprintf("(esc to cancel · %s limit)", m.queryTimeout)
	}
	return "(esc to cancel)"
}

// runPageQuery wraps the original query with LIMIT/OFFSET and executes it
// asynchronously with cancellation support. Non-SELECT statements (DESCRIBE,
// SHOW, EXPLAIN, etc.) are executed directly without pagination wrapping, since
// they can't be used as subqueries.
func (m *Model) runPageQuery() tea.Cmd {
	// Cancel any previously in-flight query before starting a new one.
	if m.queryCancel != nil {
		m.queryCancel()
		m.queryCancel = nil
	}

	offset := m.page * m.pageSize
	query := strings.TrimRight(m.lastQuery, ";")

	conn := m.connection
	tx := m.tx // nil unless a manual transaction (:begin) is active
	page := m.page
	pageSize := m.pageSize
	lastQuery := m.lastQuery

	ctx, cancel := m.queryContext()
	m.queryCancel = cancel
	m.queryRunning = true
	m.queryCancelled = false
	m.queryStart = time.Now()
	m.querySpinner = 0

	// Only wrap simple SELECT queries; everything else runs as-is.
	// JOIN queries can't be wrapped because MySQL requires unique column
	// names in derived tables, and JOINs often produce duplicates (e.g.
	// both tables have an "id" column).
	var execQuery string
	if isSelectQuery(query) && !hasJoinClause(query) {
		execQuery = fmt.Sprintf("SELECT * FROM (%s) AS _gsql_page LIMIT %d OFFSET %d",
			query, pageSize+1, offset)
	} else {
		execQuery = query
	}

	execCmd := func() tea.Msg {
		// Run on the active manual transaction when one is open, so reads see
		// the tx's uncommitted writes and writes stage inside it. The ex
		// guards (refuse :commit/:rollback while queryRunning) keep this
		// goroutine from racing the tx lifecycle.
		var (
			result db.Result
			err    error
		)
		if tx != nil {
			result, err = tx.ExecuteContext(ctx, execQuery)
		} else {
			result, err = conn.DB().ExecuteContext(ctx, execQuery)
		}
		return queryExecutedMsg{
			query:     lastQuery,
			result:    result,
			err:       err,
			page:      page,
			pageSize:  pageSize,
			cancelled: errors.Is(err, context.Canceled),
			timedOut:  errors.Is(err, context.DeadlineExceeded),
		}
	}

	return tea.Batch(execCmd, spinnerTick())
}

// isSelectQuery returns true if the query is a SELECT (or WITH ... SELECT)
// statement that can safely be wrapped in a subquery for pagination.
func isSelectQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

// hasJoinClause reports whether the query contains a JOIN. These can't be
// wrapped in a subquery for pagination because MySQL requires unique column
// names in derived tables, and JOINs frequently produce duplicates.
func hasJoinClause(query string) bool {
	upper := " " + strings.ToUpper(query) + " "
	for _, kw := range []string{" JOIN ", " INNER JOIN ", " LEFT JOIN ", " RIGHT JOIN ",
		" OUTER JOIN ", " FULL JOIN ", " FULL OUTER JOIN ", " CROSS JOIN ", " NATURAL JOIN "} {
		if strings.Contains(upper, kw) {
			return true
		}
	}
	// Also detect comma-separated FROM (implicit cross join): "FROM a, b"
	fromIdx := strings.Index(upper, " FROM ")
	if fromIdx >= 0 {
		rest := upper[fromIdx:]
		// Check for a comma before any WHERE/GROUP/ORDER/LIMIT clause.
		for _, end := range []string{" WHERE ", " GROUP ", " ORDER ", " LIMIT ", " HAVING "} {
			if i := strings.Index(rest, end); i >= 0 {
				rest = rest[:i]
			}
		}
		if strings.Contains(rest, ",") {
			return true
		}
	}
	return false
}
