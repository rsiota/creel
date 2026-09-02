package ui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/config"
)

// groupedConns returns a mix of grouped and ungrouped connections used across
// the group tests: Work(2), Personal(1), ungrouped(1).
func groupedConns() []config.ConnectionConfig {
	return []config.ConnectionConfig{
		{Name: "wk-a", Driver: "sqlite", Database: "/tmp/a.db", Group: "Work"},
		{Name: "wk-b", Driver: "sqlite", Database: "/tmp/b.db", Group: "Work"},
		{Name: "pers-c", Driver: "sqlite", Database: "/tmp/c.db", Group: "Personal"},
		{Name: "solo", Driver: "sqlite", Database: "/tmp/d.db"},
	}
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripAnsi(s string) string { return ansiRe.ReplaceAllString(s, "") }

// With no groups in use the list renders flat — no tab strip.
func TestGroupsFlatWhenNoneGrouped(t *testing.T) {
	m := newConnListModel(t, makeConns(3), 40)
	if m.connList.hasGroups() {
		t.Fatal("hasGroups=true for all-ungrouped list")
	}
	if m.connList.showGroupTabs() {
		t.Fatal("showGroupTabs=true when no groups")
	}
	view := stripAnsi(m.connList.View())
	if strings.Contains(view, "Ungrouped") {
		t.Errorf("flat list should have no group tabs:\n%s", view)
	}
}

// Tab order is named groups A–Z, then Ungrouped when any ungrouped exist.
func TestGroupsTabOrder(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.CancelFilter()
	tabs := m.connList.availableGroupTabs()
	want := []string{"Personal", "Work", ""}
	if len(tabs) != len(want) {
		t.Fatalf("tabs=%v, want %v", tabs, want)
	}
	for i, g := range want {
		if tabs[i] != g {
			t.Fatalf("tab[%d]=%q, want %q", i, tabs[i], g)
		}
	}
	// Default tab is Ungrouped when ungrouped connections exist.
	if got := m.connList.ActiveGroupTab(); got != "" {
		t.Errorf("default groupTab=%q, want Ungrouped (\"\")", got)
	}
	rows := m.connList.rows()
	if len(rows) != 1 || rows[0].conn.name != "solo" {
		t.Errorf("default tab rows=%v, want [solo]", namesOf(rows))
	}
}

// Switching tabs shows only that group's connections.
func TestGroupsTabSwitchShowsGroup(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.CancelFilter()
	m.connList.setGroupTab("Work")
	rows := m.connList.rows()
	if got := namesOf(rows); strings.Join(got, ",") != "wk-a,wk-b" {
		t.Errorf("Work tab = %v, want wk-a,wk-b", got)
	}
	m.connList.setGroupTab("Personal")
	if got := namesOf(m.connList.rows()); strings.Join(got, ",") != "pers-c" {
		t.Errorf("Personal tab = %v, want pers-c", got)
	}
}

// GroupTabBar is a right-aligned strip separate from the row View (tabs sit
// above the filter prompt in the popup).
func TestGroupsTabBarAbovePrompt(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.CancelFilter()
	m.connList.setGroupTab("Work")
	tabLine := stripAnsi(m.connList.GroupTabBar())
	for _, label := range []string{"Personal", "Work", "Ungrouped"} {
		if !strings.Contains(tabLine, label) {
			t.Errorf("tab bar missing %q: %q", label, tabLine)
		}
	}
	view := stripAnsi(m.connList.View())
	if strings.Contains(view, "Ungrouped") {
		t.Errorf("row View should not include the tab strip:\n%s", view)
	}
	if !strings.Contains(view, "wk-a") || strings.Contains(view, "pers-c") {
		t.Errorf("Work tab should show wk-a only:\n%s", view)
	}
}

// Filtering flattens across groups; the tab strip stays visible and follows
// the match under the cursor.
func TestGroupsFilterKeepsTabsVisible(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.StartFilter()
	m.connList.FilterAddChar("wk") // matches wk-a, wk-b
	if !m.connList.showGroupTabs() {
		t.Error("filter should keep group tabs visible")
	}
	if bar := m.connList.GroupTabBar(); bar == "" {
		t.Error("GroupTabBar empty during filter")
	}
	rows := m.connList.rows()
	if len(rows) != 2 {
		t.Errorf("filter matches = %d rows, want 2", len(rows))
	}
	m.connList.SetCursor(0)
	if got := m.connList.ActiveGroupTab(); got != "Work" {
		t.Errorf("tab highlight after SetCursor=%q, want Work", got)
	}
}

// Committing a filter selection jumps to that connection's group tab.
func TestGroupsCommitFilterRelocates(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.StartFilter()
	m.connList.FilterAddChar("pers") // matches pers-c
	m.connList.SetCursor(0)
	m.connList.CommitFilter()

	if m.connList.IsFiltering() {
		t.Error("CommitFilter should exit filter mode")
	}
	if got := m.connList.ActiveGroupTab(); got != "Personal" {
		t.Errorf("after commit groupTab=%q, want Personal", got)
	}
	if got := m.connList.SelectedName(); got != "pers-c" {
		t.Errorf("after commit cursor = %q, want pers-c", got)
	}
}

// ExpandedHeight counts connections only (tabs are not list rows).
func TestGroupsExpandedHeightIsConnectionCount(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	if got := m.connList.ExpandedHeight(); got != 4 {
		t.Errorf("ExpandedHeight=%d, want 4", got)
	}
}

// Popup height stays constant while switching group tabs.
func TestGroupsPopupHeightConstantWhileSwitchingTabs(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 60)
	m.connList.CancelFilter()
	_, h0 := m.connListPopupDims()
	m.connList.MoveGroupTab(1)
	_, h1 := m.connListPopupDims()
	if h0 != h1 {
		t.Errorf("popup height changed on tab switch: %d -> %d", h0, h1)
	}
}

// [ / ] cycle group tabs via the app key handler.
func TestGroupsBracketKeysSwitchTabsViaApp(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.CancelFilter()
	if m.connList.ActiveGroupTab() != "" {
		t.Fatalf("start on Ungrouped, got %q", m.connList.ActiveGroupTab())
	}

	mm, _ := m.updateConnections(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)
	if got := m.connList.ActiveGroupTab(); got != "Personal" {
		// tabs: Personal, Work, Ungrouped — ] from Ungrouped wraps to Personal
		t.Errorf("] from Ungrouped -> %q, want Personal", got)
	}
	if got := m.connList.SelectedName(); got != "pers-c" {
		t.Errorf("Personal tab selection = %q, want pers-c", got)
	}

	mm, _ = m.updateConnections(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)
	if got := m.connList.ActiveGroupTab(); got != "Work" {
		t.Errorf("] from Personal -> %q, want Work", got)
	}

	mm, _ = m.updateConnections(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m = mm.(Model)
	if got := m.connList.ActiveGroupTab(); got != "Personal" {
		t.Errorf("[ from Work -> %q, want Personal", got)
	}
}

// SelectByName switches onto the connection's group tab.
func TestGroupsSelectByNameSwitchesTab(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.CancelFilter()
	if !m.connList.SelectByName("wk-b") {
		t.Fatal("SelectByName wk-b failed")
	}
	if got := m.connList.ActiveGroupTab(); got != "Work" {
		t.Errorf("groupTab=%q, want Work", got)
	}
	if got := m.connList.SelectedName(); got != "wk-b" {
		t.Errorf("SelectedName=%q, want wk-b", got)
	}
}

// The form round-trips the Group field, and editing a connection preserves it.
func TestFormGroupFieldRoundTrip(t *testing.T) {
	f := NewConnectionForm()
	f.fields[fieldName].SetValue("x")
	f.fields[fieldDatabase].SetValue("/tmp/x.db")
	f.fields[fieldGroup].SetValue("Work")
	cfg, errMsg := f.EnterPressed()
	if errMsg != "" {
		t.Fatalf("EnterPressed error: %q", errMsg)
	}
	if cfg.Group != "Work" {
		t.Errorf("cfg.Group=%q, want Work", cfg.Group)
	}

	edit := NewConnectionFormEdit(config.ConnectionConfig{
		Name: "x", Driver: "sqlite", Database: "/tmp/x.db", Group: "Personal",
	})
	if got := stripAnsi(edit.fields[fieldGroup].Value()); got != "Personal" {
		t.Errorf("edit form group=%q, want Personal", got)
	}
}

func namesOf(rows []connRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.kind == rowConn {
			out = append(out, r.conn.name)
		}
	}
	return out
}
