package ui

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestShowImportErrorOverlay(t *testing.T) {
	m := &Model{}
	m.showImportErrorOverlay("dump.sql", []db.ImportError{
		{Statement: "INSERT INTO t VALUES (1)", Err: errors.New("duplicate key")},
		{Statement: "BAD", Err: errors.New("syntax")},
	})
	if !m.lookupPanel.IsVisible() {
		t.Fatal("expected overlay")
	}
	if !strings.Contains(m.lookupPanel.title, "Import errors") {
		t.Fatalf("title = %q", m.lookupPanel.title)
	}
	if got := len(m.lookupPanel.result.Rows); got != 2 {
		t.Fatalf("rows = %d", got)
	}
}

func TestShowRestoreErrorOverlay(t *testing.T) {
	m := &Model{}
	m.showRestoreErrorOverlay("/tmp/x.sql", []string{"ERROR 1064", "ERROR 1146"})
	if !m.lookupPanel.IsVisible() {
		t.Fatal("expected overlay")
	}
	if got := len(m.lookupPanel.result.Rows); got != 2 {
		t.Fatalf("rows = %d", got)
	}
}

func TestRestoreDoneMsgOpensOverlay(t *testing.T) {
	m := Model{width: 120, height: 40}
	next, _ := m.update(restoreDoneMsg{
		path:         "/tmp/dump.sql",
		bytes:        1024,
		clientStderr: "ERROR 1064 at line 1\nERROR 1146 at line 2\n",
		continued:    true,
	})
	mm := next.(Model)
	if !mm.lookupPanel.IsVisible() {
		t.Fatal("continued restore with stderr should open overlay")
	}
	if !strings.Contains(mm.exportMsg, "2 errors") {
		t.Fatalf("exportMsg = %q", mm.exportMsg)
	}
}

func TestRestoreDoneMsgStopHintsBang(t *testing.T) {
	m := Model{}
	next, _ := m.update(restoreDoneMsg{
		path: "/tmp/dump.sql",
		err:  errors.New("mysql: ERROR 1064"),
	})
	mm := next.(Model)
	if !strings.Contains(mm.exportMsg, ":restore!") {
		t.Fatalf("exportMsg = %q", mm.exportMsg)
	}
}

func TestImportDoneMsgOpensOverlay(t *testing.T) {
	m := Model{width: 120, height: 40}
	next, _ := m.update(importDoneMsg{
		filename: "x.sql",
		result: db.ImportResult{
			Statements: 3,
			Errors: []db.ImportError{
				{Statement: "BAD", Err: errors.New("nope")},
			},
		},
	})
	mm := next.(Model)
	if !mm.lookupPanel.IsVisible() {
		t.Fatal("import errors should open overlay")
	}
	if !strings.Contains(mm.exportMsg, "review overlay") {
		t.Fatalf("exportMsg = %q", mm.exportMsg)
	}
}

func TestExRestoreBangContinuesOnError(t *testing.T) {
	restorePath := db.SwapLookPathMysql(func(string) (string, error) {
		return "/usr/bin/mysql", nil
	})
	t.Cleanup(restorePath)

	dump := filepath.Join(t.TempDir(), "x.sql")
	if err := os.WriteFile(dump, []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotArgs []string
	restoreRun := db.SwapRunMysqlCmd(func(cmd *exec.Cmd) error {
		gotArgs = append([]string{}, cmd.Args[1:]...)
		if cmd.Stdin != nil {
			_, _ = io.Copy(io.Discard, cmd.Stdin)
		}
		if cmd.Stderr != nil {
			_, _ = cmd.Stderr.Write([]byte("ERROR 1064\n"))
		}
		return errors.New("exit status 1")
	})
	t.Cleanup(restoreRun)

	m := &Model{connection: db.ConnectionFromConfig(db.ConnectionConfig{
		Driver: db.DriverMySQL, Database: "shop", Host: "127.0.0.1",
	})}
	cmd := m.runExCommand("restore! " + dump)
	if cmd == nil {
		t.Fatalf("expected async cmd, schemaMsg=%q", m.schemaMsg)
	}
	var msg restoreDoneMsg
	for i := 0; i < 20; i++ {
		switch v := cmd().(type) {
		case restoreDoneMsg:
			msg = v
			goto done
		case restoreProgressWrapper:
			cmd = waitForRestoreProgress(v.progress, v.done)
		default:
			t.Fatalf("unexpected %T", v)
		}
	}
	t.Fatal("timed out waiting for restoreDoneMsg")
done:
	if msg.err != nil {
		t.Fatalf("want soft success, got %v", msg.err)
	}
	if !msg.continued {
		t.Fatal("continued = false")
	}
	if !strings.Contains(strings.Join(gotArgs, " "), "--force") {
		t.Fatalf("args = %v", gotArgs)
	}
	if !strings.Contains(msg.clientStderr, "ERROR 1064") {
		t.Fatalf("stderr = %q", msg.clientStderr)
	}
}
