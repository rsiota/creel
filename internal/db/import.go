package db

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ImportError describes a single statement that failed during import.
type ImportError struct {
	Statement string
	Err       error
}

// ImportResult holds the outcome of a SQL import.
type ImportResult struct {
	Statements int
	Errors     []ImportError
}

// Summary returns a human-readable one-line summary for a status bar.
func (r ImportResult) Summary(filename string) string {
	if len(r.Errors) == 0 {
		return fmt.Sprintf("Imported %d statements → %s", r.Statements, filename)
	}
	return fmt.Sprintf("Imported %d statements, %d failed → %s", r.Statements, len(r.Errors), filename)
}

// ImportSQL reads a SQL dump from r and executes each statement against the
// database. Statements that fail are collected but do not stop the import —
// the caller receives an ImportResult with the count and any errors.
//
// The parser tracks single-quote strings (with '' escaping), double-quote
// identifiers, line comments (-- ...), block comments (/* ... */), and MySQL
// conditional comments (/*! ... */; which are executable, not real comments).
// This allows semicolons inside string literals or comments to be ignored.
//
// Each statement is executed via Exec. The onProgress callback (if non-nil)
// is called after each statement with the byte offset and total file size,
// enabling streaming progress reporting.
func ImportSQL(r io.Reader, database DB, totalSize int64, onProgress func(bytesRead int64, total int64)) (ImportResult, error) {
	scanner := bufio.NewReader(r)
	var result ImportResult

	// Pin to a single connection so session-scoped settings set by the dump
	// (MySQL FOREIGN_KEY_CHECKS=0, SQL_MODE, ...) persist across statements
	// instead of being lost across a connection pool.
	session, err := database.Session()
	if err != nil {
		return result, fmt.Errorf("acquire session: %w", err)
	}
	defer session.Close()

	var stmt strings.Builder
	var inSingleQuote, inDoubleQuote bool
	var inLineComment, inBlockComment, inMySQLComment bool
	var bytesRead int64

	flush := func() {
		s := strings.TrimSpace(stmt.String())
		stmt.Reset()
		if s == "" {
			return
		}
		// Skip comment-only statements. A MySQL conditional comment
		// (/*!...*/) is executable SQL on MySQL (it carries session setup like
		// FOREIGN_KEY_CHECKS=0), so send it through there; on other engines it
		// is an inert comment whose execution can return a nil driver result
		// and panic on RowsAffected, so skip it.
		_, isMySQL := database.(*MySQL)
		if !hasExecutableSQL(s) && !(isMySQL && strings.HasPrefix(s, "/*!")) {
			return
		}
		result.Statements++
		if _, err := session.Exec(s); err != nil {
			result.Errors = append(result.Errors, ImportError{
				Statement: truncate(s, 120),
				Err:       err,
			})
		}
		if onProgress != nil {
			onProgress(bytesRead, totalSize)
		}
	}

	for {
		b, err := scanner.ReadByte()
		if err == io.EOF {
			flush()
			break
		}
		if err != nil {
			return result, err
		}
		bytesRead++

		ch := byte(0)
		peek, err := scanner.Peek(1)
		if err == nil {
			ch = peek[0]
		}

		// Handle line comments (-- to end of line): discard entirely.
		if inLineComment {
			if b == '\n' {
				inLineComment = false
			}
			continue
		}
		// Handle block comments (/* ... */): discard entirely.
		if inBlockComment {
			if b == '*' && ch == '/' {
				scanner.ReadByte()
				bytesRead++
				inBlockComment = false
			}
			continue
		}
		// MySQL conditional comments (/*! ... */) are preserved verbatim —
		// including the markers and version number — so MySQL executes them
		// (version-gated) and SQLite ignores them as ordinary comments. Quote
		// state is tracked so a */ inside a string literal doesn't end the
		// comment prematurely; semicolons inside do NOT terminate the statement.
		if inMySQLComment {
			if b == '\'' && !inDoubleQuote {
				if inSingleQuote && ch == '\'' {
					stmt.WriteByte(b)
					stmt.WriteByte(ch)
					scanner.ReadByte()
					bytesRead++
					continue
				}
				inSingleQuote = !inSingleQuote
			}
			if b == '"' && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
			stmt.WriteByte(b)
			if b == '*' && ch == '/' && !inSingleQuote && !inDoubleQuote {
				scanner.ReadByte()
				bytesRead++
				stmt.WriteByte('/')
				inMySQLComment = false
			}
			continue
		}

		// Detect comment starts when not inside a string.
		if !inSingleQuote && !inDoubleQuote {
			if b == '-' && ch == '-' {
				scanner.ReadByte()
				bytesRead++
				inLineComment = true
				continue
			}
			if b == '/' && ch == '*' {
				scanner.ReadByte()
				bytesRead++
				// /*! starts a MySQL conditional comment: preserve it verbatim.
				peek2, err := scanner.Peek(1)
				if err == nil && peek2[0] == '!' {
					scanner.ReadByte()
					bytesRead++
					stmt.WriteString("/*!")
					inMySQLComment = true
				} else {
					inBlockComment = true
				}
				continue
			}
		}

		// Track string state.
		if b == '\'' && !inDoubleQuote {
			// Check for escaped quote ('').
			if inSingleQuote && ch == '\'' {
				stmt.WriteByte(b)
				stmt.WriteByte(ch)
				scanner.ReadByte()
				bytesRead++
				continue
			}
			inSingleQuote = !inSingleQuote
		}
		if b == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
		}

		// Statement terminator.
		if b == ';' && !inSingleQuote && !inDoubleQuote {
			flush()
			continue
		}

		stmt.WriteByte(b)
	}

	return result, nil
}

// hasExecutableSQL reports whether stmt contains any SQL once all comments
// (block /*...*/, MySQL conditional /*!...*/, line -- and #) and surrounding
// whitespace are removed. Comment-only statements are skipped during import.
// The first non-comment, non-whitespace byte short-circuits to true, so string
// literals are never misinterpreted as comments.
func hasExecutableSQL(stmt string) bool {
	for i := 0; i < len(stmt); {
		switch stmt[i] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			i++
		case '/':
			if i+1 >= len(stmt) || stmt[i+1] != '*' {
				return true
			}
			end := strings.Index(stmt[i+2:], "*/")
			if end < 0 {
				return false
			}
			i += end + 4
		case '-':
			if i+1 >= len(stmt) || stmt[i+1] != '-' {
				return true
			}
			nl := strings.IndexByte(stmt[i:], '\n')
			if nl < 0 {
				return false
			}
			i += nl + 1
		case '#':
			nl := strings.IndexByte(stmt[i:], '\n')
			if nl < 0 {
				return false
			}
			i += nl + 1
		default:
			return true
		}
	}
	return false
}

// truncate shortens s to at most n characters, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
