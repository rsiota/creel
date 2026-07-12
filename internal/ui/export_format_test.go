package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
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
	path, count, err := writeExport(fmtJSON, cols, rows, "gsql_test_unit.json")
	if err != nil {
		t.Fatalf("writeExport: %v", err)
	}
	if count != 1 {
		t.Errorf("count=%d, want 1", count)
	}
	if !strings.HasSuffix(path, "gsql_test_unit.json") {
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

// The format picker opens on the last-used format and Commit records the choice.
func TestFormatPickerLastUsed(t *testing.T) {
	p := NewFormatPicker()
	p.Show()
	// Default is CSV.
	if p.Selected() != fmtCSV {
		t.Fatalf("default selected=%v, want csv", p.Selected())
	}
	// Move to JSON and commit.
	p.Down() // csv -> json
	if p.Selected() != fmtJSON {
		t.Fatalf("after Down, selected=%v, want json", p.Selected())
	}
	if got := p.Commit(); got != fmtJSON {
		t.Fatalf("Commit=%v, want json", got)
	}
	// Reopening should start on JSON (last-used).
	p.Show()
	if p.Selected() != fmtJSON {
		t.Errorf("last-used should be json, got %v", p.Selected())
	}
}

// Up/Down clamp at the ends.
func TestFormatPickerClamp(t *testing.T) {
	p := NewFormatPicker()
	p.Show()
	p.Up() // already at top
	if p.cursor != 0 {
		t.Errorf("Up at top moved cursor to %d", p.cursor)
	}
	// Move to bottom.
	for i := 0; i < len(exportFormats); i++ {
		p.Down()
	}
	last := len(exportFormats) - 1
	if p.cursor != last {
		t.Errorf("expected cursor clamped at %d, got %d", last, p.cursor)
	}
}

// g X in the results panel opens the format picker. (g x is close-tab, so the
// export-as binding uses capital X — this guards against regressing back to a
// clash with the tab-management prefix.)
func TestGXOpensFormatPicker(t *testing.T) {
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
	// X opens the picker.
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = res.(Model)
	if !m.formatPicker.IsVisible() {
		t.Errorf("format picker should be visible after g X")
	}
}

// With the picker open, j moves the cursor and enter exports (and closes it).
func TestFormatPickerNavAndExport(t *testing.T) {
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
	m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "1 row")

	// Open the picker and move to JSON (csv -> json).
	m.formatPicker.Show()
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = res.(Model)
	if m.formatPicker.Selected() != fmtJSON {
		t.Fatalf("cursor on %v, want json", m.formatPicker.Selected())
	}
	// Enter exports in JSON and hides the picker.
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(Model)
	if m.formatPicker.IsVisible() {
		t.Error("picker should hide after enter")
	}
	if !strings.Contains(m.exportMsg, "exported") {
		t.Errorf("expected an export status message, got %q", m.exportMsg)
	}
	if !strings.Contains(m.exportMsg, ".json") {
		t.Errorf("expected a .json file in %q", m.exportMsg)
	}
}
