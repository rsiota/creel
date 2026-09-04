package db

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildRemoteMysqlRestoreCmd(t *testing.T) {
	cfg := ConnectionConfig{
		Database: "spra_dev",
		Username: "u",
		Password: "p'ass",
		Host:     "127.0.0.1",
		Port:     3306,
	}
	cmd := buildRemoteMysqlRestoreCmd(cfg)
	for _, want := range []string{
		"mysql --defaults-extra-file=",
		"--host=127.0.0.1",
		"--port=3306",
		"'spra_dev'",
		"base64 -d",
		"command -v mysql",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in:\n%s", want, cmd)
		}
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(MysqlDumpDefaults(cfg)))
	if !strings.Contains(cmd, b64) {
		t.Fatal("defaults must be base64-encoded in remote cmd")
	}
	if strings.Contains(cmd, "mysqldump") {
		t.Fatal("restore cmd must not invoke mysqldump")
	}
}

func TestRemoteMysqlUnavailable(t *testing.T) {
	if !remoteMysqlUnavailable(fmtError("remote mysql: mysql not found on SSH host")) {
		t.Fatal("expected unavailable")
	}
	if remoteMysqlUnavailable(fmtError("remote mysql: Access denied")) {
		t.Fatal("access denied should not fall back as unavailable")
	}
}
