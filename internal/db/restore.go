package db

import (
	"fmt"
	"strings"
)

// RestoreOpts configures a mysql/psql dump load (:restore).
type RestoreOpts struct {
	// ContinueOnError keeps loading after SQL errors (mysql --force, or
	// psql without ON_ERROR_STOP). Default false stops on the first error.
	ContinueOnError bool
	OnBytes         func(int64)
}

// RestoreResult is the outcome of RunMysqlRestore / RunPgRestore.
type RestoreResult struct {
	Bytes int64
	// Err is a hard failure (could not run the client, or stop-on-first SQL
	// error). When Continued is set and the client finished the stream with
	// SQL errors, Err is nil and ClientStderr holds the ignored messages.
	Err error
	// ClientStderr is the raw client stderr (trimmed). Useful for the error
	// review overlay after a continue-on-error restore.
	ClientStderr string
	Continued    bool
}

// finalizeCLIRestore maps a client exit into RestoreResult. With
// ContinueOnError, a non-zero exit after the stream has started (or with
// stderr) is treated as soft success so ignored SQL errors can be reviewed.
// Missing-client messages stay hard so SSH remotes can fall back to a local
// binary.
func finalizeCLIRestore(prefix string, continued bool, bytes int64, stderr string, runErr error) RestoreResult {
	out := strings.TrimSpace(stderr)
	r := RestoreResult{Bytes: bytes, Continued: continued, ClientStderr: out}
	if runErr == nil {
		return r
	}
	if continued && (out != "" || bytes > 0) && !clientBinaryMissing(out) {
		return r
	}
	if out != "" {
		r.Err = fmt.Errorf("%s: %s", prefix, out)
	} else {
		r.Err = fmt.Errorf("%s: %w", prefix, runErr)
	}
	return r
}

func clientBinaryMissing(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "not found") ||
		strings.Contains(s, "command not found") ||
		strings.Contains(s, "no such file")
}

// StderrErrorLines splits client stderr into non-empty lines for an error
// review overlay. Caps at 500 rows so a noisy dump cannot blow up the UI.
func StderrErrorLines(stderr string) []string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return nil
	}
	raw := strings.Split(stderr, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= 500 {
			break
		}
	}
	return out
}
