package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestExtractSQL_BareStatement(t *testing.T) {
	in := "SELECT * FROM users ORDER BY created_at DESC LIMIT 10;"
	got := ExtractSQL(in)
	want := "SELECT * FROM users ORDER BY created_at DESC LIMIT 10;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractSQL_FencedWithLanguageTag(t *testing.T) {
	in := "Here you go:\n\n```sql\nSELECT id, email FROM users;\n```\n"
	got := ExtractSQL(in)
	want := "SELECT id, email FROM users;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractSQL_FencedNoTag(t *testing.T) {
	in := "```\nWITH recent AS (\n  SELECT * FROM orders\n)\nSELECT * FROM recent;\n```"
	got := ExtractSQL(in)
	want := "WITH recent AS (\n  SELECT * FROM orders\n)\nSELECT * FROM recent;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractSQL_ProseThenStatement(t *testing.T) {
	in := "Sure! This query finds the top 10:\n\nSELECT * FROM users LIMIT 10;"
	got := ExtractSQL(in)
	want := "SELECT * FROM users LIMIT 10;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractSQL_Empty(t *testing.T) {
	if got := ExtractSQL(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestSchemaContext(t *testing.T) {
	conn := &fakeConn{
		tables: []string{"users", "orders"},
		schema: map[string][]Column{
			"users":  {{Name: "id", Type: "INTEGER"}, {Name: "email", Type: "TEXT"}, {Name: "created_at", Type: "TIMESTAMP"}},
			"orders": {{Name: "id", Type: "INTEGER"}, {Name: "user_id", Type: "INTEGER"}, {Name: "total", Type: "NUMERIC"}},
		},
		pks: map[string][]string{
			"users":  {"id"},
			"orders": {"id"},
		},
		fks: map[string][]ForeignKey{
			"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		},
	}
	got, err := SchemaContext(conn)
	if err != nil {
		t.Fatalf("SchemaContext error: %v", err)
	}
	// Spot-check the structural pieces the model needs.
	for _, want := range []string{
		"CREATE TABLE users",
		"CREATE TABLE orders",
		"id INTEGER  -- primary key",
		"user_id INTEGER  -- FK -> users.id",
		"email TEXT",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestSchemaContext_NilConn(t *testing.T) {
	got, err := SchemaContext(nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty context for nil conn, got %q", got)
	}
}

func TestSchemaContext_BadTableStillListed(t *testing.T) {
	conn := &fakeConn{
		tables:    []string{"good", "bad"},
		schema:    map[string][]Column{"good": {{Name: "id", Type: "INTEGER"}}},
		schemaErr: map[string]error{"bad": errSentinel},
	}
	got, err := SchemaContext(conn)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(got, "CREATE TABLE bad") {
		t.Errorf("bad table should still be listed:\n%s", got)
	}
	if !strings.Contains(got, "schema unavailable") {
		t.Errorf("bad table should note schema unavailable:\n%s", got)
	}
}

func TestAskSQL_BuildsRequestAndExtracts(t *testing.T) {
	var seenBody chatRequest
	fence := "```"
	respBody := `{"choices":[{"message":{"role":"assistant","content":"` +
		fence + `sql\nSELECT 1;\n` + fence + `"}}]}`
	stub := &httpStub{
		status: 200,
		body:   respBody,
		onBody: func(b []byte) {
			_ = json.Unmarshal(b, &seenBody)
		},
	}
	c := New(Config{APIKey: "k", BaseURL: "http://stub", Model: "stub-model"})
	c.http = stub

	out, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	// Complete returns the model's raw reply; ExtractSQL (tested above)
	// handles fence stripping. Just confirm we got the fenced SQL back.
	if want := fence + "sql"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
	if seenBody.Model != "stub-model" {
		t.Errorf("request model = %q, want stub-model", seenBody.Model)
	}
	if len(seenBody.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(seenBody.Messages))
	}
	if stub.gotAuth != "Bearer k" {
		t.Errorf("auth header = %q, want Bearer k", stub.gotAuth)
	}
	if !strings.HasSuffix(stub.gotURL, "/chat/completions") {
		t.Errorf("URL = %q, want to end with /chat/completions", stub.gotURL)
	}
}

func TestComplete_NoAPIKey(t *testing.T) {
	c := New(Config{BaseURL: "http://stub"})
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != ErrNoAPIKey {
		t.Errorf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestComplete_HTTPError(t *testing.T) {
	c := New(Config{APIKey: "k", BaseURL: "http://stub"})
	c.http = &httpStub{status: 401, body: `{"error":"invalid api key"}`}
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err == nil || !strings.Contains(err.Error(), http.StatusText(401)) {
		t.Fatalf("expected a 401 error, got %v", err)
	}
}

func TestCompleteStream_DeltasAndFull(t *testing.T) {
	var seenBody chatRequest
	// SSE frames: a reasoning delta, then two content deltas, then [DONE].
	sse := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"need top 10\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"SELECT\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" 1\"}}]}\n" +
		"data: [DONE]\n"
	stub := &httpStub{
		status: 200,
		body:   sse,
		onBody: func(b []byte) { _ = json.Unmarshal(b, &seenBody) },
	}
	c := New(Config{APIKey: "k", BaseURL: "http://stub", Model: "stub-model"})
	c.http = stub

	var contentDeltas, reasoningDeltas []string
	full, err := c.CompleteStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, func(d StreamDelta) {
		if d.Content != "" {
			contentDeltas = append(contentDeltas, d.Content)
		}
		if d.Reasoning != "" {
			reasoningDeltas = append(reasoningDeltas, d.Reasoning)
		}
	})
	if err != nil {
		t.Fatalf("CompleteStream error: %v", err)
	}
	// Only content is accumulated into the reply; reasoning is transient.
	if full != "SELECT 1" {
		t.Errorf("full = %q, want %q", full, "SELECT 1")
	}
	if len(contentDeltas) != 2 || contentDeltas[0] != "SELECT" || contentDeltas[1] != " 1" {
		t.Errorf("content deltas = %v, want [SELECT ' 1']", contentDeltas)
	}
	if len(reasoningDeltas) != 1 || reasoningDeltas[0] != "need top 10" {
		t.Errorf("reasoning deltas = %v, want [need top 10]", reasoningDeltas)
	}
	if !seenBody.Stream {
		t.Errorf("streaming request did not set stream:true")
	}
	if stub.gotAuth != "Bearer k" {
		t.Errorf("auth header = %q, want Bearer k", stub.gotAuth)
	}
}

func TestCompleteStream_HTTPError(t *testing.T) {
	c := New(Config{APIKey: "k", BaseURL: "http://stub"})
	c.http = &httpStub{status: 429, body: `rate limited`}
	_, err := c.CompleteStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil || !strings.Contains(err.Error(), http.StatusText(429)) {
		t.Fatalf("expected a 429 error, got %v", err)
	}
}

// --- fakes ---

var errSentinel = &sentinelErr{"lookup failed"}

type sentinelErr struct{ msg string }

func (e *sentinelErr) Error() string { return e.msg }

type fakeConn struct {
	tables    []string
	schema    map[string][]Column
	schemaErr map[string]error
	pks       map[string][]string
	fks       map[string][]ForeignKey
}

func (f *fakeConn) Tables() ([]string, error)                  { return f.tables, nil }
func (f *fakeConn) TableSchema(t string) ([]Column, error)     { return f.schema[t], f.schemaErr[t] }
func (f *fakeConn) PrimaryKeys(t string) ([]string, error)     { return f.pks[t], nil }
func (f *fakeConn) ForeignKeys(t string) ([]ForeignKey, error) { return f.fks[t], nil }

type httpStub struct {
	status  int
	body    string
	gotURL  string
	gotAuth string
	onBody  func([]byte)
}

func (s *httpStub) Do(req *http.Request) (*http.Response, error) {
	s.gotURL = req.URL.String()
	s.gotAuth = req.Header.Get("Authorization")
	if s.onBody != nil && req.Body != nil {
		if b, err := io.ReadAll(req.Body); err == nil {
			s.onBody(b)
		}
	}
	return &http.Response{
		StatusCode: s.status,
		Status:     http.StatusText(s.status),
		Body:       io.NopCloser(strings.NewReader(s.body)),
	}, nil
}
