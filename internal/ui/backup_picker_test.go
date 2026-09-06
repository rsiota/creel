package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestBackupPickerShowSortedAndMarked(t *testing.T) {
	p := NewBackupPicker()
	p.Show([]db.TableSize{
		{Name: "small", Rows: 10, DiskBytes: 100},
		{Name: "big", Rows: 1000, RowsApprox: true, DiskBytes: 9_000_000},
		{Name: "mid", Rows: 100, DiskBytes: 50_000},
	}, "/usr/bin/mysqldump")

	if !p.IsVisible() {
		t.Fatal("visible")
	}
	if p.Bin() != "/usr/bin/mysqldump" {
		t.Fatalf("bin = %q", p.Bin())
	}
	if p.items[0].name != "big" || p.items[2].name != "small" {
		t.Fatalf("sort order: %v %v %v", p.items[0].name, p.items[1].name, p.items[2].name)
	}
	if p.cursor != 0 || p.scrollRow != 0 {
		t.Fatalf("cursor should start at top, got cursor=%d scroll=%d", p.cursor, p.scrollRow)
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
	}, "")
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
	p.Show([]db.TableSize{{Name: "a", DiskBytes: 1}, {Name: "b", DiskBytes: 2}}, "")
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
	p.Show([]db.TableSize{{Name: "a", DiskBytes: 1}}, "bin")
	p.Hide()
	if p.IsVisible() || p.Bin() != "" || len(p.items) != 0 {
		t.Fatal("hide should clear")
	}
}

func TestBackupPickerScrollStaysUntilViewportEdge(t *testing.T) {
	sizes := make([]db.TableSize, 20)
	for i := range sizes {
		sizes[i] = db.TableSize{Name: fmt.Sprintf("t%02d", i), DiskBytes: int64(20 - i)}
	}
	p := NewBackupPicker()
	p.Show(sizes, "")
	p.SetSize(78, 16) // maxVisible = 16-5 = 11

	if p.scrollRow != 0 || p.cursor != 0 {
		t.Fatalf("start cursor=%d scroll=%d", p.cursor, p.scrollRow)
	}
	// Move within the viewport — content must not scroll.
	for i := 0; i < 10; i++ {
		p.CursorDown()
	}
	if p.cursor != 10 || p.scrollRow != 0 {
		t.Fatalf("within viewport: cursor=%d scroll=%d", p.cursor, p.scrollRow)
	}
	// One more step past the bottom — scroll advances by one.
	p.CursorDown()
	if p.cursor != 11 || p.scrollRow != 1 {
		t.Fatalf("past bottom: cursor=%d scroll=%d", p.cursor, p.scrollRow)
	}
}

func TestBackupPickerCursorTopBottom(t *testing.T) {
	sizes := make([]db.TableSize, 20)
	for i := range sizes {
		sizes[i] = db.TableSize{Name: fmt.Sprintf("t%02d", i), DiskBytes: int64(20 - i)}
	}
	p := NewBackupPicker()
	p.Show(sizes, "")
	p.SetSize(78, 16)

	p.CursorBottom()
	if p.cursor != 19 {
		t.Fatalf("G cursor=%d", p.cursor)
	}
	if p.scrollRow != 19-p.maxVisible()+1 {
		t.Fatalf("G scroll=%d want %d", p.scrollRow, 19-p.maxVisible()+1)
	}

	p.CursorTop()
	if p.cursor != 0 || p.scrollRow != 0 {
		t.Fatalf("g cursor=%d scroll=%d", p.cursor, p.scrollRow)
	}
}

func TestBackupPickerSelectedRowUsesPrimaryBackground(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	p := NewBackupPicker()
	p.Show([]db.TableSize{{Name: "users", Rows: 1, DiskBytes: 10}}, "")
	p.SetSize(78, 16)
	selected := p.renderRow(0, 70)
	// Unselected sibling for contrast.
	p.cursor = -1
	plain := p.renderRow(0, 70)
	if selected == plain {
		t.Fatal("selected row should differ from unselected styling")
	}
	if !strings.Contains(selected, "users") {
		t.Fatalf("missing name: %q", selected)
	}
	// Primary selection paints a background (CSI 48 / 4x).
	if !strings.Contains(selected, "\x1b[") {
		t.Fatalf("expected ANSI styling on selected row: %q", selected)
	}
}

func TestBackupPickerHeaderUsesPrimary(t *testing.T) {
	defer applyPalette(defaultPalette)
	applyPalette(defaultPalette)

	p := NewBackupPicker()
	p.Show([]db.TableSize{{Name: "users", DiskBytes: 1}}, "")
	p.SetSize(78, 16)
	view := p.View()
	if !strings.Contains(view, "Table") || !strings.Contains(view, "Sch") {
		t.Fatalf("missing headers in %q", view)
	}
	if !strings.Contains(view, "\x1b[") {
		t.Fatalf("expected styled header: %q", view)
	}
}
