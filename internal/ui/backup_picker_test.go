package ui

import (
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestBackupPickerShowSortedAndMarked(t *testing.T) {
	p := NewBackupPicker()
	p.Show([]db.TableSize{
		{Name: "small", Rows: 10, DiskBytes: 100},
		{Name: "big", Rows: 1000, RowsApprox: true, DiskBytes: 9_000_000},
		{Name: "mid", Rows: 100, DiskBytes: 50_000},
	}, "mid", "/usr/bin/mysqldump")

	if !p.IsVisible() {
		t.Fatal("visible")
	}
	if p.Bin() != "/usr/bin/mysqldump" {
		t.Fatalf("bin = %q", p.Bin())
	}
	if p.items[0].name != "big" || p.items[2].name != "small" {
		t.Fatalf("sort order: %v %v %v", p.items[0].name, p.items[1].name, p.items[2].name)
	}
	if p.cursor != 1 { // mid
		t.Fatalf("cursor = %d", p.cursor)
	}
	if p.SchemaCount() != 3 || p.DataCount() != 3 {
		t.Fatalf("counts schema=%d data=%d", p.SchemaCount(), p.DataCount())
	}
	if !p.DumpPlan().IsFull() {
		t.Fatal("all selected should be full dump")
	}
}

func TestBackupPickerSelectivePlan(t *testing.T) {
	p := NewBackupPicker()
	p.Show([]db.TableSize{
		{Name: "users", Rows: 1, DiskBytes: 10},
		{Name: "logs", Rows: 2, DiskBytes: 20},
	}, "", "")
	p.cursor = 0 // logs (larger)
	p.ToggleData()
	plan := p.DumpPlan()
	if plan.IsFull() {
		t.Fatal("expected selective")
	}
	if len(plan.SchemaTables()) != 2 || len(plan.DataTables()) != 1 {
		t.Fatalf("schema=%v data=%v", plan.SchemaTables(), plan.DataTables())
	}
	if plan.DataTables()[0] != "users" {
		t.Fatalf("data = %v", plan.DataTables())
	}
}

func TestBackupPickerToggleIncludeAndSchemaOnly(t *testing.T) {
	p := NewBackupPicker()
	p.Show([]db.TableSize{{Name: "a", DiskBytes: 1}, {Name: "b", DiskBytes: 2}}, "", "")
	p.ToggleInclude()
	if p.SchemaCount() != 1 || p.DataCount() != 1 {
		t.Fatalf("after omit: schema=%d data=%d", p.SchemaCount(), p.DataCount())
	}
	p.SchemaOnlyAll()
	if p.SchemaCount() != 2 || p.DataCount() != 0 {
		t.Fatalf("schema-only: schema=%d data=%d", p.SchemaCount(), p.DataCount())
	}
	p.SelectNone()
	if p.DumpPlan().HasContent() {
		t.Fatal("none should have no content")
	}
	p.SelectAll()
	if !p.DumpPlan().IsFull() {
		t.Fatal("all should be full")
	}
}

func TestBackupPickerHide(t *testing.T) {
	p := NewBackupPicker()
	p.Show([]db.TableSize{{Name: "a", DiskBytes: 1}}, "", "bin")
	p.Hide()
	if p.IsVisible() || p.Bin() != "" || len(p.items) != 0 {
		t.Fatal("hide should clear")
	}
}
