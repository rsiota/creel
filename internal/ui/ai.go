package ui

import (
	"context"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/ai"
	"github.com/ruben/gsql/internal/db"
)

// Phase 0 AI integration. The :ai <question> ex-command builds a schema
// context from the active connection, asks the model to turn the question
// into SQL asynchronously, and — on success — drops the SQL into the editor
// for review (the user runs it with ctrl+e). Errors and short feedback are
// shown via the transient status bar (m.aiMsg), exactly like the other
// result-message fields.
//
// Configuration is environment-based for now (GSQL_AI_API_KEY /
// GSQL_AI_BASE_URL / GSQL_AI_MODEL); Phase 1 will move it into
// config.Settings and the OS keychain so the key is not visible in the shell.

// aiResultMsg carries the model's reply (raw + extracted SQL), or the
// failure. toPanel routes the result: true appends to the assistant panel's
// transcript, false drops the SQL into the editor (the :ai command).
type aiResultMsg struct {
	reply   string
	sql     string
	toPanel bool
	err     error
}

// dbAdapter exposes db.DB to the ai package via its own small types, keeping
// internal/ai free of an internal/db import (and so independently testable).
type dbAdapter struct{ d db.DB }

// Compile-time proof that dbAdapter satisfies ai.SchemaIntrospector. If a
// method signature drifts, this fails to build rather than failing at runtime.
var _ ai.SchemaIntrospector = dbAdapter{}

func (a dbAdapter) Tables() ([]string, error) { return a.d.Tables() }

func (a dbAdapter) TableSchema(table string) ([]ai.Column, error) {
	cols, err := a.d.TableSchema(table)
	if err != nil {
		return nil, err
	}
	out := make([]ai.Column, len(cols))
	for i, c := range cols {
		out[i] = ai.Column{Name: c.Name, Type: c.Type}
	}
	return out, nil
}

func (a dbAdapter) PrimaryKeys(table string) ([]string, error) {
	return a.d.PrimaryKeys(table)
}

func (a dbAdapter) ForeignKeys(table string) ([]ai.ForeignKey, error) {
	fks, err := a.d.ForeignKeys(table)
	if err != nil {
		return nil, err
	}
	out := make([]ai.ForeignKey, len(fks))
	for i, f := range fks {
		out[i] = ai.ForeignKey{Column: f.Column, RefTable: f.RefTable, RefColumn: f.RefColumn}
	}
	return out, nil
}

// aiConfigFromEnv reads the provider configuration from the environment.
// It tries, in order, GSQL_AI_API_KEY, OPENAI_API_KEY, then ZAI_API_KEY (the
// env var pi documents for z.ai) — so a z.ai setup inherited from pi works
// with no extra wiring.
//
// When the key is a z.ai key (sourced from ZAI_API_KEY) and the caller did not
// pin an explicit base URL / model, the z.ai *coding* endpoint and a cheap GLM
// model are used by default. This matters: z.ai coding-plan keys are rejected
// on the generic /api/paas/v4 endpoint, which is the usual cause of an
// "unauthorized" / "token expired or incorrect" error. Override either with
// GSQL_AI_BASE_URL / GSQL_AI_MODEL. Pointing the base URL at a local runtime
// (e.g. http://localhost:11434/v1 for Ollama) makes :ai fully offline.
func aiConfigFromEnv() ai.Config {
	var key, src string
	for _, env := range []string{"GSQL_AI_API_KEY", "OPENAI_API_KEY", "ZAI_API_KEY"} {
		if v := os.Getenv(env); v != "" {
			key, src = v, env
			break
		}
	}
	baseURL := os.Getenv("GSQL_AI_BASE_URL")
	model := os.Getenv("GSQL_AI_MODEL")
	if src == "ZAI_API_KEY" {
		if baseURL == "" {
			baseURL = "https://api.z.ai/api/coding/paas/v4"
		}
		if model == "" {
			model = "glm-4.6" // fast, non-reasoning; ~3x quicker than glm-4.5-air
		}
	}
	return ai.Config{
		APIKey:  key,
		BaseURL: firstNonEmpty(baseURL, ai.DefaultBaseURL),
		Model:   firstNonEmpty(model, ai.DefaultModel),
		Timeout: 60 * time.Second,
	}
}

// errString safely stringifies an error for display.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// summaryFor produces a one-line caption for an assistant turn. We currently
// surface the SQL itself in the transcript, so the summary mirrors it; kept as
// a seam so a future change (e.g. showing a model-provided label) is local.
func summaryFor(msg aiResultMsg) string {
	if msg.sql != "" {
		return msg.sql
	}
	return strings.TrimSpace(msg.reply)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// aiAuthHint returns a short, actionable suffix appended to AI auth failures.
// The dominant real-world cause is a z.ai coding-plan key hitting the wrong
// endpoint (the generic /api/paas/v4 path rejects it as "unauthorized" /
// "token expired or incorrect"), so we detect that signature and point at the
// base URL rather than letting the user assume their key is bad.
func aiAuthHint(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "401"), strings.Contains(s, "403"),
		strings.Contains(s, "unauthorized"), strings.Contains(s, "token expired"):
		return " — check GSQL_AI_BASE_URL matches your provider (z.ai coding keys need the /api/coding/paas/v4 endpoint)"
	}
	return ""
}

// exAI dispatches the asynchronous natural-language-to-SQL request for the
// ":ai" ex-command. The generated SQL is routed to the editor for review.
func (m *Model) exAI(question string) tea.Cmd {
	if m.connection == nil {
		m.aiMsg = ":ai needs an open connection"
		return nil
	}
	q := strings.TrimSpace(question)
	if q == "" {
		m.aiMsg = ":ai needs a question — try :ai 10 most recent users"
		return nil
	}
	schema, _ := ai.SchemaContext(dbAdapter{m.connection.DB()})
	messages := []ai.Message{
		{Role: "system", Content: ai.SystemPrompt(schema)},
		{Role: "user", Content: q},
	}
	return m.dispatchAI(messages, false, q)
}

// sendAssistant dispatches a request from the assistant panel. The full
// transcript is sent as conversation context so follow-up turns ("now filter
// to active users") work; the result is appended to the panel's transcript.
func (m *Model) sendAssistant(question string) tea.Cmd {
	if m.connection == nil {
		return nil
	}
	schema, _ := ai.SchemaContext(dbAdapter{m.connection.DB()})
	messages := []ai.Message{{Role: "system", Content: ai.SystemPrompt(schema)}}
	for _, t := range m.assistant.ConversationMessages() {
		messages = append(messages, ai.Message{Role: t.role, Content: t.content})
	}
	messages = append(messages, ai.Message{Role: "user", Content: question})
	return m.dispatchAI(messages, true, question)
}

// dispatchAI is the shared core for both routes: it validates config, marks the
// request in-flight (driving the status-bar spinner / esc-cancel gating), and
// returns a batched command whose result arrives as aiResultMsg. toPanel
// records the destination so the result handler knows where to deliver it.
func (m *Model) dispatchAI(messages []ai.Message, toPanel bool, question string) tea.Cmd {
	cfg := aiConfigFromEnv()
	if cfg.APIKey == "" {
		m.aiMsg = "set $GSQL_AI_API_KEY / $OPENAI_API_KEY / $ZAI_API_KEY to use :ai"
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	m.aiRunning = true
	m.aiToPanel = toPanel
	m.aiCancel = cancel
	m.aiQuestion = question
	m.aiStart = time.Now()
	m.aiMsg = ""

	c := ai.New(cfg)
	ask := func() tea.Msg {
		defer cancel()
		reply, err := c.Complete(ctx, messages)
		if err != nil {
			return aiResultMsg{err: err, toPanel: toPanel}
		}
		return aiResultMsg{reply: reply, sql: ai.ExtractSQL(reply), toPanel: toPanel}
	}
	// Batch the request with a spinner tick so the pending hint animates the
	// elapsed timer; without it a slow model looks frozen.
	return tea.Batch(ask, spinnerTick())
}
