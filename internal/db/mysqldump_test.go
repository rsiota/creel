package db

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMysqlDumpGuard(t *testing.T) {
	if err := MysqlDumpGuard(ConnectionConfig{Driver: DriverSQLite, Database: "x"}); err == nil {
		t.Fatal("sqlite should be rejected")
	}
	if err := MysqlDumpGuard(ConnectionConfig{Driver: DriverMySQL}); err == nil {
		t.Fatal("empty database should be rejected")
	}
	if err := MysqlDumpGuard(ConnectionConfig{
		Driver: DriverMySQL, Database: "app", SSHHost: "bastion",
	}); err != nil {
		t.Fatalf("SSH should be allowed (local forward): %v", err)
	}
	if err := MysqlDumpGuard(ConnectionConfig{Driver: DriverMySQL, Database: "app"}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildMysqlDumpArgsOmitsPassword(t *testing.T) {
	cfg := ConnectionConfig{
		Driver:   DriverMySQL,
		Database: "shop",
		Host:     "db.example",
		Port:     3307,
		Username: "root",
		Password: "s3cret",
		SSLMode:  "require",
	}
	args := BuildMysqlDumpArgs(cfg, "/tmp/c.cnf", "/tmp/out.sql", false)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "s3cret") {
		t.Fatalf("password leaked onto argv: %v", args)
	}
	if args[0] != "--defaults-extra-file=/tmp/c.cnf" {
		t.Fatalf("defaults file must be first: %v", args)
	}
	want := []string{
		"--protocol=TCP",
		"--host=db.example",
		"--port=3307",
		"--ssl-mode=REQUIRED",
		"--single-transaction",
		"--routines",
		"--events",
		"--max-allowed-packet=1073741824",
		"--result-file=/tmp/out.sql",
		"shop",
	}
	got := strings.Join(args[1:], " ")
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %v", w, args)
		}
	}
	if strings.Contains(joined, "--compress") {
		t.Fatalf("compress is SSH-only: %v", args)
	}
}

func TestBuildMysqlDumpArgsSSHCompress(t *testing.T) {
	args := BuildMysqlDumpArgs(ConnectionConfig{
		Driver: DriverMySQL, Database: "shop", Host: "127.0.0.1", Port: 3306,
	}, "/tmp/c.cnf", "/tmp/out.sql", true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--compress") {
		t.Fatalf("SSH dumps should compress: %v", args)
	}
}

func TestBuildMysqlDumpArgsSocket(t *testing.T) {
	cfg := ConnectionConfig{
		Driver:   DriverMySQL,
		Database: "app",
		Socket:   "/tmp/mysql.sock",
	}
	args := BuildMysqlDumpArgs(cfg, "/tmp/c.cnf", "/tmp/out.sql", false)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--socket=/tmp/mysql.sock") {
		t.Fatalf("socket: %v", args)
	}
	if strings.Contains(joined, "--host=") || strings.Contains(joined, "--protocol=TCP") {
		t.Fatalf("socket dump should not force TCP: %v", args)
	}
}

func TestMysqlDumpDefaultsQuotesPassword(t *testing.T) {
	cfg := ConnectionConfig{Username: "u", Password: `p"ass#`}
	got := MysqlDumpDefaults(cfg)
	if !strings.Contains(got, `password="p\"ass#"`) && !strings.Contains(got, `password="`) {
		t.Fatalf("quoted password missing:\n%s", got)
	}
	if strings.Contains(got, "password=p\"ass") {
		t.Fatalf("unquoted special password:\n%s", got)
	}
}

func TestFindMysqlDumpMissing(t *testing.T) {
	restore := SwapLookPathMysqlDump(func(string) (string, error) {
		return "", errors.New("executable file not found in $PATH")
	})
	t.Cleanup(restore)
	if _, err := FindMysqlDump(); err == nil {
		t.Fatal("expected missing binary")
	}
}

func TestMysqlDumpGuardAllowsReadOnly(t *testing.T) {
	if err := MysqlDumpGuard(ConnectionConfig{
		Driver: DriverMySQL, Database: "app", ReadOnly: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRunMysqlDumpUsesDefaultsFileNotPassword(t *testing.T) {
	restorePath := SwapLookPathMysqlDump(func(string) (string, error) {
		return "/usr/bin/mysqldump", nil
	})
	t.Cleanup(restorePath)
	restoreHelp := SwapRunMysqlDumpHelp(func(string) ([]byte, error) {
		return []byte("no column stats here"), nil
	})
	t.Cleanup(restoreHelp)

	var gotArgs []string
	restoreRun := SwapRunMysqlDumpCmd(func(cmd *exec.Cmd) error {
		gotArgs = append([]string{cmd.Path}, cmd.Args[1:]...)
		return nil
	})
	t.Cleanup(restoreRun)

	cfg := ConnectionConfig{
		Driver:   DriverMySQL,
		Database: "shop",
		Host:     "db.example",
		Username: "root",
		Password: "s3cret",
	}
	out := filepath.Join(t.TempDir(), "shop.sql")
	if err := RunMysqlDump("/usr/bin/mysqldump", cfg, out, nil); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(gotArgs, " ")
	if strings.Contains(joined, "s3cret") {
		t.Fatalf("password on argv: %v", gotArgs)
	}
	if !strings.Contains(joined, "--defaults-extra-file=") {
		t.Fatalf("missing defaults file: %v", gotArgs)
	}
	if !strings.Contains(joined, "--result-file="+out) {
		t.Fatalf("missing result file: %v", gotArgs)
	}
	if strings.Contains(joined, "column-statistics") {
		t.Fatalf("MariaDB-style binary should not get column-statistics: %v", gotArgs)
	}
}

func TestRunMysqlDumpDisablesColumnStatisticsForMySQL8(t *testing.T) {
	restoreHelp := SwapRunMysqlDumpHelp(func(string) ([]byte, error) {
		return []byte("  --column-statistics  Add ANALYZE TABLE...\ncolumn-statistics TRUE\n"), nil
	})
	t.Cleanup(restoreHelp)

	var gotArgs []string
	restoreRun := SwapRunMysqlDumpCmd(func(cmd *exec.Cmd) error {
		gotArgs = cmd.Args[1:]
		return nil
	})
	t.Cleanup(restoreRun)

	cfg := ConnectionConfig{Driver: DriverMySQL, Database: "shop", Host: "db"}
	out := filepath.Join(t.TempDir(), "shop.sql")
	if err := RunMysqlDump("/usr/bin/mysqldump", cfg, out, nil); err != nil {
		t.Fatal(err)
	}
	if len(gotArgs) < 2 || !strings.HasPrefix(gotArgs[0], "--defaults-extra-file=") {
		t.Fatalf("defaults file must stay first: %v", gotArgs)
	}
	if gotArgs[1] != "--column-statistics=0" {
		t.Fatalf("want column-statistics=0 second, got %v", gotArgs)
	}
}

func TestMysqlDumpConfigViaForward(t *testing.T) {
	fwd := &LocalForward{Host: "127.0.0.1", Port: 54321}
	got := mysqlDumpConfigViaForward(ConnectionConfig{
		Driver:   DriverMySQL,
		Database: "app",
		Host:     "db.internal",
		Port:     3306,
		SSHHost:  "bastion",
		Socket:   "/ignored.sock",
		SSLMode:  "verify-full",
		Password: "x",
	}, fwd)
	if got.SSHHost != "" || got.Socket != "" {
		t.Fatalf("SSH/socket should be cleared: %+v", got)
	}
	if got.Host != "127.0.0.1" || got.Port != 54321 {
		t.Fatalf("local endpoint: %+v", got)
	}
	if got.SSLMode != "disable" {
		t.Fatalf("ssl: %q", got.SSLMode)
	}
	args := BuildMysqlDumpArgs(got, "/tmp/c.cnf", "/tmp/out.sql", true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--host=127.0.0.1") || !strings.Contains(joined, "--port=54321") {
		t.Fatalf("args: %v", args)
	}
	if !strings.Contains(joined, "--ssl-mode=DISABLED") {
		t.Fatalf("expected DISABLED ssl over local forward: %v", args)
	}
	if strings.Contains(joined, "bastion") || strings.Contains(joined, "db.internal") {
		t.Fatalf("remote hosts leaked onto argv: %v", args)
	}
}

func TestRunMysqlDumpSSHRequiresLiveConnection(t *testing.T) {
	cfg := ConnectionConfig{
		Driver: DriverMySQL, Database: "app", Host: "db", SSHHost: "bastion",
	}
	err := RunMysqlDump("/usr/bin/mysqldump", cfg, filepath.Join(t.TempDir(), "x.sql"), nil)
	if err == nil || !strings.Contains(err.Error(), "SSH") {
		t.Fatalf("got %v", err)
	}
}
