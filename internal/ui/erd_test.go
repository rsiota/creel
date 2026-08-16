package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rsiota/creel/internal/db"
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
	for _, want := range []rune{'┌', '└', '│', '─', '◆', '◇', arrowheadL()} {
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
	// Body clicks fire on release (press records a pending drag, release commits
	// the click), so a MouseLeft must be followed by a MouseRelease.
	m4 := Model{erdPanel: ERDPanel{width: 10, height: 6}}
	m4.erdPanel.graph = newGcanvas(40, 30)
	m4.erdPanel.cards = []*gcard{{name: "t", x: 5, y: 4, w: 4, h: 3}}
	mm4, _ := m4.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 6, Y: 5})
	mm4, _ = mm4.(Model).handleERDMouse(tea.MouseMsg{Type: tea.MouseRelease, X: 6, Y: 5})
	if ep := mm4.(Model).erdPanel; ep.selected != "t" {
		t.Errorf("single-click card: selected=%q want %q", ep.selected, "t")
	}
	if ep := mm4.(Model).erdPanel; ep.scrollX != 0 || ep.scrollY != 0 {
		t.Errorf("single-click should not pan: scroll=(%d,%d)", ep.scrollX, ep.scrollY)
	}

	// Double left-click on the same card browses it (SELECT *), closing the ERD.
	m5 := Model{erdPanel: ERDPanel{width: 10, height: 6}, editor: NewQueryEditor()}
	m5.erdPanel.graph = newGcanvas(40, 30)
	m5.erdPanel.cards = []*gcard{{name: "t", x: 5, y: 4, w: 4, h: 3}}
	m5.lastERDClickTime = time.Now()
	m5.lastERDClickCard = "t"
	mm5, _ := m5.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, X: 6, Y: 5})
	mm5, _ = mm5.(Model).handleERDMouse(tea.MouseMsg{Type: tea.MouseRelease, X: 6, Y: 5})
	if ep := mm5.(Model).erdPanel; ep.IsVisible() {
		t.Errorf("double-click card should close the ERD")
	}
	if got := mm5.(Model).editor.Value(); !strings.Contains(got, "SELECT * FROM t") {
		t.Errorf("double-click editor = %q, want SELECT * FROM t", got)
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

	none := layout.render("", "", erdPath{})
	sel := layout.render("orders", "", erdPath{})

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

// TestERDHighlightArrowOnTop verifies that a highlighted (blue) connector is
// never buried under a dimmed (grey) one where the two share canvas cells.
// Two tables both referencing the same parent draw arrows whose arrowhead and
// vertical riser land on the exact same cells; selecting just one child must
// leave those shared cells blue (the selected relationship) rather than grey
// (the unselected one), regardless of arrow draw order.
func TestERDHighlightArrowOnTop(t *testing.T) {
	sp, sa, sg, sf := colorPrimary, colorAccent, colorBorderUnfocused, colorFg
	colorPrimary, colorAccent, colorBorderUnfocused, colorFg = lipgloss.Color("1"), lipgloss.Color("2"), lipgloss.Color("3"), lipgloss.Color("7")
	defer func() { colorPrimary, colorAccent, colorBorderUnfocused, colorFg = sp, sa, sg, sf }()

	// orders and posts both reference users → their FK arrows share the
	// arrowhead cell at users' right edge and the vertical riser beside it.
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	layout := computeERDLayout(tables, schemas, pks, fks)
	if layout == nil {
		t.Fatal("expected a layout")
	}
	users := cardByName(layout.cards, "users")

	// Select orders: orders→users turns blue, posts→users stays grey. Both
	// arrows paint the same cells at users' right edge, so blue must win there.
	sel := layout.render("orders", "", erdPath{})

	// The shared arrowhead (left-pointing, into users' right border).
	headCount := 0
	var headFg string
	for y := 0; y < sel.h; y++ {
		for x := 0; x < sel.w; x++ {
			if g, fg, _ := cellGlyph(sel.cells[y][x]); g == arrowheadL() {
				headFg = fg
				headCount++
			}
		}
	}
	if headCount != 1 {
		t.Fatalf("expected one shared left arrowhead, found %d", headCount)
	}
	if headFg != string(colorPrimary) {
		t.Errorf("shared arrowhead fg=%q want primary (blue) — highlight buried under grey arrow", headFg)
	}

	// The shared vertical-riser bend just outside users' right border (vertX,
	// parent PK row): both arrows draw a line through it, so it must be blue.
	if users != nil {
		py := users.colRowY("id")
		vertX := users.x + users.w + 1
		if sel.inBounds(vertX, py) {
			_, bendFg, _ := cellGlyph(sel.cells[py][vertX])
			if bendFg != string(colorPrimary) {
				t.Errorf("shared riser cell fg=%q want primary (blue) — highlight line buried under grey", bendFg)
			}
		}
	}
}

// TestERDFocusIconRender verifies the header drill-in icon (⤢) is painted on
// every card except the focus root in a focused ERD, and on no cards in the
// whole-schema view (no focus) or a bare render.
func TestERDFocusIconRender(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	layout := computeERDLayout(tables, schemas, pks, fks)
	if layout == nil {
		t.Fatal("expected a layout")
	}
	iconAt := func(c *gcanvas, card *gcard) rune {
		g, _, _ := cellGlyph(c.cells[card.y+1][card.x+card.w-3])
		return g
	}

	// Whole-schema view (no focus): no icons anywhere.
	layout.focus = ""
	whole := layout.render("", "", erdPath{})
	for _, card := range layout.cards {
		if g := iconAt(whole, card); g == erdFocusIcon {
			t.Errorf("whole-schema: %s header has icon, want none", card.name)
		}
	}

	// Focused on users: icon on orders & posts, not on the root users card.
	layout.focus = "users"
	foc := layout.render("", "", erdPath{})
	users := cardByName(layout.cards, "users")
	if g := iconAt(foc, users); g == erdFocusIcon {
		t.Errorf("focus root users header has icon, want none")
	}
	for _, name := range []string{"orders", "posts"} {
		card := cardByName(layout.cards, name)
		if g := iconAt(foc, card); g != erdFocusIcon {
			t.Errorf("non-root %s header glyph=%q want %q (drill-in icon)", name, g, erdFocusIcon)
		}
	}
}

// TestERDDrillInHitTest covers the panel's header hit-testing: any canvas cell
// on a non-root card's title row resolves to that card (the whole header is the
// target), while the root card's header, a column cell, and the whole-schema
// view all yield no drill-in.
func TestERDDrillInHitTest(t *testing.T) {
	tables := []string{"users", "orders"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}}
	fks := map[string][]db.ForeignKey{"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}}}
	layout := computeERDLayout(tables, schemas, pks, fks)
	users := cardByName(layout.cards, "users")
	orders := cardByName(layout.cards, "orders")

	// Focused ERD: any header cell of orders (left name area, glyph cell, even
	// the border cells) returns orders; the root's header and a column cell do
	// not.
	layout.focus = "users"
	e := ERDPanel{cards: layout.cards, layout: layout}
	for _, sx := range []int{orders.x, orders.x + 2, orders.x + orders.w - 3, orders.x + orders.w - 1} {
		if got := e.drillInCard(sx, orders.y+1); got == nil || got.name != "orders" {
			t.Errorf("header cell x=%d: got %v want orders", sx, got)
		}
	}
	if got := e.drillInCard(users.x+2, users.y+1); got != nil {
		t.Errorf("root header: got %v want nil (no drill-in on root)", got)
	}
	if got := e.drillInCard(orders.x+3, orders.y+3); got != nil {
		t.Errorf("column cell: got %v want nil (not the header)", got)
	}

	// Whole-schema view: no drill-in anywhere.
	layout.focus = ""
	if got := e.drillInCard(orders.x+2, orders.y+1); got != nil {
		t.Errorf("whole-schema header: got %v want nil (no drill-in)", got)
	}
}

// TestERDDrillInClick drives the full click path: opening a focused ERD then
// clicking a non-root card's header re-focuses the diagram on that table,
// while a click on the card body still toggles highlight.
func TestERDDrillInClick(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	focusOf := func(ep ERDPanel) string {
		if ep.layout != nil {
			return ep.layout.focus
		}
		return ""
	}

	m := Model{
		connection:  &db.Connection{},
		tables:      tables,
		columnCache: schemas,
		pkCache:     pks,
		fkCache:     fks,
		width:       80,
		height:      24,
	}
	m.openERD("users")
	if focusOf(m.erdPanel) != "users" {
		t.Fatalf("openERD(users): focus=%q want users", focusOf(m.erdPanel))
	}

	orders := m.erdPanel.cardNamed("orders")
	if orders == nil {
		t.Fatal("orders card missing from users-focused ERD")
	}
	_, _, offX, offY := m.erdPanel.placedBounds()
	// A column-row cell of orders is the card body, not the header: it toggles
	// highlight without changing the focus.
	bodySX := orders.x + 3 - m.erdPanel.scrollX + offX
	bodySY := orders.y + 3 - m.erdPanel.scrollY + offY
	mb, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, X: bodySX, Y: bodySY})
	mb, _ = mb.(Model).handleERDMouse(tea.MouseMsg{Type: tea.MouseRelease, X: bodySX, Y: bodySY})
	pb := mb.(Model).erdPanel
	if focusOf(pb) != "users" {
		t.Errorf("body click changed focus to %q; want users (header-only drill-in)", focusOf(pb))
	}
	if pb.selected != "orders" {
		t.Errorf("body click selected=%q want orders (toggle highlight)", pb.selected)
	}

	// Any header cell of orders (here the title area, well clear of the glyph)
	// re-focuses the ERD on orders.
	headSX := orders.x + 2 - m.erdPanel.scrollX + offX
	headSY := orders.y + 1 - m.erdPanel.scrollY + offY
	mi, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, X: headSX, Y: headSY})
	pi := mi.(Model).erdPanel
	if focusOf(pi) != "orders" {
		t.Errorf("header click focus=%q want orders", focusOf(pi))
	}
	if pi.cardNamed("orders") == nil {
		t.Error("header click: orders card missing from re-focused ERD")
	}
}

// TestERDFocusAccent checks the keyboard-focus border colour: a focused card
// is accent, a focused+selected card stays primary (selection outranks focus),
// and a focused card on a dimmed diagram is still accent so the cursor stays
// visible.
func TestERDFocusAccent(t *testing.T) {
	sp, sa, sg, sf := colorPrimary, colorAccent, colorBorderUnfocused, colorFg
	colorPrimary, colorAccent, colorBorderUnfocused, colorFg = lipgloss.Color("1"), lipgloss.Color("2"), lipgloss.Color("3"), lipgloss.Color("7")
	defer func() { colorPrimary, colorAccent, colorBorderUnfocused, colorFg = sp, sa, sg, sf }()

	tables, schemas, pks, fks := erdFixture() // users, orders
	layout := computeERDLayout(tables, schemas, pks, fks)
	users := cardByName(layout.cards, "users")
	orders := cardByName(layout.cards, "orders")
	borderFg := func(c *gcanvas, card *gcard) string {
		_, fg, _ := cellGlyph(c.cells[card.y][card.x])
		return fg
	}

	// Focus orders, nothing selected: orders accent, users primary.
	c := layout.render("", "orders", erdPath{})
	if fg := borderFg(c, orders); fg != string(colorAccent) {
		t.Errorf("focused orders border=%q want accent", fg)
	}
	if fg := borderFg(c, users); fg != string(colorPrimary) {
		t.Errorf("non-focused users border=%q want primary", fg)
	}
	// Focus + select the same card: selection wins → primary.
	c2 := layout.render("orders", "orders", erdPath{})
	if fg := borderFg(c2, orders); fg != string(colorPrimary) {
		t.Errorf("selected+focused orders border=%q want primary (selection wins)", fg)
	}
	// Select users, focus orders (a different, dimmed card): orders stays accent
	// so the cursor is visible even while dimmed; users is primary.
	c3 := layout.render("users", "orders", erdPath{})
	if fg := borderFg(c3, orders); fg != string(colorAccent) {
		t.Errorf("focused-on-dimmed orders border=%q want accent", fg)
	}
	if fg := borderFg(c3, users); fg != string(colorPrimary) {
		t.Errorf("selected users border=%q want primary", fg)
	}
}

// TestERDKeyboardNav exercises spatial j/k/h/l focus movement: j/k step through
// a column's stack and h/l move between rank columns.
func TestERDKeyboardNav(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = "" // whole schema: every card is navigable
	users := cardByName(layout.cards, "users")
	orders := cardByName(layout.cards, "orders")
	posts := cardByName(layout.cards, "posts")
	if !(users.x < orders.x) {
		t.Fatalf("users should be left of orders (users.x=%d orders.x=%d)", users.x, orders.x)
	}
	if !(orders.y < posts.y) {
		t.Fatalf("orders should be above posts (orders.y=%d posts.y=%d)", orders.y, posts.y)
	}
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24}

	// j/k step through the column-1 stack (orders ↔ posts).
	ep.focusName = "orders"
	ep.graph = layout.render("", "orders", erdPath{})
	ep = ep.Update(key("j"))
	if ep.focusName != "posts" {
		t.Errorf("j from orders: focus=%q want posts", ep.focusName)
	}
	ep = ep.Update(key("k"))
	if ep.focusName != "orders" {
		t.Errorf("k from posts: focus=%q want orders", ep.focusName)
	}
	// h from orders → users (the only card to the left).
	ep = ep.Update(key("h"))
	if ep.focusName != "users" {
		t.Errorf("h from orders: focus=%q want users", ep.focusName)
	}
	// l from users → a column-1 card (orders or posts); h then returns to users.
	ep = ep.Update(key("l"))
	if ep.focusName != "orders" && ep.focusName != "posts" {
		t.Errorf("l from users: focus=%q want orders or posts", ep.focusName)
	}
	col1 := ep.focusName
	ep = ep.Update(key("h"))
	if ep.focusName != "users" {
		t.Errorf("h from %s: focus=%q want users", col1, ep.focusName)
	}
	// h from the leftmost card is a no-op.
	ep = ep.Update(key("h"))
	if ep.focusName != "users" {
		t.Errorf("h from users: focus=%q want users (no card left)", ep.focusName)
	}
}

// TestERDKeyboardHighlight checks Space toggles highlight on the focused card
// (and that a selected+focused card renders primary, not accent).
func TestERDKeyboardHighlight(t *testing.T) {
	sp, sa, sg := colorPrimary, colorAccent, colorBorderUnfocused
	colorPrimary, colorAccent, colorBorderUnfocused = lipgloss.Color("1"), lipgloss.Color("2"), lipgloss.Color("3")
	defer func() { colorPrimary, colorAccent, colorBorderUnfocused = sp, sa, sg }()

	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	orders := cardByName(layout.cards, "orders")
	users := cardByName(layout.cards, "users")
	borderFg := func(c *gcanvas, card *gcard) string {
		_, fg, _ := cellGlyph(c.cells[card.y][card.x])
		return fg
	}
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "orders"}
	ep.graph = layout.render("", "orders", erdPath{})

	ep = ep.Update(tea.KeyMsg{Type: tea.KeySpace})
	if ep.selected != "orders" {
		t.Fatalf("space: selected=%q want orders", ep.selected)
	}
	if ep.focusName != "orders" {
		t.Errorf("space moved focus: %q", ep.focusName)
	}
	if fg := borderFg(ep.graph, orders); fg != string(colorPrimary) {
		t.Errorf("selected+focused orders border=%q want primary", fg)
	}
	if fg := borderFg(ep.graph, users); fg != string(colorBorderUnfocused) {
		t.Errorf("dimmed users border=%q want grey", fg)
	}
	// Space again clears the selection.
	ep = ep.Update(tea.KeyMsg{Type: tea.KeySpace})
	if ep.selected != "" {
		t.Errorf("space again: selected=%q want empty", ep.selected)
	}
}

// TestERDEnterOpensTable checks Enter closes the ERD and loads SELECT * for
// the focused card (through erdEnter → openTable).
func TestERDEnterOpensTable(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	m := Model{
		connection:  &db.Connection{},
		tables:      tables,
		columnCache: schemas,
		pkCache:     pks,
		fkCache:     fks,
		width:       80,
		height:      24,
		editor:      NewQueryEditor(),
	}
	m.openERD("")
	if !m.erdPanel.IsVisible() {
		t.Fatal("openERD(\"\") did not show the panel")
	}
	m.erdPanel.focusName = "orders"
	mm, _ := m.erdEnter()
	if mm.erdPanel.IsVisible() {
		t.Error("enter should close the ERD overlay")
	}
	if got := mm.editor.Value(); !strings.Contains(got, "SELECT * FROM orders") {
		t.Errorf("editor = %q, want SELECT * FROM orders", got)
	}
}

// TestERDDrillIn checks `f` re-focuses the ERD on the keyboard-focused card's
// neighbourhood (the previous Enter behaviour).
func TestERDDrillIn(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	focusOf := func(ep ERDPanel) string {
		if ep.layout != nil {
			return ep.layout.focus
		}
		return ""
	}
	m := Model{
		connection:  &db.Connection{},
		tables:      tables,
		columnCache: schemas,
		pkCache:     pks,
		fkCache:     fks,
		width:       80,
		height:      24,
	}
	m.openERD("")
	if !m.erdPanel.IsVisible() {
		t.Fatal("openERD(\"\") did not show the panel")
	}
	m.erdPanel.focusName = "orders"
	mm, _ := m.erdDrillIn()
	got := mm.erdPanel
	if focusOf(got) != "orders" {
		t.Errorf("f: layout focus=%q want orders", focusOf(got))
	}
	if got.cardNamed("orders") == nil {
		t.Error("f: orders card missing from re-focused ERD")
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
	e.graph = layout.render("", "", erdPath{})
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
	// Body clicks fire on release, so send MouseLeft then MouseRelease.
	mm, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: sx, Y: sy})
	mm, _ = mm.(Model).Update(tea.MouseMsg{Type: tea.MouseRelease, X: sx, Y: sy})
	got := mm.(Model).erdPanel
	if got.selected != "orders" {
		t.Errorf("click via Update: selected=%q want orders (panel not sized → hit-test rejected?)", got.selected)
	}
	if cw := got.contentWidth(); cw != 120 {
		t.Errorf("persistent panel width after click = %d, want 120", cw)
	}
}

// TestERDHintsOnStatusBar checks the ERD panel advertises its keybindings on
// the status bar: the graph view shows the spatial-nav keys (j/k/h/l, space,
// enter) while the Mermaid source view drops the graph-only keys.
func TestERDHintsOnStatusBar(t *testing.T) {
	m := Model{state: stateWorkspace}
	m.erdPanel.visible = true

	graph := strings.Join(m.hintList(), " ")
	for _, want := range []string{"j/k/h/l", "space", "enter", "m", "g/G", "ctrl+d/ctrl+u", "y", "esc"} {
		if !strings.Contains(graph, want) {
			t.Errorf("graph hints missing %q: got %v", want, m.hintList())
		}
	}

	m.erdPanel.merm = true
	merm := strings.Join(m.hintList(), " ")
	for _, want := range []string{"j/k", "enter", "m", "g/G", "ctrl+d/ctrl+u", "y", "esc"} {
		if !strings.Contains(merm, want) {
			t.Errorf("mermaid hints missing %q: got %v", want, m.hintList())
		}
	}
	// Mermaid view must not advertise the graph-only keys.
	for _, gone := range []string{"space", "h/l", "j/k/h/l"} {
		if strings.Contains(merm, gone) {
			t.Errorf("mermaid hints should not include %q: got %v", gone, m.hintList())
		}
	}
}

// TestERDRegistrySection confirms the ERD section exists in the help-overlay
// registry (so its keybindings are documented under "?") and that its hint
// fields yield the graph-view status-bar set.
func TestERDRegistrySection(t *testing.T) {
	var sec *Section
	for i := range registry() {
		if registry()[i].Title == "ERD" {
			sec = &registry()[i]
			break
		}
	}
	if sec == nil {
		t.Fatal("no ERD section in registry")
	}
	displays := map[string]bool{}
	for _, b := range sec.Items {
		displays[b.Display] = true
	}
	for _, want := range []string{"esc / q / ctrl+c", "j/k/h/l", "space", "enter", "m", "g / G", "ctrl+d / ctrl+u", "y", "s"} {
		if !displays[want] {
			t.Errorf("ERD section missing binding %q", want)
		}
	}
	hints := strings.Join(hintsForSection("ERD"), " ")
	for _, want := range []string{"j/k/h/l", "space", "enter", "m", "g/G", "ctrl+d/ctrl+u", "y", "esc"} {
		if !strings.Contains(hints, want) {
			t.Errorf("ERD hints missing %q: got %v", want, hints)
		}
	}
}

// erdSearchFixture builds a 3-table layout (users, orders, order_items) whose
// names share substrings, for "/" jump-bar tests.
func erdSearchFixture() *erdLayout {
	tables := []string{"users", "orders", "order_items"}
	schemas := map[string][]db.Column{
		"users":       {{Name: "id", Type: "int"}},
		"orders":      {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"order_items": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "order_items": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders":      {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"order_items": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	l := computeERDLayout(tables, schemas, pks, fks)
	l.focus = ""
	return l
}

// TestERDSearchJump covers the "/" jump bar: typing filters cards and focuses
// the first match (alphabetical card order), tab cycles further matches and
// wraps, enter confirms, and the prompt renders.
func TestERDSearchJump(t *testing.T) {
	layout := erdSearchFixture()
	runes := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = layout.render("", "users", erdPath{})

	ep = ep.Update(runes("/"))
	if !ep.searching {
		t.Fatal("'/' did not open the search bar")
	}
	for _, r := range "ord" {
		ep = ep.Update(runes(string(r)))
	}
	// matches for "ord" (alphabetical): order_items, orders → first is order_items.
	if ep.focusName != "order_items" {
		t.Errorf("after 'ord': focus=%q want order_items", ep.focusName)
	}
	// The prompt line should be visible.
	if !strings.Contains(ep.View(), "/") {
		t.Error("search prompt not rendered in View")
	}

	// tab → next match (orders); tab again wraps to the first (order_items).
	ep = ep.Update(tea.KeyMsg{Type: tea.KeyTab})
	if ep.focusName != "orders" {
		t.Errorf("tab: focus=%q want orders", ep.focusName)
	}
	ep = ep.Update(tea.KeyMsg{Type: tea.KeyTab})
	if ep.focusName != "order_items" {
		t.Errorf("tab wrap: focus=%q want order_items", ep.focusName)
	}

	// enter confirms: the bar closes and the focus is retained.
	ep = ep.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if ep.searching {
		t.Error("enter did not close the search bar")
	}
	if ep.focusName != "order_items" {
		t.Errorf("enter: focus=%q want order_items (retained)", ep.focusName)
	}
}

// TestERDSearchEscapeRestores checks esc cancels the search and restores the
// pre-search focus.
func TestERDSearchEscapeRestores(t *testing.T) {
	layout := erdSearchFixture()
	runes := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = layout.render("", "users", erdPath{})

	ep = ep.Update(runes("/"))
	for _, r := range "ord" {
		ep = ep.Update(runes(string(r)))
	}
	if ep.focusName != "order_items" {
		t.Fatalf("precondition: focus=%q want order_items", ep.focusName)
	}

	ep = ep.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if ep.searching {
		t.Error("esc did not close the search bar")
	}
	if ep.focusName != "users" {
		t.Errorf("esc: focus=%q want users (restored)", ep.focusName)
	}
}

// TestERDSearchBackspace checks backspace shrinks the query and re-applies it:
// "orders" matches only orders, then backspacing to "order" broadens to the
// first match order_items.
func TestERDSearchBackspace(t *testing.T) {
	layout := erdSearchFixture()
	runes := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = layout.render("", "users", erdPath{})

	ep = ep.Update(runes("/"))
	for _, r := range "orders" {
		ep = ep.Update(runes(string(r)))
	}
	if ep.focusName != "orders" {
		t.Fatalf("precondition: focus=%q want orders", ep.focusName)
	}

	ep = ep.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if ep.searchQuery != "order" {
		t.Errorf("backspace: query=%q want order", ep.searchQuery)
	}
	if ep.focusName != "order_items" {
		t.Errorf("after backspace: focus=%q want order_items (first match of 'order')", ep.focusName)
	}
}

// erdPathFixture builds a 4-table layout with a 2-hop chain
// users—orders—order_items plus a disconnected "logs" table, for FK
// path-finding tests.
func erdPathFixture() *erdLayout {
	tables := []string{"users", "orders", "order_items", "logs"}
	schemas := map[string][]db.Column{
		"users":       {{Name: "id", Type: "int"}},
		"orders":      {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"order_items": {{Name: "id", Type: "int"}, {Name: "order_id", Type: "int"}},
		"logs":        {{Name: "id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "order_items": {"id"}, "logs": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders":      {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"order_items": {{Column: "order_id", RefTable: "orders", RefColumn: "id"}},
	}
	l := computeERDLayout(tables, schemas, pks, fks)
	l.focus = ""
	return l
}

// TestERDShortestPath covers the BFS: a 2-hop chain, its reverse, the same-table
// and disconnected-table nil cases.
func TestERDShortestPath(t *testing.T) {
	layout := erdPathFixture()
	join := func(p []string) string { return strings.Join(p, ",") }

	if got := join(erdShortestPath(layout, "users", "order_items")); got != "users,orders,order_items" {
		t.Errorf("users→order_items = %q want users,orders,order_items", got)
	}
	if got := join(erdShortestPath(layout, "order_items", "users")); got != "order_items,orders,users" {
		t.Errorf("order_items→users = %q want order_items,orders,users", got)
	}
	if erdShortestPath(layout, "users", "users") != nil {
		t.Error("same-table path should be nil")
	}
	if erdShortestPath(layout, "users", "logs") != nil {
		t.Error("users→logs (disconnected) should be nil")
	}
}

// TestERDPathToggle covers the three-state "p" cycle: anchor → trace → clear.
func TestERDPathToggle(t *testing.T) {
	layout := erdPathFixture()
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = ep.renderedGraph()

	// 1st p: anchor the focused card (users).
	ep = ep.Update(key("p"))
	if ep.pathFrom != "users" || len(ep.pathCards) != 0 {
		t.Errorf("anchor: pathFrom=%q pathCards=%v want users/[]", ep.pathFrom, ep.pathCards)
	}

	// Move focus to the target and trace.
	ep.focusName = "order_items"
	ep = ep.Update(key("p"))
	if got := strings.Join(ep.pathCards, ","); got != "users,orders,order_items" {
		t.Errorf("trace: pathCards=%v want [users orders order_items]", ep.pathCards)
	}

	// 3rd p: clear back to idle.
	ep = ep.Update(key("p"))
	if ep.pathFrom != "" || len(ep.pathCards) != 0 {
		t.Errorf("clear: pathFrom=%q pathCards=%v want empty", ep.pathFrom, ep.pathCards)
	}
}

// TestERDPathNoPath checks a trace to a disconnected target notes a no-path
// message and leaves the anchor in place.
func TestERDPathNoPath(t *testing.T) {
	layout := erdPathFixture()
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = ep.renderedGraph()

	ep = ep.togglePath() // anchor users
	if ep.pathFrom != "users" {
		t.Fatalf("anchor: pathFrom=%q want users", ep.pathFrom)
	}
	ep.focusName = "logs" // disconnected from users
	ep = ep.togglePath()
	if ep.pathMsg == "" {
		t.Error("expected a no-path message")
	}
	if len(ep.pathCards) != 0 {
		t.Errorf("should not have traced: pathCards=%v", ep.pathCards)
	}
	if ep.pathFrom != "users" {
		t.Errorf("anchor should remain: pathFrom=%q want users", ep.pathFrom)
	}
}

// TestERDPathRender checks a traced path keeps path cards vivid (primary) and
// dims the rest (grey).
func TestERDPathRender(t *testing.T) {
	sp, sa, sg := colorPrimary, colorAccent, colorBorderUnfocused
	colorPrimary, colorAccent, colorBorderUnfocused = lipgloss.Color("1"), lipgloss.Color("2"), lipgloss.Color("3")
	defer func() { colorPrimary, colorAccent, colorBorderUnfocused = sp, sa, sg }()

	layout := erdPathFixture()
	p := pathHighlight([]string{"users", "orders", "order_items"})
	c := layout.render("", "", p)
	users := cardByName(layout.cards, "users")
	logs := cardByName(layout.cards, "logs") // off the path
	borderFg := func(card *gcard) string {
		_, fg, _ := cellGlyph(c.cells[card.y][card.x])
		return fg
	}
	if fg := borderFg(users); fg != string(colorPrimary) {
		t.Errorf("path card users border=%q want primary", fg)
	}
	if fg := borderFg(logs); fg != string(colorBorderUnfocused) {
		t.Errorf("off-path card logs border=%q want grey", fg)
	}
}

// TestERDPathClear checks clearPath resets all path state (used by esc).
func TestERDPathClear(t *testing.T) {
	layout := erdPathFixture()
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = ep.renderedGraph()

	ep = ep.togglePath()
	ep.focusName = "order_items"
	ep = ep.togglePath()
	if len(ep.pathCards) == 0 {
		t.Fatal("precondition: path not traced")
	}

	ep = ep.clearPath()
	if ep.pathFrom != "" || len(ep.pathCards) != 0 || ep.pathMsg != "" {
		t.Errorf("clearPath left state: pathFrom=%q pathCards=%v msg=%q", ep.pathFrom, ep.pathCards, ep.pathMsg)
	}
}

// --- free-form drag: dynamic routing (Level B) ------------------------------

// mkCard builds a *gcard with named columns so colRowY resolves rows. The first
// column is treated as the FK for arrow-endpoint purposes in these tests.
func mkCard(name string, x, y, w, h int, cols ...string) *gcard {
	c := &gcard{name: name, x: x, y: y, w: w, h: h, pkSet: map[string]bool{}, fkSet: map[string]bool{}}
	for _, cn := range cols {
		c.cols = append(c.cols, db.Column{Name: cn, Type: "int"})
	}
	if len(cols) > 0 {
		c.fkSet[cols[0]] = true
		c.pkSet[cols[len(cols)-1]] = true
	}
	return c
}

// segHitsRect reports whether the orthogonal segment p→q passes through any cell
// of the rectangle [rx, rx+rw) × [ry, ry+rh).
func segHitsRect(p, q erdPoint, rx, ry, rw, rh int) bool {
	if p.y == q.y { // horizontal run
		x0, x1 := p.x, q.x
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if !(ry <= p.y && p.y < ry+rh) {
			return false
		}
		return x0 < rx+rw && rx <= x1
	}
	// vertical run (p.x == q.x)
	y0, y1 := p.y, q.y
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	if !(rx <= p.x && p.x < rx+rw) {
		return false
	}
	return y0 < ry+rh && ry <= y1
}

// polylineAvoids reports whether every segment of pts misses the card's
// rectangle — the Level B invariant that an arrow never passes under a card
// (cards paint over arrows, so a hidden line reads as a missing relationship).
func polylineAvoids(pts []erdPoint, c *gcard) bool {
	for i := 1; i < len(pts); i++ {
		if segHitsRect(pts[i-1], pts[i], c.x, c.y, c.w, c.h) {
			return false
		}
	}
	return true
}

// TestERDRouteSideClear verifies a clean side-channel elbow is found when the
// child and parent sit in separate X bands with a clear gutter, and that the
// arrowhead faces the parent.
func TestERDRouteSideClear(t *testing.T) {
	child := mkCard("orders", 0, 0, 8, 6, "user_id", "id")
	parent := mkCard("users", 20, 0, 8, 6, "id")
	all := []*gcard{child, parent}
	pts, side := routeArrow(child, parent, "user_id", "id", all, nil)
	if len(pts) != 4 {
		t.Fatalf("side route: expected 4 vertices, got %d (%v)", len(pts), pts)
	}
	if side != erdRight {
		t.Errorf("side route: headSide=%v want erdRight (parent is right of child)", side)
	}
	// Arrowhead lands just outside the parent's left border.
	last := pts[len(pts)-1]
	if last.x != parent.x-1 {
		t.Errorf("side route: arrowhead x=%d want %d", last.x, parent.x-1)
	}
	for _, c := range all {
		if !polylineAvoids(pts, c) {
			t.Errorf("side route: polyline enters card %q", c.name)
		}
	}
}

// TestERDRouteArrowAvoidsObstacle is the core Level B behaviour: with a third
// card blocking the direct horizontal between child and parent, the router
// routes around it (here, beneath the cards) so no segment passes under any card.
func TestERDRouteArrowAvoidsObstacle(t *testing.T) {
	child := mkCard("orders", 0, 0, 8, 6, "user_id", "id")
	parent := mkCard("users", 20, 0, 8, 6, "id")
	obstacle := mkCard("products", 10, 2, 6, 4, "id") // blocks row 3 across x[10,15]
	all := []*gcard{child, parent, obstacle}
	pts, _ := routeArrow(child, parent, "user_id", "id", all, nil)
	if len(pts) < 4 {
		t.Fatalf("obstacle route: expected ≥4 vertices, got %d (%v)", len(pts), pts)
	}
	for _, c := range all {
		if !polylineAvoids(pts, c) {
			t.Errorf("obstacle route: polyline enters card %q (pts=%v)", c.name, pts)
		}
	}
}

// TestERDRerouteArrows verifies that after a card is moved the layout's arrows
// are re-resolved to polylines (pts set) that avoid every card, and the canvas
// is grown to contain the new positions.
func TestERDRerouteArrows(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	if layout == nil || len(layout.arrows) != 1 {
		t.Fatalf("precondition: want 1 arrow, got %v", layout)
	}
	if layout.arrows[0].pts != nil {
		t.Fatal("precondition: initial layout arrows should use legacy routing (pts nil)")
	}
	beforeW, beforeH := layout.canvasW, layout.canvasH

	// Drag "orders" far to the right, well past users, then re-route.
	orders := cardByName(layout.cards, "orders")
	orders.x = beforeW + 30
	orders.y = 5
	rerouteArrows(layout)

	if layout.arrows[0].pts == nil {
		t.Fatal("rerouteArrows: arrow pts still nil")
	}
	for _, a := range layout.arrows {
		for _, c := range layout.cards {
			if !polylineAvoids(a.pts, c) {
				t.Errorf("rerouteArrows: arrow %s→%s enters card %q", a.child.name, a.parent.name, c.name)
			}
		}
	}
	if layout.canvasW <= beforeW {
		t.Errorf("rerouteArrows: canvasW=%d should grow past %d after a rightward drag", layout.canvasW, beforeW)
	}
	if layout.canvasH < beforeH {
		t.Errorf("rerouteArrows: canvasH=%d shrank below %d", layout.canvasH, beforeH)
	}
}

// TestERDDragFlow exercises the full mouse drag on the panel: a press on a card
// body, motion to a new spot, and release moves the card and leaves arrows
// re-routed. A press with no motion followed by release is still a click
// (toggles highlight) — drag never steals a click.
func TestERDDragFlow(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	ep := ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders"}
	ep.graph = ep.renderedGraph()
	orders := ep.cardNamed("orders")
	origX, origY := orders.x, orders.y
	grabX, grabY := orders.x+3, orders.y+3 // a body cell

	// Press records a pending drag (no move yet, no highlight toggle).
	ep = ep.dragBeginPress(orders, grabX, grabY)
	if ep.dragPending != "orders" || ep.dragCard != "" {
		t.Fatalf("press: dragPending=%q dragCard=%q want orders/empty", ep.dragPending, ep.dragCard)
	}
	if ep.selected != "" {
		t.Errorf("press toggled highlight prematurely: selected=%q", ep.selected)
	}

	// First motion promotes to an active drag and moves the card.
	var promoted bool
	ep, promoted = ep.dragPromote(grabX+12, grabY+8)
	if !promoted || ep.dragCard != "orders" {
		t.Fatalf("promote: promoted=%v dragCard=%q want true/orders", promoted, ep.dragCard)
	}
	ep = ep.dragMove(grabX+12, grabY+8)
	if orders.x != origX+12 || orders.y != origY+8 {
		t.Errorf("move: card at (%d,%d) want (%d,%d)", orders.x, orders.y, origX+12, origY+8)
	}
	if len(layout.arrows) > 0 && layout.arrows[0].pts == nil {
		t.Error("move: arrows not re-routed to polylines during drag")
	}

	// Release commits and clears drag state; the card stays dropped.
	ep = ep.dragCommit()
	if ep.dragCard != "" || ep.dragPending != "" {
		t.Errorf("commit: dragCard=%q dragPending=%q want empty", ep.dragCard, ep.dragPending)
	}
	if orders.x != origX+12 {
		t.Errorf("commit: card moved from drop position to %d", orders.x)
	}
}

// TestERDDragCancel verifies esc restores a dragged card to its pre-drag spot.
func TestERDDragCancel(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	ep := ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders"}
	ep.graph = ep.renderedGraph()
	orders := ep.cardNamed("orders")
	origX, origY := orders.x, orders.y
	gx, gy := orders.x+3, orders.y+3

	ep = ep.dragBeginPress(orders, gx, gy)
	ep, _ = ep.dragPromote(gx+10, gy+5)
	ep = ep.dragMove(gx+10, gy+5)
	if orders.x == origX {
		t.Fatal("precondition: card did not move during drag")
	}
	ep = ep.dragCancel()
	if orders.x != origX || orders.y != origY {
		t.Errorf("cancel: card at (%d,%d) want original (%d,%d)", orders.x, orders.y, origX, origY)
	}
	if ep.dragCard != "" {
		t.Errorf("cancel: dragCard=%q want empty", ep.dragCard)
	}
}

// TestERDDragViaMouseEvents drives the full press→motion→release sequence
// through handleERDMouse with the Action fields bubbletea actually emits. The
// trap this guards against: left-button drag motion is reported as
// Type=MouseLeft + Action=MouseActionMotion (NOT Type=MouseMotion), so routing
// on Type alone makes every motion re-enter the press handler and the drag
// never starts — the card stays put and the release fires a click instead.
func TestERDDragViaMouseEvents(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders"}}
	m.erdPanel.graph = m.erdPanel.renderedGraph()
	orders := m.erdPanel.cardNamed("orders")
	origX, origY := orders.x, orders.y
	// Press (MouseLeft + Action=Press) records a pending drag, no highlight.
	// Screen coords are derived from the live centring offset (screen = canvas - scroll + off).
	screenOf := func(cx, cy int) (int, int) {
		_, _, ox, oy := m.erdPanel.placedBounds()
		return cx - m.erdPanel.scrollX + ox, cy - m.erdPanel.scrollY + oy
	}
	gx, gy := screenOf(orders.x+3, orders.y+3) // cursor on a body cell
	mm, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, X: gx, Y: gy})
	m = mm.(Model)
	if m.erdPanel.dragPending != "orders" || m.erdPanel.dragCard != "" {
		t.Fatalf("press: dragPending=%q dragCard=%q want orders/empty", m.erdPanel.dragPending, m.erdPanel.dragCard)
	}
	if m.erdPanel.selected != "" {
		t.Errorf("press highlighted prematurely: selected=%q", m.erdPanel.selected)
	}

	// Motion arrives as MouseLeft + Action=Motion — must promote and move.
	mx, my := screenOf(origX+3+15, origY+3+9)
	mm, _ = m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionMotion, X: mx, Y: my})
	m = mm.(Model)
	if m.erdPanel.dragCard != "orders" {
		t.Fatalf("motion: dragCard=%q want orders (drag did not start)", m.erdPanel.dragCard)
	}
	if orders.x != origX+15 || orders.y != origY+9 {
		t.Errorf("motion: card at (%d,%d) want (%d,%d)", orders.x, orders.y, origX+15, origY+9)
	}

	// Release commits; no highlight toggles (it was a drag, not a click).
	mm, _ = m.handleERDMouse(tea.MouseMsg{Type: tea.MouseRelease, Action: tea.MouseActionRelease, X: mx, Y: my})
	m = mm.(Model)
	if m.erdPanel.dragCard != "" {
		t.Errorf("release: dragCard=%q want empty", m.erdPanel.dragCard)
	}
	if m.erdPanel.selected != "" {
		t.Errorf("release: selected=%q want empty (a drag must not toggle highlight)", m.erdPanel.selected)
	}
	if orders.x != origX+15 {
		t.Errorf("release: card moved from drop position to %d", orders.x)
	}
}

// TestERDClickViaMouseEvents confirms a press→release with no motion between
// them still fires the click (toggle highlight), so the drag path doesn't
// swallow plain clicks.
func TestERDClickViaMouseEvents(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders"}}
	m.erdPanel.graph = m.erdPanel.renderedGraph()
	orders := m.erdPanel.cardNamed("orders")
	_, _, offX, offY := m.erdPanel.placedBounds()
	gx, gy := orders.x+3+offX, orders.y+3+offY // screen coords of a body cell (scroll 0)

	mm, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, X: gx, Y: gy})
	mm, _ = mm.(Model).handleERDMouse(tea.MouseMsg{Type: tea.MouseRelease, Action: tea.MouseActionRelease, X: gx, Y: gy})
	m = mm.(Model)
	if m.erdPanel.selected != "orders" {
		t.Errorf("plain click: selected=%q want orders (click should toggle highlight)", m.erdPanel.selected)
	}
}

// TestERDFitToScreen checks the "z" zoom-to-fit: scroll is set so the bounding
// box of all cards is centred in the viewport (clamped to the scrollable
// range), and zeroes when the diagram already fits.
func TestERDFitToScreen(t *testing.T) {
	// Diagram larger than the viewport, cards spread to the corners.
	e := ERDPanel{width: 20, height: 10}
	e.graph = newGcanvas(60, 40) // maxX=40, maxY=30
	e.cards = []*gcard{
		{name: "a", x: 5, y: 5, w: 4, h: 3},
		{name: "b", x: 40, y: 25, w: 4, h: 3},
	}
	// bbox: x[5,44] y[5,28] → centre (24,16); viewport 20×10.
	e = e.fitToScreen()
	wantX, wantY := 14, 16-5 // 24-10, 16-5
	if e.scrollX != wantX || e.scrollY != wantY {
		t.Errorf("fit (large): scroll=(%d,%d) want (%d,%d)", e.scrollX, e.scrollY, wantX, wantY)
	}

	// Diagram smaller than the viewport → scroll zeroes (View centres it).
	e2 := ERDPanel{width: 80, height: 40}
	e2.graph = newGcanvas(20, 10)
	e2.cards = []*gcard{{name: "a", x: 2, y: 2, w: 4, h: 3}}
	e2.scrollX, e2.scrollY = 5, 5
	e2 = e2.fitToScreen()
	if e2.scrollX != 0 || e2.scrollY != 0 {
		t.Errorf("fit (small): scroll=(%d,%d) want (0,0)", e2.scrollX, e2.scrollY)
	}
}

// TestERDFitToScreenKey confirms "zz" dispatches to fitToScreen via Update
// (z is now a prefix: the first z arms it, the second resolves zz → fit).
func TestERDFitToScreenKey(t *testing.T) {
	e := ERDPanel{width: 20, height: 10}
	e.graph = newGcanvas(60, 40)
	e.cards = []*gcard{
		{name: "a", x: 5, y: 5, w: 4, h: 3},
		{name: "b", x: 40, y: 25, w: 4, h: 3},
	}
	e.scrollX, e.scrollY = 0, 0
	zKey := func() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")} }
	// First z arms the prefix without fitting.
	e = e.Update(zKey())
	if !e.zPrefix {
		t.Fatalf("first z: zPrefix=false, want true (prefix armed)")
	}
	if e.scrollX != 0 || e.scrollY != 0 {
		t.Errorf("first z should not fit yet: scroll=(%d,%d)", e.scrollX, e.scrollY)
	}
	// zz resolves to fit-to-screen.
	e = e.Update(zKey())
	if e.zPrefix {
		t.Errorf("zz: zPrefix=true, want false (consumed)")
	}
	if e.scrollX != 14 || e.scrollY != 11 {
		t.Errorf("zz: scroll=(%d,%d) want (14,11)", e.scrollX, e.scrollY)
	}
}

// --- free-form drag: canvas grows up/left of the origin ---------------------

// TestERDDragLeftmostCardLeft is the regression test for the bug where the
// leftmost card (sitting at the canvas origin x=0) could not be dragged left
// while every other card roamed freely. The card's position was clamped to a
// minimum of 0 and the canvas origin was fixed at (0,0), so the leftmost card
// was trapped against the border. Now the card moves into negative logical
// space, the layout's origin shifts to contain it, the canvas grows leftward,
// and render paints it at the new (shifted) cell.
func TestERDDragLeftmostCardLeft(t *testing.T) {
	tables, schemas, pks, fks := erdFixture() // users (rank 0) is the leftmost card
	layout := computeERDLayout(tables, schemas, pks, fks)
	ep := ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "users"}
	ep.graph = ep.renderedGraph()
	users := ep.cardNamed("users")
	if users.x != 0 {
		t.Fatalf("precondition: leftmost users.x=%d want 0", users.x)
	}
	beforeW, beforeOriginX := layout.canvasW, layout.originX

	// Grab a body cell and drag it 5 cells left, past the origin.
	grabX, grabY := users.x+3, users.y+3
	ep = ep.dragBeginPress(users, grabX, grabY)
	ep, promoted := ep.dragPromote(grabX-5, grabY)
	if !promoted {
		t.Fatal("drag did not promote on leftward motion")
	}
	ep = ep.dragMove(grabX-5, grabY)

	// dragOffX = grabX - users.x(0) = 3, so the new origin is (grabX-5) - 3 = -5.
	// Before the fix this clamped to 0 and the card stayed pinned.
	if users.x != -5 {
		t.Errorf("leftmost card x=%d want -5 (should move left past the origin)", users.x)
	}
	// The layout origin shifts left to contain the moved card, and the canvas
	// grows leftward to match — it does not stay pinned at (0,0)/beforeW.
	if layout.originX >= beforeOriginX || layout.originX > 0 {
		t.Errorf("originX=%d should be < %d and <= 0 (canvas must grow left)", layout.originX, beforeOriginX)
	}
	if layout.canvasW <= beforeW {
		t.Errorf("canvasW=%d should grow past %d after a leftward drag", layout.canvasW, beforeW)
	}

	// Re-rendering must not panic on the negative coordinate, and the moved card
	// must still appear (its origin-shifted cells land inside the canvas).
	g := ep.renderedGraph()
	if g == nil {
		t.Fatal("renderedGraph returned nil after a leftward drag")
	}
	if !containsText(g, "users") {
		t.Error("users card not rendered after being dragged left of the origin")
	}
	// The leftmost card's top-left corner now sits at rendered column 0 (logical
	// x == originX), proving the cell-grid shift keeps it on-canvas.
	cornerX, cornerY := users.x-layout.originX, users.y-layout.originY
	if cornerX != 0 {
		t.Errorf("leftmost card rendered x=%d want 0 (origin shift)", cornerX)
	}
	if gr, _, _ := cellGlyph(g.cells[cornerY][cornerX]); gr != '┌' {
		t.Errorf("rendered (%d,%d) = %q want ┌ (card top-left corner)", cornerX, cornerY, gr)
	}

	// Commit keeps the drop; the card stays at its negative coordinate.
	ep = ep.dragCommit()
	if users.x != -5 {
		t.Errorf("after commit users.x=%d want -5", users.x)
	}
}

// TestERDDragTopmostCardUp mirrors the leftward case for the vertical axis: the
// topmost card (y=0) can be dragged up into negative space and the canvas grows
// upward to contain it.
func TestERDDragTopmostCardUp(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	ep := ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24}
	ep.graph = ep.renderedGraph()
	// Pick whichever card sits highest (smallest y) — that's the topmost one.
	top := layout.cards[0]
	for _, c := range layout.cards {
		if c != nil && c.y < top.y {
			top = c
		}
	}
	if top.y != 0 {
		t.Fatalf("precondition: topmost card y=%d want 0", top.y)
	}
	grabX, grabY := top.x+3, top.y+3
	ep = ep.dragBeginPress(top, grabX, grabY)
	ep, _ = ep.dragPromote(grabX, grabY-4)
	ep = ep.dragMove(grabX, grabY-4)
	if top.y >= 0 {
		t.Errorf("topmost card y=%d want < 0 (should move up past the origin)", top.y)
	}
	if layout.originY > 0 {
		t.Errorf("originY=%d should be <= 0 (canvas must grow up)", layout.originY)
	}
	if ep.renderedGraph() == nil {
		t.Fatal("renderedGraph returned nil after an upward drag")
	}
}

// TestERDGcanvasOrigin verifies the drawing primitives shift logical coordinates
// by the canvas origin (ox, oy) before writing cells, so a card routed into
// negative logical space still lands inside the cell grid instead of being
// clipped or wrapping to a negative index.
func TestERDGcanvasOrigin(t *testing.T) {
	c := newGcanvas(3, 3)
	c.ox, c.oy = -2, -1
	// Logical (0,0) maps to rendered (2,1): the origin sits left/up of (0,0),
	// so logical coords shift right/down in the cell grid.
	c.setCh(0, 0, 'A', "", false)
	if gr, _, _ := cellGlyph(c.cells[1][2]); gr != 'A' {
		t.Errorf("logical (0,0) wrote %q at rendered (2,1), want A", gr)
	}
	// The origin cell itself: logical (-2,-1) maps to rendered (0,0).
	c.setCh(-2, -1, 'B', "", false)
	if gr, _, _ := cellGlyph(c.cells[0][0]); gr != 'B' {
		t.Errorf("logical (-2,-1) wrote %q at rendered (0,0), want B", gr)
	}
	// A logical coord below the origin maps before rendered 0 → silently dropped.
	c.setCh(-3, -1, 'X', "", false) // rendered (-1, 0) → out of bounds
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if gr, _, _ := cellGlyph(c.cells[y][x]); gr == 'X' {
				t.Errorf("below-origin logical (-3,-1) wrote X at rendered (%d,%d)", x, y)
			}
		}
	}
	// hline honours the shift: a logical span lands in the shifted columns and
	// nowhere else.
	c2 := newGcanvas(5, 1)
	c2.ox = -2
	c2.hline(0, 2, 0, "") // logical x[0,2] → rendered x[2,4]
	for x := 2; x <= 4; x++ {
		if c2.cells[0][x].con == 0 {
			t.Errorf("hline logical [0,2] missing connection at rendered x=%d", x)
		}
	}
	for x := 0; x < 2; x++ {
		if c2.cells[0][x].con != 0 {
			t.Errorf("hline leaked into rendered x=%d (before the shifted span)", x)
		}
	}
}

// TestERDContentToCanvasUnboundedAllowsNegative confirms the drag-time
// screen→canvas transform no longer clamps to 0: a cursor in the centred left
// margin of a small diagram maps to a negative logical coordinate, which is
// what lets the leftmost card be dragged left of the origin.
func TestERDContentToCanvasUnboundedAllowsNegative(t *testing.T) {
	// A 6×3 canvas centred in a 40×10 viewport: body starts at offX=17.
	e := ERDPanel{width: 40, height: 10}
	e.graph = newGcanvas(6, 3)
	_, _, offX, offY := e.placedBounds()
	if offX <= 0 {
		t.Fatalf("precondition: need a centred margin, got offX=%d", offX)
	}
	// A point in the left margin (sx=0) maps to a negative logical x.
	cx, cy, ok := e.contentToCanvasUnbounded(0, offY)
	if !ok {
		t.Fatal("contentToCanvasUnbounded rejected the left margin during a drag")
	}
	if cx >= 0 {
		t.Errorf("left margin cx=%d want < 0 (transform must not clamp to 0)", cx)
	}
	if cy != 0 {
		t.Errorf("margin row cy=%d want 0", cy)
	}
}

// --- card collapse/expand ("zc"/"zo"/"za") ---------------------------------

// TestERDColRowYCollapsed verifies colRowY falls back to the header-row centre
// for a collapsed card regardless of which column is asked for — the router
// must not assume the column rows exist on a header-only card.
func TestERDColRowYCollapsed(t *testing.T) {
	c := &gcard{name: "t", cols: []db.Column{{Name: "id"}, {Name: "x"}}, y: 10, h: 7, fullH: 7}
	// Expanded: id lives on row y+3 = 13.
	if got := c.colRowY("id"); got != 13 {
		t.Errorf("expanded colRowY(id)=%d want 13", got)
	}
	// Collapsed: h=3, every column (and absent ones) → title centre y+1 = 11.
	c.collapsed = true
	c.h = erdCollapsedH
	for _, col := range []string{"id", "x", "absent"} {
		if got := c.colRowY(col); got != 11 {
			t.Errorf("collapsed colRowY(%q)=%d want 11 (header centre)", col, got)
		}
	}
}

// TestERDCollapseKeys drives the full z-prefix through Update: zc collapses the
// focused card (h→erdCollapsedH, fullH preserved), zo expands it back, and za
// toggles both ways. Each chord consumes the z-prefix.
func TestERDCollapseKeys(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "orders"}
	ep.graph = ep.renderedGraph()
	orders := ep.cardNamed("orders")
	fullH := orders.h
	if orders.collapsed {
		t.Fatal("precondition: orders should start expanded")
	}

	// zc → collapse.
	ep = ep.Update(key("z"))
	ep = ep.Update(key("c"))
	if ep.zPrefix {
		t.Error("zc: zPrefix should be consumed")
	}
	if !orders.collapsed || orders.h != erdCollapsedH {
		t.Errorf("zc: orders collapsed=%v h=%d want true/%d", orders.collapsed, orders.h, erdCollapsedH)
	}
	if orders.fullH != fullH {
		t.Errorf("zc: fullH=%d want %d (preserved for expand)", orders.fullH, fullH)
	}

	// zo → expand back to full height.
	ep = ep.Update(key("z"))
	ep = ep.Update(key("o"))
	if orders.collapsed || orders.h != fullH {
		t.Errorf("zo: orders collapsed=%v h=%d want false/%d", orders.collapsed, orders.h, fullH)
	}

	// za toggles both ways.
	ep = ep.Update(key("z"))
	ep = ep.Update(key("a"))
	if !orders.collapsed || orders.h != erdCollapsedH {
		t.Errorf("za→collapse: orders collapsed=%v h=%d", orders.collapsed, orders.h)
	}
	ep = ep.Update(key("z"))
	ep = ep.Update(key("a"))
	if orders.collapsed || orders.h != fullH {
		t.Errorf("za→expand: orders collapsed=%v h=%d", orders.collapsed, orders.h)
	}
}

// TestERDCollapseNoFocus is a no-op guard: with no focused card, zc/zo/za do
// nothing (and still consume the prefix).
func TestERDCollapseNoFocus(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: ""}
	ep.graph = ep.renderedGraph()
	before := ep.cardNamed("orders").h
	ep = ep.Update(key("z"))
	ep = ep.Update(key("c"))
	if ep.zPrefix {
		t.Error("zc with no focus: zPrefix should still be consumed")
	}
	if h := ep.cardNamed("orders").h; h != before {
		t.Errorf("zc with no focus changed a card height: %d want %d", h, before)
	}
}

// TestERDCollapseRender checks a collapsed card paints header-only: the fold
// marker ▸ at the title's left edge, the bottom border directly under the title
// (no separator row), and no column text.
func TestERDCollapseRender(t *testing.T) {
	schemas := map[string][]db.Column{
		"t": {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}, {Name: "x", Type: "int"}},
	}
	pks := map[string][]string{"t": {"id"}}
	layout := computeERDLayout([]string{"t"}, schemas, pks, nil)
	t1 := cardByName(layout.cards, "t")
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "t"}
	ep.graph = ep.renderedGraph()
	ep = ep.setCollapsed("t", true)
	c := ep.graph
	if t1.h != erdCollapsedH {
		t.Fatalf("setCollapsed: h=%d want %d", t1.h, erdCollapsedH)
	}
	// Fold marker ▸ at the title row's left content cell.
	if g, _, _ := cellGlyph(c.cells[t1.y+1][t1.x+1]); g != '▸' {
		t.Errorf("collapsed title left cell=%q want ▸", g)
	}
	// Row y+2 is the bottom border └, not a ─ separator (no separator row).
	if g, _, _ := cellGlyph(c.cells[t1.y+2][t1.x]); g != '└' {
		t.Errorf("row y+2 left edge=%q want └ (bottom border; no separator)", g)
	}
	// Column text must not appear in the header-only render.
	if containsText(c, "name") {
		t.Error("collapsed card should not render column 'name'")
	}
	if containsText(c, "INT") {
		t.Error("collapsed card should not render any column type")
	}
}

// TestERDCollapseArrowAtHeader verifies that after a card collapses, its FK
// arrow re-routes so the arrowhead lands on the header-row centre (colRowY on a
// collapsed card), proving the router copes with a header-only endpoint.
func TestERDCollapseArrowAtHeader(t *testing.T) {
	tables, schemas, pks, fks := erdFixture() // orders.user_id → users.id (users is the parent, left of orders)
	layout := computeERDLayout(tables, schemas, pks, fks)
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = ep.renderedGraph()
	users := ep.cardNamed("users")
	ep = ep.setCollapsed("users", true)
	py := users.colRowY("id")
	if py != users.y+1 {
		t.Fatalf("collapsed colRowY(id)=%d want %d (title row)", py, users.y+1)
	}
	if len(layout.arrows) != 1 || layout.arrows[0].pts == nil {
		t.Fatal("collapse did not re-route the arrow to a polyline")
	}
	last := layout.arrows[0].pts[len(layout.arrows[0].pts)-1]
	if last.y != py {
		t.Errorf("arrowhead y=%d want %d (collapsed parent header centre)", last.y, py)
	}
}

// TestERDZPrefixFallthrough confirms `z` + an unrecognized key clears the
// prefix and lets the second key act normally (zj still moves focus) instead of
// swallowing it.
func TestERDZPrefixFallthrough(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	orders := cardByName(layout.cards, "orders")
	posts := cardByName(layout.cards, "posts")
	if !(orders.y < posts.y) {
		t.Fatalf("precondition: orders must sit above posts (orders.y=%d posts.y=%d)", orders.y, posts.y)
	}
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "orders"}
	ep.graph = layout.render("", "orders", erdPath{})

	ep = ep.Update(key("z"))
	if !ep.zPrefix {
		t.Fatal("z did not arm the prefix")
	}
	ep = ep.Update(key("j"))
	if ep.zPrefix {
		t.Error("zj: zPrefix should be cleared after fallthrough")
	}
	if ep.focusName != "posts" {
		t.Errorf("zj: focus=%q want posts (j must fall through, not be eaten)", ep.focusName)
	}
}

// TestERDCollapseThenDrag checks collapse composes with free-form drag:
// collapsing a card and then dragging it moves the (still-folded) card, keeps
// its collapsed state, and re-routes arrows around the new box.
func TestERDCollapseThenDrag(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	ep := ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders"}
	ep.graph = ep.renderedGraph()
	orders := ep.cardNamed("orders")
	origX, origY := orders.x, orders.y

	// Fold first.
	ep = ep.setCollapsed("orders", true)
	if !orders.collapsed {
		t.Fatal("precondition: orders not collapsed")
	}
	// Drag the folded card by a title-row cell.
	grabX, grabY := orders.x+1, orders.y+1
	ep = ep.dragBeginPress(orders, grabX, grabY)
	ep, promoted := ep.dragPromote(grabX+8, grabY+4)
	if !promoted {
		t.Fatal("drag did not promote on a collapsed card")
	}
	ep = ep.dragMove(grabX+8, grabY+4)
	if orders.x != origX+8 || orders.y != origY+4 {
		t.Errorf("drag move: card at (%d,%d) want (%d,%d)", orders.x, orders.y, origX+8, origY+4)
	}
	if !orders.collapsed {
		t.Error("drag lost the collapsed state")
	}
	if len(layout.arrows) > 0 && layout.arrows[0].pts == nil {
		t.Error("drag after collapse: arrows not re-routed to polylines")
	}
	// Commit keeps both the drop and the fold.
	ep = ep.dragCommit()
	if orders.x != origX+8 || !orders.collapsed {
		t.Errorf("commit: card x=%d collapsed=%v want %d/true", orders.x, orders.collapsed, origX+8)
	}
}

// TestERDCollapseAllKeys drives zM/zR through Update: zM folds every card (each
// h→erdCollapsedH, fullH preserved) and re-routes every arrow; zR unfolds them
// all back to full height. Both consume the z-prefix.
func TestERDCollapseAllKeys(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	fullH := map[string]int{}
	for _, c := range layout.cards {
		fullH[c.name] = c.h
	}
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = ep.renderedGraph()

	// zM collapses every card; arrows become dynamic polylines routed around the
	// folded boxes.
	ep = ep.Update(key("z"))
	ep = ep.Update(key("M"))
	if ep.zPrefix {
		t.Error("zM: zPrefix should be consumed")
	}
	for _, c := range layout.cards {
		if !c.collapsed || c.h != erdCollapsedH {
			t.Errorf("zM: %s collapsed=%v h=%d want true/%d", c.name, c.collapsed, c.h, erdCollapsedH)
		}
		if c.fullH != fullH[c.name] {
			t.Errorf("zM: %s fullH=%d want %d (preserved)", c.name, c.fullH, fullH[c.name])
		}
	}
	for i := range layout.arrows {
		if layout.arrows[i].pts == nil {
			t.Errorf("zM: arrow %d not re-routed to a polyline", i)
		}
	}

	// zR expands every card back to its full height.
	ep = ep.Update(key("z"))
	ep = ep.Update(key("R"))
	if ep.zPrefix {
		t.Error("zR: zPrefix should be consumed")
	}
	for _, c := range layout.cards {
		if c.collapsed || c.h != fullH[c.name] {
			t.Errorf("zR: %s collapsed=%v h=%d want false/%d", c.name, c.collapsed, c.h, fullH[c.name])
		}
	}
}

// TestERDCollapseAllMixedState confirms zM/zR are absolute (not toggles): zM
// folds every card even if some were already folded, and zR unfolds every card
// even if some were already open.
func TestERDCollapseAllMixedState(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	users := cardByName(layout.cards, "users")
	orders := cardByName(layout.cards, "orders")
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "orders"}
	ep.graph = ep.renderedGraph()

	// Start mixed: users folded, orders open.
	ep = ep.setCollapsed("users", true)
	if !users.collapsed || orders.collapsed {
		t.Fatal("precondition: mixed state not set")
	}

	// zM folds all → both folded.
	ep = ep.Update(key("z"))
	ep = ep.Update(key("M"))
	if !users.collapsed || !orders.collapsed {
		t.Errorf("zM from mixed: users=%v orders=%v want both folded", users.collapsed, orders.collapsed)
	}

	// Fold one back manually, then zR unfolds all → both open.
	ep = ep.setCollapsed("orders", true)
	ep = ep.Update(key("z"))
	ep = ep.Update(key("R"))
	if users.collapsed || orders.collapsed {
		t.Errorf("zR from mixed: users=%v orders=%v want both open", users.collapsed, orders.collapsed)
	}
}

// TestERDCollapseAllReframes checks the bulk fold reframes the viewport: after
// zM the (now much shorter) diagram is fit to the viewport, so it doesn't sit
// shrunken in a corner with stale scroll offsets.
func TestERDCollapseAllReframes(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}, {Name: "body", Type: "text"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	// A tall diagram in a short viewport so the bulk fold shrinks it to fit.
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 30, height: 5, focusName: "users"}
	ep.graph = ep.renderedGraph()
	// Scroll to the bottom before folding, so a stale offset would misframe.
	ep.scrollY = 999
	ep = ep.setAllCollapsed(true)
	// After folding + reframe the diagram (3 short cards) fits a 5-row viewport,
	// so the vertical scroll zeroes (View centres a diagram that fits).
	if ep.graph != nil && ep.graph.h <= ep.contentHeight() && ep.scrollY != 0 {
		t.Errorf("zM reframe: folded diagram fits viewport but scrollY=%d want 0", ep.scrollY)
	}
}

// TestERDCollapseAllContracts verifies zM contracts the layout: cards in the
// same rank column are re-stacked tightly (a 1-row gap, not the gap the folded
// bodies left behind) and the canvas shrinks. This is the "reclaim the space"
// behaviour — zM re-runs the ranked layout on the new card sizes.
func TestERDCollapseAllContracts(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}, {Name: "body", Type: "text"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	orders := cardByName(layout.cards, "orders")
	posts := cardByName(layout.cards, "posts")
	// orders & posts share rank 1; orders sorts above posts.
	if orders.rank != posts.rank {
		t.Fatalf("precondition: orders.rank=%d posts.rank=%d want equal", orders.rank, posts.rank)
	}
	if !(orders.y < posts.y) {
		t.Fatalf("precondition: orders (%d) should sit above posts (%d)", orders.y, posts.y)
	}
	expandedH := layout.canvasH
	// Expanded, the gap between them is orders.h + 1 (a 1-row gutter), but
	// orders.h is the full card height — so the bodies are tight already in the
	// fresh layout. The point of zM is that after folding, they STAY tight
	// (the column re-stacks) rather than leaving orders' old body as a gap.
	expandedGap := posts.y - orders.y // == orders.h + 1
	if expandedGap != orders.h+1 {
		t.Fatalf("precondition: expanded gap=%d want %d (orders.h+1)", expandedGap, orders.h+1)
	}

	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "users"}
	ep.graph = ep.renderedGraph()
	ep = ep.Update(key("z"))
	ep = ep.Update(key("M"))

	// Both folded, and re-stacked tightly: posts sits one row below orders'
	// (now 3-row) body, not one row below orders' old full body.
	if orders.h != erdCollapsedH || posts.h != erdCollapsedH {
		t.Fatalf("zM: orders.h=%d posts.h=%d want %d", orders.h, posts.h, erdCollapsedH)
	}
	if got := posts.y - orders.y; got != orders.h+1 {
		t.Errorf("zM did not contract: gap orders→posts=%d want %d (orders.h+1, tight stack)", got, orders.h+1)
	}
	if layout.canvasH >= expandedH {
		t.Errorf("zM: canvasH=%d should be < expanded %d (layout did not contract)", layout.canvasH, expandedH)
	}
}

// TestERDCollapseSticky confirms the per-card zc is an in-place shrink (the
// layout does NOT contract): a card folded with zc leaves its old body as a
// gap, so cards below it stay put — keeping fold composable with free-form
// drag. (zM/zR are the contracting operations; zc/zo/za are not.)
func TestERDCollapseSticky(t *testing.T) {
	tables := []string{"users", "orders", "posts"}
	schemas := map[string][]db.Column{
		"users":  {{Name: "id", Type: "int"}, {Name: "name", Type: "text"}},
		"orders": {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}},
		"posts":  {{Name: "id", Type: "int"}, {Name: "user_id", Type: "int"}, {Name: "body", Type: "text"}},
	}
	pks := map[string][]string{"users": {"id"}, "orders": {"id"}, "posts": {"id"}}
	fks := map[string][]db.ForeignKey{
		"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		"posts":  {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
	}
	layout := computeERDLayout(tables, schemas, pks, fks)
	layout.focus = ""
	orders := cardByName(layout.cards, "orders")
	posts := cardByName(layout.cards, "posts")
	users := cardByName(layout.cards, "users")
	ordersY, postsY, usersY := orders.y, posts.y, users.y

	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	ep := ERDPanel{cards: layout.cards, layout: layout, width: 80, height: 24, focusName: "orders"}
	ep.graph = ep.renderedGraph()
	ep = ep.Update(key("z"))
	ep = ep.Update(key("c"))

	if !orders.collapsed || orders.h != erdCollapsedH {
		t.Fatalf("zc: orders collapsed=%v h=%d want true/%d", orders.collapsed, orders.h, erdCollapsedH)
	}
	// Sticky: no card moved (the folded body is left as a gap, not reclaimed).
	if orders.y != ordersY {
		t.Errorf("zc moved orders: y=%d want %d (sticky)", orders.y, ordersY)
	}
	if posts.y != postsY {
		t.Errorf("zc moved posts: y=%d want %d (sticky — should not contract)", posts.y, postsY)
	}
	if users.y != usersY {
		t.Errorf("zc moved users: y=%d want %d (sticky)", users.y, usersY)
	}
}

// TestERDHoverViaMouseEvents pins the button-less-motion gotcha: hover events
// arrive as Type=MouseMotion + Action=MouseActionMotion (NOT Type=MouseLeft,
// which is a left-drag), and handleERDMouse routes them to the hover path that
// sets hoverCard to the table under the cursor (or "" over empty space). See
// docs/tui-mouse.md — this is the hover counterpart of TestERDDragViaMouseEvents.
func TestERDHoverViaMouseEvents(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders"}}
	m.erdPanel.graph = m.erdPanel.renderedGraph()
	orders := m.erdPanel.cardNamed("orders")
	users := m.erdPanel.cardNamed("users")

	screenOf := func(cx, cy int) (int, int) { return m.erdPanel.canvasToScreen(cx, cy) }

	// Button-less motion over a body cell of "orders" → hoverCard set.
	sx, sy := screenOf(orders.x+3, orders.y+4)
	mm, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseMotion, Action: tea.MouseActionMotion, X: sx, Y: sy})
	m = mm.(Model)
	if m.erdPanel.hoverCard != "orders" {
		t.Fatalf("hover over orders: hoverCard=%q want orders", m.erdPanel.hoverCard)
	}

	// Motion over a different card updates hoverCard (not stuck on the first).
	ux, uy := screenOf(users.x+2, users.y+3)
	mm, _ = m.handleERDMouse(tea.MouseMsg{Type: tea.MouseMotion, Action: tea.MouseActionMotion, X: ux, Y: uy})
	m = mm.(Model)
	if m.erdPanel.hoverCard != "users" {
		t.Fatalf("hover over users: hoverCard=%q want users", m.erdPanel.hoverCard)
	}

	// Motion over empty space (content corner sits in the centred margin) → cleared.
	mm, _ = m.handleERDMouse(tea.MouseMsg{Type: tea.MouseMotion, Action: tea.MouseActionMotion, X: 0, Y: 0})
	m = mm.(Model)
	if m.erdPanel.hoverCard != "" {
		t.Fatalf("hover over empty space: hoverCard=%q want empty", m.erdPanel.hoverCard)
	}

	// A left-drag motion (Type=MouseLeft, the gotcha) must NOT be treated as
	// hover: it carries a held button and belongs to the drag path. With no
	// press pending it should leave hoverCard untouched (empty here).
	dx, dy := screenOf(orders.x+3, orders.y+4)
	mm, _ = m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionMotion, X: dx, Y: dy})
	m = mm.(Model)
	if m.erdPanel.hoverCard != "" {
		t.Errorf("left-drag motion leaked into hover: hoverCard=%q want empty", m.erdPanel.hoverCard)
	}
	if m.erdPanel.dragPending != "" || m.erdPanel.dragCard != "" {
		t.Errorf("left-drag motion with no press started a drag: pending=%q card=%q", m.erdPanel.dragPending, m.erdPanel.dragCard)
	}
}

// TestERDHoverIgnoredInMermaid confirms hover only applies to the graph view:
// button-less motion while the Mermaid source is showing leaves hoverCard empty.
func TestERDHoverIgnoredInMermaid(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders", merm: true}}
	m.erdPanel.graph = m.erdPanel.renderedGraph()
	orders := m.erdPanel.cardNamed("orders")
	sx, sy := m.erdPanel.canvasToScreen(orders.x+3, orders.y+4)
	mm, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseMotion, Action: tea.MouseActionMotion, X: sx, Y: sy})
	m = mm.(Model)
	if m.erdPanel.hoverCard != "" {
		t.Fatalf("mermaid view set hoverCard=%q want empty", m.erdPanel.hoverCard)
	}
}

// TestERDTooltipTextExpandedShowsFKRefsOnly confirms the tooltip never
// repeats what the card already paints: an expanded card lists only its FK
// references (col → refTable.refCol) — the one detail absent from the card —
// and omits its non-FK columns entirely.
func TestERDTooltipTextExpandedShowsFKRefsOnly(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24}}
	orders := m.erdPanel.cardNamed("orders") // expanded, FK: user_id → users.id
	out := ansi.Strip(m.erdPanel.tooltipText(orders))
	for _, want := range []string{
		"orders",   // title
		"◇",        // FK marker
		"user_id",  // FK column
		"→",        // reference arrow
		"users.id", // target (NOT shown on the card)
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expanded tooltip missing %q:\n%s", want, out)
		}
	}
	// Non-redundancy: a non-FK column already visible on the card must NOT be
	// re-listed in the tooltip. total/id are orders columns but not FKs.
	for _, redundant := range []string{"total", "INT", "REAL"} {
		if strings.Contains(out, redundant) {
			t.Errorf("expanded tooltip redundantly shows %q (already on the card):\n%s", redundant, out)
		}
	}
}

// TestERDTooltipTextExpandedNoFKsIsEmpty: an expanded card with no FK
// references has nothing to add, so the tooltip is suppressed ("").
func TestERDTooltipTextExpandedNoFKsIsEmpty(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24}}
	users := m.erdPanel.cardNamed("users") // expanded, no outbound FK
	if out := m.erdPanel.tooltipText(users); out != "" {
		t.Errorf("expanded no-FK tooltip should be empty, got:\n%s", ansi.Strip(out))
	}
}

// TestERDTooltipTextCollapsedRevealsColumns: a collapsed card hides its
// columns, so the tooltip reveals them (marker + name + type) and annotates
// each FK column with its target. This is the non-redundant case — the info is
// genuinely hidden by the fold.
func TestERDTooltipTextCollapsedRevealsColumns(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24}}
	orders := m.erdPanel.cardNamed("orders")
	orders.collapsed = true
	out := ansi.Strip(m.erdPanel.tooltipText(orders))
	for _, want := range []string{
		"orders",  // title
		"◆", "id", // PK column revealed
		"total", "REAL", // non-FK column + type revealed
		"◇", "user_id", "→", "users.id", // FK column + its target
	} {
		if !strings.Contains(out, want) {
			t.Errorf("collapsed reveal missing %q:\n%s", want, out)
		}
	}
}

// TestERDHoverTooltipRevealsCollapsedCard is the integration test for the
// overlay: a collapsed card hides its columns (header-only), so the columns
// appear in View() ONLY when the tooltip overlays them on hover.
func TestERDHoverTooltipRevealsCollapsedCard(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders"}}
	m.erdPanel.graph = m.erdPanel.renderedGraph()
	// Collapse the focused card via the zc key sequence (mirrors TestERDZCCollapse).
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	m.erdPanel = m.erdPanel.Update(key("z"))
	m.erdPanel = m.erdPanel.Update(key("c"))
	orders := m.erdPanel.cardNamed("orders")
	if !orders.collapsed {
		t.Fatalf("setup: orders not collapsed")
	}

	// Without hover: a collapsed card renders no column rows, so its FK column
	// (unique to orders) is absent from the whole view.
	if bare := ansi.Strip(m.erdPanel.View()); strings.Contains(bare, "user_id") {
		t.Errorf("collapsed orders leaked its columns without hover")
	}

	// Hover the collapsed card's header and the tooltip reveals the columns.
	m.erdPanel.hoverCard = "orders" // simulate a motion event landing on the header
	if hovered := ansi.Strip(m.erdPanel.View()); !strings.Contains(hovered, "user_id") || !strings.Contains(hovered, "total") {
		t.Errorf("hover did not reveal collapsed card's columns:\n%s", hovered)
	}
}

// TestERDHoverClearedOnInput confirms a tooltip never lingers after the diagram
// moves under a static cursor: any key clears hoverCard (focus move / fold /
// scroll / mermaid toggle), as does the wheel and a drag promotion.
func TestERDHoverClearedOnInput(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	layout := computeERDLayout(tables, schemas, pks, fks)
	newModel := func() Model {
		m := Model{erdPanel: ERDPanel{layout: layout, cards: layout.cards, width: 80, height: 24, focusName: "orders"}}
		m.erdPanel.graph = m.erdPanel.renderedGraph()
		m.erdPanel.hoverCard = "orders"
		return m
	}
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

	t.Run("key", func(t *testing.T) {
		m := newModel()
		m.erdPanel = m.erdPanel.Update(key("j")) // move focus
		if m.erdPanel.hoverCard != "" {
			t.Errorf("key did not clear hoverCard: %q", m.erdPanel.hoverCard)
		}
	})
	t.Run("mermaid-toggle", func(t *testing.T) {
		m := newModel()
		m.erdPanel = m.erdPanel.Update(key("m"))
		if m.erdPanel.hoverCard != "" {
			t.Errorf("mermaid toggle did not clear hoverCard: %q", m.erdPanel.hoverCard)
		}
	})
	t.Run("wheel", func(t *testing.T) {
		m := newModel()
		m.erdPanel = m.erdPanel.Wheel(1, 0)
		if m.erdPanel.hoverCard != "" {
			t.Errorf("wheel did not clear hoverCard: %q", m.erdPanel.hoverCard)
		}
	})
	t.Run("drag-promote", func(t *testing.T) {
		m := newModel()
		orders := m.erdPanel.cardNamed("orders")
		// Press on a body cell, then a motion far enough to promote to a drag.
		sx, sy := m.erdPanel.canvasToScreen(orders.x+3, orders.y+4)
		mm, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, X: sx, Y: sy})
		m = mm.(Model)
		mx, my := m.erdPanel.canvasToScreen(orders.x+3+15, orders.y+4+9)
		mm, _ = m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionMotion, X: mx, Y: my})
		m = mm.(Model)
		if m.erdPanel.dragCard != "orders" {
			t.Fatalf("drag did not promote: dragCard=%q", m.erdPanel.dragCard)
		}
		if m.erdPanel.hoverCard != "" {
			t.Errorf("drag promote did not clear hoverCard: %q", m.erdPanel.hoverCard)
		}
	})
}

func TestERDCrossHighlightPath(t *testing.T) {
	tables, schemas, pks, fks := erdFixture()
	m := Model{
		connection:  &db.Connection{},
		tables:      tables,
		columnCache: schemas,
		pkCache:     pks,
		fkCache:     fks,
		width:       80,
		height:      24,
		results:     NewResultsTable(),
		editor:      NewQueryEditor(),
	}
	m.results.SetResult([]string{"id", "user_id", "total"}, [][]string{{"1", "2", "9"}}, "")
	m.results.SetEditable("orders", []string{"id"})
	m.results.SetForeignKeys("orders", []db.ForeignKey{{Column: "user_id", RefTable: "users", RefColumn: "id"}})
	m.results.SetCursor(0, 1) // user_id

	m.openERD("")
	if len(m.erdPanel.pathCards) < 2 {
		t.Fatalf("pathCards=%v, want a path from orders to users", m.erdPanel.pathCards)
	}
	if m.erdPanel.focusName != "users" {
		t.Errorf("focus=%q, want users (FK target)", m.erdPanel.focusName)
	}
}
