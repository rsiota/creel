package ui

import (
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/db"
)

// erdFixture is a tiny schema: orders.user_id → users.id.
func erdFixture() (tables []string, schemas map[string][]db.Column, pks map[string][]string, fks map[string][]db.ForeignKey) {
	tables = []string{"users", "orders"}
	schemas = map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}, {Name: "total", Type: "real"}},
	}
	pks = map[string][]string{"users": {"id"}, "orders": {"id"}}
	fks = map[string][]db.ForeignKey{"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}}}
	return
}

func TestBuildMermaidERD(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	out := strings.Join(buildMermaidERD(tables, schemas, pks, fks), "\n")

	if !strings.HasPrefix(out, "erDiagram") {
		t.Errorf("missing erDiagram header:\n%s", out)
	}
	for _, want := range []string{
		"orders {",       // entity block
		"int id PK",      // PK marker
		"int user_id FK", // FK marker
		"users {",
		"users ||--o{ orders : user_id", // relationship
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A column that is both PK and FK gets the combined PK,FK marker.
func TestBuildMermaidERDPKAndFK(t *testing.T) {
	schemas := map[string][]db.Column{
		"memberships": {{Name: "user_id", Type: "int"}},
	}
	fks := map[string][]db.ForeignKey{
		"memberships": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	pks := map[string][]string{"memberships": {"user_id"}}
	out := strings.Join(buildMermaidERD([]string{"memberships"}, schemas, pks, fks), "\n")
	if !strings.Contains(out, "int user_id PK,FK") {
		t.Errorf("expected PK,FK marker:\n%s", out)
	}
}

// Fks whose parent is outside the drawn entity set are omitted, so a focused
// neighbourhood stays self-contained.
func TestBuildMermaidERDOmitsCrossSetFKs(t *testing.T) {
	_, schemas, pks, fks := erdFixture()
	// Draw only "orders"; its user_id → users FK must be dropped (users absent).
	out := strings.Join(buildMermaidERD([]string{"orders"}, schemas, pks, fks), "\n")
	if strings.Contains(out, "||--o{") {
		t.Errorf("cross-set FK should be omitted:\n%s", out)
	}
}

func TestBuildMermaidERDNoTables(t *testing.T) {
	out := buildMermaidERD(nil, nil, nil, nil)
	if len(out) != 1 || !strings.Contains(out[0], "no tables") {
		t.Errorf("expected a no-tables message, got %v", out)
	}
}

func TestERDFocusSet(t *testing.T) {
	tables, _, _, fks := erdFixture()
	// Focusing "users" pulls in orders (inbound FK) — and users itself.
	got := erdFocusSet("users", tables, fks)
	want := map[string]bool{"users": true, "orders": true}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected focus member %q", name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("focus set = %v, want %v", got, want)
	}
}

func TestERDType(t *testing.T) {
	cases := map[string]string{
		"int":                         "int",
		"varchar(255)":                "varchar",
		"timestamp without time zone": "timestamp",
		"":                            "text",
		"weird/type!":                 "weird_type_",
	}
	for in, want := range cases {
		if got := erdType(in); got != want {
			t.Errorf("erdType(%q) = %q, want %q", in, got, want)
		}
	}
}

// hasRune reports whether the canvas contains r anywhere.
func hasRune(c *gcanvas, r rune) bool {
	if c == nil {
		return false
	}
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if g, _, _ := cellGlyph(c.cells[y][x]); g == r {
				return true
			}
		}
	}
	return false
}

// containsText reports whether the canvas contains the given (unstyled) text on
// a single row.
func containsText(c *gcanvas, s string) bool {
	if c == nil {
		return false
	}
	for y := 0; y < c.h; y++ {
		if strings.Contains(c.emitRow(y, 0, c.w), s) {
			return true
		}
	}
	return false
}

func TestRenderGraphERD(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	c := renderGraphERD(tables, schemas, pks, fks)
	if c == nil {
		t.Fatal("expected a canvas")
	}
	if c.w < 10 || c.h < 5 {
		t.Errorf("canvas too small: %dx%d", c.w, c.h)
	}
	// Card borders, PK/FK markers, and an arrow glyph must all be present.
	for _, want := range []rune{'╭', '╰', '│', '─', '◆', '◇', '◂'} {
		if !hasRune(c, want) {
			t.Errorf("canvas missing glyph %q", string(want))
		}
	}
	for _, want := range []string{"users", "orders", "user_id"} {
		if !containsText(c, want) {
			t.Errorf("canvas missing text %q", want)
		}
	}
}

func TestRenderGraphERDNoTables(t *testing.T) {
	if c := renderGraphERD(nil, nil, nil, nil); c != nil {
		t.Errorf("expected nil canvas for no tables, got %dx%d", c.w, c.h)
	}
}

// TestGcanvasPutTextRuneAligned is a regression guard: putText must advance by
// rune count, not byte offset, so a multibyte marker (◆ is 3 UTF-8 bytes)
// doesn't shift the following characters and break column alignment.
func TestGcanvasPutTextRuneAligned(t *testing.T) {
	c := newGcanvas(5, 1)
	c.putText(0, 0, "◆x", "", false)
	g0, _, _ := cellGlyph(c.cells[0][0])
	g1, _, _ := cellGlyph(c.cells[0][1])
	if g0 != '◆' || g1 != 'x' {
		t.Errorf("putText mispositioned multibyte rune: got %q,%q want ◆,x", g0, g1)
	}
}

// TestERDCardColumnAlignment asserts that a PK column line (◆ name) and a plain
// column line (no marker) start their name at the same canvas x, so the cards'
// column text lines up.
func TestERDCardColumnAlignment(t *testing.T) {
	schemas := map[string][]db.Column{
		"t": {{Name: "id", Type: "int"}, {Name: "note", Type: "text"}},
	}
	pks := map[string][]string{"t": {"id"}}
	c := renderGraphERD([]string{"t"}, schemas, pks, nil)
	// Find the x of the first letter of each column name on its row.
	nameX := func(name string) int {
		for y := 0; y < c.h; y++ {
			for x := 0; x+len(name) <= c.w; x++ {
				match := true
				for i := 0; i < len(name); i++ {
					g, _, _ := cellGlyph(c.cells[y][x+i])
					if g != []rune(name)[i] {
						match = false
						break
					}
				}
				if match {
					return x
				}
			}
		}
		return -1
	}
	if x1, x2 := nameX("id"), nameX("note"); x1 != x2 {
		t.Errorf("column names misaligned: id@%d note@%d", x1, x2)
	}
}

// TestERDCardTypeRightAligned asserts column types are flush to the card's
// inner right edge (one cell left of the right border) while names start at a
// fixed left offset — i.e. names left-aligned, types right-aligned.
func TestERDCardTypeRightAligned(t *testing.T) {
	cols := []db.Column{{Name: "id", Type: "int"}, {Name: "user_id", Type: "integer"}, {Name: "note", Type: "text"}}
	schemas := map[string][]db.Column{"t": cols}
	pks := map[string][]string{"t": {"id"}}
	c := renderGraphERD([]string{"t"}, schemas, pks, nil)

	// The card's right border │ on the separator row (row 2: border/title/sep).
	borderX := -1
	for x := c.w - 1; x >= 0; x-- {
		if g, _, _ := cellGlyph(c.cells[2][x]); g == '│' {
			borderX = x
			break
		}
	}
	if borderX < 0 {
		t.Fatal("can't locate card right border")
	}
	innerRight := borderX - 1 - erdCardRightPad
	for i, col := range cols {
		y := 3 + i // top border + title + separator, then columns
		typ := []rune(strings.ToUpper(erdType(col.Type)))
		if g, _, _ := cellGlyph(c.cells[y][innerRight]); g != typ[len(typ)-1] {
			t.Errorf("type %q not at inner edge x=%d (got %q)", col.Type, innerRight, g)
		}
		// the cell between the type and the border must be a space
		if g, _, _ := cellGlyph(c.cells[y][borderX-1]); g != ' ' {
			t.Errorf("expected space before right border x=%d (got %q)", borderX-1, g)
		}
		if g, _, _ := cellGlyph(c.cells[y][3]); g != []rune(col.Name)[0] {
			t.Errorf("name %q not at left offset x=3 (got %q)", col.Name, g)
		}
	}
}
