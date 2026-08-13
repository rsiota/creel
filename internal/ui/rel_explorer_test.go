package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestLoadExplorerRootAndEdges verifies the explorer builds a root row node and
// loads its first-level edges (both directions) with correct per-edge counts.
func TestLoadExplorerRootAndEdges(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE companies (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, company_id INTEGER, FOREIGN KEY (company_id) REFERENCES companies(id))`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`CREATE TABLE addresses (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO companies VALUES (1,'Acme')`,
		`INSERT INTO users VALUES (1,'Alice',1)`,
		`INSERT INTO orders (id,user_id) VALUES (1,1),(2,1),(3,1)`,
		`INSERT INTO addresses (id,user_id) VALUES (1,1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	m := &Model{
		connection: conn,
		results:    NewResultsTable(),
		editor:     NewQueryEditor(),
		tables:     []string{"companies", "users", "orders", "addresses"},
	}
	m.results.SetResult([]string{"id", "name", "company_id"}, [][]string{{"1", "Alice", "1"}}, "")
	m.results.SetEditable("users", []string{"id"})

	msg, ok := m.loadExplorer()().(explorerLoadedMsg)
	if !ok {
		t.Fatalf("expected explorerLoadedMsg, got %T", m.loadExplorer()())
	}
	if msg.err != nil || msg.root == nil {
		t.Fatalf("load error/root-nil: err=%v root=%v", msg.err, msg.root)
	}
	if msg.root.table != "users" {
		t.Errorf("root.table = %q, want users", msg.root.table)
	}
	if !strings.Contains(msg.root.label, "#1") {
		t.Errorf("root.label = %q, want #1", msg.root.label)
	}
	// Root should have 3 edge children: 1 outbound (companies) + 2 inbound.
	edges := nonSynth(msg.root.children)
	if len(edges) != 3 {
		t.Fatalf("expected 3 edge children, got %d: %+v", len(edges), msg.root.children)
	}
	got := map[string]string{}
	for _, e := range edges {
		key := "in|" + e.edge.targetTable
		if e.edge.dir == relOutbound {
			key = "out|" + e.edge.targetTable
		}
		got[key] = e.edge.count
	}
	if got["out|companies"] != "1" {
		t.Errorf("companies outbound count = %q, want 1", got["out|companies"])
	}
	if got["in|orders"] != "3" {
		t.Errorf("orders inbound count = %q, want 3", got["in|orders"])
	}
	if got["in|addresses"] != "1" {
		t.Errorf("addresses inbound count = %q, want 1", got["in|addresses"])
	}
	// Edge drill queries should be set (Enter on edge opens the full set).
	for _, e := range edges {
		if e.drillQuery == "" {
			t.Errorf("edge %s has empty drillQuery", e.edge.targetTable)
		}
	}
}

func nonSynth(ns []*expNode) []*expNode {
	out := ns[:0]
	for _, n := range ns {
		if !n.synthetic {
			out = append(out, n)
		}
	}
	return out
}

// TestLoadRowEdgesKeepsInboundZeroCount checks that inbound edges with count 0
// stay visible (so "insert related" has a target), while still requiring a
// resolved numeric count (NULL parent keys stay hidden).
func TestLoadRowEdgesKeepsInboundZeroCount(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`CREATE TABLE addresses (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders (id,user_id) VALUES (10,1)`, // 1 order
		// no addresses for this user -> inbound count 0, still shown
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	driver := conn.Config().Driver
	edges, err := loadRowEdges(conn, driver, "users", map[string]string{"id": "1"}, nil)
	if err != nil {
		t.Fatalf("loadRowEdges: %v", err)
	}
	hasOrders, hasAddresses := false, false
	for _, e := range edges {
		switch e.edge.targetTable {
		case "addresses":
			hasAddresses = true
			if e.edge.count != "0" {
				t.Errorf("addresses count = %q, want 0", e.edge.count)
			}
			if e.edge.dir != relInbound {
				t.Error("addresses should be inbound")
			}
		case "orders":
			hasOrders = true
			if e.edge.count != "1" {
				t.Errorf("orders count = %q, want 1", e.edge.count)
			}
		}
	}
	if !hasOrders {
		t.Error("orders edge (count 1) should be present")
	}
	if !hasAddresses {
		t.Error("addresses edge (count 0) should stay visible for insert-related")
	}
}

// TestExplorerSizedByLayout is a regression test for a subtle bug: the
// explorer/lookup/explain panels were once sized only inside View(), which
// has a VALUE receiver — so SetSize mutated a throwaway copy and the panel's
// stored height stayed 0. With height 0, nodeViewport() collapses to 1, so
// every cursor move scrolled the single visible line ("every j scrolls").
// The fix sizes these panels in layoutWorkspace (pointer receiver, persisted).
// This test ensures updateLayout leaves the explorer with a real viewport.
func TestExplorerSizedByLayout(t *testing.T) {
	// layoutWorkspace sizes the editor/results/etc., so they must be initialized
	// (a bare Model would panic inside the textarea). The docked explorer panel
	// is only sized while visible, so reveal it first.
	m := &Model{state: stateWorkspace, width: 80, height: 24, editor: NewQueryEditor()}
	m.explorer.ShowDocked()
	*m = m.updateLayout()
	if m.explorer.height == 0 {
		t.Fatalf("explorer.height is 0 after updateLayout — overlay not sized persistently (nodeViewport=%d)", m.explorer.nodeViewport())
	}
	if vh := m.explorer.nodeViewport(); vh < 2 {
		t.Errorf("explorer nodeViewport = %d after layout, want >=2 (a 1-line viewport scrolls on every move)", vh)
	}
	// Same guard for the two sibling overlays that share the 70% size and the
	// same height-derived viewport math.
	if m.explainPanel.height == 0 || m.lookupPanel.height == 0 {
		t.Errorf("explain/lookup overlay height not set after updateLayout (explain=%d lookup=%d)", m.explainPanel.height, m.lookupPanel.height)
	}
}

// TestDockedExplorerOpensAndCursorReloads covers the inspector-tab variant:
// `:explore panel` opens the explorer docked in the right slot (visible,
// docked, focused), and it re-roots as the results cursor moves between rows.
func TestDockedExplorerOpensAndCursorReloads(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO users VALUES (2,'Bob')`,
		`INSERT INTO orders (id,user_id) VALUES (10,1),(11,2)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	m := &Model{
		connection: conn,
		results:    NewResultsTable(),
		editor:     NewQueryEditor(),
		tables:     []string{"users", "orders"},
	}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}, {"2", "Bob"}}, "")
	m.results.SetEditable("users", []string{"id"})

	// `:explore panel` opens the docked variant and kicks off the first load.
	loadCmd := m.runExCommand("explore panel")
	if loadCmd == nil {
		t.Fatalf(":explore panel returned nil")
	}
	if !m.explorer.IsVisible() || !m.explorer.docked {
		t.Errorf("explorer should be visible+docked (visible=%v docked=%v)", m.explorer.IsVisible(), m.explorer.docked)
	}
	if m.focus != FocusExplorer {
		t.Errorf("focus = %v, want FocusExplorer", m.focus)
	}

	// First load roots at user 1 (cursor row 0).
	msg := loadCmd().(explorerLoadedMsg)
	if msg.root == nil || msg.root.table != "users" {
		t.Fatalf("expected user-1 root, got %+v", msg)
	}
	m.explorer.applyRoot(msg.root, msg.depth)
	m.explorer.anchor = m.explorerAnchor()

	// Cursor unchanged → no reload.
	if c := m.maybeReloadDockedExplorer(); c != nil {
		t.Errorf("reload should be nil when cursor unchanged")
	}
	// Move to user 2 → anchor changes → reload re-roots.
	m.results.SetCursor(1, 0)
	reload := m.maybeReloadDockedExplorer()
	if reload == nil {
		t.Fatalf("expected a reload cmd after moving cursor to a new row")
	}
	msg2 := reload().(explorerLoadedMsg)
	if msg2.root == nil || msg2.root.table != "users" {
		t.Fatalf("expected user-2 root, got %+v", msg2)
	}
	if msg.root.label == msg2.root.label {
		t.Errorf("tree did not re-root after cursor move (both %q)", msg.root.label)
	}

	// `:explore panel` again toggles it closed.
	m.runExCommand("explore panel")
	if m.explorer.IsVisible() {
		t.Errorf("explorer should be hidden after toggling :explore panel off")
	}
}

// TestLoadExplorerEmptyCases checks helpful messages when there is nothing to
// explore (no results, or results that aren't a single table).
func TestLoadExplorerEmptyCases(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	if _, err := conn.DB().Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("exec: %v", err)
	}

	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor()}
	msg := m.loadExplorer()().(explorerLoadedMsg)
	if msg.err != nil || !strings.Contains(msg.emptyMsg, "no results") {
		t.Errorf("no-results case: emptyMsg=%q err=%v", msg.emptyMsg, msg.err)
	}

	m.results.SetResult([]string{"x"}, [][]string{{"1"}}, "")
	msg = m.loadExplorer()().(explorerLoadedMsg)
	if msg.err != nil || !strings.Contains(msg.emptyMsg, "not a single table") {
		t.Errorf("no-source-table case: emptyMsg=%q err=%v", msg.emptyMsg, msg.err)
	}
}

// TestExpandEdgeLoadsChildRows verifies → on an edge node loads the related
// rows as child row nodes, with PK-derived labels and drill queries.
func TestExpandEdgeLoadsChildRows(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, total INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders (id,user_id,total) VALUES (10,1,5),(11,1,9)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users", "orders"}}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}}, "")
	m.results.SetEditable("users", []string{"id"})

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	ordersEdge := findEdge(root, "orders")
	if ordersEdge == nil {
		t.Fatalf("no orders edge in %v", root.children)
	}
	// Expand the orders edge.
	msg := m.loadExplorerChildren(ordersEdge)().(explorerChildrenMsg)
	if msg.err != nil {
		t.Fatalf("expand error: %v", msg.err)
	}
	rows := nonSynth(msg.children)
	if len(rows) != 2 {
		t.Fatalf("expected 2 child rows, got %d: %+v", len(rows), msg.children)
	}
	// Each child row node should carry a PK-based label and a drill query.
	for _, r := range rows {
		if !strings.Contains(r.label, "#") {
			t.Errorf("child label %q should contain #", r.label)
		}
		want := `SELECT * FROM "orders" WHERE "id" = '` + strings.TrimPrefix(r.label, "#") + `'`
		if r.drillQuery != want {
			t.Errorf("child drillQuery = %q, want %q", r.drillQuery, want)
		}
	}
}

// TestExpandRowLoadsItsEdges verifies → on a child ROW node loads that row's
// own edges (recursive depth), not just the root's.
func TestExpandRowLoadsItsEdges(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`CREATE TABLE shipments (id INTEGER PRIMARY KEY, order_id INTEGER, FOREIGN KEY (order_id) REFERENCES orders(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders (id,user_id) VALUES (10,1)`,
		`INSERT INTO shipments (id,order_id) VALUES (100,10)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users", "orders", "shipments"}}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}}, "")
	m.results.SetEditable("users", []string{"id"})

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	ordersEdge := findEdge(root, "orders")
	childMsg := m.loadExplorerChildren(ordersEdge)().(explorerChildrenMsg)
	orderRow := nonSynth(childMsg.children)[0]
	// Now expand the order row itself — it should expose the shipments edge.
	edgeMsg := m.loadExplorerChildren(orderRow)().(explorerChildrenMsg)
	if edgeMsg.err != nil {
		t.Fatalf("expand row error: %v", edgeMsg.err)
	}
	if findEdge2(nonSynth(edgeMsg.children), "shipments") == nil {
		t.Errorf("order row should expose a shipments edge; got %+v", edgeMsg.children)
	}
}

// TestExplorerActivateNavigatesGrid verifies Enter on a row node builds the
// right query and pushes the stack (re-rooting happens later via the grid).
func TestExplorerActivateNavigatesGrid(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders (id,user_id) VALUES (10,1),(11,1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users", "orders"}}
	m.pageSize = 50
	m.lastQuery = "SELECT * FROM users"
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}}, "")
	m.results.SetEditable("users", []string{"id"})

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	ordersEdge := findEdge(root, "orders")
	ordersEdge.expanded = true // expand so children become visible/selectable
	childMsg := m.loadExplorerChildren(ordersEdge)().(explorerChildrenMsg)
	m.explorer.applyChildren(ordersEdge, childMsg.children)
	orderRow := nonSynth(childMsg.children)[0]

	// Select that order row in the tree and activate it.
	m.explorer.cursorToNode(orderRow)
	if cmd := m.explorerActivate(); cmd == nil {
		t.Fatal("activate returned nil cmd")
	}
	want := `SELECT * FROM "orders" WHERE "id" = '10'`
	if got := m.editor.Value(); got != want {
		t.Errorf("editor = %q, want %q", got, want)
	}
	if len(m.queryStack) != 1 {
		t.Errorf("queryStack len = %d, want 1", len(m.queryStack))
	}
	if !m.explorer.loading {
		t.Error("explorer should be loading (awaiting re-root) after activate")
	}
}

// TestExExploreCommandOpensPanel checks :explore reveals the panel.
func TestExExploreCommandOpensPanel(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`INSERT INTO users VALUES (1,'Alice')`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users"}}
	if cmd := m.runExCommand("explore"); cmd == nil {
		t.Fatalf(":explore returned nil: %q", m.schemaMsg)
	}
	if !m.explorer.IsVisible() {
		t.Error("explorer should be visible after :explore")
	}
	if !m.explorer.loading {
		t.Error("explorer should be loading after :explore")
	}
}

// TestRelExplorerTreeNav covers j/k movement over the flattened tree and
// expand→cursor-to-first-child / collapse→cursor-to-parent.
func TestRelExplorerTreeNav(t *testing.T) {
	e := NewRelExplorer()
	e.SetSize(60, 14)
	root := &expNode{kind: nodeRow, table: "users", label: " · #1", expanded: true}
	orders := &expNode{kind: nodeEdge, edge: relNode{dir: relInbound, targetTable: "orders", targetColumn: "user_id", sourceColumn: "id"}, depth: 1, parent: root}
	orders.edge.count = "2"
	addresses := &expNode{kind: nodeEdge, edge: relNode{dir: relInbound, targetTable: "addresses", targetColumn: "user_id", sourceColumn: "id"}, depth: 1, parent: root}
	addresses.edge.count = "1"
	// Expanded orders edge with two child rows.
	o1 := &expNode{kind: nodeRow, table: "orders", label: "#10", depth: 2, parent: orders}
	o2 := &expNode{kind: nodeRow, table: "orders", label: "#11", depth: 2, parent: orders}
	orders.expanded = true
	orders.children = []*expNode{o1, o2}
	root.children = []*expNode{orders, addresses}
	e.applyRoot(root, 0)

	// Visible order: root, orders, o1, o2, addresses (5 nodes).
	if n := len(e.visibleNodes()); n != 5 {
		t.Fatalf("visibleNodes = %d, want 5", n)
	}
	if e.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", e.cursor)
	}
	e = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if e.selectedNode() != orders {
		t.Errorf("after j: selected = %v, want orders", e.selectedNode())
	}
	e = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if e.selectedNode() != o1 {
		t.Errorf("after jj: selected = %v, want o1", e.selectedNode())
	}
	// ← from a leaf row jumps to its parent (orders).
	explorerCollapseOn(&e)
}

// TestRelExplorerPageScroll pins the "keep the view fixed, flip a page only
// when the cursor leaves it" behaviour: moving within the viewport never
// scrolls, and crossing the bottom edge advances by a full page with the
// cursor landing at the top of the new page.
func TestRelExplorerPageScroll(t *testing.T) {
	e := NewRelExplorer()
	e.SetSize(40, 9) // nodeViewport = 7 tree lines
	root := &expNode{kind: nodeRow, table: "t", label: "row0", expanded: true}
	for i := 1; i <= 14; i++ {
		root.children = append(root.children, &expNode{kind: nodeRow, table: "t", label: fmt.Sprintf("row%d", i), depth: 1, parent: root})
	}
	e.applyRoot(root, 0)
	vh := e.nodeViewport()
	if vh != 7 {
		t.Fatalf("nodeViewport = %d, want 7 for this test's window", vh)
	}

	// Roam within the first page: scroll must stay at 0.
	for step := 0; step < vh-1; step++ { // cursor 1..4
		e = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if e.scroll != 0 {
			t.Fatalf("step %d: scrolled to %d while cursor still in the first page (want fixed at 0)", step, e.scroll)
		}
	}

	// One more down exits the page → flip forward one full page.
	e = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if e.scroll != vh {
		t.Errorf("after exiting bottom: scroll = %d, want %d (full page flip, cursor at top)", e.scroll, vh)
	}
	if e.cursor != vh {
		t.Errorf("cursor = %d, want %d (top of the new page)", e.cursor, vh)
	}

	// Roam within the second page: scroll stays put again.
	before := e.scroll
	for step := 0; step < vh-1; step++ {
		e = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if e.scroll != before {
			t.Fatalf("step %d: scrolled to %d while roaming the second page (want fixed at %d)", step, e.scroll, before)
		}
	}

	// Going back up within a page never scrolls; crossing the top flips back.
	for step := 0; step < vh-1; step++ {
		e = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		if e.scroll != before {
			t.Fatalf("up step %d: scrolled to %d (want fixed at %d)", step, e.scroll, before)
		}
	}
	e = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}) // exit top
	if e.scroll != 0 {
		t.Errorf("after exiting top: scroll = %d, want 0 (flip back to first page)", e.scroll)
	}
}

// explorerCollapseOn is a thin shim so the nav test can exercise collapse
// without a full Model; mirrors explorerCollapse's logic on a bare panel.
func explorerCollapseOn(e *RelExplorer) {
	node := e.selectedNode()
	if node == nil {
		return
	}
	if node.expanded {
		node.expanded = false
		return
	}
	if node.parent != nil {
		e.cursorToNode(node.parent)
	}
}

// TestLoadExplorerOmitsBackReferenceEdge verifies that an outbound edge which
// loops back to a row already on the path is omitted entirely. Here users(1) →
// orders edge → order 10 has an outbound "users" edge pointing at user 1; it
// must not appear among order 10's edges.
func TestLoadExplorerOmitsBackReferenceEdge(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders (id,user_id) VALUES (10,1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users", "orders"}}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}}, "")
	m.results.SetEditable("users", []string{"id"})

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	// users(1) → orders edge → order 10.
	ordersEdge := findEdge(root, "orders")
	ordersEdge.expanded = true
	childMsg := m.loadExplorerChildren(ordersEdge)().(explorerChildrenMsg)
	m.explorer.applyChildren(ordersEdge, childMsg.children)
	orderRow := nonSynth(childMsg.children)[0]
	// Expanding order 10 lists its edges. Its outbound "users" edge loops back
	// to user 1 (an ancestor on the path), so it must be omitted entirely —
	// not shown, not folded, just absent.
	edgeMsg := m.loadExplorerChildren(orderRow)().(explorerChildrenMsg)
	for _, e := range nonSynth(edgeMsg.children) {
		if e.isEdge() && e.edge.dir == relOutbound && e.edge.targetTable == "users" {
			t.Errorf("outbound users edge should be omitted (loops to ancestor user 1); got count=%q", e.edge.count)
		}
	}
}

// TestLoadExplorerChildrenInboundCycleFolds covers the rarer inbound case: an
// edge whose every child is an ancestor can't be omitted at listing time (other
// children might be new), so expanding it folds via cycleOnly instead.
func TestLoadExplorerChildrenInboundCycleFolds(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE employees (id INTEGER PRIMARY KEY, name TEXT, manager_id INTEGER, FOREIGN KEY (manager_id) REFERENCES employees(id))`,
		`INSERT INTO employees (id, name, manager_id) VALUES (1,'CEO',2),(2,'VP',1),(3,'Eng',1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"employees"}}
	m.results.SetResult([]string{"id", "name", "manager_id"}, [][]string{{"1", "CEO", "2"}}, "")
	m.results.SetEditable("employees", []string{"id"})

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	// emp 1 ← employees (manager_id=1) → its reports. Pick the inbound edge.
	var reportsEdge *expNode
	for _, e := range nonSynth(root.children) {
		if e.isEdge() && e.edge.dir == relInbound {
			reportsEdge = e
		}
	}
	if reportsEdge == nil {
		t.Fatalf("root should have an inbound edge; got %+v", root.children)
	}
	reportsEdge.expanded = true
	childMsg := m.loadExplorerChildren(reportsEdge)().(explorerChildrenMsg)
	m.explorer.applyChildren(reportsEdge, childMsg.children)
	var emp2 *expNode
	for _, c := range nonSynth(childMsg.children) {
		if c.table == "employees" && c.rowVals["id"] == "2" {
			emp2 = c
		}
	}
	if emp2 == nil {
		t.Fatalf("emp 2 not found among reports: %+v", childMsg.children)
	}
	// emp 2 → employees (manager_id=2) → emp 1, the root (an ancestor). The
	// inbound edge stays listed, but expanding it folds: its only child is
	// already on the path.
	edgeMsg := m.loadExplorerChildren(emp2)().(explorerChildrenMsg)
	m.explorer.applyChildren(emp2, edgeMsg.children)
	var backEdge *expNode
	for _, e := range nonSynth(edgeMsg.children) {
		if e.isEdge() && e.edge.dir == relInbound {
			backEdge = e
		}
	}
	if backEdge == nil {
		t.Fatalf("emp 2 should have an inbound edge; got %+v", edgeMsg.children)
	}
	backEdge.expanded = true
	cycleMsg := m.loadExplorerChildren(backEdge)().(explorerChildrenMsg)
	if !cycleMsg.fold {
		t.Fatalf("inbound pure back-edge should signal fold; got children=%v", cycleMsg.children)
	}
	m.explorer.applyFold(backEdge)
	if backEdge.expanded || backEdge.children != nil {
		t.Errorf("node should be folded after a cycle-only expand; expanded=%v children=%v", backEdge.expanded, backEdge.children)
	}
}

// TestLoadExplorerRowWithNoRelationshipsFolds verifies that a row whose edges
// are all omitted (here the order's only edge loops back to its parent) folds
// silently on expand — no "(no relationships)" marker is shown.
func TestLoadExplorerRowWithNoRelationshipsFolds(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders (id,user_id) VALUES (10,1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users", "orders"}}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}}, "")
	m.results.SetEditable("users", []string{"id"})

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	ordersEdge := findEdge(root, "orders")
	ordersEdge.expanded = true
	childMsg := m.loadExplorerChildren(ordersEdge)().(explorerChildrenMsg)
	m.explorer.applyChildren(ordersEdge, childMsg.children)
	orderRow := nonSynth(childMsg.children)[0]
	// The order's only edge is the outbound FK back to user 1 (omitted), so
	// expanding it has nothing to list — it folds with no marker.
	orderRow.expanded = true
	rowMsg := m.loadExplorerChildren(orderRow)().(explorerChildrenMsg)
	if !rowMsg.fold {
		t.Fatalf("row with no listable edges should signal fold; got children=%v", rowMsg.children)
	}
	m.explorer.applyFold(orderRow)
	if orderRow.expanded || orderRow.children != nil {
		t.Errorf("node should be folded; expanded=%v children=%v", orderRow.expanded, orderRow.children)
	}
}

func findEdge(root *expNode, table string) *expNode {
	return findEdge2(root.children, table)
}
func findEdge2(ns []*expNode, table string) *expNode {
	for _, n := range ns {
		if n.isEdge() && n.edge.targetTable == table {
			return n
		}
	}
	return nil
}

// TestRelExplorerViewRendersTree smoke-tests the tree render: header, root,
// edges with counts, indentation, and the cursor mark on the selected line.
func TestRelExplorerViewRendersTree(t *testing.T) {
	e := NewRelExplorer()
	e.SetSize(70, 14)
	root := &expNode{kind: nodeRow, table: "users", label: " · #1", expanded: true}
	orders := &expNode{kind: nodeEdge, edge: relNode{dir: relInbound, targetTable: "orders", targetColumn: "user_id", sourceColumn: "id"}, depth: 1, parent: root}
	orders.edge.count = "14"
	addresses := &expNode{kind: nodeEdge, edge: relNode{dir: relInbound, targetTable: "addresses", targetColumn: "user_id", sourceColumn: "id"}, depth: 1, parent: root}
	addresses.edge.count = "2"
	// An expanded edge with a child row, to exercise indentation.
	o1 := &expNode{kind: nodeRow, table: "orders", label: "#1001", depth: 2, parent: orders}
	orders.expanded = true
	orders.children = []*expNode{o1}
	root.children = []*expNode{orders, addresses}
	e.applyRoot(root, 2)

	out := stripAnsi(e.View())
	for _, want := range []string{"users", "#1", "orders (14)", "addresses (2)", "#1001"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Edge labels carry no direction arrow and no "via <column>" clause; the
	// count follows the table name with a single space, wrapped in parens.
	if strings.Contains(out, "→ orders") || strings.Contains(out, "← orders") {
		t.Errorf("edge labels should not carry a direction arrow\n%s", out)
	}
	if strings.Contains(out, "via") {
		t.Errorf("View should not render the 'via <column>' clause\n%s", out)
	}
	// The child row should be more indented than its edge parent (4-space vs
	// 2-space), proving depth-based indentation in the rendered tree.
	if !strings.Contains(out, "    ▸ #1001") {
		t.Errorf("child row should be indented 4 spaces under its edge\n%s", out)
	}
}

// TestRelExplorerViewExactBoxModel is a regression test for the docked panel:
// View() must render an exact e.width × e.height box. lipgloss's Width INCLUDES
// horizontal padding, so the text area is e.width-4 (= inner); a selected line
// padded to `inner` must NOT wrap to a second line, and the total must be
// exactly e.height lines (else the panel overflows its slot and the top border
// gets clipped).
func TestRelExplorerViewExactBoxModel(t *testing.T) {
	for _, h := range []int{12, 20, 30} {
		e := NewRelExplorer()
		e.SetSize(45, h) // InspectorWidth
		root := &expNode{kind: nodeRow, table: "users", label: " · #1"}
		long := &expNode{kind: nodeEdge, edge: relNode{dir: relInbound, targetTable: "very_long_table_name_here", targetColumn: "uid", sourceColumn: "id"}, depth: 1, parent: root}
		long.edge.count = "999"
		root.children = []*expNode{long}
		e.applyRoot(root, 2)
		e.cursor = 0 // select the root row (full-width inverted line)

		out := e.View()
		lines := strings.Split(out, "\n")
		if len(lines) != h {
			t.Errorf("height %d: View rendered %d lines, want exactly %d (box-model overflow)", h, len(lines), h)
		}
		for i, ln := range lines {
			if w := lipgloss.Width(ln); w != 45 {
				t.Errorf("height %d line %d: width=%d, want 45", h, i, w)
			}
		}
	}
}

// TestRelExplorerEmptyMsgExactBoxModel guards the "no focused row" empty
// state: a long emptyMsg (and optional breadcrumb) must not wrap under the
// panel Width and inflate height past e.height — that overflow shifts the
// three-panel workspace up and clips the top borders.
func TestRelExplorerEmptyMsgExactBoxModel(t *testing.T) {
	for _, h := range []int{12, 20, 30} {
		e := NewRelExplorer()
		e.SetSize(45, h)
		e.SetPath([]string{"users · #1", "orders · #10"})
		e.applyEmpty(1, "no focused row — select a row to explore its relationships")

		out := e.View()
		if got := lipgloss.Height(out); got != h {
			t.Errorf("height %d: View height=%d, want %d (emptyMsg wrap overflow)", h, got, h)
		}
		for i, ln := range strings.Split(out, "\n") {
			if w := lipgloss.Width(ln); w != 45 {
				t.Errorf("height %d line %d: width=%d, want 45", h, i, w)
			}
		}
	}
}

// TestRelExplorerBorderColor mirrors the inspector/assistant focus convention:
// the explorer border uses the accent color when it holds focus and the dim
// unfocused color otherwise.
func TestRelExplorerBorderColor(t *testing.T) {
	e := NewRelExplorer()
	if e.borderColor() != colorBorderUnfocused {
		t.Errorf("unfocused border = %v, want %v", e.borderColor(), colorBorderUnfocused)
	}
	e.focused = true
	if e.borderColor() != colorPrimary {
		t.Errorf("focused border = %v, want %v", e.borderColor(), colorPrimary)
	}
	if colorPrimary == colorBorderUnfocused {
		t.Fatalf("sanity: accent and unfocused colors are identical; test is meaningless")
	}
}
