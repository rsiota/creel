package db

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// buildRemotePgRestoreCmd is a bash -c script run on the SSH host: write a
// 0600 .pgpass from base64, then run psql reading the dump from stdin
// (session.Stdin is the local dump file — not the script).
func buildRemotePgRestoreCmd(cfg ConnectionConfig, continueOnError bool) string {
	remote := cfg
	if strings.TrimSpace(remote.Socket) == "" {
		remote.Host = "127.0.0.1"
		if remote.Port == 0 {
			remote.Port = 5432
		}
	}
	remote.SSHHost = ""
	passB64 := base64.StdEncoding.EncodeToString([]byte(PgPassLine(remote) + "\n"))
	args := BuildPsqlArgs(remote, continueOnError)
	var quoted []string
	for _, a := range args {
		quoted = append(quoted, shellSingleQuote(a))
	}
	return fmt.Sprintf(`set -euo pipefail
umask 077
PASSFILE=$(mktemp /tmp/creel-pgpass.XXXXXX)
trap 'rm -f "$PASSFILE"' EXIT
echo %s | base64 -d > "$PASSFILE"
export PGPASSFILE="$PASSFILE"
export PGSSLMODE=prefer
command -v psql >/dev/null || { echo "psql not found on SSH host" >&2; exit 127; }
psql %s
`, shellSingleQuote(passB64), strings.Join(quoted, " "))
}

// runRemotePgRestore runs psql on the SSH host via the live tunnel and streams
// dumpFile into its stdin.
func (c *Connection) runRemotePgRestore(dumpFile string, opts RestoreOpts) RestoreResult {
	if c == nil || c.db == nil {
		return RestoreResult{Err: fmt.Errorf(":restore needs an active SSH connection"), Continued: opts.ContinueOnError}
	}
	tunnel := c.sshTunnel()
	if tunnel == nil {
		return RestoreResult{Err: fmt.Errorf("no active SSH tunnel"), Continued: opts.ContinueOnError}
	}
	in, err := os.Open(dumpFile)
	if err != nil {
		return RestoreResult{Err: err, Continued: opts.ContinueOnError}
	}
	defer in.Close()

	session, err := tunnel.NewSession()
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
	cmd := "bash -c " + shellSingleQuote(buildRemotePgRestoreCmd(c.config, opts.ContinueOnError))
	runErr := session.Run(cmd)
	return finalizeCLIRestore("remote psql", opts.ContinueOnError, last, stderr.String(), runErr)
}

func remotePsqlUnavailable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "psql not found") ||
		strings.Contains(s, "command not found") ||
		strings.Contains(s, "no such file")
}
