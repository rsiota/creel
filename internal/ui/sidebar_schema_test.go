package ui

import (
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestCompleteSchemaOffersCachedNames(t *testing.T) {
	m := &Model{
		connection:  db.ConnectionFromConfig(db.ConnectionConfig{Driver: db.DriverPostgres, Database: "app"}),
		schemaNames: []string{"public", "analytics", "billing"},
	}
	got := completeSchema(m, nil, "")
	if len(got) != 3 || got[0] != "public" || got[2] != "billing" {
		t.Fatalf("completeSchema = %#v", got)
	}
	if completeSchema(m, []string{"public"}, "") != nil {
		t.Fatal("expected nil past first argument")
	}
}

func TestConnectionInfoShowsPostgresSchema(t *testing.T) {
	m := Model{
		connection: db.ConnectionFromConfig(db.ConnectionConfig{
			Driver:   db.DriverPostgres,
			Database: "app",
			Schema:   "analytics",
			Host:     "localhost",
		}),
	}
	out := stripAnsi(m.connectionInfo("prod"))
	if !strings.Contains(out, "app") {
		t.Fatalf("missing database in %q", out)
	}
	if !strings.Contains(out, "analytics") {
		t.Fatalf("missing schema in %q", out)
	}
}

func TestGroupedSidebarItemsActiveFirst(t *testing.T) {
	m := Model{
		connection: db.ConnectionFromConfig(db.ConnectionConfig{
			Driver: db.DriverPostgres, Database: "app", Schema: "public", Host: "localhost",
		}),
		schemaNames: []string{"analytics", "public", "billing"},
		schemaTableCache: map[string][]string{
			"public":    {"users", "orders"},
			"analytics": {"events"},
			"billing":   {"invoices"},
		},
		tables: []string{"users", "orders"},
		views:  map[string]bool{"orders": true},
	}
	if !m.useGroupedSidebar() {
		t.Fatal("expected grouped sidebar")
	}
	items := m.sidebarItems()
	// Active schema expanded by default; others collapsed → headers + public tables only.
	want := []struct {
		text     string
		isSchema bool
		isView   bool
	}{
		{"public", true, false},
		{"users", false, false},
		{"orders", false, true},
		{"analytics", true, false},
		{"billing", true, false},
	}
	if len(items) != len(want) {
		t.Fatalf("items=%d want %d: %+v", len(items), len(want), items)
	}
	for i, w := range want {
		if items[i].text != w.text || items[i].isSchema != w.isSchema || items[i].isView != w.isView {
			t.Errorf("[%d] got text=%q schema=%v view=%v want %+v", i, items[i].text, items[i].isSchema, items[i].isView, w)
		}
	}
	if items[0].text != "public" || !items[0].isSchema {
		t.Fatal("active schema should be first header")
	}
}

func TestToggleSchemaSectionRevealsTables(t *testing.T) {
	m := Model{
		connection: db.ConnectionFromConfig(db.ConnectionConfig{
			Driver: db.DriverPostgres, Database: "app", Schema: "public", Host: "localhost",
		}),
		schemaNames: []string{"public", "analytics"},
		schemaTableCache: map[string][]string{
			"public":    {"users"},
			"analytics": {"events"},
		},
		tables: []string{"users"},
	}
	m.toggleSchemaSection("analytics")
	items := m.sidebarItems()
	found := false
	for _, it := range items {
		if it.isTableRow() && it.text == "events" && it.schema == "analytics" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events under analytics missing: %+v", items)
	}
}

func TestSidebarActivateForeignTablePromptsSchema(t *testing.T) {
	m := Model{
		connection: db.ConnectionFromConfig(db.ConnectionConfig{
			Driver: db.DriverPostgres, Database: "app", Schema: "public", Host: "localhost",
		}),
		schemaNames: []string{"public", "analytics"},
		schemaTableCache: map[string][]string{
			"public":    {"users"},
			"analytics": {"events"},
		},
		tables: []string{"users"},
	}
	m.toggleSchemaSection("analytics")
	item := &sidebarItem{text: "events", schema: "analytics"}
	if cmd := m.sidebarActivateItem(item); cmd != nil {
		t.Fatal("expected no open command for foreign table")
	}
	if !strings.Contains(m.schemaMsg, ":schema analytics") {
		t.Fatalf("schemaMsg = %q", m.schemaMsg)
	}
}

func TestMySQLKeepsFlatSidebar(t *testing.T) {
	m := Model{
		connection: db.ConnectionFromConfig(db.ConnectionConfig{
			Driver: db.DriverMySQL, Database: "app", Host: "localhost",
		}),
		schemaNames: []string{"app", "other"},
		schemaTableCache: map[string][]string{
			"app":   {"users"},
			"other": {"things"},
		},
		tables: []string{"users"},
	}
	if m.useGroupedSidebar() {
		t.Fatal("MySQL must stay flat (schemas are databases)")
	}
	items := m.sidebarItems()
	if len(items) != 1 || items[0].text != "users" || items[0].isSchema {
		t.Fatalf("flat items = %+v", items)
	}
}
