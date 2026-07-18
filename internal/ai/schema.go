package ai

import (
	"fmt"
	"strings"
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
// The returned string is prefixed with a short legend so the model can parse
// the inline annotations.
func SchemaContext(conn SchemaIntrospector) (string, error) {
	if conn == nil {
		return "", nil
	}
	tables, err := conn.Tables()
	if err != nil {
		return "", fmt.Errorf("listing tables: %w", err)
	}

	var b strings.Builder
	b.WriteString("-- Tables, columns, and relationships. PK marks a primary key;\n")
	b.WriteString("-- FK: <col> -> <table>.<col> marks a foreign key.\n\n")

	truncated := false
	for i, t := range tables {
		if i >= maxTablesInContext {
			truncated = true
			break
		}
		writeTableContext(&b, conn, t)
	}
	if truncated {
		fmt.Fprintf(&b, "\n-- (... %d more tables omitted to fit context)\n", len(tables)-maxTablesInContext)
	}
	return b.String(), nil
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
