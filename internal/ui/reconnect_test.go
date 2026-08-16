package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestMaybeReconnectOnError(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor()}

	ok, cmd := m.maybeReconnectOnError(fmt.Errorf("syntax error"), true)
	if ok || cmd != nil {
		t.Fatal("SQL errors must not trigger reconnect")
	}

	ok, cmd = m.maybeReconnectOnError(fmt.Errorf("driver: bad connection"), true)
	if !ok || cmd == nil {
		t.Fatal("connection errors should trigger reconnect")
	}
	if !m.reconnecting {
		t.Fatal("expected reconnecting flag")
	}
	if !m.reconnectRetry {
		t.Fatal("expected retry flag")
	}
	if !strings.Contains(m.schemaMsg, "reconnecting") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	// Second call while already reconnecting is a no-op.
	ok, cmd = m.maybeReconnectOnError(fmt.Errorf("connection reset"), true)
	if ok || cmd != nil {
		t.Fatal("should not stack reconnects")
	}
}

func TestReconnectResultPreservesWorkspace(t *testing.T) {
	old := newSQLiteTestConn(t)
	defer old.Close()
	fresh := newSQLiteTestConn(t)
	defer fresh.Close()

	m := &Model{
		connection:   old,
		results:      NewResultsTable(),
		editor:       NewQueryEditor(),
		lastQuery:    "SELECT * FROM users",
		reconnecting: true,
		page:         2,
	}
	m.editor.SetValue("SELECT * FROM users")
	m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")

	updated, cmd := m.Update(reconnectResultMsg{conn: fresh})
	mm := updated.(Model)
	if mm.reconnecting {
		t.Fatal("reconnecting should clear on success")
	}
	if mm.connection != fresh {
		t.Fatal("expected new connection")
	}
	if mm.lastQuery != "SELECT * FROM users" {
		t.Errorf("lastQuery = %q", mm.lastQuery)
	}
	if mm.page != 2 {
		t.Errorf("page = %d, want 2", mm.page)
	}
	if mm.editor.Value() != "SELECT * FROM users" {
		t.Errorf("editor cleared: %q", mm.editor.Value())
	}
	if !strings.Contains(mm.schemaMsg, "reconnected") {
		t.Errorf("schemaMsg = %q", mm.schemaMsg)
	}
	_ = cmd
}

func TestReconnectResultFailureKeepsConnection(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{connection: conn, reconnecting: true, results: NewResultsTable()}
	updated, _ := m.Update(reconnectResultMsg{err: fmt.Errorf("ssh dial failed")})
	mm := updated.(Model)
	if mm.connection != conn {
		t.Fatal("failed reconnect should keep the prior connection handle")
	}
	if mm.reconnecting {
		t.Fatal("reconnecting should clear on failure")
	}
	if !strings.Contains(mm.schemaMsg, "reconnect failed") {
		t.Errorf("schemaMsg = %q", mm.schemaMsg)
	}
}

func TestQueryErrorTriggersReconnect(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := Model{
		connection: conn,
		results:    NewResultsTable(),
		editor:     NewQueryEditor(),
		lastQuery:  "SELECT 1",
		state:      stateWorkspace,
	}
	updated, cmd := m.Update(queryExecutedMsg{
		query: "SELECT 1",
		err:   fmt.Errorf("read: connection reset by peer"),
	})
	mm := updated.(Model)
	if !mm.reconnecting {
		t.Fatal("query connection error should start reconnect")
	}
	if !mm.reconnectRetry {
		t.Fatal("query reconnect should retry the page")
	}
	if cmd == nil {
		t.Fatal("expected reconnect cmd")
	}
	if mm.results.Message() != "" && strings.Contains(mm.results.Message(), "connection reset") {
		t.Error("should not dump the raw error into results when reconnecting")
	}
}

func TestNeedsKeepAlive(t *testing.T) {
	m := Model{}
	if m.needsKeepAlive() {
		t.Fatal("nil connection")
	}
	sqlite := newSQLiteTestConn(t)
	defer sqlite.Close()
	m.connection = sqlite
	if m.needsKeepAlive() {
		t.Fatal("sqlite should not keep-alive ping")
	}
}

func TestExReconnectRejectsSQLite(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{connection: conn}
	if cmd := m.exReconnect(); cmd != nil {
		t.Fatal("sqlite :reconnect should be a no-op cmd")
	}
	if !strings.Contains(m.schemaMsg, "MySQL/Postgres") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestStatusMessageReconnecting(t *testing.T) {
	m := Model{reconnecting: true, querySpinner: 0}
	got := m.statusMessage()
	if !strings.Contains(ansiStrip.ReplaceAllString(got, ""), "reconnecting") {
		t.Errorf("status = %q", got)
	}
}

func TestKeepAliveTickStaleGen(t *testing.T) {
	m := Model{keepAliveGen: 5}
	updated, cmd := m.handleKeepAliveTick(keepAliveTickMsg{gen: 1})
	if cmd != nil {
		t.Fatal("stale tick should be ignored")
	}
	if updated.keepAliveGen != 5 {
		t.Errorf("gen changed: %d", updated.keepAliveGen)
	}
}
