package db

// SessionInfo describes one live database backend/connection for :who.
type SessionInfo struct {
	PID     string
	User    string
	Host    string // client address / host:port when known
	DB      string // current database when known
	State   string // active, idle, idle in transaction, Sleep, …
	Age     string // how long in this state / running, when known
	Query   string // current or last query
	Self    bool   // true when this is Creel's own connection
	Waiting bool   // true when the session appears to be waiting on a lock/IO
}
