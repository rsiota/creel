package db

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
)

func TestIsConnError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("syntax error at or near \"SELCT\""), false},
		{fmt.Errorf("column \"x\" is required"), false},
		{fmt.Errorf("query error: %w", fmt.Errorf("driver: bad connection")), true},
		{sql.ErrConnDone, true},
		{io.EOF, true},
		{io.ErrUnexpectedEOF, true},
		{net.ErrClosed, true},
		{fmt.Errorf("read tcp 1.2.3.4:5432: connection reset by peer"), true},
		{fmt.Errorf("ssh dial bastion: connection refused"), true},
		{fmt.Errorf("dial tcp: i/o timeout"), true},
		{errors.New("pq: sorry, too many clients already"), false},
	}
	for _, tc := range cases {
		got := IsConnError(tc.err)
		if got != tc.want {
			t.Errorf("IsConnError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestFormatConnectError(t *testing.T) {
	cases := []struct {
		driver Driver
		err    error
		want   string
	}{
		{DriverMySQL, fmt.Errorf("dial tcp 127.0.0.1:3306: connect: connection refused"),
			"MySQL is not running or not accepting connections on that host and port — dial tcp 127.0.0.1:3306: connect: connection refused"},
		{DriverMySQL, fmt.Errorf("dial tcp: i/o timeout"),
			"MySQL did not respond — the server may be stopped or unreachable — dial tcp: i/o timeout"},
		{DriverSQLite, fmt.Errorf("unable to open database file: no such file or directory"),
			"SQLite database file or parent directory does not exist — unable to open database file: no such file or directory"},
		{DriverSQLite, fmt.Errorf("database is locked"),
			"SQLite database is locked — another process may have it open — database is locked"},
		{DriverPostgres, fmt.Errorf("dial tcp 127.0.0.1:5432: connect: connection refused"),
			"PostgreSQL is not running or not accepting connections on that host and port — dial tcp 127.0.0.1:5432: connect: connection refused"},
		{DriverMySQL, fmt.Errorf("Error 1045 (28000): Access denied for user 'root'@'localhost'"),
			"Error 1045 (28000): Access denied for user 'root'@'localhost'"},
	}
	for _, tc := range cases {
		got := FormatConnectError(tc.driver, tc.err)
		if got != tc.want {
			t.Errorf("FormatConnectError(%q, %v)\n  got  %q\n  want %q", tc.driver, tc.err, got, tc.want)
		}
	}
}
