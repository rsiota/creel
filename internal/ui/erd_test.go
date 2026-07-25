package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	c, _ := renderGraphERD(tables, schemas, pks, fks)
	if c == nil {
		t.Fatal("expected a canvas")
	}
	if c.w < 10 || c.h < 5 {
		t.Errorf("canvas too small: %dx%d", c.w, c.h)
	}
	// Card borders, PK/FK markers, and an arrow glyph must all be present.
	for _, want := range []rune{'╭', '╰', '│', '─', '◆', '◇', arrowheadL()} {
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

// TestERDArrowElbow asserts the arrow's vertical run bends into its arrowhead
// as a corner (┐/┘), not a straight stub (│) left dangling beside it. In the
// fixture the parent (users) sits left of the child (orders), so the arrowhead
// is ◀ and its vertical column is the cell immediately to its right.
func TestERDArrowElbow(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	c, _ := renderGraphERD(tables, schemas, pks, fks)
	if c == nil {
		t.Fatal("expected a canvas")
	}
	head := arrowheadL()
	checked := 0
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			g, _, _ := cellGlyph(c.cells[y][x])
			if g != head {
				continue
			}
			nx := x + 1 // vertical runs one cell right of a left-pointing head
			if !c.inBounds(nx, y) {
				t.Fatalf("arrowhead at (%d,%d) has no cell to its right", x, y)
			}
			ng, _, _ := cellGlyph(c.cells[y][nx])
			switch ng {
			case '┐', '┘':
				// elbow bending into the arrowhead — the wanted shape
			case '─':
				// endpoints aligned: a pure horizontal feed, no vertical to bend
			default:
				t.Errorf("cell beside arrowhead is %q, want an elbow (┐/┘) or horizontal (─)", string(ng))
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no ◀ arrowhead found on the canvas")
	}
}

func TestRenderGraphERDNoTables(t *testing.T) {
	if c, _ := renderGraphERD(nil, nil, nil, nil); c != nil {
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
	c, _ := renderGraphERD([]string{"t"}, schemas, pks, nil)
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
	c, _ := renderGraphERD([]string{"t"}, schemas, pks, nil)

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

// TestERDCardPKBold asserts the primary-key column's name renders bold while
// a non-key column does not.
func TestERDCardPKBold(t *testing.T) {
	cols := []db.Column{{Name: "id", Type: "int"}, {Name: "note", Type: "text"}}
	schemas := map[string][]db.Column{"t": cols}
	pks := map[string][]string{"t": {"id"}}
	c, _ := renderGraphERD([]string{"t"}, schemas, pks, nil)
	nameBoldAt := func(y int, name string) bool {
		nr := []rune(name)[0]
		for x := 0; x < c.w; x++ {
			if g, _, bold := cellGlyph(c.cells[y][x]); g == nr {
				return bold
			}
		}
		return false
	}
	if !nameBoldAt(3, "id") {
		t.Error("PK column 'id' should be bold")
	}
	if nameBoldAt(4, "note") {
		t.Error("non-key column 'note' should not be bold")
	}
}

// cardRects finds every table card on the canvas by locating its ╭ top-left
// corner and measuring to its ╮ / ╰ edges, returning (x0,y0,x1,y1) inclusive.
func cardRects(c *gcanvas) [][4]int {
	var rects [][4]int
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			g, _, _ := cellGlyph(c.cells[y][x])
			if g != '╭' {
				continue
			}
			// width: scan right on this row for the ╮ corner.
			x1 := x
			for xx := x + 1; xx < c.w; xx++ {
				if gg, _, _ := cellGlyph(c.cells[y][xx]); gg == '╮' {
					x1 = xx
					break
				}
			}
			// height: scan down this column for the ╰ corner.
			y1 := y
			for yy := y + 1; yy < c.h; yy++ {
				if gg, _, _ := cellGlyph(c.cells[yy][x]); gg == '╰' {
					y1 = yy
					break
				}
			}
			rects = append(rects, [4]int{x, y, x1, y1})
		}
	}
	return rects
}

// isLineCell reports whether a cell holds part of an arrow (a connection
// segment or an arrowhead), as opposed to card text/border or blank space.
func isLineCell(c gcell) bool {
	if c.con != 0 {
		return true
	}
	switch c.ch {
	case arrowheadL(), arrowheadR(), '▾', '▲', '◄', '►':
		return true
	}
	return false
}

// TestERDNoLineCrossesCard builds a schema with a non-adjacent FK whose
// over-the-top riser would, with the old routing, rise through a table card
// stacked above the child in the same column. It asserts no arrow line or
// arrowhead ever lands inside a card's interior.
//
//	A(id)  B(id,a_id→A)  X(id,b_id→B)  Z(id,b_id→B,a_id→A)
//
// Ranks: A=0, B=1, X=2, Z=2. Column 2 holds X above Z; Z→A is non-adjacent
// (rank 2→0) and routes over the top margin.
func TestERDNoLineCrossesCard(t *testing.T) {
	schemas := map[string][]db.Column{
		"A": {{Name: "id", Type: "int"}},
		"B": {{Name: "id", Type: "int"}, {Name: "a_id", Type: "int"}},
		"X": {{Name: "id", Type: "int"}, {Name: "b_id", Type: "int"}},
		"Z": {{Name: "id", Type: "int"}, {Name: "b_id", Type: "int"}, {Name: "a_id", Type: "int"}},
	}
	pks := map[string][]string{"A": {"id"}, "B": {"id"}, "X": {"id"}, "Z": {"id"}}
	fks := map[string][]db.ForeignKey{
		"B": {{Column: "a_id", RefTable: "A", RefColumn: "id"}},
		"X": {{Column: "b_id", RefTable: "B", RefColumn: "id"}},
		"Z": {{Column: "b_id", RefTable: "B", RefColumn: "id"}, {Column: "a_id", RefTable: "A", RefColumn: "id"}},
	}
	c, _ := renderGraphERD([]string{"A", "B", "X", "Z"}, schemas, pks, fks)
	if c == nil {
		t.Fatal("expected a canvas")
	}
	rects := cardRects(c)
	inside := func(x, y int) bool {
		for _, r := range rects {
			if x > r[0] && x < r[2] && y > r[1] && y < r[3] {
				return true // strictly inside (excludes the border itself)
			}
		}
		return false
	}
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if !isLineCell(c.cells[y][x]) {
				continue
			}
			if inside(x, y) {
				t.Errorf("arrow line at (%d,%d) passes through a card interior", x, y)
			}
		}
	}
}

// --- mouse hit-testing (step 1 foundation) ----------------------------------

// TestRenderGraphERDReturnsCards asserts renderGraphERD now also returns the
// laid-out cards, each carrying its final canvas position — the data the panel
// needs for mouse hit-testing.
func TestRenderGraphERDReturnsCards(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	_, cards := renderGraphERD(tables, schemas, pks, fks)
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	byName := map[string]*gcard{}
	for _, gc := range cards {
		byName[gc.name] = gc
	}
	for _, name := range []string{"users", "orders"} {
		gc := byName[name]
		if gc == nil {
			t.Errorf("missing card %q", name)
			continue
		}
		if gc.w < 1 || gc.h < 1 || gc.x < 0 || gc.y < 0 {
			t.Errorf("card %q has non-final layout: %+v", name, gc)
		}
	}
}

// cardByName returns the laid-out card with the given table name, or nil.
func cardByName(cards []*gcard, name string) *gcard {
	for _, c := range cards {
		if c != nil && c.name == name {
			return c
		}
	}
	return nil
}

// findRunePos returns the (column, row) of the first occurrence of r in s,
// viewed as newline-separated rows and counted by rune (so multibyte/ANSI-free
// cells map to their visual column). ok is false if r is absent.
func findRunePos(s string, r rune) (int, int, bool) {
	for row, line := range strings.Split(s, "\n") {
		col := 0
		for _, c := range line {
			if c == r {
				return col, row, true
			}
			col++
		}
	}
	return 0, 0, false
}

// TestERDContentToCanvas verifies the screen→canvas transform inverts the scroll
// + centring that View applies, in both the centred (canvas smaller than the
// viewport) and pan (canvas larger) regimes. It renders the panel, locates a
// known marker glyph, and checks contentToCanvas maps it back to its canvas cell
// — so the transform can't silently drift from the real rendering.
func TestERDContentToCanvas(t *testing.T) {
	check := func(e ERDPanel, cx, cy int) {
		t.Helper()
		view := ansi.Strip(e.View())
		col, row, ok := findRunePos(view, '*')
		if !ok {
			t.Fatalf("marker '*' not rendered:\n%s", view)
		}
		gx, gy, ok := e.contentToCanvas(col, row)
		if !ok {
			t.Fatalf("contentToCanvas(%d,%d) rejected the marker", col, row)
		}
		if gx != cx || gy != cy {
			t.Errorf("contentToCanvas(%d,%d) = (%d,%d), want (%d,%d)", col, row, gx, gy, cx, cy)
		}
	}

	// Centred regime: a 6×3 canvas in a 40×10 viewport.
	e := ERDPanel{width: 40, height: 10}
	e.graph = newGcanvas(6, 3)
	e.graph.setCh(2, 1, '*', "", false)
	check(e, 2, 1)

	// Pan regime: a 40×30 canvas scrolled to (10,4) in a 5×3 viewport.
	e2 := ERDPanel{width: 5, height: 3, scrollX: 10, scrollY: 4}
	e2.graph = newGcanvas(40, 30)
	e2.graph.setCh(12, 6, '*', "", false)
	check(e2, 12, 6)

	// A click in the centred margin (left of a small diagram) is rejected.
	if _, _, ok := e.contentToCanvas(0, 0); ok {
		t.Error("contentToCanvas should reject the centred margin")
	}
	// Mermaid view has no canvas to hit-test.
	em := ERDPanel{width: 40, height: 10, merm: true}
	em.graph = newGcanvas(6, 3)
	if _, _, ok := em.contentToCanvas(2, 2); ok {
		t.Error("contentToCanvas should reject the Mermaid view")
	}
}

// TestERDCardAt covers the canvas→card→column hit-tests the hover/click
// interactions will lean on: a point inside a card resolves to that card and its
// column row, the title row is not a column, and empty space yields nil.
func TestERDCardAt(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	_, cards := renderGraphERD(tables, schemas, pks, fks)
	e := ERDPanel{cards: cards, width: 80, height: 24}

	users := cardByName(cards, "users")
	if users == nil {
		t.Fatal("missing users card")
	}
	// The id (PK) row resolves to column id at index 0.
	col, idx, ok := e.columnAt(users, users.colRowY("id"))
	if !ok || col.Name != "id" || idx != 0 {
		t.Errorf("columnAt(id row) = %q idx %d ok %v, want id/0/true", col.Name, idx, ok)
	}
	// A point inside that row maps to the users card.
	if got := e.cardAt(users.x+3, users.colRowY("id")); got != users {
		t.Errorf("cardAt(inside users) = %v, want users", cardName(got))
	}
	// The title row is not a column.
	if _, _, ok := e.columnAt(users, users.y+1); ok {
		t.Error("title row should not resolve to a column")
	}
	// A point outside every card is nil.
	if got := e.cardAt(-1, -1); got != nil {
		t.Errorf("cardAt(-1,-1) = %q, want nil", cardName(got))
	}
}

// cardName returns a card's name, or "<nil>" for a nil card (test messages).
func cardName(c *gcard) string {
	if c == nil {
		return "<nil>"
	}
	return c.name
}

// --- mouse interaction (step 2: wheel + click) ------------------------------

// TestERDPanelWheel covers vertical/horizontal wheel scrolling and clamping.
// Vertical moves scrollY directly (keeping the cursor in view); horizontal
// moves scrollX by a quarter-viewport.
func TestERDPanelWheel(t *testing.T) {
	e := ERDPanel{width: 10, height: 6}
	e.graph = newGcanvas(40, 30) // taller (maxY=24) and wider (maxX=30) than view

	// Vertical: one notch down scrolls erdWheelLines rows.
	e = e.Wheel(1, 0)
	if e.scrollY != erdWheelLines {
		t.Errorf("Wheel down: scrollY=%d want %d", e.scrollY, erdWheelLines)
	}
	if e.cursor < e.scrollY || e.cursor >= e.scrollY+e.contentHeight() {
		t.Errorf("Wheel down left cursor %d outside view [%d,%d)", e.cursor, e.scrollY, e.scrollY+e.contentHeight())
	}

	// Clamp at the bottom.
	e.scrollY = 23
	e = e.Wheel(1, 0)
	if e.scrollY != 24 {
		t.Errorf("Wheel at bottom: scrollY=%d want 24 (clamped)", e.scrollY)
	}

	// Clamp at the top.
	e = e.Wheel(-100, 0)
	if e.scrollY != 0 {
		t.Errorf("Wheel far up: scrollY=%d want 0", e.scrollY)
	}

	// Horizontal: one notch right pans a quarter viewport (10/4 = 2 cols).
	e = e.Wheel(0, 1)
	if e.scrollX != 2 {
		t.Errorf("Wheel right: scrollX=%d want 2", e.scrollX)
	}
	e = e.Wheel(0, 100) // clamp at maxX
	if e.scrollX != 30 {
		t.Errorf("Wheel far right: scrollX=%d want 30 (clamped)", e.scrollX)
	}
}

// TestERDPanelCenterOnCard covers click-to-recentre: the card's centre lands at
// the viewport centre, clamped to the scrollable range at the edges.
func TestERDPanelCenterOnCard(t *testing.T) {
	e := ERDPanel{width: 10, height: 6}
	e.graph = newGcanvas(40, 30) // maxX=30, maxY=24

	// A mid-diagram card recentres fully: centre (7,5) → scroll (7-5, 5-3).
	c := &gcard{name: "t", x: 5, y: 4, w: 4, h: 3} // centre canvas (7,5)
	e = e.centerOnCard(c)
	if e.scrollX != 2 || e.scrollY != 2 {
		t.Errorf("centerOnCard mid: scroll=(%d,%d) want (2,2)", e.scrollX, e.scrollY)
	}

	// A top-left card clamps to (0,0).
	e2 := ERDPanel{width: 10, height: 6}
	e2.graph = newGcanvas(40, 30)
	e2 = e2.centerOnCard(&gcard{name: "t", x: 0, y: 0, w: 4, h: 3})
	if e2.scrollX != 0 || e2.scrollY != 0 {
		t.Errorf("centerOnCard corner: scroll=(%d,%d) want (0,0)", e2.scrollX, e2.scrollY)
	}
}

// mouseWheelMsg builds a MouseMsg for a wheel event (with optional shift).
func mouseWheelMsg(typ tea.MouseEventType, shift bool) tea.MouseMsg {
	return tea.MouseMsg{Type: typ, Shift: shift}
}

// TestHandleERDMouse routes wheel and click events through the Model handler,
// verifying the full path: event → ERDPanel.Wheel / centerOnCard.
func TestHandleERDMouse(t *testing.T) {
	m := Model{erdPanel: ERDPanel{width: 10, height: 6}}
	m.erdPanel.graph = newGcanvas(40, 30)

	// Wheel down → scrollY advances by erdWheelLines.
	mm, _ := m.handleERDMouse(mouseWheelMsg(tea.MouseWheelDown, false))
	if ep := mm.(Model).erdPanel; ep.scrollY != erdWheelLines {
		t.Errorf("wheel down: scrollY=%d want %d", ep.scrollY, erdWheelLines)
	}

	// Shift+wheel down → horizontal pan (scrollX moves, scrollY unchanged).
	m2 := Model{erdPanel: ERDPanel{width: 10, height: 6}}
	m2.erdPanel.graph = newGcanvas(40, 30)
	mm2, _ := m2.handleERDMouse(mouseWheelMsg(tea.MouseWheelDown, true))
	m2 = mm2.(Model)
	if ep := m2.erdPanel; ep.scrollX != 2 || ep.scrollY != 0 {
		t.Errorf("shift+wheel: scroll=(%d,%d) want (2,0)", ep.scrollX, ep.scrollY)
	}

	// Native horizontal wheel right → pan right (continues from scrollX=2).
	mm3, _ := m2.handleERDMouse(mouseWheelMsg(tea.MouseWheelRight, false))
	if ep := mm3.(Model).erdPanel; ep.scrollX != 4 {
		t.Errorf("wheel right: scrollX=%d want 4", ep.scrollX)
	}

	// Single left-click on a card toggles its highlight (sets selected), no pan.
	m4 := Model{erdPanel: ERDPanel{width: 10, height: 6}}
	m4.erdPanel.graph = newGcanvas(40, 30)
	m4.erdPanel.cards = []*gcard{{name: "t", x: 5, y: 4, w: 4, h: 3}}
	mm4, _ := m4.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 6, Y: 5})
	if ep := mm4.(Model).erdPanel; ep.selected != "t" {
		t.Errorf("single-click card: selected=%q want %q", ep.selected, "t")
	}
	if ep := mm4.(Model).erdPanel; ep.scrollX != 0 || ep.scrollY != 0 {
		t.Errorf("single-click should not pan: scroll=(%d,%d)", ep.scrollX, ep.scrollY)
	}

	// Double left-click on the same card recentres on it.
	m5 := Model{erdPanel: ERDPanel{width: 10, height: 6}}
	m5.erdPanel.graph = newGcanvas(40, 30)
	m5.erdPanel.cards = []*gcard{{name: "t", x: 5, y: 4, w: 4, h: 3}}
	m5.lastERDClickTime = time.Now()
	m5.lastERDClickCard = "t"
	mm5, _ := m5.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 6, Y: 5})
	if ep := mm5.(Model).erdPanel; ep.scrollX != 2 || ep.scrollY != 2 {
		t.Errorf("double-click card: scroll=(%d,%d) want (2,2)", ep.scrollX, ep.scrollY)
	}

	// Left-click on empty space clears any highlight.
	m6 := Model{erdPanel: ERDPanel{width: 10, height: 6, selected: "t"}}
	m6.erdPanel.graph = newGcanvas(40, 30)
	mm6, _ := m6.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 0, Y: 0})
	if ep := mm6.(Model).erdPanel; ep.selected != "" {
		t.Errorf("click empty: selected=%q want empty", ep.selected)
	}
}

// --- mouse interaction (step 3: highlight) ----------------------------------

// TestERDHighlightRender verifies render(selected) tints the selected card's
// border and the arrows touching it with the accent colour, leaving the
// unselected render using the primary card colour and grey connectors.
func TestERDHighlightRender(t *testing.T) {
	// The palette vars are empty until applyPalette runs, so set distinct
	// non-empty colours or the fg assertions below are vacuous ("" == "").
	sp, sa, sg, sf := colorPrimary, colorAccent, colorBorderUnfocused, colorFg
	colorPrimary, colorAccent, colorBorderUnfocused, colorFg = lipgloss.Color("1"), lipgloss.Color("2"), lipgloss.Color("3"), lipgloss.Color("7")
	defer func() { colorPrimary, colorAccent, colorBorderUnfocused, colorFg = sp, sa, sg, sf }()

	tables, schemas, pks, fks := erdFixture() // orders.user_id → users.id
	layout := computeERDLayout(tables, schemas, pks, fks)
	if layout == nil {
		t.Fatal("expected a layout")
	}
	orders := cardByName(layout.cards, "orders")
	users := cardByName(layout.cards, "users")
	borderFg := func(c *gcanvas, card *gcard) string {
		_, fg, _ := cellGlyph(c.cells[card.y][card.x])
		return fg
	}
	// fg of the first char of a column's name (names start at card.x+3).
	nameFg := func(c *gcanvas, card *gcard, col string) string {
		_, fg, _ := cellGlyph(c.cells[card.colRowY(col)][card.x+3])
		return fg
	}
	// fg of the first uppercase char on a column row = the type's first char
	// (fixture column names are all lowercase, so the first A-Z is the type).
	typeFg := func(c *gcanvas, card *gcard, col string) string {
		y := card.colRowY(col)
		for x := card.x + 3; x < card.x+card.w-1; x++ {
			if g, fg, _ := cellGlyph(c.cells[y][x]); g >= 'A' && g <= 'Z' {
				return fg
			}
		}
		return ""
	}

	none := layout.render("")
	sel := layout.render("orders")

	// No selection: every card vivid (primary border).
	if fg := borderFg(none, orders); fg != string(colorPrimary) {
		t.Errorf("no-selection orders border=%q want primary", fg)
	}
	if fg := borderFg(none, users); fg != string(colorPrimary) {
		t.Errorf("no-selection users border=%q want primary", fg)
	}
	// Selected: orders stays vivid (primary), the others dim to grey.
	if fg := borderFg(sel, orders); fg != string(colorPrimary) {
		t.Errorf("selected orders border=%q want primary (stays vivid)", fg)
	}
	if fg := borderFg(sel, users); fg != string(colorBorderUnfocused) {
		t.Errorf("non-selected users border=%q want grey (dimmed)", fg)
	}
	// Connected column on the dimmed card: users.id (the PK orders.user_id
	// points at) stays readable — name in the foreground, type in primary.
	if fg := nameFg(sel, users, "id"); fg != string(colorFg) {
		t.Errorf("connected users.id name fg=%q want foreground (white)", fg)
	}
	if fg := typeFg(sel, users, "id"); fg != string(colorPrimary) {
		t.Errorf("connected users.id type fg=%q want primary (blue)", fg)
	}
	// A non-connected column on the dimmed card stays grey.
	if fg := nameFg(sel, users, "name"); fg != string(colorBorderUnfocused) {
		t.Errorf("non-connected users.name name fg=%q want grey", fg)
	}

	// The single arrow (orders→users): grey normally, blue (primary) when it
	// touches the selected card.
	findHeadFg := func(c *gcanvas) string {
		for y := 0; y < c.h; y++ {
			for x := 0; x < c.w; x++ {
				if g, fg, _ := cellGlyph(c.cells[y][x]); g == arrowheadL() {
					return fg
				}
			}
		}
		return ""
	}
	if fg := findHeadFg(none); fg != string(colorBorderUnfocused) {
		t.Errorf("no-selection arrowhead fg=%q want grey", fg)
	}
	if fg := findHeadFg(sel); fg != string(colorPrimary) {
		t.Errorf("selected arrowhead fg=%q want primary (blue)", fg)
	}
}

// TestERDToggleHighlight covers the select/switch/deselect/clear logic and that
// each toggle re-renders the canvas: the selected card stays vivid (primary)
// while the others dim to grey.
func TestERDToggleHighlight(t *testing.T) {
	sp, sa, sg := colorPrimary, colorAccent, colorBorderUnfocused
	colorPrimary, colorAccent, colorBorderUnfocused = lipgloss.Color("1"), lipgloss.Color("2"), lipgloss.Color("3")
	defer func() { colorPrimary, colorAccent, colorBorderUnfocused = sp, sa, sg }()

	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	e := ERDPanel{width: 80, height: 24}
	e.layout = layout
	e.cards = layout.cards
	e.graph = layout.render("")
	orders := cardByName(layout.cards, "orders")
	users := cardByName(layout.cards, "users")

	borderFg := func(ep ERDPanel, name string) string {
		c := cardByName(ep.cards, name)
		_, fg, _ := cellGlyph(ep.graph.cells[c.y][c.x])
		return fg
	}

	// Select orders: orders vivid (primary), users dimmed (grey).
	e = e.toggleHighlight(orders)
	if e.selected != "orders" {
		t.Fatalf("select orders: selected=%q", e.selected)
	}
	if fg := borderFg(e, "orders"); fg != string(colorPrimary) {
		t.Errorf("after select, orders border fg=%q want primary (vivid)", fg)
	}
	if fg := borderFg(e, "users"); fg != string(colorBorderUnfocused) {
		t.Errorf("after select, users border fg=%q want grey (dimmed)", fg)
	}

	// Switch to users: users vivid, orders now dimmed.
	e = e.toggleHighlight(users)
	if e.selected != "users" {
		t.Fatalf("select users: selected=%q", e.selected)
	}
	if fg := borderFg(e, "users"); fg != string(colorPrimary) {
		t.Errorf("after switch, users border fg=%q want primary (vivid)", fg)
	}
	if fg := borderFg(e, "orders"); fg != string(colorBorderUnfocused) {
		t.Errorf("after switch, orders border fg=%q want grey (dimmed)", fg)
	}

	// Toggle users off → everything vivid again.
	e = e.toggleHighlight(users)
	if e.selected != "" {
		t.Fatalf("toggle off: selected=%q", e.selected)
	}
	if fg := borderFg(e, "orders"); fg != string(colorPrimary) {
		t.Errorf("after deselect, orders border fg=%q want primary", fg)
	}

	// nil clears.
	e = e.toggleHighlight(orders)
	e = e.toggleHighlight(nil)
	if e.selected != "" {
		t.Fatalf("nil clear: selected=%q", e.selected)
	}
}

// TestERDClickViaUpdateSizesPanel is a regression test for a bug where the ERD
// panel was sized only inside View() (a value-receiver, so it sized a discarded
// copy). The persistent model kept width/height = 0, so contentToCanvas saw a
// 1×1 viewport and rejected every click. The fix sizes the persistent panel in
// Update's mouse path. This routes a click through Model.Update (not a direct
// handleERDMouse call) with the panel intentionally left unsized, and checks
// the click both lands and sizes the panel.
func TestERDClickViaUpdateSizesPanel(t *testing.T) {
	sp, sa, sg := colorPrimary, colorAccent, colorBorderUnfocused
	colorPrimary, colorAccent, colorBorderUnfocused = lipgloss.Color("1"), lipgloss.Color("2"), lipgloss.Color("3")
	defer func() { colorPrimary, colorAccent, colorBorderUnfocused = sp, sa, sg }()

	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{width: 120, height: 40}
	m.erdPanel.Show("erd", layout, nil)

	// Size once just to render and locate the card on screen, then wipe the
	// size to reproduce the bug's precondition (unsized persistent panel).
	m.erdPanel.SetSize(m.width, m.height-1)
	view := ansi.Strip(m.erdPanel.View())
	var sx, sy int
	for i, ln := range strings.Split(view, "\n") {
		if j := strings.Index(ln, "orders"); j >= 0 {
			sy, sx = i, j
			break
		}
	}
	m.erdPanel.width = 0
	m.erdPanel.height = 0
	if cw, ch := m.erdPanel.contentWidth(), m.erdPanel.contentHeight(); cw != 1 || ch != 1 {
		t.Fatalf("precondition: want cw=ch=1, got (%d,%d)", cw, ch)
	}

	// A click routed through Update must size the panel and land on the card.
	mm, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: sx, Y: sy})
	got := mm.(Model).erdPanel
	if got.selected != "orders" {
		t.Errorf("click via Update: selected=%q want orders (panel not sized → hit-test rejected?)", got.selected)
	}
	if cw := got.contentWidth(); cw != 120 {
		t.Errorf("persistent panel width after click = %d, want 120", cw)
	}
}
