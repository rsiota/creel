package db

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// MysqlHostOnSSHTarget reports whether the database host is loopback on the
// SSH machine (typical VPS: SSH and MySQL/Postgres on the same host).
func MysqlHostOnSSHTarget(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "" || h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// shellSingleQuote quotes s for a POSIX shell.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// buildRemoteMysqlDumpScript is a bash script run on the SSH host: write a
// 0600 defaults file, run mysqldump to stdout (streamed back over SSH), then
// remove the defaults file. mysqldump talks to local MySQL — no tunnel proxy.
// Selective plans may emit two mysqldump invocations (schema, then data).
func buildRemoteMysqlDumpScript(cfg ConnectionConfig, plan DumpPlan) string {
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
	defaults := MysqlDumpDefaults(cfg)
	dbArg := shellSingleQuote(cfg.Database)
	var cmds strings.Builder
	for _, opt := range mysqlDumpPasses(plan, false) {
		cmds.WriteString("mysqldump --defaults-extra-file=\"$CNF\"")
		cmds.WriteString(endpoint.String())
		cmds.WriteString(" --single-transaction")
		if opt.Routines {
			cmds.WriteString(" --routines")
		}
		if opt.Events {
			cmds.WriteString(" --events")
		}
		cmds.WriteString(" --max-allowed-packet=1073741824")
		if opt.NoData {
			cmds.WriteString(" --no-data")
		}
		if opt.NoCreateInfo {
			cmds.WriteString(" --no-create-info")
		}
		cmds.WriteString(" ")
		cmds.WriteString(dbArg)
		for _, t := range opt.Tables {
			cmds.WriteString(" ")
			cmds.WriteString(shellSingleQuote(t))
		}
		cmds.WriteString("\n")
	}
	return fmt.Sprintf(`set -euo pipefail
umask 077
CNF=$(mktemp /tmp/creel-mysqldump.XXXXXX.cnf)
trap 'rm -f "$CNF"' EXIT
cat > "$CNF" <<'CREEL_EOF_CNF'
%s
CREEL_EOF_CNF
command -v mysqldump >/dev/null || { echo "mysqldump not found on SSH host" >&2; exit 127; }
%s`, defaults, cmds.String())
}

// runRemoteMysqlDump runs mysqldump on the SSH host via the live tunnel and
// streams stdout into resultFile. This matches how large dumps succeed when
// run on the server (fast local MySQL) instead of through a localhost forward.
func (c *Connection) runRemoteMysqlDump(resultFile string, plan DumpPlan, onBytes func(int64)) error {
	if c == nil || c.db == nil {
		return fmt.Errorf(":backup needs an active SSH connection")
	}
	if !plan.HasContent() {
		return fmt.Errorf("nothing selected to backup")
	}
	m, ok := c.db.(*MySQL)
	if !ok || m.tunnel == nil {
		return fmt.Errorf("no active SSH tunnel")
	}
	out, err := os.Create(resultFile)
	if err != nil {
		return err
	}
	defer out.Close()

	session, err := m.tunnel.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stdout = &countingWriter{w: out, onBytes: onBytes}
	session.Stderr = &stderr
	session.Stdin = strings.NewReader(buildRemoteMysqlDumpScript(c.config, plan))
	if err := session.Run("bash -s"); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("remote mysqldump: %w", err)
		}
		return fmt.Errorf("remote mysqldump: %s", msg)
	}
	return nil
}

func remoteDumpUnavailable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "mysqldump not found") ||
		strings.Contains(s, "command not found") ||
		strings.Contains(s, "no such file")
}
