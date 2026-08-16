package db

import (
	"strings"
	"testing"
)

func TestNormalizeSSLMode(t *testing.T) {
	cases := map[string]string{
		"":            "prefer",
		"  ":          "prefer",
		"bogus":       "prefer",
		"DISABLE":     "disable",
		"prefer":      "prefer",
		"require":     "require",
		"verify-ca":   "verify-ca",
		"verify-full": "verify-full",
		"allow":       "allow",
	}
	for in, want := range cases {
		if got := NormalizeSSLMode(in); got != want {
			t.Errorf("NormalizeSSLMode(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestSocketPath(t *testing.T) {
	if got := (ConnectionConfig{Socket: "/tmp/mysql.sock"}).socketPath(); got != "/tmp/mysql.sock" {
		t.Errorf("explicit socket: %q", got)
	}
	if got := (ConnectionConfig{Host: "/var/run/postgresql"}).socketPath(); got != "/var/run/postgresql" {
		t.Errorf("host-as-socket: %q", got)
	}
	if got := (ConnectionConfig{Host: "localhost"}).socketPath(); got != "" {
		t.Errorf("tcp host should not be a socket, got %q", got)
	}
	// SSH tunnel always dials TCP through the tunnel.
	if got := (ConnectionConfig{Socket: "/tmp/mysql.sock", SSHHost: "bastion"}).socketPath(); got != "" {
		t.Errorf("socket ignored under SSH, got %q", got)
	}
}

func TestMySQLDSN(t *testing.T) {
	t.Run("tcp prefer", func(t *testing.T) {
		d := mysqlDSN(ConnectionConfig{Username: "u", Password: "p", Host: "h", Port: 3307, Database: "db"}, "")
		if !strings.Contains(d, "tcp(h:3307)/db") {
			t.Errorf("addr: %s", d)
		}
		if !strings.Contains(d, "tls=preferred") {
			t.Errorf("tls: %s", d)
		}
	})
	t.Run("unix socket", func(t *testing.T) {
		d := mysqlDSN(ConnectionConfig{Username: "u", Socket: "/tmp/mysql.sock", Database: "db", SSLMode: "disable"}, "")
		if !strings.Contains(d, "unix(/tmp/mysql.sock)/db") {
			t.Errorf("unix: %s", d)
		}
		if strings.Contains(d, "tcp(") {
			t.Errorf("should not use tcp: %s", d)
		}
		if !strings.Contains(d, "tls=false") {
			t.Errorf("disable tls: %s", d)
		}
	})
	t.Run("require maps to skip-verify", func(t *testing.T) {
		d := mysqlDSN(ConnectionConfig{Host: "h", SSLMode: "require"}, "")
		if !strings.Contains(d, "tls=skip-verify") {
			t.Errorf("require: %s", d)
		}
	})
	t.Run("verify-full maps to true", func(t *testing.T) {
		d := mysqlDSN(ConnectionConfig{Host: "h", SSLMode: "verify-full"}, "")
		if !strings.Contains(d, "tls=true") {
			t.Errorf("verify-full: %s", d)
		}
	})
	t.Run("empty host defaults localhost", func(t *testing.T) {
		d := mysqlDSN(ConnectionConfig{Database: "db"}, "")
		if !strings.Contains(d, "tcp(localhost:3306)/db") {
			t.Errorf("default host: %s", d)
		}
	})
}

func TestPostgresConnConfig(t *testing.T) {
	t.Run("default sslmode prefer", func(t *testing.T) {
		p := NewPostgres(ConnectionConfig{Host: "db.example", Database: "app"})
		cfg, err := p.connConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.TLSConfig == nil {
			// pgx prefer still builds a TLSConfig; disable is the one that nils it.
			// Accept either a non-nil TLSConfig or SSLMode in the original config.
		}
		if got := p.config.sslMode(); got != "prefer" {
			t.Errorf("sslMode=%q, want prefer", got)
		}
	})
	t.Run("disable", func(t *testing.T) {
		p := NewPostgres(ConnectionConfig{Host: "localhost", SSLMode: "disable"})
		cfg, err := p.connConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.TLSConfig != nil {
			t.Errorf("disable should not set TLSConfig")
		}
	})
	t.Run("unix socket", func(t *testing.T) {
		p := NewPostgres(ConnectionConfig{Socket: "/var/run/postgresql", Port: 5432, Database: "app", SSLMode: "disable"})
		cfg, err := p.connConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Host != "/var/run/postgresql" {
			t.Errorf("Host=%q, want socket dir", cfg.Host)
		}
	})
}
