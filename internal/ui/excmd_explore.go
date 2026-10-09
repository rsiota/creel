package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/db"
)

// exEditFile loads a .sql (or any text) file into the editor (:e <file>),
// replacing the current buffer — vim's :edit. The contents are not executed;
// run them from the editor as usual (statements are split at run time). ~ is
// expanded; relative paths resolve against the working directory.
func (m *Model) exEditFile(path string) tea.Cmd {
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	content, err := os.ReadFile(expanded)
	if err != nil {
		m.schemaMsg = "read failed: " + err.Error()
		return nil
	}
	m.editor.SetValue(string(content))
	m.schemaMsg = fmt.Sprintf("loaded %s (%d lines)", expanded, lineCount(string(content)))
	return nil
}

// loadStartupFile reads a .sql file into the editor for the `creel -f` startup
// flag — the non-interactive counterpart of :e. It expands ~ and resolves
// relative paths against the working directory (same as :e), returning the
// expanded path on success or the read error otherwise. Run fails fast on the
// error (a missing/unreadable file is almost always a typo); the loaded script
// is not auto-executed — review it in the editor, then :runall / :source (or
// ctrl+e for the statement under the cursor).
func (m *Model) loadStartupFile(path string) (string, error) {
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(expanded)
	if err != nil {
		return expanded, err
	}
	m.editor.SetValue(string(content))
	// A startup file is an explicit request to review this buffer, so the
	// first connect should not clobber it with a restored session.
	m.startupFileLoaded = true
	return expanded, nil
}

// exSession manages the saved workspace session for the current connection +
// database (:session clear | :session save | :session). "clear" drops the
// persisted snapshot so the next reconnect starts fresh — the live workspace
// is untouched, and it re-saves on the next quit/teardown, so it is not gated
// behind confirm_destructive. "save" snapshots now (without quitting); bare
// reports whether a session is stored and how many tabs it holds.
func (m *Model) exSession(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	conn, database, ok := m.sessionKey()
	if !ok || m.sessionStore == nil {
		m.schemaMsg = "no active session"
		return nil
	}
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch sub {
	case "clear", "off", "reset", "delete":
		if err := m.sessionStore.Clear(conn, database); err != nil {
			m.schemaMsg = "session: " + err.Error()
			return nil
		}
		m.colWidthMem = nil
		m.colWidthOverride = nil
		m.erdPosMem = nil
		m.schemaMsg = "session cleared — reconnect will start fresh"
		return nil
	case "save":
		m.saveSession()
		m.schemaMsg = "session saved"
		return nil
	case "", "status", "show":
		st, err := m.sessionStore.Load(conn, database)
		if err != nil {
			m.schemaMsg = "session: " + err.Error()
			return nil
		}
		if !st.HasContent() {
			m.schemaMsg = "no saved session"
		} else {
			active := st.Active
			if active < 0 || active >= len(st.Tabs) {
				active = 0
			}
			m.schemaMsg = fmt.Sprintf("session: %d tab(s) saved, active %d", len(st.Tabs), active+1)
		}
		return nil
	}
	m.schemaMsg = "usage: :session [clear|save]"
	return nil
}

// exWriteFile writes the editor buffer to a file (:w <file>) — vim's :write.
// It overwrites an existing file (use a versioned name if you need to keep the
// old one). ~ is expanded; relative paths resolve against the working dir.
func (m *Model) exWriteFile(path string) tea.Cmd {
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	content := m.editor.Value()
	if err := os.WriteFile(expanded, []byte(content), 0o644); err != nil {
		m.schemaMsg = "write failed: " + err.Error()
		return nil
	}
	m.schemaMsg = fmt.Sprintf("wrote %s (%d lines)", expanded, lineCount(content))
	return nil
}

// exSaveBlob writes the binary value under the results cursor to a file
// (:saveblob <file>). Binary cells are scanned as []byte and shown as
// "<BLOB …>" placeholders; this is how you recover the raw bytes.
func (m *Model) exSaveBlob(path string) tea.Cmd {
	row, col := m.results.CursorRow(), m.results.CursorCol()
	data, ok := m.results.BlobData(row, col)
	if !ok {
		m.schemaMsg = "cursor cell is not a binary value"
		return nil
	}
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	if err := os.WriteFile(expanded, data, 0o644); err != nil {
		m.schemaMsg = "write failed: " + err.Error()
		return nil
	}
	m.schemaMsg = fmt.Sprintf("wrote %s to %s", db.FormatByteSize(len(data)), expanded)
	return nil
}

// exExport writes the current result set to ~/Downloads in the given format
// (:export <fmt> [cols...]) — a non-interactive shortcut over the g X export
// dialog. <fmt> is one of csv, json, jsonl, md, tsv (case-insensitive; "markdown"
// and "json lines" are accepted). Optional trailing arguments name the columns
// to export (comma-separated within one arg or across args, e.g.
// `:export csv name,email`); when omitted, all columns are exported. The row
// scope defaults sensibly (marked rows if any, else whole table / whole
// result for a custom query, else page); use the g X dialog to choose scope
// explicitly. It reuses exportResults, so feedback flows through the same
// export status message.
func (m *Model) exExport(args []string) tea.Cmd {
	if len(args) == 0 {
		m.schemaMsg = ":export needs a format: csv, json, jsonl, md, tsv"
		return nil
	}
	format, ok := parseExportFormat(args[0])
	if !ok {
		m.schemaMsg = ":export needs a format: csv, json, jsonl, md, tsv"
		return nil
	}
	var cols []string
	for _, a := range args[1:] {
		for _, c := range strings.Split(a, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				cols = append(cols, c)
			}
		}
	}
	return m.exportResults(format, cols, m.defaultExportScope())
}

// resolveTableName finds the canonical (sidebar) name for a table the user
// typed: an exact case-insensitive match first, then a substring fallback.
// Qualified names (schema.table) match that schema; bare names prefer the
// active schema, then any expanded foreign-schema row as schema.table.
// Returns "" if nothing matches. Shared by ex commands that take a table arg.
func (m Model) resolveTableName(name string) string {
	wantSchema, wantTable := splitTableRef(name)
	if wantTable == "" {
		return ""
	}
	items := m.sidebarItems()
	active := m.currentSchemaName()

	if wantSchema != "" {
		for _, it := range items {
			if it.isTableRow() && strings.EqualFold(it.schema, wantSchema) && strings.EqualFold(it.text, wantTable) {
				return it.schema + "." + it.text
			}
		}
		for _, t := range m.schemaTableCache[wantSchema] {
			if strings.EqualFold(t, wantTable) {
				return wantSchema + "." + t
			}
		}
		return ""
	}

	// Bare name: prefer active-schema / flat-mode rows.
	for _, it := range items {
		if !it.isTableRow() || !strings.EqualFold(it.text, wantTable) {
			continue
		}
		if it.schema == "" || it.schema == active {
			return it.text
		}
	}
	// Then any foreign-schema row as schema.table.
	for _, it := range items {
		if it.isTableRow() && strings.EqualFold(it.text, wantTable) && it.schema != "" {
			return it.schema + "." + it.text
		}
	}
	// Collapsed schemas: first cache hit outside active (stable order).
	for _, schema := range m.sidebarSchemaOrder() {
		if schema == active {
			continue
		}
		for _, t := range m.schemaTableCache[schema] {
			if strings.EqualFold(t, wantTable) {
				return schema + "." + t
			}
		}
	}

	needle := strings.ToLower(wantTable)
	for _, it := range items {
		if !it.isTableRow() || !strings.Contains(strings.ToLower(it.text), needle) {
			continue
		}
		if it.schema == "" || it.schema == active {
			return it.text
		}
		return it.schema + "." + it.text
	}
	return ""
}

// resolveTableArg resolves an optional table argument for an ex command, shared
// by :refs and :uses (and future table-targeted lookups). With no name it
// falls back to the current table — the focused sidebar selection or the
// results' source table (default-to-current-object convention, #15). With a
// name it resolves case-insensitively against the sidebar (exact match, then
// substring). On failure it sets schemaMsg and returns "".
func (m *Model) resolveTableArg(name string) string {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return ""
	}
	if name == "" {
		if t := m.currentTable(); t != "" {
			return t
		}
		m.schemaMsg = "no current table — name one: e.g. :refs <table>"
		return ""
	}
	if resolved := m.resolveTableName(name); resolved != "" {
		return resolved
	}
	m.schemaMsg = fmt.Sprintf("no such table: %s", name)
	return ""
}

// resolveDDLTableArg resolves an optional table for sidebar-mirrored DDL
// (:truncate / :drop / :rename). Unlike resolveTableArg, a bare command prefers
// the sidebar cursor (sidebarSelectedActiveTable) — matching the T/D/r keys — even
// when results still show a different SourceTable or focus is not the sidebar.
// Foreign-schema targets are rejected; switch with :schema first.
func (m *Model) resolveDDLTableArg(name string) string {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return ""
	}
	if name != "" {
		if resolved := m.resolveTableName(name); resolved != "" {
			if schema, table := splitTableRef(resolved); schema != "" {
				if !strings.EqualFold(schema, m.currentSchemaName()) {
					m.schemaMsg = fmt.Sprintf("use :schema %s to switch for DDL", schema)
					return ""
				}
				return table
			}
			return resolved
		}
		m.schemaMsg = fmt.Sprintf("no such table: %s", name)
		return ""
	}
	if t := m.sidebarSelectedActiveTable(); t != "" {
		return t
	}
	if t := m.currentTable(); t != "" {
		if schema, table := splitTableRef(t); schema != "" {
			if !strings.EqualFold(schema, m.currentSchemaName()) {
				m.schemaMsg = fmt.Sprintf("use :schema %s to switch for DDL", schema)
				return ""
			}
			return table
		}
		return t
	}
	m.schemaMsg = "no current table — name one"
	return ""
}

// exRefs lists the foreign keys referencing a table (:refs <table>) — the
// reverse of g d. When the results grid is backing the same table and a row is
// focused, it additionally shows a per-referrer Count column computed against
// that row's value ("Orders (14)"), turning the relationship list into a live,
// countable inbound view — the first step toward the row-explorer fan-out.
// The lookup runs async and opens in the lookup overlay panel. It reads
// connection metadata, so it is unaffected by (and does not block on) an
// active transaction.
func (m *Model) exRefs(name string) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	conn := m.connection
	driver := conn.Config().Driver

	// Capture the focused row's column→value map on the main goroutine so the
	// async closure can compute per-referrer counts. Counts are only shown when
	// the results grid is backing this same table with a focused row.
	rowVals, label, scoped := m.focusedRowValues(table)

	return func() tea.Msg {
		refs, err := conn.DB().ReferencingForeignKeys(table)
		if err != nil {
			return lookupResultMsg{err: err}
		}

		cols := []db.Column{{Name: "Table"}, {Name: "Column"}, {Name: "References"}}
		if scoped {
			cols = append(cols, db.Column{Name: "Count"})
		}
		rows := make([][]string, len(refs))

		if scoped {
			// Fan out count queries concurrently — database/sql pools
			// connections and is safe for parallel use, and each goroutine
			// writes a distinct slice index, so the slice stays race-free.
			var wg sync.WaitGroup
			for i, r := range refs {
				wg.Add(1)
				go func(i int, r db.Referrer) {
					defer wg.Done()
					rows[i] = []string{r.Table, r.Column, table + "." + r.RefColumn, countReferrer(conn, driver, r, rowVals)}
				}(i, r)
			}
			wg.Wait()
		} else {
			for i, r := range refs {
				rows[i] = []string{r.Table, r.Column, table + "." + r.RefColumn}
			}
		}

		jumps := make([]string, len(refs))
		for i, r := range refs {
			jumps[i] = r.Table
		}
		return lookupResultMsg{
			title:  "References to " + table + label,
			result: db.Result{Columns: cols, Rows: rows},
			jumps:  jumps,
		}
	}
}

// focusedRowValues returns the focused results row as a lowercased
// column-name → value map, plus a short " · pk=val" label identifying which
// row the counts are scoped to. ok is false when the grid is not backing
// `table` (or has no focused row), in which case :refs omits the Count column
// and behaves as before. Must run on the main goroutine — it reads live UI
// state.
func (m *Model) focusedRowValues(table string) (vals map[string]string, label string, ok bool) {
	r := m.results
	if !r.HasResult() {
		return nil, "", false
	}
	if src := r.SourceTable(); src == "" || !strings.EqualFold(src, table) {
		return nil, "", false
	}
	row := r.CursorRow()
	if row < 0 || row >= r.NumRows() {
		return nil, "", false
	}
	vals = make(map[string]string, r.NumCols())
	for c := 0; c < r.NumCols(); c++ {
		vals[strings.ToLower(r.ColumnName(c))] = r.RowValue(row, c)
	}
	return vals, pkLabel(r), true
}

// pkLabel builds a " · #val" suffix identifying the focused row, preferring
// the primary key tuple and falling back to the first non-empty cell when
// there is no PK. When a glanceable title column is present it is appended
// (" · #1  Alice"). Empty when no usable value is found.
func pkLabel(r ResultsTable) string {
	cols := make([]string, r.NumCols())
	vals := make(map[string]string, r.NumCols())
	for c := 0; c < r.NumCols(); c++ {
		cols[c] = r.ColumnName(c)
		vals[strings.ToLower(cols[c])] = r.RowValue(r.CursorRow(), c)
	}
	pkCols := r.PKColumns()

	id := ""
	if tup := r.CursorPKTuple(); len(tup) > 0 {
		parts := make([]string, 0, len(tup))
		for _, v := range tup {
			if v == "" || v == "NULL" {
				continue
			}
			parts = append(parts, v)
		}
		if len(parts) > 0 {
			id = "#" + strings.Join(parts, ", ")
		}
	}
	if id == "" {
		for c := 0; c < r.NumCols(); c++ {
			if v := r.RowValue(r.CursorRow(), c); v != "" && v != "NULL" {
				id = r.ColumnName(c) + "=" + v
				break
			}
		}
	}
	if id == "" {
		return ""
	}
	if title := rowTitle(cols, vals, pkCols); title != "" {
		return " · " + id + "  " + title
	}
	return " · " + id
}

// countReferrer returns how many rows in the child table reference the focused
// parent row via this FK. "-" means nothing to match (the parent value is
// absent/NULL), "?" means the count query failed (the row still renders). Uses
// the same string-escaping convention as g d's buildForeignKeyQuery.
func countReferrer(conn *db.Connection, driver db.Driver, ref db.Referrer, rowVals map[string]string) string {
	val, ok := rowVals[strings.ToLower(ref.RefColumn)]
	if !ok || val == "" || val == "NULL" {
		return "-"
	}
	return countRelated(conn, driver, ref.Table, ref.Column, val)
}

// countRelated returns how many rows in `table` have `col` equal to `val`, as
// a string. "?" means the count query failed. Shared by :refs' per-referrer
// counts and the relationship explorer's per-edge counts.
func countRelated(conn *db.Connection, driver db.Driver, table, col, val string) string {
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = %s",
		quoteIdentD(driver, table), quoteIdentD(driver, col), quoteSQLString(val))
	res, err := conn.DB().Execute(q)
	if err != nil {
		return "?"
	}
	if len(res.Rows) == 0 || len(res.Rows[0]) == 0 {
		return "0"
	}
	return res.Rows[0][0]
}

// openDockedExplorer toggles the relationship explorer as a right-slot panel
// (the "inspector-tab" variant): non-modal, sharing the slot with the
// inspector/assistant, and cursor-driven — it re-roots to the focused results
// row as the cursor moves. Invoked by `g r` and `:explore`.
func (m *Model) openDockedExplorer() tea.Cmd {
	if m.explorer.IsVisible() && m.explorer.docked {
		m.closeDockedExplorer()
		return nil
	}
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	// The right slot holds one panel: close inspector/assistant.
	m.inspector.Hide()
	m.assistant.Hide()
	m.explorer.ShowDocked()
	m.explorer.markLoading()
	m.focus = FocusExplorer
	m.layoutWorkspace()
	m.applyFocus()
	return m.loadExplorer()
}

// closeDockedExplorer hides the docked explorer panel and returns focus to the
// results grid.
func (m *Model) closeDockedExplorer() {
	m.explorer.Hide()
	if m.focus == FocusExplorer {
		m.focus = FocusResults
	}
	m.layoutWorkspace()
	m.applyFocus()
}

// explorerAnchor is the identity of the results row the explorer would root at
// (source table + PK tuple), or "" when there is nothing to anchor to. Used to
// detect cursor moves so the docked panel can re-root without redundant loads.
func (m Model) explorerAnchor() string {
	r := m.results
	if !r.HasResult() {
		return ""
	}
	src := r.SourceTable()
	if src == "" {
		return ""
	}
	tup := r.CursorPKTuple()
	if len(tup) == 0 {
		return ""
	}
	return src + "|" + strings.Join(tup, ",")
}

// maybeReloadDockedExplorer re-roots the docked explorer when the results
// cursor has landed on a different row. It does not markLoading, so the
// previous tree stays visible until the new root arrives (no flicker on every
// cursor move).
func (m *Model) maybeReloadDockedExplorer() tea.Cmd {
	if !m.explorer.IsVisible() {
		return nil
	}
	cur := m.explorerAnchor()
	if cur == "" || cur == m.explorer.anchor {
		return nil
	}
	return m.loadExplorer()
}

// loadExplorer builds the explorer tree root from the focused results row and
// loads its first-level edges (with counts), returning explorerLoadedMsg. It
// powers openDockedExplorer and the auto-refresh after Enter/back (wired in
// app.go on queryExecutedMsg). When there is nothing to explore it returns a
// message with an emptyMsg so the panel shows a reason rather than a blank box.
func (m *Model) loadExplorer() tea.Cmd {
	r := m.results
	depth := len(m.queryStack)
	if m.connection == nil {
		return explorerMsg(explorerLoadedMsg{depth: depth, emptyMsg: "not connected"})
	}
	if !r.HasResult() {
		return explorerMsg(explorerLoadedMsg{depth: depth, emptyMsg: "no results — run a query first"})
	}
	src := r.SourceTable()
	if src == "" {
		return explorerMsg(explorerLoadedMsg{depth: depth, emptyMsg: "current results are not a single table — browse a table to explore"})
	}
	rowVals, label, ok := m.focusedRowValues(src)
	if !ok {
		return explorerMsg(explorerLoadedMsg{depth: depth, emptyMsg: "no focused row — select a row to explore its relationships"})
	}
	conn := m.connection
	driver := conn.Config().Driver
	return func() tea.Msg {
		root := &expNode{kind: nodeRow, table: src, rowVals: rowVals, label: label, expanded: true}
		edges, err := loadRowEdges(conn, driver, src, rowVals, nil)
		if err != nil {
			return explorerLoadedMsg{depth: depth, err: err}
		}
		for _, e := range edges {
			e.parent = root
			e.depth = 1
		}
		root.children = edges
		if len(edges) == 0 {
			root.children = []*expNode{synthNode(src+" has no relationships", root)}
		}
		return explorerLoadedMsg{root: root, depth: depth}
	}
}

// explorerMsg wraps a pre-resolved explorerLoadedMsg as a no-op command, for
// the synchronous empty/error paths of loadExplorer.
func explorerMsg(msg explorerLoadedMsg) tea.Cmd {
	return func() tea.Msg { return msg }
}

// loadExplorerChildren lazily loads a node's children, returning
// explorerChildrenMsg: for an edge node it loads the related rows (capped at
// explorerChildLimit); for a row node it loads that row's edges. Depth-capped
// nodes get a synthetic marker instead of a load.
func (m *Model) loadExplorerChildren(node *expNode) tea.Cmd {
	if node == nil {
		return nil
	}
	if node.depth >= maxExplorerDepth {
		return func() tea.Msg {
			return explorerChildrenMsg{parent: node, children: []*expNode{synthNode("(depth limit reached)", node)}}
		}
	}
	conn := m.connection
	driver := conn.Config().Driver
	if node.isEdge() {
		val := node.filterVal
		full := edgeDrillQuery(driver, node.edge, val)
		q := full + fmt.Sprintf(" LIMIT %d", explorerChildLimit+1)
		tbl := node.edge.targetTable
		// Collect every row already on the path from the root to this edge so
		// we can suppress children that would re-enter it. FK graphs cycle
		// (users → orders → users), and without this the drill-down re-shows
		// the row you started at, ad infinitum (until maxExplorerDepth).
		ancestors := ancestorIdentities(node)
		return func() tea.Msg {
			res, err := conn.DB().Execute(q)
			if err != nil {
				return explorerChildrenMsg{parent: node, err: err}
			}
			pkCols, _ := conn.DB().PrimaryKeys(tbl)
			rows := buildChildRowNodes(res, tbl, pkCols, driver)
			// Drop child rows that are already ancestors (cycle break).
			cycled := 0
			if len(ancestors) > 0 {
				kept := rows[:0]
				for _, r := range rows {
					if ancestors[rowIdentity(tbl, r.rowVals)] {
						cycled++
						continue
					}
					kept = append(kept, r)
				}
				rows = kept
			}
			if len(res.Rows) > explorerChildLimit {
				more := synthNode(fmt.Sprintf("(+%d more — enter to open in grid)", len(res.Rows)-explorerChildLimit), node)
				more.drillQuery = full // full, unlimited set
				if len(rows) > explorerChildLimit {
					rows = append(rows[:explorerChildLimit], more)
				} else {
					rows = append(rows, more)
				}
			}
			switch {
			case len(rows) == 0 && cycled > 0:
				return explorerChildrenMsg{parent: node, fold: true}
			case len(rows) == 0:
				rows = []*expNode{synthNode("(no rows)", node)}
			}
			return explorerChildrenMsg{parent: node, children: rows}
		}
	}
	// row node: load its edges, omitting outbound edges that loop back to a row
	// already on the path (so e.g. a child row's FK to its parent isn't shown).
	ancestors := ancestorRowNodes(node)
	return func() tea.Msg {
		edges, err := loadRowEdges(conn, driver, node.table, node.rowVals, ancestors)
		if err != nil {
			return explorerChildrenMsg{parent: node, err: err}
		}
		if len(edges) == 0 {
			// Nothing to list (no FKs, or every edge was a back-edge): fold the
			// node back up rather than show a "(no relationships)" marker.
			return explorerChildrenMsg{parent: node, fold: true}
		}
		return explorerChildrenMsg{parent: node, children: edges}
	}
}

// loadRowEdges fetches a row's outbound + inbound FK edges and resolves a live
// per-edge count, returning edge nodes ready to attach as a row's children.
// The count fan-out is concurrent (one goroutine per edge, distinct index).
func loadRowEdges(conn *db.Connection, driver db.Driver, table string, rowVals map[string]string, ancestors []*expNode) ([]*expNode, error) {
	out, errO := conn.DB().ForeignKeys(table)
	in, errI := conn.DB().ReferencingForeignKeys(table)
	if errO != nil && errI != nil {
		return nil, errO
	}
	edges := make([]*expNode, 0, len(out)+len(in))
	for _, fk := range out {
		if errO != nil {
			continue
		}
		val := rowVals[strings.ToLower(fk.Column)]
		e := &expNode{
			kind:      nodeEdge,
			edge:      relNode{dir: relOutbound, targetTable: fk.RefTable, targetColumn: fk.RefColumn, sourceColumn: fk.Column},
			filterVal: val,
		}
		e.drillQuery = edgeDrillQuery(driver, e.edge, val)
		edges = append(edges, e)
	}
	for _, rf := range in {
		if errI != nil {
			continue
		}
		val := rowVals[strings.ToLower(rf.RefColumn)]
		e := &expNode{
			kind:      nodeEdge,
			edge:      relNode{dir: relInbound, targetTable: rf.Table, targetColumn: rf.Column, sourceColumn: rf.RefColumn},
			filterVal: val,
		}
		e.drillQuery = edgeDrillQuery(driver, e.edge, val)
		edges = append(edges, e)
	}
	var wg sync.WaitGroup
	for i := range edges {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			val := edges[i].filterVal
			if val == "" || val == "NULL" {
				edges[i].edge.count = "-"
				return
			}
			edges[i].edge.count = countRelated(conn, driver, edges[i].edge.targetTable, edges[i].edge.targetColumn, val)
		}(i)
	}
	wg.Wait()

	// Keep only edges worth showing:
	//   - outbound: positive count, and not a back-edge to an ancestor
	//   - inbound: any resolved numeric count, including 0 — empty child
	//     relations stay visible so "insert related" (A) has a target
	kept := edges[:0]
	for _, e := range edges {
		if !isNumericCount(e.edge.count) {
			continue
		}
		c, _ := strconv.Atoi(e.edge.count)
		if e.edge.dir == relOutbound {
			if c <= 0 {
				continue
			}
			if isOutboundBackEdge(e, ancestors) {
				continue
			}
		}
		// inbound: keep even when c == 0
		kept = append(kept, e)
	}
	return kept, nil
}

// isOutboundBackEdge reports whether an outbound edge's target row is already
// an ancestor on the path from the root — i.e. expanding it would only loop
// back to a row already shown. Outbound FK targets are unique (a PK or unique
// column), so a matching ancestor means there is nothing new to drill into.
func isOutboundBackEdge(e *expNode, ancestors []*expNode) bool {
	col := strings.ToLower(e.edge.targetColumn)
	for _, a := range ancestors {
		if a.table == e.edge.targetTable && a.rowVals[col] == e.filterVal {
			return true
		}
	}
	return false
}

// edgeDrillQuery builds the SELECT to open an edge's full related set in the
// grid (Enter on an edge node). An absent value falls back to an unfiltered
// browse of the target table.
func edgeDrillQuery(driver db.Driver, edge relNode, val string) string {
	tbl := quoteIdentD(driver, edge.targetTable)
	col := quoteIdentD(driver, edge.targetColumn)
	if val == "" || val == "NULL" {
		return fmt.Sprintf("SELECT * FROM %s", tbl)
	}
	return fmt.Sprintf("SELECT * FROM %s WHERE %s = %s", tbl, col, quoteSQLString(val))
}

// buildChildRowNodes turns a SELECT result set into row nodes, one per row,
// each carrying its column→value map, a display label, and a drill query to
// open that exact row in the grid on Enter.
func buildChildRowNodes(res db.Result, table string, pkCols []string, driver db.Driver) []*expNode {
	cols := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		cols[i] = c.Name
	}
	nodes := make([]*expNode, 0, len(res.Rows))
	for ri, row := range res.Rows {
		vals := make(map[string]string, len(cols))
		for i := range cols {
			if i < len(row) {
				vals[strings.ToLower(cols[i])] = row[i]
			}
		}
		nodes = append(nodes, &expNode{
			kind:       nodeRow,
			table:      table,
			rowVals:    vals,
			label:      rowLabel(cols, vals, pkCols, ri),
			drillQuery: rowDrillQuery(driver, table, pkCols, vals),
		})
	}
	return nodes
}

// rowIdentity returns a canonical key for one row, used only to detect FK
// cycles in the explorer tree: a child row that matches a row already on the
// path from the root is suppressed so drilling never re-shows the row you
// started from. Two rows are "the same" exactly when every cell matches.
func rowIdentity(table string, vals map[string]string) string {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(table)
	b.WriteByte('|')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(vals[k])
	}
	return b.String()
}

// ancestorIdentities collects the row identities along the path from the root
// down to (but excluding) the given edge node — every row the explorer has
// already expanded through to reach this edge. Used to break FK cycles.
func ancestorIdentities(edge *expNode) map[string]bool {
	set := map[string]bool{}
	for p := edge.parent; p != nil; p = p.parent {
		if p.kind == nodeRow && !p.synthetic && p.rowVals != nil {
			set[rowIdentity(p.table, p.rowVals)] = true
		}
	}
	return set
}

// ancestorRowNodes returns the row nodes on the path from the root down to (but
// excluding) the given node, nearest-first. Used to omit outbound edges that
// would loop back to a row already shown.
func ancestorRowNodes(node *expNode) []*expNode {
	var out []*expNode
	for p := node.parent; p != nil; p = p.parent {
		if p.kind == nodeRow && !p.synthetic && p.rowVals != nil {
			out = append(out, p)
		}
	}
	return out
}

// rowLabel builds a human-friendly identity for a child row: the PK tuple
// ("#1001") when available, optionally followed by a glanceable title column
// ("#1001  Alice"), else the first non-empty cell, else a positional "#N".
func rowLabel(cols []string, vals map[string]string, pkCols []string, idx int) string {
	id := rowIdentityLabel(cols, vals, pkCols, idx)
	if title := rowTitle(cols, vals, pkCols); title != "" {
		return id + "  " + title
	}
	return id
}

// rowIdentityLabel is the stable identity part of a row label (PK or fallback).
func rowIdentityLabel(cols []string, vals map[string]string, pkCols []string, idx int) string {
	if len(pkCols) > 0 {
		parts := make([]string, 0, len(pkCols))
		ok := true
		for _, pk := range pkCols {
			v := vals[strings.ToLower(pk)]
			if v == "" || v == "NULL" {
				ok = false
				break
			}
			parts = append(parts, v)
		}
		if ok && len(parts) > 0 {
			return "#" + strings.Join(parts, ", ")
		}
	}
	for _, c := range cols {
		v := vals[strings.ToLower(c)]
		if v != "" && v != "NULL" {
			return c + "=" + v
		}
	}
	return fmt.Sprintf("#%d", idx)
}

// explorerTitleMax is the max visible runes for a title crumb in the explorer.
const explorerTitleMax = 40

// exactTitleNames are preferred column names for a glanceable row title
// (case-insensitive). Ordered by preference.
var exactTitleNames = []string{
	"name", "title", "label", "display_name", "fullname_name",
	"username", "email", "slug",
}

// titleNameSuffixes match columns like product_name / job_title.
var titleNameSuffixes = []string{"_name", "_title", "_label"}

// skipTitleNames are never used as explorer titles (metadata / secrets).
var skipTitleNames = map[string]bool{
	"created_at": true, "updated_at": true, "deleted_at": true,
	"created": true, "updated": true, "deleted": true,
	"password": true, "passwd": true, "secret": true, "token": true,
	"hash": true, "salt": true, "api_key": true, "access_token": true,
}

// rowTitle picks a short human-readable crumb from the row's columns, or "".
// Preference: exact name matches → *_name/*_title/*_label → first short text
// cell that isn't a PK, datetime, blob, or metadata column.
func rowTitle(cols []string, vals map[string]string, pkCols []string) string {
	pk := map[string]bool{}
	for _, c := range pkCols {
		pk[strings.ToLower(c)] = true
	}
	byLower := map[string]string{} // lower → original column name
	for _, c := range cols {
		byLower[strings.ToLower(c)] = c
	}

	try := func(col string) string {
		if pk[strings.ToLower(col)] || skipTitleNames[strings.ToLower(col)] {
			return ""
		}
		v := vals[strings.ToLower(col)]
		return sanitizeTitleValue(v)
	}

	for _, name := range exactTitleNames {
		if col, ok := byLower[name]; ok {
			if t := try(col); t != "" {
				return t
			}
		}
	}
	for _, c := range cols {
		lower := strings.ToLower(c)
		for _, suf := range titleNameSuffixes {
			if strings.HasSuffix(lower, suf) {
				if t := try(c); t != "" {
					return t
				}
				break
			}
		}
	}
	for _, c := range cols {
		lower := strings.ToLower(c)
		if pk[lower] || skipTitleNames[lower] {
			continue
		}
		if looksLikeIDColumn(lower) {
			continue
		}
		if t := try(c); t != "" {
			return t
		}
	}
	return ""
}

func looksLikeIDColumn(lower string) bool {
	if lower == "id" || lower == "uuid" || lower == "guid" {
		return true
	}
	return strings.HasSuffix(lower, "_id") ||
		strings.HasSuffix(lower, "_uuid") ||
		strings.HasSuffix(lower, "_guid")
}

// sanitizeTitleValue rejects empty/NULL/opaque/numeric-only values and
// truncates long text so the explorer stays scannable.
func sanitizeTitleValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "NULL" {
		return ""
	}
	if looksLikeOpaqueID(v) {
		return ""
	}
	// Pure numbers make poor titles (qty, totals, status codes).
	if isNumericCount(v) {
		return ""
	}
	// Collapse internal whitespace so a multi-line TEXT cell stays one crumb.
	v = strings.Join(strings.Fields(v), " ")
	runes := []rune(v)
	if len(runes) > explorerTitleMax {
		return string(runes[:explorerTitleMax-1]) + "…"
	}
	return v
}

// looksLikeOpaqueID reports UUIDs and long hex hashes that make poor titles.
func looksLikeOpaqueID(v string) bool {
	if len(v) == 36 {
		// 8-4-4-4-12 UUID shape
		ok := true
		for i, r := range v {
			switch i {
			case 8, 13, 18, 23:
				if r != '-' {
					ok = false
				}
			default:
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
					ok = false
				}
			}
		}
		if ok {
			return true
		}
	}
	if len(v) >= 32 {
		hex := true
		for _, r := range v {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				hex = false
				break
			}
		}
		if hex {
			return true
		}
	}
	return false
}

// rowDrillQuery builds the SELECT to open one exact row in the grid on Enter.
// Returns "" when the table has no PK (Enter is then disabled for that node).
func rowDrillQuery(driver db.Driver, table string, pkCols []string, vals map[string]string) string {
	if len(pkCols) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pkCols))
	for _, pk := range pkCols {
		v := vals[strings.ToLower(pk)]
		if v == "" || v == "NULL" {
			return "" // incomplete PK — can't address the row uniquely
		}
		parts = append(parts, fmt.Sprintf("%s = %s", quoteIdentD(driver, pk), quoteSQLString(v)))
	}
	return fmt.Sprintf("SELECT * FROM %s WHERE %s", quoteIdentD(driver, table), strings.Join(parts, " AND "))
}
