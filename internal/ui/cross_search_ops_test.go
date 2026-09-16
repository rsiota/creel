package ui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"100%", "100!%"},
		{"a_b", "a!_b"},
		{"a!b", "a!!b"},
		{"o'reilly", "o''reilly"},
		{"%_!", "!%!_!!"},
	}
	for _, tt := range tests {
		if got := escapeLikePattern(tt.in); got != tt.want {
			t.Errorf("escapeLikePattern(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsCrossSearchableType(t *testing.T) {
	yes := []string{"TEXT", "varchar(255)", "JSON", "jsonb", "UUID", "character varying", "", "ENUM"}
	no := []string{"INTEGER", "INT", "BIGINT", "REAL", "FLOAT", "BLOB", "BYTEA", "DATE", "TIMESTAMP", "BOOLEAN"}
	for _, typ := range yes {
		if !isCrossSearchableType(typ) {
			t.Errorf("isCrossSearchableType(%q) = false, want true", typ)
		}
	}
	for _, typ := range no {
		if isCrossSearchableType(typ) {
			t.Errorf("isCrossSearchableType(%q) = true, want false", typ)
		}
	}
}

func TestCrossSearchHideInvalidatesInFlight(t *testing.T) {
	m := newWorkspaceModel(t)
	m.crossSearch.Show()
	m.crossSearch.SetQuery("alice")
	gen := m.crossSearch.StartSearch(3)
	if gen == 0 {
		t.Fatal("expected non-zero generation")
	}

	m.crossSearch.Hide()
	updated, cmd := m.Update(crossSearchResultMsg{
		gen:        gen,
		results:    []SearchResult{{Table: "users", Column: "name", Value: "alice"}},
		tablesDone: 1,
		batchEnd:   1,
		done:       false,
	})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("stale result must not schedule next batch")
	}
	if len(m.crossSearch.results) != 0 {
		t.Fatalf("stale results appended: %+v", m.crossSearch.results)
	}
	if m.crossSearch.IsSearching() {
		t.Fatal("should not be searching after Hide")
	}
}

func TestCrossSearchStaleGenDoesNotContinue(t *testing.T) {
	m := newWorkspaceModel(t)
	m.crossSearch.Show()
	m.crossSearch.SetQuery("x")
	gen1 := m.crossSearch.StartSearch(10)
	_ = m.crossSearch.StartSearch(10)

	updated, cmd := m.Update(crossSearchResultMsg{
		gen:        gen1,
		results:    []SearchResult{{Table: "t", Column: "c", Value: "x"}},
		tablesDone: 3,
		batchEnd:   3,
		done:       false,
	})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("old generation must not continue")
	}
	if len(m.crossSearch.results) != 0 {
		t.Fatal("old generation must not append results")
	}
}

func TestCrossSearchLikeWildcardsDoNotMatchEverything(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(dir, "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	dbase := conn.DB()
	if _, err := dbase.Execute(`CREATE TABLE grep_like_test (id INTEGER PRIMARY KEY, name TEXT, n INT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbase.Execute(`INSERT INTO grep_like_test (name, n) VALUES ('100%_off', 1), ('plain', 2)`); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = conn
	m.tables = []string{"grep_like_test"}
	m.columnCache = map[string][]db.Column{
		"grep_like_test": {
			{Name: "id", Type: "INTEGER"},
			{Name: "name", Type: "TEXT"},
			{Name: "n", Type: "INT"},
		},
	}
	m.crossSearch.Show()
	gen := m.crossSearch.StartSearch(1)
	cmd := m.runCrossSearchBatch("100%", 0, gen)
	msg := cmd().(crossSearchResultMsg)
	if msg.skipped != 0 {
		t.Fatalf("unexpected skip: %+v", msg)
	}
	if len(msg.results) != 1 || msg.results[0].Value != "100%_off" {
		t.Fatalf("LIKE wildcards not escaped: %+v", msg.results)
	}
	if msg.results[0].Column != "name" {
		t.Fatalf("should match text column only, got %q", msg.results[0].Column)
	}
}

func TestCrossSearchSkipsNonTextColumns(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(dir, "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.DB().Execute(`CREATE TABLE nums (id INTEGER PRIMARY KEY, n INT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.DB().Execute(`INSERT INTO nums (n) VALUES (42)`); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.Config{})
	m.connection = conn
	m.tables = []string{"nums"}
	m.columnCache = map[string][]db.Column{
		"nums": {{Name: "id", Type: "INTEGER"}, {Name: "n", Type: "INT"}},
	}
	gen := m.crossSearch.StartSearch(1)
	msg := m.runCrossSearchBatch("42", 0, gen)().(crossSearchResultMsg)
	if len(msg.results) != 0 {
		t.Fatalf("numeric-only table should yield no hits, got %+v", msg.results)
	}
	if !msg.done {
		t.Fatal("expected done after single-table batch")
	}
}
