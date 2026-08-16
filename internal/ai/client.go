// Package ai provides a minimal client for natural-language-to-SQL via a
// hosted (or self-hosted) chat-completions API. Phase 0 speaks the OpenAI
// Chat Completions wire format, which is also served by OpenAI-compatible
// local runtimes (Ollama's /v1 endpoint, LM Studio, vLLM, etc.), so the same
// client reaches a cloud provider or a localhost model with only a base-URL
// change. Adding providers with their own wire format (Anthropic, etc.) later
// is a matter of writing a second implementation of the Completer interface.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Defaults. BaseURL is the public OpenAI endpoint; pointing it at a local
// runtime (e.g. http://localhost:11434/v1 for Ollama) makes the whole feature
// run offline. Model is a cheap, broadly available default; override per use.
const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-4o-mini"
	DefaultTimeout = 60 * time.Second
)

// Config holds everything the client needs to reach a provider. Zero values
// are filled in by New with the package defaults. APIKey is required for
// hosted providers; local runtimes typically ignore it (pass any non-empty
// placeholder since some refuse an empty header).
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// New returns a Client with defaults applied. It performs no network I/O.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Client{cfg: cfg, http: http.DefaultClient}
}

// Client talks to a chat-completions endpoint. The HTTP client is overridable
// for testing.
type Client struct {
	cfg  Config
	http interface {
		Do(*http.Request) (*http.Response, error)
	}
}

// Message is a single chat-completions message. Role is one of "system",
// "user", or "assistant".
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Complete sends the conversation and returns the assistant's reply text. The
// context bounds the whole request (dial + headers + body read); callers
// should derive it from the user's cancel action so esc can abort a slow model.
func (c *Client) Complete(ctx context.Context, messages []Message) (string, error) {
	if c.http == nil {
		return "", fmt.Errorf("ai: client not initialized")
	}
	key := c.cfg.APIKey
	if key == "" {
		return "", ErrNoAPIKey
	}

	body, err := json.Marshal(chatRequest{
		Model:    c.cfg.Model,
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("ai: encoding request: %w", err)
	}

	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Surface the provider's error body — it usually names the problem
		// (bad key, quota, unknown model) in plain text.
		return "", fmt.Errorf("ai: provider returned %s: %s", resp.Status, truncateErr(raw))
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("ai: decoding response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("ai: empty response (no choices)")
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}

// ListModels queries the provider's OpenAI-compatible GET /models endpoint and
// returns the available model ids. The assistant-panel model browser uses it
// so users can pick a model that is live for their key rather than guessing.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	if c.http == nil {
		return nil, fmt.Errorf("ai: client not initialized")
	}
	key := c.cfg.APIKey
	if key == "" {
		return nil, ErrNoAPIKey
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("ai: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ai: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ai: provider returned %s: %s", resp.Status, truncateErr(raw))
	}

	var lr struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &lr); err != nil {
		return nil, fmt.Errorf("ai: decoding response: %w", err)
	}
	ids := make([]string, 0, len(lr.Data))
	for _, m := range lr.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// StreamDelta is one piece of a streamed reply: either a content token (the
// visible answer) or a reasoning token (chain-of-thought, from reasoning
// models). At most one field is non-empty per call.
type StreamDelta struct {
	Content   string
	Reasoning string
}

// CompleteStream is the streaming variant of Complete: it posts with
// stream:true and invokes onDelta for each token as it arrives (Server-Sent
// Events) — content in StreamDelta.Content, chain-of-thought (reasoning
// models) in StreamDelta.Reasoning — returning the full accumulated reply.
// This lets the UI render the answer (and the model's thinking) forming in
// real time instead of blocking on the whole response. onDelta may be nil.
// The same context bounds the call.
func (c *Client) CompleteStream(ctx context.Context, messages []Message, onDelta func(StreamDelta)) (string, error) {
	if c.http == nil {
		return "", fmt.Errorf("ai: client not initialized")
	}
	key := c.cfg.APIKey
	if key == "" {
		return "", ErrNoAPIKey
	}

	body, err := json.Marshal(chatRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return "", fmt.Errorf("ai: encoding request: %w", err)
	}

	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai: provider returned %s: %s", resp.Status, truncateErr(raw))
	}

	// SSE frames are "data: <json>\\n" lines terminated by "data: [DONE]".
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20) // allow long frames
	var full strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue // keep-alive comments / empty lines
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				break
			}
			continue
		}
		var ch chatStreamChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			continue // skip a malformed frame rather than aborting mid-stream
		}
		if len(ch.Choices) == 0 {
			continue
		}
		d := ch.Choices[0].Delta
		if d.Content != "" {
			full.WriteString(d.Content)
		}
		if onDelta != nil && (d.Content != "" || d.Reasoning != "") {
			onDelta(StreamDelta{Content: d.Content, Reasoning: d.Reasoning})
		}
	}
	if err := scanner.Err(); err != nil {
		// A cancelled context surfaces here as a read error; surface it so the
		// caller can mark the request cancelled rather than successful-but-empty.
		return strings.TrimSpace(full.String()), fmt.Errorf("ai: reading stream: %w", err)
	}
	return strings.TrimSpace(full.String()), nil
}

// AskSQL is the high-level natural-language-to-SQL helper. It builds a system
// prompt from the database schema, appends the user's question, and returns
// just the SQL extracted from the model's reply.
func AskSQL(ctx context.Context, cfg Config, schema, question string) (string, error) {
	c := New(cfg)
	messages := []Message{
		{Role: "system", Content: systemPrompt(schema)},
		{Role: "user", Content: question},
	}
	reply, err := c.Complete(ctx, messages)
	if err != nil {
		return "", err
	}
	sql := ExtractSQL(reply)
	if sql == "" {
		return "", fmt.Errorf("ai: model gave no SQL in its reply: %s", truncateErr([]byte(reply)))
	}
	return sql, nil
}

// SystemPrompt instructs the model to return a single read-only SQL statement
// for the given schema. It forbids prose and asks for a SELECT so a
// misunderstood request cannot mutate data — a safety default we can relax
// per-command once the assistant panel has explicit "run" gating. Exported so
// the UI layer can assemble multi-turn conversations (system + prior turns).
func SystemPrompt(schema string) string {
	return systemPrompt(schema)
}

// FixSystemPrompt instructs the model to correct a failed SQL statement.
// Unlike SystemPrompt it does not force SELECT — the rewrite should match the
// original intent — but still asks for a single statement with no prose.
func FixSystemPrompt(schema string) string {
	var b strings.Builder
	b.WriteString("You are a SQL assistant inside a database browser. ")
	b.WriteString("The user ran a SQL statement that failed. Reply with ONLY a single ")
	b.WriteString("corrected SQL statement — no explanation, no markdown fences. ")
	b.WriteString("Make the smallest change that fixes the error. Preserve the user's intent ")
	b.WriteString("(do not turn a SELECT into a write, or invent DROP/TRUNCATE/DELETE). ")
	b.WriteString("Use the exact table and column names from the schema.\n\n")
	b.WriteString("Database schema:\n")
	if schema == "" {
		b.WriteString("(schema unavailable)")
	} else {
		b.WriteString(schema)
	}
	return b.String()
}

// FixUserPrompt builds the user turn for a fix-this-error request.
func FixUserPrompt(failedSQL, errMsg string) string {
	var b strings.Builder
	b.WriteString("The following SQL failed. Return a corrected SQL statement only.\n\n")
	b.WriteString("SQL:\n")
	b.WriteString(strings.TrimSpace(failedSQL))
	b.WriteString("\n\nError:\n")
	b.WriteString(strings.TrimSpace(errMsg))
	return b.String()
}

// systemPrompt instructs the model to return a single read-only SQL statement
// for the given schema. It forbids prose and asks for a SELECT so a
// misunderstood request cannot mutate data — a safety default we can relax
// per-command once the assistant panel has explicit "run" gating.
func systemPrompt(schema string) string {
	var b strings.Builder
	b.WriteString("You are a SQL assistant inside a database browser. ")
	b.WriteString("Given a schema and a natural-language request, reply with ONLY a single SQL ")
	b.WriteString("statement — no explanation, no markdown fences. ")
	b.WriteString("Prefer a SELECT. Use the exact table and column names from the schema. ")
	b.WriteString("If the request is ambiguous, make a reasonable assumption and still return SQL.\n\n")
	b.WriteString("Database schema:\n")
	if schema == "" {
		b.WriteString("(schema unavailable)")
	} else {
		b.WriteString(schema)
	}
	return b.String()
}

// chatRequest is the subset of the OpenAI Chat Completions body we send.
type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// chatResponse is the subset of the response we read.
type chatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

// chatStreamChunk is one SSE frame from a streaming completion. Content is
// the visible reply; reasoning is the model's chain-of-thought, emitted by
// reasoning models (e.g. GLM) in a separate field before/around the content.
// finish_reason is ignored (the [DONE] sentinel ends the stream).
type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// truncateErr keeps provider error bodies short for status-bar display.
func truncateErr(b []byte) string {
	const max = 300
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// ErrNoAPIKey is returned when no API key is configured. The UI maps this to
// a helpful, actionable status-bar message.
var ErrNoAPIKey = fmt.Errorf("no AI API key set — configure one to use :ai (see :help ai)")
