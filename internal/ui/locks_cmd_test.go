package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestLockJumpTable(t *testing.T) {
	if got := lockJumpTable("bookings.flights"); got != "flights" {
		t.Fatalf("got %q", got)
	}
	if got := lockJumpTable("orders"); got != "orders" {
		t.Fatalf("got %q", got)
	}
	if got := lockJumpTable(""); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestExLocksNotConnected(t *testing.T) {
	m := &Model{}
	if cmd := m.runExCommand("locks"); cmd != nil {
		t.Fatal("expected nil")
	}
	if m.schemaMsg != "not connected" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExLocksSQLite(t *testing.T) {
	m := &Model{connection: newSQLiteTestConn(t)}
	cmd := m.runExCommand("blocked")
	if cmd == nil {
		t.Fatal("expected async cmd")
	}
	msg, ok := cmd().(lookupResultMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "SQLite") {
		t.Fatalf("want SQLite error, got %v", msg.err)
	}
}

func TestExKillReadOnly(t *testing.T) {
	m := &Model{
		connection:    db.ConnectionFromConfig(db.ConnectionConfig{Driver: db.DriverPostgres, Database: "x"}),
		forceReadOnly: true,
	}
	if cmd := m.runExCommand("kill 12"); cmd != nil {
		t.Fatal("expected nil")
	}
	if !strings.Contains(m.schemaMsg, "read-only") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExKillNeedsPid(t *testing.T) {
	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverPostgres, Database: "x",
	})}
	if cmd := m.runExCommand("kill"); cmd != nil {
		t.Fatal("expected nil")
	}
	if m.schemaMsg != ":kill needs a session pid" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExKillConfirm(t *testing.T) {
	m := &Model{connection: newSQLiteTestConn(t)}
	// Default confirm_destructive is on.
	if cmd := m.runExCommand("kill 99"); cmd != nil {
		t.Fatal("expected nil while confirming")
	}
	if m.killConfirm != "99" {
		t.Fatalf("killConfirm = %q", m.killConfirm)
	}
	// Force skips confirm; SQLite rejects kill.
	m.killConfirm = ""
	cmd := m.runExCommand("kill! 99")
	if cmd == nil {
		t.Fatal("expected async kill")
	}
	msg := cmd().(killDoneMsg)
	if msg.pid != "99" {
		t.Fatalf("pid = %q", msg.pid)
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "SQLite") {
		t.Fatalf("want SQLite error, got %v", msg.err)
	}
}
