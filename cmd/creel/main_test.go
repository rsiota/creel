package main

import (
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestApplyOverrides(t *testing.T) {
	base := func() *db.ConnectionConfig {
		return &db.ConnectionConfig{
			Driver:   db.DriverMySQL,
			Database: "prod",
			Host:     "db.internal",
			Port:     3306,
			Username: "svc",
			Password: "secret-ref",
		}
	}

	t.Run("no flags set leaves conn untouched", func(t *testing.T) {
		c := base()
		before := *c
		applyOverrides(c, map[string]bool{}, "postgres", "other", "h", 1, "u", "p", "", "")
		if *c != before {
			t.Errorf("conn mutated: got %+v, want %+v", *c, before)
		}
	})

	t.Run("only database overrides", func(t *testing.T) {
		c := base()
		applyOverrides(c, map[string]bool{"database": true}, "ignored", "local_turniq", "ignored", 9999, "ignored", "ignored", "", "")
		if c.Database != "local_turniq" {
			t.Errorf("Database = %q, want local_turniq", c.Database)
		}
		// Other fields untouched.
		if c.Host != "db.internal" || c.Port != 3306 || c.Username != "svc" || c.Password != "secret-ref" {
			t.Errorf("unrelated fields changed: %+v", c)
		}
	})

	t.Run("multiple overrides", func(t *testing.T) {
		c := base()
		applyOverrides(c, map[string]bool{"host": true, "port": true, "user": true, "password": true}, "x", "x", "127.0.0.1", 13306, "admin", "pw", "", "")
		if c.Host != "127.0.0.1" || c.Port != 13306 || c.Username != "admin" || c.Password != "pw" {
			t.Errorf("overrides not applied: %+v", c)
		}
		// Database and driver untouched.
		if c.Database != "prod" || c.Driver != db.DriverMySQL {
			t.Errorf("database/driver changed: %+v", c)
		}
	})

	t.Run("driver override", func(t *testing.T) {
		c := base()
		applyOverrides(c, map[string]bool{"driver": true}, "postgres", "x", "x", 1, "x", "x", "", "")
		if c.Driver != db.DriverPostgres {
			t.Errorf("Driver = %q, want postgres", c.Driver)
		}
	})

	t.Run("sslmode and socket overrides", func(t *testing.T) {
		c := base()
		applyOverrides(c, map[string]bool{"sslmode": true, "socket": true}, "x", "x", "x", 1, "x", "x", "require", "/tmp/mysql.sock")
		if c.SSLMode != "require" || c.Socket != "/tmp/mysql.sock" {
			t.Errorf("ssl/socket not applied: %+v", c)
		}
	})
}
