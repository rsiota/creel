package db

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// MysqlHostOnSSHTarget reports whether MySQL is addressed as loopback on the
// SSH host (typical VPS: SSH and MySQL on the same machine).
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
func buildRemoteMysqlDumpScript(cfg ConnectionConfig) string {
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
	return fmt.Sprintf(`set -euo pipefail
umask 077
CNF=$(mktemp /tmp/creel-mysqldump.XXXXXX.cnf)
trap 'rm -f "$CNF"' EXIT
cat > "$CNF" <<'CREEL_EOF_CNF'
%s
CREEL_EOF_CNF
command -v mysqldump >/dev/null || { echo "mysqldump not found on SSH host" >&2; exit 127; }
mysqldump --defaults-extra-file="$CNF"%s --single-transaction --routines --events --max-allowed-packet=1073741824 %s
`, defaults, endpoint.String(), shellSingleQuote(cfg.Database))
}

// runRemoteMysqlDump runs mysqldump on the SSH host via the live tunnel and
// streams stdout into resultFile. This matches how large dumps succeed when
// run on the server (fast local MySQL) instead of through a localhost forward.
func (c *Connection) runRemoteMysqlDump(resultFile string, onBytes func(int64)) error {
	if c == nil || c.db == nil {
		return fmt.Errorf(":backup needs an active SSH connection")
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
	session.Stdin = strings.NewReader(buildRemoteMysqlDumpScript(c.config))
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
