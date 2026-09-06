package ui

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

func TestBackupProgressStatus(t *testing.T) {
	started := time.Now().Add(-2 * time.Second)
	got := backupProgressStatus(20*1024*1024, started)
	if !strings.Contains(got, "Backing up…") || !strings.Contains(got, "MB") || !strings.Contains(got, "/s") {
		t.Fatalf("got %q", got)
	}
	got = backupProgressStatus(100, time.Time{})
	if got != "Backing up… 100B" {
		t.Fatalf("got %q", got)
	}
}

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
	if !strings.Contains(m.schemaMsg, "mysqldump") && !strings.Contains(m.schemaMsg, "pg_dump") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExBackupPostgresMissingPATH(t *testing.T) {
	restore := db.SwapLookPathPgDump(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restore)

	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverPostgres, Database: "app", Host: "127.0.0.1",
	})}
	cmd := m.runExCommand("backup")
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if m.schemaMsg != "pg_dump is not on PATH" {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExBackupPostgresSSHLoadsPicker(t *testing.T) {
	restore := db.SwapLookPathPgDump(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restore)

	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverPostgres, Database: "app", Host: "127.0.0.1",
		SSHHost: "bastion",
	})}
	cmd := m.runExCommand("pg_dump")
	if cmd == nil {
		t.Fatalf("expected sizes cmd, schemaMsg=%q", m.schemaMsg)
	}
	msg, ok := cmd().(backupPickerMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	// No live DB — picker msg should report not connected.
	if msg.err == nil || !strings.Contains(msg.err.Error(), "not connected") {
		t.Fatalf("want not connected, got %v", msg.err)
	}
}

func TestExBackupSSHLocalMySQLLoadsPicker(t *testing.T) {
	restore := db.SwapLookPathMysqlDump(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restore)

	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "app", Host: "127.0.0.1",
		SSHHost: "bastion",
	})}
	cmd := m.runExCommand("backup")
	if cmd == nil {
		t.Fatalf("expected sizes cmd, schemaMsg=%q", m.schemaMsg)
	}
	msg, ok := cmd().(backupPickerMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "not connected") {
		t.Fatalf("want not connected, got %v", msg.err)
	}
}

func TestExBackupSSHNeedsLiveTunnelStillOpensPickerPath(t *testing.T) {
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
		t.Fatal("expected sizes/picker cmd")
	}
	msg, ok := cmd().(backupPickerMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if msg.bin != "/usr/bin/mysqldump" {
		t.Fatalf("bin = %q", msg.bin)
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "not connected") {
		t.Fatalf("want not connected, got %v", msg.err)
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

func TestExecNativeBackupFull(t *testing.T) {
	restorePath := db.SwapLookPathMysqlDump(func(string) (string, error) {
		return "/usr/bin/mysqldump", nil
	})
	t.Cleanup(restorePath)
	restoreHelp := db.SwapRunMysqlDumpHelp(func(string) ([]byte, error) {
		return []byte("no column stats"), nil
	})
	t.Cleanup(restoreHelp)
	restoreRun := db.SwapRunMysqlDumpCmd(func(cmd *exec.Cmd) error {
		_, _ = cmd.Stdout.Write([]byte("-- dump\n"))
		return nil
	})
	t.Cleanup(restoreRun)

	dir := t.TempDir()
	restoreDL := SwapUserDownloadsDir(func() (string, error) { return dir, nil })
	t.Cleanup(restoreDL)

	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "shop", Host: "127.0.0.1",
	})}
	cmd := m.execNativeBackup("/usr/bin/mysqldump", db.DumpPlan{})
	if cmd == nil {
		t.Fatal("expected backup cmd")
	}
	if m.exportMsg != "Backing up…" {
		t.Fatalf("exportMsg = %q", m.exportMsg)
	}
	msg := drainBackupCmd(t, cmd)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if msg.bytes == 0 {
		t.Fatal("expected dumped bytes")
	}
}

func drainBackupCmd(t *testing.T, cmd tea.Cmd) backupDoneMsg {
	t.Helper()
	for i := 0; i < 20; i++ {
		raw := cmd()
		switch msg := raw.(type) {
		case backupDoneMsg:
			return msg
		case backupProgressWrapper:
			cmd = waitForBackupProgress(msg.progress, msg.done)
		default:
			t.Fatalf("unexpected msg %T", raw)
		}
	}
	t.Fatal("backup did not finish")
	return backupDoneMsg{}
}
