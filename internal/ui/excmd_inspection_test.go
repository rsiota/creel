package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ruben/creel/internal/bookmarks"
	"github.com/ruben/creel/internal/db"
)

// Behavioral tests for the data-inspection verbs (:count, :sample/:head) and
// :bookmark. Covers the cheap-to-test paths (error guards, SQL generation,
// wiring) without a live database connection — the same convention as
// excmd_aliases_test.go. resolveTableArg + the :goto-style editor/run pattern
// are exercised without ever issuing a query (executeQuery only touches the
// DB inside the returned tea.Cmd, which the test never runs).

func TestExCountNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("count")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":count with no connection -> %q", m.schemaMsg)
	}
}

func TestExCountNoCurrentTable(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("count")
	if !strings.Contains(m.schemaMsg, "no current table") {
		t.Errorf(":count with no current table -> %q", m.schemaMsg)
	}
}

func TestExCountNoSuchTable(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("count nope")
	if !strings.Contains(m.schemaMsg, "no such table") {
		t.Errorf(":count nope -> %q", m.schemaMsg)
	}
}

func TestExCountGeneratesSQL(t *testing.T) {
	m := &Model{
		editor:     NewQueryEditor(),
		connection: &db.Connection{},
		tables:     []string{"users"},
	}
	m.runExCommand("count users")
	if got := m.editor.Value(); got != "SELECT count(*) FROM users;" {
		t.Errorf(":count users -> editor %q, want SELECT count(*) FROM users;", got)
	}
}

func TestExSampleHeadAliases(t *testing.T) {
	// Both verbs resolve to the same spec and must produce identical SQL.
	want := fmt.Sprintf("SELECT * FROM orders LIMIT %d;", defaultSampleSize)
	for _, verb := range []string{"sample", "head"} {
		m := &Model{
			editor:     NewQueryEditor(),
			connection: &db.Connection{},
			tables:     []string{"orders"},
		}
		m.runExCommand(verb + " orders")
		if got := m.editor.Value(); got != want {
			t.Errorf(":%s orders -> editor %q, want %q", verb, got, want)
		}
	}
}

func TestExSampleNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("sample")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":sample with no connection -> %q", m.schemaMsg)
	}
}

func TestExSampleDefaultsToCurrentTable(t *testing.T) {
	// With no argument, :sample targets the results' source table
	// (the default-to-current-object convention, ROADMAP #15).
	m := &Model{
		editor:     NewQueryEditor(),
		connection: &db.Connection{},
		results:    NewResultsTable(),
	}
	m.results.SetEditable("users", []string{"id"})
	m.runExCommand("sample")
	want := fmt.Sprintf("SELECT * FROM users LIMIT %d;", defaultSampleSize)
	if got := m.editor.Value(); got != want {
		t.Errorf(":sample (current table) -> editor %q, want %q", got, want)
	}
}

func TestExBookmarkWiring(t *testing.T) {
	m := &Model{
		editor:        NewQueryEditor(),
		connection:    &db.Connection{},
		bookmarkStore: bookmarks.NewStore(t.TempDir()),
	}
	m.editor.SetValue("SELECT 1;")
	m.runExCommand("bookmark")
	if m.bookmarkMsg != "bookmarked" {
		t.Errorf(":bookmark -> %q, want bookmarked", m.bookmarkMsg)
	}
	got, err := m.bookmarkStore.Get(m.connection.Config().Name)
	if err != nil {
		t.Fatalf("bookmarkStore.Get: %v", err)
	}
	if len(got) != 1 || !strings.Contains(got[0].Query, "SELECT 1") {
		t.Errorf(":bookmark stored %+v, want one entry containing SELECT 1", got)
	}

	// Bookmarking the same query again reports "already bookmarked" and does
	// not create a duplicate.
	m.runExCommand("bookmark")
	if m.bookmarkMsg != "already bookmarked" {
		t.Errorf("second :bookmark -> %q, want already bookmarked", m.bookmarkMsg)
	}
	if got, _ := m.bookmarkStore.Get(m.connection.Config().Name); len(got) != 1 {
		t.Errorf("duplicate :bookmark stored %d entries, want 1", len(got))
	}
}

func TestExBookmarkNoQuery(t *testing.T) {
	m := &Model{
		editor:        NewQueryEditor(),
		connection:    &db.Connection{},
		bookmarkStore: bookmarks.NewStore(t.TempDir()),
	}
	m.runExCommand("bookmark")
	if m.bookmarkMsg == "bookmarked" {
		t.Error(":bookmark with an empty editor should not bookmark")
	}
}
