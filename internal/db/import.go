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
	first := r.Errors[0].Err.Error()
	return fmt.Sprintf("Imported %d statements, %d failed → %s (%s)",
		r.Statements, len(r.Errors), filename, truncate(first, 80))
}

// ImportSQL reads a SQL dump from r and executes each statement against the
// database. Statements that fail are collected but do not stop the import —
// the caller receives an ImportResult with the count and any errors.
//
// The parser tracks single-quote strings (with '' escaping), double-quote
// identifiers, line comments (-- ...), block comments (/* ... */), and MySQL
// conditional comments (/*! ... */; which are executable, not real comments).
// On MySQL it also honours backslash escapes inside strings (\'), backtick
// identifiers, and # line comments, matching mysqldump / Sequel Ace dumps.
// Semicolons inside string literals, identifiers, or comments are ignored.
//
// Each statement is executed via Exec. The onProgress callback (if non-nil)
// is called after each statement with the byte offset and total file size,
// enabling streaming progress reporting.
func ImportSQL(r io.Reader, database DB, totalSize int64, onProgress func(bytesRead int64, total int64)) (ImportResult, error) {
	var result ImportResult

	// Pin to a single connection so session-scoped settings set by the dump
	// (MySQL FOREIGN_KEY_CHECKS=0, SQL_MODE, ...) persist across statements
	// instead of being lost across a connection pool.
	session, err := database.Session()
	if err != nil {
		return result, fmt.Errorf("acquire session: %w", err)
	}
	defer session.Close()

	_, isMySQL := database.(*MySQL)
	err = scanSQLStatements(r, isMySQL, func(s string, bytesRead int64) error {
		// Skip comment-only statements. A MySQL conditional comment
		// (/*!...*/) is executable SQL on MySQL (it carries session setup like
		// FOREIGN_KEY_CHECKS=0), so send it through there; on other engines it
		// is an inert comment whose execution can return a nil driver result
		// and panic on RowsAffected, so skip it.
		if !hasExecutableSQL(s) && !(isMySQL && strings.HasPrefix(s, "/*!")) {
			return nil
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
		return nil
	})
	return result, err
}

// scanSQLStatements splits a SQL dump into statements and calls fn for each
// non-empty statement. mysql enables MySQL lexical rules used by mysqldump and
// Sequel Ace: backslash escapes in strings, backtick-quoted identifiers, and
// # line comments.
func scanSQLStatements(r io.Reader, mysql bool, fn func(stmt string, bytesRead int64) error) error {
	scanner := bufio.NewReader(r)
	var stmt strings.Builder
	var inSingleQuote, inDoubleQuote, inBacktick bool
	var inLineComment, inBlockComment, inMySQLComment bool
	var bytesRead int64

	readNext := func() (byte, bool) {
		b, err := scanner.ReadByte()
		if err != nil {
			return 0, false
		}
		bytesRead++
		return b, true
	}

	flush := func() error {
		s := strings.TrimSpace(stmt.String())
		stmt.Reset()
		if s == "" {
			return nil
		}
		return fn(s, bytesRead)
	}

	// writeEscaped consumes the byte after a backslash so \' does not end a
	// string. Returns false on EOF after the backslash.
	writeEscaped := func(backslash byte) bool {
		stmt.WriteByte(backslash)
		next, ok := readNext()
		if !ok {
			return false
		}
		stmt.WriteByte(next)
		return true
	}

	inString := func() bool { return inSingleQuote || inDoubleQuote }

	for {
		b, err := scanner.ReadByte()
		if err == io.EOF {
			return flush()
		}
		if err != nil {
			return err
		}
		bytesRead++

		ch := byte(0)
		if peek, err := scanner.Peek(1); err == nil {
			ch = peek[0]
		}

		// Handle line comments (-- / # to end of line): discard entirely.
		if inLineComment {
			if b == '\n' {
				inLineComment = false
			}
			continue
		}
		// Handle block comments (/* ... */): discard entirely.
		if inBlockComment {
			if b == '*' && ch == '/' {
				readNext()
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
			if mysql && inString() && b == '\\' {
				if !writeEscaped(b) {
					return flush()
				}
				continue
			}
			if b == '\'' && !inDoubleQuote {
				if inSingleQuote && ch == '\'' {
					stmt.WriteByte(b)
					stmt.WriteByte(ch)
					readNext()
					continue
				}
				inSingleQuote = !inSingleQuote
			}
			if b == '"' && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
			stmt.WriteByte(b)
			if b == '*' && ch == '/' && !inSingleQuote && !inDoubleQuote {
				readNext()
				stmt.WriteByte('/')
				inMySQLComment = false
			}
			continue
		}

		// Backtick-quoted identifiers (MySQL): semicolons inside do not end
		// the statement. Doubled backticks are an escaped backtick.
		if inBacktick {
			stmt.WriteByte(b)
			if b == '`' {
				if ch == '`' {
					readNext()
					stmt.WriteByte('`')
					continue
				}
				inBacktick = false
			}
			continue
		}

		// Detect comment / identifier starts when not inside a string.
		if !inString() {
			if mysql && b == '`' {
				inBacktick = true
				stmt.WriteByte(b)
				continue
			}
			if mysql && b == '#' {
				inLineComment = true
				continue
			}
			if b == '-' && ch == '-' {
				readNext()
				inLineComment = true
				continue
			}
			if b == '/' && ch == '*' {
				readNext()
				// /*! starts a MySQL conditional comment: preserve it verbatim.
				peek2, err := scanner.Peek(1)
				if err == nil && peek2[0] == '!' {
					readNext()
					stmt.WriteString("/*!")
					inMySQLComment = true
				} else {
					inBlockComment = true
				}
				continue
			}
		}

		// MySQL backslash escapes inside strings: \' is an apostrophe, not
		// a terminator. Sequel Ace / mysqldump emit this form. SQLite and
		// Postgres dumps use '' instead; treating \ as escape there would
		// swallow the following quote.
		if mysql && inString() && b == '\\' {
			if !writeEscaped(b) {
				return flush()
			}
			continue
		}

		// Track string state.
		if b == '\'' && !inDoubleQuote {
			if inSingleQuote && ch == '\'' {
				stmt.WriteByte(b)
				stmt.WriteByte(ch)
				readNext()
				continue
			}
			inSingleQuote = !inSingleQuote
		}
		if b == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
		}

		// Statement terminator.
		if b == ';' && !inString() && !inBacktick {
			if err := flush(); err != nil {
				return err
			}
			continue
		}

		stmt.WriteByte(b)
	}
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
