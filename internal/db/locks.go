package db

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// LockWait describes one session waiting on a lock held by another session.
type LockWait struct {
	WaitingPID    string
	WaitingUser   string
	WaitingQuery  string
	BlockingPID   string
	BlockingUser  string
	BlockingQuery string
	BlockingState string // e.g. "idle in transaction"
	LockType      string
	Relation      string // schema.table or table when known
	WaitDuration  string // human-readable age when the driver provides it
}

// TruncateQuery shortens q for status/lookup display.
func TruncateQuery(q string, max int) string {
	q = strings.Join(strings.Fields(q), " ")
	if max < 1 || utf8.RuneCountInString(q) <= max {
		return q
	}
	runes := []rune(q)
	return string(runes[:max-1]) + "…"
}

// FormatLockWaiter renders "pid (user)" for the lookup panel.
func FormatLockWaiter(pid, user string) string {
	pid = strings.TrimSpace(pid)
	user = strings.TrimSpace(user)
	if user == "" {
		return pid
	}
	return fmt.Sprintf("%s (%s)", pid, user)
}

// FormatLockBlocker renders the blocker cell, including state when useful.
func FormatLockBlocker(pid, user, state string) string {
	base := FormatLockWaiter(pid, user)
	state = strings.TrimSpace(state)
	if state == "" || strings.EqualFold(state, "active") {
		return base
	}
	return fmt.Sprintf("%s · %s", base, state)
}

// FormatSessionPID renders a :who pid cell, marking Creel's own connection.
func FormatSessionPID(pid string, self bool) string {
	pid = strings.TrimSpace(pid)
	if self {
		return pid + " · you"
	}
	return pid
}

func parseSessionPID(pid string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(pid), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid session id %q", pid)
	}
	return id, nil
}
