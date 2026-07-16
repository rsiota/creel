package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

// Tests for :watch — the first stateful ex command (toggle + timer + status
// indicator). Interval parsing, start/stop/restart semantics, the tick handler's
// refresh/skip/self-terminate rules, and the status-bar indicator are all
// covered without a live database: runPageQuery's DB access lives inside the
// returned command, which the tests never execute.

func TestParseWatchInterval(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"3", 3 * time.Second, true},      // bare integer = seconds
		{"10", 10 * time.Second, true},
		{"3s", 3 * time.Second, true},     // Go duration
		{"1m", time.Minute, true},
		{"90s", 90 * time.Second, true},
		{"0", 0, false},                   // below minimum
		{"0.5", 0, false},                 // sub-second rejected
		{"500ms", 0, false},               // sub-second rejected
		{"abc", 0, false},                 // garbage
		{"", 0, false},
		{"-5", 0, false},                  // negative
	}
	for _, c := range cases {
		got, ok := parseWatchInterval(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseWatchInterval(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{time.Minute, "1m"},
		{90 * time.Second, "90s"},
		{2 * time.Minute, "2m"},
	}
	for _, c := range cases {
		if got := humanDuration(c.d); got != c.want {
			t.Errorf("humanDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestExWatchNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("watch")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":watch with no connection -> %q", m.schemaMsg)
	}
}

func TestExWatchNothingToWatch(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("watch")
	if !strings.Contains(m.schemaMsg, "nothing to watch") {
		t.Errorf(":watch with no last query -> %q", m.schemaMsg)
	}
	if m.watchActive {
		t.Error("watch should not activate without a query")
	}
}

func TestExWatchBadInterval(t *testing.T) {
	for _, in := range []string{"watch abc", "watch 0.5", "watch 500ms"} {
		m := &Model{connection: &db.Connection{}, lastQuery: "SELECT 1"}
		m.runExCommand(in)
		if !strings.Contains(m.schemaMsg, "interval") {
			t.Errorf(":%s -> %q, want an interval error", in, m.schemaMsg)
		}
		if m.watchActive {
			t.Errorf(":%s should not activate watch", in)
		}
	}
}

func TestExWatchStartDefaultsAndExplicit(t *testing.T) {
	// No argument -> default interval.
	m := &Model{connection: &db.Connection{}, lastQuery: "SELECT 1"}
	m.runExCommand("watch")
	if !m.watchActive || m.watchInterval != defaultWatchInterval {
		t.Errorf(":watch default -> active=%v interval=%v", m.watchActive, m.watchInterval)
	}
	if !strings.Contains(m.schemaMsg, "watching every") {
		t.Errorf(":watch message -> %q", m.schemaMsg)
	}

	// Explicit bare-integer seconds.
	m = &Model{connection: &db.Connection{}, lastQuery: "SELECT 1"}
	m.runExCommand("watch 3")
	if !m.watchActive || m.watchInterval != 3*time.Second {
		t.Errorf(":watch 3 -> active=%v interval=%v", m.watchActive, m.watchInterval)
	}

	// Duration form.
	m = &Model{connection: &db.Connection{}, lastQuery: "SELECT 1"}
	m.runExCommand("watch 1m")
	if !m.watchActive || m.watchInterval != time.Minute {
		t.Errorf(":watch 1m -> active=%v interval=%v", m.watchActive, m.watchInterval)
	}
}

func TestExWatchStopForms(t *testing.T) {
	for _, stop := range []string{"off", "stop", "0"} {
		m := &Model{connection: &db.Connection{}, lastQuery: "SELECT 1"}
		m.runExCommand("watch") // activate
		if !m.watchActive {
			t.Fatalf("setup: watch did not activate")
		}
		m.runExCommand("watch " + stop)
		if m.watchActive {
			t.Errorf(":watch %s should stop the watch", stop)
		}
	}
}

// Restarting with a new interval bumps watchGen so the prior tick chain dies
// instead of stacking a second refresh loop.
func TestExWatchRestartBumpsGen(t *testing.T) {
	m := &Model{connection: &db.Connection{}, lastQuery: "SELECT 1"}
	m.runExCommand("watch 5")
	gen1 := m.watchGen
	if gen1 == 0 {
		t.Fatal("watch should set a non-zero generation")
	}
	m.runExCommand("watch 2")
	if m.watchGen != gen1+1 {
		t.Errorf("restart should bump gen: got %d, want %d", m.watchGen, gen1+1)
	}
	if m.watchInterval != 2*time.Second {
		t.Errorf("restart should update interval: got %v", m.watchInterval)
	}
}

// handleWatchTick: the core refresh/skip/self-terminate rules.
func TestHandleWatchTick(t *testing.T) {
	base := func() Model {
		return Model{
			connection:    &db.Connection{},
			lastQuery:     "SELECT 1",
			watchActive:   true,
			watchInterval: 5 * time.Second,
			watchGen:      7,
		}
	}

	// Idle -> refreshes (runPageQuery sets queryStart) and stays active.
	out, _ := base().handleWatchTick(watchTickMsg{gen: 7})
	if out.queryStart.IsZero() {
		t.Error("idle tick should trigger a refresh (runPageQuery sets queryStart)")
	}
	if !out.watchActive {
		t.Error("watch should stay active after an idle tick")
	}

	// A query already in flight -> skip the refresh (queryStart stays zero).
	b := base()
	b.queryRunning = true
	out, _ = b.handleWatchTick(watchTickMsg{gen: 7})
	if !out.queryStart.IsZero() {
		t.Error("busy tick should skip the refresh to avoid cancel-thrashing")
	}

	// Stale generation -> ignored entirely.
	out, _ = base().handleWatchTick(watchTickMsg{gen: 999})
	if !out.queryStart.IsZero() {
		t.Error("stale-gen tick should be ignored")
	}

	// Stopped -> ignored.
	b = base()
	b.watchActive = false
	out, _ = b.handleWatchTick(watchTickMsg{gen: 7})
	if !out.queryStart.IsZero() {
		t.Error("stopped watch tick should be ignored")
	}

	// lastQuery cleared -> self-terminates.
	b = base()
	b.lastQuery = ""
	out, _ = b.handleWatchTick(watchTickMsg{gen: 7})
	if out.watchActive {
		t.Error("watch should self-terminate when lastQuery is cleared")
	}

	// Disconnected -> self-terminates.
	b = base()
	b.connection = nil
	out, _ = b.handleWatchTick(watchTickMsg{gen: 7})
	if out.watchActive {
		t.Error("watch should self-terminate when disconnected")
	}
}

// The status bar advertises an active watch (and hides it when stopped),
// mirroring the TXN indicator.
func TestStatusBarWatchIndicator(t *testing.T) {
	m := NewModel(&config.Config{})
	m.watchActive = true
	m.watchInterval = 5 * time.Second
	if !strings.Contains(m.statusBar(""), "WATCH 5s") {
		t.Error("status bar should show 'WATCH 5s' while watching")
	}
	m.watchActive = false
	if strings.Contains(m.statusBar(""), "WATCH") {
		t.Error("status bar should not show WATCH when watch is stopped")
	}
}
