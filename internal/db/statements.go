package db

import "strings"

// Statement represents a single SQL statement extracted from a multi-statement
// string. Start and End are byte offsets into the original text (inclusive of
// any trailing semicolon and whitespace before the next statement).
type Statement struct {
	Text  string
	Start int
	End   int
}

// SplitStatements splits a SQL string into individual statements at semicolons
// that are not inside single-quoted strings, double-quoted identifiers, or
// SQL comments (-- and /* */). Empty statements (resulting from trailing
// semicolons or blank segments) are omitted. Each statement's Text has
// surrounding whitespace trimmed but retains internal formatting.
func SplitStatements(sql string) []Statement {
	var statements []Statement
	var current strings.Builder
	start := 0
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false
	flushed := true

	runes := []rune(sql)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inLineComment {
			current.WriteRune(r)
			if r == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			current.WriteRune(r)
			if r == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				current.WriteRune(runes[i+1])
				i++
				inBlockComment = false
			}
			continue
		}

		if inSingleQuote {
			current.WriteRune(r)
			if r == '\'' {
				if i+1 < len(runes) && runes[i+1] == '\'' {
					current.WriteRune(runes[i+1])
					i++
					continue
				}
				inSingleQuote = false
			}
			continue
		}

		if inDoubleQuote {
			current.WriteRune(r)
			if r == '"' {
				inDoubleQuote = false
			}
			continue
		}

		// Not inside any string or comment — check for openings.
		if r == '\'' {
			inSingleQuote = true
			flushed = false
			current.WriteRune(r)
			continue
		}
		if r == '"' {
			inDoubleQuote = true
			flushed = false
			current.WriteRune(r)
			continue
		}
		if r == '-' && i+1 < len(runes) && runes[i+1] == '-' {
			inLineComment = true
			flushed = false
			current.WriteRune(r)
			current.WriteRune(runes[i+1])
			i++
			continue
		}
		if r == '/' && i+1 < len(runes) && runes[i+1] == '*' {
			inBlockComment = true
			flushed = false
			current.WriteRune(r)
			current.WriteRune(runes[i+1])
			i++
			continue
		}

		if r == ';' {
			text := strings.TrimSpace(current.String())
			if text != "" {
				statements = append(statements, Statement{
					Text:  text,
					Start: start,
					End:   i,
				})
			}
			current.Reset()
			flushed = true
			start = i + 1
			continue
		}

		if !flushed || !isSpace(r) {
			flushed = false
		}
		current.WriteRune(r)
	}

	// Handle a trailing statement without a semicolon.
	text := strings.TrimSpace(current.String())
	if text != "" {
		statements = append(statements, Statement{
			Text:  text,
			Start: start,
			End:   len(runes) - 1,
		})
	}

	return statements
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
