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
		// Skip MySQL session-save/restore wrappers that are meaningless when
		// importing (they reference @@ session variables from the dump host).
		if isSessionWrapper(s) {
			return
		}
		result.Statements++
		if _, err := database.Exec(s); err != nil {
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
		// MySQL conditional comments (/*! ... */) are executable — treat the
		// interior as regular SQL, only the opening and closing markers are
		// stripped (they pass through as comment markers that MySQL understands).
		if inMySQLComment {
			if b == '*' && ch == '/' {
				scanner.ReadByte()
				bytesRead++
				inMySQLComment = false
				continue
			}
			// Everything inside is normal SQL, so handle quotes and semicolons.
			if b == '\'' && !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
			if b == '"' && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
			if b == ';' && !inSingleQuote && !inDoubleQuote {
				flush()
				continue
			}
			stmt.WriteByte(b)
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
				// Check if this is /*! (MySQL conditional comment).
				peek2, err := scanner.Peek(1)
				if err == nil && peek2[0] == '!' {
					scanner.ReadByte()
					bytesRead++
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

// isSessionWrapper reports whether a statement is a mysqldump session
// save/restore wrapper (e.g. "/*!40101 SET @OLD_SQL_MODE=@OLD_SQL_MODE */")
// that should be skipped during import. These reference session variables
// from the dump host and have no meaning on restore.
func isSessionWrapper(stmt string) bool {
	upper := strings.ToUpper(stmt)
	// Skip SET @OLD_... = ... wrappers.
	if strings.HasPrefix(upper, "/*!") && strings.Contains(upper, "SET @OLD_") {
		return true
	}
	// Skip SET NAMES on non-MySQL... actually let it through, harmless.
	return false
}

// truncate shortens s to at most n characters, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
