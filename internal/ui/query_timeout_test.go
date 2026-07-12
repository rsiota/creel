package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

// queryExecutedMsg now carries a timedOut flag distinct from cancelled.
func TestQueryExecutedMsgHasTimeoutFlag(t *testing.T) {
	m := queryExecutedMsg{timedOut: true}
	if !m.timedOut {
		t.Error("timedOut flag should be settable")
	}
	if m.cancelled {
		t.Error("cancelled should default to false")
	}
}

// queryContext applies a deadline when queryTimeout > 0, and the context
// expires after that duration.
func TestQueryContextAppliesTimeout(t *testing.T) {
	m := NewModel(&config.Config{})
	m.queryTimeout = 20 * time.Millisecond

	ctx, cancel := m.queryContext()
	defer cancel()

	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("expected a deadline on the context when queryTimeout > 0")
	}
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.DeadlineExceeded {
			t.Errorf("ctx.Err()=%v, want DeadlineExceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("context did not expire within 1s")
	}
}

// A zero queryTimeout disables the deadline (wait indefinitely); esc still
// works via the returned cancel func.
func TestQueryContextZeroDisablesTimeout(t *testing.T) {
	m := NewModel(&config.Config{})
	m.queryTimeout = 0

	ctx, cancel := m.queryContext()
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Fatal("expected no deadline when queryTimeout == 0")
	}
	// Must not expire on its own.
	select {
	case <-ctx.Done():
		t.Fatal("context should not expire without a deadline")
	case <-time.After(30 * time.Millisecond):
	}
	// Manual cancel still works (the esc path).
	cancel()
	if err := ctx.Err(); err != context.Canceled {
		t.Errorf("after cancel, ctx.Err()=%v, want Canceled", err)
	}
}

// The default model ships with a sensible non-zero query timeout so the TUI is
// protected out of the box.
func TestDefaultQueryTimeoutIsSet(t *testing.T) {
	m := NewModel(&config.Config{})
	if m.queryTimeout <= 0 {
		t.Fatalf("default queryTimeout=%v, want > 0", m.queryTimeout)
	}
	if m.queryTimeout != defaultQueryTimeout {
		t.Fatalf("default queryTimeout=%v, want %v", m.queryTimeout, defaultQueryTimeout)
	}
}

// A timedOut result surfaces a clear "timed out" message (not the opaque driver
// error) and clears the running state.
func TestQueryTimeoutShowsMessage(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath})
	if err != nil {
		t.Fatalf("new sqlite: %v", err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = conn
	m.state = stateWorkspace
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	m.queryRunning = true
	m.queryTimeout = 5 * time.Second

	res, _ := m.Update(queryExecutedMsg{
		err:      context.DeadlineExceeded,
		timedOut: true,
	})
	mm := res.(Model)

	if mm.queryRunning {
		t.Error("queryRunning should be cleared after the result arrives")
	}
	// SetError stores the message; surface it via the rendered view.
	view := mm.results.View()
	if !strings.Contains(strings.ToLower(view), "timed out") {
		t.Errorf("expected 'timed out' in results view, got: %s", view)
	}
	// The opaque raw error must not leak through.
	if strings.Contains(view, "context deadline exceeded") {
		t.Errorf("raw driver error leaked into view: %s", view)
	}
}

// A user-cancelled query still reports "cancelled" (the timeout path must not
// shadow it): cancelled takes precedence over timedOut.
func TestQueryCancelledStillReported(t *testing.T) {
	conn := sqliteConn(t)
	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = conn
	m.state = stateWorkspace
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	m.queryRunning = true
	m.queryCancelled = true // user hit esc

	res, _ := m.Update(queryExecutedMsg{err: context.Canceled, cancelled: true})
	mm := res.(Model)
	if view := strings.ToLower(mm.results.View()); !strings.Contains(view, "cancelled") {
		t.Errorf("expected 'cancelled' message, got: %s", view)
	}
}

// sqliteConn builds a connected throwaway SQLite connection for handler tests.
func sqliteConn(t *testing.T) *db.Connection {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath})
	if err != nil {
		t.Fatalf("new sqlite: %v", err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
