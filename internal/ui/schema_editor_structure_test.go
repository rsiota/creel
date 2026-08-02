package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/db"
)

// showStructureEditor builds an editor with sample structure data already
// loaded, sized for rendering.
func showStructureEditor(t *testing.T) SchemaEditor {
	t.Helper()
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "id", Type: "INT", PrimaryKey: true, AutoIncrement: true, NotNull: true},
		{Name: "email", Type: "VARCHAR(255)", NotNull: true},
	})
	e.LoadStructure(structureData{
		pk: []string{"id"},
		fks: []db.ForeignKey{
			{Column: "org_id", RefTable: "orgs", RefColumn: "id"},
		},
		indexes: []db.Index{
			{Name: "idx_email", Columns: []string{"email"}, Unique: true},
			{Name: "idx_partial", Columns: []string{"id"}, Partial: "active = true"},
		},
		checks: []db.CheckConstraint{
			{Name: "chk_email", Expression: "email ~ '^[^@]+@'"},
			{Column: "age", Expression: "age >= 0"},
		},
		triggers: []db.Trigger{
			{Name: "trg_audit", Timing: "AFTER", Event: "UPDATE", Statement: "BEGIN\n  UPDATE log SET t = now();\nEND"},
		},
	})
	return e
}

func TestStructureTabsAvailableAndSwitch(t *testing.T) {
	e := showStructureEditor(t)

	// A table exposes Columns + the four metadata tabs; Definition is absent
	// for a non-view.
	tabs := e.availableTabs()
	want := []int{seTabColumns, seTabIndexes, seTabFK, seTabChecks, seTabTriggers}
	if len(tabs) != len(want) {
		t.Fatalf("availableTabs = %v, want %v", tabs, want)
	}
	for i := range want {
		if tabs[i] != want[i] {
			t.Errorf("tab %d = %d, want %d", i, tabs[i], want[i])
		}
	}

	// Start on Columns.
	if e.ActiveTab() != seTabColumns {
		t.Errorf("start tab = %d, want Columns", e.ActiveTab())
	}

	// L advances through the tabs.
	e2 := e
	e2.switchTab(1)
	if e2.ActiveTab() != seTabIndexes {
		t.Errorf("after switchTab(1): %d, want Indexes", e2.ActiveTab())
	}
	e2.switchTab(1)
	if e2.ActiveTab() != seTabFK {
		t.Errorf("after switchTab(2): %d, want FK", e2.ActiveTab())
	}
	// H goes back.
	e2.switchTab(-1)
	if e2.ActiveTab() != seTabIndexes {
		t.Errorf("after switchTab(-1): %d, want Indexes", e2.ActiveTab())
	}
}

func TestStructureDefinitionTabOnlyForViews(t *testing.T) {
	e := showStructureEditor(t)
	if e.tabAvailable(seTabDefinition) {
		t.Error("Definition tab should be hidden for a table")
	}

	e.LoadStructure(structureData{viewDef: "SELECT id FROM users"})
	if !e.tabAvailable(seTabDefinition) {
		t.Error("Definition tab should appear for a view")
	}
}

func TestStructureIndexesTabRendersGrid(t *testing.T) {
	e := showStructureEditor(t)
	e.activeTab = seTabIndexes
	e.SetSize(80, 24)
	out := e.View()

	for _, s := range []string{"idx_email", "yes", "email", "idx_partial"} {
		if !strings.Contains(out, s) {
			t.Errorf("indexes tab missing %q\n%s", s, out)
		}
	}
	// Partial predicate is surfaced.
	if !strings.Contains(out, "active = true") {
		t.Errorf("indexes tab missing partial predicate\n%s", out)
	}
	// The grid borders are present.
	if !strings.Contains(out, "│") {
		t.Error("indexes tab should render a grid (│ borders)")
	}
}

func TestStructureFKTabRendersGrid(t *testing.T) {
	e := showStructureEditor(t)
	e.activeTab = seTabFK
	out := e.View()
	if !strings.Contains(out, "org_id") || !strings.Contains(out, "orgs(id)") {
		t.Errorf("FK tab missing reference\n%s", out)
	}
}

func TestStructureChecksTabRendersGrid(t *testing.T) {
	e := showStructureEditor(t)
	e.activeTab = seTabChecks
	e.SetSize(100, 24)
	out := e.View()
	// Table-level (named) and column-level checks both render.
	for _, s := range []string{"chk_email", "email ~", "age", "age >= 0"} {
		if !strings.Contains(out, s) {
			t.Errorf("checks tab missing %q\n%s", s, out)
		}
	}
	if !strings.Contains(out, "│") {
		t.Error("checks tab should render a grid (│ borders)")
	}
}

// The Checks tab surfaces a per-section catalog error without hiding the tab,
// mirroring Indexes/Triggers.
func TestStructureChecksTabSurfacesError(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverPostgres, []db.TableColumnInfo{{Name: "id", Type: "INTEGER"}})
	e.LoadStructure(structureData{checkErr: "permission denied"})
	e.activeTab = seTabChecks
	out := e.View()
	if !strings.Contains(out, "permission denied") {
		t.Errorf("expected check error surfaced, got %q", out)
	}
}

func TestStructureTriggersTabSummaryAndExpand(t *testing.T) {
	e := showStructureEditor(t)
	e.activeTab = seTabTriggers
	out := e.View()
	if !strings.Contains(out, "trg_audit") || !strings.Contains(out, "AFTER") || !strings.Contains(out, "UPDATE") {
		t.Errorf("triggers summary missing fields\n%s", out)
	}

	// enter expands the statement; esc collapses.
	e2, _ := e.Update(pressKey("enter"))
	if !e2.triggerExpanded {
		t.Error("enter should expand the trigger statement")
	}
	expanded := e2.View()
	if !strings.Contains(expanded, "UPDATE log") {
		t.Errorf("expanded statement missing body\n%s", expanded)
	}
	e2, _ = e2.Update(pressKey("esc"))
	if e2.triggerExpanded {
		t.Error("esc should collapse the trigger statement")
	}
}

func TestStructureDefinitionTabRendersViewSQL(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("active_users", db.DriverSQLite, []db.TableColumnInfo{{Name: "id", Type: "INTEGER"}})
	e.LoadStructure(structureData{viewDef: "SELECT id, name\nFROM users\nWHERE email IS NOT NULL"})
	e.activeTab = seTabDefinition
	out := e.View()
	if !strings.Contains(out, "SELECT id, name") {
		t.Errorf("definition tab missing view SQL\n%s", out)
	}
	if !strings.Contains(out, "view") {
		t.Errorf("expected view badge in header\n%s", out)
	}
}

func TestStructureReadOnlyNavDoesNotEditColumns(t *testing.T) {
	e := showStructureEditor(t)
	e.activeTab = seTabIndexes
	// j on a read-only tab moves the read-only cursor, not the column cursor.
	prevRow := e.roCursor
	e2, _ := e.Update(pressKey("j"))
	if e2.roCursor != prevRow+1 {
		t.Errorf("roCursor = %d, want %d", e2.roCursor, prevRow+1)
	}
	if e2.cursorRow != 0 {
		t.Errorf("column cursor moved on a read-only tab: %d", e2.cursorRow)
	}
}

func TestStructureLoadingStates(t *testing.T) {
	e := NewSchemaEditor()
	e.SetSize(80, 24)
	e.Show("users", db.DriverMySQL, []db.TableColumnInfo{{Name: "id", Type: "INT"}})
	// structLoaded is false until LoadStructure arrives.
	e.activeTab = seTabIndexes
	out := e.View()
	if !strings.Contains(out, "Loading") {
		t.Errorf("expected loading state, got %q", out)
	}

	// A per-section error is surfaced without hiding the tab.
	e.LoadStructure(structureData{indexErr: "access denied"})
	out = e.View()
	if !strings.Contains(out, "access denied") {
		t.Errorf("expected index error surfaced, got %q", out)
	}
}

func TestRenderBoxTableFitsWidth(t *testing.T) {
	headers := []string{"Name", "Type"}
	rows := [][]string{
		{"id", strings.Repeat("x", 50)},
		{"name", "TEXT"},
	}
	// 40-wide cap must not overflow; the renderer shrinks the wide column.
	out := renderBoxTable(headers, rows, 40)
	first := out
	if i := strings.Index(out, "\n"); i >= 0 {
		first = out[:i]
	}
	if lipgloss.Width(first) > 40 {
		t.Errorf("grid width %d exceeds cap 40", lipgloss.Width(first))
	}
	if !strings.Contains(out, "id") {
		t.Errorf("grid missing content\n%s", out)
	}
}
