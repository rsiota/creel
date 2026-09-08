package db

import (
	"bytes"
	"errors"
	"io"
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
	args := BuildMysqlArgs(cfg, "/tmp/c.cnf", false, false)
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
	}, "/tmp/c.cnf", true, false)
	if !strings.Contains(strings.Join(args, " "), "--compress") {
		t.Fatalf("SSH restore should compress: %v", args)
	}
}

func TestBuildMysqlArgsForce(t *testing.T) {
	args := BuildMysqlArgs(ConnectionConfig{
		Driver: DriverMySQL, Database: "shop", Host: "127.0.0.1",
	}, "/tmp/c.cnf", false, true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--force") {
		t.Fatalf("continue-on-error should pass --force: %v", args)
	}
	if args[len(args)-1] != "shop" {
		t.Fatalf("database must remain last: %v", args)
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
	res := RunMysqlRestore("/usr/bin/mysql", cfg, dump, nil, RestoreOpts{
		OnBytes: func(n int64) { progress = append(progress, n) },
	})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	joined := strings.Join(gotArgs, " ")
	if strings.Contains(joined, "s3cret-restore") {
		t.Fatalf("password on argv: %v", gotArgs)
	}
	if !strings.HasPrefix(gotArgs[0], "--defaults-extra-file=") {
		t.Fatalf("defaults first: %v", gotArgs)
	}
	if strings.Contains(joined, "--force") {
		t.Fatalf("default restore must not force: %v", gotArgs)
	}
	if gotStdin != "SELECT 1;\n" {
		t.Fatalf("stdin = %q", gotStdin)
	}
	if len(progress) == 0 || progress[len(progress)-1] != int64(len("SELECT 1;\n")) {
		t.Fatalf("progress = %v", progress)
	}
}

func TestRunMysqlRestoreContinueOnError(t *testing.T) {
	restorePath := SwapLookPathMysql(func(string) (string, error) {
		return "/usr/bin/mysql", nil
	})
	t.Cleanup(restorePath)

	dump := filepath.Join(t.TempDir(), "in.sql")
	if err := os.WriteFile(dump, []byte("BAD;\nGOOD;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotArgs []string
	restoreRun := SwapRunMysqlCmd(func(cmd *exec.Cmd) error {
		gotArgs = append([]string{}, cmd.Args[1:]...)
		if cmd.Stdin != nil {
			_, _ = io.Copy(io.Discard, cmd.Stdin)
		}
		if cmd.Stderr != nil {
			_, _ = cmd.Stderr.Write([]byte("ERROR 1064 at line 1: syntax\n"))
		}
		return errors.New("exit status 1")
	})
	t.Cleanup(restoreRun)

	res := RunMysqlRestore("/usr/bin/mysql", ConnectionConfig{
		Driver: DriverMySQL, Database: "shop", Host: "127.0.0.1",
	}, dump, nil, RestoreOpts{ContinueOnError: true})
	if res.Err != nil {
		t.Fatalf("continue-on-error should soft-succeed, got %v", res.Err)
	}
	if !res.Continued {
		t.Fatal("Continued = false")
	}
	if !strings.Contains(res.ClientStderr, "ERROR 1064") {
		t.Fatalf("stderr = %q", res.ClientStderr)
	}
	if !strings.Contains(strings.Join(gotArgs, " "), "--force") {
		t.Fatalf("want --force in %v", gotArgs)
	}
	if res.Bytes == 0 {
		t.Fatal("expected bytes read")
	}
}

func TestRunMysqlRestoreMissingBinary(t *testing.T) {
	restorePath := SwapLookPathMysql(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restorePath)

	dump := filepath.Join(t.TempDir(), "in.sql")
	_ = os.WriteFile(dump, []byte("x"), 0o644)
	res := RunMysqlRestore("", ConnectionConfig{
		Driver: DriverMySQL, Database: "shop", Host: "127.0.0.1",
	}, dump, nil, RestoreOpts{})
	if res.Err == nil || !strings.Contains(res.Err.Error(), "not on PATH") {
		t.Fatalf("got %v", res.Err)
	}
}

func TestFinalizeCLIRestore(t *testing.T) {
	soft := finalizeCLIRestore("mysql", true, 100, "ERROR 1\n", errors.New("exit 1"))
	if soft.Err != nil || soft.ClientStderr != "ERROR 1" {
		t.Fatalf("soft = %+v", soft)
	}
	hard := finalizeCLIRestore("mysql", false, 100, "ERROR 1\n", errors.New("exit 1"))
	if hard.Err == nil || !strings.Contains(hard.Err.Error(), "ERROR 1") {
		t.Fatalf("hard = %+v", hard)
	}
	setup := finalizeCLIRestore("mysql", true, 0, "", errors.New("exec: not found"))
	if setup.Err == nil {
		t.Fatal("empty stderr + 0 bytes should stay hard")
	}
	missing := finalizeCLIRestore("remote mysql", true, 0, "mysql not found on SSH host", errors.New("exit 127"))
	if missing.Err == nil {
		t.Fatal("missing client must stay hard so SSH can fall back")
	}
}

func TestStderrErrorLines(t *testing.T) {
	lines := StderrErrorLines("\nERROR 1\n\nWARNING x\n")
	if len(lines) != 2 || lines[0] != "ERROR 1" || lines[1] != "WARNING x" {
		t.Fatalf("%v", lines)
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
