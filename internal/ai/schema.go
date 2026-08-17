package ai

import (
	"fmt"
	"strings"
	"unicode"
)

// SchemaIntrospector is the slice of db.DB / db.Connection the AI package
// needs to describe a database to the model. Defining it locally keeps the
// ai package free of an internal/db import and trivial to fake in tests.
type SchemaIntrospector interface {
	Tables() ([]string, error)
	TableSchema(table string) ([]Column, error)
	PrimaryKeys(table string) ([]string, error)
	ForeignKeys(table string) ([]ForeignKey, error)
}

// Column mirrors db.Column for the introspector interface (name + type only).
type Column struct {
	Name string
	Type string
}

// ForeignKey mirrors db.ForeignKey for the introspector interface.
type ForeignKey struct {
	Column    string
	RefTable  string
	RefColumn string
}

// Caps that protect the token budget on large databases. Once exceeded, the
// context is truncated and annotated so the model knows the schema is partial.
const (
	maxTablesInContext = 100
)

// SchemaContext renders the connected database as compact pseudo-DDL for the
// model's system prompt. Each table is one block: its columns (with PK / FK
// annotations inline as SQL comments) so the model sees names, types, and
// relationships without extra round-trips. Tables with unreadable schemas are
// listed by name only so the model at least knows they exist.
//
// focus, when it names one or more known tables, restricts the dump to those
// tables plus their direct FK neighbours (outbound references and inbound
// referrers) — cheaper and more accurate than dumping the first 100 tables of
// a large schema. Unknown focus names are ignored; if nothing matches, the
// whole schema is used (still capped at maxTablesInContext). Omitted tables
// are listed by name so the model knows they exist.
//
// The returned string is prefixed with a short legend so the model can parse
// the inline annotations.
func SchemaContext(conn SchemaIntrospector, focus ...string) (string, error) {
	if conn == nil {
		return "", nil
	}
	tables, err := conn.Tables()
	if err != nil {
		return "", fmt.Errorf("listing tables: %w", err)
	}

	chosen, neighbourhood, omitted := pickSchemaTables(conn, tables, focus)

	var b strings.Builder
	b.WriteString("-- Tables, columns, and relationships. PK marks a primary key;\n")
	b.WriteString("-- FK: <col> -> <table>.<col> marks a foreign key.\n")
	if neighbourhood {
		fmt.Fprintf(&b, "-- Restricted to the focused table(s) and their foreign-key neighbours (%d of %d tables).\n", len(chosen), len(tables))
	}
	b.WriteByte('\n')

	for _, t := range chosen {
		writeTableContext(&b, conn, t)
	}
	if len(omitted) > 0 {
		fmt.Fprintf(&b, "-- Other tables (columns omitted): %s\n", strings.Join(omitted, ", "))
	}
	return b.String(), nil
}

// pickSchemaTables chooses which tables to expand into DDL. When focus matches
// known tables, the result is their FK neighbourhood (capped); otherwise the
// full list (capped). omitted is the remaining names, in Tables() order.
func pickSchemaTables(conn SchemaIntrospector, tables []string, focus []string) (chosen []string, neighbourhood bool, omitted []string) {
	set := neighbourhoodSet(conn, tables, focus)
	if len(set) == 0 {
		if len(tables) <= maxTablesInContext {
			return tables, false, nil
		}
		return tables[:maxTablesInContext], false, tables[maxTablesInContext:]
	}
	for _, t := range tables {
		if set[t] {
			chosen = append(chosen, t)
		} else {
			omitted = append(omitted, t)
		}
	}
	if len(chosen) > maxTablesInContext {
		omitted = append(append([]string{}, chosen[maxTablesInContext:]...), omitted...)
		chosen = chosen[:maxTablesInContext]
	}
	return chosen, true, omitted
}

// neighbourhoodSet is the focused tables plus one hop of FK neighbours
// (tables they reference, and tables that reference them). Empty when focus
// matches nothing in tables.
func neighbourhoodSet(conn SchemaIntrospector, tables []string, focus []string) map[string]bool {
	seeds := canonicalFocus(tables, focus)
	if len(seeds) == 0 {
		return nil
	}
	base := make(map[string]bool, len(tables))
	for _, t := range tables {
		base[t] = true
	}
	set := make(map[string]bool, len(seeds)+8)
	for name := range seeds {
		set[name] = true
		for _, fk := range safeForeignKeys(conn, name) {
			if base[fk.RefTable] {
				set[fk.RefTable] = true
			} else if canon := lookupTable(tables, fk.RefTable); canon != "" && base[canon] {
				set[canon] = true
			}
		}
	}
	// Inbound: any table whose FK points at a seed.
	for _, child := range tables {
		if set[child] {
			continue
		}
		for _, fk := range safeForeignKeys(conn, child) {
			if seedNamed(seeds, fk.RefTable) {
				set[child] = true
				break
			}
		}
	}
	return set
}

func seedNamed(seeds map[string]bool, name string) bool {
	if seeds[name] {
		return true
	}
	for s := range seeds {
		if strings.EqualFold(s, name) {
			return true
		}
	}
	return false
}

func canonicalFocus(tables, focus []string) map[string]bool {
	seeds := make(map[string]bool)
	for _, f := range focus {
		if t := lookupTable(tables, f); t != "" {
			seeds[t] = true
		}
	}
	return seeds
}

func lookupTable(tables []string, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, t := range tables {
		if strings.EqualFold(t, name) {
			return t
		}
	}
	return ""
}

// MentionedTables returns the names in tables that appear as identifiers in
// sql (case-insensitive, bounded by non-identifier characters). Used to seed
// SchemaContext from a question or a failed statement so :ai / :aifix still
// see tables the user named even when they are not the grid's current table.
func MentionedTables(sql string, tables []string) []string {
	if sql == "" || len(tables) == 0 {
		return nil
	}
	var out []string
	for _, t := range tables {
		if identInText(sql, t) {
			out = append(out, t)
		}
	}
	return out
}

func identInText(text, ident string) bool {
	if ident == "" {
		return false
	}
	lower := strings.ToLower(text)
	want := strings.ToLower(ident)
	for i := 0; i <= len(lower)-len(want); i++ {
		if lower[i:i+len(want)] != want {
			continue
		}
		if i > 0 && isIdentChar(rune(lower[i-1])) {
			continue
		}
		end := i + len(want)
		if end < len(lower) && isIdentChar(rune(lower[end])) {
			continue
		}
		return true
	}
	return false
}

func isIdentChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// writeTableContext appends one table's compact DDL to b. Schema or key lookup
// failures are downgraded to comments rather than aborting the whole context —
// a model that knows a table exists but not its columns is still useful.
func writeTableContext(b *strings.Builder, conn SchemaIntrospector, table string) {
	fmt.Fprintf(b, "CREATE TABLE %s (\n", table)

	cols, err := conn.TableSchema(table)
	if err != nil {
		fmt.Fprintf(b, "  -- schema unavailable: %v\n);\n\n", err)
		return
	}

	pkSet := safePrimaryKeys(conn, table)
	fks := safeForeignKeys(conn, table)
	// Index FK columns by their local column for inline annotation.
	fkByCol := make(map[string]ForeignKey, len(fks))
	for _, fk := range fks {
		fkByCol[fk.Column] = fk
	}

	for ci, c := range cols {
		note := ""
		if pkSet[c.Name] {
			note = "primary key"
		}
		if fk, ok := fkByCol[c.Name]; ok {
			if note != "" {
				note += "; "
			}
			note += fmt.Sprintf("FK -> %s.%s", fk.RefTable, fk.RefColumn)
		}
		line := fmt.Sprintf("  %s %s", c.Name, c.Type)
		if note != "" {
			line += "  -- " + note
		}
		if ci < len(cols)-1 {
			line += ","
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(");\n\n")
}

func safePrimaryKeys(conn SchemaIntrospector, table string) map[string]bool {
	pks, err := conn.PrimaryKeys(table)
	if err != nil || len(pks) == 0 {
		return nil
	}
	set := make(map[string]bool, len(pks))
	for _, p := range pks {
		set[p] = true
	}
	return set
}

func safeForeignKeys(conn SchemaIntrospector, table string) []ForeignKey {
	fks, err := conn.ForeignKeys(table)
	if err != nil {
		return nil
	}
	return fks
}
