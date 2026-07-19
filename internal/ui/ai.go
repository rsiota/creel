package ui

import (
	"context"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/ai"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
	"github.com/ruben/gsql/internal/secrets"
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

// aiStreamChunkMsg carries one coalesced batch of streamed tokens for the
// in-flight panel request (content = the answer, reasoning = the model's
// chain-of-thought), so the reply renders forming in real time instead of
// all at once when the whole response lands.
type aiStreamChunkMsg struct {
	content   string
	reasoning string
}

// aiColumns / aiForeignKeys convert the db package's structs into the ai
// package's mirror types, keeping internal/ai free of an internal/db import.
func aiColumns(cols []db.Column) []ai.Column {
	out := make([]ai.Column, len(cols))
	for i, c := range cols {
		out[i] = ai.Column{Name: c.Name, Type: c.Type}
	}
	return out
}

func aiForeignKeys(fks []db.ForeignKey) []ai.ForeignKey {
	out := make([]ai.ForeignKey, len(fks))
	for i, f := range fks {
		out[i] = ai.ForeignKey{Column: f.Column, RefTable: f.RefTable, RefColumn: f.RefColumn}
	}
	return out
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
	return aiColumns(cols), nil
}

func (a dbAdapter) PrimaryKeys(table string) ([]string, error) {
	return a.d.PrimaryKeys(table)
}

func (a dbAdapter) ForeignKeys(table string) ([]ai.ForeignKey, error) {
	fks, err := a.d.ForeignKeys(table)
	if err != nil {
		return nil, err
	}
	return aiForeignKeys(fks), nil
}

// cachedAIIntrospector serves the AI schema context from the app's in-memory
// caches (populated by prefetchSchemas), so an AI turn no longer re-runs
// 1+3N metadata queries against the connection every time — a real cost on
// remote MySQL/Postgres. Any table missing from a cache (cold start, or a
// table the background prefetch skipped on error) falls back to the live
// connection so the model still sees a complete schema.
type cachedAIIntrospector struct {
	tables  []string
	columns map[string][]db.Column
	pks     map[string][]string
	fks     map[string][]db.ForeignKey
	live    dbAdapter
}

// Compile-time proof that cachedAIIntrospector satisfies the interface.
var _ ai.SchemaIntrospector = cachedAIIntrospector{}

func (c cachedAIIntrospector) Tables() ([]string, error) {
	if len(c.tables) > 0 {
		return c.tables, nil
	}
	return c.live.Tables()
}

func (c cachedAIIntrospector) TableSchema(table string) ([]ai.Column, error) {
	if cols, ok := c.columns[table]; ok {
		return aiColumns(cols), nil
	}
	return c.live.TableSchema(table)
}

func (c cachedAIIntrospector) PrimaryKeys(table string) ([]string, error) {
	if pk, ok := c.pks[table]; ok {
		return pk, nil
	}
	return c.live.PrimaryKeys(table)
}

func (c cachedAIIntrospector) ForeignKeys(table string) ([]ai.ForeignKey, error) {
	if fks, ok := c.fks[table]; ok {
		return aiForeignKeys(fks), nil
	}
	return c.live.ForeignKeys(table)
}

// aiIntrospector builds a schema introspector for an AI request: cache-backed
// (the prefetched tables/columns/PKs/FKs), with the live connection as a
// cold-cache fallback. Built on the main loop from m's caches, so the
// resulting schema string can be passed to the request goroutine with no data
// race.
func (m Model) aiIntrospector() ai.SchemaIntrospector {
	return cachedAIIntrospector{
		tables:  m.tables,
		columns: m.columnCache,
		pks:     m.pkCache,
		fks:     m.fkCache,
		live:    dbAdapter{m.connection.DB()},
	}
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
	schema, _ := ai.SchemaContext(m.aiIntrospector())
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
	schema, _ := ai.SchemaContext(m.aiIntrospector())
	messages := []ai.Message{{Role: "system", Content: ai.SystemPrompt(schema)}}
	for _, t := range m.assistant.ConversationMessages() {
		messages = append(messages, ai.Message{Role: t.role, Content: t.content})
	}
	messages = append(messages, ai.Message{Role: "user", Content: question})
	return m.dispatchAI(messages, true, question)
}

// activeProvider resolves the provider a request should use: the in-memory
// choice (m.aiProvider, set by the picker) if set, else the config's default.
// Returns the provider and true when one is configured; otherwise false and
// the caller falls back to the environment.
func (m Model) activeProvider() (config.AIProvider, bool) {
	name := m.effectiveProviderName()
	for _, p := range m.config.AI.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return config.AIProvider{}, false
}

// effectiveProviderName is the active provider name shown in the UI / used to
// place the picker cursor: the picker choice if set, else the config default.
func (m Model) effectiveProviderName() string {
	if m.aiProvider != "" {
		return m.aiProvider
	}
	return m.config.AI.Default
}

// aiConfig builds the AI client config. A configured provider wins (its API
// key is resolved — plaintext or keychain secret:// ref — and its base URL /
// model fall back to the ai defaults when empty); with no provider configured
// it falls through to the GSQL_AI_* environment variables. This is the single
// place that resolves which key/endpoint/model a request actually uses.
func (m Model) aiConfig() ai.Config {
	if p, ok := m.activeProvider(); ok {
		key, err := secrets.Resolve(p.APIKey)
		if err != nil {
			key = "" // an unreadable key ref surfaces as an auth error downstream
		}
		return ai.Config{
			APIKey:  key,
			BaseURL: firstNonEmpty(p.BaseURL, ai.DefaultBaseURL),
			Model:   firstNonEmpty(p.Model, ai.DefaultModel),
			Timeout: 60 * time.Second,
		}
	}
	return aiConfigFromEnv()
}

// effectiveAIModel is the model id shown in the UI / used to place the picker
// cursor: the active provider's model if one is configured, else the env
// default.
func (m Model) effectiveAIModel() string {
	if p, ok := m.activeProvider(); ok {
		return firstNonEmpty(p.Model, ai.DefaultModel)
	}
	return aiConfigFromEnv().Model
}

// hasAIProviders reports whether any provider is configured (and so the `M`
// switcher has something to show). When false the panel runs in env-only mode.
func (m Model) hasAIProviders() bool { return len(m.config.AI.Providers) > 0 }

// fetchModelsMsg carries the model ids returned by the active provider's
// /models endpoint (the standard OpenAI-compatible listing), or the failure.
// The model browser consumes it to populate its list.
type fetchModelsMsg struct {
	models []string
	err    error
}

// fetchModelsCmd queries the active provider for its available models. The
// resolved config is captured on the main loop so the returned goroutine never
// touches the Model (no data race).
func (m Model) fetchModelsCmd() tea.Cmd {
	client := ai.New(m.aiConfig())
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		models, err := client.ListModels(ctx)
		// Gemini's OpenAI-compatible /models returns "models/<id>"; normalise to
		// the bare form so the list matches the config convention and the
		// browser can land the cursor on the provider's current model.
		for i, id := range models {
			models[i] = strings.TrimPrefix(id, "models/")
		}
		return fetchModelsMsg{models: models, err: err}
	}
}

// dispatchAI is the shared core for both routes: it validates config, marks the
// request in-flight (driving the status-bar spinner / esc-cancel gating), and
// returns a batched command whose result arrives as aiResultMsg. toPanel
// records the destination so the result handler knows where to deliver it.
func (m *Model) dispatchAI(messages []ai.Message, toPanel bool, question string) tea.Cmd {
	cfg := m.aiConfig()
	if cfg.APIKey == "" {
		if _, ok := m.activeProvider(); ok {
			m.aiMsg = "active AI provider has no api_key — set it in ~/.config/gsql/config.yaml"
		} else {
			m.aiMsg = "set $GSQL_AI_API_KEY / $OPENAI_API_KEY / $ZAI_API_KEY, or configure an ai: provider in config.yaml"
		}
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

	if toPanel {
		// Streamed: a goroutine parses the SSE response and pushes chunk msgs
		// onto a channel; waitAIStream drains one msg per cycle, re-issuing
		// itself until the terminal aiResultMsg arrives. Deltas are coalesced
		// (~40ms) so a fast model can't flood the render loop.
		ch := make(chan tea.Msg, 16)
		m.aiStream = ch
		go func() {
			defer close(ch)
			var content, reasoning strings.Builder
			last := time.Now()
			flush := func() {
				if content.Len() == 0 && reasoning.Len() == 0 {
					return
				}
				ch <- aiStreamChunkMsg{content: content.String(), reasoning: reasoning.String()}
				content.Reset()
				reasoning.Reset()
				last = time.Now()
			}
			full, err := c.CompleteStream(ctx, messages, func(d ai.StreamDelta) {
				if d.Content != "" {
					content.WriteString(d.Content)
				}
				if d.Reasoning != "" {
					reasoning.WriteString(d.Reasoning)
				}
				if time.Since(last) >= streamChunkInterval {
					flush()
				}
			})
			flush() // flush the tail before the terminal result
			if err != nil {
				ch <- aiResultMsg{err: err, toPanel: true}
			} else {
				ch <- aiResultMsg{reply: full, sql: ai.ExtractSQL(full), toPanel: true}
			}
		}()
		return tea.Batch(waitAIStream(ch), spinnerTick())
	}

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

// streamChunkInterval caps how often a streamed reply flushes to the panel,
// bounding re-renders for a fast model.
const streamChunkInterval = 40 * time.Millisecond

// waitAIStream returns a command that waits for the next streamed message.
// Paired with re-issuing itself in the chunk handler, it drains the channel
// until the terminal aiResultMsg (which does not re-issue).
func waitAIStream(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}
