package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestExBackupNotConnected(t *testing.T) {
	m := &Model{}
	cmd := m.runExCommand("backup")
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	if m.schemaMsg != "not connected" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExBackupSQLite(t *testing.T) {
	m := &Model{connection: newSQLiteTestConn(t)}
	cmd := m.runExCommand("backup")
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	if !strings.Contains(m.schemaMsg, "mysqldump") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExBackupPostgres(t *testing.T) {
	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverPostgres, Database: "app",
	})}
	cmd := m.runExCommand("mysqldump")
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if !strings.Contains(m.schemaMsg, "mysqldump") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExBackupSSHUsesPATHNotRefusal(t *testing.T) {
	restore := db.SwapLookPathMysqlDump(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restore)

	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "app", SSHHost: "bastion",
	})}
	cmd := m.runExCommand("backup")
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	// SSH is allowed; without mysqldump we still fail on PATH.
	if m.schemaMsg != "mysqldump is not on PATH" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExBackupSSHNeedsLiveTunnel(t *testing.T) {
	restorePath := db.SwapLookPathMysqlDump(func(string) (string, error) {
		return "/usr/bin/mysqldump", nil
	})
	t.Cleanup(restorePath)

	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "app", Host: "db.internal",
		SSHHost: "bastion",
	})}
	cmd := m.runExCommand("backup")
	if cmd == nil {
		t.Fatal("expected async backup cmd")
	}
	if m.exportMsg != "Backing up…" {
		t.Fatalf("exportMsg = %q", m.exportMsg)
	}
	msg, ok := cmd().(backupDoneMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "SSH") {
		t.Fatalf("want SSH/tunnel error, got %v", msg.err)
	}
}

func TestExBackupMissingPATH(t *testing.T) {
	restore := db.SwapLookPathMysqlDump(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restore)

	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "shop", Host: "127.0.0.1",
	})}
	cmd := m.runExCommand("backup")
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if m.schemaMsg != "mysqldump is not on PATH" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}
