package ui

import (
	"fmt"
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
	cmd := m.runCrossSearchBatch("100%", gen)
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
	msg := m.runCrossSearchBatch("42", gen)().(crossSearchResultMsg)
	if len(msg.results) != 0 {
		t.Fatalf("numeric-only table should yield no hits, got %+v", msg.results)
	}
	if !msg.done {
		t.Fatal("expected done after single-table batch")
	}
}

func TestCrossSearchDrainsTablePastPageUnderCap(t *testing.T) {
	m := grepFixture(t, map[string]int{"notes": 25})
	gen := m.crossSearch.StartSearch(len(m.tables))
	msg := m.runCrossSearchBatch("hit", gen)().(crossSearchResultMsg)
	if msg.done != true || msg.capped {
		t.Fatalf("under the cap a single table should drain: done=%v capped=%v hits=%d", msg.done, msg.capped, len(msg.results))
	}
	if len(msg.results) != 25 {
		t.Fatalf("hits = %d, want all 25 (page is 20)", len(msg.results))
	}
}

func TestCrossSearchContinueResumesPastCap(t *testing.T) {
	m := grepFixture(t, map[string]int{"alpha": 30, "beta": 30})
	// Keep table order stable: alpha then beta.
	m.tables = []string{"alpha", "beta"}
	gen := m.crossSearch.StartSearch(len(m.tables))
	m.crossSearch.hitLimit = 25
	m.crossSearch.SetQuery("hit")

	msg := m.runCrossSearchBatch("hit", gen)().(crossSearchResultMsg)
	updated, cmd := m.Update(msg)
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("a full page should pause for ctrl+n, not schedule another batch")
	}
	if !m.crossSearch.capped || !m.crossSearch.CanLoadMore() {
		t.Fatalf("capped=%v canLoad=%v hits=%d", m.crossSearch.capped, m.crossSearch.CanLoadMore(), len(m.crossSearch.results))
	}
	if len(m.crossSearch.results) != 25 {
		t.Fatalf("first page = %d hits, want 25", len(m.crossSearch.results))
	}
	fromAlpha, fromBeta := countGrepTables(m.crossSearch.results)
	if fromAlpha != 20 || fromBeta != 5 {
		t.Fatalf("first page should spread 20/5 across tables, got alpha=%d beta=%d", fromAlpha, fromBeta)
	}

	gen, ok := m.crossSearch.ContinueSearch()
	if !ok {
		t.Fatal("ContinueSearch")
	}
	msg = m.runCrossSearchBatch("hit", gen)().(crossSearchResultMsg)
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if len(m.crossSearch.results) <= 25 {
		t.Fatalf("continue added no hits: %d", len(m.crossSearch.results))
	}
	seen := map[string]int{}
	for _, r := range m.crossSearch.results {
		seen[r.Table+"/"+r.Value]++
		if seen[r.Table+"/"+r.Value] > 1 {
			t.Fatalf("duplicate hit %s.%s", r.Table, r.Value)
		}
	}
	// The second page resumes beta (which was cut at 5) before going back
	// for alpha's deferred rows, and must include a row past beta's first five.
	if seen["beta/b-hit-05"] == 0 {
		t.Fatal("continue should resume beta past the five hits already shown")
	}
}

func grepFixture(t *testing.T, counts map[string]int) Model {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(dir, "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	// Sort so table order in the scan is deterministic.
	sortStrings(names)
	cache := map[string][]db.Column{}
	for _, name := range names {
		stmt := fmt.Sprintf(`CREATE TABLE %s (id INTEGER PRIMARY KEY, body TEXT)`, name)
		if _, err := conn.DB().Execute(stmt); err != nil {
			t.Fatal(err)
		}
		prefix := string(name[0])
		for i := 0; i < counts[name]; i++ {
			ins := fmt.Sprintf(`INSERT INTO %s (body) VALUES ('%s-hit-%02d')`, name, prefix, i)
			if _, err := conn.DB().Execute(ins); err != nil {
				t.Fatal(err)
			}
		}
		cache[name] = []db.Column{{Name: "id", Type: "INTEGER"}, {Name: "body", Type: "TEXT"}}
	}

	m := NewModel(&config.Config{})
	m.connection = conn
	m.tables = names
	m.columnCache = cache
	m.crossSearch.Show()
	m.crossSearch.SetQuery("hit")
	return m
}

func countGrepTables(results []SearchResult) (alpha, beta int) {
	for _, r := range results {
		switch r.Table {
		case "alpha":
			alpha++
		case "beta":
			beta++
		}
	}
	return alpha, beta
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
