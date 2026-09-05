package db

import (
	"strings"
	"testing"
)

func TestTruncateQuery(t *testing.T) {
	if got := TruncateQuery("  select   *\n from  t  ", 100); got != "select * from t" {
		t.Fatalf("got %q", got)
	}
	got := TruncateQuery("abcdefghij", 5)
	if got != "abcd…" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatLockWaiterBlocker(t *testing.T) {
	if got := FormatLockWaiter("12", "app"); got != "12 (app)" {
		t.Fatalf("got %q", got)
	}
	if got := FormatLockBlocker("9", "root", "idle in transaction"); got != "9 (root) · idle in transaction" {
		t.Fatalf("got %q", got)
	}
	if got := FormatLockBlocker("9", "root", "active"); got != "9 (root)" {
		t.Fatalf("got %q", got)
	}
}

func TestParseSessionPID(t *testing.T) {
	if _, err := parseSessionPID("0"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := parseSessionPID("abc"); err == nil {
		t.Fatal("expected error")
	}
	id, err := parseSessionPID(" 42 ");
	if err != nil || id != 42 {
		t.Fatalf("got %d %v", id, err)
	}
}

func TestSQLiteLocksUnsupported(t *testing.T) {
	s := NewSQLite(ConnectionConfig{Driver: DriverSQLite, Database: ":memory:"})
	if err := s.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Locks(); err == nil || !strings.Contains(err.Error(), "SQLite") {
		t.Fatalf("got %v", err)
	}
	if err := s.KillSession("1"); err == nil || !strings.Contains(err.Error(), "SQLite") {
		t.Fatalf("got %v", err)
	}
}
