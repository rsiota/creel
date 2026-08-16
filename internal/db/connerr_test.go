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
