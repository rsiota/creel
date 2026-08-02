package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

// JSON export emits an array of objects keyed by column name.
func TestSerializeJSON(t *testing.T) {
	cols := []string{"id", "name"}
	rows := [][]string{{"1", "Ada"}, {"2", "Alan"}}
	got := serializeJSON(cols, rows)
	if !strings.HasPrefix(strings.TrimSpace(got), "[") {
		t.Errorf("expected JSON array, got %s", got)
	}
	for _, want := range []string{`"id": "1"`, `"name": "Ada"`, `"name": "Alan"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

// NULL cells become JSON null, not the string "NULL".
func TestSerializeJSONNull(t *testing.T) {
	got := serializeJSON([]string{"a"}, [][]string{{"NULL"}})
	if !strings.Contains(got, `"a": null`) {
		t.Errorf("expected null for NULL cell, got %s", got)
	}
	if strings.Contains(got, `"NULL"`) {
		t.Errorf("NULL sentinel leaked as string: %s", got)
	}
}

// Special characters in JSON values are escaped properly.
func TestSerializeJSONEscaping(t *testing.T) {
	got := serializeJSON([]string{"a"}, [][]string{{`he said "hi"`}})
	if !strings.Contains(got, `"he said \"hi\""`) {
		t.Errorf("expected escaped quotes, got %s", got)
	}
}

// JSON Lines emits one compact object per line.
func TestSerializeJSONL(t *testing.T) {
	cols := []string{"id", "name"}
	rows := [][]string{{"1", "Ada"}, {"2", "Alan"}}
	got := serializeJSONL(cols, rows)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], `"id":"1"`) || !strings.Contains(lines[0], `"name":"Ada"`) {
		t.Errorf("first line malformed: %s", lines[0])
	}
}

// Markdown renders a header, a separator, then one row per record; pipes in
// values are escaped so they don't break the table.
func TestSerializeMarkdown(t *testing.T) {
	cols := []string{"id", "name"}
	rows := [][]string{{"1", "Ada|Grace"}}
	got := serializeMarkdown(cols, rows)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header/sep/row), got %d: %q", len(lines), got)
	}
	if lines[0] != "| id | name |" {
		t.Errorf("header line malformed: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "| --- |") {
		t.Errorf("separator line malformed: %q", lines[1])
	}
	if !strings.Contains(lines[2], `Ada\|Grace`) {
		t.Errorf("pipe should be escaped in value: %q", lines[2])
	}
}

// TSV is tab-separated with a header row; NULL becomes empty.
func TestSerializeTSV(t *testing.T) {
	cols := []string{"id", "name"}
	rows := [][]string{{"1", "NULL"}}
	got := serializeTSV(cols, rows)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), got)
	}
	if lines[0] != "id\tname" {
		t.Errorf("header malformed: %q", lines[0])
	}
	if lines[1] != "1\t" {
		t.Errorf("data row should have empty cell for NULL, got %q", lines[1])
	}
}

// serializeFormat dispatches by format and rejects unknown formats.
func TestSerializeFormatDispatch(t *testing.T) {
	cols := []string{"a"}
	rows := [][]string{{"x"}}
	for _, f := range exportFormats {
		if _, err := serializeFormat(f, cols, rows); err != nil {
			t.Errorf("format %s: unexpected error %v", f, err)
		}
	}
	if _, err := serializeFormat(exportFormat("xml"), cols, rows); err == nil {
		t.Error("expected error for unsupported format")
	}
}

// writeExport writes a file with the correct extension and content.
func TestWriteExportJSON(t *testing.T) {
	cols := []string{"id"}
	rows := [][]string{{"1"}}
	path, count, err := writeExport(fmtJSON, cols, rows, "creel_test_unit.json")
	if err != nil {
		t.Fatalf("writeExport: %v", err)
	}
	if count != 1 {
		t.Errorf("count=%d, want 1", count)
	}
	if !strings.HasSuffix(path, "creel_test_unit.json") {
		t.Errorf("path=%s, want .json suffix", path)
	}
}

// exportFilename picks the extension per format.
func TestExportFilenamePerFormat(t *testing.T) {
	cases := map[exportFormat]string{
		fmtCSV:      ".csv",
		fmtJSON:     ".json",
		fmtJSONL:    ".jsonl",
		fmtMarkdown: ".md",
		fmtTSV:      ".tsv",
	}
	for f, ext := range cases {
		if got := exportFilename("t", "20260101_000000", f); !strings.HasSuffix(got, ext) {
			t.Errorf("format %s: got %q, want suffix %s", f, got, ext)
		}
	}
}

// The overlay opens with all columns checked, defaulting to CSV and a
// whole-table scope; Commit returns the selection and remembers the format.
func TestExportOverlayDefaults(t *testing.T) {
	o := NewExportOverlay()
	o.Show([]string{"id", "name", "email"}, true, 0, 200, 5234, true)

	// Down moves the cursor off the format rows without changing the selection.
	o.CursorDown() // onto JSON row, but selection stays CSV until activated
	if o.selectedFmt != fmtCSV {
		t.Errorf("traversing format rows changed selection to %v", o.selectedFmt)
	}
	// Activate selects JSON.
	o.Activate()
	if o.selectedFmt != fmtJSON {
		t.Fatalf("Activate: selectedFmt=%v, want json", o.selectedFmt)
	}

	format, cols, scope := o.Commit()
	if format != fmtJSON {
		t.Fatalf("Commit format=%v, want json", format)
	}
	if cols != nil {
		t.Errorf("all columns checked should yield nil cols, got %v", cols)
	}
	if scope != scopeAll {
		t.Errorf("default scope=%v, want scopeAll (whole table)", scope)
	}

	// Reopening remembers the last-used format (JSON).
	o.Show([]string{"id"}, true, 0, 1, 0, false)
	if o.selectedFmt != fmtJSON {
		t.Errorf("last-used format should be json, got %v", o.selectedFmt)
	}
}

// Column toggles project the export; the last checked column is protected.
func TestExportOverlayColumnToggle(t *testing.T) {
	o := NewExportOverlay()
	o.Show([]string{"id", "name", "email"}, false, 0, 2, 0, false)

	// Cursor starts at format row 0. Walk down to the first column (id):
	// formats (5) then column 0.
	for i := 0; i < len(exportFormats); i++ {
		o.CursorDown()
	}
	// id is checked; toggling it off leaves name+email.
	o.Activate()
	_, cols, _ := o.Commit()
	if len(cols) != 2 || cols[0] != "name" || cols[1] != "email" {
		t.Fatalf("after unchecking id, cols=%v, want [name email]", cols)
	}

	// Uncheck down to a single column; further unchecks are refused.
	o.Show([]string{"id", "name", "email"}, false, 0, 2, 0, false)
	o.SelectNoneCols() // keeps the first column (id)
	// Move to column 1 (name) and try to toggle it off — should be refused.
	for i := 0; i < len(exportFormats)+1; i++ {
		o.CursorDown()
	}
	o.Activate()
	o.Activate() // toggling again would turn it on; leave it
	_, cols, _ = o.Commit()
	if len(cols) != 1 {
		t.Fatalf("expected exactly one column kept, got %v", cols)
	}
}

// Cursor navigation clamps at the top and bottom of the flat entry list.
func TestExportOverlayNavClamp(t *testing.T) {
	o := NewExportOverlay()
	o.Show([]string{"a", "b"}, true, 0, 1, 0, false)
	total := len(o.entries)
	o.CursorUp() // already at top
	if o.cursor != 0 {
		t.Errorf("CursorUp at top moved cursor to %d", o.cursor)
	}
	for i := 0; i < total; i++ {
		o.CursorDown()
	}
	if o.cursor != total-1 {
		t.Errorf("expected cursor clamped at %d, got %d", total-1, o.cursor)
	}
}

// g X in the results panel opens the export dialog. (g x is close-tab, so the
// export-as binding uses capital X — this guards against regressing back to a
// clash with the tab-management prefix.)
func TestGXOpensExportDialog(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(dir, "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = conn
	m.state = stateWorkspace
	m.focus = FocusResults
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}}, "1 row")

	// g sets the pending-G prefix.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = res.(Model)
	if !m.resultsPendingG {
		t.Fatal("g should arm the g-prefix")
	}
	// X opens the dialog.
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = res.(Model)
	if !m.exportOverlay.IsVisible() {
		t.Errorf("export dialog should be visible after g X")
	}
}

// With the dialog open, the default selection (whole-table CSV, all columns)
// exports immediately on enter. This is the headline two-keystroke flow.
func TestExportDialogEnterDefaults(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.db")
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	// Seed a table with more than one row so whole-table export is meaningful.
	if _, err := conn.DB().Execute(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.DB().Execute(`INSERT INTO users (id, name, email) VALUES (1,'a','a@x'), (2,'b','b@x')`); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = conn
	m.state = stateWorkspace
	m.focus = FocusResults
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	m.results.SetEditable("users", []string{"id"})
	m.results.SetResult([]string{"id", "name", "email"}, [][]string{{"1", "a", "a@x"}}, "1 row")

	// Open the dialog (g X) and immediately commit.
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = res.(Model)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = res.(Model)
	if !m.exportOverlay.IsVisible() {
		t.Fatal("dialog should be open after g X")
	}
	// Enter triggers an async whole-table export; it returns a command, so
	// exportMsg is empty until exportDoneMsg lands. Verify the dialog closed.
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(Model)
	if m.exportOverlay.IsVisible() {
		t.Error("dialog should hide after enter")
	}
	if cmd == nil {
		t.Fatal("whole-table export should return an async command")
	}
}

// With a couple of columns unchecked, enter exports the projected set to a CSV.
func TestExportDialogColumnProjection(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.db")
	conn, err := db.New(db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.DB().Execute(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.DB().Execute(`INSERT INTO users (id, name, email) VALUES (1,'a','a@x')`); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = conn
	m.state = stateWorkspace
	m.focus = FocusResults
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	m.results.SetEditable("users", []string{"id"})
	m.results.SetResult([]string{"id", "name", "email"}, [][]string{{"1", "a", "a@x"}}, "1 row")

	// Open the dialog and switch scope to Current page (so the export is the
	// in-memory, synchronous path and sets exportMsg directly).
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = res.(Model)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = res.(Model)

	// Cursor starts at format row 0 (CSV). Walk down past the formats and the
	// three columns to land on the Scope section, then to the Current page row
	// (the last scope option).
	steps := len(exportFormats) /*formats*/ + 3 /*columns*/ + 1 /*first scope*/
	for i := 0; i < steps; i++ {
		res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = res.(Model)
	}
	// Select the Current-page scope.
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = res.(Model)

	// Now uncheck the "name" column: move up into the columns section. Cursor
	// is on the first scope row; the columns are directly above. Move up to the
	// third column (email), then to the second (name).
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}) // email
	m = res.(Model)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}) // name
	m = res.(Model)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // uncheck name
	m = res.(Model)

	// Enter exports the current page (id, email) to CSV.
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(Model)
	if !strings.Contains(m.exportMsg, "exported") {
		t.Fatalf("expected an export status message, got %q", m.exportMsg)
	}
	if !strings.Contains(m.exportMsg, ".csv") {
		t.Errorf("expected a .csv file in %q", m.exportMsg)
	}
}
