package db

import (
	"strings"
	"testing"
)

func TestMysqlHostOnSSHTarget(t *testing.T) {
	for _, h := range []string{"", "localhost", "127.0.0.1", "::1", "LOCALHOST"} {
		if !MysqlHostOnSSHTarget(h) {
			t.Errorf("%q should be on SSH target", h)
		}
	}
	if MysqlHostOnSSHTarget("db.internal") {
		t.Fatal("internal hostname is not loopback")
	}
}

func TestBuildRemoteMysqlDumpScript(t *testing.T) {
	script := buildRemoteMysqlDumpScript(ConnectionConfig{
		Database: "spra_dev",
		Username: "u",
		Password: "p'ass",
		Host:     "127.0.0.1",
		Port:     3306,
	})
	for _, want := range []string{
		"mysqldump --defaults-extra-file=",
		"--host=127.0.0.1",
		"--port=3306",
		"'spra_dev'",
		"password=",
		"command -v mysqldump",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("missing %q in:\n%s", want, script)
		}
	}
	if strings.Contains(script, "--result-file=") {
		t.Fatal("remote dump must stream stdout, not --result-file")
	}
}

func TestShellSingleQuote(t *testing.T) {
	if got := shellSingleQuote(`a'b`); got != `'a'"'"'b'` {
		t.Fatalf("got %q", got)
	}
}

func TestRemoteDumpUnavailable(t *testing.T) {
	if !remoteDumpUnavailable(fmtError("remote mysqldump: mysqldump not found on SSH host")) {
		t.Fatal("expected unavailable")
	}
	if remoteDumpUnavailable(fmtError("remote mysqldump: Access denied")) {
		t.Fatal("access denied should not fall back silently as unavailable")
	}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
