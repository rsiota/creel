package ai

import (
	"context"
	"strings"
	"testing"
)

func TestThinkFilter_Basic(t *testing.T) {
	var f ThinkFilter
	c, r := f.Feed("<think>need top 10</think>SELECT 1")
	if c != "SELECT 1" || r != "need top 10" {
		t.Fatalf("Feed = content %q reasoning %q", c, r)
	}
}

func TestThinkFilter_ChunkedTags(t *testing.T) {
	var f ThinkFilter
	var content, reasoning strings.Builder
	for _, chunk := range []string{"<th", "ink>plan", "</thi", "nk>SELECT", " 1"} {
		c, r := f.Feed(chunk)
		content.WriteString(c)
		reasoning.WriteString(r)
	}
	c, r := f.Flush()
	content.WriteString(c)
	reasoning.WriteString(r)
	if content.String() != "SELECT 1" {
		t.Errorf("content = %q, want SELECT 1", content.String())
	}
	if reasoning.String() != "plan" {
		t.Errorf("reasoning = %q, want plan", reasoning.String())
	}
}

func TestStripThinkTags(t *testing.T) {
	in := "<think>plan</think>\nSELECT * FROM users;"
	if got := StripThinkTags(in); got != "\nSELECT * FROM users;" {
		t.Errorf("StripThinkTags = %q", got)
	}
	if got := ExtractSQL(in); got != "SELECT * FROM users;" {
		t.Errorf("ExtractSQL with think tags = %q", got)
	}
}

func TestLooksLikeSQL_RejectsEnglish(t *testing.T) {
	no := []string{
		"Select the right table for signups",
		"Create a query that joins users",
		"Show me the recent orders",
		"With the schema in mind, I should…",
		"Explain why this fails",
	}
	for _, s := range no {
		if looksLikeSQL(s) {
			t.Errorf("looksLikeSQL(%q) = true, want false", s)
		}
	}
	yes := []string{
		"SELECT * FROM users",
		"WITH cte AS (SELECT 1)",
		"CREATE TABLE t (id INT)",
		"SHOW TABLES",
		"EXPLAIN SELECT 1",
		"SELECT",
	}
	for _, s := range yes {
		if !looksLikeSQL(s) {
			t.Errorf("looksLikeSQL(%q) = false, want true", s)
		}
	}
}

func TestCompleteStream_ReasoningFieldAndThinkTags(t *testing.T) {
	// Groq/Qwen parsed field name + raw <think> tags in content.
	sse := "data: {\"choices\":[{\"delta\":{\"reasoning\":\"from field\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"<think>from tags</think>SELECT 1\"}}]}\n" +
		"data: [DONE]\n"
	c := New(Config{APIKey: "k", BaseURL: "http://stub", Model: "stub"})
	c.http = &httpStub{status: 200, body: sse}

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
		t.Fatalf("CompleteStream: %v", err)
	}
	if full != "SELECT 1" {
		t.Errorf("full = %q, want SELECT 1 (think tags stripped)", full)
	}
	gotReason := strings.Join(reasoningDeltas, "")
	if !strings.Contains(gotReason, "from field") || !strings.Contains(gotReason, "from tags") {
		t.Errorf("reasoning deltas = %v, want field + tags", reasoningDeltas)
	}
}
