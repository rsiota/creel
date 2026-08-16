package db

import (
	"database/sql"
	"time"
)

// configurePool applies shared pool limits for MySQL and Postgres. Idle and
// lifetime caps recycle connections before cloud LBs / idle_session_timeout
// silently kill them; the UI keep-alive ping covers the rest.
func configurePool(db *sql.DB) {
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	// Call Idle before Lifetime so the cleaner wakes on the shorter interval
	// (see golang/go#45993).
	db.SetConnMaxIdleTime(3 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
}
