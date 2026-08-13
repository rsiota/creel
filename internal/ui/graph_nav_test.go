package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestNavBreadcrumbTracksDrillIn verifies that Enter on an explorer node
// pushes a labeled stack entry and the breadcrumb lists parent › child.
func TestNavBreadcrumbTracksDrillIn(t *testing.T) {
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
	if !strings.Contains(m.queryStack[0].label, "users") {
		t.Errorf("stack label = %q, want users crumb", m.queryStack[0].label)
	}

	// Simulate re-root on the order row (what queryExecutedMsg → loadExplorer does).
	m.results.SetResult([]string{"id", "user_id"}, [][]string{{"10", "1"}}, "")
	m.results.SetEditable("orders", []string{"id"})
	m.lastQuery = orderRow.drillQuery
	newRoot := m.loadExplorer()().(explorerLoadedMsg).root
	m.explorer.applyRoot(newRoot, 1)

	crumbs := m.navBreadcrumb()
	joined := strings.Join(crumbs, " › ")
	if !strings.Contains(joined, "users") || !strings.Contains(joined, "orders") {
		t.Errorf("breadcrumb = %q, want users › orders", joined)
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

// TestRelExplorerBreadcrumbInView reserves a path line above the tree.
func TestRelExplorerBreadcrumbInView(t *testing.T) {
	e := NewRelExplorer()
	e.SetSize(45, 12)
	e.SetPath([]string{"users · #1", "orders · #10"})
	root := &expNode{kind: nodeRow, table: "orders", label: " · #10"}
	e.applyRoot(root, 1)
	out := e.View()
	if !strings.Contains(out, "›") && !strings.Contains(out, "users") {
		// lipgloss may strip; check bodyLines directly.
		lines := e.bodyLines()
		if len(lines) == 0 || !strings.Contains(lines[0], "users") {
			t.Fatalf("breadcrumb missing from bodyLines: %v\nview:\n%s", lines, out)
		}
	}
	if lipgloss.Height(out) != 12 {
		t.Errorf("View height = %d, want 12", lipgloss.Height(out))
	}
}
