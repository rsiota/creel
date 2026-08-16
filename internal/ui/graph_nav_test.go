package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

// TestNavDrillInPushesStack verifies that Enter on an explorer node
// pushes a stack entry so u/back can restore the prior query.
func TestNavDrillInPushesStack(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders VALUES (10, 1)`,
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
	ordersEdge.expanded = true
	childMsg := m.loadExplorerChildren(ordersEdge)().(explorerChildrenMsg)
	m.explorer.applyChildren(ordersEdge, childMsg.children)
	orderRow := nonSynth(childMsg.children)[0]
	m.explorer.cursorToNode(orderRow)

	if cmd := m.explorerActivate(); cmd == nil {
		t.Fatal("activate returned nil")
	}
	if len(m.queryStack) != 1 {
		t.Fatalf("stack len = %d, want 1", len(m.queryStack))
	}
	if m.queryStack[0].query != "SELECT * FROM users" {
		t.Errorf("stack query = %q, want users SELECT", m.queryStack[0].query)
	}
}

// TestExplorerOpenInTabLeavesParentTab verifies t opens the node's drill
// query in a new tab without consuming the current tab's query stack.
func TestExplorerOpenInTabLeavesParentTab(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders VALUES (10, 1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{
		connection: conn,
		results:    NewResultsTable(),
		editor:     NewQueryEditor(),
		tabBar:     NewTabBar(),
		tables:     []string{"users", "orders"},
		pageSize:   50,
	}
	m.lastQuery = "SELECT * FROM users"
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}}, "")
	m.results.SetEditable("users", []string{"id"})
	parent := NewResultsTab(1, "users")
	m.resultsTabs = []*ResultsTab{parent}
	m.activeTabID = 1
	m.nextTabID = 2
	m.saveTabState()
	m.explorer.ShowDocked()

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	ordersEdge := findEdge(root, "orders")
	ordersEdge.expanded = true
	childMsg := m.loadExplorerChildren(ordersEdge)().(explorerChildrenMsg)
	m.explorer.applyChildren(ordersEdge, childMsg.children)
	orderRow := nonSynth(childMsg.children)[0]
	m.explorer.cursorToNode(orderRow)

	if cmd := m.explorerOpenInTab(); cmd == nil {
		t.Fatal("open in tab returned nil")
	}
	if len(m.resultsTabs) != 2 {
		t.Fatalf("tabs = %d, want 2", len(m.resultsTabs))
	}
	if m.resultsTabs[0].LastQuery != "SELECT * FROM users" {
		t.Errorf("parent tab query = %q", m.resultsTabs[0].LastQuery)
	}
	if !strings.Contains(m.lastQuery, "orders") {
		t.Errorf("active query = %q, want orders drill", m.lastQuery)
	}
	if len(m.queryStack) != 0 {
		t.Errorf("new tab should not inherit the parent stack, len=%d", len(m.queryStack))
	}
	if m.resultsTabs[0].QueryStack != nil && len(m.resultsTabs[0].QueryStack) != 0 {
		t.Errorf("parent stack should still be empty, got %d", len(m.resultsTabs[0].QueryStack))
	}
}

// TestExplorerGoBackUsesStack ensures u / goBackQuery restores the prior query
// and marks the explorer loading when it is open.
func TestExplorerGoBackUsesStack(t *testing.T) {
	m := &Model{results: NewResultsTable(), editor: NewQueryEditor()}
	m.lastQuery = "SELECT * FROM users"
	m.page = 0
	m.results.SetResult([]string{"id"}, [][]string{{"1"}}, "")
	m.explorer.ShowDocked()
	m.pushQueryStack()
	m.lastQuery = "SELECT * FROM orders"
	m.explorer.applyRoot(&expNode{kind: nodeRow, table: "orders", label: " · #10"}, 1)

	cmd := m.goBackQuery()
	if cmd == nil {
		t.Fatal("goBackQuery returned nil")
	}
	if m.lastQuery != "SELECT * FROM users" {
		t.Errorf("lastQuery = %q, want users", m.lastQuery)
	}
	if !m.explorer.loading {
		t.Error("explorer should be loading after back")
	}
	if len(m.queryStack) != 0 {
		t.Errorf("stack should be empty, len=%d", len(m.queryStack))
	}
}

// TestInboundZeroCountEdgesVisible: empty child relations stay in the tree so
// "insert related" has a target even when count is 0.
func TestInboundZeroCountEdgesVisible(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		// no orders for Alice
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users", "orders"}}
	m.results.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}}, "")
	m.results.SetEditable("users", []string{"id"})

	msg := m.loadExplorer()().(explorerLoadedMsg)
	if msg.root == nil {
		t.Fatalf("root nil: %v %q", msg.err, msg.emptyMsg)
	}
	orders := findEdge(msg.root, "orders")
	if orders == nil {
		t.Fatal("expected inbound orders edge even with count 0")
	}
	if orders.edge.count != "0" {
		t.Errorf("orders count = %q, want 0", orders.edge.count)
	}
}

// TestExplorerInsertRelatedPrefillsFK: A on an inbound edge navigates to the
// child table and opens insert with the FK set.
func TestExplorerInsertRelatedPrefillsFK(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, status TEXT, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
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
	m.explorer.ShowDocked()

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	ordersEdge := findEdge(root, "orders")
	if ordersEdge == nil {
		t.Fatal("missing orders edge")
	}
	m.explorer.cursorToNode(ordersEdge)

	cmd := m.explorerInsertRelated()
	if cmd == nil {
		t.Fatalf("insert related returned nil: %q", m.schemaMsg)
	}
	if m.pendingRelatedInsert["user_id"] != "1" {
		t.Errorf("pending prefills = %v, want user_id=1", m.pendingRelatedInsert)
	}
	if !strings.Contains(m.lastQuery, "orders") || !strings.Contains(m.lastQuery, "user_id") {
		t.Errorf("lastQuery = %q", m.lastQuery)
	}

	// Simulate the query completing with an empty orders page.
	res, err := conn.DB().Execute(m.lastQuery)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	msg := queryExecutedMsg{query: m.lastQuery, result: res, page: 0, pageSize: m.pageSize}
	updated, _ := m.Update(msg)
	mm := updated.(Model)

	if !mm.inspector.IsInserting() {
		t.Fatal("inspector should be in insert mode after related insert")
	}
	vals := mm.inspector.InsertValues()
	// Find user_id column index on orders results.
	found := false
	for i := 0; i < mm.results.NumCols(); i++ {
		if strings.EqualFold(mm.results.ColumnName(i), "user_id") {
			if vals[i] != "1" {
				t.Errorf("user_id prefill = %q, want 1", vals[i])
			}
			found = true
		}
	}
	if !found {
		t.Fatal("user_id column not in results")
	}
	if mm.explorer.IsVisible() {
		t.Error("explorer should yield the right slot to the inspector")
	}
	if !mm.restoreExplorerAfterInsert {
		t.Error("insert related should remember to restore the explorer")
	}

	mm.inspector.CancelInsert()
	if cmd := mm.maybeRestoreExplorerAfterInsert(); cmd == nil {
		t.Error("restore should reload the explorer")
	}
	if !mm.explorer.IsVisible() {
		t.Fatal("explorer should return after cancel")
	}
	if mm.inspector.IsVisible() {
		t.Error("inspector should yield the right slot back to the explorer")
	}
	if mm.focus != FocusExplorer {
		t.Errorf("focus = %v, want explorer", mm.focus)
	}
}

// TestExplorerInsertRelatedRejectsOutbound ensures A on an outbound edge is a
// no-op with a clear message.
func TestExplorerInsertRelatedRejectsOutbound(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders VALUES (10, 1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users", "orders"}}
	m.results.SetResult([]string{"id", "user_id"}, [][]string{{"10", "1"}}, "")
	m.results.SetEditable("orders", []string{"id"})
	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	usersEdge := findEdge(root, "users")
	if usersEdge == nil {
		t.Fatal("missing outbound users edge")
	}
	m.explorer.cursorToNode(usersEdge)
	if cmd := m.explorerInsertRelated(); cmd != nil {
		t.Fatal("expected nil cmd for outbound edge")
	}
	if !strings.Contains(m.schemaMsg, "inbound") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestExplorerHighlightLinkedFK(t *testing.T) {
	conn := newSQLiteTestConn(t)
	defer conn.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY (user_id) REFERENCES users(id))`,
		`INSERT INTO users VALUES (1,'Alice')`,
		`INSERT INTO orders VALUES (10, 1)`,
	} {
		if _, err := conn.DB().Exec(q); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}
	m := &Model{connection: conn, results: NewResultsTable(), editor: NewQueryEditor(), tables: []string{"users", "orders"}}
	m.results.SetResult([]string{"id", "user_id"}, [][]string{{"10", "1"}}, "")
	m.results.SetEditable("orders", []string{"id"})
	m.results.SetForeignKeys("orders", []db.ForeignKey{{Column: "user_id", RefTable: "users", RefColumn: "id"}})
	m.results.SetCursor(0, 1)
	m.focus = FocusResults

	root := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(root, 0)
	m.syncExplorerFKHighlight()
	n := m.explorer.selectedNode()
	if n == nil || !n.isEdge() || n.edge.targetTable != "users" {
		t.Fatalf("linked cursor = %+v, want outbound users edge", n)
	}
}
