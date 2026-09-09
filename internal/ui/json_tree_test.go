package ui

import (
	"strings"
	"testing"
)

func TestIsJSONValue(t *testing.T) {
	if !isJSONValue(`{"a":1}`) {
		t.Fatal("object should be JSON")
	}
	if !isJSONValue(`[1,2]`) {
		t.Fatal("array should be JSON")
	}
	if isJSONValue(`"hello"`) {
		t.Fatal("string scalar should not fold")
	}
	if isJSONValue(`42`) {
		t.Fatal("number scalar should not fold")
	}
	if isJSONValue(`{bad`) {
		t.Fatal("invalid should not fold")
	}
}

func TestJSONTreeValueContentCollapsed(t *testing.T) {
	got, ok := jsonTreeValueContent(`{"a":1,"b":2}`, 40, false, nil, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	plain := stripANSI(got)
	if !strings.Contains(plain, jsonTreeCollapsedGlyph) {
		t.Errorf("missing collapsed glyph: %q", plain)
	}
	if !strings.Contains(plain, "2 keys") {
		t.Errorf("want key count, got %q", plain)
	}
	if strings.Contains(plain, "\n") {
		t.Errorf("collapsed should be one line, got %q", plain)
	}
}

func TestBuildJSONTreeRowsNested(t *testing.T) {
	v, ok := parseJSONContainer(`{"a":{"x":1,"y":2},"b":3}`)
	if !ok {
		t.Fatal("parse")
	}
	open := map[string]bool{jsonTreeRootPath: true}
	rows := buildJSONTreeRows(v, open)
	// root + collapsed "a" + leaf "b"
	if len(rows) != 3 {
		t.Fatalf("rows=%d, want 3 (root, a collapsed, b): %+v", len(rows), rows)
	}
	if rows[1].label != `"a"` || !rows[1].foldable || rows[1].open {
		t.Fatalf("row1 should be collapsed a: %+v", rows[1])
	}
	if rows[2].label != `"b"` || rows[2].foldable {
		t.Fatalf("row2 should be leaf b: %+v", rows[2])
	}

	open[rows[1].path] = true
	rows = buildJSONTreeRows(v, open)
	if len(rows) != 5 { // root, a, x, y, b
		t.Fatalf("expanded a: rows=%d, want 5: %+v", len(rows), rows)
	}
	if rows[2].label != `"x"` || rows[3].label != `"y"` {
		t.Fatalf("nested leaves: %+v %+v", rows[2], rows[3])
	}
}

func TestJSONTreeValueContentExpandedShowsChildren(t *testing.T) {
	raw := `{"name":"ada","n":1}`
	open := map[string]bool{jsonTreeRootPath: true}
	got, ok := jsonTreeValueContent(raw, 40, true, open, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	plain := stripANSI(got)
	if !strings.Contains(plain, jsonTreeExpandedGlyph) {
		t.Errorf("missing expanded glyph: %q", plain)
	}
	if !strings.Contains(plain, `"name"`) {
		t.Errorf("expanded tree should show keys: %q", plain)
	}
	if lines := strings.Count(plain, "\n") + 1; lines < 3 {
		t.Errorf("expanded should be multi-line, got %d lines: %q", lines, plain)
	}
}

func TestJSONTreeValueContentArraySummary(t *testing.T) {
	got, ok := jsonTreeValueContent(`[1,2,3]`, 20, false, nil, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	plain := stripANSI(got)
	if !strings.Contains(plain, "[3]") {
		t.Errorf("want array length, got %q", plain)
	}
}

func newJSONInspector(t *testing.T) Inspector {
	t.Helper()
	var i Inspector
	i.visible = true
	i.SetSize(40, 30)
	return i
}

func newJSONResults() ResultsTable {
	r := ResultsTable{}
	r.SetResult(
		[]string{"id", "meta", "name"},
		[][]string{{"1", `{"a":{"x":1},"b":2,"c":[9,8]}`, "ada"}},
		"1 row",
	)
	return r
}

func TestInspectorJSONFoldToggle(t *testing.T) {
	i := newJSONInspector(t)
	r := newJSONResults()
	i.cursorField = 1 // meta

	view := stripANSI(i.View(r))
	if !strings.Contains(view, jsonTreeCollapsedGlyph) {
		t.Fatalf("focused JSON should start collapsed: %q", view)
	}
	if strings.Contains(view, `"a"`) {
		t.Fatalf("collapsed view should not dump keys: %q", view)
	}

	if !i.ToggleJSONFold(r) {
		t.Fatal("ToggleJSONFold should succeed on JSON field")
	}
	view = stripANSI(i.View(r))
	if !strings.Contains(view, jsonTreeExpandedGlyph) {
		t.Fatalf("expanded missing glyph: %q", view)
	}
	if !strings.Contains(view, `"a"`) {
		t.Fatalf("expanded should show keys: %q", view)
	}
	if strings.Contains(view, `"x"`) {
		t.Fatalf("nested object should start collapsed: %q", view)
	}

	i.CursorDown(r) // leave JSON field via field nav (tree not capturing)
	if i.jsonExpanded {
		t.Fatal("leaving field should collapse fold state")
	}
}

func TestInspectorJSONNestedNav(t *testing.T) {
	i := newJSONInspector(t)
	r := newJSONResults()
	i.cursorField = 1
	i.ToggleJSONFold(r) // open field; cursor on root

	// j to first child "a"
	if !i.JSONTreeDown(r) {
		t.Fatal("expected tree down")
	}
	if i.jsonTreeCursor != 1 {
		t.Fatalf("cursor=%d, want 1", i.jsonTreeCursor)
	}
	// enter expands nested "a"
	i.ToggleJSONFold(r)
	rows := i.jsonTreeRows(r)
	if !rows[1].open {
		t.Fatal("expected a to be open")
	}
	view := stripANSI(i.View(r))
	if !strings.Contains(view, `"x"`) {
		t.Fatalf("expanded nested should show x: %q", view)
	}

	// o on root collapses whole field
	i.JSONTreeTop(r)
	i.ToggleJSONFold(r)
	if i.jsonExpanded {
		t.Fatal("collapsing root should close field fold")
	}
}

func TestInspectorJSONTreeEdgeSpillsToField(t *testing.T) {
	i := newJSONInspector(t)
	r := newJSONResults()
	i.cursorField = 1
	i.ToggleJSONFold(r)

	if i.JSONTreeUp(r) {
		t.Fatal("up at root should spill")
	}
	if i.jsonExpanded {
		t.Fatal("spill should collapse tree")
	}
	i.cursorField = 1
	i.ToggleJSONFold(r)
	// move to last row then spill down
	i.JSONTreeBottom(r)
	if i.JSONTreeDown(r) {
		t.Fatal("down at end should spill")
	}
	if i.jsonExpanded {
		t.Fatal("spill should collapse tree")
	}
}

func TestInspectorJSONFoldIgnoresPlainField(t *testing.T) {
	i := newJSONInspector(t)
	r := newJSONResults()
	i.cursorField = 2 // name
	if i.ToggleJSONFold(r) {
		t.Fatal("plain text field should not toggle")
	}
}

func TestInspectorJSONClickAccountsForExpandedHeight(t *testing.T) {
	i := newJSONInspector(t)
	r := newJSONResults()
	i.cursorField = 1
	i.ToggleJSONFold(r)
	i.ensureFieldVisible(r)

	view := stripANSI(i.View(r))
	y := -1
	for idx, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "name") && !strings.Contains(line, `"name"`) {
			y = idx
			break
		}
	}
	if y < 0 {
		t.Fatalf("name label not found in view:\n%s", view)
	}
	col := i.ClickField(y, r)
	if col != 2 {
		t.Fatalf("ClickField col=%d, want 2 (name); y=%d view=\n%s", col, y, view)
	}
	if i.cursorField != 2 {
		t.Fatalf("cursorField=%d, want 2", i.cursorField)
	}
}

func TestInspectorJSONEscCollapses(t *testing.T) {
	i := newJSONInspector(t)
	r := newJSONResults()
	i.cursorField = 1
	i.ToggleJSONFold(r)
	i.CollapseJSONTree()
	if i.JSONTreeActive() {
		t.Fatal("esc should collapse")
	}
}

func TestInspectorJSONHLExpandCollapse(t *testing.T) {
	i := newJSONInspector(t)
	r := newJSONResults()
	i.cursorField = 1

	// l on collapsed field opens the tree
	if !i.JSONTreeExpand(r) {
		t.Fatal("l should open JSON field")
	}
	if !i.JSONTreeActive() {
		t.Fatal("expected tree active")
	}

	i.JSONTreeDown(r) // onto collapsed "a"
	if !i.JSONTreeExpand(r) {
		t.Fatal("l should expand nested")
	}
	rows := i.jsonTreeRows(r)
	if !rows[1].open {
		t.Fatal("a should be open after l")
	}
	// l again drills into first child
	prev := i.jsonTreeCursor
	i.JSONTreeExpand(r)
	if i.jsonTreeCursor != prev+1 {
		t.Fatalf("l on open node should move to child: cursor=%d want %d", i.jsonTreeCursor, prev+1)
	}

	// h on leaf jumps to parent
	i.JSONTreeCollapse(r)
	if i.jsonTreeCursor != 1 {
		t.Fatalf("h on child should land on parent a: cursor=%d", i.jsonTreeCursor)
	}
	// h collapses a
	i.JSONTreeCollapse(r)
	rows = i.jsonTreeRows(r)
	if rows[1].open {
		t.Fatal("h should collapse a")
	}
	// h on collapsed a jumps to root; h on open root closes field
	i.JSONTreeCollapse(r) // to root
	if i.jsonTreeCursor != 0 {
		t.Fatalf("cursor=%d, want root", i.jsonTreeCursor)
	}
	i.JSONTreeCollapse(r) // collapse root → close field
	if i.JSONTreeActive() {
		t.Fatal("h on root should close field fold")
	}
}
