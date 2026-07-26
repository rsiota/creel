package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/db"
)

// buildMermaidERD renders a Mermaid erDiagram for the given tables from their
// cached column/PK/FK metadata. It is the static counterpart to the
// interactive `g r` explorer: a copy-pasteable diagram (GitHub/GitLab render
// it inline in markdown) or one that can be saved to a .mmd file.
//
// tables is the entity set to draw (already filtered to base tables and, for a
// focused ERD, limited to the table + its FK neighbours). FKs whose endpoints
// both lie in tables are drawn as relationships; cross-set FKs are omitted so a
// focused diagram stays self-contained.
//
// Cardinality is a fixed one-to-many (`||--o{`) per FK. The column nullability
// that would distinguish `||--||` (one) from `||--o{` (zero-or-many) is not in
// the cached schema, so the diagram conveys structure, not optionality.
func buildMermaidERD(tables []string, schemas map[string][]db.Column, pks map[string][]string, fks map[string][]db.ForeignKey) []string {
	if len(tables) == 0 {
		return []string{mutedStyle.Render("(no tables)")}
	}
	inSet := make(map[string]bool, len(tables))
	for _, t := range tables {
		inSet[t] = true
	}

	sorted := append([]string(nil), tables...)
	sort.Strings(sorted)

	var lines []string
	lines = append(lines, "erDiagram")

	for _, t := range sorted {
		pkSet := make(map[string]bool)
		for _, c := range pks[t] {
			pkSet[c] = true
		}
		fkSet := make(map[string]bool)
		for _, fk := range fks[t] {
			fkSet[fk.Column] = true
		}
		lines = append(lines, "  "+erdIdent(t)+" {")
		// With no cached schema the entity is emitted attribute-less (still
		// valid Mermaid) so the relationship graph remains intact.
		for _, c := range schemas[t] {
			key := ""
			switch {
			case pkSet[c.Name] && fkSet[c.Name]:
				key = " PK,FK"
			case pkSet[c.Name]:
				key = " PK"
			case fkSet[c.Name]:
				key = " FK"
			}
			lines = append(lines, "    "+erdType(c.Type)+" "+erdIdent(c.Name)+key)
		}
		lines = append(lines, "  }")
	}

	// Relationships: one parent-to-many child per FK, both endpoints in set.
	seen := make(map[string]bool)
	for _, child := range sorted {
		for _, fk := range fks[child] {
			if !inSet[fk.RefTable] {
				continue
			}
			relKey := fk.RefTable + "\x00" + child + "\x00" + fk.Column
			if seen[relKey] {
				continue
			}
			seen[relKey] = true
			lines = append(lines, "  "+erdIdent(fk.RefTable)+" ||--o{ "+erdIdent(child)+" : "+erdIdent(fk.Column))
		}
	}
	return lines
}

// erdFocusSet returns the table plus its direct FK neighbours — tables it
// references outbound and tables that reference it inbound — restricted to the
// base tables present in allTables. Used by :erd <table> / g R to draw a
// self-contained neighbourhood instead of the whole schema.
func erdFocusSet(table string, allTables []string, fks map[string][]db.ForeignKey) []string {
	base := make(map[string]bool, len(allTables))
	for _, t := range allTables {
		base[t] = true
	}
	set := map[string]bool{table: true}
	// Outbound: tables this one points at.
	for _, fk := range fks[table] {
		if base[fk.RefTable] {
			set[fk.RefTable] = true
		}
	}
	// Inbound: tables pointing at this one.
	for child, childFks := range fks {
		if !base[child] {
			continue
		}
		for _, fk := range childFks {
			if fk.RefTable == table {
				set[child] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	return out
}

// erdType sanitizes a column type for Mermaid, which expects a single
// whitespace-free token. It takes the run up to the first space or '(' so
// "varchar(255)" → "varchar", "timestamp without time zone" → "timestamp",
// then keeps only identifier-safe characters.
func erdType(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return "text"
	}
	if i := strings.IndexAny(t, " ("); i >= 0 {
		t = t[:i]
	}
	t = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, t)
	if t == "" {
		return "text"
	}
	return t
}

// erdIdent collapses any whitespace in an identifier to underscores. Mermaid
// erDiagram entity/attribute names and relationship labels must be free of
// spaces; real schema identifiers almost always already are.
func erdIdent(s string) string {
	return strings.Join(strings.Fields(s), "_")
}

// baseTables returns the sidebar's tables with views excluded, since an ERD
// models base-table entities and their FKs (views have neither PKs nor FKs).
func (m Model) baseTables() []string {
	if len(m.views) == 0 {
		return m.tables
	}
	out := make([]string, 0, len(m.tables))
	for _, t := range m.tables {
		if !m.views[t] {
			out = append(out, t)
		}
	}
	return out
}

// openERD generates a Mermaid erDiagram and shows it in the ERD panel. With a
// non-empty focus it draws the table plus its direct FK neighbours; otherwise
// the whole schema. Data comes from the connection-time schema caches
// (populated by prefetchSchemas), so this is synchronous and instant.
func (m *Model) openERD(focus string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	tables := m.baseTables()
	targets := tables
	title := "ERD — schema"
	if focus != "" {
		targets = erdFocusSet(focus, tables, m.fkCache)
		title = "ERD — " + focus + " + neighbours"
	}
	layout := computeERDLayout(targets, m.columnCache, m.pkCache, m.fkCache)
	if layout != nil {
		layout.focus = focus
	}
	mermaid := buildMermaidERD(targets, m.columnCache, m.pkCache, m.fkCache)
	m.erdPanel.Show(title, layout, mermaid)
	m.erdPanel.SetSize(m.width, m.height-1)
	return nil
}

// exERD is the `:erd` command: bare draws the whole schema, `:erd <table>`
// draws the table + its FK neighbours, and `:erd save [file]` writes the
// whole-schema diagram to a file without opening the panel.
func (m *Model) exERD(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	// `:erd save [file]` — write the whole-schema diagram and return.
	if len(args) >= 1 && strings.EqualFold(args[0], "save") {
		path := "erd.mmd"
		if len(args) > 1 {
			path = args[1]
		}
		lines := buildMermaidERD(m.baseTables(), m.columnCache, m.pkCache, m.fkCache)
		m.saveERDToFile(path, lines)
		return nil
	}
	focus := ""
	if len(args) > 0 {
		focus = strings.TrimSpace(args[0])
		if canon, ok := erdLookupTable(focus, m.tables); ok {
			focus = canon
		} else {
			m.schemaMsg = "no such table: " + focus
			return nil
		}
	}
	return m.openERD(focus)
}

// erdLookupTable resolves a table name case-insensitively against the sidebar
// list, returning the canonical name. Used by :erd <table>.
func erdLookupTable(name string, tables []string) (string, bool) {
	for _, t := range tables {
		if strings.EqualFold(t, name) {
			return t, true
		}
	}
	return "", false
}

// saveERDToFile writes the given diagram lines to path (relative to the
// working directory), reporting the result via schemaMsg. It overwrites an
// existing file — pass a versioned name to keep the old one, matching :w
// <file>. Shared by the panel's `s` key and the `:erd save` command.
func (m *Model) saveERDToFile(path string, lines []string) {
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = "erd save failed: " + err.Error()
		return
	}
	if err := os.WriteFile(expanded, []byte(joinERDLines(lines)), 0o644); err != nil {
		m.schemaMsg = "erd save failed: " + err.Error()
		return
	}
	m.schemaMsg = "erd saved to " + expanded
}
