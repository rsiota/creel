package db

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPgDumpGuard(t *testing.T) {
	if err := PgDumpGuard(ConnectionConfig{Driver: DriverMySQL, Database: "x"}); err == nil {
		t.Fatal("mysql should be rejected")
	}
	if err := PgDumpGuard(ConnectionConfig{Driver: DriverPostgres}); err == nil {
		t.Fatal("empty database should be rejected")
	}
	if err := PgDumpGuard(ConnectionConfig{
		Driver: DriverPostgres, Database: "app", SSHHost: "bastion",
	}); err != nil {
		t.Fatalf("SSH should be allowed: %v", err)
	}
}

func TestPgPassLineEscapes(t *testing.T) {
	line := PgPassLine(ConnectionConfig{
		Host: "db.example", Port: 5433, Database: "shop",
		Username: "u:ser", Password: `p\ass:word`,
	})
	if !strings.Contains(line, `u\:ser`) || !strings.Contains(line, `p\\ass\:word`) {
		t.Fatalf("got %q", line)
	}
	if strings.HasPrefix(line, "db.example:5433:shop:") == false {
		t.Fatalf("got %q", line)
	}
}

func TestBuildPgDumpArgsOmitsPassword(t *testing.T) {
	cfg := ConnectionConfig{
		Driver: DriverPostgres, Database: "shop",
		Host: "db.example", Port: 5433, Username: "root", Password: "s3cret",
	}
	args := BuildPgDumpArgs(cfg)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "s3cret") {
		t.Fatalf("password leaked onto argv: %v", args)
	}
	for _, want := range []string{
		"--no-password", "--format=plain",
		"--host=db.example", "--port=5433",
		"--username=root", "--dbname=shop",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

func TestRunPgDumpLocal(t *testing.T) {
	restorePath := SwapLookPathPgDump(func(string) (string, error) {
		return "/usr/bin/pg_dump", nil
	})
	t.Cleanup(restorePath)

	var gotArgs []string
	var gotEnv string
	restoreRun := SwapRunPgDumpCmd(func(cmd *exec.Cmd) error {
		gotArgs = append([]string{}, cmd.Args[1:]...)
		for _, e := range cmd.Env {
			if strings.HasPrefix(e, "PGPASSFILE=") {
				gotEnv = e
				b, err := os.ReadFile(strings.TrimPrefix(e, "PGPASSFILE="))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(b), "s3cret-pg") {
					t.Fatalf("passfile missing password: %q", b)
				}
			}
			if strings.HasPrefix(e, "PGPASSWORD=") {
				t.Fatal("PGPASSWORD should be stripped")
			}
		}
		_, _ = cmd.Stdout.Write([]byte("-- dump\n"))
		return nil
	})
	t.Cleanup(restoreRun)

	out := filepath.Join(t.TempDir(), "out.sql")
	cfg := ConnectionConfig{
		Driver: DriverPostgres, Database: "shop", Host: "127.0.0.1", Port: 5432,
		Username: "u", Password: "s3cret-pg",
	}
	var progress []int64
	if err := RunPgDump("/usr/bin/pg_dump", cfg, out, nil, func(n int64) {
		progress = append(progress, n)
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(gotArgs, " "), "s3cret-pg") {
		t.Fatalf("password on argv: %v", gotArgs)
	}
	if gotEnv == "" {
		t.Fatal("expected PGPASSFILE")
	}
	body, err := os.ReadFile(out)
	if err != nil || string(body) != "-- dump\n" {
		t.Fatalf("out = %q err=%v", body, err)
	}
	if len(progress) == 0 {
		t.Fatal("expected progress")
	}
}

func TestRunPgDumpMissingBinary(t *testing.T) {
	restorePath := SwapLookPathPgDump(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(restorePath)

	err := RunPgDump("", ConnectionConfig{
		Driver: DriverPostgres, Database: "shop", Host: "127.0.0.1",
	}, filepath.Join(t.TempDir(), "x.sql"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("got %v", err)
	}
}

func TestPgDumpConfigViaForward(t *testing.T) {
	fwd := &LocalForward{Host: "127.0.0.1", Port: 55432}
	got := pgDumpConfigViaForward(ConnectionConfig{
		Driver: DriverPostgres, Database: "app", Host: "db.internal",
		Port: 5432, SSHHost: "bastion", SSLMode: "require", Socket: "/tmp/.s.PGSQL.5432",
	}, fwd)
	if got.SSHHost != "" || got.Socket != "" {
		t.Fatalf("ssh/socket should clear: %+v", got)
	}
	if got.Host != "127.0.0.1" || got.Port != 55432 || got.SSLMode != "disable" {
		t.Fatalf("got %+v", got)
	}
}

func TestBuildRemotePgDumpCmd(t *testing.T) {
	cmd := buildRemotePgDumpCmd(ConnectionConfig{
		Database: "app", Username: "u", Password: "p", Host: "127.0.0.1", Port: 5432,
	})
	for _, want := range []string{"pg_dump", "base64 -d", "PGPASSFILE", "command -v pg_dump", "--dbname=app"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q in:\n%s", want, cmd)
		}
	}
}

func TestRemotePgDumpUnavailable(t *testing.T) {
	if !remotePgDumpUnavailable(fmtError("remote pg_dump: pg_dump not found on SSH host")) {
		t.Fatal("expected unavailable")
	}
	if remotePgDumpUnavailable(fmtError("remote pg_dump: Access denied")) {
		t.Fatal("access denied should not fall back")
	}
}

func TestRunPgDumpCapturesStderr(t *testing.T) {
	restoreRun := SwapRunPgDumpCmd(func(cmd *exec.Cmd) error {
		_, _ = cmd.Stderr.Write([]byte("permission denied\n"))
		return errors.New("exit 1")
	})
	t.Cleanup(restoreRun)

	err := RunPgDump("/usr/bin/pg_dump", ConnectionConfig{
		Driver: DriverPostgres, Database: "shop", Host: "127.0.0.1",
	}, filepath.Join(t.TempDir(), "x.sql"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("got %v", err)
	}
}

func TestPgClientEnv(t *testing.T) {
	t.Setenv("PGPASSWORD", "ambient")
	t.Setenv("PGSSLMODE", "disable")
	cfg := ConnectionConfig{SSLMode: "require"}
	env := pgClientEnv(cfg, "/tmp/pass")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "PGPASSWORD=") {
		t.Fatal("PGPASSWORD should be removed")
	}
	if !strings.Contains(joined, "PGPASSFILE=/tmp/pass") {
		t.Fatal("missing PGPASSFILE")
	}
	if !strings.Contains(joined, "PGSSLMODE=require") {
		t.Fatal("missing PGSSLMODE")
	}
}
