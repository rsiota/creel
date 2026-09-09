package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/config"
	"github.com/rsiota/creel/internal/db"
)

func TestExSetTransparentBackground(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{}
	m := NewModel(cfg)

	m.runExCommand("set transparent_background on")
	if !m.settings.TransparentBackground || !cfg.Settings.TransparentBackground {
		t.Errorf("transparent_background not on: settings=%v config=%v",
			m.settings.TransparentBackground, cfg.Settings.TransparentBackground)
	}
	if !strings.Contains(m.schemaMsg, "on") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	m.runExCommand("set transparent off")
	if m.settings.TransparentBackground || cfg.Settings.TransparentBackground {
		t.Error("transparent alias should turn background painting back on")
	}
}

func TestExSetShowsValue(t *testing.T) {
	m := NewModel(&config.Config{})
	m.runExCommand("set transparent_background")
	if !strings.Contains(m.schemaMsg, "transparent_background=off") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExSetUnknownOption(t *testing.T) {
	m := NewModel(&config.Config{})
	m.runExCommand("set not_a_real_option on")
	if !strings.Contains(m.schemaMsg, "unknown setting") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExSetConfirmDestructive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{}
	m := NewModel(cfg)

	m.runExCommand("set confirm_destructive off")
	if cfg.Settings.ConfirmDestructive == nil || *cfg.Settings.ConfirmDestructive {
		t.Error("confirm_destructive should be false")
	}

	m.runExCommand("set confirm_destructive default")
	if cfg.Settings.ConfirmDestructive != nil {
		t.Error("default should clear confirm_destructive pointer")
	}
}

func TestExSetStatusHints(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{}
	m := NewModel(cfg)
	m.width = 160
	m.state = stateWorkspace
	m.focus = FocusResults

	if !m.statusHintsEnabled() {
		t.Fatal("status_hints should default on")
	}
	hints := m.hintList()
	if len(hints) == 0 {
		t.Fatal("expected Results hints")
	}
	// Prefer a token that won't appear in "? help" or "status_hints=off".
	needle := ""
	for _, h := range hints {
		for _, part := range strings.Split(h, "/") {
			if len(part) >= 2 && part != "help" && !strings.Contains(part, "hint") {
				needle = part
				break
			}
		}
		if needle != "" {
			break
		}
	}
	if needle == "" {
		t.Fatalf("no distinctive hint in %v", hints)
	}
	barOn := stripAnsi(m.statusBar("c"))
	if !strings.Contains(barOn, needle) {
		t.Fatalf("status bar missing hint %q from %v: %q", needle, hints, barOn)
	}

	m.runExCommand("set status_hints off")
	if cfg.Settings.StatusHints == nil || *cfg.Settings.StatusHints {
		t.Error("status_hints should be false")
	}
	if m.statusHintsEnabled() {
		t.Error("statusHintsEnabled should be false after off")
	}
	m.schemaMsg = "" // avoid "status_hints=off" matching the needle
	barOff := stripAnsi(m.statusBar("c"))
	if strings.Contains(barOff, needle) {
		t.Errorf("context hint %q should be hidden: %q", needle, barOff)
	}
	if !strings.Contains(barOff, "?") || !strings.Contains(barOff, "help") {
		t.Errorf("? help should remain: %q", barOff)
	}

	m.runExCommand("set hints on")
	if cfg.Settings.StatusHints != nil {
		t.Error("on should clear status_hints pointer (default)")
	}
	if !m.statusHintsEnabled() {
		t.Error("statusHintsEnabled should be true after on")
	}

	m.runExCommand("set status_hints off")
	m.runExCommand("set status_hints default")
	if cfg.Settings.StatusHints != nil {
		t.Error("default should clear status_hints pointer")
	}
}

func TestExSetPageSize(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{}
	m := NewModel(cfg)

	m.runExCommand("set page_size 321")
	if m.pageSize != 321 || cfg.Settings.PageSize != 321 {
		t.Errorf("page_size = model %d config %d, want 321", m.pageSize, cfg.Settings.PageSize)
	}
}

func TestExSetCompletion(t *testing.T) {
	m := &Model{}
	m.ex.input = "set trans"
	m.recomputeExCompletion()
	got := exCandidates(m.ex.comp)
	if len(got) == 0 || got[0] != "transparent_background" {
		t.Errorf("set partial candidates = %v, want transparent_background first", got)
	}

	m.ex.input = "set transparent_background "
	m.recomputeExCompletion()
	got = exCandidates(m.ex.comp)
	if len(got) < 2 || !containsAll(got, "on", "off") {
		t.Errorf("bool value candidates = %v", got)
	}
}

func containsAll(items []string, want ...string) bool {
	set := make(map[string]bool, len(items))
	for _, s := range items {
		set[s] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func TestLookupSettingAliases(t *testing.T) {
	if lookupSetting("transparent") == nil {
		t.Fatal("transparent alias not resolved")
	}
	if lookupSetting("timeout") == nil {
		t.Fatal("timeout alias not resolved")
	}
	if lookupSetting("inspector") == nil {
		t.Fatal("inspector alias not resolved")
	}
}

func TestExSetInspectorOpen(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{}
	m := NewModel(cfg)
	m.state = stateWorkspace

	m.runExCommand("set inspector_open on")
	if !m.settings.InspectorOpen || !cfg.Settings.InspectorOpen {
		t.Errorf("inspector_open not on: settings=%v config=%v",
			m.settings.InspectorOpen, cfg.Settings.InspectorOpen)
	}
	if !m.inspector.IsVisible() {
		t.Error("inspector should open immediately on :set on")
	}
	if m.focus != FocusInspector {
		t.Errorf("focus = %v, want inspector after :set on", m.focus)
	}

	m.runExCommand("set inspector off")
	if m.settings.InspectorOpen || cfg.Settings.InspectorOpen {
		t.Error("inspector alias should turn preference off")
	}
	if m.inspector.IsVisible() {
		t.Error("inspector should close on :set off")
	}
}

func TestExSetInspectorSync(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{}
	m := NewModel(cfg)
	if m.settings.InspectorSync {
		t.Fatal("inspector_sync should default off")
	}

	m.runExCommand("set inspector_sync on")
	if !m.settings.InspectorSync || !cfg.Settings.InspectorSync {
		t.Errorf("inspector_sync not on: settings=%v config=%v",
			m.settings.InspectorSync, cfg.Settings.InspectorSync)
	}
	if !strings.Contains(m.schemaMsg, "inspector_sync=on") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	m.runExCommand("set column_sync off")
	if m.settings.InspectorSync || cfg.Settings.InspectorSync {
		t.Error("column_sync alias should turn preference off")
	}
}

func TestConnectOpensInspectorWhenPreferred(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "startup.db")
	s := db.NewSQLite(db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath})
	if err := s.Connect(); err != nil {
		t.Fatalf("setup connect: %v", err)
	}
	if _, err := s.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	s.Close()

	cfg := &config.Config{Settings: config.Settings{InspectorOpen: true}}
	m := NewModel(cfg)
	m.connectWithConfig(db.ConnectionConfig{
		Driver:   db.DriverSQLite,
		Database: dbPath,
	})
	t.Cleanup(func() {
		if m.connection != nil {
			m.connection.Close()
		}
	})
	if m.connError != "" {
		t.Fatalf("connError = %q", m.connError)
	}
	if !m.inspector.IsVisible() {
		t.Fatal("inspector should open on connect when inspector_open is on")
	}
	// Startup opens the panel without stealing editor focus.
	if m.focus == FocusInspector {
		t.Error("connect should not steal focus to inspector")
	}
}

