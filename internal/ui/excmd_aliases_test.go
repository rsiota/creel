package ui

import (
	"strings"
	"testing"

	"github.com/ruben/creel/internal/config"
	"github.com/ruben/creel/internal/db"
)

// Behavioral tests for the Step 5 ":" aliases. These cover the cheap-to-test
// paths (error guards, wiring) without a live database connection.

func TestExFormat(t *testing.T) {
	m := &Model{editor: NewQueryEditor()}
	m.editor.SetValue("select   1")
	m.runExCommand("format")
	want := formatSQL("select   1")
	if m.editor.Value() != want {
		t.Errorf(":format -> %q, want %q", m.editor.Value(), want)
	}
}

func TestExThemeBadName(t *testing.T) {
	m := &Model{}
	m.runExCommand("theme nope-not-a-theme")
	if !strings.Contains(m.schemaMsg, "no such theme") {
		t.Errorf(":theme bad name -> %q", m.schemaMsg)
	}
}

func TestExThemeMissingArg(t *testing.T) {
	m := &Model{}
	m.runExCommand("theme")
	if !strings.Contains(m.schemaMsg, "needs a name") {
		t.Errorf(":theme with no arg -> %q", m.schemaMsg)
	}
}

func TestExIconsBadName(t *testing.T) {
	m := &Model{}
	m.runExCommand("icons nope")
	if !strings.Contains(m.schemaMsg, "unknown icon set") {
		t.Errorf(":icons bad name -> %q", m.schemaMsg)
	}
}

func TestExIconsMissingArg(t *testing.T) {
	m := &Model{}
	m.runExCommand("icons")
	if !strings.Contains(m.schemaMsg, "needs a name") {
		t.Errorf(":icons with no arg -> %q", m.schemaMsg)
	}
}

// TestExIconsAppliesAndPersists verifies :icons nerdfont swaps the active glyph
// set, updates Settings, writes config, and reports status — mirroring exTheme
// — and that :icons unicode restores the triangles. The package-level icons
// var is reset on exit so other tests are unaffected.
func TestExIconsAppliesAndPersists(t *testing.T) {
	defer applyIcons("") // restore default triangles

	// :icons persists via m.config.Save(); redirect the config dir so the test
	// does not overwrite the user's real ~/.config/creel/config.yaml.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{}
	m := NewModel(cfg)
	if got := icons.collapsed; got != "▸" {
		t.Fatalf("default collapsed glyph = %q, want ▸", got)
	}

	m.runExCommand("icons nerdfont")
	if icons.collapsed != "\uf105" || icons.expanded != "\uf107" {
		t.Errorf(":icons nerdfont -> collapsed=%q expanded=%q, want U+F105/U+F107",
			icons.collapsed, icons.expanded)
	}
	if m.settings.Icons != "nerdfont" || cfg.Settings.Icons != "nerdfont" {
		t.Errorf(":icons nerdfont not persisted: settings=%q config=%q",
			m.settings.Icons, cfg.Settings.Icons)
	}
	if !strings.Contains(m.schemaMsg, "icons:") {
		t.Errorf(":icons nerdfont schemaMsg = %q", m.schemaMsg)
	}

	m.runExCommand("icons unicode")
	if icons.collapsed != "▸" || icons.expanded != "▾" {
		t.Errorf(":icons unicode -> collapsed=%q expanded=%q", icons.collapsed, icons.expanded)
	}
	if m.settings.Icons != "" || cfg.Settings.Icons != "" {
		t.Errorf(":icons unicode should clear Icons, got settings=%q config=%q",
			m.settings.Icons, cfg.Settings.Icons)
	}
}

func TestExStatsNoResults(t *testing.T) {
	m := &Model{results: NewResultsTable()}
	m.runExCommand("stats")
	if !strings.Contains(m.schemaMsg, "no results") {
		t.Errorf(":stats with no results -> %q", m.schemaMsg)
	}
}

func TestExStatsBadColumn(t *testing.T) {
	m := &Model{connection: &db.Connection{}, results: NewResultsTable()}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "a"}}, "")
	m.runExCommand("stats nope")
	if !strings.Contains(m.schemaMsg, "no such column") {
		t.Errorf(":stats bad column -> %q", m.schemaMsg)
	}
}

func TestExDescribeNotConnected(t *testing.T) {
	m := &Model{}
	m.runExCommand("describe users")
	if !strings.Contains(m.schemaMsg, "not connected") {
		t.Errorf(":describe with no connection -> %q", m.schemaMsg)
	}
}

func TestExHistoryNoConnection(t *testing.T) {
	m := &Model{}
	m.runExCommand("history")
	// No connection → toggleHistory is a no-op; panel must stay hidden, no panic.
	if m.history.IsVisible() {
		t.Error(":history with no connection should not open the panel")
	}
}

func TestExBookmarksNoConnection(t *testing.T) {
	m := &Model{}
	m.runExCommand("bookmarks")
	if m.bookmarks.IsVisible() {
		t.Error(":bookmarks with no connection should not open the panel")
	}
}
