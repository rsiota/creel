package ui

import (
	"fmt"
	"testing"
)

func TestColumnPickerScrollStaysUntilViewportEdge(t *testing.T) {
	cols := make([]string, 30)
	for i := range cols {
		cols[i] = fmt.Sprintf("c%02d", i)
	}
	p := NewColumnPicker()
	p.Show(cols, nil)
	p.SetSize(71, 19) // maxVisible = 19-5 = 14

	for i := 0; i < 13; i++ {
		p.CursorDown()
	}
	if p.cursor != 13 || p.scrollRow != 0 {
		t.Fatalf("within viewport: cursor=%d scroll=%d", p.cursor, p.scrollRow)
	}
	p.CursorDown()
	if p.cursor != 14 || p.scrollRow != 1 {
		t.Fatalf("past bottom: cursor=%d scroll=%d", p.cursor, p.scrollRow)
	}
}

func TestDatabasePickerScrollStaysUntilViewportEdge(t *testing.T) {
	dbs := make([]string, 40)
	for i := range dbs {
		dbs[i] = fmt.Sprintf("db%02d", i)
	}
	p := NewDatabasePicker()
	p.Show(dbs, false)
	p.SetSize(71, 19) // maxVisible = 19-3 = 16

	for i := 0; i < 15; i++ {
		p.CursorDown()
	}
	if p.cursor != 15 || p.scrollRow != 0 {
		t.Fatalf("within viewport: cursor=%d scroll=%d", p.cursor, p.scrollRow)
	}
	p.CursorDown()
	if p.cursor != 16 || p.scrollRow != 1 {
		t.Fatalf("past bottom: cursor=%d scroll=%d", p.cursor, p.scrollRow)
	}
}
