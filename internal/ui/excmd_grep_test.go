package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestExGrepNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("grep")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":grep with no connection -> %q", m.schemaMsg)
	}
}

func TestExGrepNoTables(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("grep")
	if !strings.Contains(m.schemaMsg, "no tables") {
		t.Errorf(":grep with no tables -> %q", m.schemaMsg)
	}
}

func TestExGrepOpensEmptyPanel(t *testing.T) {
	m := &Model{
		connection: &db.Connection{},
		tables:     []string{"users"},
	}
	cmd := m.runExCommand("grep")
	if cmd != nil {
		t.Fatal(":grep with no query should not start a search cmd")
	}
	if !m.crossSearch.IsVisible() {
		t.Fatal(":grep should open the cross-search panel")
	}
	if got := m.crossSearch.Query(); got != "" {
		t.Fatalf("query = %q, want empty", got)
	}
}

func TestExGrepPrefillsAndStarts(t *testing.T) {
	m := &Model{
		connection: &db.Connection{},
		tables:     []string{"users"},
	}
	cmd := m.runExCommand("grep hello world")
	if !m.crossSearch.IsVisible() {
		t.Fatal(":grep should open the cross-search panel")
	}
	if got := m.crossSearch.Query(); got != "hello world" {
		t.Fatalf("query = %q, want %q", got, "hello world")
	}
	if cmd == nil {
		t.Fatal(":grep with a query should return startCrossSearch cmd")
	}
}
