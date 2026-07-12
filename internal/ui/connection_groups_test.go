package ui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
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

func countRows(rows []connRow) (headers, conns int) {
	for _, r := range rows {
		if r.kind == rowGroup {
			headers++
		} else {
			conns++
		}
	}
	return
}

// With no groups in use the list renders flat — no headers, byte-for-byte the
// pre-groups behaviour.
func TestGroupsFlatWhenNoneGrouped(t *testing.T) {
	m := newConnListModel(t, makeConns(3), 40)
	if m.connList.hasGroups() {
		t.Fatal("hasGroups=true for all-ungrouped list")
	}
	view := stripAnsi(m.connList.View())
	if strings.Contains(view, "▾") || strings.Contains(view, "▸") {
		t.Errorf("flat list should have no fold markers:\n%s", view)
	}
}

// The grouped layout puts ungrouped first, then named groups alphabetically,
// each under a header.
func TestGroupsLayoutOrder(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	rows := m.connList.rows()
	headers, conns := countRows(rows)
	if headers != 3 || conns != 4 {
		t.Fatalf("expanded rows: headers=%d conns=%d, want 3/4", headers, conns)
	}
	// Verify header sequence and that each header is followed by its conns.
	type want struct {
		group string
		conns []string
	}
	expected := []want{
		{"", []string{"solo"}},
		{"Personal", []string{"pers-c"}},
		{"Work", []string{"wk-a", "wk-b"}},
	}
	idx := 0
	for _, w := range expected {
		if rows[idx].kind != rowGroup || rows[idx].group != w.group {
			t.Fatalf("row %d: want header %q, got %+v", idx, w.group, rows[idx])
		}
		idx++
		for _, name := range w.conns {
			if rows[idx].kind != rowConn || rows[idx].conn.name != name {
				t.Fatalf("row %d: want conn %q, got %+v", idx, name, rows[idx])
			}
			idx++
		}
	}
}

// Collapsing a group hides its connections but keeps the header.
func TestGroupsCollapseHidesConnections(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	// Cursor onto the Work header (last header).
	m.connList.SetCursor(groupHeaderIndex(m.connList.rows(), "Work"))
	m.connList.ToggleGroupAtCursor()

	rows := m.connList.rows()
	headers, conns := countRows(rows)
	if headers != 3 || conns != 2 { // Work's 2 conns hidden
		t.Fatalf("after collapse: headers=%d conns=%d, want 3/2", headers, conns)
	}
	// The Work header is the last row now.
	last := rows[len(rows)-1]
	if last.kind != rowGroup || last.group != "Work" {
		t.Errorf("expected Work header last, got %+v", last)
	}
	// Expanding again restores the connections.
	m.connList.ToggleGroupAtCursor()
	if _, conns := countRows(m.connList.rows()); conns != 4 {
		t.Errorf("after re-expand conns=%d, want 4", conns)
	}
}

// Collapsing the group that contains the cursor relocates the cursor to that
// group's header rather than a hidden row.
func TestGroupsCollapseRelocatesCursor(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	// Cursor onto a Work connection.
	connIdx := indexOfConnName(m.connList.rows(), "wk-a")
	m.connList.SetCursor(connIdx)
	m.connList.ToggleGroupAtCursor()

	if !m.connList.CursorOnGroupHeader() {
		t.Fatal("cursor should rest on the Work header after collapsing it")
	}
	if got := m.connList.rows()[m.connList.cursor].group; got != "Work" {
		t.Errorf("cursor on wrong header %q", got)
	}
}

// A header row is not connectable: SelectedName is empty there.
func TestGroupsHeaderNotSelectable(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.SetCursor(groupHeaderIndex(m.connList.rows(), "Work"))
	if name := m.connList.SelectedName(); name != "" {
		t.Errorf("SelectedName on header = %q, want empty", name)
	}
	m.connList.SetCursor(indexOfConnName(m.connList.rows(), "wk-a"))
	if name := m.connList.SelectedName(); name != "wk-a" {
		t.Errorf("SelectedName on conn = %q, want wk-a", name)
	}
}

// g/G land on connection rows, skipping headers.
func TestGroupsGAndGSkipHeaders(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.SetCursor(m.connList.firstConnRow())
	if m.connList.CursorOnGroupHeader() {
		t.Error("firstConnRow landed on a header")
	}
	if got := m.connList.SelectedName(); got != "solo" {
		t.Errorf("first conn = %q, want solo", got)
	}
	m.connList.SetCursor(m.connList.lastConnRow())
	if m.connList.CursorOnGroupHeader() {
		t.Error("lastConnRow landed on a header")
	}
	if got := m.connList.SelectedName(); got != "wk-b" {
		t.Errorf("last conn = %q, want wk-b", got)
	}
}

// Filtering flattens the layout: matches show with no group headers.
func TestGroupsFilterFlattensLayout(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.StartFilter()
	m.connList.FilterAddChar("wk") // matches wk-a, wk-b
	rows := m.connList.rows()
	if headers, _ := countRows(rows); headers != 0 {
		t.Errorf("filtering should show no headers, got %d", headers)
	}
	if len(rows) != 2 {
		t.Errorf("filter matches = %d rows, want 2", len(rows))
	}
}

// Committing a filter selection keeps the cursor on that connection in the
// restored grouped layout.
func TestGroupsCommitFilterRelocates(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.StartFilter()
	m.connList.FilterAddChar("pers") // matches pers-c
	m.connList.SetCursor(0)          // the single match
	m.connList.CommitFilter()

	if m.connList.IsFiltering() {
		t.Error("CommitFilter should exit filter mode")
	}
	if got := m.connList.SelectedName(); got != "pers-c" {
		t.Errorf("after commit cursor = %q, want pers-c", got)
	}
	if m.connList.CursorOnGroupHeader() {
		t.Error("cursor should be on the connection, not a header")
	}
}

// ExpandedHeight accounts for group headers (one line each).
func TestGroupsExpandedHeightIncludesHeaders(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	// 4 conns * linesPerField + 3 headers.
	want := 4*linesPerField + 3
	if got := m.connList.ExpandedHeight(); got != want {
		t.Errorf("ExpandedHeight=%d, want %d", got, want)
	}
}

// Popup height stays constant while folding groups (same guarantee as
// filtering), because it's based on the fully-expanded layout.
func TestGroupsPopupHeightConstantWhileFolding(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 60)
	_, h0 := m.connListPopupDims()
	m.connList.SetCursor(groupHeaderIndex(m.connList.rows(), "Work"))
	m.connList.ToggleGroupAtCursor()
	_, h1 := m.connListPopupDims()
	if h0 != h1 {
		t.Errorf("popup height changed on collapse: %d -> %d (want equal)", h0, h1)
	}
}

// Grouped connection boxes render indented under their header (so the
// parent-child relationship reads visually), and the header shows the
// connection count. Flat mode is not indented.
func TestGroupsChildrenIndentedUnderHeader(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.CancelFilter()
	view := stripAnsi(m.connList.View())

	// Child boxes expand exactly under the first letter of the header (past the
	// "▾ " marker prefix = 2 columns).
	if !strings.Contains(view, "\n  \u250c") { // "  ┌"
		t.Errorf("grouped child boxes should be indented under the first letter (expected '  ┌'):\n%s", view)
	}
	// The Work header carries its connection count.
	var workLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Work") {
			workLine = line
			break
		}
	}
	if workLine == "" {
		t.Fatalf("Work header not found:\n%s", view)
	}
	if !strings.Contains(workLine, "2") {
		t.Errorf("Work header should show count '2': %q", workLine)
	}

	// Flat mode (no groups) is not indented at all.
	flat := newConnListModel(t, makeConns(2), 40)
	if strings.Contains(stripAnsi(flat.connList.View()), "\n  \u250c") {
		t.Errorf("flat mode should not indent child boxes")
	}
}

// space folds/unfolds the group under the cursor, mirroring the sidebar.
func TestGroupsSpaceFoldsViaApp(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	m.connList.CancelFilter()
	m.connList.SetCursor(indexOfConnName(m.connList.rows(), "pers-c"))

	mm, _ := m.updateConnections(tea.KeyMsg{Type: tea.KeySpace})
	m = mm.(Model)
	if !m.connList.collapsed["Personal"] {
		t.Error("space on pers-c should collapse Personal")
	}
}

// enter on a group header folds it; enter on a connection connects (returns a
// non-nil connect command path). tab folds from anywhere within a group.
func TestGroupsEnterAndTabFoldViaApp(t *testing.T) {
	m := newConnListModel(t, groupedConns(), 40)
	// The popup opens in filter mode by default; folding is a normal-mode
	// operation (the grouped tree is browsed after dismissing the filter).
	m.connList.CancelFilter()
	workHdr := groupHeaderIndex(m.connList.rows(), "Work")

	// enter on the Work header folds it.
	m.connList.SetCursor(workHdr)
	mm, _ := m.updateConnections(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if !m.connList.collapsed["Work"] {
		t.Error("enter on Work header should collapse Work")
	}

	// tab on a Personal connection folds Personal.
	m.connList.SetCursor(indexOfConnName(m.connList.rows(), "pers-c"))
	mm, _ = m.updateConnections(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	if !m.connList.collapsed["Personal"] {
		t.Error("tab on pers-c should collapse Personal")
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

	// Edit form prefills the group.
	edit := NewConnectionFormEdit(config.ConnectionConfig{
		Name: "x", Driver: "sqlite", Database: "/tmp/x.db", Group: "Personal",
	})
	if got := stripAnsi(edit.fields[fieldGroup].Value()); got != "Personal" {
		t.Errorf("edit form group=%q, want Personal", got)
	}
}

// indexOfConnName returns the row index of the named connection, or -1.
func indexOfConnName(rows []connRow, name string) int {
	for i, r := range rows {
		if r.kind == rowConn && r.conn.name == name {
			return i
		}
	}
	return -1
}
