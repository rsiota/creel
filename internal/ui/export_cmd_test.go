package ui

import (
	"strings"
	"testing"
)

func TestParseExportFormat(t *testing.T) {
	cases := []struct {
		in   string
		want exportFormat
		ok   bool
	}{
		{"csv", fmtCSV, true},
		{"JSON", fmtJSON, true}, // case-insensitive
		{"jsonl", fmtJSONL, true},
		{"md", fmtMarkdown, true},       // extension
		{"markdown", fmtMarkdown, true}, // label
		{"json lines", fmtJSONL, true},  // label (lowercased)
		{"tsv", fmtTSV, true},
		{"  CSV ", fmtCSV, true}, // whitespace trimmed
		{"", "", false},
		{"bogus", "", false},
		{"xml", "", false},
	}
	for _, c := range cases {
		got, ok := parseExportFormat(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseExportFormat(%q) = %q,%v; want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestExExportInvalidFormat(t *testing.T) {
	m := &Model{results: NewResultsTable()}
	m.runExCommand("export xml")
	if !strings.Contains(m.schemaMsg, "needs a format") {
		t.Errorf(":export xml -> %q", m.schemaMsg)
	}
}

func TestExExportNoArg(t *testing.T) {
	m := &Model{results: NewResultsTable()}
	m.runExCommand("export")
	if !strings.Contains(m.schemaMsg, "needs a format") {
		t.Errorf(":export (no arg) -> %q", m.schemaMsg)
	}
}

// With no result rows, exportResults short-circuits to "nothing to export"
// without touching the filesystem — a clean way to verify the wiring reuses
// the exporter without writing to ~/Downloads in a unit test.
func TestExExportNothingToExport(t *testing.T) {
	m := &Model{results: NewResultsTable()}
	cmd := m.runExCommand("export json")
	if cmd != nil {
		t.Errorf(":export with no rows should return nil, got %v", cmd)
	}
	if m.exportMsg != "nothing to export" {
		t.Errorf(":export json (no rows) -> exportMsg %q, want %q", m.exportMsg, "nothing to export")
	}
}
