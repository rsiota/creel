package db

import (
	"database/sql"
	"errors"
	"io"
	"net"
	"strings"
)

// IsConnError reports whether err looks like a lost or unusable connection
// (dead SSH tunnel, idle kill, reset peer) rather than a SQL/user error.
// Used to trigger in-place reconnect instead of dumping the user out of the
// workspace.
func IsConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrConnDone) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	s := strings.ToLower(err.Error())
	for _, needle := range connErrorNeedles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// Substrings matched case-insensitively against driver / OS error text.
// Keep this list specific — "timeout" alone would catch statement timeouts.
var connErrorNeedles = []string{
	"driver: bad connection",
	"invalid connection",
	"broken pipe",
	"connection reset",
	"connection refused",
	"connect: connection refused",
	"no connection",
	"conn closed",
	"connection closed",
	"server closed the connection",
	"unexpected eof",
	"use of closed network connection",
	"sql: database is closed",
	"i/o timeout",
	"network is unreachable",
	"ssh: ",
	"ssh dial",
	"wsarecv",
	"wsasend",
}

// FormatConnectError turns a driver connect failure into a short, actionable
// message for the UI. When a known failure mode is recognized, a plain-language
// prefix is prepended; the original driver error is kept for detail.
func FormatConnectError(driver Driver, err error) string {
	if err == nil {
		return ""
	}
	raw := err.Error()
	msg := strings.ToLower(raw)

	switch driver {
	case DriverMySQL:
		switch {
		case strings.Contains(msg, "connection refused"):
			return "MySQL is not running or not accepting connections on that host and port — " + raw
		case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "connection timed out"):
			return "MySQL did not respond — the server may be stopped or unreachable — " + raw
		case strings.Contains(msg, "no such host"), strings.Contains(msg, "cannot resolve"):
			return "Could not resolve the MySQL host — " + raw
		}
	case DriverPostgres:
		switch {
		case strings.Contains(msg, "connection refused"):
			return "PostgreSQL is not running or not accepting connections on that host and port — " + raw
		case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "connection timed out"):
			return "PostgreSQL did not respond — the server may be stopped or unreachable — " + raw
		case strings.Contains(msg, "no such host"), strings.Contains(msg, "cannot resolve"):
			return "Could not resolve the PostgreSQL host — " + raw
		}
	case DriverSQLite:
		switch {
		case strings.Contains(msg, "no such file") || strings.Contains(msg, "no such file or directory") ||
			strings.Contains(msg, "unable to open database file (14)"):
			return "SQLite database file or parent directory does not exist — " + raw
		case strings.Contains(msg, "permission denied"):
			return "No permission to open the SQLite database file — " + raw
		case strings.Contains(msg, "database is locked"), strings.Contains(msg, "sqlite_busy"):
			return "SQLite database is locked — another process may have it open — " + raw
		case strings.Contains(msg, "unable to open database file"):
			return "Could not open the SQLite database file — " + raw
		}
	}
	return raw
}
