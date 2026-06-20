package ui

import (
	"strings"
	"testing"

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
