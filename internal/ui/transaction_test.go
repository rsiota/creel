package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

// newSQLiteTestConn opens a fresh SQLite database in a per-test temp dir.
func newSQLiteTestConn(t *testing.T) *db.Connection {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: path})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return conn
}

// --- guard logic (no database needed) ---

func TestExCommitNoTxn(t *testing.T) {
	m := &Model{}
	m.runExCommand("commit")
	if !strings.Contains(m.schemaMsg, "no transaction") {
		t.Errorf(":commit with no tx -> %q", m.schemaMsg)
	}
}

func TestExRollbackNoTxn(t *testing.T) {
	m := &Model{}
	m.runExCommand("rollback")
	if !strings.Contains(m.schemaMsg, "no transaction") {
		t.Errorf(":rollback with no tx -> %q", m.schemaMsg)
	}
}

func TestExBeginNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("begin")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":begin with no connection -> %q", m.schemaMsg)
	}
	if m.tx != nil {
		t.Error(":begin should not open a tx when not connected")
	}
}

func TestExBeginReadOnly(t *testing.T) {
	m := &Model{connection: &db.Connection{}, forceReadOnly: true}
	m.runExCommand("begin")
	if !strings.Contains(m.schemaMsg, "read-only") {
		t.Errorf(":begin in read-only mode -> %q", m.schemaMsg)
	}
	if m.tx != nil {
		t.Error(":begin should not open a tx in read-only mode")
	}
}

// --- live round-trip against SQLite ---

func TestExTransactionLive(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()

	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor()}

	// :begin opens a transaction.
	m.runExCommand("begin")
	if m.tx == nil {
		t.Fatalf(":begin did not open a transaction: %q", m.schemaMsg)
	}
	if !strings.Contains(m.schemaMsg, "started") {
		t.Errorf(":begin -> %q", m.schemaMsg)
	}

	// The status bar advertises the active transaction.
	if !strings.Contains(m.statusBar(""), "TXN") {
		t.Error("status bar should show the TXN indicator while a tx is active")
	}

	// A second :begin is refused.
	m.runExCommand("begin")
	if !strings.Contains(m.schemaMsg, "already in progress") {
		t.Errorf("second :begin -> %q", m.schemaMsg)
	}

	// Cell edits are refused for the duration (they'd commit outside the tx).
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "alice"}}, "")
	m.results.SetDirtyCell(0, 1, "bob")
	if cmd := m.saveEdits(); cmd != nil {
		t.Errorf("saveEdits during tx should return nil, got %v", cmd)
	}
	if !strings.Contains(m.schemaMsg, "transaction active") {
		t.Errorf("saveEdits during tx -> %q", m.schemaMsg)
	}

	// :commit finishes it.
	m.runExCommand("commit")
	if m.tx != nil {
		t.Error(":commit should clear the transaction")
	}
	if !strings.Contains(m.schemaMsg, "committed") {
		t.Errorf(":commit -> %q", m.schemaMsg)
	}
	if strings.Contains(m.statusBar(""), "TXN") {
		t.Error("status bar should drop the TXN indicator after commit")
	}
}

func TestExTransactionRollback(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()

	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor()}

	m.runExCommand("begin")
	if m.tx == nil {
		t.Fatalf(":begin did not open a transaction: %q", m.schemaMsg)
	}
	m.runExCommand("rollback")
	if m.tx != nil {
		t.Error(":rollback should clear the transaction")
	}
	if !strings.Contains(m.schemaMsg, "rolled back") {
		t.Errorf(":rollback -> %q", m.schemaMsg)
	}
}

func TestExBeginIsolation(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor()}

	m.runExCommand("begin bogosity")
	if m.tx != nil {
		t.Fatal("bad isolation should not open a tx")
	}
	if !strings.Contains(m.schemaMsg, "unknown isolation") {
		t.Errorf("bad isolation -> %q", m.schemaMsg)
	}

	// SQLite accepts serializable (its effective level).
	m.runExCommand("begin serializable")
	if m.tx == nil {
		t.Fatalf(":begin serializable failed: %q", m.schemaMsg)
	}
	if m.txIsolation != db.IsolationSerializable {
		t.Errorf("txIsolation = %v", m.txIsolation)
	}
	if !strings.Contains(m.schemaMsg, "serializable") {
		t.Errorf(":begin serializable -> %q", m.schemaMsg)
	}
	bar := m.statusBar("")
	if !strings.Contains(bar, "TXN S") {
		t.Errorf("status bar = %q, want TXN S", bar)
	}
	m.runExCommand("rollback")

	m.runExCommand("begin rr")
	if m.tx == nil {
		t.Fatalf(":begin rr failed: %q", m.schemaMsg)
	}
	if m.txIsolation != db.IsolationRepeatableRead {
		t.Errorf("txIsolation = %v, want RR", m.txIsolation)
	}
	if !strings.Contains(m.statusBar(""), "TXN RR") {
		t.Errorf("status bar missing TXN RR: %q", m.statusBar(""))
	}
	m.runExCommand("commit")
	if m.txIsolation != db.IsolationDefault {
		t.Error("commit should clear txIsolation")
	}
}

func TestExBeginRepeatableReadSpaced(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{connection: conn, results: NewResultsTable()}
	m.runExCommand("begin repeatable read")
	if m.tx == nil {
		t.Fatalf(":begin repeatable read failed: %q", m.schemaMsg)
	}
	if m.txIsolation != db.IsolationRepeatableRead {
		t.Errorf("got %v", m.txIsolation)
	}
	m.runExCommand("rollback")
}

func TestTxnStatusLabel(t *testing.T) {
	if got := txnStatusLabel(db.IsolationDefault); got != "TXN ●" {
		t.Errorf("default = %q", got)
	}
	if got := txnStatusLabel(db.IsolationSerializable); got != "TXN S" {
		t.Errorf("serializable = %q", got)
	}
}
