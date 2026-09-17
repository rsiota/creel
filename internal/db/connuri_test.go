package db

import (
	"testing"
)

func TestLooksLikeConnectionURI(t *testing.T) {
	yes := []string{
		"postgres://u@h/db",
		"postgresql://localhost/db",
		"mysql://root@localhost/app",
		"mariadb://u:p@h:3306/db",
		"sqlite:///tmp/x.db",
		"file:///tmp/x.db",
		`  "postgres://x"  `,
	}
	no := []string{"", "localhost", "/tmp/x.db", "http://example.com", "postgres"}
	for _, s := range yes {
		if !LooksLikeConnectionURI(s) {
			t.Errorf("LooksLikeConnectionURI(%q) = false", s)
		}
	}
	for _, s := range no {
		if LooksLikeConnectionURI(s) {
			t.Errorf("LooksLikeConnectionURI(%q) = true", s)
		}
	}
}

func TestParseConnectionURI_Postgres(t *testing.T) {
	cfg, err := ParseConnectionURI("postgres://alice:s3cret@db.example:5433/orders?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Driver != DriverPostgres {
		t.Fatalf("driver = %s", cfg.Driver)
	}
	if cfg.Username != "alice" || cfg.Password != "s3cret" {
		t.Fatalf("user/pass = %q/%q", cfg.Username, cfg.Password)
	}
	if cfg.Host != "db.example" || cfg.Port != 5433 {
		t.Fatalf("host/port = %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.Database != "orders" {
		t.Fatalf("database = %q", cfg.Database)
	}
	if cfg.SSLMode != "require" {
		t.Fatalf("sslmode = %q", cfg.SSLMode)
	}
}

func TestParseConnectionURI_PostgresSocket(t *testing.T) {
	cfg, err := ParseConnectionURI("postgresql:///mydb?host=/var/run/postgresql")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Socket != "/var/run/postgresql" {
		t.Fatalf("socket = %q", cfg.Socket)
	}
	if cfg.Database != "mydb" {
		t.Fatalf("database = %q", cfg.Database)
	}
	if cfg.Host != "" {
		t.Fatalf("host should be empty, got %q", cfg.Host)
	}
}

func TestParseConnectionURI_PasswordEncoding(t *testing.T) {
	cfg, err := ParseConnectionURI("postgres://u:p%40ss@localhost/db")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "p@ss" {
		t.Fatalf("password = %q, want p@ss", cfg.Password)
	}
}

func TestParseConnectionURI_MySQL(t *testing.T) {
	cfg, err := ParseConnectionURI("mysql://root:secret@127.0.0.1:3307/app?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Driver != DriverMySQL {
		t.Fatalf("driver = %s", cfg.Driver)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 3307 {
		t.Fatalf("host/port = %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.Database != "app" || cfg.SSLMode != "disable" {
		t.Fatalf("db/ssl = %q/%q", cfg.Database, cfg.SSLMode)
	}
}

func TestParseConnectionURI_MySQLSocket(t *testing.T) {
	cfg, err := ParseConnectionURI("mysql://root@localhost/app?socket=/tmp/mysql.sock")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Socket != "/tmp/mysql.sock" {
		t.Fatalf("socket = %q", cfg.Socket)
	}
}

func TestParseConnectionURI_SQLite(t *testing.T) {
	cfg, err := ParseConnectionURI("sqlite:////tmp/demo.db")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Driver != DriverSQLite {
		t.Fatalf("driver = %s", cfg.Driver)
	}
	if cfg.Database != "/tmp/demo.db" {
		t.Fatalf("database = %q, want /tmp/demo.db", cfg.Database)
	}

	cfg, err = ParseConnectionURI("file:///tmp/other.db")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database != "/tmp/other.db" {
		t.Fatalf("file URI database = %q", cfg.Database)
	}
}

func TestParseConnectionURI_Unsupported(t *testing.T) {
	if _, err := ParseConnectionURI("http://example.com/db"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSuggestConnectionName(t *testing.T) {
	if got := SuggestConnectionName(ConnectionConfig{
		Driver: DriverPostgres, Host: "db.example", Database: "orders",
	}); got != "db.example/orders" {
		t.Fatalf("got %q", got)
	}
	if got := SuggestConnectionName(ConnectionConfig{
		Driver: DriverSQLite, Database: "/tmp/creel-demo.db",
	}); got != "creel-demo.db" {
		t.Fatalf("got %q", got)
	}
}
