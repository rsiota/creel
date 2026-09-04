package db

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMysqlRestoreGuard(t *testing.T) {
	if err := MysqlRestoreGuard(ConnectionConfig{Driver: DriverSQLite, Database: "x"}); err == nil {
		t.Fatal("sqlite should be rejected")
	}
	if err := MysqlRestoreGuard(ConnectionConfig{Driver: DriverMySQL}); err == nil {
		t.Fatal("empty database should be rejected")
	}
	if err := MysqlRestoreGuard(ConnectionConfig{
		Driver: DriverMySQL, Database: "app", ReadOnly: true,
	}); err == nil {
		t.Fatal("read-only should be rejected")
	}
	if err := MysqlRestoreGuard(ConnectionConfig{
		Driver: DriverMySQL, Database: "app", SSHHost: "bastion",
	}); err != nil {
		t.Fatalf("SSH should be allowed: %v", err)
	}
}

func TestBuildMysqlArgsOmitsPassword(t *testing.T) {
	cfg := ConnectionConfig{
		Driver:   DriverMySQL,
		Database: "shop",
		Host:     "db.example",
		Port:     3307,
		Username: "root",
		Password: "s3cret",
		SSLMode:  "require",
	}
	args := BuildMysqlArgs(cfg, "/tmp/c.cnf", false)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "s3cret") {
		t.Fatalf("password leaked onto argv: %v", args)
	}
	if args[0] != "--defaults-extra-file=/tmp/c.cnf" {
		t.Fatalf("defaults file must be first: %v", args)
	}
	for _, want := range []string{
		"--protocol=TCP",
		"--host=db.example",
		"--port=3307",
		"--ssl-mode=REQUIRED",
		"--max-allowed-packet=1073741824",
		"shop",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
	if strings.Contains(joined, "--compress") {
		t.Fatalf("compress is SSH-only: %v", args)
	}
	if strings.Contains(joined, "--single-transaction") {
		t.Fatalf("restore should not pass dump-only flags: %v", args)
	}
}

func TestBuildMysqlArgsSSHCompress(t *testing.T) {
	args := BuildMysqlArgs(ConnectionConfig{
		Driver: DriverMySQL, Database: "shop", Host: "127.0.0.1", Port: 3306,
	}, "/tmp/c.cnf", true)
	if !strings.Contains(strings.Join(args, " "), "--compress") {
		t.Fatalf("SSH restore should compress: %v", args)
	}
}

func TestRunMysqlRestoreLocal(t *testing.T) {
	restorePath := SwapLookPathMysql(func(string) (string, error) {
		return "/usr/bin/mysql", nil
	})
	t.Cleanup(restorePath)

	dump := filepath.Join(t.TempDir(), "in.sql")
	if err := os.WriteFile(dump, []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotArgs []string
	var gotStdin string
	restoreRun := SwapRunMysqlCmd(func(cmd *exec.Cmd) error {
		gotArgs = append([]string{}, cmd.Args[1:]...)
		buf := new(bytes.Buffer)
		if cmd.Stdin != nil {
			_, _ = buf.ReadFrom(cmd.Stdin)
		}
		gotStdin = buf.String()
		return nil
	})
	t.Cleanup(restoreRun)

	cfg := ConnectionConfig{
		Driver: DriverMySQL, Database: "shop", Host: "127.0.0.1", Port: 3306,
		Username: "u", Password: "s3cret-restore",
	}
	var progress []int64
	if err := RunMysqlRestore("/usr/bin/mysql", cfg, dump, nil, func(n int64) {
		progress = append(progress, n)
	}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(gotArgs, " ")
	if strings.Contains(joined, "s3cret-restore") {
		t.Fatalf("password on argv: %v", gotArgs)
	}
	if !strings.HasPrefix(gotArgs[0], "--defaults-extra-file=") {
		t.Fatalf("defaults first: %v", gotArgs)
	}
	if gotStdin != "SELECT 1;\n" {
		t.Fatalf("stdin = %q", gotStdin)
	}
	if len(progress) == 0 || progress[len(progress)-1] != int64(len("SELECT 1;\n")) {
		t.Fatalf("progress = %v", progress)
	}
}

func TestRunMysqlRestoreMissingBinary(t *testing.T) {
	restorePath := SwapLookPathMysql(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restorePath)

	dump := filepath.Join(t.TempDir(), "in.sql")
	_ = os.WriteFile(dump, []byte("x"), 0o644)
	err := RunMysqlRestore("", ConnectionConfig{
		Driver: DriverMySQL, Database: "shop", Host: "127.0.0.1",
	}, dump, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("got %v", err)
	}
}

func TestCountingReader(t *testing.T) {
	var got []int64
	r := &countingReader{
		r:       strings.NewReader("abcdef"),
		onBytes: func(n int64) { got = append(got, n) },
	}
	buf := make([]byte, 3)
	_, _ = r.Read(buf)
	_, _ = r.Read(buf)
	if len(got) != 2 || got[0] != 3 || got[1] != 6 {
		t.Fatalf("got %v", got)
	}
}
