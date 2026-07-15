package ui

import (
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/db"
)

// Behavioral tests for the Step 5 ":" aliases. These cover the cheap-to-test
// paths (error guards, wiring) without a live database connection.

func TestExFormat(t *testing.T) {
	m := &Model{editor: NewQueryEditor()}
	m.editor.SetValue("select   1")
	m.runExCommand("format")
	want := formatSQL("select   1")
	if m.editor.Value() != want {
		t.Errorf(":format -> %q, want %q", m.editor.Value(), want)
	}
}

func TestExThemeBadName(t *testing.T) {
	m := &Model{}
	m.runExCommand("theme nope-not-a-theme")
	if !strings.Contains(m.schemaMsg, "no such theme") {
		t.Errorf(":theme bad name -> %q", m.schemaMsg)
	}
}

func TestExThemeMissingArg(t *testing.T) {
	m := &Model{}
	m.runExCommand("theme")
	if !strings.Contains(m.schemaMsg, "needs a name") {
		t.Errorf(":theme with no arg -> %q", m.schemaMsg)
	}
}

func TestExStatsNoResults(t *testing.T) {
	m := &Model{results: NewResultsTable()}
	m.runExCommand("stats")
	if !strings.Contains(m.schemaMsg, "no results") {
		t.Errorf(":stats with no results -> %q", m.schemaMsg)
	}
}

func TestExStatsBadColumn(t *testing.T) {
	m := &Model{connection: &db.Connection{}, results: NewResultsTable()}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}}, "")
	m.runExCommand("stats nope")
	if !strings.Contains(m.schemaMsg, "no such column") {
		t.Errorf(":stats bad column -> %q", m.schemaMsg)
	}
}

func TestExDescribeNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("describe users")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":describe with no connection -> %q", m.schemaMsg)
	}
}

func TestExHistoryNoConnection(t *testing.T) {
	m := &Model{}
	m.runExCommand("history")
	// No connection → toggleHistory is a no-op; panel must stay hidden, no panic.
	if m.history.IsVisible() {
		t.Error(":history with no connection should not open the panel")
	}
}

func TestExBookmarksNoConnection(t *testing.T) {
	m := &Model{}
	m.runExCommand("bookmarks")
	if m.bookmarks.IsVisible() {
		t.Error(":bookmarks with no connection should not open the panel")
	}
}
