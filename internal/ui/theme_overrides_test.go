package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/rsiota/creel/internal/config"
)

func TestParseHexColor(t *testing.T) {
	cases := []struct {
		in   string
		want lipgloss.Color
		ok   bool
	}{
		{"#abc", "#aabbcc", true},
		{"#AaBbCc", "#aabbcc", true},
		{"aabbcc", "#aabbcc", true},
		{"#fff", "#ffffff", true},
		{"bad", "", false}, // bare 3-char rejected
		{"#gg0000", "", false},
		{"#ab", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := parseHexColor(tc.in)
		if ok != tc.ok {
			t.Errorf("parseHexColor(%q) ok=%v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("parseHexColor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWithThemeOverrides(t *testing.T) {
	base := defaultPalette
	got := withThemeOverrides(base, map[string]string{
		"muted": "#abcdef",
		"error": "#ff0000", // alias for err
		"nope":  "#123456", // unknown slot skipped
		"fg":    "notahex", // invalid hex skipped
	})
	if got.muted != lipgloss.Color("#abcdef") {
		t.Errorf("muted = %q, want #abcdef", got.muted)
	}
	if got.err != lipgloss.Color("#ff0000") {
		t.Errorf("err = %q, want #ff0000", got.err)
	}
	if got.fg != base.fg {
		t.Errorf("invalid fg override should leave base fg")
	}
	if got.primary != base.primary {
		t.Errorf("unknown slot should not change primary")
	}
}

func TestExColorOverridePersistsAndApplies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	defer applyPalette(defaultPalette)

	cfg := &config.Config{Settings: config.Settings{Theme: "tokyo-night"}}
	m := NewModel(cfg)

	m.runExCommand("color muted #abcdef")
	if cfg.Settings.ThemeOverrides["muted"] != "#abcdef" {
		t.Fatalf("config override = %v", cfg.Settings.ThemeOverrides)
	}
	if colorMuted != lipgloss.Color("#abcdef") {
		t.Errorf("colorMuted = %q after :color", colorMuted)
	}

	// Theme switch keeps the override.
	m.runExCommand("theme nord")
	if colorMuted != lipgloss.Color("#abcdef") {
		t.Errorf("override should survive :theme: colorMuted=%q", colorMuted)
	}
	if colorPrimary != nordPalette.primary {
		t.Errorf("nord primary not applied: %q", colorPrimary)
	}

	m.runExCommand("color muted default")
	if cfg.Settings.ThemeOverrides != nil {
		t.Errorf("clear should nil map, got %v", cfg.Settings.ThemeOverrides)
	}
	if colorMuted != nordPalette.muted {
		t.Errorf("after clear, muted should be nord's: got %q want %q", colorMuted, nordPalette.muted)
	}
}

func TestExColorListAndShow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(&config.Config{})
	m.runExCommand("color")
	if !strings.Contains(m.schemaMsg, "no theme overrides") {
		t.Errorf("empty list: %q", m.schemaMsg)
	}
	m.runExCommand("color muted #abc")
	m.runExCommand("color")
	if !strings.Contains(m.schemaMsg, "muted=#aabbcc") {
		t.Errorf("list overrides: %q", m.schemaMsg)
	}
	m.runExCommand("color muted")
	if !strings.Contains(m.schemaMsg, "override") {
		t.Errorf("show override: %q", m.schemaMsg)
	}
}

func TestExColorsOverlay(t *testing.T) {
	defer applyPalette(defaultPalette)
	cfg := &config.Config{Settings: config.Settings{
		Theme: "nord",
		ThemeOverrides: map[string]string{
			"muted": "#abcdef",
		},
	}}
	m := NewModel(cfg)
	m.runExCommand("colors")
	if !m.lookupPanel.IsVisible() {
		t.Fatal(":colors should open the lookup overlay")
	}
	if !strings.Contains(m.lookupPanel.title, "nord") {
		t.Errorf("title = %q, want nord", m.lookupPanel.title)
	}
	if got := len(m.lookupPanel.result.Rows); got != len(themeOverrideSlots) {
		t.Fatalf("rows = %d, want %d slots", got, len(themeOverrideSlots))
	}
	foundMuted := false
	foundFK := false
	for _, row := range m.lookupPanel.result.Rows {
		if len(row) < 3 {
			t.Fatalf("row too short: %v", row)
		}
		if len(row[1]) != 7 || row[1][0] != '#' {
			t.Errorf("slot %q colour %q should be #rrggbb for column alignment", row[0], row[1])
		}
		if row[0] == "muted" {
			foundMuted = true
			if row[1] != "#abcdef" {
				t.Errorf("muted colour = %q, want #abcdef", row[1])
			}
			if row[2] != "override" {
				t.Errorf("muted source = %q, want override", row[2])
			}
		}
		if row[0] == "primary" && row[2] != "theme" {
			t.Errorf("primary source = %q, want theme", row[2])
		}
		if row[0] == "fk" {
			foundFK = true
			if row[2] != "derived" {
				t.Errorf("fk source = %q, want derived (empty palette slot)", row[2])
			}
		}
	}
	if !foundMuted {
		t.Fatal("muted row missing")
	}
	if !foundFK {
		t.Fatal("fk row missing")
	}
}

func TestNewModelAppliesThemeOverrides(t *testing.T) {
	defer applyPalette(defaultPalette)
	cfg := &config.Config{Settings: config.Settings{
		Theme: "gruvbox",
		ThemeOverrides: map[string]string{
			"primary": "#112233",
		},
	}}
	_ = NewModel(cfg)
	if colorPrimary != lipgloss.Color("#112233") {
		t.Errorf("startup override not applied: primary=%q", colorPrimary)
	}
	if colorBg != gruvboxPalette.bg {
		t.Errorf("base theme bg should remain: got %q want %q", colorBg, gruvboxPalette.bg)
	}
}

func TestThemePickerPreviewKeepsOverrides(t *testing.T) {
	defer applyPalette(defaultPalette)
	p := NewThemePicker()
	p.Show("tokyo-night", map[string]string{"muted": "#abcdef"})
	p.Down() // gruvbox
	if colorMuted != lipgloss.Color("#abcdef") {
		t.Errorf("picker preview should keep muted override: %q", colorMuted)
	}
	if colorPrimary != gruvboxPalette.primary {
		t.Errorf("expected gruvbox primary, got %q", colorPrimary)
	}
}
