package main

import (
	"path/filepath"
	"strings"
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

func TestResolveCLIQuery(t *testing.T) {
	t.Run("literal -e value", func(t *testing.T) {
		got, err := resolveCLIQuery("SELECT 1", strings.NewReader("ignored"))
		if err != nil || got != "SELECT 1" {
			t.Fatalf("got %q err=%v", got, err)
		}
	})

	t.Run("-e - reads stdin", func(t *testing.T) {
		got, err := resolveCLIQuery("-", strings.NewReader("  SELECT 2;\n"))
		if err != nil || got != "SELECT 2;" {
			t.Fatalf("got %q err=%v", got, err)
		}
	})

	t.Run("empty -e - is an error", func(t *testing.T) {
		_, err := resolveCLIQuery("-", strings.NewReader("  \n"))
		if err == nil || !strings.Contains(err.Error(), "empty stdin") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("bare -cli reads stdin", func(t *testing.T) {
		got, err := resolveCLIQuery("", strings.NewReader("SELECT 3"))
		if err != nil || got != "SELECT 3" {
			t.Fatalf("got %q err=%v", got, err)
		}
	})

	t.Run("bare -cli with empty stdin errors", func(t *testing.T) {
		_, err := resolveCLIQuery("", strings.NewReader(""))
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRunCLISuccessAndQueryError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")
	cfg := &db.ConnectionConfig{Driver: db.DriverSQLite, Database: path}

	mustCLI(t, cfg, "CREATE TABLE t(id INTEGER)")
	mustCLI(t, cfg, "INSERT INTO t VALUES (42)")

	if err := runCLI(cfg, "SELECT id FROM t", "tsv"); err != nil {
		t.Fatalf("success path: %v", err)
	}

	err := runCLI(cfg, "SELECT * FROM definitely_missing", "tsv")
	if err == nil {
		t.Fatal("expected query error to propagate (non-zero exit path)")
	}
}

func mustCLI(t *testing.T, cfg *db.ConnectionConfig, q string) {
	t.Helper()
	if err := runCLI(cfg, q, "tsv"); err != nil {
		t.Fatalf("runCLI(%q): %v", q, err)
	}
}
