package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
)

func newParamModel() Model {
	return NewModel(&config.Config{})
}

func TestExpandQueryParamsBasic(t *testing.T) {
	params := map[string]string{
		"start": "2026-01-01",
		"limit": "10",
		"flag":  "NULL",
	}
	got, err := expandQueryParams(
		`SELECT * FROM t WHERE created_at > :start AND n < :limit AND deleted_at IS :flag`,
		params,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT * FROM t WHERE created_at > '2026-01-01' AND n < 10 AND deleted_at IS NULL`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestExpandQueryParamsSkipsLiteralsCommentsCasts(t *testing.T) {
	params := map[string]string{"x": "1"}
	in := "SELECT ':x', \":x\", `:x`, 1::int, -- :x\n/* :x */ :x"
	got, err := expandQueryParams(in, params)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT ':x', \":x\", `:x`, 1::int, -- :x\n/* :x */ 1"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestExpandQueryParamsUndefined(t *testing.T) {
	_, err := expandQueryParams(`SELECT :missing`, nil)
	if err == nil || !strings.Contains(err.Error(), "undefined parameter :missing") {
		t.Fatalf("err=%v", err)
	}
}

func TestExpandQueryParamsNoPlaceholders(t *testing.T) {
	got, err := expandQueryParams(`SELECT 1`, map[string]string{"x": "1"})
	if err != nil || got != `SELECT 1` {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestExParamSetListClear(t *testing.T) {
	m := newParamModel()
	m.exParam([]string{"start", "2026-01-01"}, false)
	if m.queryParams["start"] != "2026-01-01" {
		t.Fatalf("set: %v", m.queryParams)
	}
	if !strings.Contains(m.schemaMsg, ":start") {
		t.Fatalf("status: %q", m.schemaMsg)
	}

	m.exParam(nil, false)
	if !strings.Contains(m.schemaMsg, "params:") || !strings.Contains(m.schemaMsg, ":start=") {
		t.Fatalf("list: %q", m.schemaMsg)
	}

	m.exParam([]string{"note", "hello", "world"}, false)
	if m.queryParams["note"] != "hello world" {
		t.Fatalf("joined value: %q", m.queryParams["note"])
	}

	m.exParam([]string{"start"}, true)
	if _, ok := m.queryParams["start"]; ok {
		t.Fatal("expected :start cleared")
	}
	if _, ok := m.queryParams["note"]; !ok {
		t.Fatal("note should remain")
	}

	m.exParam(nil, true)
	if len(m.queryParams) != 0 {
		t.Fatalf("clear all: %v", m.queryParams)
	}
	if m.paramStatusLabel() != "" {
		t.Fatalf("status label should be empty, got %q", m.paramStatusLabel())
	}
}

func TestExParamInvalidName(t *testing.T) {
	m := newParamModel()
	m.exParam([]string{"1bad", "x"}, false)
	if !strings.Contains(m.schemaMsg, "invalid") {
		t.Fatalf("msg=%q", m.schemaMsg)
	}
	if len(m.queryParams) != 0 {
		t.Fatalf("should not set: %v", m.queryParams)
	}
}

func TestParamStatusLabel(t *testing.T) {
	m := newParamModel()
	m.ensureQueryParams()
	m.queryParams["a"] = "1"
	m.queryParams["b"] = "2"
	if got := m.paramStatusLabel(); got != "PARAM 2" {
		t.Fatalf("got %q", got)
	}
}

func TestCompleteParam(t *testing.T) {
	m := newParamModel()
	m.ensureQueryParams()
	m.queryParams["start"] = "1"
	m.queryParams["status"] = "ok"
	got := completeParam(&m, nil, "st")
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	// Second arg: no completion.
	if completeParam(&m, []string{"start"}, "x") != nil {
		t.Fatal("expected nil when name already supplied")
	}
}

func TestRunPageQueryExpandsParams(t *testing.T) {
	// Expansion happens before exec; undefined param must abort without running.
	m := newParamModel()
	m.lastQuery = "SELECT :x"
	m.ensureQueryParams()
	cmd := m.runPageQuery()
	if cmd != nil {
		t.Fatal("expected nil cmd when param missing")
	}
	if !strings.Contains(m.schemaMsg, "undefined parameter :x") {
		t.Fatalf("msg=%q", m.schemaMsg)
	}
	if m.queryRunning {
		t.Fatal("should not mark query running")
	}
}
