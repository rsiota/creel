package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestSortTableSizes(t *testing.T) {
	sizes := []db.TableSize{
		{Name: "small", Rows: 10, DiskBytes: 1000},
		{Name: "big", Rows: 100, DiskBytes: 9000},
		{Name: "medium", Rows: 50, DiskBytes: 5000},
		{Name: "unknown", Rows: 999, DiskBytes: -1},
	}
	sortTableSizes(sizes)
	if sizes[0].Name != "big" || sizes[1].Name != "medium" || sizes[2].Name != "small" {
		t.Fatalf("disk sort order = %v, want big, medium, small first", tableSizeNames(sizes))
	}
	if sizes[len(sizes)-1].Name != "unknown" {
		t.Fatalf("unknown disk should sort last, got %v", tableSizeNames(sizes))
	}
}

func TestFormatTableSizeRows(t *testing.T) {
	if got := formatTableSizeRows(db.TableSize{Rows: -1}); got != "—" {
		t.Errorf("missing rows = %q", got)
	}
	if got := formatTableSizeRows(db.TableSize{Rows: 1200, RowsApprox: true}); got != "~1,200" {
		t.Errorf("approx rows = %q, want ~1,200", got)
	}
	if got := formatTableSizeRows(db.TableSize{Rows: 42}); got != "42" {
		t.Errorf("exact rows = %q, want 42", got)
	}
}

func TestExSizes(t *testing.T) {
	conn := newSQLiteTestConn(t)
	if _, err := conn.DB().Exec(`CREATE TABLE events (id INTEGER PRIMARY KEY, note TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := conn.DB().Exec(`INSERT INTO events (note) VALUES ('a'), ('b')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	m := &Model{connection: conn}
	cmd := m.exSizes()
	if cmd == nil {
		t.Fatal("exSizes returned nil")
	}
	msg := cmd().(lookupResultMsg)
	if msg.err != nil {
		t.Fatalf("lookup failed: %v", msg.err)
	}
	if msg.title != "Table sizes" {
		t.Errorf("title = %q", msg.title)
	}
	if len(msg.result.Columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(msg.result.Columns))
	}
	if len(msg.result.Rows) == 0 {
		t.Fatal("expected at least one table row")
	}
	// Largest disk first; events is the only user table here.
	if msg.result.Rows[0][0] != "events" {
		t.Errorf("first row table = %q, want events", msg.result.Rows[0][0])
	}
	if !strings.Contains(msg.result.Rows[0][1], "2") {
		t.Errorf("events rows = %q, want 2", msg.result.Rows[0][1])
	}
}

func tableSizeNames(sizes []db.TableSize) []string {
	out := make([]string, len(sizes))
	for i, ts := range sizes {
		out[i] = ts.Name
	}
	return out
}
