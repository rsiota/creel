package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// exportScope selects which rows an export writes. It is independent of column
// selection (handled by the cols argument to exportResults): scope picks the
// row set, cols picks the columns.
type exportScope int

const (
	// scopePage exports the rows currently held in memory (the visible page).
	// It is the only option for result sets with no backing table, and the
	// behaviour of the instant `x` key.
	scopePage exportScope = iota
	// scopeAll re-queries the source table (SELECT cols FROM t, no LIMIT) so
	// the export is not capped at the page size. Only available when the
	// results back a known table.
	scopeAll
	// scopeMarked re-queries the marked rows by primary key (SELECT cols FROM
	// t WHERE pk IN (...)), so marks that span multiple pages are exported in
	// full. Only available when rows are marked on an editable table.
	scopeMarked
)

// scopeOpt is one selectable row in the Scope section.
type scopeOpt struct {
	scope exportScope
	label string
}

// exportSection identifies one of the three logical groups in the overlay.
type exportSection int

const (
	secFormat exportSection = iota
	secColumns
	secScope
)

// exportEntry is one selectable (non-header) row, pointing back to its
// section and the index within that section's option list. The overlay keeps a
// flat slice of these so navigation is a single cursor over a 1-D list while
// rendering still groups them under headers.
type exportEntry struct {
	kind exportSection
	idx  int
}

// ExportOverlay is the `g X` export dialog: a single navigable list with three
// sections — Format (radio), Columns (checkbox), and Scope (radio). It
// replaces the old FormatPicker, folding column selection and row scope
// (whole-table vs. page vs. marked) into one place.
//
// Selection model: radio sections (Format, Scope) track a chosen index
// separately from the cursor, so traversing a section with the cursor never
// silently changes the selection. Columns track a per-column checked bit; the
// cursor only positions the toggle.
type ExportOverlay struct {
	visible bool
	width   int
	height  int

	formats []exportFormat // static list (exportFormats)
	columns []string       // result columns
	scopes  []scopeOpt     // dynamic, built in Show

	selectedFmt  exportFormat // chosen format (mirrors lastFmt on open)
	lastFmt      exportFormat // remembered across opens
	colChecked   []bool       // per column
	selectedScope int         // chosen scope index into scopes

	entries  []exportEntry // flat selectable list (format, then columns, then scope)
	cursor   int           // index into entries
	scrollRow int          // display scroll offset (content lines)
}

// NewExportOverlay returns an overlay defaulting to CSV.
func NewExportOverlay() ExportOverlay {
	return ExportOverlay{formats: exportFormats, lastFmt: fmtCSV}
}

// Show populates the overlay for the current result set and reveals it.
//
//   - columns    every column in the result set (all checked by default)
//   - hasSourceTable whether the results back a known table (enables Whole table)
//   - markCount  marked row count (enables Marked rows when > 0)
//   - pageCount  rows on the current page (shown in the Current page label)
//   - totalRows  total table rows if known (totalRowsSet), else 0
func (o *ExportOverlay) Show(columns []string, hasSourceTable bool, markCount, pageCount int, totalRows int, totalRowsSet bool) {
	o.columns = append(o.columns[:0], columns...)
	o.colChecked = make([]bool, len(columns))
	for i := range o.colChecked {
		o.colChecked[i] = true
	}

	// Build the Scope section adaptively. Order is stable: Whole table,
	// Marked rows, Current page — each shown only when it applies.
	o.scopes = o.scopes[:0]
	if hasSourceTable {
		label := "Whole table (all rows)"
		if totalRowsSet && totalRows > 0 {
			label = fmt.Sprintf("Whole table (%d)", totalRows)
		}
		o.scopes = append(o.scopes, scopeOpt{scopeAll, label})
	}
	if markCount > 0 {
		o.scopes = append(o.scopes, scopeOpt{scopeMarked, fmt.Sprintf("Marked rows (%d)", markCount)})
	}
	o.scopes = append(o.scopes, scopeOpt{scopePage, fmt.Sprintf("Current page (%d)", pageCount)})

	// Default scope: marked if any, else whole table if available, else page.
	o.selectedScope = indexofScope(o.scopes, scopePage)
	if markCount > 0 {
		o.selectedScope = indexofScope(o.scopes, scopeMarked)
	} else if hasSourceTable {
		o.selectedScope = indexofScope(o.scopes, scopeAll)
	}

	o.selectedFmt = o.lastFmt

	// Flat selectable list: formats, then columns, then scopes.
	o.entries = o.entries[:0]
	for i := range o.formats {
		o.entries = append(o.entries, exportEntry{secFormat, i})
	}
	for i := range o.columns {
		o.entries = append(o.entries, exportEntry{secColumns, i})
	}
	for i := range o.scopes {
		o.entries = append(o.entries, exportEntry{secScope, i})
	}

	o.cursor = 0 // top of the list (first format) — enter = whole-table CSV
	o.scrollRow = 0
	o.visible = true
}

func indexofScope(scopes []scopeOpt, s exportScope) int {
	for i, opt := range scopes {
		if opt.scope == s {
			return i
		}
	}
	return 0
}

// Hide closes the overlay.
func (o *ExportOverlay) Hide() { o.visible = false }

// IsVisible reports whether the overlay is shown.
func (o ExportOverlay) IsVisible() bool { return o.visible }

// SetSize sets the rendering dimensions.
func (o *ExportOverlay) SetSize(width, height int) {
	o.width = width
	o.height = height
}

// CursorUp moves the cursor up, clamped at the first entry.
func (o *ExportOverlay) CursorUp() {
	if o.cursor > 0 {
		o.cursor--
	}
	o.adjustScroll()
}

// CursorDown moves the cursor down, clamped at the last entry.
func (o *ExportOverlay) CursorDown() {
	if o.cursor < len(o.entries)-1 {
		o.cursor++
	}
	o.adjustScroll()
}

// Activate performs the context-sensitive action for the row under the cursor:
// select the format, select the scope, or toggle the column. Toggling the last
// checked column is refused so an export never has zero columns.
func (o *ExportOverlay) Activate() {
	if o.cursor < 0 || o.cursor >= len(o.entries) {
		return
	}
	e := o.entries[o.cursor]
	switch e.kind {
	case secFormat:
		o.selectedFmt = o.formats[e.idx]
	case secScope:
		o.selectedScope = e.idx
	case secColumns:
		checked := 0
		for _, c := range o.colChecked {
			if c {
				checked++
			}
		}
		if o.colChecked[e.idx] && checked <= 1 {
			return // never leave zero columns selected
		}
		o.colChecked[e.idx] = !o.colChecked[e.idx]
	}
}

// SelectAllCols checks every column.
func (o *ExportOverlay) SelectAllCols() {
	for i := range o.colChecked {
		o.colChecked[i] = true
	}
}

// SelectNoneCols unchecks every column except the first (an export needs at
// least one column).
func (o *ExportOverlay) SelectNoneCols() {
	for i := range o.colChecked {
		o.colChecked[i] = false
	}
	if len(o.colChecked) > 0 {
		o.colChecked[0] = true
	}
}

// Commit hides the overlay and returns the chosen format, columns, and scope.
// columns is nil when every column is selected (meaning "all" to exportResults,
// which skips projection and preserves column order exactly).
func (o *ExportOverlay) Commit() (exportFormat, []string, exportScope) {
	format := o.selectedFmt
	o.lastFmt = format
	scope := scopePage
	if o.selectedScope >= 0 && o.selectedScope < len(o.scopes) {
		scope = o.scopes[o.selectedScope].scope
	}
	var cols []string
	for i, c := range o.columns {
		if i < len(o.colChecked) && o.colChecked[i] {
			cols = append(cols, c)
		}
	}
	if len(cols) == len(o.columns) {
		cols = nil // all → let exportResults use the full set
	}
	o.visible = false
	return format, cols, scope
}

// displayRow is one rendered line in the overlay: either a header/blank/footer
// (entry=false) or a selectable row (entry=true, with its entry index).
type displayRow struct {
	text  string
	entry bool
}

func renderExportHeader(label string) string {
	return lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(label)
}

// renderExportRow renders one selectable row: a leading marker (› for the
// cursor row, else two spaces), a state glyph (● selected/checked, ○ not), and
// the label. The cursor row gets a solid highlight bar like the other pickers.
func renderExportRow(glyph, label string, isCursor bool, width int) string {
	label = truncateRunes(label, width)
	if isCursor {
		return lipgloss.NewStyle().
			Background(colorPrimary).Foreground(colorBg).
			Render("› " + glyph + " " + label)
	}
	gc := lipgloss.NewStyle().Foreground(colorMuted)
	if glyph == "●" {
		gc = gc.Foreground(colorFg)
	}
	return "  " + gc.Render(glyph) + " " + lipgloss.NewStyle().Foreground(colorFg).Render(label)
}

// contentWidth is the usable width for row labels inside the panel: panel width
// minus borders (2) minus padding (2) minus the leading "› ● " prefix (4).
func (o ExportOverlay) contentWidth() int {
	w := o.width - 2 - 2 - 4
	if w < 8 {
		w = 8
	}
	return w
}

// maxVisibleRows is the number of content lines that fit inside the panel.
func (o ExportOverlay) maxVisibleRows() int {
	h := o.height - 2 // top + bottom border
	if h < 1 {
		h = 1
	}
	return h
}

// adjustScroll keeps the cursor entry visible within the scrolling window.
func (o *ExportOverlay) adjustScroll() {
	maxRows := o.maxVisibleRows()
	if maxRows < 1 {
		o.scrollRow = 0
		return
	}
	curDisplay := o.cursorDisplayRow()
	if curDisplay < o.scrollRow {
		o.scrollRow = curDisplay
	}
	if curDisplay >= o.scrollRow+maxRows {
		o.scrollRow = curDisplay - maxRows + 1
	}
}

// cursorDisplayRow returns the display-line index (including headers/blanks)
// of the entry under the cursor, so adjustScroll can keep it in view.
func (o ExportOverlay) cursorDisplayRow() int {
	// Display layout: header(1) + formats + blank(1) + header(1) + columns +
	// blank(1) + header(1) + scopes + blank(1) + footer(1).
	row := 1 // "Format" header
	nFmt := len(o.formats)
	nCols := len(o.columns)
	e := o.entries[o.cursor]
	switch e.kind {
	case secFormat:
		row += e.idx
	case secColumns:
		row += nFmt + 1 /*blank*/ + 1 /*header*/ + e.idx
	case secScope:
		row += nFmt + 1 + 1 + nCols + 1 /*blank*/ + 1 /*header*/ + e.idx
	}
	return row
}

// View renders the overlay panel.
func (o ExportOverlay) View() string {
	if !o.visible {
		return ""
	}

	labelW := o.contentWidth()
	var rows []displayRow
	entryIdx := 0
	addHeader := func(s string) { rows = append(rows, displayRow{text: renderExportHeader(s)}) }
	addBlank := func() { rows = append(rows, displayRow{}) }
	addEntry := func(glyph, label string) {
		isCursor := entryIdx == o.cursor
		rows = append(rows, displayRow{
			text:  renderExportRow(glyph, label, isCursor, labelW),
			entry: true,
		})
		entryIdx++
	}

	addHeader("Format")
	for _, f := range o.formats {
		glyph := "○"
		if f == o.selectedFmt {
			glyph = "●"
		}
		addEntry(glyph, exportFormatLabel[f])
	}
	addBlank()
	checked := 0
	for _, c := range o.colChecked {
		if c {
			checked++
		}
	}
	addHeader(fmt.Sprintf("Columns (%d/%d)", checked, len(o.columns)))
	for i, c := range o.columns {
		glyph := "○"
		if i < len(o.colChecked) && o.colChecked[i] {
			glyph = "●"
		}
		addEntry(glyph, c)
	}
	addBlank()
	addHeader("Scope")
	for i, s := range o.scopes {
		glyph := "○"
		if i == o.selectedScope {
			glyph = "●"
		}
		addEntry(glyph, s.label)
	}
	addBlank()

	// Window the rows if they exceed the available height.
	maxRows := o.maxVisibleRows()
	start := o.scrollRow
	if len(rows) > maxRows {
		if start < 0 {
			start = 0
		}
		if start > len(rows)-maxRows {
			start = len(rows) - maxRows
		}
	}
	end := start + maxRows
	if end > len(rows) {
		end = len(rows)
	}
	visible := rows[start:end]

	content := strings.Join(displayTexts(visible), "\n")

	panel := lipgloss.NewStyle().
		Width(o.width-2).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)

	return panel
}

func displayTexts(rows []displayRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.text
	}
	return out
}
