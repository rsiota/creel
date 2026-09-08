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

func TestPgRestoreGuard(t *testing.T) {
	if err := PgRestoreGuard(ConnectionConfig{Driver: DriverSQLite, Database: "x"}); err == nil {
		t.Fatal("sqlite should be rejected")
	}
	if err := PgRestoreGuard(ConnectionConfig{Driver: DriverPostgres}); err == nil {
		t.Fatal("empty database should be rejected")
	}
	if err := PgRestoreGuard(ConnectionConfig{
		Driver: DriverPostgres, Database: "app", ReadOnly: true,
	}); err == nil {
		t.Fatal("read-only should be rejected")
	}
}

func TestBuildPsqlArgsOmitsPassword(t *testing.T) {
	cfg := ConnectionConfig{
		Driver: DriverPostgres, Database: "shop",
		Host: "db.example", Port: 5433, Username: "root", Password: "s3cret",
	}
	args := BuildPsqlArgs(cfg, false)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "s3cret") {
		t.Fatalf("password leaked onto argv: %v", args)
	}
	for _, want := range []string{
		"--no-password", "ON_ERROR_STOP=1",
		"--host=db.example", "--port=5433",
		"--username=root", "--dbname=shop",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

func TestBuildPsqlArgsContinueOnError(t *testing.T) {
	args := BuildPsqlArgs(ConnectionConfig{
		Driver: DriverPostgres, Database: "shop", Host: "127.0.0.1",
	}, true)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "ON_ERROR_STOP") {
		t.Fatalf("continue-on-error must omit ON_ERROR_STOP: %v", args)
	}
}

func TestRunPgRestoreLocal(t *testing.T) {
	restorePath := SwapLookPathPsql(func(string) (string, error) {
		return "/usr/bin/psql", nil
	})
	t.Cleanup(restorePath)

	dump := filepath.Join(t.TempDir(), "in.sql")
	if err := os.WriteFile(dump, []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotArgs []string
	var gotStdin string
	restoreRun := SwapRunPsqlCmd(func(cmd *exec.Cmd) error {
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
		Driver: DriverPostgres, Database: "shop", Host: "127.0.0.1", Port: 5432,
		Username: "u", Password: "s3cret-restore",
	}
	var progress []int64
	res := RunPgRestore("/usr/bin/psql", cfg, dump, nil, RestoreOpts{
		OnBytes: func(n int64) { progress = append(progress, n) },
	})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if strings.Contains(strings.Join(gotArgs, " "), "s3cret-restore") {
		t.Fatalf("password on argv: %v", gotArgs)
	}
	if !strings.Contains(strings.Join(gotArgs, " "), "ON_ERROR_STOP=1") {
		t.Fatalf("default should stop on error: %v", gotArgs)
	}
	if gotStdin != "SELECT 1;\n" {
		t.Fatalf("stdin = %q", gotStdin)
	}
	if len(progress) == 0 || progress[len(progress)-1] != int64(len("SELECT 1;\n")) {
		t.Fatalf("progress = %v", progress)
	}
}

func TestRunPgRestoreContinueOnError(t *testing.T) {
	restorePath := SwapLookPathPsql(func(string) (string, error) {
		return "/usr/bin/psql", nil
	})
	t.Cleanup(restorePath)

	dump := filepath.Join(t.TempDir(), "in.sql")
	if err := os.WriteFile(dump, []byte("BAD;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var gotArgs []string
	restoreRun := SwapRunPsqlCmd(func(cmd *exec.Cmd) error {
		gotArgs = append([]string{}, cmd.Args[1:]...)
		if cmd.Stdin != nil {
			_, _ = io.Copy(io.Discard, cmd.Stdin)
		}
		if cmd.Stderr != nil {
			_, _ = cmd.Stderr.Write([]byte("ERROR:  syntax error\n"))
		}
		return errors.New("exit status 3")
	})
	t.Cleanup(restoreRun)

	res := RunPgRestore("/usr/bin/psql", ConnectionConfig{
		Driver: DriverPostgres, Database: "shop", Host: "127.0.0.1",
	}, dump, nil, RestoreOpts{ContinueOnError: true})
	if res.Err != nil {
		t.Fatalf("soft success expected, got %v", res.Err)
	}
	if strings.Contains(strings.Join(gotArgs, " "), "ON_ERROR_STOP") {
		t.Fatalf("unexpected ON_ERROR_STOP: %v", gotArgs)
	}
	if !strings.Contains(res.ClientStderr, "syntax error") {
		t.Fatalf("stderr = %q", res.ClientStderr)
	}
}

func TestRunPgRestoreMissingBinary(t *testing.T) {
	restorePath := SwapLookPathPsql(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restorePath)

	dump := filepath.Join(t.TempDir(), "in.sql")
	_ = os.WriteFile(dump, []byte("x"), 0o644)
	res := RunPgRestore("", ConnectionConfig{
		Driver: DriverPostgres, Database: "shop", Host: "127.0.0.1",
	}, dump, nil, RestoreOpts{})
	if res.Err == nil || !strings.Contains(res.Err.Error(), "not on PATH") {
		t.Fatalf("got %v", res.Err)
	}
}

func TestBuildRemotePgRestoreCmd(t *testing.T) {
	cmd := buildRemotePgRestoreCmd(ConnectionConfig{
		Database: "app", Username: "u", Password: "p", Host: "127.0.0.1", Port: 5432,
	}, false)
	for _, want := range []string{"psql", "base64 -d", "PGPASSFILE", "command -v psql", "--dbname=app", "ON_ERROR_STOP=1"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in:\n%s", want, cmd)
		}
	}
	if strings.Contains(cmd, "pg_dump") {
		t.Fatal("restore must not invoke pg_dump")
	}
	force := buildRemotePgRestoreCmd(ConnectionConfig{
		Database: "app", Username: "u", Password: "p", Host: "127.0.0.1",
	}, true)
	if strings.Contains(force, "ON_ERROR_STOP") {
		t.Fatalf("continue-on-error remote must omit ON_ERROR_STOP:\n%s", force)
	}
}

func TestRemotePsqlUnavailable(t *testing.T) {
	if !remotePsqlUnavailable(fmtError("remote psql: psql not found on SSH host")) {
		t.Fatal("expected unavailable")
	}
	if remotePsqlUnavailable(fmtError("remote psql: Access denied")) {
		t.Fatal("access denied should not fall back")
	}
}
