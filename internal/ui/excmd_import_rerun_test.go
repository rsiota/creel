package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/ruben/gsql/internal/db"
	"github.com/ruben/gsql/internal/history"
)

// Behavioral tests for :import and :rerun, plus the history-panel numbering
// that makes :rerun <n> discoverable. Guards and wiring are tested without a
// live database (executeQuery only touches the DB inside the returned cmd,
// which the tests never run).

func TestExImportNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("import ~/x.sql")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":import with no connection -> %q", m.schemaMsg)
	}
}

func TestExImportMissingArg(t *testing.T) {
	m := &Model{connection: &db.Connection{}}
	m.runExCommand("import")
	if !strings.Contains(m.schemaMsg, "needs a file path") {
		t.Errorf(":import with no arg -> %q", m.schemaMsg)
	}
}

func TestExRerunNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("rerun 1")
	if !strings.Contains(m.schemaMsg, "no history") {
		t.Errorf(":rerun with no connection -> %q", m.schemaMsg)
	}
}

func TestExRerunBadNumber(t *testing.T) {
	m := &Model{
		connection:   &db.Connection{},
		historyStore: history.NewStore(t.TempDir()),
	}
	for _, in := range []string{"rerun abc", "rerun 0", "rerun -1", "rerun"} {
		m.schemaMsg = ""
		m.runExCommand(in)
		if !strings.Contains(m.schemaMsg, "positive number") && !strings.Contains(m.schemaMsg, "needs a number") {
			t.Errorf(":%s -> %q, want a positive-number message", in, m.schemaMsg)
		}
	}
}

func TestExRerunNoHistory(t *testing.T) {
	m := &Model{
		connection:   &db.Connection{},
		historyStore: history.NewStore(t.TempDir()),
	}
	m.runExCommand("rerun 1")
	if !strings.Contains(m.schemaMsg, "no history yet") {
		t.Errorf(":rerun 1 with empty history -> %q", m.schemaMsg)
	}
}

func TestExRerunOutOfBounds(t *testing.T) {
	m := &Model{
		editor:       NewQueryEditor(),
		connection:   &db.Connection{},
		historyStore: history.NewStore(t.TempDir()),
	}
	m.historyStore.Record("", "SELECT 1;", true)
	m.historyStore.Record("", "SELECT 2;", true)
	m.runExCommand("rerun 5")
	if !strings.Contains(m.schemaMsg, "only 2 entries") {
		t.Errorf(":rerun 5 with 2 entries -> %q", m.schemaMsg)
	}
}

func TestExRerunLoadsByRank(t *testing.T) {
	m := &Model{
		editor:       NewQueryEditor(),
		connection:   &db.Connection{},
		historyStore: history.NewStore(t.TempDir()),
	}
	m.historyStore.Record("", "SELECT 1;", true) // oldest
	m.historyStore.Record("", "SELECT 2;", true)
	m.historyStore.Record("", "SELECT 3;", true) // most recent

	// n=1 -> most recent; n=3 -> oldest. executeQuery's DB access is in the
	// returned cmd, which we never run, so the editor holds the loaded query.
	cases := []struct{ n, want string }{
		{"1", "SELECT 3;"},
		{"2", "SELECT 2;"},
		{"3", "SELECT 1;"},
	}
	for _, c := range cases {
		m.runExCommand("rerun " + c.n)
		if got := m.editor.Value(); got != c.want {
			t.Errorf(":rerun %s -> editor %q, want %q", c.n, got, c.want)
		}
	}
}

// History panel rows are numbered 1..N most-recent-first, and each entry keeps
// its number when the list is fuzzy-filtered — so :rerun <n> always refers to
// the number shown next to the entry.
func TestHistoryPanelNumberingStableThroughFilter(t *testing.T) {
	h := NewHistoryPanel()
	h.SetEntries([]history.Entry{
		{Query: "alpha", RunAt: time.Now()}, // oldest
		{Query: "beta", RunAt: time.Now()},
		{Query: "gamma", RunAt: time.Now()}, // newest
	})

	// Unfiltered: most-recent-first → origIdx 0,1,2 == gamma,beta,alpha.
	got := h.filteredEntries()
	wantQ := []string{"gamma", "beta", "alpha"}
	if len(got) != len(wantQ) {
		t.Fatalf("unfiltered: got %d entries, want %d", len(got), len(wantQ))
	}
	for i, fe := range got {
		if fe.origIdx != i {
			t.Errorf("unfiltered[%d].origIdx = %d, want %d", i, fe.origIdx, i)
		}
		if fe.entry.Query != wantQ[i] {
			t.Errorf("unfiltered[%d].query = %q, want %q", i, fe.entry.Query, wantQ[i])
		}
	}

	// Filtering reorders by match quality, but each entry must retain its
	// permanent number (origIdx). "a" matches alpha (origIdx 2) and gamma (0).
	h.filter = "a"
	got = h.filteredEntries()
	byIdx := map[int]string{}
	for _, fe := range got {
		byIdx[fe.origIdx] = fe.entry.Query
	}
	if byIdx[0] != "gamma" {
		t.Errorf("filtered origIdx 0 = %q, want gamma", byIdx[0])
	}
	if byIdx[2] != "alpha" {
		t.Errorf("filtered origIdx 2 = %q, want alpha", byIdx[2])
	}
}

func TestHistoryPanelNumberedRenderDoesNotPanic(t *testing.T) {
	h := NewHistoryPanel()
	h.SetSize(80, 24)
	h.SetEntries([]history.Entry{
		{Query: "SELECT 1;", RunAt: time.Now()},
		{Query: "SELECT 2;", RunAt: time.Now()},
	})
	// Toggle uses a pointer receiver indirectly via the value; flip visible.
	h.visible = true
	view := h.View()
	if !strings.Contains(view, "SELECT") {
		t.Errorf("numbered render produced no query text: %q", view)
	}
}
