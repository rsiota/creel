package db

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// buildRemotePgDumpCmd is a bash -c script run on the SSH host: write a 0600
// .pgpass from base64, run pg_dump to stdout (streamed back over SSH).
// Selective plans may emit two pg_dump invocations (schema, then data).
func buildRemotePgDumpCmd(cfg ConnectionConfig, plan DumpPlan) string {
	remote := cfg
	// On the SSH host, dial local Postgres (socket or loopback).
	if strings.TrimSpace(remote.Socket) == "" {
		remote.Host = "127.0.0.1"
		if remote.Port == 0 {
			remote.Port = 5432
		}
	}
	remote.SSHHost = ""
	passB64 := base64.StdEncoding.EncodeToString([]byte(PgPassLine(remote) + "\n"))
	var cmds strings.Builder
	for _, opt := range pgDumpPasses(plan) {
		args := BuildPgDumpArgs(remote, opt)
		var quoted []string
		for _, a := range args {
			quoted = append(quoted, shellSingleQuote(a))
		}
		cmds.WriteString("pg_dump ")
		cmds.WriteString(strings.Join(quoted, " "))
		cmds.WriteString("\n")
	}
	return fmt.Sprintf(`set -euo pipefail
umask 077
PASSFILE=$(mktemp /tmp/creel-pgpass.XXXXXX)
trap 'rm -f "$PASSFILE"' EXIT
echo %s | base64 -d > "$PASSFILE"
export PGPASSFILE="$PASSFILE"
export PGSSLMODE=prefer
command -v pg_dump >/dev/null || { echo "pg_dump not found on SSH host" >&2; exit 127; }
%s`, shellSingleQuote(passB64), cmds.String())
}

// runRemotePgDump runs pg_dump on the SSH host via the live tunnel and streams
// stdout into resultFile.
func (c *Connection) runRemotePgDump(resultFile string, plan DumpPlan, onBytes func(int64)) error {
	if c == nil || c.db == nil {
		return fmt.Errorf(":backup needs an active SSH connection")
	}
	if !plan.HasContent() {
		return fmt.Errorf("nothing selected to backup")
	}
	tunnel := c.sshTunnel()
	if tunnel == nil {
		return fmt.Errorf("no active SSH tunnel")
	}
	out, err := os.Create(resultFile)
	if err != nil {
		return err
	}
	defer out.Close()

	session, err := tunnel.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stdout = &countingWriter{w: out, onBytes: onBytes}
	session.Stderr = &stderr
	cmd := "bash -c " + shellSingleQuote(buildRemotePgDumpCmd(c.config, plan))
	if err := session.Run(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("remote pg_dump: %w", err)
		}
		return fmt.Errorf("remote pg_dump: %s", msg)
	}
	return nil
}

func remotePgDumpUnavailable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "pg_dump not found") ||
		strings.Contains(s, "command not found") ||
		strings.Contains(s, "no such file")
}
