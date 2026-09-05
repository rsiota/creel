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
func buildRemotePgRestoreCmd(cfg ConnectionConfig) string {
	remote := cfg
	if strings.TrimSpace(remote.Socket) == "" {
		remote.Host = "127.0.0.1"
		if remote.Port == 0 {
			remote.Port = 5432
		}
	}
	remote.SSHHost = ""
	passB64 := base64.StdEncoding.EncodeToString([]byte(PgPassLine(remote) + "\n"))
	args := BuildPsqlArgs(remote)
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
func (c *Connection) runRemotePgRestore(dumpFile string, onBytes func(int64)) error {
	if c == nil || c.db == nil {
		return fmt.Errorf(":restore needs an active SSH connection")
	}
	tunnel := c.sshTunnel()
	if tunnel == nil {
		return fmt.Errorf("no active SSH tunnel")
	}
	in, err := os.Open(dumpFile)
	if err != nil {
		return err
	}
	defer in.Close()

	session, err := tunnel.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stdin = &countingReader{r: in, onBytes: onBytes}
	session.Stderr = &stderr
	cmd := "bash -c " + shellSingleQuote(buildRemotePgRestoreCmd(c.config))
	if err := session.Run(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("remote psql: %w", err)
		}
		return fmt.Errorf("remote psql: %s", msg)
	}
	return nil
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
