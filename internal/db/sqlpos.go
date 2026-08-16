package db

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

// PageWrapPrefix is the fixed prefix runPageQuery adds around simple SELECTs.
// Postgres Position is relative to the wrapped string; LocateQueryError
// subtracts this to map back onto the user's query.
const PageWrapPrefix = "SELECT * FROM ("

var (
	reMySQLNearLine = regexp.MustCompile(`(?i)near '([^']*)' at line (\d+)`)
	reMySQLAtLine   = regexp.MustCompile(`(?i)\bat line (\d+)\b`)
	rePGNear        = regexp.MustCompile(`(?i)at or near "([^"]*)"`)
	rePGLine        = regexp.MustCompile(`(?i)\bLINE (\d+):`)
	reSQLiteNear    = regexp.MustCompile(`(?i)near "([^"]*)"`)
	reGenericNear   = regexp.MustCompile(`(?i)near ['"]([^'"]+)['"]`)
)

// QueryErrorPos is a location inside a user SQL string derived from a driver
// error. Line and Col are 0-based. OK is false when nothing useful was found.
type QueryErrorPos struct {
	Line  int
	Col   int
	Token string
	OK    bool
}

// LocateQueryError maps err onto userQuery (the statement the user wrote).
// execQuery is what was actually sent (may be the pagination wrapper); pass
// the same string as userQuery when they match.
func LocateQueryError(err error, userQuery, execQuery string) QueryErrorPos {
	if err == nil || strings.TrimSpace(userQuery) == "" {
		return QueryErrorPos{}
	}
	userQuery = strings.TrimRight(userQuery, ";")
	if execQuery == "" {
		execQuery = userQuery
	}

	if pos := locatePgPosition(err, userQuery, execQuery); pos.OK {
		return pos
	}

	msg := err.Error()
	token, line1 := parseErrorHints(err, msg)
	token = normalizeNearToken(token, userQuery)

	if line1 > 0 || token != "" {
		return locateByLineToken(userQuery, line1, token)
	}
	return QueryErrorPos{}
}

func locatePgPosition(err error, userQuery, execQuery string) QueryErrorPos {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Position <= 0 {
		return QueryErrorPos{}
	}
	// Position is 1-based into the query string sent to the server.
	off := int(pgErr.Position) - 1
	off = mapExecOffsetToUser(off, userQuery, execQuery)
	if off < 0 {
		off = 0
	}
	if off > len(userQuery) {
		off = len(userQuery)
	}
	line, col := offsetToLineCol(userQuery, off)
	token := ""
	if m := rePGNear.FindStringSubmatch(pgErr.Message); len(m) == 2 {
		token = m[1]
	}
	pos := QueryErrorPos{Line: line, Col: col, Token: token, OK: true}
	// If the mapped offset landed at the start (common when Position points
	// into the pagination wrapper) but the message names a token elsewhere,
	// prefer the token location.
	if token != "" && off == 0 {
		if refined := locateByLineToken(userQuery, 0, token); refined.OK && (refined.Line != 0 || refined.Col != 0) {
			return refined
		}
	}
	return pos
}

// normalizeNearToken strips pagination-wrap leakage from driver "near …"
// snippets and shrinks the token until it appears in userQuery.
func normalizeNearToken(token, userQuery string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	// MySQL often reports near 'FORM users) AS _creel_page LIMIT …' when the
	// SELECT was wrapped for pagination. Cut at the wrap suffix first.
	if i := strings.Index(token, ") AS _creel_page"); i >= 0 {
		token = strings.TrimSpace(token[:i])
	}
	if token == "" {
		return ""
	}
	if strings.Contains(userQuery, token) {
		return token
	}
	// Shrink from the end until a prefix matches the user's SQL.
	runes := []rune(token)
	for n := len(runes) - 1; n >= 1; n-- {
		cand := strings.TrimSpace(string(runes[:n]))
		if cand == "" {
			continue
		}
		if strings.Contains(userQuery, cand) {
			return cand
		}
	}
	return token // keep original for status text even if unplaced
}

// mapExecOffsetToUser converts a 0-based byte offset in execQuery to one in
// userQuery, undoing the SELECT * FROM (…) pagination wrap when present.
func mapExecOffsetToUser(off int, userQuery, execQuery string) int {
	if execQuery == userQuery {
		return off
	}
	if strings.HasPrefix(execQuery, PageWrapPrefix) &&
		strings.Contains(execQuery, ") AS _creel_page") &&
		strings.HasPrefix(execQuery[len(PageWrapPrefix):], userQuery) {
		inner := off - len(PageWrapPrefix)
		if inner < 0 {
			return 0
		}
		if inner > len(userQuery) {
			return len(userQuery)
		}
		return inner
	}
	// Unknown wrap — fall back to the raw offset clamped to userQuery.
	if off > len(userQuery) {
		return len(userQuery)
	}
	return off
}

func parseErrorHints(err error, msg string) (token string, line1 int) {
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		msg = myErr.Message
	}

	if m := reMySQLNearLine.FindStringSubmatch(msg); len(m) == 3 {
		token = m[1]
		line1 = atoiPositive(m[2])
		return token, line1
	}
	if m := rePGNear.FindStringSubmatch(msg); len(m) == 2 {
		token = m[1]
	}
	if m := rePGLine.FindStringSubmatch(msg); len(m) == 2 {
		line1 = atoiPositive(m[1])
	}
	if token == "" {
		if m := reSQLiteNear.FindStringSubmatch(msg); len(m) == 2 {
			token = m[1]
		} else if m := reGenericNear.FindStringSubmatch(msg); len(m) == 2 {
			token = m[1]
		}
	}
	if line1 == 0 {
		if m := reMySQLAtLine.FindStringSubmatch(msg); len(m) == 2 {
			line1 = atoiPositive(m[1])
		}
	}
	return token, line1
}

func locateByLineToken(query string, line1 int, token string) QueryErrorPos {
	lines := strings.Split(query, "\n")
	if line1 > 0 {
		idx := line1 - 1
		if idx >= len(lines) {
			idx = len(lines) - 1
		}
		if idx < 0 {
			idx = 0
		}
		if token != "" {
			if i := strings.Index(lines[idx], token); i >= 0 {
				col := utf8.RuneCountInString(lines[idx][:i])
				return QueryErrorPos{Line: idx, Col: col, Token: token, OK: true}
			}
			// Line hint is often wrong for wrapped SELECTs (MySQL always says
			// "at line 1"). Fall through to a whole-query token search rather
			// than parking the caret at column 0 of that line.
			if i := strings.Index(query, token); i >= 0 {
				line, col := offsetToLineCol(query, i)
				return QueryErrorPos{Line: line, Col: col, Token: token, OK: true}
			}
			// Token still not found — line only (better than a false OK at col 0
			// claiming a token we couldn't place).
			return QueryErrorPos{Line: idx, Col: 0, Token: token, OK: true}
		}
		return QueryErrorPos{Line: idx, Col: 0, OK: true}
	}
	if token == "" {
		return QueryErrorPos{}
	}
	// Search the whole query for the first occurrence of the token.
	if i := strings.Index(query, token); i >= 0 {
		line, col := offsetToLineCol(query, i)
		return QueryErrorPos{Line: line, Col: col, Token: token, OK: true}
	}
	return QueryErrorPos{}
}

func offsetToLineCol(s string, byteOff int) (line, col int) {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff > len(s) {
		byteOff = len(s)
	}
	prefix := s[:byteOff]
	line = strings.Count(prefix, "\n")
	if i := strings.LastIndex(prefix, "\n"); i >= 0 {
		col = utf8.RuneCountInString(prefix[i+1:])
	} else {
		col = utf8.RuneCountInString(prefix)
	}
	return line, col
}

func atoiPositive(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
