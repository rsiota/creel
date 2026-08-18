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

// sqlCompleteScope is the first-cut editor completion context: tables named
// in FROM/JOIN (and aliases), plus whether the next token is a table or a column.
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
func sqlCompleteScopeFrom(prefix string, known map[string]string) sqlCompleteScope {
	scope := sqlCompleteScope{want: wantAny, aliases: map[string]string{}}
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
	lastKW := ""

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
			case "AS":
				expectTable = false
				lastKW = kw
			case "WHERE", "ON", "SET", "HAVING", "AND", "OR", "BY",
				"SELECT", "DELETE", "INSERT", "VALUES",
				"GROUP", "ORDER", "LIMIT", "OFFSET", "RETURNING",
				"UNION", "EXCEPT", "INTERSECT", "WITH":
				expectTable = false
				lastKW = kw
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
			if tok.text == "," && (lastKW == "FROM" || lastKW == "JOIN" || lastKW == "INTO") {
				expectTable = true
				pendingAlias = false
			} else if tok.text != "." {
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

	scope.qualifier = trailingQualifier(tokens)
	if q := scope.qualifier; q != "" {
		scope.want = wantColumn
		if canon, ok := lookupKnown(known, q); ok {
			scope.tables = []string{canon}
		} else if canon, ok := scope.aliases[strings.ToLower(unquoteIdent(q))]; ok {
			scope.tables = []string{canon}
		}
		return scope
	}

	switch lastKW {
	case "FROM", "JOIN", "INTO", "UPDATE", "TABLE":
		scope.want = wantTable
	case "WHERE", "ON", "SET", "HAVING", "AND", "OR", "BY":
		scope.want = wantColumn
	default:
		scope.want = wantAny
	}
	return scope
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
			if s.want == wantTable {
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
