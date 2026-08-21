package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
	"github.com/rsiota/creel/internal/db"
)

const maxCellWidth = 40

const (
	copyFlashInterval   = 150 // milliseconds between flash toggles
	copyFlashTickCount  = 6   // three on/off cycles
	copyMessageDuration = 2   // seconds to show "copied to clipboard"
)

// cellRef identifies a single cell by row and column index.
type cellRef struct {
	row int
	col int
}

// ResultsTable renders query results as a scrollable table with
// per-cell truncation and horizontal scrolling. When editable, it
// supports a cell cursor and inline editing.
type ResultsTable struct {
	columns []string
	rows    [][]string
	rawRows [][]string // un-sanitized rows, preserving newlines/tabs for the cell viewer
	// blobs holds raw binary cell data keyed by (row, col). Display uses a
	// "<BLOB …>" placeholder in rows; save-to-file and INSERT literals read here.
	blobs     map[db.BlobKey][]byte
	scrollRow int
	scrollCol int
	colWidths []int
	width     int
	height    int
	message   string
	hasResult bool

	// Inline editing
	editable        bool               // can these results be edited?
	sourceTable     string             // table backing these results
	pkColumns       []string           // primary key column names
	cursorRow       int                // highlighted cell cursor row
	cursorCol       int                // highlighted cell cursor col
	editing         bool               // inline edit mode active
	editInput       textinput.Model    // input buffer for the cell being edited
	dirtyCells      map[cellRef]string // pending unsaved edits (new values)
	saved           bool               // all dirty cells were saved (show confirmation)
	saveError       string             // last save error
	copied          bool               // show clipboard copy confirmation
	copyFlash       cellRef
	copyFlashActive bool
	copyFlashOn     bool
	copyFlashTicks  int
	resultTable     string
	foreignKeys     map[string]db.ForeignKey // keyed by lowercase column name
	columnTypes     map[string]string        // column name -> database type (for inspector)
	tableColumns    []db.TableColumnInfo
	sortCol         string // column currently sorted by ("" = none)
	sortDir         string // "ASC" or "DESC"

	// Row marks (staging area for building WHERE pk IN (...) filters).
	// keyed by joined PK values; markedTable guards staleness so marks
	// survive same-table re-queries but auto-invalidate on table change.
	markedRows  map[string][]string
	markedTable string

	// Column marks (ordered) for charting — e.g. :bar uses the first marked
	// column as labels and the second as values. Indices into columns;
	// cleared when the result set is replaced. Cap is maxColumnMarks.
	markedCols []int

	// Hidden columns (display-only). Keyed by column name so the hidden
	// set survives same-table re-queries; hiddenTable guards staleness like
	// markedRows. Hiding never touches the data layer — editing, sort,
	// stats, and export always see the full column set.
	hiddenCols  map[string]bool
	hiddenTable string

	// Client-side search highlight: matcher is set while/after g/ search so
	// View() can tint matching cells. nil when no search is active.
	searchMatcher func(string) bool

	// watchDelta marks rows whose content is new since the previous :watch /
	// :tail refresh (fingerprinted). Cleared on the next SetResult / Clear.
	watchDelta map[int]bool

	// Visual mode (line-wise selection). When visualActive is true, the range
	// [min(visualAnchor, cursorRow), max(visualAnchor, cursorRow)] is
	// highlighted and can be committed to marks with enter.
	visualActive bool
	visualAnchor int

	// borderColor is the colour used for the table's box-drawing border.
	// When non-empty it overrides the default colorBorder, allowing the
	// caller to reflect focus state.
	borderColor lipgloss.Color
}

// NewResultsTable creates a new results table component.
func NewResultsTable() ResultsTable {
	return ResultsTable{}
}

// SetEditable configures the table for inline editing.
func (r *ResultsTable) SetEditable(table string, pkCols []string) {
	r.editable = true
	r.sourceTable = table
	r.pkColumns = pkCols
	r.cursorRow = 0
	r.cursorCol = 0
	r.dirtyCells = make(map[cellRef]string)
	r.editing = false
	r.saveError = ""
	r.ClearVisualMode()
	r.computeColWidths()
}

// SetTableColumns stores schema metadata for inserts.
func (r *ResultsTable) SetTableColumns(cols []db.TableColumnInfo) {
	r.tableColumns = cols
}

// TableColumns returns schema metadata for the editable table.
func (r ResultsTable) TableColumns() []db.TableColumnInfo {
	return r.tableColumns
}

// IsAutoIncrementCol reports whether a column is an auto-increment primary key.
func (r ResultsTable) IsAutoIncrementCol(col int) bool {
	info, ok := r.columnInfo(col)
	return ok && info.AutoIncrement
}

func (r ResultsTable) columnInfo(col int) (db.TableColumnInfo, bool) {
	name := r.ColumnName(col)
	for _, c := range r.tableColumns {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return db.TableColumnInfo{}, false
}

// ClearForeignKeys removes foreign-key navigation metadata.
func (r *ResultsTable) ClearForeignKeys() {
	r.resultTable = ""
	r.foreignKeys = nil
}

// SetForeignKeys records outbound foreign keys for the current result table.
func (r *ResultsTable) SetForeignKeys(table string, fks []db.ForeignKey) {
	r.resultTable = table
	r.foreignKeys = make(map[string]db.ForeignKey)
	colSet := make(map[string]bool, len(r.columns))
	for _, c := range r.columns {
		colSet[strings.ToLower(c)] = true
	}
	for _, fk := range fks {
		key := strings.ToLower(fk.Column)
		if colSet[key] {
			r.foreignKeys[key] = fk
		}
	}
}

// HasForeignKeys returns whether the current results expose FK navigation.
func (r ResultsTable) HasForeignKeys() bool {
	return len(r.foreignKeys) > 0
}

// ForeignKeyAt returns the foreign key for a column index, if any.
func (r ResultsTable) ForeignKeyAt(col int) (db.ForeignKey, bool) {
	name := strings.ToLower(r.ColumnName(col))
	fk, ok := r.foreignKeys[name]
	return fk, ok
}

// ForeignKeyAtCursor returns the foreign key for the cell under the cursor.
func (r ResultsTable) ForeignKeyAtCursor() (db.ForeignKey, bool) {
	return r.ForeignKeyAt(r.cursorCol)
}

// IsNavigableForeignKey reports whether a cell can be followed via gd.
func (r ResultsTable) IsNavigableForeignKey(row, col int) bool {
	if _, ok := r.ForeignKeyAt(col); !ok {
		return false
	}
	val := r.RowValue(row, col)
	return val != "" && val != "NULL"
}

// IsCellTruncated reports whether the value at (row, col) is wider than the
// rendered column cell (i.e. shown with an ellipsis). This is the condition
// under which the inline editor is replaced by the expanded cell popup.
func (r ResultsTable) IsCellTruncated(row, col int) bool {
	if row < 0 || row >= len(r.rows) || col < 0 || col >= len(r.colWidths) {
		return false
	}
	w := r.colWidths[col]
	if r.IsNavigableForeignKey(row, col) {
		w -= 2 // " →" arrow suffix
	}
	if w < 1 {
		w = 1
	}
	return runeLen(sanitizeCellValue(r.RowValue(row, col))) > w
}

// ResultTable returns the source table for the current results, if known.
func (r ResultsTable) ResultTable() string {
	return r.resultTable
}

// CursorCol returns the current cursor column index.
func (r ResultsTable) CursorCol() int {
	return r.cursorCol
}

// SetCursor moves the cell cursor and keeps it visible.
func (r *ResultsTable) SetCursor(row, col int) {
	r.cursorRow = row
	r.cursorCol = col
	r.clampCursor()
	r.ensureCursorVisible()
}

// ClearEditable disables inline editing.
func (r *ResultsTable) ClearEditable() {
	r.editable = false
	r.sourceTable = ""
	r.pkColumns = nil
	r.dirtyCells = nil
	r.editing = false
	r.tableColumns = nil
	r.ClearVisualMode()
	r.computeColWidths()
}

// IsEditable returns whether the current results support inline editing.
func (r ResultsTable) IsEditable() bool {
	return r.editable
}

// HasPrimaryKey reports whether the editable table has primary key columns.
// Row updates and deletes require a primary key; inserts do not.
func (r ResultsTable) HasPrimaryKey() bool {
	return len(r.pkColumns) > 0
}

// IsEditing returns whether a cell is currently being edited inline.
func (r ResultsTable) IsEditing() bool {
	return r.editing
}

// HasDirtyCells returns whether there are unsaved edits.
func (r ResultsTable) HasDirtyCells() bool {
	return len(r.dirtyCells) > 0
}

// DirtyCells returns all pending cell edits as a slice of (row, col, value).
func (r ResultsTable) DirtyCells() []CellEdit {
	var edits []CellEdit
	for ref, val := range r.dirtyCells {
		edits = append(edits, CellEdit{
			Row:      ref.row,
			Col:      ref.col,
			NewValue: val,
		})
	}
	return edits
}

// CellEdit represents a single pending cell modification.
type CellEdit struct {
	Row      int
	Col      int
	NewValue string
}

// ColumnName returns the column name for a given column index.
func (r ResultsTable) ColumnName(col int) string {
	if col < 0 || col >= len(r.columns) {
		return ""
	}
	return r.columns[col]
}

// ColumnNames returns a copy of all column names in the result set.
func (r ResultsTable) ColumnNames() []string {
	out := make([]string, len(r.columns))
	copy(out, r.columns)
	return out
}

// RowValue returns the current value at (row, col), accounting for dirty edits.
func (r ResultsTable) RowValue(row, col int) string {
	ref := cellRef{row: row, col: col}
	if val, ok := r.dirtyCells[ref]; ok {
		return val
	}
	if row < 0 || row >= len(r.rows) {
		return ""
	}
	if col < 0 || col >= len(r.rows[row]) {
		return ""
	}
	return r.rows[row][col]
}

// BlobData returns the raw bytes for a binary cell, if any.
func (r ResultsTable) BlobData(row, col int) ([]byte, bool) {
	if r.blobs == nil {
		return nil, false
	}
	d, ok := r.blobs[db.BlobKey{Row: row, Col: col}]
	return d, ok
}

// IsBlobCell reports whether the cell holds binary data (scanned as []byte).
func (r ResultsTable) IsBlobCell(row, col int) bool {
	_, ok := r.BlobData(row, col)
	return ok
}

// SetBlobs stores binary cell data for the current result set. Call after
// SetResult; SetResult clears any previous map.
func (r *ResultsTable) SetBlobs(blobs map[db.BlobKey][]byte) {
	r.blobs = blobs
}

// RawRowValue returns the un-sanitized cell value (preserving newlines and
// tabs) for viewers that render multi-line content, such as the cell-edit
// popup. Pending dirty edits take precedence. When raw rows aren't available
// it falls back to the sanitized value from RowValue.
func (r ResultsTable) RawRowValue(row, col int) string {
	ref := cellRef{row: row, col: col}
	if val, ok := r.dirtyCells[ref]; ok {
		return val
	}
	if row < 0 || row >= len(r.rawRows) {
		return r.RowValue(row, col)
	}
	if col < 0 || col >= len(r.rawRows[row]) {
		return r.RowValue(row, col)
	}
	return r.rawRows[row][col]
}

// SourceTable returns the table name backing editable results.
func (r ResultsTable) SourceTable() string {
	return r.sourceTable
}

// RenameTableReferences updates cached table names after a table rename.
func (r *ResultsTable) RenameTableReferences(oldName, newName string) {
	if r.sourceTable == oldName {
		r.sourceTable = newName
	}
	if r.resultTable == oldName {
		r.resultTable = newName
	}
	if r.markedTable == oldName {
		r.markedTable = newName
	}
	if r.hiddenTable == oldName {
		r.hiddenTable = newName
	}
}

// PKColumns returns the primary key column names.
func (r ResultsTable) PKColumns() []string {
	return r.pkColumns
}

// PKTypes returns the database type for each primary key column, aligned
// with PKColumns, so callers can build type-correct IN clauses.
func (r ResultsTable) PKTypes() []string {
	types := make([]string, len(r.pkColumns))
	for i, pk := range r.pkColumns {
		for j, c := range r.columns {
			if strings.EqualFold(c, pk) {
				types[i] = r.ColumnType(j)
				break
			}
		}
	}
	return types
}

// CursorPKTuple returns the PK values for the row under the cursor, or nil if
// the cursor row has no valid PK tuple.
func (r ResultsTable) CursorPKTuple() []string {
	return r.pkTuple(r.cursorRow)
}

// pkTuple returns the PK column values for a row, aligned with pkColumns.
// Returns nil if the row or any PK column is out of range.
func (r ResultsTable) pkTuple(rowIdx int) []string {
	if rowIdx < 0 || rowIdx >= len(r.rows) || len(r.pkColumns) == 0 {
		return nil
	}
	tuple := make([]string, 0, len(r.pkColumns))
	for _, pk := range r.pkColumns {
		found := false
		for j, c := range r.columns {
			if strings.EqualFold(c, pk) {
				if j < len(r.rows[rowIdx]) {
					tuple = append(tuple, r.rows[rowIdx][j])
				}
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return tuple
}

// pkKey joins a PK tuple into a stable map key (unit separator delimited).
func pkKey(tuple []string) string {
	return strings.Join(tuple, "\x1f")
}

// marksStale reports whether stored marks belong to a different table.
func (r ResultsTable) marksStale() bool {
	return r.markedTable != "" && !strings.EqualFold(r.markedTable, r.sourceTable)
}

// MarkCount returns the number of marked rows for the current table (0 if
// marks belong to a different table).
func (r ResultsTable) MarkCount() int {
	if r.marksStale() {
		return 0
	}
	return len(r.markedRows)
}

// IsMarkedRow reports whether the given row is currently marked.
func (r ResultsTable) IsMarkedRow(rowIdx int) bool {
	if r.marksStale() || len(r.markedRows) == 0 {
		return false
	}
	tuple := r.pkTuple(rowIdx)
	if tuple == nil {
		return false
	}
	_, ok := r.markedRows[pkKey(tuple)]
	return ok
}

// ToggleMark flips the mark on the cursor row. Marks are keyed by PK tuple,
// so they survive pagination and same-table re-queries. Switching tables
// invalidates them.
func (r *ResultsTable) ToggleMark() {
	if !r.editable || r.sourceTable == "" || len(r.pkColumns) == 0 {
		return
	}
	// Reset if marks are from a different (or now-empty) table.
	if r.markedTable == "" || r.marksStale() {
		r.markedRows = make(map[string][]string)
		r.markedTable = r.sourceTable
	}
	tuple := r.pkTuple(r.cursorRow)
	if tuple == nil {
		return
	}
	key := pkKey(tuple)
	if _, ok := r.markedRows[key]; ok {
		delete(r.markedRows, key)
	} else {
		r.markedRows[key] = tuple
	}
}

// MarkedPKs returns the marked PK tuples for the current table, sorted by
// their joined key for deterministic output.
func (r ResultsTable) MarkedPKs() [][]string {
	if r.marksStale() {
		return nil
	}
	keys := make([]string, 0, len(r.markedRows))
	for k := range r.markedRows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, r.markedRows[k])
	}
	return out
}

// ClearMarks removes all row marks.
func (r *ResultsTable) ClearMarks() {
	r.markedRows = nil
	r.markedTable = ""
}

// maxColumnMarks is how many columns :bar (and similar) can consume at once
// — label then value.
const maxColumnMarks = 2

// ToggleColumnMark flips the mark on the cursor column. Marks are ordered:
// the first marked column is the label axis, the second the value axis.
// At most maxColumnMarks columns may be marked; attempting a third returns
// false without changing state.
func (r *ResultsTable) ToggleColumnMark() bool {
	col := r.cursorCol
	if col < 0 || col >= len(r.columns) {
		return true
	}
	for i, c := range r.markedCols {
		if c == col {
			r.markedCols = append(r.markedCols[:i], r.markedCols[i+1:]...)
			return true
		}
	}
	if len(r.markedCols) >= maxColumnMarks {
		return false
	}
	r.markedCols = append(r.markedCols, col)
	return true
}

// ClearColumnMarks removes all column marks.
func (r *ResultsTable) ClearColumnMarks() {
	r.markedCols = nil
}

// ClearAllMarks clears both row and column marks.
func (r *ResultsTable) ClearAllMarks() {
	r.ClearMarks()
	r.ClearColumnMarks()
}

// ColumnMarkCount returns how many columns are currently marked.
func (r ResultsTable) ColumnMarkCount() int {
	return len(r.markedCols)
}

// MarkedColumns returns the ordered column indices currently marked.
func (r ResultsTable) MarkedColumns() []int {
	if len(r.markedCols) == 0 {
		return nil
	}
	out := make([]int, len(r.markedCols))
	copy(out, r.markedCols)
	return out
}

// IsMarkedColumn reports whether the given column index is marked.
func (r ResultsTable) IsMarkedColumn(col int) bool {
	for _, c := range r.markedCols {
		if c == col {
			return true
		}
	}
	return false
}

// ColumnMarkOrdinal returns the 1-based mark order for col (1 = label, 2 =
// value), or 0 if unmarked.
func (r ResultsTable) ColumnMarkOrdinal(col int) int {
	for i, c := range r.markedCols {
		if c == col {
			return i + 1
		}
	}
	return 0
}

const copyAsInsertMaxRows = 500

// CopyAsInsert builds an INSERT INTO ... statement for the marked rows (or
// all rows if none are marked), skipping hidden columns. Values are
// SQL-escaped: NULL stays NULL, numeric types are passed bare, everything
// else is single-quoted. Returns the SQL string and the row count.
func (r ResultsTable) CopyAsInsert() (string, int) {
	if len(r.columns) == 0 || len(r.rows) == 0 {
		return "", 0
	}
	table := r.sourceTable
	if table == "" {
		table = "table"
	}

	var visibleCols []int
	for i := range r.columns {
		if !r.IsColumnHidden(i) {
			visibleCols = append(visibleCols, i)
		}
	}
	if len(visibleCols) == 0 {
		return "", 0
	}

	// Determine which rows to copy.
	type markedKey struct{}
	markedSet := map[string]bool{}
	hasMarks := false
	if !r.marksStale() && len(r.markedRows) > 0 {
		hasMarks = true
		for k := range r.markedRows {
			markedSet[k] = true
		}
	}

	var b strings.Builder
	colNames := make([]string, len(visibleCols))
	for j, ci := range visibleCols {
		colNames[j] = r.columns[ci]
	}
	b.WriteString("INSERT INTO ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" (")
	b.WriteString(strings.Join(colNames, ", "))
	b.WriteString(") VALUES\n")

	count := 0
	for rowIdx := range r.rows {
		if hasMarks {
			tuple := r.pkTuple(rowIdx)
			if tuple == nil {
				continue
			}
			if !markedSet[pkKey(tuple)] {
				continue
			}
		}
		if count >= copyAsInsertMaxRows {
			break
		}
		if count > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("  (")
		for j, ci := range visibleCols {
			if j > 0 {
				b.WriteString(", ")
			}
			if data, ok := r.BlobData(rowIdx, ci); ok {
				b.WriteString(db.BlobSQLLiteral(data, r.columnType(ci)))
			} else {
				val := r.RowValue(rowIdx, ci)
				b.WriteString(sqlEscape(val, r.columnType(ci)))
			}
		}
		b.WriteString(")")
		count++
	}
	b.WriteString(";")
	return b.String(), count
}

// CopyAsDelimited renders the marked rows (or the cursor row when none are
// marked) in a delimited format (csv/tsv/md/json/jsonl) for clipboard copy.
// Only visible columns are included, mirroring CopyAsInsert. Returns the
// serialized text and the number of rows copied (0 when there is nothing to
// copy). NULL cells keep their "NULL" sentinel text in the output, consistent
// with the grid.
func (r ResultsTable) CopyAsDelimited(format exportFormat) (string, int) {
	visibleCols := r.visibleColRange()
	if len(visibleCols) == 0 || len(r.rows) == 0 {
		return "", 0
	}

	// Select rows: marked rows if any, otherwise the cursor row. Unlike
	// CopyAsInsert (which falls back to the whole page), the delimited copy is
	// for grabbing a row or two into a spreadsheet/chat — dumping the whole
	// page would be surprising and is already covered by :export / g X.
	var selected []int
	if !r.marksStale() && len(r.markedRows) > 0 {
		for rowIdx := range r.rows {
			tuple := r.pkTuple(rowIdx)
			if tuple == nil {
				continue
			}
			if _, marked := r.markedRows[pkKey(tuple)]; !marked {
				continue
			}
			selected = append(selected, rowIdx)
		}
	} else if r.cursorRow >= 0 && r.cursorRow < len(r.rows) {
		selected = []int{r.cursorRow}
	}
	if len(selected) == 0 {
		return "", 0
	}

	cols := make([]string, len(visibleCols))
	for j, ci := range visibleCols {
		cols[j] = r.columns[ci]
	}
	rows := make([][]string, len(selected))
	for i, rowIdx := range selected {
		row := make([]string, len(visibleCols))
		for j, ci := range visibleCols {
			row[j] = r.RowValue(rowIdx, ci)
		}
		rows[i] = row
	}

	content, err := serializeFormat(format, cols, rows)
	if err != nil {
		return "", 0
	}
	return content, len(selected)
}

func (r ResultsTable) columnType(col int) string {
	name := r.columns[col]
	if typ := r.columnTypes[name]; typ != "" {
		return typ
	}
	if info, ok := r.columnInfo(col); ok {
		return info.Type
	}
	return ""
}

func quoteIdent(name string) string {
	return "\"" + strings.ReplaceAll(name, "\"", "\"\"") + "\""
}

// sqlEscape renders a value as a SQL literal. Only the "NULL" sentinel maps
// to SQL NULL; a genuine empty string becomes ” (not NULL) so real empty
// strings round-trip correctly through :copyinsert / copy-as-INSERT. Numeric
// types pass bare; everything else is single-quoted with embedded quotes
// doubled.
func sqlEscape(val, typ string) string {
	if val == "NULL" {
		return "NULL"
	}
	if val == "" {
		return "''"
	}
	if isNumericType(typ) {
		return val
	}
	// Heuristic: bare integers/floats without special chars pass unquoted.
	if typ == "" {
		if isBareNumeric(val) {
			return val
		}
	}
	return "'" + strings.ReplaceAll(val, "'", "''") + "'"
}

func isBareNumeric(s string) bool {
	if s == "" {
		return false
	}
	hasDigit := false
	for i, c := range s {
		if c >= '0' && c <= '9' {
			hasDigit = true
			continue
		}
		if c == '.' && i > 0 {
			continue
		}
		if c == '-' && i == 0 {
			continue
		}
		return false
	}
	return hasDigit
}

// CloneRow represents a single row's column values ready for cloning.
type CloneRow struct {
	Values map[string]string
	// Blobs holds raw binary values keyed by column name. When present for a
	// column, buildInsertQuery uses the bytes instead of Values[name] (which
	// would be the "<BLOB …>" display placeholder).
	Blobs map[string][]byte
}

// CloneRowsData returns the rows to clone: marked rows if any exist,
// otherwise the cursor row. Auto-increment PK values are omitted so
// the database assigns new IDs. Hidden columns are also omitted.
// Returns the table name, column schema, and the row data.
func (r ResultsTable) CloneRowsData() (string, []db.TableColumnInfo, []CloneRow) {
	if r.sourceTable == "" || len(r.rows) == 0 || len(r.tableColumns) == 0 {
		return "", nil, nil
	}

	// Determine which rows to clone.
	markedSet := map[string]bool{}
	hasMarks := false
	if !r.marksStale() && len(r.markedRows) > 0 {
		hasMarks = true
		for k := range r.markedRows {
			markedSet[k] = true
		}
	}

	// Build a set of hidden column names.
	hiddenNames := map[string]bool{}
	for i, col := range r.columns {
		if r.IsColumnHidden(i) {
			hiddenNames[strings.ToLower(col)] = true
		}
	}

	// Build column name → row index map for value lookup.
	colIdx := map[string]int{}
	for i, c := range r.columns {
		colIdx[strings.ToLower(c)] = i
	}

	var rows []CloneRow
	for rowIdx := range r.rows {
		if hasMarks {
			tuple := r.pkTuple(rowIdx)
			if tuple == nil || !markedSet[pkKey(tuple)] {
				continue
			}
		} else if rowIdx != r.cursorRow {
			continue
		}

		vals := make(map[string]string)
		var rowBlobs map[string][]byte
		for _, tc := range r.tableColumns {
			if tc.AutoIncrement {
				continue
			}
			if hiddenNames[strings.ToLower(tc.Name)] {
				continue
			}
			if idx, ok := colIdx[strings.ToLower(tc.Name)]; ok {
				if data, ok := r.BlobData(rowIdx, idx); ok {
					if rowBlobs == nil {
						rowBlobs = make(map[string][]byte)
					}
					rowBlobs[tc.Name] = data
					vals[tc.Name] = r.RowValue(rowIdx, idx) // placeholder; overridden via Blobs
					continue
				}
				vals[tc.Name] = r.RowValue(rowIdx, idx)
			}
		}
		rows = append(rows, CloneRow{Values: vals, Blobs: rowBlobs})
	}

	return r.sourceTable, r.tableColumns, rows
}

// SetVisualMode activates line-wise visual selection anchored at the cursor row.
func (r *ResultsTable) SetVisualMode() {
	r.visualActive = true
	r.visualAnchor = r.cursorRow
}

// ClearVisualMode deactivates visual selection.
func (r *ResultsTable) ClearVisualMode() {
	r.visualActive = false
}

// IsVisualMode reports whether visual selection is active.
func (r ResultsTable) IsVisualMode() bool {
	return r.visualActive
}

// VisualRange returns the inclusive [lo, hi] row indices of the visual
// selection. Returns 0, 0 when visual mode is not active.
func (r ResultsTable) VisualRange() (int, int) {
	if !r.visualActive {
		return 0, 0
	}
	lo := r.visualAnchor
	hi := r.cursorRow
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo, hi
}

// VisualRangeSize returns the number of rows in the visual selection (0 if
// visual mode is not active).
func (r ResultsTable) VisualRangeSize() int {
	if !r.visualActive {
		return 0
	}
	lo, hi := r.VisualRange()
	return hi - lo + 1
}

// isVisualRow reports whether rowIdx falls within the active visual range.
func (r ResultsTable) isVisualRow(rowIdx int) bool {
	if !r.visualActive {
		return false
	}
	lo, hi := r.VisualRange()
	return rowIdx >= lo && rowIdx <= hi
}

// hiddenStale reports whether stored hidden columns belong to a different table.
func (r ResultsTable) hiddenStale() bool {
	return r.hiddenTable != "" && !strings.EqualFold(r.hiddenTable, r.sourceTable)
}

// hiddenSet returns the live hidden-cols map, initializing it (and binding it
// to the current table) on first use or when a new table is detected.
func (r *ResultsTable) hiddenSet() map[string]bool {
	if r.hiddenTable == "" || r.hiddenStale() {
		r.hiddenCols = make(map[string]bool)
		r.hiddenTable = r.sourceTable
	}
	return r.hiddenCols
}

// HiddenCount returns the number of hidden columns for the current table.
func (r ResultsTable) HiddenCount() int {
	if r.hiddenStale() || len(r.hiddenCols) == 0 {
		return 0
	}
	n := 0
	for _, c := range r.columns {
		if r.hiddenCols[c] {
			n++
		}
	}
	return n
}

// IsColumnHidden reports whether the given column index is hidden.
func (r ResultsTable) IsColumnHidden(col int) bool {
	if r.hiddenStale() || len(r.hiddenCols) == 0 || col < 0 || col >= len(r.columns) {
		return false
	}
	return r.hiddenCols[r.columns[col]]
}

// VisibleColumnCount returns the number of currently visible columns.
func (r ResultsTable) VisibleColumnCount() int {
	return len(r.columns) - r.HiddenCount()
}

// nextVisibleCol returns the first visible column at or after col, or -1.
func (r ResultsTable) nextVisibleCol(col int) int {
	for i := col; i < len(r.columns); i++ {
		if !r.IsColumnHidden(i) {
			return i
		}
	}
	return -1
}

// prevVisibleCol returns the first visible column at or before col, or -1.
func (r ResultsTable) prevVisibleCol(col int) int {
	for i := col; i >= 0; i-- {
		if !r.IsColumnHidden(i) {
			return i
		}
	}
	return -1
}

// HideColumn hides the column at the given index. It refuses to hide the last
// visible column so the table is never empty. Returns false if it was a no-op.
func (r *ResultsTable) HideColumn(col int) bool {
	if col < 0 || col >= len(r.columns) {
		return false
	}
	if r.VisibleColumnCount() <= 1 {
		return false
	}
	hs := r.hiddenSet()
	name := r.columns[col]
	if hs[name] {
		return false
	}
	hs[name] = true
	r.clampCursor()
	r.ensureCursorVisible()
	return true
}

// ShowColumn makes the column at the given index visible again.
func (r *ResultsTable) ShowColumn(col int) {
	if col < 0 || col >= len(r.columns) {
		return
	}
	if r.hiddenStale() {
		return
	}
	delete(r.hiddenCols, r.columns[col])
}

// ShowAllColumns clears all hidden columns for the current table.
func (r *ResultsTable) ShowAllColumns() {
	r.hiddenCols = nil
	r.hiddenTable = ""
}

// HiddenColumnNames returns the names of currently hidden columns, in column
// order. Used to initialize the column-visibility overlay.
func (r ResultsTable) HiddenColumnNames() []string {
	var out []string
	if r.hiddenStale() || len(r.hiddenCols) == 0 {
		return out
	}
	for _, c := range r.columns {
		if r.hiddenCols[c] {
			out = append(out, c)
		}
	}
	return out
}

// SetHiddenColumns replaces the hidden set with the given column names. Names
// not present in the result set are ignored. Hiding all columns is rejected.
func (r *ResultsTable) SetHiddenColumns(names []string) {
	hs := r.hiddenSet()
	for k := range hs {
		delete(hs, k)
	}
	present := make(map[string]bool, len(r.columns))
	for _, c := range r.columns {
		present[c] = true
	}
	for _, n := range names {
		if present[n] {
			hs[n] = true
		}
	}
	if len(hs) >= len(r.columns) {
		// Never hide everything.
		for k := range hs {
			delete(hs, k)
		}
	}
	r.clampCursor()
	r.ensureCursorVisible()
}

// SetSearchMatcher installs a matcher used by View() to tint matching cells.
// Pass nil to clear highlighting.
func (r *ResultsTable) SetSearchMatcher(matcher func(string) bool) {
	r.searchMatcher = matcher
}

// ConfirmSaved clears dirty cells and shows a confirmation message.
func (r *ResultsTable) ConfirmSaved() {
	r.dirtyCells = make(map[cellRef]string)
	r.saved = true
	r.saveError = ""
	r.copied = false
}

// ApplySavedEdits moves dirty cell values into the displayed and raw row
// buffers, then clears the dirty map. Display rows are sanitized; raw rows
// keep the exact saved text for viewers like the cell-edit popup.
func (r *ResultsTable) ApplySavedEdits() {
	for ref, val := range r.dirtyCells {
		if ref.row >= 0 && ref.row < len(r.rows) &&
			ref.col >= 0 && ref.col < len(r.rows[ref.row]) {
			r.rows[ref.row][ref.col] = sanitizeCellValue(val)
		}
		if ref.row >= 0 && ref.row < len(r.rawRows) &&
			ref.col >= 0 && ref.col < len(r.rawRows[ref.row]) {
			r.rawRows[ref.row][ref.col] = val
		}
	}
	r.ConfirmSaved()
}

// StartCopyFeedback marks the current cell as copied and begins a flash animation.
func (r *ResultsTable) StartCopyFeedback() {
	r.copied = true
	r.copyFlash = cellRef{row: r.cursorRow, col: r.cursorCol}
	r.copyFlashActive = true
	r.copyFlashOn = true
	r.copyFlashTicks = copyFlashTickCount
}

// AdvanceCopyFlash toggles the copy flash state. It returns whether more ticks remain.
func (r *ResultsTable) AdvanceCopyFlash() bool {
	if !r.copyFlashActive || r.copyFlashTicks <= 0 {
		return false
	}
	r.copyFlashTicks--
	r.copyFlashOn = !r.copyFlashOn
	if r.copyFlashTicks <= 0 {
		r.copyFlashActive = false
		return false
	}
	return true
}

// ClearCopiedMessage hides the clipboard confirmation in the status line.
func (r *ResultsTable) ClearCopiedMessage() {
	r.copied = false
}

func (r ResultsTable) isCopyFlashCell(row, col int) bool {
	return r.copyFlashActive && r.copyFlash.row == row && r.copyFlash.col == col
}

// DiscardEdits clears all pending cell edits.
func (r *ResultsTable) DiscardEdits() {
	r.dirtyCells = make(map[cellRef]string)
	r.saved = false
	r.saveError = ""
}

// SetSaveError records a save error message.
func (r *ResultsTable) SetSaveError(msg string) {
	r.saveError = msg
	r.saved = false
}

// SetResult populates the table with query results.
func (r *ResultsTable) SetResult(cols []string, rows [][]string, message string) {
	// Keep the raw rows for viewers that render multi-line content (the
	// cell-edit popup); the grid display reads the sanitized copy below.
	r.rawRows = rows
	r.blobs = nil
	// Flatten control characters to spaces so they don't break the
	// single-line cell layout (multi-line TEXT fields, tabs, etc.).
	cols = sanitizeCellRow(cols)
	sanitized := make([][]string, len(rows))
	for i, row := range rows {
		sanitized[i] = sanitizeCellRow(row)
	}
	r.columns = cols
	r.rows = sanitized
	r.message = message
	r.hasResult = true
	r.scrollRow = 0
	r.scrollCol = 0
	r.colWidths = nil
	r.cursorRow = 0
	r.cursorCol = 0
	r.dirtyCells = make(map[cellRef]string)
	r.editing = false
	r.saved = false
	r.saveError = ""
	r.copied = false
	r.copyFlashActive = false
	r.watchDelta = nil
	r.ClearColumnMarks()
	r.ClearForeignKeys()
	r.computeColWidths()
}

// SetError displays an error message in the results area.
func (r *ResultsTable) SetError(err string) {
	r.columns = nil
	r.rows = nil
	r.rawRows = nil
	r.blobs = nil
	r.message = err
	r.hasResult = true
}

// Clear resets the table to empty.
func (r *ResultsTable) Clear() {
	r.columns = nil
	r.rows = nil
	r.rawRows = nil
	r.blobs = nil
	r.message = ""
	r.hasResult = false
	r.scrollRow = 0
	r.scrollCol = 0
	r.colWidths = nil
	r.cursorRow = 0
	r.cursorCol = 0
	r.dirtyCells = make(map[cellRef]string)
	r.editing = false
	r.copied = false
	r.copyFlashActive = false
	r.watchDelta = nil
	r.ClearColumnMarks()
	r.ClearForeignKeys()
}

// SetWatchDelta marks rows that changed since the previous :watch / :tail
// refresh so View can tint them. Pass nil to clear.
func (r *ResultsTable) SetWatchDelta(delta map[int]bool) {
	r.watchDelta = delta
}

// IsWatchDeltaRow reports whether rowIdx is highlighted as a watch change.
func (r ResultsTable) IsWatchDeltaRow(rowIdx int) bool {
	return r.watchDelta[rowIdx]
}
func (r ResultsTable) Message() string {
	return r.message
}

// HasResult returns whether the table has data to display.
func (r ResultsTable) HasResult() bool {
	return r.hasResult
}

// CursorRow returns the current cursor row index (editable mode).
func (r ResultsTable) CursorRow() int {
	return r.cursorRow
}

// ScrollRow returns the current vertical scroll position.
func (r ResultsTable) ScrollRow() int {
	return r.scrollRow
}

// NumRows returns the number of data rows.
func (r ResultsTable) NumRows() int {
	return len(r.rows)
}

// NumCols returns the number of columns.
func (r ResultsTable) NumCols() int {
	return len(r.columns)
}

// ColumnAtX maps a relative X coordinate (0 = left border of the results
// panel) to a visible column index, or -1 if the X falls outside any column.
func (r ResultsTable) ColumnAtX(relX int) int {
	if relX < 1 {
		return -1
	}
	offset := 1 // skip left border
	for _, i := range r.visibleColRange() {
		cellW := r.colWidths[i] + 2 // padding + value
		if relX < offset+cellW+1 {  // include trailing separator
			return i
		}
		offset += cellW + 1
	}
	return -1
}

// IsDirty returns whether a cell has a pending unsaved edit.
func (r ResultsTable) IsDirty(row, col int) bool {
	_, ok := r.dirtyCells[cellRef{row: row, col: col}]
	return ok
}

// DirtyCellCount returns the number of pending unsaved edits.
func (r ResultsTable) DirtyCellCount() int {
	return len(r.dirtyCells)
}

// IsCopied reports whether the copy-to-clipboard confirmation is showing.
func (r ResultsTable) IsCopied() bool { return r.copied }

// IsSaved reports whether the save confirmation is showing.
func (r ResultsTable) IsSaved() bool { return r.saved }

// SaveError returns the last save error message, if any.
func (r ResultsTable) SaveError() string { return r.saveError }

// SetDirtyCell records a pending cell edit (e.g. from the inspector).
func (r *ResultsTable) SetDirtyCell(row, col int, val string) {
	ref := cellRef{row: row, col: col}
	if r.dirtyCells == nil {
		r.dirtyCells = make(map[cellRef]string)
	}
	r.dirtyCells[ref] = val
}

// SetColumnTypes stores column type metadata from the query result.
func (r *ResultsTable) SetColumnTypes(types map[string]string) {
	r.columnTypes = types
}

// SetSort stores the current sort state for header arrow display.
func (r *ResultsTable) SetSort(col, dir string) {
	r.sortCol = col
	r.sortDir = dir
}

// ColumnType returns the database type for a given column index.
func (r ResultsTable) ColumnType(col int) string {
	if col < 0 || col >= len(r.columns) {
		return ""
	}
	if r.columnTypes == nil {
		return ""
	}
	return r.columnTypes[r.columns[col]]
}

// SetSize sets the dimensions of the results panel.
func (r *ResultsTable) SetSize(width, height int) {
	r.width = width
	r.height = height
	r.clampScrollRow()
	r.clampScrollCol()
	r.clampCursor()
}

// SetBorderColor sets the colour used for the table border, allowing the
// caller to reflect focus state.
func (r *ResultsTable) SetBorderColor(c lipgloss.Color) {
	r.borderColor = c
}

func (r *ResultsTable) computeColWidths() {
	if len(r.columns) == 0 {
		return
	}

	r.colWidths = make([]int, len(r.columns))

	for i, col := range r.columns {
		w := runeLen(col)
		// Reserve room for PK suffix ("*") and sort indicator (" ↑"/" ↓")
		// that are appended at render time. The sort indicator can appear
		// on any column, so always reserve space for it.
		if r.editable && r.isPKColumn(col) {
			w += runeLen(" *")
		}
		w += runeLen(" ↑")
		r.colWidths[i] = w
	}

	for _, row := range r.rows {
		for i := 0; i < len(r.columns) && i < len(row); i++ {
			l := runeLen(row[i])
			if l > r.colWidths[i] {
				r.colWidths[i] = l
			}
		}
	}

	for i := range r.colWidths {
		if r.colWidths[i] > maxCellWidth {
			r.colWidths[i] = maxCellWidth
		}
	}
}

// ApplyRememberedWidths raises any column's width to at least the remembered
// value for that column name (matched case-insensitively). Caps at
// maxCellWidth. No-op when saved is empty.
func (r *ResultsTable) ApplyRememberedWidths(saved map[string]int) {
	if len(saved) == 0 || len(r.colWidths) == 0 {
		return
	}
	lookup := make(map[string]int, len(saved))
	for k, v := range saved {
		lookup[strings.ToLower(k)] = v
	}
	for i, name := range r.columns {
		w, ok := lookup[strings.ToLower(name)]
		if !ok || w <= r.colWidths[i] {
			continue
		}
		if w > maxCellWidth {
			w = maxCellWidth
		}
		r.colWidths[i] = w
	}
}

// SnapshotWidths returns the current column widths keyed by column name.
func (r ResultsTable) SnapshotWidths() map[string]int {
	if len(r.columns) == 0 || len(r.colWidths) == 0 {
		return nil
	}
	out := make(map[string]int, len(r.columns))
	for i, name := range r.columns {
		if i < len(r.colWidths) {
			out[name] = r.colWidths[i]
		}
	}
	return out
}

// ColWidth returns the display width for a column index, or 0 if unknown.
func (r ResultsTable) ColWidth(col int) int {
	if col < 0 || col >= len(r.colWidths) {
		return 0
	}
	return r.colWidths[col]
}

func runeLen(s string) int {
	return uniseg.StringWidth(s)
}

func (r *ResultsTable) clampScrollRow() {
	maxVisible := r.maxVisibleRows()
	if r.scrollRow < 0 {
		r.scrollRow = 0
	}
	if r.scrollRow > len(r.rows)-maxVisible && len(r.rows) > maxVisible {
		r.scrollRow = len(r.rows) - maxVisible
	}
}

func (r *ResultsTable) clampScrollCol() {
	if r.scrollCol < 0 {
		r.scrollCol = 0
	}
	totalCols := len(r.colWidths)
	if r.scrollCol >= totalCols {
		r.scrollCol = totalCols - 1
	}
	if r.scrollCol < 0 {
		r.scrollCol = 0
	}
}

func (r *ResultsTable) clampCursor() {
	if len(r.rows) == 0 {
		r.cursorRow = 0
	} else if r.cursorRow >= len(r.rows) {
		r.cursorRow = len(r.rows) - 1
	}
	if r.cursorRow < 0 {
		r.cursorRow = 0
	}

	if len(r.columns) == 0 {
		r.cursorCol = 0
	} else if r.cursorCol >= len(r.columns) {
		r.cursorCol = len(r.columns) - 1
	}
	if r.cursorCol < 0 {
		r.cursorCol = 0
	}
	// Never leave the cursor on a hidden column; move to the nearest
	// visible one.
	if r.IsColumnHidden(r.cursorCol) {
		if next := r.nextVisibleCol(r.cursorCol); next >= 0 {
			r.cursorCol = next
		} else if prev := r.prevVisibleCol(r.cursorCol); prev >= 0 {
			r.cursorCol = prev
		}
	}
}

// ensureCursorVisible adjusts scroll so the cursor cell is in view.
func (r *ResultsTable) ensureCursorVisible() {
	maxVisible := r.maxVisibleRows()
	if r.cursorRow < r.scrollRow {
		r.scrollRow = r.cursorRow
	}
	if r.cursorRow >= r.scrollRow+maxVisible {
		r.scrollRow = r.cursorRow - maxVisible + 1
	}
	r.clampScrollRow()

	// Horizontal: keep cursor column visible (skipping hidden columns).
	for {
		vis := r.visibleColRange()
		if len(vis) == 0 {
			break
		}
		visible := false
		for _, c := range vis {
			if c == r.cursorCol {
				visible = true
				break
			}
		}
		if visible {
			break
		}
		if r.cursorCol < vis[0] {
			// Cursor is left of the visible window; scroll to it.
			r.scrollCol = r.cursorCol
			continue
		}
		// Cursor is right of the visible window; step right.
		if r.scrollCol < len(r.colWidths)-1 {
			r.scrollCol++
		} else {
			break
		}
	}
}

func (r ResultsTable) maxVisibleRows() int {
	max := r.height - 4
	if max < 0 {
		max = 0
	}
	return max
}

// ScrollDown moves the visible rows down by one.
func (r *ResultsTable) ScrollDown() {
	maxVisible := r.maxVisibleRows()
	if r.scrollRow < len(r.rows)-maxVisible {
		r.scrollRow++
	}
}

// ScrollUp moves the visible rows up by one.
func (r *ResultsTable) ScrollUp() {
	if r.scrollRow > 0 {
		r.scrollRow--
	}
}

// ScrollBy shifts the visible rows by rows (signed: + down, - up), clamped to
// the valid range. Used to apply a coalesced wheel delta in one move.
func (r *ResultsTable) ScrollBy(rows int) {
	maxVisible := r.maxVisibleRows()
	maxScroll := len(r.rows) - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	r.scrollRow += rows
	if r.scrollRow < 0 {
		r.scrollRow = 0
	}
	if r.scrollRow > maxScroll {
		r.scrollRow = maxScroll
	}
}

// ScrollRight moves the visible columns right by one.
func (r *ResultsTable) ScrollRight() {
	if r.scrollCol < len(r.colWidths)-1 {
		r.scrollCol++
	}
}

// ScrollLeft moves the visible columns left by one.
func (r *ResultsTable) ScrollLeft() {
	if r.scrollCol > 0 {
		r.scrollCol--
	}
}

// ScrollTop scrolls to the first row.
func (r *ResultsTable) ScrollTop() {
	r.scrollRow = 0
}

// ScrollBottom scrolls to the last row.
func (r *ResultsTable) ScrollBottom() {
	maxVisible := r.maxVisibleRows()
	if len(r.rows) > maxVisible {
		r.scrollRow = len(r.rows) - maxVisible
	}
}

// CursorCellValue returns the full value at the current cell cursor.
func (r ResultsTable) CursorCellValue() string {
	return r.RowValue(r.cursorRow, r.cursorCol)
}

// hasCellCursor reports whether the table shows a cell selection cursor.
func (r ResultsTable) hasCellCursor() bool {
	return r.hasResult && len(r.rows) > 0
}

// CursorTop moves the cell cursor to the first row.
func (r *ResultsTable) CursorTop() {
	r.cursorRow = 0
	r.ensureCursorVisible()
}

// CursorBottom moves the cell cursor to the last row.
func (r *ResultsTable) CursorBottom() {
	r.cursorRow = len(r.rows)
	r.clampCursor()
	r.ensureCursorVisible()
}

// CursorDown moves the cell cursor down.
func (r *ResultsTable) CursorDown() {
	if r.cursorRow < len(r.rows)-1 {
		r.cursorRow++
	}
	r.ensureCursorVisible()
}

// CursorUp moves the cell cursor up.
func (r *ResultsTable) CursorUp() {
	if r.cursorRow > 0 {
		r.cursorRow--
	}
	r.ensureCursorVisible()
}

// CursorRight moves the cell cursor right (editable mode), skipping hidden columns.
func (r *ResultsTable) CursorRight() {
	if next := r.nextVisibleCol(r.cursorCol + 1); next >= 0 {
		r.cursorCol = next
	}
	r.ensureCursorVisible()
}

// CursorLeft moves the cell cursor left (editable mode), skipping hidden columns.
func (r *ResultsTable) CursorLeft() {
	if prev := r.prevVisibleCol(r.cursorCol - 1); prev >= 0 {
		r.cursorCol = prev
	}
	r.ensureCursorVisible()
}

// CursorFirstCol moves the cell cursor to the first visible column.
func (r *ResultsTable) CursorFirstCol() {
	if first := r.nextVisibleCol(0); first >= 0 {
		r.cursorCol = first
	}
	r.ensureCursorVisible()
}

// CursorLastCol moves the cell cursor to the last visible column.
func (r *ResultsTable) CursorLastCol() {
	if last := r.prevVisibleCol(len(r.columns) - 1); last >= 0 {
		r.cursorCol = last
	}
	r.ensureCursorVisible()
}

// StartEdit enters inline edit mode on the current cell.
func (r *ResultsTable) StartEdit() {
	if !r.editable || !r.HasPrimaryKey() || len(r.rows) == 0 {
		return
	}
	// Binary cells can't be edited as text; use :saveblob to export them.
	if r.IsBlobCell(r.cursorRow, r.cursorCol) {
		return
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0 // no limit
	currentVal := r.RowValue(r.cursorRow, r.cursorCol)
	if currentVal == "NULL" {
		currentVal = ""
	}
	ti.SetValue(currentVal)
	colWidth := maxCellWidth
	if r.cursorCol < len(r.colWidths) {
		colWidth = r.colWidths[r.cursorCol]
	}
	// textinput.View() renders Width+1 chars (cursor takes a column), so
	// subtract 1 to keep cell borders aligned with non-editing rows.
	ti.Width = colWidth - 1

	ti.TextStyle = lipgloss.NewStyle().Foreground(colorEdit)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colorFg).Background(colorBg)

	ti.Focus()
	r.editInput = ti
	r.editing = true
	r.saved = false
}

// CommitEdit saves the current edit buffer to dirtyCells and exits edit mode.
func (r *ResultsTable) CommitEdit() {
	if !r.editing {
		return
	}
	newVal := r.editInput.Value()
	ref := cellRef{row: r.cursorRow, col: r.cursorCol}
	r.dirtyCells[ref] = newVal
	r.editing = false
}

// CancelEdit discards the current edit and exits edit mode.
func (r *ResultsTable) CancelEdit() {
	r.editing = false
}

// Update handles messages for the results table.
func (r ResultsTable) Update(msg tea.Msg) (ResultsTable, tea.Cmd) {
	if r.editing {
		var cmd tea.Cmd
		r.editInput, cmd = r.editInput.Update(msg)
		return r, cmd
	}
	return r, nil
}

// sanitizeCellValue replaces control characters (newlines, tabs, etc.)
// with spaces so a cell value renders on a single grid line.
func sanitizeCellValue(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == '\v' || r == '\f' {
			return ' '
		}
		return r
	}, v)
}

// sanitizeCellRow replaces control characters (newlines, tabs, etc.)
// with spaces so cell values render on a single line.
func sanitizeCellRow(row []string) []string {
	out := make([]string, len(row))
	for i, v := range row {
		out[i] = sanitizeCellValue(v)
	}
	return out
}

// truncateCell truncates a string to fit within width display columns,
// appending "…" if truncated.
func truncateCell(s string, width int) string {
	l := runeLen(s)
	if l <= width {
		return s + strings.Repeat(" ", width-l)
	}
	if width <= 1 {
		return "…"
	}
	// Walk runes accumulating display width until we reach width-1.
	var b strings.Builder
	bW := 0
	target := width - 1 // leave room for "…"
	for _, r := range s {
		rw := uniseg.StringWidth(string(r))
		if bW+rw > target {
			break
		}
		b.WriteRune(r)
		bW += rw
	}
	return b.String() + "…"
}

// truncateCellRight truncates like truncateCell but left-pads the value
// (right-aligned), for numeric columns.
func truncateCellRight(s string, width int) string {
	l := runeLen(s)
	if l <= width {
		return strings.Repeat(" ", width-l) + s
	}
	if width <= 1 {
		return "…"
	}
	var b strings.Builder
	bW := 0
	target := width - 1
	for _, r := range s {
		rw := uniseg.StringWidth(string(r))
		if bW+rw > target {
			break
		}
		b.WriteRune(r)
		bW += rw
	}
	return b.String() + "…"
}

// renderEditInput renders the textinput value with an underline cursor that
// overlays the character at the cursor position (a space when the cursor is at
// the end). Because the cursor is an overlay rather than an inserted glyph, the
// field always occupies exactly width columns — switching between normal and
// insert modes never shifts the text by a column, which an inserted bar glyph
// ("▏") would do. width is the total display width to fill, matching the
// field's column width. fg is the foreground color applied to the text; the
// cursor cell uses colorFg.
func renderEditInput(ti textinput.Model, width int, fg lipgloss.Color) string {
	if width < 1 {
		return ""
	}
	value := ti.Value()
	pos := ti.Position()
	runes := []rune(value)

	// Scroll window over the full width (cursor cell included): keep the
	// cursor visible, biasing toward the right edge.
	textStart := 0
	if pos > width-1 {
		textStart = pos - (width - 1)
	}
	textEnd := textStart + width
	if textEnd > len(runes) {
		textEnd = len(runes)
	}

	before := string(runes[textStart:pos])
	after := ""
	if pos+1 <= textEnd {
		after = string(runes[pos+1 : textEnd])
	}

	// The character under the cursor (a space when at the end / empty).
	cursorChar := " "
	if pos < len(runes) {
		cursorChar = string(runes[pos])
	}

	pad := width - runeLen(before) - runeLen(cursorChar) - runeLen(after)
	if pad < 0 {
		pad = 0
	}

	textStyle := lipgloss.NewStyle().Foreground(fg)
	cursorStyle := lipgloss.NewStyle().Foreground(colorFg).Underline(true)

	return textStyle.Render(before) +
		cursorStyle.Render(cursorChar) +
		textStyle.Render(after) +
		strings.Repeat(" ", pad)
}

// visibleColRange returns the start and end column indices that fit
// within the available width, starting from scrollCol.
func (r ResultsTable) visibleColRange() []int {
	// Each column renders as: " " + value(colWidth) + " " + "│" = colWidth + 3
	// The leftmost "│" is 1 extra char.
	available := r.width - 1
	if available < 1 {
		available = 1
	}

	used := 0
	var out []int
	for i := r.scrollCol; i < len(r.colWidths); i++ {
		if r.IsColumnHidden(i) {
			continue
		}
		colW := r.colWidths[i] + 3
		if used+colW > available && len(out) > 0 {
			break
		}
		used += colW
		out = append(out, i)
	}

	return out
}

// isPKColumn returns true if colName is a primary key column.
func (r ResultsTable) isPKColumn(colName string) bool {
	for _, pk := range r.pkColumns {
		if pk == colName {
			return true
		}
	}
	return false
}

// fkColumnFg returns the foreground for a foreign-key results cell.
// PK columns (including PK+FK) stay unstyled. Headers are never tinted.
func (r ResultsTable) fkColumnFg(col int) (lipgloss.Color, bool) {
	if name := r.ColumnName(col); name != "" && r.isPKColumn(name) {
		return "", false
	}
	if _, ok := r.ForeignKeyAt(col); ok {
		return colorFK, true
	}
	return "", false
}

// statusColumnFg returns a semantic foreground for status/state enum cells.
func (r ResultsTable) statusColumnFg(col int, val string) (lipgloss.Color, bool) {
	if !isStatusColumnName(r.ColumnName(col)) {
		return "", false
	}
	return statusValueFg(val)
}

// cellContentFg picks the default (non-highlight) foreground for a cell:
// NULL/blob muted, then status enum, then FK tint, else normal fg.
func (r ResultsTable) cellContentFg(col int, val string) lipgloss.Color {
	if val == "NULL" || db.IsBlobPlaceholder(val) {
		return colorMuted
	}
	if fg, ok := r.statusColumnFg(col, val); ok {
		return fg
	}
	if keyFg, ok := r.fkColumnFg(col); ok {
		return keyFg
	}
	return colorFg
}

// View renders the results table.
func (r ResultsTable) View() string {
	if !r.hasResult {
		return mutedStyle.Render(" Run a query to see results.")
	}

	if r.message != "" && len(r.columns) == 0 {
		// Keep the error inside the panel: wrap to width and cap height so a
		// long driver message cannot blow past the allocated results slot.
		w := r.width
		if w < 1 {
			w = 40
		}
		style := errorStyle.Width(w)
		if r.height > 0 {
			style = style.MaxHeight(r.height)
		}
		return style.Render(r.message)
	}

	if len(r.columns) == 0 || r.height < 4 {
		return mutedStyle.Render(r.message)
	}

	maxVisible := r.maxVisibleRows()

	rowStart := r.scrollRow
	rowEnd := rowStart + maxVisible
	if rowEnd > len(r.rows) {
		rowEnd = len(r.rows)
	}

	cols := r.visibleColRange()

	// Extend the last visible column so the border fills the full table
	// width (the table border doubles as the panel frame). Copy the slice
	// first — the value receiver shares the backing array, so mutating
	// r.colWidths directly would permanently grow widths on every render.
	if len(cols) > 0 {
		r.colWidths = append([]int(nil), r.colWidths...)
		used := 0
		for _, i := range cols {
			used += r.colWidths[i] + 3 // " value " + "│"
		}
		if fill := r.width - 1 - used; fill > 0 {
			r.colWidths[cols[len(cols)-1]] += fill
		}
	}

	var b strings.Builder

	// Frame colour: the outer rectangle follows focus state; all inner
	// lines (column separators, header rule) are always muted.
	bc := colorBorder
	if r.borderColor != "" {
		bc = r.borderColor
	}
	outerStyle := lipgloss.NewStyle().Foreground(bc)
	innerStyle := lipgloss.NewStyle().Foreground(colorBorder)

	// ── Top frame: solid blue line, no junctions. ──────────────────
	if len(cols) > 0 {
		totalW := 0
		for _, i := range cols {
			totalW += r.colWidths[i] + 3
		}
		b.WriteString(outerStyle.Render("┌" + strings.Repeat("─", totalW-1) + "┐"))
	}
	b.WriteString("\n")

	// ── Header row ─────────────────────────────────────────────────
	b.WriteString(outerStyle.Render("│"))
	for j, i := range cols {
		header := r.columns[i]
		if header == r.sortCol && r.sortDir != "" {
			if r.sortDir == "ASC" {
				header = header + " ↑"
			} else {
				header = header + " ↓"
			}
		}
		if r.editable && r.isPKColumn(r.columns[i]) {
			header = header + " *"
		}
		if ord := r.ColumnMarkOrdinal(i); ord > 0 {
			header = fmt.Sprintf("%d·%s", ord, header)
		}
		cell := truncateCell(header, r.colWidths[i])
		baseStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
		if r.IsMarkedColumn(i) {
			baseStyle = lipgloss.NewStyle().Foreground(colorMark).Bold(true)
		}
		if r.hasCellCursor() && i == r.cursorCol {
			// Underline only the text, not the cell padding.
			text := strings.TrimRight(cell, " ")
			padW := r.colWidths[i] - runeLen(text)
			b.WriteString(baseStyle.Render(" "))
			b.WriteString(baseStyle.Underline(true).Render(text))
			if padW > 0 {
				b.WriteString(baseStyle.Render(strings.Repeat(" ", padW)))
			}
			b.WriteString(baseStyle.Render(" "))
		} else {
			b.WriteString(baseStyle.Render(" " + cell + " "))
		}
		if j < len(cols)-1 {
			b.WriteString(innerStyle.Render("│"))
		}
	}
	b.WriteString(outerStyle.Render("│"))
	b.WriteString("\n")

	// ── Header separator: muted dashes and ┼ inside; blue │ at the
	// edges keeps the vertical frame continuous without horizontal
	// ticks poking inward. ──────────────────────────────────────────
	if len(cols) > 0 {
		b.WriteString(outerStyle.Render("│"))
		for j, i := range cols {
			w := r.colWidths[i] + 2
			b.WriteString(innerStyle.Render(strings.Repeat("─", w)))
			if j < len(cols)-1 {
				b.WriteString(innerStyle.Render("┼"))
			}
		}
		b.WriteString(outerStyle.Render("│"))
	}
	b.WriteString("\n")

	// ── Data rows ──────────────────────────────────────────────────
	for rowIdx := rowStart; rowIdx < rowEnd; rowIdx++ {
		row := r.rows[rowIdx]
		isCursorRow := r.hasCellCursor() && rowIdx == r.cursorRow

		// Determine background colour for the row.
		var bg lipgloss.Color
		switch {
		case r.isVisualRow(rowIdx):
			bg = colorVisual
		case isCursorRow:
			bg = colorCursorRow
		case r.IsWatchDeltaRow(rowIdx):
			bg = colorSearch
		case rowIdx%2 == 1:
			bg = colorStripe
		}

		// Left border (outer frame)
		if r.IsMarkedRow(rowIdx) {
			b.WriteString(lipgloss.NewStyle().Foreground(colorMark).Render("◆"))
		} else {
			leftStyle := outerStyle
			if bg != "" {
				leftStyle = lipgloss.NewStyle().Foreground(bc).Background(bg)
			}
			b.WriteString(leftStyle.Render("│"))
		}

		for j, i := range cols {
			ref := cellRef{row: rowIdx, col: i}
			dirtyVal, isDirty := r.dirtyCells[ref]
			isCursorCell := isCursorRow && i == r.cursorCol
			isCopyFlash := r.isCopyFlashCell(rowIdx, i)

			val := ""
			if i < len(row) {
				val = row[i]
			}
			if isDirty {
				val = sanitizeCellValue(dirtyVal)
			}

			// If this is the cell being edited, show the input buffer.
			if r.editing && isCursorCell {
				inputView := renderEditInput(r.editInput, r.colWidths[i], colorEdit)
				b.WriteString(" " + inputView + " ")
			} else {
				cell := truncateCell(val, r.colWidths[i])
				if isNumericType(r.columnType(i)) {
					cell = truncateCellRight(val, r.colWidths[i])
				} else if isStatusColumnName(r.ColumnName(i)) {
					if disp, _, ok := statusCellDisplay(val, r.colWidths[i]); ok {
						cell = disp
					}
				}
				if isCursorCell && r.IsNavigableForeignKey(rowIdx, i) {
					arrow := " →"
					arrowW := lipgloss.Width(arrow)
					cell = truncateCell(val, r.colWidths[i]-arrowW) + arrow
				}

				// Style the cell
				isMarked := r.IsMarkedRow(rowIdx)
				isVisualRow := r.isVisualRow(rowIdx)
				isColMarked := r.IsMarkedColumn(i)
				isSearchMatch := r.searchMatcher != nil && r.searchMatcher(val)
				isWatchDelta := r.IsWatchDeltaRow(rowIdx)
				var style lipgloss.Style
				switch {
				case isCopyFlash && r.copyFlashOn:
					style = lipgloss.NewStyle().Foreground(colorBg).Background(colorSuccess)
				case isCursorCell:
					style = lipgloss.NewStyle().Foreground(colorBg).Background(colorPrimary)
				case isDirty:
					style = lipgloss.NewStyle().Foreground(colorBg).Background(colorWarn)
				case isVisualRow:
					style = lipgloss.NewStyle().Foreground(colorFg).Background(colorVisual)
				case isMarked:
					style = lipgloss.NewStyle().Foreground(colorMark)
					if isColMarked {
						style = style.Background(colorVisual)
					}
				case isColMarked:
					style = lipgloss.NewStyle().Foreground(colorFg).Background(colorVisual)
				case isSearchMatch:
					style = lipgloss.NewStyle().Foreground(colorFg).Background(colorSearch)
				case isWatchDelta:
					style = lipgloss.NewStyle().Foreground(colorFg).Background(colorSearch)
				case isCursorRow:
					style = lipgloss.NewStyle().Foreground(r.cellContentFg(i, val)).Background(colorCursorRow)
				default:
					style = lipgloss.NewStyle().Foreground(r.cellContentFg(i, val))
					if rowIdx%2 == 1 {
						style = style.Background(colorStripe)
					}
				}
				b.WriteString(style.Render(" " + cell + " "))
			}

			// Inner column separator (muted) or right border (frame)
			if j < len(cols)-1 {
				sepStyle := innerStyle
				if bg != "" {
					sepStyle = lipgloss.NewStyle().Foreground(colorBorder).Background(bg)
				}
				b.WriteString(sepStyle.Render("│"))
			}
		}

		// Right border (outer frame)
		rightStyle := outerStyle
		if bg != "" {
			rightStyle = lipgloss.NewStyle().Foreground(bc).Background(bg)
		}
		b.WriteString(rightStyle.Render("│"))
		b.WriteString("\n")
	}

	// ── Padding rows: fill the remaining height so the frame stretches
	// to the bottom even with fewer rows than the panel allows. ─────
	renderedRows := rowEnd - rowStart
	for p := renderedRows; p < maxVisible; p++ {
		var bg lipgloss.Color
		if p%2 == 1 {
			bg = colorStripe
		}
		leftStyle := outerStyle
		rightStyle := outerStyle
		if bg != "" {
			leftStyle = lipgloss.NewStyle().Foreground(bc).Background(bg)
			rightStyle = leftStyle
		}
		b.WriteString(leftStyle.Render("│"))
		for j, i := range cols {
			empty := lipgloss.NewStyle()
			if bg != "" {
				empty = empty.Background(bg)
			}
			b.WriteString(empty.Render(strings.Repeat(" ", r.colWidths[i]+2)))
			if j < len(cols)-1 {
				sepStyle := innerStyle
				if bg != "" {
					sepStyle = lipgloss.NewStyle().Foreground(colorBorder).Background(bg)
				}
				b.WriteString(sepStyle.Render("│"))
			}
		}
		b.WriteString(rightStyle.Render("│"))
		b.WriteString("\n")
	}

	// ── Bottom frame: solid blue line, no junctions. ───────────────
	if len(cols) > 0 {
		totalW := 0
		for _, i := range cols {
			totalW += r.colWidths[i] + 3
		}
		b.WriteString(outerStyle.Render("└" + strings.Repeat("─", totalW-1) + "┘"))
	}

	return b.String()
}
