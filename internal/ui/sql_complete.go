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
// SELECT-list / start-of-query popup stays keywords (+ tables + schemas).
// Qualifiers: alias/table. → columns; schema. → tables; schema.table. → columns.
type sqlCompleteScope struct {
	want         sqlCompleteWant
	tables       []string          // FROM-list tables; may be "schema.table"
	aliases      map[string]string // lower(alias) → canonical table
	qualifier    string            // single ident before trailing "."; empty if none
	qualParts    []string          // 1–2 idents before trailing "."
	schemaFilter string            // when set, only tables in this schema
	activeSchema string            // current connection schema (bare tables live here)
}

func knownTablesFrom(all []completionItem) map[string]string {
	out := map[string]string{}
	for _, it := range all {
		// Active-schema tables only (schema == ""). Qualified cache entries use schema!= "".
		if it.kind == kindTable && it.text != "" && it.schema == "" {
			out[strings.ToLower(it.text)] = it.text
		}
	}
	return out
}

func knownSchemasFrom(all []completionItem) map[string]string {
	out := map[string]string{}
	for _, it := range all {
		if it.kind == kindSchema && it.text != "" {
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
	return sqlCompleteScopeFromQuery(prefix, "", known, nil, "")
}

// sqlCompleteScopeFromQuery derives completion intent from prefix, optionally
// enriching SELECT lists from statement, and resolving schema qualifiers via
// knownSchemas / activeSchema.
func sqlCompleteScopeFromQuery(prefix, statement string, known, schemas map[string]string, activeSchema string) sqlCompleteScope {
	scope, lastKW, inInsertCols := scanSQLComplete(prefix, known, schemas)
	scope.activeSchema = activeSchema

	parts := trailingQualifierParts(tokenizeSQL(prefix))
	scope.qualParts = parts
	if len(parts) == 1 {
		scope.qualifier = parts[0]
	}
	if len(parts) == 2 {
		scope.want = wantColumn
		scope.tables = []string{parts[0] + "." + parts[1]}
		scope.qualifier = parts[1]
		return scope
	}
	if len(parts) == 1 {
		q := parts[0]
		if canon, ok := lookupKnown(known, q); ok {
			scope.want = wantColumn
			scope.tables = []string{canon}
			return scope
		}
		if canon, ok := scope.aliases[strings.ToLower(unquoteIdent(q))]; ok {
			scope.want = wantColumn
			scope.tables = []string{canon}
			return scope
		}
		if canon, ok := lookupKnown(schemas, q); ok {
			scope.want = wantTable
			scope.schemaFilter = canon
			return scope
		}
		// Unknown qualifier — still treat as column context (empty tables).
		scope.want = wantColumn
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
		if len(scope.tables) == 0 && strings.TrimSpace(statement) != "" {
			fromStmt, _, _ := scanSQLComplete(statement, known, schemas)
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
// aliases, and whether the cursor is inside an INSERT column list. schema.table
// refs after FROM are recorded as "schema.table" when the first ident is a
// known schema.
func scanSQLComplete(prefix string, known, schemas map[string]string) (scope sqlCompleteScope, lastKW string, inInsertCols bool) {
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
	pendingSchema := ""
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
			pendingSchema = ""
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
				pendingSchema = ""
			case ")":
				inInsertCols = false
				pendingAlias = false
				pendingSchema = ""
			case ",":
				if lastKW == "FROM" || lastKW == "JOIN" || lastKW == "INTO" {
					expectTable = true
					pendingAlias = false
				}
				pendingSchema = ""
			case ".":
				// pendingSchema retained for schema.table
			default:
				pendingAlias = false
				pendingSchema = ""
			}
			continue
		}

		raw := strings.TrimSpace(tok.text)
		if !isSQLIdent(raw) {
			pendingAlias = false
			pendingSchema = ""
			continue
		}
		if lastKW == "AS" && lastTable != "" {
			flushAlias(raw)
			lastKW = ""
			pendingSchema = ""
			continue
		}
		if pendingSchema != "" {
			qual := pendingSchema + "." + unquoteIdent(raw)
			addTable(qual)
			lastTable = qual
			expectTable = false
			pendingAlias = true
			pendingSchema = ""
			continue
		}
		if canon, ok := lookupKnown(known, raw); ok {
			addTable(canon)
			lastTable = canon
			expectTable = false
			pendingAlias = true
			pendingSchema = ""
			continue
		}
		if pendingAlias {
			flushAlias(raw)
			pendingAlias = false
			pendingSchema = ""
			continue
		}
		if expectTable {
			if schema, ok := lookupKnown(schemas, raw); ok {
				pendingSchema = schema
				pendingAlias = false
				continue
			}
			pendingAlias = false
			continue
		}
	}
	return scope, lastKW, inInsertCols
}

// trailingQualifierParts returns the 1–2 idents before a trailing ".".
// "u." → ["u"]; "public.users." → ["public","users"].
func trailingQualifierParts(tokens []sqlToken) []string {
	i := len(tokens) - 1
	for i >= 0 && strings.TrimSpace(tokens[i].text) == "" {
		i--
	}
	if i < 0 || tokens[i].kind != tokenOperator || tokens[i].text != "." {
		return nil
	}
	var parts []string
	for len(parts) < 2 {
		i--
		for i >= 0 && strings.TrimSpace(tokens[i].text) == "" {
			i--
		}
		if i < 0 {
			break
		}
		raw := strings.TrimSpace(tokens[i].text)
		if !isSQLIdent(raw) {
			break
		}
		parts = append([]string{unquoteIdent(raw)}, parts...)
		j := i - 1
		for j >= 0 && strings.TrimSpace(tokens[j].text) == "" {
			j--
		}
		if j < 0 || tokens[j].kind != tokenOperator || tokens[j].text != "." {
			break
		}
		i = j
	}
	return parts
}

func trailingQualifier(tokens []sqlToken) string {
	parts := trailingQualifierParts(tokens)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func (s sqlCompleteScope) tableMatchesSchema(it completionItem) bool {
	if s.schemaFilter == "" {
		return true
	}
	if it.schema != "" {
		return strings.EqualFold(it.schema, s.schemaFilter)
	}
	// Bare active-schema tables.
	return s.activeSchema != "" && strings.EqualFold(s.activeSchema, s.schemaFilter)
}

func (s sqlCompleteScope) filter(all []completionItem) []completionItem {
	inTable := map[string]bool{}
	for _, t := range s.tables {
		inTable[strings.ToLower(t)] = true
	}
	restrictCols := s.want == wantColumn && len(s.tables) > 0
	scopedCols := (s.want == wantAny || s.want == wantColumn) && len(s.tables) > 0
	hideUnscopedCols := s.want == wantAny && len(s.tables) == 0
	hasQualifier := len(s.qualParts) > 0 || s.qualifier != ""

	var out []completionItem
	seenCol := map[string]bool{}
	for _, it := range all {
		switch it.kind {
		case kindKeyword:
			if hasQualifier {
				continue
			}
			out = append(out, it)
		case kindSchema:
			if hasQualifier || s.want == wantColumn {
				continue
			}
			out = append(out, it)
		case kindTable:
			if s.want == wantColumn {
				continue
			}
			if s.schemaFilter != "" {
				if !s.tableMatchesSchema(it) {
					continue
				}
			} else if it.schema != "" {
				// Other-schema tables only appear after schema.
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
