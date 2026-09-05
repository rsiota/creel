package db

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// lookPathPsql is exec.LookPath for "psql"; tests replace it.
var lookPathPsql = exec.LookPath

// runPsqlCmd runs a prepared psql command; tests replace it.
var runPsqlCmd = func(cmd *exec.Cmd) error { return cmd.Run() }

// SwapLookPathPsql replaces PATH lookup; the returned func restores it.
func SwapLookPathPsql(fn func(string) (string, error)) func() {
	prev := lookPathPsql
	lookPathPsql = fn
	return func() { lookPathPsql = prev }
}

// SwapRunPsqlCmd replaces command execution; the returned func restores it.
func SwapRunPsqlCmd(fn func(*exec.Cmd) error) func() {
	prev := runPsqlCmd
	runPsqlCmd = fn
	return func() { runPsqlCmd = prev }
}

// FindPsql returns the psql client binary on PATH, or an error if missing.
func FindPsql() (string, error) {
	p, err := lookPathPsql("psql")
	if err != nil {
		return "", err
	}
	return p, nil
}

// PgRestoreGuard reports why cfg cannot be restored with psql, or nil.
func PgRestoreGuard(cfg ConnectionConfig) error {
	if cfg.Driver != DriverPostgres {
		return fmt.Errorf(":restore uses psql (PostgreSQL); use I to import")
	}
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("no database selected")
	}
	if cfg.ReadOnly {
		return fmt.Errorf("connection is read-only")
	}
	return nil
}

// BuildPsqlArgs is the psql argv after the binary. Password is never included
// (use PGPASSFILE). The dump is read from stdin (caller sets cmd.Stdin).
func BuildPsqlArgs(cfg ConnectionConfig) []string {
	args := []string{
		"--no-password",
		"--set", "ON_ERROR_STOP=1",
	}
	if sock := cfg.socketPath(); sock != "" {
		args = append(args, "--host="+sock)
	} else {
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = 5432
		}
		args = append(args, "--host="+host, fmt.Sprintf("--port=%d", port))
	}
	if cfg.Username != "" {
		args = append(args, "--username="+cfg.Username)
	}
	args = append(args, "--dbname="+cfg.Database)
	return args
}

// RunPgRestore feeds dumpFile into psql for cfg's database.
//
// SSH + Postgres on the SSH host: run psql remotely and stream the dump over
// SSH stdin. Falls back to local psql + port forward when needed.
//
// onBytes is called with the cumulative bytes sent (may be nil).
func RunPgRestore(bin string, cfg ConnectionConfig, dumpFile string, conn *Connection, onBytes func(int64)) error {
	if err := PgRestoreGuard(cfg); err != nil {
		return err
	}

	throughSSH := strings.TrimSpace(cfg.SSHHost) != ""
	if throughSSH && MysqlHostOnSSHTarget(cfg.Host) && conn != nil {
		if err := conn.runRemotePgRestore(dumpFile, onBytes); err == nil {
			return nil
		} else if !remotePsqlUnavailable(err) {
			return err
		}
	}

	if bin == "" {
		var err error
		bin, err = FindPsql()
		if err != nil {
			return fmt.Errorf("psql is not on PATH")
		}
	}

	restoreCfg := cfg
	if throughSSH {
		if conn == nil {
			return fmt.Errorf(":restore needs an active SSH connection")
		}
		fwd, err := conn.startMysqlDumpForward()
		if err != nil {
			return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), ":backup", ":restore"))
		}
		defer fwd.Close()
		restoreCfg = pgDumpConfigViaForward(cfg, fwd)
	}

	passFile, err := writePgPassFile(restoreCfg)
	if err != nil {
		return err
	}
	defer os.Remove(passFile)

	in, err := os.Open(dumpFile)
	if err != nil {
		return err
	}
	defer in.Close()

	args := BuildPsqlArgs(restoreCfg)
	cmd := exec.Command(bin, args...)
	cmd.Env = pgClientEnv(restoreCfg, passFile)
	var stderr bytes.Buffer
	cmd.Stdin = &countingReader{r: in, onBytes: onBytes}
	cmd.Stderr = &stderr
	if err := runPsqlCmd(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("psql: %w", err)
		}
		return fmt.Errorf("psql: %s", msg)
	}
	return nil
}
