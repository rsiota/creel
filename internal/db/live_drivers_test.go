package db

import (
	"os"
	"strconv"
	"testing"
)

// liveDBEnabled is set in CI (and optionally locally) to exercise real
// MySQL/Postgres servers. Without it, live tests Skip so `go test ./...`
// stays offline-friendly.
func liveDBEnabled() bool {
	return os.Getenv("CREEL_LIVE_DB") == "1"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envPort(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func livePostgresConfig() ConnectionConfig {
	return ConnectionConfig{
		Driver:   DriverPostgres,
		Host:     envOr("CREEL_PG_HOST", "127.0.0.1"),
		Port:     envPort("CREEL_PG_PORT", 5432),
		Username: envOr("CREEL_PG_USER", "postgres"),
		Password: envOr("CREEL_PG_PASSWORD", "postgres"),
		Database: envOr("CREEL_PG_DATABASE", "postgres"),
		SSLMode:  "disable",
	}
}

func liveMySQLConfig() ConnectionConfig {
	return ConnectionConfig{
		Driver:   DriverMySQL,
		Host:     envOr("CREEL_MYSQL_HOST", "127.0.0.1"),
		Port:     envPort("CREEL_MYSQL_PORT", 3306),
		Username: envOr("CREEL_MYSQL_USER", "root"),
		Password: envOr("CREEL_MYSQL_PASSWORD", "root"),
		Database: envOr("CREEL_MYSQL_DATABASE", "creel"),
		SSLMode:  "disable",
	}
}

func TestLivePostgresConnectAndSchema(t *testing.T) {
	if !liveDBEnabled() {
		t.Skip("set CREEL_LIVE_DB=1 to run live Postgres tests")
	}
	conn, err := New(livePostgresConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.DB().Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	dbs, err := conn.DB().Databases()
	if err != nil {
		t.Fatalf("databases: %v", err)
	}
	if len(dbs) == 0 {
		t.Fatal("expected at least one database")
	}
	if err := conn.EnsureActiveSchema(); err != nil {
		t.Fatal(err)
	}
	if got := conn.Config().Schema; got != "public" {
		t.Fatalf("Schema = %q, want public", got)
	}
}

func TestLiveMySQLConnectAndTables(t *testing.T) {
	if !liveDBEnabled() {
		t.Skip("set CREEL_LIVE_DB=1 to run live MySQL tests")
	}
	conn, err := New(liveMySQLConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.DB().Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	// Create a throwaway table so Tables() is non-empty on a fresh service DB.
	if _, err := conn.DB().Exec(`CREATE TABLE IF NOT EXISTS creel_live_smoke (
		id INT PRIMARY KEY,
		name VARCHAR(32) NOT NULL
	)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.DB().Exec(`DROP TABLE IF EXISTS creel_live_smoke`)
	})
	tables, err := conn.DB().Tables()
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	found := false
	for _, name := range tables {
		if name == "creel_live_smoke" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected creel_live_smoke in %v", tables)
	}
}
