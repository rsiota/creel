package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/ai"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestFormatExplainPlanStripsANSI(t *testing.T) {
	result := db.Result{
		Columns: []db.Column{{Name: "id"}, {Name: "parent"}, {Name: "notused"}, {Name: "detail"}},
		Rows: [][]string{
			{"0", "0", "0", "SCAN users"},
			{"1", "0", "0", "USE TEMP B-TREE FOR ORDER BY"},
		},
	}
	got := formatExplainPlan(result, db.DriverSQLite)
	if strings.Contains(got, "\x1b[") {
		t.Errorf("plan still has ANSI: %q", got)
	}
	if !strings.Contains(got, "SCAN users") {
		t.Errorf("missing plan detail:\n%s", got)
	}
}

func TestExAIExplainGuards(t *testing.T) {
	t.Run("no connection", func(t *testing.T) {
		m := &Model{}
		m.runExCommand("aiexplain")
		if !strings.Contains(m.aiMsg, "open connection") {
			t.Errorf("aiMsg = %q", m.aiMsg)
		}
	})
	t.Run("no query", func(t *testing.T) {
		conn := newSQLiteTestConn(t)
		defer conn.Close()
		m := &Model{connection: conn, editor: NewQueryEditor()}
		m.runExCommand("aiexplain")
		if !strings.Contains(m.aiMsg, "no query") {
			t.Errorf("aiMsg = %q", m.aiMsg)
		}
	})
	t.Run("why alias", func(t *testing.T) {
		if exLookup("why") == nil {
			t.Fatal("why alias missing")
		}
		if exLookup("aiexplain") == nil {
			t.Fatal("aiexplain missing")
		}
	})
}

func TestExAIExplainUsesCachedPlan(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := &Model{
		connection:      conn,
		editor:          NewQueryEditor(),
		assistant:       NewAssistant(),
		config:          &config.Config{},
		lastExplainSQL:  "SELECT 1",
		lastExplainText: "SCAN CONSTANT ROW",
	}
	m.editor.SetValue("SELECT 1;")

	cmd := m.exAIExplain("")
	if !m.assistant.IsVisible() {
		t.Error("assistant should open for explain")
	}
	if cmd == nil {
		if !strings.Contains(m.aiMsg, "AI") && !strings.Contains(m.aiMsg, "provider") && !strings.Contains(strings.ToLower(m.aiMsg), "key") {
			t.Errorf("aiMsg = %q, want missing-key hint", m.aiMsg)
		}
		return
	}
	if m.aiCancel != nil {
		m.aiCancel()
	}
}

func TestExplainPrompts(t *testing.T) {
	sys := ai.ExplainSystemPrompt("CREATE TABLE users (id INT);")
	for _, want := range []string{"EXPLAIN", "bottleneck", "CREATE TABLE users", "Do not reply with only SQL"} {
		if !strings.Contains(sys, want) {
			t.Errorf("ExplainSystemPrompt missing %q\n%s", want, sys)
		}
	}
	user := ai.ExplainUserPrompt("SELECT * FROM users;", "SCAN users", "why no index")
	for _, want := range []string{"SELECT * FROM users;", "SCAN users", "why no index"} {
		if !strings.Contains(user, want) {
			t.Errorf("ExplainUserPrompt missing %q\n%s", want, user)
		}
	}
	user2 := ai.ExplainUserPrompt("SELECT 1;", "", "")
	if !strings.Contains(user2, "No EXPLAIN plan") {
		t.Errorf("empty plan should note unavailability:\n%s", user2)
	}
}

func TestExplainResultMsgCachesPlan(t *testing.T) {
	m := Model{
		results: NewResultsTable(),
		editor:  NewQueryEditor(),
		state:   stateWorkspace,
	}
	updated, _ := m.Update(explainResultMsg{
		query: "SELECT id FROM users",
		result: db.Result{
			Columns: []db.Column{{Name: "detail"}},
			Rows:    [][]string{{"SCAN users"}},
		},
	})
	mm := updated.(Model)
	if mm.lastExplainSQL != "SELECT id FROM users" {
		t.Errorf("sql = %q", mm.lastExplainSQL)
	}
	if !strings.Contains(mm.lastExplainText, "SCAN users") {
		t.Errorf("plan = %q", mm.lastExplainText)
	}
	if !mm.explainPanel.IsVisible() {
		t.Error("overlay should show for a normal EXPLAIN")
	}
}

func TestExplainResultMsgForAIOpensAssistant(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := Model{
		connection: conn,
		results:    NewResultsTable(),
		editor:     NewQueryEditor(),
		assistant:  NewAssistant(),
		state:      stateWorkspace,
		config:     &config.Config{},
	}
	updated, cmd := m.Update(explainResultMsg{
		query: "SELECT 1",
		forAI: true,
		result: db.Result{
			Columns: []db.Column{{Name: "detail"}},
			Rows:    [][]string{{"SCAN"}},
		},
	})
	mm := updated.(Model)
	if mm.lastExplainText == "" {
		t.Fatal("expected cached plan")
	}
	if mm.explainPanel.IsVisible() {
		t.Error("forAI path should not open the EXPLAIN overlay")
	}
	if !mm.assistant.IsVisible() {
		t.Error("assistant should open for explain")
	}
	if cmd == nil {
		if !strings.Contains(mm.aiMsg, "AI") && !strings.Contains(mm.aiMsg, "provider") && !strings.Contains(strings.ToLower(mm.aiMsg), "key") {
			t.Errorf("aiMsg = %q, want missing-key hint", mm.aiMsg)
		}
		return
	}
	if mm.aiCancel != nil {
		mm.aiCancel()
	}
}

func TestAIExplainResultKeepsProse(t *testing.T) {
	m := Model{
		editor:     NewQueryEditor(),
		results:    NewResultsTable(),
		assistant:  NewAssistant(),
		state:      stateWorkspace,
		aiQuestion: aiExplainQuestion,
	}
	m.assistant.Show()
	m.assistant.SetPending(true)
	updated, _ := m.Update(aiResultMsg{
		reply:   "This query scans users without an index.",
		sql:     "SELECT * FROM users;", // spurious extract — must be ignored
		toPanel: true,
	})
	mm := updated.(Model)
	if sql := mm.assistant.LatestSQL(); sql != "" {
		t.Errorf("explain reply should not expose Apply SQL, got %q", sql)
	}
	if !mm.assistant.HasTurns() {
		t.Fatal("expected assistant transcript turn")
	}
	mm.assistant.SetSize(40, 20)
	view := strings.Join(mm.assistant.renderTranscriptLines(), "\n")
	if !strings.Contains(view, "scans users") {
		t.Errorf("transcript should show prose, got:\n%s", view)
	}
	if strings.Contains(view, "(no SQL returned)") {
		t.Error("prose reply must not fall back to (no SQL returned)")
	}
	if strings.Contains(view, "SELECT * FROM users") {
		t.Error("spurious ExtractSQL must not replace the explanation")
	}
}

func TestAssistantRendersProseWhenSQLEmpty(t *testing.T) {
	a := NewAssistant()
	a.Show()
	a.AppendAssistant("This plan does a full table scan.", "")
	a.SetSize(40, 20)
	view := strings.Join(a.renderTranscriptLines(), "\n")
	if !strings.Contains(view, "full table scan") {
		t.Fatalf("expected prose body, got:\n%s", view)
	}
	if strings.Contains(view, "(no SQL returned)") {
		t.Fatal("empty sql with text must show text")
	}
}
