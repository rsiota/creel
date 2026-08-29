package ui

import (
	"strings"
	"unicode"
)

// sqlCompleteWant is what the cursor is typing toward.
type sqlCompleteWant int

const (
	wantAny sqlCompleteWant = iota
	wantTable
	wantColumn
)

// sqlCompleteScope is the editor completion context: tables named in FROM/JOIN
// /UPDATE/INTO (and aliases), plus whether the next token is a table or a
// column. Before any table is known, wantAny suppresses columns so the
// SELECT-list / start-of-query popup stays keywords (+ tables). Once FROM
// tables are visible in the statement (including after the cursor), SELECT-list
// / INSERT (…) / SET completion prefers those columns.
type sqlCompleteScope struct {
	want      sqlCompleteWant
	tables    []string          // canonical table names in the FROM list
	aliases   map[string]string // lower(alias) → canonical table
	qualifier string            // ident before a trailing "."; empty if none
}

func knownTablesFrom(all []completionItem) map[string]string {
	out := map[string]string{}
	for _, it := range all {
		if it.kind == kindTable && it.text != "" {
			out[strings.ToLower(it.text)] = it.text
		}
	}
	return out
}

func lookupKnown(known map[string]string, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	canon, ok := known[strings.ToLower(unquoteIdent(name))]
	return canon, ok
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if n := len(s); n >= 2 {
		if (s[0] == '"' && s[n-1] == '"') || (s[0] == '`' && s[n-1] == '`') {
			return s[1 : n-1]
		}
	}
	return s
}

func isSQLIdent(s string) bool {
	s = unquoteIdent(s)
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
			return false
		}
	}
	return true
}

// sqlCompleteScopeFrom inspects SQL to the left of the token being typed.
// Prefer sqlCompleteScopeFromQuery when the full statement is available so a
// SELECT list can see FROM tables that appear after the cursor.
func sqlCompleteScopeFrom(prefix string, known map[string]string) sqlCompleteScope {
	return sqlCompleteScopeFromQuery(prefix, "", known)
}

// sqlCompleteScopeFromQuery is like sqlCompleteScopeFrom, but statement (the
// statement under the cursor) is used to discover FROM/JOIN tables when the
// cursor sits in a SELECT list before those clauses.
func sqlCompleteScopeFromQuery(prefix, statement string, known map[string]string) sqlCompleteScope {
	scope, lastKW, inInsertCols := scanSQLComplete(prefix, known)

	scope.qualifier = trailingQualifier(tokenizeSQL(prefix))
	if q := scope.qualifier; q != "" {
		scope.want = wantColumn
		if canon, ok := lookupKnown(known, q); ok {
			scope.tables = []string{canon}
		} else if canon, ok := scope.aliases[strings.ToLower(unquoteIdent(q))]; ok {
			scope.tables = []string{canon}
		}
		return scope
	}

	switch {
	case inInsertCols:
		scope.want = wantColumn
	case lastKW == "FROM" || lastKW == "JOIN" || lastKW == "INTO" || lastKW == "UPDATE" || lastKW == "TABLE":
		scope.want = wantTable
	case lastKW == "WHERE" || lastKW == "ON" || lastKW == "SET" || lastKW == "HAVING" ||
		lastKW == "AND" || lastKW == "OR" || lastKW == "BY":
		scope.want = wantColumn
	case lastKW == "SELECT" || lastKW == "DISTINCT":
		// SELECT list: pull FROM/JOIN tables from the full statement when the
		// cursor sits before them (prefix alone cannot see them).
		if len(scope.tables) == 0 && strings.TrimSpace(statement) != "" {
			fromStmt, _, _ := scanSQLComplete(statement, known)
			scope.tables = fromStmt.tables
			scope.aliases = fromStmt.aliases
		}
		if len(scope.tables) > 0 {
			scope.want = wantColumn
		} else {
			scope.want = wantAny
		}
	default:
		scope.want = wantAny
	}
	return scope
}

// scanSQLComplete tokenizes prefix and collects FROM/JOIN/UPDATE/INTO tables,
// aliases, and whether the cursor is inside an INSERT column list.
func scanSQLComplete(prefix string, known map[string]string) (scope sqlCompleteScope, lastKW string, inInsertCols bool) {
	scope = sqlCompleteScope{want: wantAny, aliases: map[string]string{}}
	seen := map[string]bool{}
	addTable := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		scope.tables = append(scope.tables, name)
	}

	tokens := tokenizeSQL(prefix)
	expectTable := false
	pendingAlias := false
	lastTable := ""

	flushAlias := func(raw string) {
		ident := unquoteIdent(raw)
		if lastTable == "" || ident == "" {
			return
		}
		scope.aliases[strings.ToLower(ident)] = lastTable
	}

	for _, tok := range tokens {
		if tok.kind == tokenComment || strings.TrimSpace(tok.text) == "" {
			continue
		}
		if tok.kind == tokenOperator && tok.text == "." {
			pendingAlias = false
			continue
		}
		if tok.kind == tokenKeyword {
			kw := strings.ToUpper(tok.text)
			pendingAlias = false
			switch kw {
			case "FROM", "JOIN", "INTO", "UPDATE":
				expectTable = true
				lastKW = kw
				if kw != "INTO" {
					inInsertCols = false
				}
			case "AS":
				expectTable = false
				lastKW = kw
			case "VALUES":
				expectTable = false
				lastKW = kw
				inInsertCols = false
			case "WHERE", "ON", "SET", "HAVING", "AND", "OR", "BY",
				"SELECT", "DELETE", "INSERT",
				"GROUP", "ORDER", "LIMIT", "OFFSET", "RETURNING",
				"UNION", "EXCEPT", "INTERSECT", "WITH", "DISTINCT":
				expectTable = false
				lastKW = kw
				if kw != "INSERT" {
					inInsertCols = false
				}
			default:
				if kw != "INNER" && kw != "LEFT" && kw != "RIGHT" &&
					kw != "OUTER" && kw != "FULL" && kw != "CROSS" &&
					kw != "NATURAL" {
					expectTable = false
					lastKW = kw
				}
			}
			continue
		}
		if tok.kind == tokenOperator {
			switch tok.text {
			case "(":
				if lastKW == "INTO" && lastTable != "" {
					inInsertCols = true
				}
				pendingAlias = false
			case ")":
				inInsertCols = false
				pendingAlias = false
			case ",":
				if lastKW == "FROM" || lastKW == "JOIN" || lastKW == "INTO" {
					expectTable = true
					pendingAlias = false
				}
			case ".":
				// kept for completeness; handled above too
			default:
				pendingAlias = false
			}
			continue
		}

		raw := strings.TrimSpace(tok.text)
		if !isSQLIdent(raw) {
			pendingAlias = false
			continue
		}
		if lastKW == "AS" && lastTable != "" {
			flushAlias(raw)
			lastKW = ""
			continue
		}
		if canon, ok := lookupKnown(known, raw); ok {
			addTable(canon)
			lastTable = canon
			expectTable = false
			pendingAlias = true
			continue
		}
		if pendingAlias {
			flushAlias(raw)
			pendingAlias = false
			continue
		}
		if expectTable {
			// Unknown ident after FROM — might be a schema qualifier; wait for
			// schema.table. If it isn't, we still shouldn't treat it as a column.
			pendingAlias = false
			continue
		}
	}
	return scope, lastKW, inInsertCols
}

func trailingQualifier(tokens []sqlToken) string {
	// Walk back over trailing space and find ident "."
	i := len(tokens) - 1
	for i >= 0 && strings.TrimSpace(tokens[i].text) == "" {
		i--
	}
	if i < 0 || tokens[i].kind != tokenOperator || tokens[i].text != "." {
		return ""
	}
	i--
	for i >= 0 && strings.TrimSpace(tokens[i].text) == "" {
		i--
	}
	if i < 0 {
		return ""
	}
	raw := strings.TrimSpace(tokens[i].text)
	if !isSQLIdent(raw) {
		return ""
	}
	return unquoteIdent(raw)
}

func (s sqlCompleteScope) filter(all []completionItem) []completionItem {
	inTable := map[string]bool{}
	for _, t := range s.tables {
		inTable[strings.ToLower(t)] = true
	}
	restrictCols := s.want == wantColumn && len(s.tables) > 0
	scopedCols := (s.want == wantAny || s.want == wantColumn) && len(s.tables) > 0
	// Start of query / SELECT list: no FROM tables yet → hide columns.
	hideUnscopedCols := s.want == wantAny && len(s.tables) == 0

	var out []completionItem
	seenCol := map[string]bool{}
	for _, it := range all {
		switch it.kind {
		case kindKeyword:
			if s.qualifier != "" {
				continue
			}
			out = append(out, it)
		case kindTable:
			if s.want == wantColumn {
				continue
			}
			out = append(out, it)
		case kindColumn:
			if s.want == wantTable || hideUnscopedCols {
				continue
			}
			if restrictCols || scopedCols {
				if !inTable[strings.ToLower(it.table)] {
					continue
				}
			}
			key := strings.ToLower(it.text)
			if seenCol[key] {
				continue
			}
			seenCol[key] = true
			out = append(out, it)
		}
	}
	return out
}
