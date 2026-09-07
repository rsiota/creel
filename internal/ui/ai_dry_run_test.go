package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestDeliverGeneratedSQLDefaultFillsEditor(t *testing.T) {
	m := NewModel(&config.Config{})
	m.state = stateWorkspace
	before := len(m.resultsTabs)
	cmd := m.deliverGeneratedSQL("SELECT 1", "ten users")
	if cmd != nil {
		t.Fatal("default path should not auto-run")
	}
	if len(m.resultsTabs) != before {
		t.Fatalf("tabs changed: %d → %d", before, len(m.resultsTabs))
	}
	if m.editor.Value() != "SELECT 1" {
		t.Errorf("editor = %q", m.editor.Value())
	}
	if !strings.Contains(m.aiMsg, "review then ctrl+e") {
		t.Errorf("aiMsg = %q", m.aiMsg)
	}
}

func TestDeliverGeneratedSQLDryRunOpensTabAndRunsSelect(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	m := NewModel(&config.Config{Settings: config.Settings{AIDryRun: true}})
	m.state = stateWorkspace
	m.connection = conn
	before := len(m.resultsTabs)

	cmd := m.deliverGeneratedSQL("SELECT 1 AS n", "count rows")
	if len(m.resultsTabs) != before+1 {
		t.Fatalf("tabs = %d, want %d", len(m.resultsTabs), before+1)
	}
	if tab := m.activeTab(); tab == nil || tab.Title != "AI scratch" {
		t.Fatalf("active tab = %+v, want AI scratch", tab)
	}
	if m.editor.Value() != "SELECT 1 AS n" {
		t.Errorf("editor = %q", m.editor.Value())
	}
	if cmd == nil {
		t.Fatal("SELECT should return an execute command")
	}
	if !strings.Contains(m.aiMsg, "running") {
		t.Errorf("aiMsg = %q", m.aiMsg)
	}
}

func TestDeliverGeneratedSQLDryRunSkipsWriteAutoRun(t *testing.T) {
	m := NewModel(&config.Config{Settings: config.Settings{AIDryRun: true}})
	m.state = stateWorkspace
	before := len(m.resultsTabs)

	cmd := m.deliverGeneratedSQL("DELETE FROM users", "apply")
	if cmd != nil {
		t.Fatal("write must not auto-run")
	}
	if len(m.resultsTabs) != before+1 {
		t.Fatalf("tabs = %d, want %d", len(m.resultsTabs), before+1)
	}
	if m.editor.Value() != "DELETE FROM users" {
		t.Errorf("editor = %q", m.editor.Value())
	}
	if tab := m.activeTab(); tab == nil || tab.LastQuery != "" {
		t.Errorf("write scratch LastQuery should be empty (dirty); got %+v", tab)
	}
	if !strings.Contains(m.aiMsg, "not auto-run") {
		t.Errorf("aiMsg = %q", m.aiMsg)
	}
}

func TestDeliverGeneratedSQLDryRunFixTitle(t *testing.T) {
	m := NewModel(&config.Config{Settings: config.Settings{AIDryRun: true}})
	m.state = stateWorkspace
	_ = m.deliverGeneratedSQL("SELECT 1", "fix")
	if tab := m.activeTab(); tab == nil || tab.Title != "AI fix" {
		t.Fatalf("title = %q, want AI fix", tab.Title)
	}
}

func TestAIResultMsgRespectsDryRun(t *testing.T) {
	m := NewModel(&config.Config{Settings: config.Settings{AIDryRun: true}})
	m.state = stateWorkspace
	m.aiQuestion = "list users"
	before := len(m.resultsTabs)
	updated, _ := m.Update(aiResultMsg{sql: "SELECT id FROM users", toPanel: false})
	mm := updated.(Model)
	if len(mm.resultsTabs) != before+1 {
		t.Fatalf("tabs = %d, want %d", len(mm.resultsTabs), before+1)
	}
}

func TestApplyAssistantSQLMsgRespectsDryRun(t *testing.T) {
	m := NewModel(&config.Config{Settings: config.Settings{AIDryRun: true}})
	m.state = stateWorkspace
	m.assistant.AppendAssistant("here", "SELECT 42")
	before := len(m.resultsTabs)
	updated, _ := m.Update(applyAssistantSQLMsg{})
	mm := updated.(Model)
	if len(mm.resultsTabs) != before+1 {
		t.Fatalf("tabs = %d, want %d", len(mm.resultsTabs), before+1)
	}
	if mm.editor.Value() != "SELECT 42" {
		t.Errorf("editor = %q", mm.editor.Value())
	}
}

func TestExSetAIDryRun(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.Config{}
	m := NewModel(cfg)
	m.runExCommand("set ai_dry_run on")
	if !m.settings.AIDryRun || !cfg.Settings.AIDryRun {
		t.Fatal("ai_dry_run should be on")
	}
	m.runExCommand("set dry_run off")
	if m.settings.AIDryRun || cfg.Settings.AIDryRun {
		t.Fatal("alias should turn ai_dry_run off")
	}
}

func TestIsWriteQueryExported(t *testing.T) {
	if db.IsWriteQuery("SELECT 1") {
		t.Fatal("SELECT should not be a write")
	}
	if !db.IsWriteQuery("DELETE FROM t") {
		t.Fatal("DELETE should be a write")
	}
}
