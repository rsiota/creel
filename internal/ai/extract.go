package ai

import "strings"

// ExtractSQL pulls the SQL statement out of a model reply that may be wrapped
// in markdown fences and/or surrounded by prose. Models are inconsistent, so
// this is intentionally lenient: it prefers fenced blocks, then falls back to
// the first ;-terminated statement, then to the whole trimmed reply.
//
// It strips leading SQL dialect markers ("sql", "SQL", "postgres", …) from a
// fenced block and trims trailing semicolons only if the statement is a
// single one — leaving the caller (the editor) in control of execution.
func ExtractSQL(reply string) string {
	s := strings.TrimSpace(reply)
	if s == "" {
		return ""
	}

	// Fast path: many well-behaved models return the bare statement.
	if !strings.HasPrefix(s, "```") && !strings.Contains(s, "\n```") {
		if looksLikeSQL(s) {
			return strings.TrimSpace(stripTrailingChat(s))
		}
	}

	// Fenced block(s): take the first code fence as the canonical SQL.
	if sql := firstFencedBlock(s); sql != "" {
		return sql
	}

	// Last resort: scan for the first ;-terminated statement.
	if stmt := firstStatement(s); stmt != "" {
		return stmt
	}
	return ""
}

// firstFencedBlock extracts the contents of the first ``` ... ``` block,
// stripping an optional language tag on the opening fence. Returns "" if no
// complete fence pair is found.
func firstFencedBlock(s string) string {
	const fence = "```"
	start := strings.Index(s, fence)
	if start < 0 {
		return ""
	}
	rest := s[start+len(fence):]
	// Drop a trailing newline and any language tag on the opening fence line.
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	}
	end := strings.Index(rest, fence)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// firstStatement returns the substring up to and including the first ';' that
// ends a line (or the whole string if there is none). Used only when the model
// ignored the "no prose" instruction.
func firstStatement(s string) string {
	// Walk lines; once we find SQL-looking content, read until ';'.
	var b strings.Builder
	seenSQL := false
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if !seenSQL && !looksLikeSQL(t) {
			continue // skip leading prose
		}
		seenSQL = true
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		if idx := strings.IndexByte(t, ';'); idx >= 0 {
			return strings.TrimSpace(b.String())
		}
	}
	if b.Len() > 0 {
		return strings.TrimSpace(b.String())
	}
	return ""
}

// stripTrailingChat removes trailing conversational text after the statement
// (e.g. a model that appends "Let me know if you need changes.").
func stripTrailingChat(s string) string {
	// If there's a ';' followed by more text on following lines, cut there.
	if idx := strings.IndexByte(s, ';'); idx >= 0 {
		head := s[:idx+1]
		tail := strings.TrimSpace(s[idx+1:])
		if tail != "" && !looksLikeSQL(tail) {
			return strings.TrimSpace(head)
		}
	}
	return s
}

// looksLikeSQL is a cheap heuristic: does the line start with a SQL keyword?
// Good enough to separate statements from prose without a full grammar.
func looksLikeSQL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	prefixes := []string{
		"select", "with", "insert", "update", "delete", "create", "alter",
		"drop", "truncate", "show", "describe", "desc ", "explain", "set ",
		"begin", "commit", "rollback", "use ",
	}
	low := strings.ToLower(s)
	for _, p := range prefixes {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	return false
}
