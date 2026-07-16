package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

// Tests for :tail — the append-only/event-table companion to :watch. It reuses
// the watch machinery (watchActive/watchInterval/watchGen + the tick chain) and
// adds a newest-first query built from the table's primary key. PK-driven
// ordering and composite-PK fallback need a live SQLite connection; the rest is
// pure dispatch. (runPageQuery's DB access lives in the returned command, which
// the tests never run.)

func TestExTailNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("tail events")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":tail with no connection -> %q", m.schemaMsg)
	}
}

func TestExTailNoTable(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("tail")
	if !strings.Contains(m.schemaMsg, "no current table") && !strings.Contains(m.schemaMsg, "name one") {
		t.Errorf(":tail with no table -> %q", m.schemaMsg)
	}
	if m.watchActive {
		t.Error(":tail with no table should not activate")
	}
}

func TestExTailBadInterval(t *testing.T) {
	m := &Model{connection: &db.Connection{}, tables: []string{"events"}}
	m.runExCommand("tail events abc")
	if !strings.Contains(m.schemaMsg, "interval") {
		t.Errorf(":tail bad interval -> %q", m.schemaMsg)
	}
	if m.watchActive {
		t.Error(":tail bad interval should not activate")
	}
}

// A single-column PK (the usual append-only shape) yields a newest-first query.
func TestExTailBuildsNewestFirstQuery(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Execute("CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, msg TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	m := &Model{connection: conn, tables: []string{"events"}}
	m.runExCommand("tail events")
	if !m.watchActive {
		t.Fatalf(":tail events -> not active; msg=%q", m.schemaMsg)
	}
	if m.watchMode != "tail" {
		t.Errorf("watchMode = %q, want tail", m.watchMode)
	}
	if m.watchInterval != defaultTailInterval {
		t.Errorf("interval = %v, want %v", m.watchInterval, defaultTailInterval)
	}
	if !strings.Contains(m.lastQuery, "ORDER BY") || !strings.Contains(m.lastQuery, "DESC") {
		t.Errorf("tail query should order by PK DESC: %q", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, `"id"`) {
		t.Errorf("tail query should order by the pk column: %q", m.lastQuery)
	}
	if !strings.Contains(m.schemaMsg, "tailing events") {
		t.Errorf("tail message -> %q", m.schemaMsg)
	}
}

// A composite PK is left unordered rather than guessing an ordering.
func TestExTailCompositePKNoOrderBy(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Execute("CREATE TABLE composite (a INTEGER, b INTEGER, msg TEXT, PRIMARY KEY(a,b))"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	m := &Model{connection: conn, tables: []string{"composite"}}
	m.runExCommand("tail composite")
	if !m.watchActive {
		t.Fatalf(":tail composite -> not active; msg=%q", m.schemaMsg)
	}
	if strings.Contains(m.lastQuery, "ORDER BY") {
		t.Errorf("composite-PK tail should be unordered: %q", m.lastQuery)
	}
}

func TestExTailExplicitInterval(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Execute("CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, msg TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	m := &Model{connection: conn, tables: []string{"events"}}
	m.runExCommand("tail events 3")
	if !m.watchActive || m.watchInterval != 3*time.Second {
		t.Errorf(":tail events 3 -> active=%v interval=%v", m.watchActive, m.watchInterval)
	}
}

// :tail off and :watch off are interchangeable — either stops whichever
// background refresh is running, and the stop message names the right one.
func TestExTailStopCrossVerb(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Execute("CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, msg TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Start a tail, stop with :watch off.
	m := &Model{connection: conn, tables: []string{"events"}}
	m.runExCommand("tail events")
	if !m.watchActive {
		t.Fatal("tail did not start")
	}
	m.runExCommand("watch off")
	if m.watchActive {
		t.Error(":watch off should stop an active tail")
	}
	if !strings.Contains(m.schemaMsg, "tail stopped") {
		t.Errorf("cross-verb stop message -> %q", m.schemaMsg)
	}

	// Start a watch, stop with :tail off.
	m2 := &Model{connection: conn, tables: []string{"events"}, lastQuery: "SELECT 1"}
	m2.runExCommand("watch")
	if !m2.watchActive {
		t.Fatal("watch did not start")
	}
	m2.runExCommand("tail off")
	if m2.watchActive {
		t.Error(":tail off should stop an active watch")
	}
	if !strings.Contains(m2.schemaMsg, "watch stopped") {
		t.Errorf("cross-verb stop message -> %q", m2.schemaMsg)
	}
}

// Stopping with nothing active is a no-op with a clear message.
func TestExTailStopNothingActive(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("tail off")
	if !strings.Contains(m.schemaMsg, "no active") {
		t.Errorf(":tail off with nothing running -> %q", m.schemaMsg)
	}
}

// The status bar shows TAIL (not WATCH) for an active tail.
func TestStatusBarTailIndicator(t *testing.T) {
	m := NewModel(&config.Config{})
	m.watchActive = true
	m.watchMode = "tail"
	m.watchInterval = 2 * time.Second
	sb := m.statusBar("")
	if !strings.Contains(sb, "TAIL 2s") {
		t.Errorf("status bar should show TAIL 2s: got %q", sb)
	}
	if strings.Contains(sb, "WATCH") {
		t.Errorf("status bar should not say WATCH for a tail: %q", sb)
	}
}
