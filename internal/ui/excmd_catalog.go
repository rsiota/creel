package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/db"
)

// exUses lists the objects (views, functions, procedures, triggers) that
// reference a table in their definitions (:uses <table>) — a textual
// dependency scan, complementary to :refs. Same conventions as exRefs:
// default to the current table, async, transaction-unaffected.
func (m *Model) exUses(name string) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		uses, err := conn.DB().Uses(table)
		if err != nil {
			return lookupResultMsg{err: err}
		}
		cols := []db.Column{{Name: "Type"}, {Name: "Name"}}
		rows := make([][]string, 0, len(uses))
		for _, u := range uses {
			rows = append(rows, []string{u.Kind, u.Name})
		}
		return lookupResultMsg{
			title:  fmt.Sprintf("Objects using %s", table),
			result: db.Result{Columns: cols, Rows: rows},
		}
	}
}

// exDescribe opens the structure view for a table (:describe [table]),
// defaulting to the current table. It reuses resolveTableArg (so it works
// unqualified on the focused/last-queried table) and points the sidebar at
// the resolved table so openSchemaPanel — which reads the sidebar selection —
// targets it. The d key does the same for the sidebar cursor; this adds a
// name-addressable path.
func (m *Model) exDescribe(name string) tea.Cmd {
	return m.exOpenStructureTab(name, seTabColumns)
}

// exOpenStructureTab opens the structure panel on a specific tab for a table
// (:columns / :indexes / :fk / :constraints / :describe). Shares openSchemaPanel
// with the d key and :describe. Accepts schema.table for foreign namespaces.
func (m *Model) exOpenStructureTab(name string, tab int) tea.Cmd {
	table := m.resolveTableArg(name)
	if table == "" {
		return nil
	}
	if schema, tbl := splitTableRef(table); schema != "" {
		m.ensureSchemaSectionExpanded(schema)
		m.syncSidebarCursorToQualified(schema, tbl)
	} else {
		m.syncSidebarCursorToTable(table)
	}
	cmd := m.openSchemaPanelFor(table)
	if m.schemaEditor.IsVisible() {
		m.schemaEditor.SetActiveTab(tab)
	}
	return cmd
}

// exListNames shows a single-column lookup overlay of names (tables, views,
// schemas). When jumpable is true, Enter on a row opens that name as a table.
func (m *Model) exListNames(title string, names []string, jumpable bool) tea.Cmd {
	cols := []db.Column{{Name: "Name"}}
	rows := make([][]string, 0, len(names))
	for _, n := range names {
		rows = append(rows, []string{n})
	}
	var jumps []string
	if jumpable {
		jumps = append([]string(nil), names...)
	}
	return func() tea.Msg {
		return lookupResultMsg{
			title:  title,
			result: db.Result{Columns: cols, Rows: rows},
			jumps:  jumps,
		}
	}
}

// exTables lists base tables in the lookup overlay (:tables / :dt). Views are
// excluded when Views() succeeds so the two list verbs stay distinct.
func (m *Model) exTables() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	viewSet := map[string]bool{}
	if views, err := m.connection.DB().Views(); err == nil {
		for _, v := range views {
			viewSet[v] = true
		}
	}
	names := make([]string, 0, len(m.tables))
	for _, t := range m.tables {
		if !viewSet[t] {
			names = append(names, t)
		}
	}
	if len(names) == 0 {
		m.schemaMsg = "no tables"
		return nil
	}
	return m.exListNames("Tables", names, true)
}

// exSizes lists base tables with row and on-disk size estimates in the lookup
// overlay (:sizes). Sorted largest-first by disk, then rows. Row counts are
// prefixed with ~ when the driver reports a catalog estimate.
func (m *Model) exSizes() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		sizes, err := conn.DB().TableSizes()
		if err != nil {
			return lookupResultMsg{err: err}
		}
		if len(sizes) == 0 {
			return lookupResultMsg{
				title:  "Table sizes",
				result: db.Result{Columns: []db.Column{{Name: "Table"}, {Name: "Rows"}, {Name: "Disk"}}},
			}
		}
		sortTableSizes(sizes)
		rows := make([][]string, len(sizes))
		jumps := make([]string, len(sizes))
		for i, ts := range sizes {
			rows[i] = []string{ts.Name, formatTableSizeRows(ts), db.FormatTableDiskSize(ts.DiskBytes)}
			jumps[i] = ts.Name
		}
		return lookupResultMsg{
			title: "Table sizes",
			result: db.Result{
				Columns: []db.Column{{Name: "Table"}, {Name: "Rows"}, {Name: "Disk"}},
				Rows:    rows,
			},
			jumps: jumps,
		}
	}
}

// sortTableSizes orders sizes by disk (desc), then rows (desc), then name.
func sortTableSizes(sizes []db.TableSize) {
	sort.Slice(sizes, func(i, j int) bool {
		di, dj := diskSortKey(sizes[i].DiskBytes), diskSortKey(sizes[j].DiskBytes)
		if di != dj {
			return di > dj
		}
		ri, rj := rowSortKey(sizes[i].Rows), rowSortKey(sizes[j].Rows)
		if ri != rj {
			return ri > rj
		}
		return sizes[i].Name < sizes[j].Name
	})
}

func diskSortKey(n int64) int64 {
	if n < 0 {
		return -1
	}
	return n
}

func rowSortKey(n int64) int64 {
	if n < 0 {
		return -1
	}
	return n
}

func formatTableSizeRows(ts db.TableSize) string {
	if ts.Rows < 0 {
		return "—"
	}
	s := formatCount(int(ts.Rows))
	if ts.RowsApprox {
		return "~" + s
	}
	return s
}

// exLocks lists sessions waiting on locks held by other sessions (:locks /
// :blocked). Opens the lookup overlay. Enter jumps to the locked relation when
// it looks like a table name. Use :kill <pid> on a blocking pid to terminate it.
func (m *Model) exLocks() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		waits, err := conn.DB().Locks()
		if err != nil {
			return lookupResultMsg{err: err}
		}
		cols := []db.Column{
			{Name: "Waiter"},
			{Name: "Blocked by"},
			{Name: "Wait"},
			{Name: "Relation"},
			{Name: "Query"},
		}
		if len(waits) == 0 {
			return lookupResultMsg{
				title:  "Lock waits",
				result: db.Result{Columns: cols},
			}
		}
		rows := make([][]string, len(waits))
		jumps := make([]string, len(waits))
		for i, w := range waits {
			rows[i] = []string{
				db.FormatLockWaiter(w.WaitingPID, w.WaitingUser),
				db.FormatLockBlocker(w.BlockingPID, w.BlockingUser, w.BlockingState),
				w.WaitDuration,
				w.Relation,
				db.TruncateQuery(w.WaitingQuery, 72),
			}
			jumps[i] = lockJumpTable(w.Relation)
		}
		title := fmt.Sprintf("Lock waits (%d)", len(waits))
		return lookupResultMsg{
			title:  title,
			result: db.Result{Columns: cols, Rows: rows},
			jumps:  jumps,
		}
	}
}

// lockJumpTable returns a bare table name for Enter-to-open when relation is
// "schema.table" or "table"; otherwise "".
func lockJumpTable(relation string) string {
	relation = strings.TrimSpace(relation)
	if relation == "" {
		return ""
	}
	if i := strings.LastIndex(relation, "."); i >= 0 {
		relation = relation[i+1:]
	}
	if relation == "" || strings.ContainsAny(relation, " \t") {
		return ""
	}
	return relation
}

// exWho lists live database sessions (:who / :sessions) in the lookup overlay.
// Use :kill <pid> on a pid from the list (not Creel's own "· you" row).
func (m *Model) exWho() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	conn := m.connection
	return func() tea.Msg {
		sessions, err := conn.DB().Sessions()
		if err != nil {
			return lookupResultMsg{err: err}
		}
		cols := []db.Column{
			{Name: "PID"},
			{Name: "User"},
			{Name: "State"},
			{Name: "Age"},
			{Name: "Query"},
		}
		if len(sessions) == 0 {
			return lookupResultMsg{
				title:  "Sessions",
				result: db.Result{Columns: cols},
			}
		}
		rows := make([][]string, len(sessions))
		for i, s := range sessions {
			state := s.State
			if s.Waiting && state != "" {
				state = state + " · waiting"
			} else if s.Waiting {
				state = "waiting"
			}
			rows[i] = []string{
				db.FormatSessionPID(s.PID, s.Self),
				s.User,
				state,
				s.Age,
				db.TruncateQuery(s.Query, 72),
			}
		}
		return lookupResultMsg{
			title:  fmt.Sprintf("Sessions (%d)", len(sessions)),
			result: db.Result{Columns: cols, Rows: rows},
		}
	}
}

// exKill terminates a database session by pid (:kill <pid>). Gated by
// read-only mode and confirm_destructive unless forced (:kill!).
func (m *Model) exKill(pid string, force bool) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: kill disabled"
		return nil
	}
	pid = strings.TrimSpace(pid)
	if pid == "" {
		m.schemaMsg = ":kill needs a session pid"
		return nil
	}
	if !force && m.confirmDestructive() {
		m.killConfirm = pid
		return nil
	}
	return m.execKill(pid)
}

func (m *Model) execKill(pid string) tea.Cmd {
	conn := m.connection
	return func() tea.Msg {
		err := conn.DB().KillSession(pid)
		return killDoneMsg{pid: pid, err: err}
	}
}

// exViews lists views in the lookup overlay (:views / :dv).
func (m *Model) exViews() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	views, err := m.connection.DB().Views()
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	if len(views) == 0 {
		m.schemaMsg = "no views"
		return nil
	}
	return m.exListNames("Views", views, true)
}

// exSchemasList lists schemas/namespaces in the lookup overlay (:schemas).
// Distinct from :schema [name], which switches (or status-lists) the active
// schema. SQLite is unsupported.
func (m *Model) exSchemasList() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	schemas, err := m.connection.Schemas()
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	if len(schemas) == 0 {
		m.schemaMsg = "no schemas"
		return nil
	}
	return m.exListNames("Schemas", schemas, false)
}

// searchHit is one row in the :search / :find lookup overlay.
type searchHit struct {
	kind   string // "table", "view", "column"
	name   string
	parent string // table for columns; empty otherwise
}

// exGrep opens the cross-table cell-value search popup (:grep), optionally
// prefilling and running a query. Distinct from :search/:find (schema names)
// and from results g/ (in-page regex). Same overlay as sidebar S.
func (m *Model) exGrep(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if len(m.tables) == 0 {
		m.schemaMsg = "no tables to search"
		return nil
	}
	query := strings.TrimSpace(strings.Join(args, " "))
	m.crossSearch.Show()
	if query != "" {
		m.crossSearch.SetQuery(query)
		m.layoutWorkspace()
		return m.startCrossSearch()
	}
	m.layoutWorkspace()
	return nil
}

// exSearch fuzzy-finds tables, views, and columns by name (:search / :find).
// Uses cached sidebar metadata (m.tables + columnCache); distinct from the
// results g / regex and from cross-search (cell values).
func (m *Model) exSearch(needle string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if needle == "" {
		m.schemaMsg = ":search needs a name"
		return nil
	}

	var hits []searchHit
	viewSet := map[string]bool{}
	if views, err := m.connection.DB().Views(); err == nil {
		for _, v := range views {
			viewSet[v] = true
		}
	}

	for _, t := range m.tables {
		kind := "table"
		if viewSet[t] {
			kind = "view"
		}
		hits = append(hits, searchHit{kind: kind, name: t})
	}
	for table, cols := range m.columnCache {
		for _, c := range cols {
			hits = append(hits, searchHit{kind: "column", name: c.Name, parent: table})
		}
	}

	ranked := fuzzyRank(needle, hits, func(h searchHit) string {
		if h.parent != "" {
			return h.parent + "." + h.name
		}
		return h.name
	}, nil)
	if len(ranked) == 0 {
		m.schemaMsg = fmt.Sprintf("no matches for %q", needle)
		return nil
	}

	cols := []db.Column{{Name: "Kind"}, {Name: "Name"}, {Name: "Parent"}}
	rows := make([][]string, 0, len(ranked))
	jumps := make([]string, 0, len(ranked))
	for _, r := range ranked {
		rows = append(rows, []string{r.Item.kind, r.Item.name, r.Item.parent})
		switch r.Item.kind {
		case "table", "view":
			jumps = append(jumps, r.Item.name)
		case "column":
			jumps = append(jumps, r.Item.parent)
		default:
			jumps = append(jumps, "")
		}
	}
	title := fmt.Sprintf("Search: %s", needle)
	return func() tea.Msg {
		return lookupResultMsg{
			title:  title,
			result: db.Result{Columns: cols, Rows: rows},
			jumps:  jumps,
		}
	}
}

// exStats shows summary statistics for a column (:stats [column]), defaulting
// to the cursor column. With an argument it first moves the cursor to that
// column (case-insensitive exact match), so fetchColumnStats — which reads the
// cursor column — summarizes it.
func (m *Model) exStats(arg string) tea.Cmd {
	if m.results.NumRows() == 0 || m.connection == nil {
		m.schemaMsg = "no results to summarize"
		return nil
	}
	if arg != "" {
		idx := -1
		for i := 0; i < m.results.NumCols(); i++ {
			if strings.EqualFold(m.results.ColumnName(i), arg) {
				idx = i
				break
			}
		}
		if idx < 0 {
			m.schemaMsg = fmt.Sprintf("no such column: %s", arg)
			return nil
		}
		m.results.SetCursor(m.results.CursorRow(), idx)
	}
	return m.fetchColumnStats()
}

// exBar opens a horizontal bar chart in the results slot. Columns come from
// args (`:bar label value [sum|count|avg]`) or from ordered column marks (M).
// One column (named or a single mark) counts distinct values. Duplicate
// labels are grouped. The current page supplies the rows unless force
// (`:bar!`) re-runs lastQuery without the page LIMIT.
func (m *Model) exBar(args []string, force bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}

	labelCol, valueCol, agg, err := m.resolveBarColumns(args)
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	label := m.results.ColumnName(labelCol)
	value := m.results.ColumnName(valueCol)
	title := fmt.Sprintf("bar · %s × %s · %s", label, value, agg)
	emptyErr := "no numeric values in " + value
	if agg == barAggCount && labelCol == valueCol {
		title = fmt.Sprintf("bar · %s · count", label)
		emptyErr = "no values to chart"
	}
	return m.runChart(chartSpec{
		kind:     chartKindBar,
		agg:      agg,
		colNames: []string{label, value},
		title:    title,
		emptyErr: emptyErr,
	}, force)
}

// exFreq opens a frequency bar chart of one column (count of each distinct
// column. `:freq!` re-runs lastQuery without the page LIMIT.
func (m *Model) exFreq(args []string, force bool) tea.Cmd {
	return m.exFreqLike(args, force, chartKindBar, "freq")
}

// exPie opens a pie chart of one column (same counts as :freq). The column
// comes from args, a single column mark, or the cursor column. `:pie!`
// re-runs lastQuery without the page LIMIT.
func (m *Model) exPie(args []string, force bool) tea.Cmd {
	return m.exFreqLike(args, force, chartKindPie, "pie")
}

func (m *Model) exFreqLike(args []string, force bool, kind chartKind, prefix string) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	col, err := m.resolveFreqColumn(args)
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	name := m.results.ColumnName(col)
	return m.runChart(chartSpec{
		kind:     kind,
		agg:      barAggCount,
		colNames: []string{name, name},
		title:    fmt.Sprintf("%s · %s", prefix, name),
		emptyErr: "no values to chart",
	}, force)
}

// exLine opens a line chart in the results slot. Columns come from args
// (`:line x y`) or from the two ordered column marks (M: x, then y).
// `:line!` re-runs lastQuery without the page LIMIT.
func (m *Model) exLine(args []string, force bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	xCol, yCol, err := m.resolveXYColumns(args, "line")
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	xName := m.results.ColumnName(xCol)
	yName := m.results.ColumnName(yCol)
	title := fmt.Sprintf("line · %s × %s", xName, yName)
	return m.runChart(chartSpec{
		kind:     chartKindLine,
		colNames: []string{xName, yName},
		title:    title,
		emptyErr: "no numeric or datetime x/y pairs in " + xName + " × " + yName,
	}, force)
}

// exScatter opens a scatter chart in the results slot. Columns come from
// args (`:scatter x y`) or from the two ordered column marks (M: x, then y).
// `:scatter!` re-runs lastQuery without the page LIMIT.
func (m *Model) exScatter(args []string, force bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	xCol, yCol, err := m.resolveXYColumns(args, "scatter")
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	xName := m.results.ColumnName(xCol)
	yName := m.results.ColumnName(yCol)
	title := fmt.Sprintf("scatter · %s × %s", xName, yName)
	return m.runChart(chartSpec{
		kind:     chartKindScatter,
		colNames: []string{xName, yName},
		title:    title,
		emptyErr: "no numeric or datetime x/y pairs in " + xName + " × " + yName,
	}, force)
}

// exHist opens a histogram of one numeric column. The column comes from
// args, a single column mark, or the cursor column. bins is optional
// (Sturges, clamped 8–20). `:hist!` re-runs lastQuery without the page LIMIT.
func (m *Model) exHist(args []string, force bool) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no results to chart"
		return nil
	}
	col, bins, err := m.resolveHistColumn(args)
	if err != "" {
		m.schemaMsg = err
		return nil
	}
	name := m.results.ColumnName(col)
	title := fmt.Sprintf("hist · %s", name)
	if bins > 0 {
		title += fmt.Sprintf(" · %d bins", bins)
	}
	return m.runChart(chartSpec{
		kind:     chartKindHist,
		colNames: []string{name},
		bins:     bins,
		title:    title,
		emptyErr: "no numeric values in " + name,
	}, force)
}

// resolveXYColumns picks x/y column indices from :line / :scatter args or
// from ordered column marks.
func (m *Model) resolveXYColumns(args []string, verb string) (xCol, yCol int, err string) {
	find := m.resultColumnIndex
	switch len(args) {
	case 0:
		marked := m.results.MarkedColumns()
		if len(marked) != 2 {
			return 0, 0, fmt.Sprintf("mark 2 columns with M (x, then y), or :%s <x> <y>", verb)
		}
		return marked[0], marked[1], ""
	case 1:
		return 0, 0, fmt.Sprintf("usage: :%s <x> <y> (or mark 2 columns with M)", verb)
	default:
		xCol = find(args[0])
		if xCol < 0 {
			return 0, 0, fmt.Sprintf("no such column: %s", args[0])
		}
		yCol = find(args[1])
		if yCol < 0 {
			return 0, 0, fmt.Sprintf("no such column: %s", args[1])
		}
		return xCol, yCol, ""
	}
}

// resolveHistColumn picks a numeric column and an optional bin count from
// :hist args. With no column name, a single M mark wins, else the cursor
// column (same as :stats). A lone numeric arg that is not a column name is
// the bin count.
func (m *Model) resolveHistColumn(args []string) (col, bins int, err string) {
	defaultCol := func() (int, string) {
		marked := m.results.MarkedColumns()
		switch len(marked) {
		case 0:
			return m.results.CursorCol(), ""
		case 1:
			return marked[0], ""
		default:
			return 0, "mark 1 column with M, or :hist <column> [bins]"
		}
	}
	parseBins := func(s string) (int, string) {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 {
			return 0, fmt.Sprintf("invalid bin count: %s", s)
		}
		if n > 100 {
			n = 100
		}
		return n, ""
	}

	switch len(args) {
	case 0:
		col, err = defaultCol()
		return col, 0, err
	case 1:
		if idx := m.resultColumnIndex(args[0]); idx >= 0 {
			return idx, 0, ""
		}
		if _, e := strconv.Atoi(args[0]); e == nil {
			n, err := parseBins(args[0])
			if err != "" {
				return 0, 0, err
			}
			col, err = defaultCol()
			return col, n, err
		}
		return 0, 0, fmt.Sprintf("no such column: %s", args[0])
	default:
		col = m.resultColumnIndex(args[0])
		if col < 0 {
			return 0, 0, fmt.Sprintf("no such column: %s", args[0])
		}
		n, err := parseBins(args[1])
		return col, n, err
	}
}

// resolveFreqColumn picks the column for :freq / :pie. With no name, a single M mark
// wins, else the cursor column (same as :hist / :stats).
func (m *Model) resolveFreqColumn(args []string) (col int, err string) {
	defaultCol := func() (int, string) {
		marked := m.results.MarkedColumns()
		switch len(marked) {
		case 0:
			return m.results.CursorCol(), ""
		case 1:
			return marked[0], ""
		default:
			return 0, "mark 1 column with M, or :freq/:pie <column>"
		}
	}
	switch len(args) {
	case 0:
		return defaultCol()
	case 1:
		if idx := m.resultColumnIndex(args[0]); idx >= 0 {
			return idx, ""
		}
		return 0, fmt.Sprintf("no such column: %s", args[0])
	default:
		return 0, "usage: :freq [column] (or :pie [column])"
	}
}

// resolveBarColumns picks label/value column indices and an aggregate from
// :bar args or from ordered column marks. A single column (named or marked)
// is a frequency count. Returns a user-facing error string on failure.
func (m *Model) resolveBarColumns(args []string) (labelCol, valueCol int, agg barAgg, err string) {
	find := m.resultColumnIndex
	freq := func(col int) (int, int, barAgg, string) {
		return col, col, barAggCount, ""
	}
	fromMarks := func(a barAgg, explicit bool) (int, int, barAgg, string) {
		marked := m.results.MarkedColumns()
		switch len(marked) {
		case 1:
			if !explicit || a == barAggCount {
				return freq(marked[0])
			}
			return 0, 0, 0, "mark 2 columns with M (label, then value), or :bar <label> <value> [sum|count|avg]"
		case 2:
			return marked[0], marked[1], a, ""
		default:
			return 0, 0, 0, "mark 2 columns with M (label, then value), :bar <label> to count, or :bar <label> <value> [sum|count|avg]"
		}
	}
	fromNames := func(label, value string, a barAgg) (int, int, barAgg, string) {
		lc := find(label)
		if lc < 0 {
			return 0, 0, 0, fmt.Sprintf("no such column: %s", label)
		}
		vc := find(value)
		if vc < 0 {
			return 0, 0, 0, fmt.Sprintf("no such column: %s", value)
		}
		return lc, vc, a, ""
	}

	switch len(args) {
	case 0:
		return fromMarks(barAggSum, false)
	case 1:
		if a, ok := parseBarAgg(args[0]); ok {
			return fromMarks(a, true)
		}
		if idx := find(args[0]); idx >= 0 {
			return freq(idx)
		}
		return 0, 0, 0, fmt.Sprintf("no such column: %s", args[0])
	case 2:
		// `:bar status count` is a one-column frequency unless `count` is
		// also a real column (then it stays label × value).
		if a, ok := parseBarAgg(args[1]); ok && find(args[1]) < 0 {
			lc := find(args[0])
			if lc < 0 {
				return 0, 0, 0, fmt.Sprintf("no such column: %s", args[0])
			}
			if a != barAggCount {
				return 0, 0, 0, "one-column :bar only supports count (or pass a value column)"
			}
			return freq(lc)
		}
		return fromNames(args[0], args[1], barAggSum)
	default:
		a, ok := parseBarAgg(args[2])
		if !ok {
			return 0, 0, 0, fmt.Sprintf("unknown aggregate: %s (try sum, count, or avg)", args[2])
		}
		return fromNames(args[0], args[1], a)
	}
}

// exTheme switches to a named theme (:theme <name>), applying it live and
// persisting the choice — the non-interactive counterpart of the g c picker.
// The name must match a known theme (case-insensitive).
func (m *Model) exTheme(name string) tea.Cmd {
	resolved := ""
	for _, t := range themeNames() {
		if strings.EqualFold(t, name) {
			resolved = t
			break
		}
	}
	if resolved == "" {
		m.schemaMsg = fmt.Sprintf("no such theme: %s", name)
		return nil
	}
	m.settings.Theme = resolved
	if m.config != nil {
		m.config.Settings.Theme = resolved
		_ = m.config.Save()
	}
	applyTheme(resolved, m.settings.ThemeOverrides)
	m.schemaMsg = "theme: " + resolved
	return nil
}

// exIcons switches the tree expand/collapse glyph set (:icons <unicode|nerdfont>),
// applying it live and persisting the choice to config — the same shape as
// exTheme. "unicode" (or "default") restores the portable triangles and
// clears the stored value (so it omits from YAML); "nerdfont" uses Nerd Font
// angle chevrons (U+F105/U+F107), which need a Nerd Font in the terminal.
func (m *Model) exIcons(name string) tea.Cmd {
	resolved, ok := resolveIconSet(name)
	if !ok {
		m.schemaMsg = fmt.Sprintf("unknown icon set: %s (try :icons unicode or :icons nerdfont)", name)
		return nil
	}
	m.settings.Icons = resolved
	if m.config != nil {
		m.config.Settings.Icons = resolved
		_ = m.config.Save()
	}
	applyIcons(resolved)
	label := resolved
	if label == "" {
		label = "unicode"
	}
	m.schemaMsg = "icons: " + label
	return nil
}
