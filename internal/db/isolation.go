package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// IsolationLevel is the transaction isolation requested by :begin.
// IsolationDefault leaves the choice to the driver (SQLite serializable,
// MySQL/Postgres their engine defaults).
type IsolationLevel int

const (
	IsolationDefault IsolationLevel = iota
	IsolationReadUncommitted
	IsolationReadCommitted
	IsolationRepeatableRead
	IsolationSerializable
)

// String returns a short label for the status bar / messages.
func (l IsolationLevel) String() string {
	switch l {
	case IsolationReadUncommitted:
		return "read uncommitted"
	case IsolationReadCommitted:
		return "read committed"
	case IsolationRepeatableRead:
		return "repeatable read"
	case IsolationSerializable:
		return "serializable"
	default:
		return "default"
	}
}

// Short returns a compact status-bar token (RC, RR, S, RU), or "" for default.
func (l IsolationLevel) Short() string {
	switch l {
	case IsolationReadUncommitted:
		return "RU"
	case IsolationReadCommitted:
		return "RC"
	case IsolationRepeatableRead:
		return "RR"
	case IsolationSerializable:
		return "S"
	default:
		return ""
	}
}

// SQL maps to database/sql's isolation constants.
func (l IsolationLevel) SQL() sql.IsolationLevel {
	switch l {
	case IsolationReadUncommitted:
		return sql.LevelReadUncommitted
	case IsolationReadCommitted:
		return sql.LevelReadCommitted
	case IsolationRepeatableRead:
		return sql.LevelRepeatableRead
	case IsolationSerializable:
		return sql.LevelSerializable
	default:
		return sql.LevelDefault
	}
}

// ParseIsolation accepts forms like "serializable", "s", "repeatable read",
// "repeatable-read", "rr", "read committed", "rc", etc.
func ParseIsolation(s string) (IsolationLevel, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.Join(strings.Fields(s), " ")
	switch s {
	case "", "default":
		return IsolationDefault, nil
	case "serializable", "serial", "s":
		return IsolationSerializable, nil
	case "repeatable read", "repeatable", "rr":
		return IsolationRepeatableRead, nil
	case "read committed", "committed", "rc":
		return IsolationReadCommitted, nil
	case "read uncommitted", "uncommitted", "ru":
		return IsolationReadUncommitted, nil
	default:
		return IsolationDefault, fmt.Errorf("unknown isolation %q (want serializable, repeatable read, read committed, read uncommitted)", s)
	}
}
