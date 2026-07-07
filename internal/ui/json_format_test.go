package ui

import (
	"strings"
	"testing"
)

func TestFormatJSON_Object(t *testing.T) {
	input := `{"name":"alice","age":30}`
	got, ok := formatJSON(input)
	if !ok {
		t.Fatal("expected ok=true for valid JSON object")
	}
	if !strings.Contains(got, `"name"`) || !strings.Contains(got, `"alice"`) {
		t.Errorf("output missing expected keys: %s", got)
	}
	// Should be indented (pretty-printed)
	if !strings.Contains(got, "\n") {
		t.Error("pretty-printed JSON should contain newlines")
	}
}

func TestFormatJSON_Array(t *testing.T) {
	input := `[1,2,3]`
	got, ok := formatJSON(input)
	if !ok {
		t.Fatal("expected ok=true for valid JSON array")
	}
	if !strings.Contains(got, "1") || !strings.Contains(got, "3") {
		t.Errorf("output missing values: %s", got)
	}
}

func TestFormatJSON_Scalar(t *testing.T) {
	// Scalars should not be reformatted
	if _, ok := formatJSON(`"hello"`); ok {
		t.Error("string scalar should return ok=false")
	}
	if _, ok := formatJSON(`42`); ok {
		t.Error("number scalar should return ok=false")
	}
	if _, ok := formatJSON(`true`); ok {
		t.Error("boolean scalar should return ok=false")
	}
	if _, ok := formatJSON(`null`); ok {
		t.Error("null should return ok=false")
	}
}

func TestFormatJSON_Invalid(t *testing.T) {
	if _, ok := formatJSON(`{invalid}`); ok {
		t.Error("invalid JSON should return ok=false")
	}
	if _, ok := formatJSON(`{"unclosed":`); ok {
		t.Error("unclosed JSON should return ok=false")
	}
}

func TestFormatJSON_Empty(t *testing.T) {
	if _, ok := formatJSON(``); ok {
		t.Error("empty string should return ok=false")
	}
	if _, ok := formatJSON(`   `); ok {
		t.Error("whitespace-only should return ok=false")
	}
}

func TestFormatJSON_Nested(t *testing.T) {
	input := `{"user":{"name":"bob","tags":["a","b"]},"count":2}`
	got, ok := formatJSON(input)
	if !ok {
		t.Fatal("expected ok=true for nested JSON")
	}
	if !strings.Contains(got, `"user"`) {
		t.Errorf("missing nested key: %s", got)
	}
}

func TestFormatJSON_TrimsWhitespace(t *testing.T) {
	input := `  {"x": 1}  `
	_, ok := formatJSON(input)
	if !ok {
		t.Error("should handle surrounding whitespace")
	}
}

func TestCompactJSON_Basic(t *testing.T) {
	input := `{"name": "alice", "age": 30}`
	got, ok := compactJSON(input)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if strings.Contains(got, "\n") {
		t.Errorf("compact JSON should not contain newlines: %s", got)
	}
}

func TestCompactJSON_Scalar(t *testing.T) {
	// compactJSON accepts any valid JSON (including scalars)
	got, ok := compactJSON(`"hello"`)
	if !ok {
		t.Fatal("string scalar should be valid JSON")
	}
	if got != `"hello"` {
		t.Errorf("expected \"hello\", got %q", got)
	}
}

func TestCompactJSON_Invalid(t *testing.T) {
	if _, ok := compactJSON(`{invalid}`); ok {
		t.Error("invalid JSON should return ok=false")
	}
}

func TestCompactJSON_Empty(t *testing.T) {
	if _, ok := compactJSON(``); ok {
		t.Error("empty string should return ok=false")
	}
}

func TestHighlightJSON_Object(t *testing.T) {
	// Just ensure it doesn't crash and returns non-empty output for valid JSON
	input := `{"key": "value", "num": 42, "flag": true, "nothing": null}`
	got := highlightJSON(input)
	if got == "" {
		t.Error("highlightJSON should return non-empty output")
	}
}

func TestHighlightJSON_PreservesStructure(t *testing.T) {
	// The highlighted output should still contain the literal values
	input := `{"name":"alice","count":3}`
	got := highlightJSON(input)
	if !strings.Contains(got, "alice") {
		t.Error("highlighted output should contain 'alice'")
	}
	if !strings.Contains(got, "3") {
		t.Error("highlighted output should contain '3'")
	}
}

func TestHighlightJSON_Empty(t *testing.T) {
	got := highlightJSON("")
	if got != "" {
		t.Error("empty input should return empty output")
	}
}
