package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

// Tests for the Tier 3 small wins: :limit (page size), :timing (elapsed
// display), and :peek (table summary). :peek's introspection needs a live
// SQLite connection; the rest is pure dispatch. (runPageQuery's DB access lives
// in the returned command, which the tests never run — exLimit checks instead
// whether a re-run command was produced.)

// --- :limit -------------------------------------------------------------

func TestExLimitNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("limit 50")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":limit with no connection -> %q", m.schemaMsg)
	}
}

func TestExLimitBadArg(t *testing.T) {
	for _, in := range []string{"limit abc", "limit 0", "limit -5"} {
		m := &Model{connection: &db.Connection{}, pageSize: defaultPageSize}
		m.runExCommand(in)
		if !strings.Contains(m.schemaMsg, "positive number") {
			t.Errorf(":%s -> %q, want a positive-number message", in, m.schemaMsg)
		}
		if m.pageSize != defaultPageSize {
			t.Errorf(":%s should not change page size (got %d)", in, m.pageSize)
		}
	}
}

func TestExLimitSetsAndReruns(t *testing.T) {
	m := &Model{connection: &db.Connection{}, lastQuery: "SELECT 1", pageSize: defaultPageSize}
	cmd := m.runExCommand("limit 50")
	if m.pageSize != 50 {
		t.Errorf("page size = %d, want 50", m.pageSize)
	}
	if m.page != 0 {
		t.Errorf("page = %d, want 0 (position resets under a new size)", m.page)
	}
	if cmd == nil {
		t.Error("expected a re-run command when a query is active")
	}
}

func TestExLimitNoRerunWithoutQuery(t *testing.T) {
	m := &Model{connection: &db.Connection{}, pageSize: defaultPageSize} // no lastQuery
	cmd := m.runExCommand("limit 50")
	if m.pageSize != 50 {
		t.Errorf("page size = %d, want 50", m.pageSize)
	}
	if cmd != nil {
		t.Error("should not re-run when there's no last query")
	}
}

func TestExLimitOffResetsDefault(t *testing.T) {
	for _, in := range []string{"limit off", "limit default"} {
		m := &Model{connection: &db.Connection{}, lastQuery: "SELECT 1", pageSize: 7}
		cmd := m.runExCommand(in)
		if m.pageSize != defaultPageSize {
			t.Errorf(":%s -> page size %d, want default %d", in, m.pageSize, defaultPageSize)
		}
		if cmd == nil {
			t.Errorf(":%s should re-run to apply the default", in)
		}
	}
}

func TestExLimitBareReportsCurrent(t *testing.T) {
	m := &Model{connection: &db.Connection{}, pageSize: 42}
	cmd := m.runExCommand("limit")
	if !strings.Contains(m.schemaMsg, "42") {
		t.Errorf(":limit bare should report current size: %q", m.schemaMsg)
	}
	if cmd != nil {
		t.Error(":limit bare should not re-run")
	}
}

// --- :timing ------------------------------------------------------------

func TestExTimingToggle(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("timing")
	if !m.showTiming || !strings.Contains(m.schemaMsg, "on") {
		t.Errorf(":timing toggle on -> show=%v msg=%q", m.showTiming, m.schemaMsg)
	}
	m.runExCommand("timing")
	if m.showTiming || !strings.Contains(m.schemaMsg, "off") {
		t.Errorf(":timing toggle off -> show=%v msg=%q", m.showTiming, m.schemaMsg)
	}
}

func TestExTimingExplicit(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("timing on")
	if !m.showTiming {
		t.Error(":timing on should enable")
	}
	m.runExCommand("timing off")
	if m.showTiming {
		t.Error(":timing off should disable")
	}
}

func TestExTimingBadArg(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("timing yes")
	if !strings.Contains(m.schemaMsg, "on, off") {
		t.Errorf(":timing yes -> %q", m.schemaMsg)
	}
}

func TestStatusBarTimingIndicator(t *testing.T) {
	m := NewModel(&config.Config{})
	m.showTiming = true
	m.lastQueryElapsed = 123 * time.Millisecond
	if sb := m.statusBar(""); !strings.Contains(sb, "0.123s") {
		t.Errorf("status bar should show elapsed when timing on: %q", sb)
	}
	m.showTiming = false
	if sb := m.statusBar(""); strings.Contains(sb, "0.123s") {
		t.Errorf("status bar should hide elapsed when timing off: %q", sb)
	}
	// On but no query run yet → no elapsed to show.
	m.showTiming = true
	m.lastQueryElapsed = 0
	if sb := m.statusBar(""); strings.Contains(sb, "s") && strings.Contains(sb, "0.000s") {
		t.Errorf("status bar should not show a zero elapsed: %q", sb)
	}
}

// --- :peek --------------------------------------------------------------

func TestExPeekNotConnected(t *testing.T) {
	m := &Model{}
	cmd := m.runExCommand("peek events")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":peek with no connection -> %q", m.schemaMsg)
	}
	if cmd != nil {
		t.Error(":peek with no connection should not produce a command")
	}
}

func TestExPeekNoTable(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("peek")
	if !strings.Contains(m.schemaMsg, "no current table") && !strings.Contains(m.schemaMsg, "name one") {
		t.Errorf(":peek with no table -> %q", m.schemaMsg)
	}
}

func TestExPeekSummary(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Execute("CREATE TABLE events (id INTEGER PRIMARY KEY AUTOINCREMENT, msg TEXT, level INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := conn.DB().Execute("INSERT INTO events (msg, level) VALUES ('a', 1), ('b', 2)"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	m := &Model{connection: conn, tables: []string{"events"}}
	cmd := m.runExCommand("peek events")
	if cmd == nil {
		t.Fatalf(":peek events -> no command; msg=%q", m.schemaMsg)
	}
	msg := cmd()
	lrm, ok := msg.(lookupResultMsg)
	if !ok {
		t.Fatalf("expected lookupResultMsg, got %T", msg)
	}
	if lrm.err != nil {
		t.Fatalf("peek error: %v", lrm.err)
	}
	if !strings.Contains(lrm.title, "Peek") || !strings.Contains(lrm.title, "events") {
		t.Errorf("peek title = %q", lrm.title)
	}
	val := peekValue(lrm, "rows")
	if val != "2" {
		t.Errorf("peek rows = %q, want 2", val)
	}
	if val := peekValue(lrm, "columns"); val != "3" {
		t.Errorf("peek columns = %q, want 3", val)
	}
	if val := peekValue(lrm, "primary key"); val != "id" {
		t.Errorf("peek primary key = %q, want id", val)
	}
	if val := peekValue(lrm, "column names"); val != "id, msg, level" {
		t.Errorf("peek column names = %q, want 'id, msg, level'", val)
	}
}

// peekValue finds the Value for a given Field in a :peek lookup result.
func peekValue(lrm lookupResultMsg, field string) string {
	for _, row := range lrm.result.Rows {
		if len(row) >= 2 && row[0] == field {
			return row[1]
		}
	}
	return ""
}
