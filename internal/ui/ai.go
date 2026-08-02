package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/creel/internal/ai"
	"github.com/ruben/creel/internal/config"
	"github.com/ruben/creel/internal/db"
	"github.com/ruben/creel/internal/secrets"
)

// AI integration. The :ai <question> ex-command and the assistant panel build
// a schema context from the active connection, ask the model to turn the
// question into SQL asynchronously, and — on success — drop the SQL into the
// editor for review (the user runs it with ctrl+e). Errors and short feedback
// are shown via the transient status bar (m.aiMsg), exactly like the other
// result-message fields.
//
// Providers are configured in the `ai:` config block (edited in-app via the
// `M` picker → n/e provider form, which stores the API key in the OS keychain
// as a secret:// ref). With no providers configured, the env vars
// CREEL_AI_API_KEY / CREEL_AI_BASE_URL / CREEL_AI_MODEL are used as a fallback.

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
// It tries, in order, CREEL_AI_API_KEY, the deprecated GSQL_AI_API_KEY,
// OPENAI_API_KEY, then ZAI_API_KEY (the env var pi documents for z.ai) — so a
// z.ai setup inherited from pi works with no extra wiring, and users upgrading
// from gsql keep working until they migrate their shell rc. CREEL_AI_BASE_URL
// and CREEL_AI_MODEL likewise fall back to their GSQL_AI_* equivalents.
//
// When the key is a z.ai key (sourced from ZAI_API_KEY) and the caller did not
// pin an explicit base URL / model, the z.ai *coding* endpoint and a cheap GLM
// model are used by default. This matters: z.ai coding-plan keys are rejected
// on the generic /api/paas/v4 endpoint, which is the usual cause of an
// "unauthorized" / "token expired or incorrect" error. Override either with
// CREEL_AI_BASE_URL / CREEL_AI_MODEL. Pointing the base URL at a local runtime
// (e.g. http://localhost:11434/v1 for Ollama) makes :ai fully offline.
func aiConfigFromEnv() ai.Config {
	var key, src string
	for _, env := range []string{"CREEL_AI_API_KEY", "GSQL_AI_API_KEY", "OPENAI_API_KEY", "ZAI_API_KEY"} {
		if v := os.Getenv(env); v != "" {
			key, src = v, env
			break
		}
	}
	baseURL := envOrDeprecated("CREEL_AI_BASE_URL", "GSQL_AI_BASE_URL")
	model := envOrDeprecated("CREEL_AI_MODEL", "GSQL_AI_MODEL")
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

// envOrDeprecated returns name's value, falling back to deprecatedName when
// name is unset/empty. Used so the CREEL_AI_* env vars keep honouring their
// GSQL_AI_* predecessors until users migrate their shell configuration.
func envOrDeprecated(name, deprecatedName string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return os.Getenv(deprecatedName)
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
		return " — check CREEL_AI_BASE_URL matches your provider (z.ai coding keys need the /api/coding/paas/v4 endpoint)"
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
// it falls through to the CREEL_AI_* environment variables. This is the single
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

// --- provider form plumbing ------------------------------------------------
//
// Messages and helpers for the add/edit provider form (opened from the `M`
// picker via n / e). The form lives in ai_provider_form.go; these wire it into
// the Update loop and the config + keychain.

// openProviderFormAddMsg opens the provider form in add mode.
type openProviderFormAddMsg struct{}

// openProviderFormEditMsg opens the provider form pre-filled from the named
// provider (selected in the `M` picker when `e` was pressed).
type openProviderFormEditMsg struct {
	name string
}

// providerTestResultMsg carries the outcome of a /models probe issued from
// the form's ctrl+t test.
type providerTestResultMsg struct {
	err error
}

// storeProviderSecret migrates a provider's API key to the OS keychain when
// mode is "keychain", replacing it in the config with an opaque "secret://"
// reference. editName is the provider's pre-edit name ("" in add mode): when
// set and the provider is being renamed (or switched to plaintext), the old
// keychain entry is purged so a stale secret does not linger. Returns the
// (possibly modified) provider and an error describing why the keychain could
// not be used; in that case the provider is returned unchanged so the caller
// falls back to plaintext.
func storeProviderSecret(p config.AIProvider, mode, editName string) (config.AIProvider, error) {
	if mode != "keychain" {
		// Plain mode: ensure no keychain entry lingers for this provider. A
		// missing key (it never used the keychain) is not an error.
		if editName != "" {
			_ = secrets.DeleteAI(editName)
		}
		return p, nil
	}
	if !secrets.Available() {
		return p, fmt.Errorf("keychain unavailable on this system; api key stored in config file")
	}
	if p.APIKey != "" && !secrets.IsReference(p.APIKey) {
		ref, err := secrets.StoreAI(p.Name, p.APIKey)
		if err != nil {
			return p, fmt.Errorf("storing api key: %w", err)
		}
		p.APIKey = ref
	}
	// Purge the pre-edit keychain entry when the provider was renamed, so the
	// old "ai/<editName>/api_key" key does not orphan.
	if editName != "" && editName != p.Name {
		_ = secrets.DeleteAI(editName)
	}
	return p, nil
}

// testProvider probes the provider the form is editing by hitting its /models
// endpoint — the same call the `m` browser makes. The form's current field
// values (not the saved config) are used so an unsaved key can be validated
// before committing. The key is resolved (plaintext or ref) so an edit-mode
// form holding an unreadable ref still tests the saved value's intent.
func (m *Model) testProvider() tea.Cmd {
	f := &m.providerForm
	key := strings.TrimSpace(f.fields[pfKey].Value())
	baseURL := strings.TrimSpace(f.fields[pfBaseURL].Value())
	if key == "" {
		f.errMsg = "api key is required"
		return nil
	}
	resolved, _ := secrets.Resolve(key)
	cfg := ai.Config{
		APIKey:  resolved,
		BaseURL: firstNonEmpty(baseURL, ai.DefaultBaseURL),
		Model:   ai.DefaultModel,
		Timeout: 20 * time.Second,
	}
	f.SetTesting(true)
	client := ai.New(cfg)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()
		_, err := client.ListModels(ctx)
		return providerTestResultMsg{err: err}
	}
}

// saveProviderForm commits the provider form: validates, checks the name is
// unique, migrates the API key to the keychain when requested, persists the
// provider to the config (auto-defaulting if it is the first), then reopens
// the `M` picker over the saved list. edit-mode renames are handled by
// dropping the old entry and (in keychain mode) purging its old key.
func (m *Model) saveProviderForm() tea.Cmd {
	p, errMsg := m.providerForm.EnterPressed()
	if errMsg != "" {
		m.providerForm.SetError(errMsg)
		return nil
	}
	editName := m.providerForm.editName

	// Name uniqueness. Editing in place (no rename) is always fine; any other
	// collision (a rename onto an existing name, or a duplicate in add mode) is
	// rejected before any mutation.
	self := editName != "" && editName == p.Name
	if !self && m.config.GetAIProvider(p.Name) != nil {
		m.providerForm.SetError("a provider named '" + p.Name + "' already exists")
		return nil
	}

	// Edit mode: drop the old entry (its keychain key is reconciled below).
	if editName != "" {
		m.config.RemoveAIProvider(editName)
	}

	// Migrate the API key to the keychain when requested. A keychain failure is
	// non-fatal: the key stays plaintext in the config and the message is
	// surfaced — mirroring the connection form.
	p, secErr := storeProviderSecret(p, m.providerForm.secretsMode(), editName)
	m.aiMsg = ""
	if secErr != nil {
		m.aiMsg = secErr.Error()
	}

	m.config.AddAIProvider(p)

	// First provider configured: make it the default so it is active right
	// away. Otherwise keep the existing default (the `M` picker sets it on
	// enter).
	if m.config.AI.Default == "" {
		m.config.AI.Default = p.Name
	}

	// Keep the in-memory active choice in step with a rename.
	if editName != "" && m.aiProvider == editName {
		m.aiProvider = p.Name
	}

	if err := m.config.Save(); err != nil {
		m.providerForm.SetError(err.Error())
		return nil
	}

	if m.aiMsg == "" {
		m.aiMsg = "saved provider: " + p.Name
	}
	m.providerForm.Hide()
	m.providerPicker.Show(m.config.AI.Providers, p.Name)
	return nil
}

// deleteSelectedProvider removes the provider under the `M` picker cursor,
// purging its keychain key and reconciling the default / in-memory active
// choice. Mirrors deleteSelectedConnection (no confirmation, matching the
// connection list's convention).
// deleteSelectedProvider is the `d` entry point from the `M` picker: it
// honours the confirm_destructive gate, staging a y/n prompt when on and
// deleting immediately when off. The cursor selection is captured into the
// confirm flag so the prompt names the right provider even though keys are
// swallowed while it is up.
func (m *Model) deleteSelectedProvider() tea.Cmd {
	name := m.providerPicker.Selected()
	if name == "" {
		return nil
	}
	if m.confirmDestructive() {
		m.deleteProviderConfirm = name
		return nil
	}
	return m.deleteProvider(name)
}

// deleteProvider removes the named provider, purging its keychain key and
// reconciling the default / in-memory active choice. Shared by the gated
// (y-confirmed) and ungated paths so the confirmed action is identical. No
// confirmation here — the gate is decided by the caller.
func (m *Model) deleteProvider(name string) tea.Cmd {
	if name == "" {
		return nil
	}
	_ = secrets.DeleteAI(name) // best-effort keychain purge; missing key is fine
	m.config.RemoveAIProvider(name)
	if m.aiProvider == name {
		m.aiProvider = ""
	}
	// RemoveAIProvider clears Default when it matched; if providers remain,
	// keep a sensible default (the first) so the panel does not silently drop
	// to env-only mode.
	if m.hasAIProviders() && m.config.AI.Default == "" {
		m.config.AI.Default = m.config.AI.Providers[0].Name
	}
	if err := m.config.Save(); err != nil {
		m.aiMsg = err.Error()
		return nil
	}
	m.aiMsg = "removed provider: " + name
	// Reopen the picker over the remaining providers, or hide it if that was
	// the last one (the panel drops to env-only mode).
	if m.hasAIProviders() {
		m.providerPicker.Show(m.config.AI.Providers, m.effectiveProviderName())
	} else {
		m.providerPicker.Hide()
	}
	return nil
}

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
			m.aiMsg = "active AI provider has no api key — press M then e to set it"
		} else {
			m.aiMsg = "no AI provider configured — press M then n to add one (or set $CREEL_AI_API_KEY)"
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
