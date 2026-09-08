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
// When continueOnError is false (default), ON_ERROR_STOP=1 aborts on the first
// SQL error; when true, psql keeps going so ignored errors can be reviewed.
func BuildPsqlArgs(cfg ConnectionConfig, continueOnError bool) []string {
	args := []string{"--no-password"}
	if !continueOnError {
		args = append(args, "--set", "ON_ERROR_STOP=1")
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
// With opts.ContinueOnError, ON_ERROR_STOP is left off so SQL errors do not
// abort the load; ClientStderr then holds the ignored messages for review.
func RunPgRestore(bin string, cfg ConnectionConfig, dumpFile string, conn *Connection, opts RestoreOpts) RestoreResult {
	if err := PgRestoreGuard(cfg); err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}

	throughSSH := strings.TrimSpace(cfg.SSHHost) != ""
	if throughSSH && MysqlHostOnSSHTarget(cfg.Host) && conn != nil {
		res := conn.runRemotePgRestore(dumpFile, opts)
		if res.Err == nil || !remotePsqlUnavailable(res.Err) {
			return res
		}
	}

	if bin == "" {
		var err error
		bin, err = FindPsql()
		if err != nil {
			return RestoreResult{Err: fmt.Errorf("psql is not on PATH"), Continued: opts.ContinueOnError}
		}
	}

	restoreCfg := cfg
	if throughSSH {
		if conn == nil {
			return RestoreResult{Err: fmt.Errorf(":restore needs an active SSH connection"), Continued: opts.ContinueOnError}
		}
		fwd, err := conn.startMysqlDumpForward()
		if err != nil {
			return RestoreResult{
				Err:       fmt.Errorf("%s", strings.ReplaceAll(err.Error(), ":backup", ":restore")),
				Continued: opts.ContinueOnError,
			}
		}
		defer fwd.Close()
		restoreCfg = pgDumpConfigViaForward(cfg, fwd)
	}

	passFile, err := writePgPassFile(restoreCfg)
	if err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}
	defer os.Remove(passFile)

	in, err := os.Open(dumpFile)
	if err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}
	defer in.Close()

	var last int64
	onBytes := opts.OnBytes
	count := func(n int64) {
		last = n
		if onBytes != nil {
			onBytes(n)
		}
	}

	args := BuildPsqlArgs(restoreCfg, opts.ContinueOnError)
	cmd := exec.Command(bin, args...)
	cmd.Env = pgClientEnv(restoreCfg, passFile)
	var stderr bytes.Buffer
	cmd.Stdin = &countingReader{r: in, onBytes: count}
	cmd.Stderr = &stderr
	runErr := runPsqlCmd(cmd)
	return finalizeCLIRestore("psql", opts.ContinueOnError, last, stderr.String(), runErr)
}
