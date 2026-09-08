package db

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// lookPathMysql is exec.LookPath for "mysql"; tests replace it.
var lookPathMysql = exec.LookPath

// runMysqlCmd runs a prepared mysql command; tests replace it.
var runMysqlCmd = func(cmd *exec.Cmd) error { return cmd.Run() }

// SwapLookPathMysql replaces PATH lookup; the returned func restores it.
func SwapLookPathMysql(fn func(string) (string, error)) func() {
	prev := lookPathMysql
	lookPathMysql = fn
	return func() { lookPathMysql = prev }
}

// SwapRunMysqlCmd replaces command execution; the returned func restores it.
func SwapRunMysqlCmd(fn func(*exec.Cmd) error) func() {
	prev := runMysqlCmd
	runMysqlCmd = fn
	return func() { runMysqlCmd = prev }
}

// FindMysql returns the mysql client binary on PATH, or an error if missing.
func FindMysql() (string, error) {
	p, err := lookPathMysql("mysql")
	if err != nil {
		return "", err
	}
	return p, nil
}

// MysqlRestoreGuard reports why cfg cannot be restored with the mysql CLI, or nil.
func MysqlRestoreGuard(cfg ConnectionConfig) error {
	if cfg.Driver != DriverMySQL {
		return fmt.Errorf(":restore uses mysql (MySQL/MariaDB); use I to import")
	}
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("no database selected")
	}
	if cfg.ReadOnly {
		return fmt.Errorf("connection is read-only")
	}
	return nil
}

// BuildMysqlArgs is the mysql client argv after the binary. defaultsFile must
// be the first option (--defaults-extra-file). Password is never included.
// throughSSH adds protocol compression for bulk stdin over a localhost forward.
// continueOnError adds --force so SQL errors do not abort the load.
// The dump is read from stdin (caller sets cmd.Stdin).
func BuildMysqlArgs(cfg ConnectionConfig, defaultsFile string, throughSSH, continueOnError bool) []string {
	args := []string{"--defaults-extra-file=" + defaultsFile}
	if sock := cfg.socketPath(); sock != "" {
		args = append(args, "--socket="+sock)
	} else {
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = 3306
		}
		args = append(args,
			"--protocol=TCP",
			"--host="+host,
			fmt.Sprintf("--port=%d", port),
		)
	}
	if ssl := mysqlDumpSSLMode(cfg.SSLMode); ssl != "" {
		args = append(args, "--ssl-mode="+ssl)
	}
	args = append(args, "--max-allowed-packet=1073741824")
	if throughSSH {
		args = append(args, "--compress")
	}
	if continueOnError {
		args = append(args, "--force")
	}
	args = append(args, cfg.Database)
	return args
}

// RunMysqlRestore feeds dumpFile into the mysql client for cfg's database.
//
// SSH + MySQL on the SSH host (localhost/127.0.0.1): run mysql on the remote
// machine and stream the dump over SSH stdin — same path as
// `ssh host mysql … < dump.sql`. Falls back to local mysql + port forward when
// the remote binary is missing or MySQL is on a different internal host.
//
// With opts.ContinueOnError, mysql --force keeps loading after SQL errors;
// ClientStderr then holds the ignored messages for review.
func RunMysqlRestore(bin string, cfg ConnectionConfig, dumpFile string, conn *Connection, opts RestoreOpts) RestoreResult {
	if err := MysqlRestoreGuard(cfg); err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}

	throughSSH := strings.TrimSpace(cfg.SSHHost) != ""
	if throughSSH && MysqlHostOnSSHTarget(cfg.Host) && conn != nil {
		res := conn.runRemoteMysqlRestore(dumpFile, opts)
		if res.Err == nil || !remoteMysqlUnavailable(res.Err) {
			return res
		}
		// Remote has no mysql — try local binary + forward below.
	}

	if bin == "" {
		var err error
		bin, err = FindMysql()
		if err != nil {
			return RestoreResult{Err: fmt.Errorf("mysql is not on PATH"), Continued: opts.ContinueOnError}
		}
	}

	restoreCfg := cfg
	if throughSSH {
		if conn == nil {
			return RestoreResult{Err: fmt.Errorf(":restore needs an active SSH connection"), Continued: opts.ContinueOnError}
		}
		fwd, err := conn.startMysqlDumpForward()
		if err != nil {
			// Rephrase backup-oriented forward errors for restore.
			return RestoreResult{
				Err:       fmt.Errorf("%s", strings.ReplaceAll(err.Error(), ":backup", ":restore")),
				Continued: opts.ContinueOnError,
			}
		}
		defer fwd.Close()
		restoreCfg = mysqlDumpConfigViaForward(cfg, fwd)
	}

	tmp, err := os.CreateTemp("", "creel-mysql-*.cnf")
	if err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}
	defaultsPath := tmp.Name()
	defer os.Remove(defaultsPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}
	if _, err := tmp.WriteString(MysqlDumpDefaults(restoreCfg)); err != nil {
		tmp.Close()
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}
	if err := tmp.Close(); err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}

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

	args := BuildMysqlArgs(restoreCfg, defaultsPath, throughSSH, opts.ContinueOnError)
	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stdin = &countingReader{r: in, onBytes: count}
	cmd.Stderr = &stderr
	runErr := runMysqlCmd(cmd)
	return finalizeCLIRestore("mysql", opts.ContinueOnError, last, stderr.String(), runErr)
}

// countingReader counts bytes read and reports the running total.
type countingReader struct {
	r       io.Reader
	n       int64
	onBytes func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n += int64(n)
		if c.onBytes != nil {
			c.onBytes(c.n)
		}
	}
	return n, err
}
