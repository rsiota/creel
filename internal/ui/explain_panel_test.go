package ui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

func TestExplainPanelSQLiteTree(t *testing.T) {
	result := db.Result{
		Columns: []db.Column{
			{Name: "id", Type: "INT"},
			{Name: "parent", Type: "INT"},
			{Name: "notused", Type: "INT"},
			{Name: "detail", Type: "TEXT"},
		},
		Rows: [][]string{
			{"2", "0", "0", "SCAN users"},
			{"3", "2", "0", "SEARCH orders USING INDEX idx_uid (user_id=?)"},
		},
	}

	p := ExplainPanel{}
	p.Show(result, db.DriverSQLite)
	lines := p.renderedLines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// First line (depth 0, no indent).
	if lines[0] != "SCAN users" {
		t.Errorf("line 0 = %q, want %q", lines[0], "SCAN users")
	}
	// Second line (depth 1, 2-space indent).
	if lines[1] != "  SEARCH orders USING INDEX idx_uid (user_id=?)" {
		t.Errorf("line 1 = %q, want indented", lines[1])
	}
}

func TestExplainPanelPostgresVerbatim(t *testing.T) {
	result := db.Result{
		Columns: []db.Column{{Name: "QUERY PLAN", Type: "TEXT"}},
		Rows: [][]string{
			{"Seq Scan on users  (cost=0.00..12.00 rows=100 width=4)"},
			{"  ->  Index Scan using idx_email on users"},
		},
	}

	p := ExplainPanel{}
	p.Show(result, db.DriverPostgres)
	lines := p.renderedLines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "Seq Scan on users  (cost=0.00..12.00 rows=100 width=4)" {
		t.Errorf("line 0 = %q", lines[0])
	}
}

func TestExplainPanelMySQLTable(t *testing.T) {
	result := db.Result{
		Columns: []db.Column{
			{Name: "id"},
			{Name: "select_type"},
			{Name: "table"},
			{Name: "type"},
			{Name: "possible_keys"},
			{Name: "key"},
			{Name: "rows"},
			{Name: "Extra"},
		},
		Rows: [][]string{
			{"1", "SIMPLE", "users", "ALL", "", "", "100", ""},
			{"1", "SIMPLE", "orders", "ref", "idx_uid", "idx_uid", "10", "Using index"},
		},
	}

	p := ExplainPanel{}
	p.Show(result, db.DriverMySQL)
	lines := p.renderedLines()
	// Header + separator + 2 data rows.
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (header+sep+2 rows), got %d: %v", len(lines), lines)
	}
	// Header should contain column names.
	if !strings.Contains(lines[0], "ID") || !strings.Contains(lines[0], "TABLE") {
		t.Errorf("header line = %q, expected column names", lines[0])
	}
	// Data rows should contain table names.
	if !strings.Contains(lines[2], "users") {
		t.Errorf("row 1 = %q, expected 'users'", lines[2])
	}
	if !strings.Contains(lines[3], "orders") {
		t.Errorf("row 2 = %q, expected 'orders'", lines[3])
	}
}

func TestExplainPanelEmptyResult(t *testing.T) {
	result := db.Result{
		Columns: []db.Column{{Name: "detail"}},
		Rows:    nil,
	}

	p := ExplainPanel{}
	p.Show(result, db.DriverSQLite)
	lines := p.renderedLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "no plan") {
		t.Errorf("expected 'no plan' message, got %v", lines)
	}
}

func TestExplainPanelVisibility(t *testing.T) {
	p := ExplainPanel{}
	if p.IsVisible() {
		t.Fatal("should not be visible initially")
	}
	p.Show(db.Result{Rows: [][]string{{"x"}}}, db.DriverSQLite)
	if !p.IsVisible() {
		t.Fatal("should be visible after Show")
	}
	p.Hide()
	if p.IsVisible() {
		t.Fatal("should not be visible after Hide")
	}
}

func TestExplainPanelScroll(t *testing.T) {
	// Build a plan with many lines.
	rows := make([][]string, 50)
	for i := range rows {
		rows[i] = []string{"0", "0", "0", "line " + strconv.Itoa(i)}
	}
	result := db.Result{
		Columns: []db.Column{
			{Name: "id"}, {Name: "parent"}, {Name: "notused"}, {Name: "detail"},
		},
		Rows: rows,
	}

	p := ExplainPanel{}
	p.SetSize(60, 10)
	p.Show(result, db.DriverSQLite)

	// Cursor starts at 0.
	if p.cursor != 0 || p.scroll != 0 {
		t.Fatalf("cursor=%d scroll=%d, want 0/0", p.cursor, p.scroll)
	}

	// Move down 15 lines.
	for i := 0; i < 15; i++ {
		p = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	if p.cursor != 15 {
		t.Fatalf("cursor=%d, want 15", p.cursor)
	}
	// Scroll should have advanced to keep cursor visible.
	if p.scroll > 15 || p.scroll+p.height <= 15 {
		t.Fatalf("scroll=%d with height=%d, cursor should be visible", p.scroll, p.height)
	}

	// Jump to bottom with G.
	p = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if p.cursor != 49 {
		t.Fatalf("cursor=%d, want 49", p.cursor)
	}

	// Jump to top with g.
	p = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if p.cursor != 0 || p.scroll != 0 {
		t.Fatalf("cursor=%d scroll=%d, want 0/0", p.cursor, p.scroll)
	}
}
