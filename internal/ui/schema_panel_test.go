package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

func TestFormatColumnFlags(t *testing.T) {
	got := formatColumnFlags(db.TableColumnInfo{
		PrimaryKey:    true,
		AutoIncrement: true,
		NotNull:       true,
	})
	if !strings.Contains(got, "PK") || !strings.Contains(got, "AI") {
		t.Fatalf("flags = %q", got)
	}
}

func TestFormatDefaultDisplay(t *testing.T) {
	if got := formatDefaultDisplay(db.TableColumnInfo{}); got != "—" {
		t.Fatalf("no default = %q", got)
	}
	if got := formatDefaultDisplay(db.TableColumnInfo{HasDefault: true, DefaultValue: "0"}); got != "0" {
		t.Fatalf("default = %q", got)
	}
}

func TestSchemaPanelFilter(t *testing.T) {
	p := NewSchemaPanel()
	p.Show("users", db.DriverSQLite, []db.TableColumnInfo{
		{Name: "id", Type: "INTEGER"},
		{Name: "email", Type: "TEXT"},
	})
	p.filter = "em"
	cols := p.filteredColumns()
	if len(cols) != 1 || cols[0].Name != "email" {
		t.Fatalf("filtered = %+v", cols)
	}
}

func TestSchemaPanelActionsMode(t *testing.T) {
	p := NewSchemaPanel()
	p.Show("users", db.DriverMySQL, []db.TableColumnInfo{{Name: "id", Type: "int"}})
	p.OpenActions()
	actions := p.currentActions()
	if len(actions) == 0 {
		t.Fatal("expected mysql column actions")
	}
	action, ok := p.SelectedAction()
	if !ok || action != actions[0] {
		t.Fatalf("selected = %v, ok=%v", action, ok)
	}
}

func TestSchemaPanelListRows(t *testing.T) {
	if got := schemaPanelListRows(24); got < 12 {
		t.Fatalf("24-line terminal should fit at least 12 column rows, got %d", got)
	}
	if got := schemaPanelListRows(15); got < 8 {
		t.Fatalf("small terminal should still fit 8 rows, got %d", got)
	}
}

func TestSchemaPanelScrollStartsAfterViewport(t *testing.T) {
	p := NewSchemaPanel()
	p.SetSize(76, 24)

	var cols []db.TableColumnInfo
	for i := 0; i < 30; i++ {
		cols = append(cols, db.TableColumnInfo{Name: fmt.Sprintf("col_%d", i), Type: "TEXT"})
	}
	p.Show("users", db.DriverSQLite, cols)

	viewport := p.listHeight()
	for i := 0; i < viewport-1; i++ {
		p.CursorDown()
	}
	if p.scrollRow != 0 {
		t.Fatalf("scrollRow = %d after %d moves, want 0 before viewport fills", p.scrollRow, viewport-1)
	}
	p.CursorDown()
	if p.scrollRow == 0 {
		t.Fatalf("expected scroll after filling viewport of %d rows", viewport)
	}
}

func TestSchemaPanelActionsView(t *testing.T) {
	p := NewSchemaPanel()
	p.SetSize(76, 24)
	p.Show("users", db.DriverMySQL, []db.TableColumnInfo{
		{Name: "email", Type: "varchar(255)", NotNull: true},
	})
	p.OpenActions()

	view := p.View()
	if strings.Contains(view, "Column") && strings.Contains(view, "Flags") {
		t.Fatal("actions view should not show schema column headers")
	}
	if strings.Contains(view, "/ filter") || strings.Contains(view, "add column") {
		t.Fatal("actions view should not show schema browse keybindings")
	}
	if !strings.Contains(view, "Actions · users.email") {
		t.Fatal("actions view should show actions title with table.column")
	}
	if !strings.Contains(view, "enter select") || !strings.Contains(view, "esc back") {
		t.Fatal("actions view should show action keybindings")
	}
}

func TestSchemaPanelFixedHeightWhileScrolling(t *testing.T) {
	p := NewSchemaPanel()
	p.SetSize(76, 30)

	var cols []db.TableColumnInfo
	for i := 0; i < 25; i++ {
		cols = append(cols, db.TableColumnInfo{Name: fmt.Sprintf("col_%02d", i), Type: "TEXT"})
	}
	p.Show("users", db.DriverSQLite, cols)

	h0 := lipgloss.Height(p.View())
	p.CursorDown()
	p.CursorDown()
	p.CursorDown()
	h1 := lipgloss.Height(p.View())
	if h0 != h1 {
		t.Fatalf("height changed while scrolling: %d -> %d", h0, h1)
	}

	// Near the bottom fewer raw rows used to render; height must stay stable.
	for i := 0; i < 30; i++ {
		p.CursorDown()
	}
	h2 := lipgloss.Height(p.View())
	if h0 != h2 {
		t.Fatalf("height changed at bottom: %d -> %d", h0, h2)
	}
}
