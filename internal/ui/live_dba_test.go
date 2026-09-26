package ui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

// Command-level live coverage for :who, :locks, :explain, and :explain!.
// The db package owns the lock-wait / kill / ANALYZE assertions; this file
// checks the TUI wrappers format those results the way the overlay expects.

func TestLiveExWhoLocksExplainPostgres(t *testing.T) {
	liveExWhoLocksExplain(t, liveUIPostgresConfig())
}

func TestLiveExWhoLocksExplainMySQL(t *testing.T) {
	liveExWhoLocksExplain(t, liveUIMySQLConfig())
}

func liveExWhoLocksExplain(t *testing.T, cfg db.ConnectionConfig) {
	t.Helper()
	conn := liveUIConnect(t, cfg)
	off := false
	m := &Model{
		connection: conn,
		editor:     NewQueryEditor(),
		state:      stateWorkspace,
		settings:   config.Settings{ConfirmDestructive: &off},
	}

	who := m.runExCommand("who")
	if who == nil {
		t.Fatalf(":who returned nil, schemaMsg=%q", m.schemaMsg)
	}
	whoMsg, ok := who().(lookupResultMsg)
	if !ok {
		t.Fatalf(":who got %T", who())
	}
	if whoMsg.err != nil {
		t.Fatalf(":who: %v", whoMsg.err)
	}
	if !strings.Contains(whoMsg.title, "Sessions") {
		t.Fatalf(":who title = %q", whoMsg.title)
	}
	foundYou := false
	for _, row := range whoMsg.result.Rows {
		if len(row) > 0 && strings.Contains(row[0], "you") {
			foundYou = true
			break
		}
	}
	if !foundYou {
		t.Fatalf(":who missing · you row: %+v", whoMsg.result.Rows)
	}

	locks := m.runExCommand("locks")
	if locks == nil {
		t.Fatalf(":locks returned nil, schemaMsg=%q", m.schemaMsg)
	}
	lockMsg, ok := locks().(lookupResultMsg)
	if !ok {
		t.Fatalf(":locks got %T", locks())
	}
	if lockMsg.err != nil {
		t.Fatalf(":locks: %v", lockMsg.err)
	}
	if !strings.Contains(lockMsg.title, "Lock waits") {
		t.Fatalf(":locks title = %q", lockMsg.title)
	}

	m.editor.SetValue("SELECT 1")
	explain := m.explainQueryOpts(false, "", false)
	if explain == nil {
		t.Fatal(":explain returned nil")
	}
	exMsg := explain().(explainResultMsg)
	if exMsg.err != nil {
		t.Fatalf(":explain: %v", exMsg.err)
	}
	if exMsg.analyze {
		t.Fatal(":explain set analyze")
	}
	if len(exMsg.result.Rows) == 0 {
		t.Fatal(":explain returned no rows")
	}

	analyze := m.explainQueryOpts(false, "", true)
	if analyze == nil {
		t.Fatal(":explain! returned nil")
	}
	anMsg := analyze().(explainResultMsg)
	if anMsg.err != nil {
		t.Fatalf(":explain!: %v", anMsg.err)
	}
	if !anMsg.analyze {
		t.Fatal(":explain! did not set analyze")
	}
	if len(anMsg.result.Rows) == 0 {
		t.Fatal(":explain! returned no rows")
	}
	text := strings.ToLower(strings.Join(flattenRows(anMsg.result.Rows), "\n"))
	if !strings.Contains(text, "actual time") && !strings.Contains(text, "execution time") {
		t.Fatalf(":explain! missing timing:\n%s", text)
	}

	diag := m.runExCommand("diagnose")
	if diag == nil {
		t.Fatalf(":diagnose returned nil, schemaMsg=%q", m.schemaMsg)
	}
	diagMsg := diag().(diagnoseResultMsg)
	if diagMsg.err != nil {
		t.Fatalf(":diagnose: %v", diagMsg.err)
	}
	if len(diagMsg.result.Rows) == 0 {
		t.Fatal(":diagnose returned no findings")
	}
}

func flattenRows(rows [][]string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, strings.Join(row, " "))
	}
	return out
}

func liveUIConnect(t *testing.T, cfg db.ConnectionConfig) *db.Connection {
	t.Helper()
	if os.Getenv("CREEL_LIVE_DB") != "1" {
		t.Skip("set CREEL_LIVE_DB=1 to run live MySQL/Postgres tests")
	}
	if cfg.Driver == db.DriverMySQL && cfg.Database != "" {
		if err := liveUIEnsureMySQL(cfg); err != nil {
			if os.Getenv("CI") == "" {
				t.Skipf("live mysql unavailable: %v", err)
			}
			t.Fatalf("ensure mysql database %q: %v", cfg.Database, err)
		}
	}
	conn, err := db.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		if os.Getenv("CI") == "" {
			t.Skipf("live %s unavailable: %v", cfg.Driver, err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func liveUIEnsureMySQL(cfg db.ConnectionConfig) error {
	want := cfg.Database
	cfg.Database = ""
	conn, err := db.New(cfg)
	if err != nil {
		return err
	}
	if err := conn.Connect(); err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.DB().Exec("CREATE DATABASE IF NOT EXISTS `" + want + "`")
	return err
}

func liveUIPostgresConfig() db.ConnectionConfig {
	return db.ConnectionConfig{
		Driver:   db.DriverPostgres,
		Host:     liveUIEnv("CREEL_PG_HOST", "127.0.0.1"),
		Port:     liveUIPort("CREEL_PG_PORT", 5432),
		Username: liveUIEnv("CREEL_PG_USER", "postgres"),
		Password: liveUIEnv("CREEL_PG_PASSWORD", "postgres"),
		Database: liveUIEnv("CREEL_PG_DATABASE", "postgres"),
		SSLMode:  "disable",
	}
}

func liveUIMySQLConfig() db.ConnectionConfig {
	return db.ConnectionConfig{
		Driver:   db.DriverMySQL,
		Host:     liveUIEnv("CREEL_MYSQL_HOST", "127.0.0.1"),
		Port:     liveUIPort("CREEL_MYSQL_PORT", 3306),
		Username: liveUIEnv("CREEL_MYSQL_USER", "root"),
		Password: liveUIEnv("CREEL_MYSQL_PASSWORD", "root"),
		Database: liveUIEnv("CREEL_MYSQL_DATABASE", "creel"),
		SSLMode:  "disable",
	}
}

func liveUIEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func liveUIPort(key string, fallback int) int {
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
