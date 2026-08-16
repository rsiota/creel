package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

// keepAliveInterval is how often the workspace pings MySQL/Postgres so idle
// kills and dead SSH tunnels are noticed before the next user query fails.
const keepAliveInterval = 30 * time.Second

// keepAliveTickMsg drives the periodic connection health check.
type keepAliveTickMsg struct{ gen uint64 }

// keepAliveFailMsg is emitted when a background Ping fails.
type keepAliveFailMsg struct{ err error }

// reconnectResultMsg is the outcome of an in-place reconnect attempt.
type reconnectResultMsg struct {
	conn *db.Connection
	err  error
}

// needsKeepAlive reports whether the active connection should be pinged in
// the background. SQLite is a local file — no tunnel/idle-server problem.
func (m Model) needsKeepAlive() bool {
	if m.connection == nil {
		return false
	}
	switch m.connection.Config().Driver {
	case db.DriverMySQL, db.DriverPostgres:
		return true
	default:
		return false
	}
}

// scheduleKeepAlive arms the next keep-alive tick for the current generation.
func (m *Model) scheduleKeepAlive() tea.Cmd {
	if !m.needsKeepAlive() {
		return nil
	}
	m.keepAliveGen++
	gen := m.keepAliveGen
	return tea.Tick(keepAliveInterval, func(time.Time) tea.Msg {
		return keepAliveTickMsg{gen: gen}
	})
}

// stopKeepAlive invalidates any in-flight keep-alive tick chain.
func (m *Model) stopKeepAlive() {
	m.keepAliveGen++
	m.reconnecting = false
	m.reconnectRetry = false
}

// pingConnection runs Ping on the active connection asynchronously.
func (m *Model) pingConnection() tea.Cmd {
	if m.connection == nil {
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		if err := conn.DB().Ping(); err != nil {
			return keepAliveFailMsg{err: err}
		}
		return nil
	}
}

// reconnectInPlace rebuilds the active connection (including SSH tunnel) from
// the current config without resetting the workspace. Open transactions are
// abandoned — they cannot survive a reconnect.
func (m *Model) reconnectInPlace() tea.Cmd {
	if m.connection == nil || m.reconnecting {
		return nil
	}
	cfg := m.connection.Config()
	old := m.connection
	m.rollbackTxn()
	m.reconnecting = true
	m.schemaMsg = "reconnecting…"
	return tea.Batch(spinnerTick(), func() tea.Msg {
		conn, err := db.New(cfg)
		if err != nil {
			return reconnectResultMsg{err: err}
		}
		if err := conn.Connect(); err != nil {
			_ = conn.Close()
			return reconnectResultMsg{err: err}
		}
		_ = old.Close()
		return reconnectResultMsg{conn: conn}
	})
}

// handleKeepAliveTick processes a keep-alive timer fire.
func (m Model) handleKeepAliveTick(msg keepAliveTickMsg) (Model, tea.Cmd) {
	if msg.gen != m.keepAliveGen {
		return m, nil
	}
	if !m.needsKeepAlive() {
		return m, nil
	}
	next := m.scheduleKeepAlive()
	if m.reconnecting || m.queryRunning || m.aiRunning {
		return m, next
	}
	return m, tea.Batch(m.pingConnection(), next)
}

// handleKeepAliveFail starts an in-place reconnect after a failed Ping.
func (m Model) handleKeepAliveFail(msg keepAliveFailMsg) (Model, tea.Cmd) {
	if m.connection == nil || m.reconnecting {
		return m, nil
	}
	if !db.IsConnError(msg.err) {
		// Unexpected non-connection Ping error — surface it, don't thrash.
		m.schemaMsg = fmt.Sprintf("connection check failed: %v", msg.err)
		return m, nil
	}
	m.reconnectRetry = false
	return m, m.reconnectInPlace()
}

// handleReconnectResult swaps in a fresh connection or reports failure.
func (m Model) handleReconnectResult(msg reconnectResultMsg) (Model, tea.Cmd) {
	m.reconnecting = false
	if msg.err != nil {
		m.schemaMsg = fmt.Sprintf("reconnect failed: %v", msg.err)
		m.reconnectRetry = false
		// Keep the existing connection pointer — Connect failed before swap,
		// so the prior (possibly dead) handle is still what we have.
		return m, nil
	}
	m.connection = msg.conn
	m.connError = ""
	retry := m.reconnectRetry
	m.reconnectRetry = false
	m.schemaMsg = "reconnected"
	var cmd tea.Cmd
	if retry && m.lastQuery != "" {
		cmd = m.runPageQuery()
	}
	if ka := m.scheduleKeepAlive(); ka != nil {
		if cmd != nil {
			cmd = tea.Batch(cmd, ka)
		} else {
			cmd = ka
		}
	}
	return m, cmd
}

// maybeReconnectOnError starts an in-place reconnect when err is a lost
// connection. When retry is true, the last page query is re-run after success.
// Returns (true, cmd) when reconnect was started.
func (m *Model) maybeReconnectOnError(err error, retry bool) (bool, tea.Cmd) {
	if err == nil || m.connection == nil || m.reconnecting {
		return false, nil
	}
	if !db.IsConnError(err) {
		return false, nil
	}
	m.reconnectRetry = retry
	return true, m.reconnectInPlace()
}
