package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestRecordAndClearQueryFailure(t *testing.T) {
	m := &Model{}
	m.recordQueryFailure("SELECT * FORM users;", fmt.Errorf(`near "FORM": syntax error`))
	if m.lastQueryFailSQL != "SELECT * FORM users" {
		t.Errorf("sql = %q", m.lastQueryFailSQL)
	}
	if !strings.Contains(m.lastQueryFailErr, "FORM") {
		t.Errorf("err = %q", m.lastQueryFailErr)
	}
	m.clearQueryFailure()
	if m.lastQueryFailSQL != "" || m.lastQueryFailErr != "" {
		t.Fatal("expected cleared")
	}
}

func TestQueryExecutedMsgRecordsFailure(t *testing.T) {
	m := Model{
		results: NewResultsTable(),
		editor:  NewQueryEditor(),
		state:   stateWorkspace,
	}
	updated, _ := m.Update(queryExecutedMsg{
		query: "SELECT * FORM users",
		err:   fmt.Errorf(`near "FORM": syntax error`),
	})
	mm := updated.(Model)
	if mm.lastQueryFailSQL != "SELECT * FORM users" {
		t.Errorf("sql = %q", mm.lastQueryFailSQL)
	}
	if mm.lastQueryFailErr == "" {
		t.Fatal("expected error remembered")
	}
}

func TestQueryExecutedMsgClearsFailureOnSuccess(t *testing.T) {
	m := Model{
		results:          NewResultsTable(),
		editor:           NewQueryEditor(),
		state:            stateWorkspace,
		lastQueryFailSQL: "SELECT * FORM users",
		lastQueryFailErr: "syntax error",
	}
	updated, _ := m.Update(queryExecutedMsg{
		query: "SELECT 1",
		result: db.Result{
			Columns: []db.Column{{Name: "n"}},
			Rows:    [][]string{{"1"}},
			Message: "1 row",
		},
		page:     0,
		pageSize: 50,
	})
	mm := updated.(Model)
	if mm.lastQueryFailSQL != "" || mm.lastQueryFailErr != "" {
		t.Fatalf("success should clear failure: sql=%q err=%q", mm.lastQueryFailSQL, mm.lastQueryFailErr)
	}
}

func TestExAIFixGuards(t *testing.T) {
	t.Run("no connection", func(t *testing.T) {
		m := &Model{}
		m.runExCommand("aifix")
		if !strings.Contains(m.aiMsg, "open connection") {
			t.Errorf("aiMsg = %q", m.aiMsg)
		}
	})
	t.Run("no failure", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn}
		m.runExCommand("aifix")
		if !strings.Contains(m.aiMsg, "no failed query") {
			t.Errorf("aiMsg = %q", m.aiMsg)
		}
	})
	t.Run("fixsql alias", func(t *testing.T) {
		if exLookup("fixsql") == nil {
			t.Fatal("fixsql alias missing")
		}
	})
}

func TestExAIFixDispatchesWithoutCallingProvider(t *testing.T) {
	// With a remembered failure but no API key, dispatchAI should refuse
	// before any network call and leave a clear status message.
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{
		connection:       conn,
		editor:           NewQueryEditor(),
		results:          NewResultsTable(),
		config:           &config.Config{},
		lastQueryFailSQL: "SELECT * FORM users",
		lastQueryFailErr: `near "FORM": syntax error`,
	}
	cmd := m.runExCommand("aifix")
	if cmd != nil {
		t.Fatal("expected nil cmd when no AI key is configured")
	}
	if !strings.Contains(m.aiMsg, "AI") && !strings.Contains(m.aiMsg, "provider") && !strings.Contains(m.aiMsg, "API") && !strings.Contains(strings.ToLower(m.aiMsg), "key") {
		t.Errorf("aiMsg = %q, want missing-key hint", m.aiMsg)
	}
}

func TestAIFixResultMessage(t *testing.T) {
	m := Model{
		editor:     NewQueryEditor(),
		results:    NewResultsTable(),
		state:      stateWorkspace,
		aiQuestion: aiFixQuestion,
	}
	updated, _ := m.Update(aiResultMsg{sql: "SELECT * FROM users", toPanel: false})
	mm := updated.(Model)
	if mm.editor.Value() != "SELECT * FROM users" {
		t.Errorf("editor = %q", mm.editor.Value())
	}
	if !strings.Contains(mm.aiMsg, "AI fixed query") {
		t.Errorf("aiMsg = %q", mm.aiMsg)
	}
}
