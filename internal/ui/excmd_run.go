package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/version"
)

// exWrite commits staged cell edits (:w).
func (m *Model) exWrite(force bool) tea.Cmd {
	_ = force // :w! is accepted for parity; saves don't prompt otherwise
	if !m.results.HasDirtyCells() {
		m.schemaMsg = "no changes to save"
		return nil
	}
	return m.saveEdits()
}

// exQuit closes the active tab (:q), or quits the app when it is the last tab
// — mirroring vim, where :q on the final window exits. (The q / ctrl+q keys
// quit unconditionally; :q reaches the same path once no tabs remain.) Unsaved
// edits block unless forced (:q!).
func (m *Model) exQuit(force bool) tea.Cmd {
	if !force && m.results.HasDirtyCells() {
		m.schemaMsg = "unsaved changes — use :q! to discard"
		return nil
	}
	if len(m.resultsTabs) <= 1 {
		m.beginQuit()
		return tea.Quit
	}
	m.closeTab(m.activeTabID)
	return nil
}

// exQuitAll quits the app after closing every tab (:qa). Unsaved edits in any
// tab block unless forced (:qa!). Dirty state is checked across all tabs via
// saveTabState so inactive tabs are included.
func (m *Model) exQuitAll(force bool) tea.Cmd {
	m.saveTabState()
	if !force {
		for _, tab := range m.resultsTabs {
			if tab.Results.HasDirtyCells() {
				m.schemaMsg = "unsaved changes — use :qa! to discard"
				return nil
			}
		}
	}
	m.beginQuit()
	return tea.Quit
}

// exRun executes the statement under the cursor (:run / :r). Shares
// executeQuery with ctrl+e / \.
func (m *Model) exRun() tea.Cmd {
	if m.editor.StatementAtCursor() == "" {
		m.schemaMsg = "nothing to run"
		return nil
	}
	return m.executeQuery()
}

// exRunAll executes every statement in the editor buffer in order (:runall),
// stopping on the first error. Shares executeAllQueries with :source.
func (m *Model) exRunAll() tea.Cmd {
	return m.executeAllQueries(m.editor.Value())
}

// exSource runs every statement in the editor (:source) or from a .sql file
// (:source <path>). File contents are executed without replacing the editor
// buffer; errors report the failing statement index (no cursor jump into a
// buffer that doesn't contain the file).
func (m *Model) exSource(args []string) tea.Cmd {
	if len(args) == 0 {
		return m.exRunAll()
	}
	expanded, err := expandTilde(filepath.Clean(args[0]))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	content, err := os.ReadFile(expanded)
	if err != nil {
		m.schemaMsg = "read failed: " + err.Error()
		return nil
	}
	m.schemaMsg = fmt.Sprintf("sourcing %s", expanded)
	return m.executeAllQueries(string(content))
}

// exTabNew opens a new results tab with the current editor contents, matching
// the bare `t` key in results/connections.
func (m *Model) exTabNew() tea.Cmd {
	query := m.editor.Value()
	m.addTab(generateTabTitle(query), query)
	return nil
}

// exTabClose closes the active tab (:tabclose). Unlike :q, the last tab is
// refused (vim :tabclose) — use :q to quit. Unsaved edits block unless forced.
func (m *Model) exTabClose(force bool) tea.Cmd {
	if len(m.resultsTabs) <= 1 {
		m.schemaMsg = "cannot close the last tab — use :q to quit"
		return nil
	}
	if !force && m.results.HasDirtyCells() {
		m.schemaMsg = "unsaved changes — use :tabclose! to discard"
		return nil
	}
	m.closeTab(m.activeTabID)
	return nil
}

// exTabNext activates the next tab (cyclic), sharing TabBar.NextTab with g t.
func (m *Model) exTabNext() tea.Cmd {
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	if nextID := m.tabBar.NextTab(); nextID >= 0 {
		m.setActiveTab(nextID)
	}
	return nil
}

// exTabPrev activates the previous tab (cyclic), sharing TabBar.PrevTab with g T.
func (m *Model) exTabPrev() tea.Cmd {
	m.tabBar.SetTabs(m.resultsTabs, m.activeTabID)
	if prevID := m.tabBar.PrevTab(); prevID >= 0 {
		m.setActiveTab(prevID)
	}
	return nil
}

// exTabs lists open tabs in the status bar, marking the active one with [].
func (m *Model) exTabs() tea.Cmd {
	if len(m.resultsTabs) == 0 {
		m.schemaMsg = "no tabs"
		return nil
	}
	parts := make([]string, 0, len(m.resultsTabs))
	for i, tab := range m.resultsTabs {
		title := tab.Title
		if title == "" {
			title = "untitled"
		}
		label := fmt.Sprintf("%d:%s", i+1, title)
		if tab.ID == m.activeTabID {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	m.schemaMsg = strings.Join(parts, "  ")
	return nil
}

// exDiff compares the loaded result pages of two tabs (:diff [a] [b]).
// Tab numbers are 1-based, same as :tabs. With no args, diffs the previous tab
// against the active one; with one arg, diffs the active tab against that
// number; with two args, diffs those two. Schema diff is intentionally not
// supported.
func (m *Model) exDiff(args []string) tea.Cmd {
	if len(m.resultsTabs) < 2 {
		m.schemaMsg = "diff needs at least two tabs — :tabnew, then load results in each"
		return nil
	}
	m.saveTabState()

	left, right, errMsg := m.resolveDiffTabs(args)
	if errMsg != "" {
		m.schemaMsg = errMsg
		return nil
	}
	a := snapshotFromTab(m.resultsTabs[left])
	b := snapshotFromTab(m.resultsTabs[right])
	if !a.hasRows() || !b.hasRows() {
		m.schemaMsg = "both tabs need a loaded result page"
		return nil
	}
	if left == right {
		m.schemaMsg = "pick two different tabs"
		return nil
	}
	d := computeResultDiff(a, b)
	m.diffPanel.Show(d)
	m.schemaMsg = fmt.Sprintf("diff %d:%s → %d:%s  %s",
		left+1, a.title, right+1, b.title, d.summary())
	return nil
}

// resolveDiffTabs maps :diff args to 0-based indexes into resultsTabs.
func (m *Model) resolveDiffTabs(args []string) (left, right int, errMsg string) {
	active := -1
	for i, tab := range m.resultsTabs {
		if tab.ID == m.activeTabID {
			active = i
			break
		}
	}
	if active < 0 {
		active = 0
	}

	parseIdx := func(s string) (int, string) {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 || n > len(m.resultsTabs) {
			return -1, fmt.Sprintf("bad tab %q — use 1..%d (see :tabs)", s, len(m.resultsTabs))
		}
		return n - 1, ""
	}

	switch len(args) {
	case 0:
		// Previous tab (cyclic) vs active.
		left = active - 1
		if left < 0 {
			left = len(m.resultsTabs) - 1
		}
		right = active
		return left, right, ""
	case 1:
		right, errMsg = parseIdx(args[0])
		if errMsg != "" {
			return 0, 0, errMsg
		}
		return active, right, ""
	default:
		left, errMsg = parseIdx(args[0])
		if errMsg != "" {
			return 0, 0, errMsg
		}
		right, errMsg = parseIdx(args[1])
		if errMsg != "" {
			return 0, 0, errMsg
		}
		return left, right, ""
	}
}

// exCopy copies the cell under the cursor to the clipboard (:copy). Shares
// copyCursorCell with the yy chord.
func (m *Model) exCopy() tea.Cmd {
	return m.copyCursorCell()
}

// exDiscard discards staged cell edits (:discard), sharing discardResultsEdits
// with the results D key. Stages the y/enter confirmation when
// confirm_destructive is on. Gives feedback when there is nothing to discard
// (the key stays silent).
func (m *Model) exDiscard(force bool) tea.Cmd {
	if m.discardResultsEdits(force) {
		return nil
	}
	if !m.results.IsEditable() {
		m.schemaMsg = "nothing to discard — results not editable"
	} else {
		m.schemaMsg = "no changes to discard"
	}
	return nil
}

// exClone duplicates the marked rows or the cursor row (:clone), sharing
// cloneRows with the results P key. Gives feedback for the no-op cases the key
// swallows silently (no connection / nothing editable).
func (m *Model) exClone() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if !m.results.IsEditable() || m.results.NumRows() == 0 {
		m.schemaMsg = "no editable rows to clone"
		return nil
	}
	return m.cloneRows()
}

// exFollow follows the foreign key under the cursor (:follow), sharing
// followForeignKey with the g d chord. Gives feedback when there is no FK on
// the current column (the key silently no-ops). From the inspector, the
// focused field is the current column.
func (m *Model) exFollow() tea.Cmd {
	if m.results.NumCols() == 0 {
		m.schemaMsg = "no results to navigate"
		return nil
	}
	col := m.results.CursorCol()
	if m.focus == FocusInspector && m.insertTarget == nil && !m.inspector.IsInserting() {
		col = m.inspector.selectedColumn(m.results)
	}
	if _, ok := m.results.ForeignKeyAt(col); !ok {
		m.schemaMsg = "no foreign key on this column"
		return nil
	}
	return m.followForeignKeyAt(col)
}

// exBack returns to the previous query in the navigation stack (:back),
// sharing goBackQuery with the g b chord.
func (m *Model) exBack() tea.Cmd {
	if len(m.queryStack) == 0 {
		m.schemaMsg = "nowhere to go back to"
		return nil
	}
	return m.goBackQuery()
}

// exKeep keeps only rows equal to the cursor cell (:keep); exHide is its
// inverse. Both share quickFilterCell with the * / ! keys.
func (m *Model) exKeep() tea.Cmd { return m.exQuickFilter(false) }
func (m *Model) exHide() tea.Cmd { return m.exQuickFilter(true) }

func (m *Model) exQuickFilter(negate bool) tea.Cmd {
	if !m.canFilter() || m.results.NumRows() == 0 {
		m.schemaMsg = "no rows to filter"
		return nil
	}
	if m.results.CursorCellValue() == "" {
		m.schemaMsg = "cursor cell is empty — move to a value first"
		return nil
	}
	return m.quickFilterCell(negate)
}

// exUndo removes the last filter (:undo); exUnfilter clears all of them
// (:unfilter). Both share undoFilter / clearFilters with the u / c keys.
func (m *Model) exUndo() tea.Cmd {
	if len(m.filters) == 0 {
		m.schemaMsg = "no filters to undo"
		return nil
	}
	return m.undoFilter()
}

func (m *Model) exUnfilter() tea.Cmd {
	if len(m.filters) == 0 {
		m.schemaMsg = "no filters to clear"
		return nil
	}
	return m.clearFilters()
}

// exCopyInsert copies the current result rows as INSERT statements to the
// clipboard (:copyinsert), sharing copyRowsAsInsert with the Y key.
func (m *Model) exCopyInsert() tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no rows to copy"
		return nil
	}
	if cmd := m.copyRowsAsInsert(); cmd != nil {
		return cmd
	}
	m.schemaMsg = "nothing to copy"
	return nil
}

// exCopyRow copies the marked rows (or the cursor row when none are marked)
// to the clipboard as TSV by default, or a specified format
// (:copyrow csv|tsv|md|json|jsonl), sharing copyRowsDelimited.
func (m *Model) exCopyRow(args []string) tea.Cmd {
	format := fmtTSV
	if len(args) > 0 {
		f, ok := parseExportFormat(args[0])
		if !ok {
			m.schemaMsg = ":copyrow format must be one of: csv, json, jsonl, md, tsv"
			return nil
		}
		format = f
	}
	return m.copyRowsDelimited(format)
}

// exRegex applies a regex search to the current page (:regex <pattern>),
// sharing applySearch with the g/ search mode. Patterns may contain spaces.
func (m *Model) exRegex(args []string) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.schemaMsg = "no rows to search"
		return nil
	}
	pattern := strings.Join(args, " ")
	if strings.TrimSpace(pattern) == "" {
		m.schemaMsg = ":regex needs a pattern"
		return nil
	}
	m.applySearch(pattern)
	return nil
}

// exHideColumn hides a column (:hidecolumn [col]). Defaults to the column
// under the cursor; with a name it hides that column. Mirrors the H key.
func (m *Model) exHideColumn(args []string) tea.Cmd {
	if m.results.NumCols() == 0 {
		m.schemaMsg = "no columns to hide"
		return nil
	}
	col := m.results.CursorCol()
	if len(args) > 0 {
		found := -1
		for i := 0; i < m.results.NumCols(); i++ {
			if strings.EqualFold(m.results.ColumnName(i), args[0]) {
				found = i
				break
			}
		}
		if found < 0 {
			m.schemaMsg = fmt.Sprintf("no such column: %s", args[0])
			return nil
		}
		col = found
	}
	if !m.results.HideColumn(col) {
		m.schemaMsg = "column already hidden or is the last visible column"
		return nil
	}
	return nil
}

// exShowColumns reveals all hidden columns (:showcolumns), mirroring g H.
func (m *Model) exShowColumns() tea.Cmd {
	m.results.ShowAllColumns()
	return nil
}

// exNew clears the editor to an empty scratch buffer (:new). Does not open a
// new tab — use :tabnew for that.
func (m *Model) exNew() tea.Cmd {
	m.editor.SetValue("")
	m.schemaMsg = "new buffer"
	return m.editor.Focus()
}

// exVersion prints the build version in the status bar (:version).
func (m *Model) exVersion() tea.Cmd {
	m.schemaMsg = version.String()
	return nil
}

// exMenu opens the contextual action menu for the focused panel (:menu) —
// same helper as g m, usable from the editor where typed g-chords cannot
// steal vim's g.
func (m *Model) exMenu() tea.Cmd {
	m.openActionMenu()
	return nil
}

// exRecent lists or re-opens recently touched tables (:recent [n|name]).
// Tables are recorded by openTable (:goto, sidebar enter, mouse). Bare lists
// them in the lookup overlay; a number opens by MRU rank (1 = most recent);
// a name opens if it appears in the recent list.
func (m *Model) exRecent(args []string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	names := m.liveRecentTables()
	if len(names) == 0 {
		m.schemaMsg = "no recent tables"
		return nil
	}
	if len(args) == 0 {
		return m.exListNames("Recent", names, true)
	}
	arg := args[0]
	if n, err := strconv.Atoi(arg); err == nil {
		if n < 1 || n > len(names) {
			m.schemaMsg = fmt.Sprintf("recent rank out of range (1-%d)", len(names))
			return nil
		}
		return m.openTable(names[n-1])
	}
	name := resolveNameInList(arg, names)
	if name == "" {
		m.schemaMsg = fmt.Sprintf("not in recent: %s", arg)
		return nil
	}
	return m.openTable(name)
}
