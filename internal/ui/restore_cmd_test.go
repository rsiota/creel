package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rsiota/creel/internal/db"
)

func TestRestoreProgressStatus(t *testing.T) {
	started := time.Now().Add(-2 * time.Second)
	got := restoreProgressStatus(20*1024*1024, started)
	if !strings.Contains(got, "Restoring…") || !strings.Contains(got, "MB") || !strings.Contains(got, "/s") {
		t.Fatalf("got %q", got)
	}
	got = restoreProgressStatus(100, time.Time{})
	if got != "Restoring… 100B" {
		t.Fatalf("got %q", got)
	}
}

func TestExRestoreNotConnected(t *testing.T) {
	m := &Model{}
	cmd := m.runExCommand("restore /tmp/x.sql")
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if m.schemaMsg != "not connected" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExRestoreNeedsPath(t *testing.T) {
	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "app",
	})}
	cmd := m.runExCommand("restore")
	if cmd != nil {
		t.Fatal("expected nil")
	}
	if m.schemaMsg != ":restore needs a file path" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExRestoreSQLite(t *testing.T) {
	m := &Model{connection: newSQLiteTestConn(t)}
	cmd := m.runExCommand("restore /tmp/x.sql")
	if cmd != nil {
		t.Fatal("expected nil")
	}
	if !strings.Contains(m.schemaMsg, "mysql") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExRestoreReadOnly(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "x.sql")
	if err := os.WriteFile(dump, []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{
		connection: db.ConnectionFromConfig(db.ConnectionConfig{
			Driver: db.DriverMySQL, Database: "app",
		}),
		forceReadOnly: true,
	}
	cmd := m.runExCommand("restore " + dump)
	if cmd != nil {
		t.Fatal("expected nil")
	}
	if !strings.Contains(m.schemaMsg, "read-only") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExRestoreSSHLocalMySQLSkipsLocalPATH(t *testing.T) {
	restore := db.SwapLookPathMysql(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restore)

	dump := filepath.Join(t.TempDir(), "x.sql")
	if err := os.WriteFile(dump, []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "app", Host: "127.0.0.1",
		SSHHost: "bastion",
	})}
	cmd := m.runExCommand("restore " + dump)
	if cmd == nil {
		t.Fatalf("expected async cmd, schemaMsg=%q", m.schemaMsg)
	}
	msg, ok := cmd().(restoreDoneMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "SSH") {
		t.Fatalf("want SSH/tunnel error without local PATH check, got %v", msg.err)
	}
}

func TestExRestoreMissingPATH(t *testing.T) {
	restore := db.SwapLookPathMysql(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restore)

	dump := filepath.Join(t.TempDir(), "x.sql")
	if err := os.WriteFile(dump, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "shop", Host: "127.0.0.1",
	})}
	cmd := m.runExCommand("mysqlload " + dump)
	if cmd != nil {
		t.Fatal("expected nil")
	}
	if m.schemaMsg != "mysql is not on PATH" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExRestoreMissingFile(t *testing.T) {
	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "shop", Host: "127.0.0.1",
	})}
	cmd := m.runExCommand("restore " + filepath.Join(t.TempDir(), "nope.sql"))
	if cmd != nil {
		t.Fatal("expected nil")
	}
	if m.schemaMsg == "" {
		t.Fatal("expected file error")
	}
}
