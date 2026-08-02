package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

// zaiKeyForTest returns a z.ai API key for integration tests, from $ZAI_API_KEY
// or pi's auth.json. The whole live test is opt-in: it only runs when
// $CREEL_AI_E2E is set, so an ordinary `go test ./...` never hits the network.
func zaiKeyForTest(t *testing.T) string {
	t.Helper()
	if os.Getenv("CREEL_AI_E2E") == "" {
		t.Skip("CREEL_AI_E2E not set — skipping live integration test")
	}
	if k := os.Getenv("ZAI_API_KEY"); k != "" {
		return k
	}
	home, _ := os.UserHomeDir()
	if b, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "auth.json")); err == nil {
		var v struct {
			Zai struct {
				Key string `json:"key"`
			} `json:"zai"`
		}
		if json.Unmarshal(b, &v) == nil && v.Zai.Key != "" {
			return v.Zai.Key
		}
	}
	t.Skip("no z.ai key (set $ZAI_API_KEY) — skipping live integration test")
	return ""
}

// TestAssistantEndToEnd drives the real UI: connect a Model to a temp SQLite
// DB, open the panel, submit a question, run the dispatched request against the
// live z.ai endpoint, and feed the result back through Update. Asserts the SQL
// lands in the transcript (not the editor, since it's the panel route) and the
// conversation context now carries the turn for follow-ups.
func TestAssistantEndToEnd(t *testing.T) {
	t.Setenv("ZAI_API_KEY", zaiKeyForTest(t))
	t.Setenv("CREEL_AI_MODEL", "glm-4.6")

	dbPath := filepath.Join(t.TempDir(), "ai.db")
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, created_at TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m.connection = conn
	m.state = stateWorkspace

	// Open the panel, type a question, and submit (as HandleKey(enter) would
	// in the real app — it returns a cmd that emits submitAssistantMsg).
	m.assistant.Show()
	m.assistant.input.SetValue("10 most recent users by signup")
	a, handleCmd := m.assistant.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m.assistant = a
	if handleCmd == nil {
		t.Fatal("enter produced no command")
	}
	submitMsg := handleCmd()
	mm2, dispatch := m.update(submitMsg)
	m = mm2.(Model)

	// The user turn should be recorded immediately, and a request dispatched.
	if !m.assistant.HasTurns() {
		t.Fatal("user turn not recorded on submit")
	}
	if !m.assistant.IsPending() {
		t.Fatal("panel should be pending after submit")
	}
	if dispatch == nil {
		t.Fatal("no request dispatched for the question")
	}

	// The panel path streams: a goroutine pushes chunk msgs then a terminal
	// aiResultMsg onto m.aiStream. Drain it until the result arrives.
	var resMsg aiResultMsg
	found := false
	for msg := range m.aiStream {
		if r, ok := msg.(aiResultMsg); ok {
			resMsg = r
			found = true
			break
		}
	}
	if !found {
		t.Fatal("stream produced no aiResultMsg")
	}
	if resMsg.err != nil {
		t.Fatalf("live request failed: %v", resMsg.err)
	}
	mm3, _ := m.update(resMsg)
	m = mm3.(Model)

	if m.assistant.IsPending() {
		t.Error("panel should not be pending after the result arrived")
	}
	sql := m.assistant.LatestSQL()
	if sql == "" {
		t.Fatal("transcript has no SQL after a successful turn")
	}
	t.Logf("generated SQL: %s", sql)

	// Panel route must NOT touch the editor.
	if m.editor.Value() != "" {
		t.Errorf("panel route leaked SQL into the editor: %q", m.editor.Value())
	}

	// The conversation context should now include the turn (1 user + 1 ai).
	turns := m.assistant.ConversationMessages()
	if len(turns) != 2 {
		t.Errorf("expected 2 context turns after one exchange, got %d", len(turns))
	}
}
