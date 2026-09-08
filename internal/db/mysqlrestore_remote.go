package db

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// buildRemoteMysqlRestoreCmd is a bash -c script run on the SSH host: write a
// 0600 defaults file from base64, then run mysql reading the dump from stdin
// (session.Stdin is the local dump file — not the script).
func buildRemoteMysqlRestoreCmd(cfg ConnectionConfig, continueOnError bool) string {
	var endpoint strings.Builder
	if sock := strings.TrimSpace(cfg.Socket); sock != "" {
		fmt.Fprintf(&endpoint, " --socket=%s", shellSingleQuote(sock))
	} else {
		port := cfg.Port
		if port == 0 {
			port = 3306
		}
		fmt.Fprintf(&endpoint, " --protocol=TCP --host=127.0.0.1 --port=%d", port)
	}
	force := ""
	if continueOnError {
		force = " --force"
	}
	defaultsB64 := base64.StdEncoding.EncodeToString([]byte(MysqlDumpDefaults(cfg)))
	return fmt.Sprintf(`set -euo pipefail
umask 077
CNF=$(mktemp /tmp/creel-mysql.XXXXXX.cnf)
trap 'rm -f "$CNF"' EXIT
echo %s | base64 -d > "$CNF"
command -v mysql >/dev/null || { echo "mysql not found on SSH host" >&2; exit 127; }
mysql --defaults-extra-file="$CNF"%s --max-allowed-packet=1073741824%s %s
`, shellSingleQuote(defaultsB64), endpoint.String(), force, shellSingleQuote(cfg.Database))
}

// runRemoteMysqlRestore runs mysql on the SSH host via the live tunnel and
// streams dumpFile into its stdin.
func (c *Connection) runRemoteMysqlRestore(dumpFile string, opts RestoreOpts) RestoreResult {
	if c == nil || c.db == nil {
		return RestoreResult{Err: fmt.Errorf(":restore needs an active SSH connection"), Continued: opts.ContinueOnError}
	}
	m, ok := c.db.(*MySQL)
	if !ok || m.tunnel == nil {
		return RestoreResult{Err: fmt.Errorf("no active SSH tunnel"), Continued: opts.ContinueOnError}
	}
	in, err := os.Open(dumpFile)
	if err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}
	defer in.Close()

	session, err := m.tunnel.NewSession()
	if err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}
	defer session.Close()

	var last int64
	onBytes := opts.OnBytes
	count := func(n int64) {
		last = n
		if onBytes != nil {
			onBytes(n)
		}
	}

	var stderr bytes.Buffer
	session.Stdin = &countingReader{r: in, onBytes: count}
	session.Stderr = &stderr
	cmd := "bash -c " + shellSingleQuote(buildRemoteMysqlRestoreCmd(c.config, opts.ContinueOnError))
	runErr := session.Run(cmd)
	return finalizeCLIRestore("remote mysql", opts.ContinueOnError, last, stderr.String(), runErr)
}

func remoteMysqlUnavailable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "mysql not found") ||
		strings.Contains(s, "command not found") ||
		strings.Contains(s, "no such file")
}
