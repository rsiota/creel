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
