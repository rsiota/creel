package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/rsiota/creel/internal/db"
)

func TestResultsTableCursorNavigation(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name", "email"},
		[][]string{
			{"1", "alice", "alice@test.com"},
			{"2", "bob", "bob@test.com"},
			{"3", "carol", "carol@test.com"},
		},
		"3 rows",
	)

	// Cursor starts at (0,0)
	if r.CursorRow() != 0 || r.CursorCol() != 0 {
		t.Fatalf("cursor should be at (0,0), got (%d,%d)", r.CursorRow(), r.CursorCol())
	}

	r.CursorDown()
	if r.CursorRow() != 1 {
		t.Errorf("after CursorDown, row should be 1, got %d", r.CursorRow())
	}

	r.CursorUp()
	if r.CursorRow() != 0 {
		t.Errorf("after CursorUp, row should be 0, got %d", r.CursorRow())
	}

	r.CursorUp()
	if r.CursorRow() != 0 {
		t.Errorf("CursorUp at top should stay 0, got %d", r.CursorRow())
	}

	r.CursorRight()
	r.CursorRight()
	if r.CursorCol() != 2 {
		t.Errorf("after 2x CursorRight, col should be 2, got %d", r.CursorCol())
	}

	r.CursorRight()
	if r.CursorCol() != 2 {
		t.Errorf("CursorRight at edge should stay 2, got %d", r.CursorCol())
	}

	r.CursorLeft()
	if r.CursorCol() != 1 {
		t.Errorf("after CursorLeft, col should be 1, got %d", r.CursorCol())
	}

	r.CursorFirstCol()
	if r.CursorCol() != 0 {
		t.Errorf("after CursorFirstCol, col should be 0, got %d", r.CursorCol())
	}

	r.CursorLastCol()
	if r.CursorCol() != 2 {
		t.Errorf("after CursorLastCol, col should be 2, got %d", r.CursorCol())
	}

	r.CursorTop()
	if r.CursorRow() != 0 {
		t.Errorf("after CursorTop, row should be 0, got %d", r.CursorRow())
	}

	r.CursorBottom()
	if r.CursorRow() != 2 {
		t.Errorf("after CursorBottom, row should be 2, got %d", r.CursorRow())
	}
}

func TestResultsTableColumnHiding(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name", "email", "phone"},
		[][]string{{"1", "alice", "alice@test.com", "555-1234"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})

	// Initially no hidden columns
	if r.HiddenCount() != 0 {
		t.Fatalf("expected 0 hidden columns, got %d", r.HiddenCount())
	}
	if r.VisibleColumnCount() != 4 {
		t.Fatalf("expected 4 visible columns, got %d", r.VisibleColumnCount())
	}
	if r.IsColumnHidden(1) {
		t.Error("col 1 should not be hidden")
	}

	// Hide column 1 (name)
	if !r.HideColumn(1) {
		t.Error("HideColumn(1) should return true")
	}
	if r.HiddenCount() != 1 {
		t.Errorf("expected 1 hidden column, got %d", r.HiddenCount())
	}
	if !r.IsColumnHidden(1) {
		t.Error("col 1 should now be hidden")
	}
	if r.VisibleColumnCount() != 3 {
		t.Errorf("expected 3 visible columns, got %d", r.VisibleColumnCount())
	}

	// HiddenColumnNames
	names := r.HiddenColumnNames()
	if len(names) != 1 || names[0] != "name" {
		t.Errorf("expected ['name'], got %v", names)
	}

	// Show column 1 again
	r.ShowColumn(1)
	if r.HiddenCount() != 0 {
		t.Errorf("after ShowColumn, expected 0 hidden, got %d", r.HiddenCount())
	}

	// Hide 3 of 4 columns, then try to hide the last visible one
	r.HideColumn(0)
	r.HideColumn(1)
	r.HideColumn(2)
	if r.HiddenCount() != 3 {
		t.Fatalf("expected 3 hidden, got %d", r.HiddenCount())
	}
	if r.HideColumn(3) {
		t.Error("should refuse to hide the last visible column")
	}
	if r.HiddenCount() != 3 {
		t.Errorf("should still be 3 hidden, got %d", r.HiddenCount())
	}

	// ShowAllColumns resets
	r.ShowAllColumns()
	if r.HiddenCount() != 0 {
		t.Errorf("after ShowAllColumns, expected 0 hidden, got %d", r.HiddenCount())
	}
}

func TestResultsTableSetHiddenColumns(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name", "email"},
		[][]string{{"1", "alice", "a@t.com"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})

	// Set specific columns as hidden
	r.SetHiddenColumns([]string{"name"})
	if r.HiddenCount() != 1 {
		t.Fatalf("expected 1 hidden, got %d", r.HiddenCount())
	}
	if !r.IsColumnHidden(1) {
		t.Error("col 'name' should be hidden")
	}

	// Replace with a different set
	r.SetHiddenColumns([]string{"id", "email"})
	if r.HiddenCount() != 2 {
		t.Errorf("expected 2 hidden, got %d", r.HiddenCount())
	}
	if !r.IsColumnHidden(0) {
		t.Error("col 'id' should be hidden")
	}
	if !r.IsColumnHidden(2) {
		t.Error("col 'email' should be hidden")
	}
	if r.IsColumnHidden(1) {
		t.Error("col 'name' should NOT be hidden")
	}

	// Setting all columns hidden should be rejected (never hide everything)
	r.SetHiddenColumns([]string{"id", "name", "email"})
	if r.HiddenCount() != 0 {
		t.Errorf("should never hide all columns, got %d hidden", r.HiddenCount())
	}
}

func TestResultsTableColumnHidingCursorClamp(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name", "email"},
		[][]string{{"1", "alice", "a@t.com"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})

	// Move cursor to col 2
	r.CursorRight()
	r.CursorRight()
	if r.CursorCol() != 2 {
		t.Fatalf("cursor should be at col 2, got %d", r.CursorCol())
	}

	// Hide col 2 — cursor should clamp to a visible column
	r.HideColumn(2)
	if r.IsColumnHidden(r.CursorCol()) {
		t.Errorf("cursor at col %d should not be on a hidden column", r.CursorCol())
	}
}

func TestResultsTableMarks(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name"},
		[][]string{
			{"1", "alice"},
			{"2", "bob"},
			{"3", "carol"},
		},
		"3 rows",
	)
	r.SetEditable("users", []string{"id"})

	// No marks initially
	if r.MarkCount() != 0 {
		t.Fatalf("expected 0 marks, got %d", r.MarkCount())
	}

	// Toggle mark on row 0
	r.ToggleMark()
	if r.MarkCount() != 1 {
		t.Errorf("expected 1 mark, got %d", r.MarkCount())
	}
	if !r.IsMarkedRow(0) {
		t.Error("row 0 should be marked")
	}

	// Toggle mark on row 1
	r.CursorDown()
	r.ToggleMark()
	if r.MarkCount() != 2 {
		t.Errorf("expected 2 marks, got %d", r.MarkCount())
	}

	// Unmark row 1
	r.ToggleMark()
	if r.MarkCount() != 1 {
		t.Errorf("expected 1 mark after unmark, got %d", r.MarkCount())
	}
	if r.IsMarkedRow(1) {
		t.Error("row 1 should not be marked")
	}

	// MarkedPKs should return only the remaining mark
	pks := r.MarkedPKs()
	if len(pks) != 1 || len(pks[0]) != 1 || pks[0][0] != "1" {
		t.Errorf("expected [['1']], got %v", pks)
	}

	// ClearMarks
	r.ClearMarks()
	if r.MarkCount() != 0 {
		t.Errorf("expected 0 marks after ClearMarks, got %d", r.MarkCount())
	}
}

func TestResultsTableMarksNotEditable(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}},
		"1 row",
	)

	// ToggleMark on a non-editable table is a no-op
	r.ToggleMark()
	if r.MarkCount() != 0 {
		t.Errorf("non-editable table should have 0 marks, got %d", r.MarkCount())
	}
}

func TestResultsTableMarksInvalidateOnTableChange(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})

	r.ToggleMark()
	if r.MarkCount() != 1 {
		t.Fatalf("expected 1 mark, got %d", r.MarkCount())
	}

	// Switch to a different table — marks should be invalidated
	r.SetEditable("orders", []string{"id"})
	if r.MarkCount() != 0 {
		t.Errorf("marks from old table should not count, got %d", r.MarkCount())
	}
}

func TestResultsTableVisualMode(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name"},
		[][]string{
			{"1", "alice"},
			{"2", "bob"},
			{"3", "carol"},
			{"4", "dave"},
		},
		"4 rows",
	)

	// Not in visual mode initially
	if r.IsVisualMode() {
		t.Fatal("should not be in visual mode initially")
	}
	if r.VisualRangeSize() != 0 {
		t.Fatal("visual range should be 0 when inactive")
	}

	// Enter visual mode at row 0
	r.SetVisualMode()
	if !r.IsVisualMode() {
		t.Fatal("should be in visual mode after SetVisualMode")
	}
	if r.VisualRangeSize() != 1 {
		t.Errorf("range should be 1 (cursor only), got %d", r.VisualRangeSize())
	}

	lo, hi := r.VisualRange()
	if lo != 0 || hi != 0 {
		t.Errorf("range should be (0,0), got (%d,%d)", lo, hi)
	}

	// Move cursor down — range expands
	r.CursorDown()
	r.CursorDown()
	lo, hi = r.VisualRange()
	if lo != 0 || hi != 2 {
		t.Errorf("range should be (0,2), got (%d,%d)", lo, hi)
	}
	if r.VisualRangeSize() != 3 {
		t.Errorf("range size should be 3, got %d", r.VisualRangeSize())
	}

	// isVisualRow
	if !r.isVisualRow(1) {
		t.Error("row 1 should be in visual range")
	}
	if r.isVisualRow(3) {
		t.Error("row 3 should not be in visual range")
	}

	// Move cursor back up past anchor — range should still cover correctly
	r.CursorTop()
	lo, hi = r.VisualRange()
	if lo != 0 || hi != 0 {
		t.Errorf("range with cursor at 0 should be (0,0), got (%d,%d)", lo, hi)
	}

	// Test upward selection: anchor at row 2, cursor moves up
	r.ClearVisualMode()
	r.cursorRow = 2
	r.SetVisualMode()
	r.CursorTop()
	lo, hi = r.VisualRange()
	if lo != 0 || hi != 2 {
		t.Errorf("upward range should be (0,2), got (%d,%d)", lo, hi)
	}

	// Clear visual mode
	r.ClearVisualMode()
	if r.IsVisualMode() {
		t.Error("should not be in visual mode after ClearVisualMode")
	}
}

func TestResultsTableClearAndSetResult(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id"},
		[][]string{{"1"}},
		"1 row",
	)

	if !r.HasResult() {
		t.Fatal("should have result after SetResult")
	}
	if r.NumRows() != 1 {
		t.Errorf("expected 1 row, got %d", r.NumRows())
	}
	if r.NumCols() != 1 {
		t.Errorf("expected 1 col, got %d", r.NumCols())
	}
	if r.Message() != "1 row" {
		t.Errorf("expected message '1 row', got %q", r.Message())
	}

	// SetError should set an error message
	r.SetError("something went wrong")
	if r.Message() != "something went wrong" {
		t.Errorf("expected error message, got %q", r.Message())
	}

	// Clear should reset
	r.Clear()
	if r.HasResult() {
		t.Error("should not have result after Clear")
	}
	if r.NumRows() != 0 {
		t.Errorf("expected 0 rows after Clear, got %d", r.NumRows())
	}
}

func TestResultsTableErrorFitsPanel(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(40, 5)
	long := strings.Repeat("syntax error near FORM ", 20)
	r.SetError(long)
	view := r.View()
	if lipgloss.Width(view) > 40 {
		t.Errorf("error view width %d > panel 40", lipgloss.Width(view))
	}
	if lipgloss.Height(view) > 5 {
		t.Errorf("error view height %d > panel 5", lipgloss.Height(view))
	}
}

func TestResultsTableSetCursor(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name", "email"},
		[][]string{
			{"1", "alice", "a@t.com"},
			{"2", "bob", "b@t.com"},
		},
		"2 rows",
	)

	r.SetCursor(1, 2)
	if r.CursorRow() != 1 || r.CursorCol() != 2 {
		t.Errorf("expected cursor at (1,2), got (%d,%d)", r.CursorRow(), r.CursorCol())
	}

	// Out-of-range cursor should be clamped
	r.SetCursor(100, 100)
	if r.CursorRow() >= r.NumRows() || r.CursorCol() >= r.NumCols() {
		t.Errorf("cursor should be clamped, got (%d,%d)", r.CursorRow(), r.CursorCol())
	}
}

func TestResultsTableDiscardEdits(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}},
		"1 row",
	)
	r.SetEditable("users", []string{"id"})

	// Make an edit
	r.CursorRight()
	r.StartEdit()
	r.editInput.SetValue("bob")
	r.CommitEdit()

	if !r.HasDirtyCells() {
		t.Fatal("should have dirty cells")
	}

	r.DiscardEdits()
	if r.HasDirtyCells() {
		t.Error("should not have dirty cells after DiscardEdits")
	}

	// Original value should be restored
	if val := r.RowValue(0, 1); val != "alice" {
		t.Errorf("expected original 'alice', got %q", val)
	}
}

func TestResultsTableRowValue(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "name"},
		[][]string{
			{"1", "alice"},
			{"2", "bob"},
		},
		"2 rows",
	)

	if got := r.RowValue(0, 0); got != "1" {
		t.Errorf("RowValue(0,0) = %q, want %q", got, "1")
	}
	if got := r.RowValue(1, 1); got != "bob" {
		t.Errorf("RowValue(1,1) = %q, want %q", got, "bob")
	}
	// Out of range returns empty
	if got := r.RowValue(5, 5); got != "" {
		t.Errorf("RowValue(5,5) = %q, want empty", got)
	}
}

func TestResultsTableColumnName(t *testing.T) {
	r := NewResultsTable()
	r.SetResult(
		[]string{"id", "name", "email"},
		[][]string{{"1", "a", "b"}},
		"1 row",
	)

	if got := r.ColumnName(0); got != "id" {
		t.Errorf("ColumnName(0) = %q, want %q", got, "id")
	}
	if got := r.ColumnName(2); got != "email" {
		t.Errorf("ColumnName(2) = %q, want %q", got, "email")
	}
	if got := r.ColumnName(-1); got != "" {
		t.Errorf("ColumnName(-1) = %q, want empty", got)
	}
	if got := r.ColumnName(99); got != "" {
		t.Errorf("ColumnName(99) = %q, want empty", got)
	}
}

func TestResultsTableSourceTable(t *testing.T) {
	r := NewResultsTable()
	if r.SourceTable() != "" {
		t.Errorf("expected empty source table, got %q", r.SourceTable())
	}

	r.SetResult([]string{"id"}, [][]string{{"1"}}, "1 row")
	r.SetEditable("users", []string{"id"})
	if r.SourceTable() != "users" {
		t.Errorf("expected 'users', got %q", r.SourceTable())
	}

	r.ClearEditable()
	if r.SourceTable() != "" {
		t.Errorf("expected empty after ClearEditable, got %q", r.SourceTable())
	}
}

func TestResultsTableRenameTableReferences(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id"}, [][]string{{"1"}}, "1 row")
	r.SetEditable("old_name", []string{"id"})

	if r.SourceTable() != "old_name" {
		t.Fatalf("expected 'old_name', got %q", r.SourceTable())
	}

	r.RenameTableReferences("old_name", "new_name")
	if r.SourceTable() != "new_name" {
		t.Errorf("expected 'new_name', got %q", r.SourceTable())
	}
}

func TestSanitizeCellRow(t *testing.T) {
	row := sanitizeCellRow([]string{"hello", "NULL", "world"})
	if row[0] != "hello" || row[1] != "NULL" || row[2] != "world" {
		t.Errorf("sanitizeCellRow should preserve values, got %v", row)
	}
	multi := sanitizeCellValue("line one\nline two")
	if multi != "line one line two" {
		t.Errorf("sanitizeCellValue = %q, want %q", multi, "line one line two")
	}
}

// TestResultsTableApplySavedEditsMultiline verifies saved multiline values stay
// in rawRows for the cell viewer while display rows remain single-line.
func TestResultsTableApplySavedEditsMultiline(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 10)
	r.SetResult(
		[]string{"id", "body"},
		[][]string{{"1", "short"}},
		"1 row",
	)
	r.SetDirtyCell(0, 1, "line one\nline two")
	r.ApplySavedEdits()

	if r.HasDirtyCells() {
		t.Fatal("dirty cells should be cleared after ApplySavedEdits")
	}
	if got := r.rows[0][1]; got != "line one line two" {
		t.Errorf("display row = %q, want sanitized single line", got)
	}
	if got := r.RawRowValue(0, 1); got != "line one\nline two" {
		t.Errorf("raw row = %q, want preserved newlines", got)
	}

	view := r.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	headerW := lipgloss.Width(lines[0])
	for _, line := range lines[2:] {
		if strings.TrimSpace(line) == "" || !strings.Contains(line, "│") {
			continue
		}
		if w := lipgloss.Width(line); w != headerW {
			t.Errorf("data row width %d != header width %d: %q", w, headerW, line)
		}
	}
}

// TestResultsTableDirtyMultilineRowWidth verifies that a dirty cell edited in
// the popup with embedded newlines still renders on one grid line so columns
// stay aligned with the header row.
func TestResultsTableDirtyMultilineRowWidth(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 10)
	r.SetResult(
		[]string{"id", "body"},
		[][]string{{"1", "short"}},
		"1 row",
	)
	r.SetDirtyCell(0, 1, "line one\nline two")
	view := r.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected header+sep+data rows, got %d lines", len(lines))
	}
	headerW := lipgloss.Width(lines[0])
	for _, line := range lines[2:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(line, "│") {
			continue
		}
		if w := lipgloss.Width(line); w != headerW {
			t.Errorf("data row width %d != header width %d: %q", w, headerW, line)
		}
	}
}

func TestTruncateCell(t *testing.T) {
	// Short strings are padded to the full width
	got := truncateCell("hi", 10)
	if w := len([]rune(got)); w != 10 {
		t.Errorf("truncateCell(\"hi\", 10) should be 10 chars, got %d: %q", w, got)
	}
	if !strings.HasPrefix(got, "hi") {
		t.Errorf("truncateCell(\"hi\", 10) should start with 'hi', got %q", got)
	}

	// Long string should be truncated with ellipsis
	got = truncateCell("hello world", 5)
	if w := len([]rune(got)); w > 6 {
		t.Errorf("truncateCell(\"hello world\", 5) result too long: %q (%d runes)", got, w)
	}
}

func TestFkColumnFg(t *testing.T) {
	r := NewResultsTable()
	r.SetResult([]string{"id", "user_id", "name"}, [][]string{{"1", "2", "a"}}, "1 row")
	r.SetEditable("orders", []string{"id"})
	r.SetForeignKeys("orders", []db.ForeignKey{
		{Column: "user_id", RefTable: "users", RefColumn: "id"},
	})

	if _, ok := r.fkColumnFg(0); ok {
		t.Error("PK id col should stay unstyled")
	}
	if fg, ok := r.fkColumnFg(1); !ok || fg != colorFK {
		t.Errorf("user_id col: got (%q, %v), want colorFK", fg, ok)
	}
	if _, ok := r.fkColumnFg(2); ok {
		t.Error("name col should not be tinted")
	}

	// PK+FK: still unstyled (PK takes precedence — no tint).
	r.SetEditable("link", []string{"user_id"})
	r.SetForeignKeys("link", []db.ForeignKey{
		{Column: "user_id", RefTable: "users", RefColumn: "id"},
	})
	if _, ok := r.fkColumnFg(1); ok {
		t.Error("PK+FK col should stay unstyled like a PK")
	}
}

func TestResultsTableFKCellColorNotHeader(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	r := NewResultsTable()
	r.SetSize(80, 12)
	r.SetResult(
		[]string{"id", "user_id", "name"},
		[][]string{{"1", "9", "alice"}},
		"1 row",
	)
	r.SetEditable("orders", []string{"id"})
	r.SetForeignKeys("orders", []db.ForeignKey{
		{Column: "user_id", RefTable: "users", RefColumn: "id"},
	})
	// Move cursor off the FK column so the cell uses the default (tinted) style,
	// not the inverted cursor style.
	r.SetCursor(0, 0)

	view := r.View()
	lines := strings.Split(view, "\n")
	if len(lines) < 4 {
		t.Fatalf("view too short: %d lines", len(lines))
	}
	header, data := lines[1], lines[3]

	// Cursor row paints FK cells with fg+bg together, so match the combined style.
	fkCell := sgrPrefix(lipgloss.NewStyle().Foreground(colorFK).Background(colorCursorRow))
	fkAlone := sgrPrefix(lipgloss.NewStyle().Foreground(colorFK).Bold(true))
	primaryHeader := sgrPrefix(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true))

	if !strings.Contains(data, fkCell) {
		t.Errorf("data row missing FK cell tint %q\nin: %q", fkCell, data)
	}
	if strings.Contains(header, fkAlone) || strings.Contains(header, string(colorFK)) {
		t.Error("FK header should stay primary, not use the FK tint")
	}
	if !strings.Contains(header, primaryHeader) {
		t.Errorf("headers should still use primary %q", primaryHeader)
	}
	if !strings.Contains(stripAnsi(view), "id *") {
		t.Error("PK header should still show the * marker")
	}
}

func TestMixColors(t *testing.T) {
	a := lipgloss.Color("#000000")
	b := lipgloss.Color("#ffffff")
	mid := mixColors(a, b, 0.5)
	if mid != lipgloss.Color("#808080") {
		t.Errorf("mix mid = %q, want #808080", mid)
	}
	if mixColors(a, b, 0) != a {
		t.Errorf("t=0 should return a")
	}
	if mixColors(a, b, 1) != b {
		t.Errorf("t=1 should return b")
	}
}
