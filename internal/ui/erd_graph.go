package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
)

// This file renders a graphical ERD — bordered table cards laid out in
// dependency-ranked columns with box-drawing arrows connecting each foreign
// key to the primary key it references. It is the in-terminal "picture" that
// complements the copy-pasteable Mermaid source (toggle with `m` in the panel).
//
// Rendering is layered: (1) arrows are recorded as connection masks on a rune
// canvas, (2) cards (border + columns) are painted over the cells they occupy,
// (3) arrowheads are painted into the gutters/margins. A cell's final glyph is
// its arrowhead if set, else the connection-derived line glyph, else the card
// text, else space — so cards and arrow routes never fight over a cell.

// --- connection masks (for box-merging line segments) ----------------------

type erdDir uint8

const (
	erdUp erdDir = 1 << iota
	erdDown
	erdLeft
	erdRight
)

// connGlyph maps a connection mask (the directions a line continues from a
// cell) to the matching box-drawing rune.
func connGlyph(d erdDir) rune {
	switch d & 15 {
	case erdLeft | erdRight:
		return '─'
	case erdUp | erdDown:
		return '│'
	case erdDown | erdRight:
		return '┌'
	case erdDown | erdLeft:
		return '┐'
	case erdUp | erdRight:
		return '└'
	case erdUp | erdLeft:
		return '┘'
	case erdUp | erdDown | erdRight:
		return '├'
	case erdUp | erdDown | erdLeft:
		return '┤'
	case erdDown | erdLeft | erdRight:
		return '┬'
	case erdUp | erdLeft | erdRight:
		return '┴'
	case erdUp | erdDown | erdLeft | erdRight:
		return '┼'
	case erdLeft, erdRight:
		return '─'
	case erdUp, erdDown:
		return '│'
	}
	return '─'
}

// --- canvas -----------------------------------------------------------------

type gcell struct {
	ch   rune // explicit glyph (card text/border or arrowhead); 0 = none
	fg   string
	bold bool
	bg   string // optional cell background (empty = terminal default)
	con  erdDir // line connections
}

type gcanvas struct {
	w, h  int
	cells [][]gcell
}

func newGcanvas(w, h int) *gcanvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	c := &gcanvas{w: w, h: h, cells: make([][]gcell, h)}
	for y := range c.cells {
		c.cells[y] = make([]gcell, w)
	}
	return c
}

func (c *gcanvas) inBounds(x, y int) bool { return x >= 0 && y >= 0 && x < c.w && y < c.h }

func (c *gcanvas) setCh(x, y int, r rune, fg string, bold bool) {
	if !c.inBounds(x, y) {
		return
	}
	c.cells[y][x].ch = r
	c.cells[y][x].fg = fg
	c.cells[y][x].bold = bold
}

func (c *gcanvas) putText(x, y int, s, fg string, bold bool) {
	off := 0
	for _, r := range s {
		c.setCh(x+off, y, r, fg, bold)
		off++
	}
}

// hline/vline mark connection masks (and a colour) for orthogonal segments;
// the glyph is derived at emit time so overlapping segments merge into tees/crosses.
// Colour precedence is last-writer-wins: render() paints the dimmed (grey)
// arrows before the highlighted (blue) ones, so where two arrows share a cell
// the highlighted connector's colour ends up on top instead of being buried
// under a crossing grey line.
// fillBg paints a background colour across [x1,x2] on row y without touching
// any cell's glyph/fg — used for subtle row tints like the card header. Safe to
// call before drawing text/borders: setCh/putText preserve an existing bg.
func (c *gcanvas) fillBg(x1, x2, y int, bg string) {
	for x := x1; x <= x2; x++ {
		if c.inBounds(x, y) {
			c.cells[y][x].bg = bg
		}
	}
}

func (c *gcanvas) hline(x1, x2, y int, fg string) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	for x := x1; x <= x2; x++ {
		if !c.inBounds(x, y) {
			continue
		}
		if x > x1 {
			c.cells[y][x].con |= erdLeft
		}
		if x < x2 {
			c.cells[y][x].con |= erdRight
		}
		c.cells[y][x].fg = fg
	}
}

func (c *gcanvas) vline(x, y1, y2 int, fg string) {
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	for y := y1; y <= y2; y++ {
		if !c.inBounds(x, y) {
			continue
		}
		if y > y1 {
			c.cells[y][x].con |= erdUp
		}
		if y < y2 {
			c.cells[y][x].con |= erdDown
		}
		c.cells[y][x].fg = fg
	}
}

// addConn ORs a connection direction into a cell's line mask, bending a
// straight segment into an elbow (or tee) where routes meet. Used to turn an
// arrow's vertical run into the corner that feeds its arrowhead, so the line
// meets the triangle's base at a 90° angle instead of a dangling stub.
func (c *gcanvas) addConn(x, y int, d erdDir) {
	if c.inBounds(x, y) {
		c.cells[y][x].con |= d
	}
}

// String emits the canvas, grouping each row into maximal runs of equal
// (fg, bold) so styling is applied per-run rather than per-cell. Grouping is
// by style, not glyph, so a styled title like " users " renders as one span.
// String emits the whole canvas. See Window for the sub-rectangle variant
// used by the scrollable panel.
func (c *gcanvas) String() string {
	return c.Window(0, c.w, 0, c.h)
}

// Window emits cells in the rectangle [x0,x0+w)×[y0,y0+h), clamped to the
// canvas. Each row is grouped into maximal runs of equal (fg, bold) so styling
// is applied per-run rather than per-cell, and wide diagrams can be clipped to
// the panel without breaking ANSI escapes (the panel renders cells directly).
func (c *gcanvas) Window(x0, w, y0, h int) string {
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	x1 := x0 + w
	if x1 > c.w {
		x1 = c.w
	}
	y1 := y0 + h
	if y1 > c.h {
		y1 = c.h
	}
	var b strings.Builder
	for y := y0; y < y1; y++ {
		b.WriteString(c.emitRow(y, x0, x1))
		if y < y1-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// emitRow renders one canvas row over [x0,x1) as styled, run-grouped text.
func (c *gcanvas) emitRow(y, x0, x1 int) string {
	row := c.cells[y]
	var b strings.Builder
	for x := x0; x < x1; {
		_, fg, bold := cellGlyph(row[x])
		bg := row[x].bg
		end := x + 1
		for end < x1 {
			_, efg, ebold := cellGlyph(row[end])
			if efg != fg || ebold != bold || row[end].bg != bg {
				break
			}
			end++
		}
		runes := make([]rune, 0, end-x)
		for i := x; i < end; i++ {
			g, _, _ := cellGlyph(row[i])
			runes = append(runes, g)
		}
		seg := string(runes)
		if fg != "" || bold || bg != "" {
			st := lipgloss.NewStyle()
			if fg != "" {
				st = st.Foreground(lipgloss.Color(fg))
			}
			if bold {
				st = st.Bold(true)
			}
			if bg != "" {
				st = st.Background(lipgloss.Color(bg))
			}
			seg = st.Render(seg)
		}
		b.WriteString(seg)
		x = end
	}
	return b.String()
}

// cellGlyph resolves a cell to (rune, fg, bold): explicit glyph wins, else a
// connection-derived line glyph, else space.
func cellGlyph(c gcell) (rune, string, bool) {
	if c.ch != 0 {
		return c.ch, c.fg, c.bold
	}
	if c.con != 0 {
		return connGlyph(c.con), c.fg, false
	}
	return ' ', c.fg, c.bold
}

// --- cards ------------------------------------------------------------------

type gcard struct {
	name  string
	cols  []db.Column
	pkSet map[string]bool
	fkSet map[string]bool
	rank  int
	x, y  int
	w, h  int
}

// colRowY returns the canvas y of a column's text row, or the card's vertical
// centre if the column is absent. Layout: row 0 = top border, row 1 = title,
// row 2 = separator, rows 3.. = columns.
func (c *gcard) colRowY(col string) int {
	for i, cc := range c.cols {
		if cc.Name == col {
			return c.y + 3 + i
		}
	}
	return c.y + c.h/2
}

// firstFK returns the first FK column name (for arrow endpoint alignment).
func (c *gcard) firstFK() string {
	for _, cc := range c.cols {
		if c.fkSet[cc.Name] {
			return cc.Name
		}
	}
	return ""
}

func (c *gcard) firstPK() string {
	for _, cc := range c.cols {
		if c.pkSet[cc.Name] {
			return cc.Name
		}
	}
	if len(c.cols) > 0 {
		return c.cols[0].Name
	}
	return ""
}

const erdCardRightPad = 1   // space between the type and the right border
const erdFocusIcon = '◎'    // header glyph: click to re-focus the ERD on this table

// measureCard computes the card's width/height from its columns. Each column
// row is laid out as: marker + space + name  …gap…  type + right-pad, with the
// name left-aligned and the type right-aligned one cell inside the right
// border, so names and types line up across rows. The card width fits the
// widest such row.
func measureCard(name string, cols []db.Column, pkSet, fkSet map[string]bool) gcard {
	const minGap = 2                  // at least two spaces between the name and the type
	contentW := len([]rune(name)) + 2 // " name " in the title row
	for _, cc := range cols {
		need := 1 + 1 + len([]rune(cc.Name)) + minGap + len([]rune(erdType(cc.Type))) + erdCardRightPad
		if need > contentW {
			contentW = need
		}
	}
	w := contentW + 2 // left + right borders
	if w < 10 {
		w = 10
	}
	h := len(cols) + 4 // top border + title + separator + columns + bottom border
	return gcard{name: name, cols: cols, pkSet: pkSet, fkSet: fkSet, w: w, h: h}
}

// --- layout -----------------------------------------------------------------

// erdRanks assigns each table a left-to-right column rank: a table's rank is
// one more than the highest rank of the tables it references (its FK parents),
// so referenced (parent/one-side) tables land left of their dependents. Edges
// that would form a cycle are ignored (relaxation is capped at len(tables)
// passes). Returns the rank map and the max rank.
func erdRanks(tables []string, fks map[string][]db.ForeignKey) (map[string]int, int) {
	in := map[string]bool{}
	for _, t := range tables {
		in[t] = true
	}
	rank := map[string]int{}
	for pass := 0; pass < len(tables)+1; pass++ {
		changed := false
		for _, child := range tables {
			for _, fk := range fks[child] {
				if !in[fk.RefTable] {
					continue
				}
				want := rank[fk.RefTable] + 1
				if want > rank[child] {
					rank[child] = want
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	maxR := 0
	for _, r := range rank {
		if r > maxR {
			maxR = r
		}
	}
	return rank, maxR
}

// placeCards assigns (x,y) positions to cards in dependency-ranked columns,
// centring each column's stack vertically beneath topMargin (lanes reserved
// for over-the-top arrow routing). Returns the canvas dimensions.
func placeCards(cards map[string]*gcard, order []string, rank map[string]int, maxRank, topMargin int) (canvasW, canvasH int) {
	const colGap = 4
	byRank := make([][]string, maxRank+1)
	for _, t := range order {
		byRank[rank[t]] = append(byRank[rank[t]], t)
	}
	for _, col := range byRank {
		sort.Strings(col)
	}

	colX := make([]int, maxRank+1)
	colMaxW := make([]int, maxRank+1)
	x := 0
	for r := 0; r <= maxRank; r++ {
		colX[r] = x
		w, h := 0, 0
		for _, t := range byRank[r] {
			c := cards[t]
			if c.w > w {
				w = c.w
			}
			h += c.h + 1 // 1-row gap between stacked cards
		}
		colMaxW[r] = w
		if h > canvasH {
			canvasH = h
		}
		x += w + colGap
	}
	canvasW = x
	canvasH += topMargin

	// Place cards: each column centred vertically beneath the top margin.
	for r := 0; r <= maxRank; r++ {
		stackH := 0
		for _, t := range byRank[r] {
			stackH += cards[t].h + 1
		}
		y := topMargin + (canvasH-topMargin-stackH)/2
		if y < topMargin {
			y = topMargin
		}
		for _, t := range byRank[r] {
			c := cards[t]
			c.x = colX[r]
			c.w = colMaxW[r] // uniform width per column: right edges align on the gutter
			c.y = y
			c.rank = r
			y += c.h + 1
		}
	}
	return canvasW, canvasH
}

// --- drawing ----------------------------------------------------------------
// Card borders/title use the primary colour, arrows the unfocused-border grey,
// and column types the muted colour. When a card is selected the other cards
// are dimmed to the arrow grey (border, title, names, markers, types) so the
// selection and its blue FK arrows stand out; the connected columns on those
// dimmed cards (the arrow endpoints) stay readable — name in the foreground,
// type in primary. These reference the palette vars (set by applyPalette) at
// draw time, so they pick up theme changes.

// drawCard paints a card's border, title, separator, and columns. fg is the
// border/title colour (primary). When dim is set the whole card — border,
// title, separator, names, markers, and types — is drawn in the arrow grey so
// a non-selected card fades out behind the focused one; columns listed in
// hlCols (the arrow endpoints touching the selection) override the dim, with
// marker/name in the foreground and type in primary.
func (c *gcanvas) drawCard(g *gcard, fg string, dim bool, hlCols map[string]bool, showIcon, reserveIcon bool) {
	grey := string(colorBorderUnfocused)
	border, text, sep, typeC := fg, "", string(colorMuted), string(colorMuted)
	if dim {
		border, text, sep, typeC = grey, grey, grey, grey
	}
	// Top border (plain — the title sits on its own row below it).
	c.setCh(g.x, g.y, '╭', border, false)
	c.setCh(g.x+g.w-1, g.y, '╮', border, false)
	for x := g.x + 1; x < g.x+g.w-1; x++ {
		c.setCh(x, g.y, '─', border, false)
	}
	// Title row: a subtle background tint fills the row between the borders,
	// with the table name centred on top (bold/primary).
	c.fillBg(g.x+1, g.x+g.w-2, g.y+1, string(colorStripe))
	c.setCh(g.x, g.y+1, '│', border, false)
	c.setCh(g.x+g.w-1, g.y+1, '│', border, false)
	nameW := len([]rune(g.name))
	// The drill-in icon sits one cell inside the right border with a gap.
	// reserveIcon keeps that column clear on every card in a focused ERD —
	// even the root, which draws no glyph — so titles stay aligned across a
	// column; the centred name then never reaches the icon's cell.
	usable := g.w - 2
	if reserveIcon {
		usable = g.w - 4
	}
	leftPad := (usable - nameW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	c.putText(g.x+1+leftPad, g.y+1, g.name, border, true)
	// Drill-in cue at the header's right end. The whole header row is the click
	// target (see ERDPanel.drillInCard); this glyph just signals the action.
	if showIcon {
		c.setCh(g.x+g.w-3, g.y+1, erdFocusIcon, string(colorFg), false)
	}
	// Title→columns separator (dimmed so it reads as a divider, not a border).
	c.setCh(g.x, g.y+2, '│', border, false)
	c.setCh(g.x+g.w-1, g.y+2, '│', border, false)
	for x := g.x + 1; x < g.x+g.w-1; x++ {
		c.setCh(x, g.y+2, '─', sep, false)
	}
	// Column rows: marker + name left-aligned, type right-aligned to the inner
	// right edge (g.x+w-2) and rendered uppercase + muted.
	for i, col := range g.cols {
		y := g.y + 3 + i
		c.setCh(g.x, y, '│', border, false)
		c.setCh(g.x+g.w-1, y, '│', border, false)
		marker := " "
		isPK := g.pkSet[col.Name]
		if isPK {
			marker = "◆"
		} else if g.fkSet[col.Name] {
			marker = "◇"
		}
		// Left: marker + space, then the name starting at g.x+3. PK columns are
		// rendered bold to set them apart.
		// A connected column (an arrow endpoint touching the selection) on a
		// dimmed card snaps back to vivid: marker/name in the foreground, type in
		// primary (blue), so each relationship stays readable on a faded card.
		nameC, markC, tC := text, text, typeC
		if dim && hlCols[col.Name] {
			nameC, markC, tC = string(colorFg), string(colorFg), string(colorPrimary)
		}
		c.putText(g.x+1, y, marker+" ", markC, isPK)
		c.putText(g.x+3, y, col.Name, nameC, isPK)
		// Right: type one cell inside the right border (a space separates it).
		typ := strings.ToUpper(erdType(col.Type))
		c.putText(g.x+g.w-1-erdCardRightPad-len([]rune(typ)), y, typ, tC, false)
	}
	// Bottom border.
	c.setCh(g.x, g.y+g.h-1, '╰', border, false)
	c.setCh(g.x+g.w-1, g.y+g.h-1, '╯', border, false)
	for x := g.x + 1; x < g.x+g.w-1; x++ {
		c.setCh(x, g.y+g.h-1, '─', border, false)
	}
}

// drawArrowRouted paints one laid-out FK arrow in colour fg. Adjacent-column
// pairs get a clean elbow in the gutter between them; everything else routes
// over the top margin in the lane assigned at layout time.
func (c *gcanvas) drawArrowRouted(a erdArrow, fg string) {
	if a.isMargin {
		c.drawMarginArrow(a.child, a.parent, a.childCol, a.parentCol, a.laneY, fg)
		return
	}
	if a.parent.x+a.parent.w <= a.child.x {
		c.drawSideArrowLeft(a.child, a.parent, a.childCol, a.parentCol, fg)
		return
	}
	if a.child.x+a.child.w <= a.parent.x {
		c.drawSideArrowRight(a.child, a.parent, a.childCol, a.parentCol, fg)
		return
	}
	// Adjacent rank but horizontally overlapping: route over the top margin.
	c.drawMarginArrow(a.child, a.parent, a.childCol, a.parentCol, a.laneY, fg)
}

// drawSideArrowLeft: parent sits to the left of child. The vertical runs one
// cell further into the gutter than the arrowhead and bends into a corner at
// the parent's row, so the line meets the arrowhead along its wide edge as an
// elbow rather than a dangling stub.
func (c *gcanvas) drawSideArrowLeft(child, parent *gcard, childCol, parentCol, fg string) {
	cy := child.colRowY(childCol)
	py := parent.colRowY(parentCol)
	exitX := child.x - 1         // gutter cell touching child's left border
	headX := parent.x + parent.w // arrowhead: tip touches parent's right border
	vertX := headX + 1           // vertical: one cell out, meets the wide edge
	c.hline(vertX, exitX, cy, fg)
	c.vline(vertX, cy, py, fg)
	c.addConn(vertX, py, erdLeft) // bend the vertical into the arrowhead
	c.setCh(headX, py, arrowheadL(), fg, true)
}

// drawSideArrowRight: parent sits to the right of child, the mirror of Left —
// the vertical bends into a corner that meets the arrowhead's wide edge.
func (c *gcanvas) drawSideArrowRight(child, parent *gcard, childCol, parentCol, fg string) {
	cy := child.colRowY(childCol)
	py := parent.colRowY(parentCol)
	exitX := child.x + child.w // gutter cell touching child's right border
	headX := parent.x - 1      // arrowhead: tip touches parent's left border
	vertX := headX - 1         // vertical: one cell out, meets the wide edge
	c.hline(exitX, vertX, cy, fg)
	c.vline(vertX, cy, py, fg)
	c.addConn(vertX, py, erdRight) // bend the vertical into the arrowhead
	c.setCh(headX, py, arrowheadR(), fg, true)
}

// drawMarginArrow routes a non-adjacent FK over the top margin. The vertical
// risers run through the column gutters (clear for the canvas's full height)
// and the horizontal crosses the top margin (above every card), so the line
// never passes through a table card — even one stacked above the endpoint in
// the same column. Each riser uses the gutter on the side of its card that
// faces the other endpoint. The parent riser bends into a corner at the
// parent's row to meet its arrowhead, matching the side-arrow elbows.
func (c *gcanvas) drawMarginArrow(child, parent *gcard, childCol, parentCol string, laneY int, fg string) {
	cy := child.colRowY(childCol)
	py := parent.colRowY(parentCol)
	var childRiserX, headX, vertX int
	var bend erdDir
	var head rune
	if parent.x <= child.x {
		childRiserX = child.x - 1   // gutter to the left of the child
		headX = parent.x + parent.w // tip touches parent's right border
		vertX = headX + 1           // riser one cell out, meets the wide edge
		head = arrowheadL()         // points left, into the parent's right edge
		bend = erdLeft              // corner turns toward the arrowhead
	} else {
		childRiserX = child.x + child.w // gutter to the right of the child
		headX = parent.x - 1            // tip touches parent's left border
		vertX = headX - 1               // riser one cell out, meets the wide edge
		head = arrowheadR()             // points right, into the parent's left edge
		bend = erdRight                 // corner turns toward the arrowhead
	}
	c.vline(childRiserX, laneY, cy, fg)
	c.hline(childRiserX, vertX, laneY, fg)
	c.vline(vertX, laneY, py, fg)
	c.addConn(vertX, py, bend) // bend the riser into the arrowhead
	c.setCh(headX, py, head, fg, true)
}

// --- entry point ------------------------------------------------------------

// erdLayout is the positioned, route-resolved blueprint of a diagram: the
// laid-out cards plus the resolved FK arrows (endpoints + route + lane). It is
// independent of colour/highlight, so render() can re-paint it for any
// selection without redoing the layout maths.
type erdLayout struct {
	cards   []*gcard
	arrows  []erdArrow
	canvasW int
	canvasH int
	// focus is the root table a focused ERD is centred on ("" = the whole
	// schema). It drives the per-card drill-in icon (shown on every card but
	// the root) and is stable across selection re-paints, so render reads it
	// without breaking the cheap re-paint-on-highlight property.
	focus string
}

// erdArrow is one resolved FK→PK connection: the endpoint cards and the column
// rows the line attaches to, whether it routes over the top margin, and (if so)
// its assigned lane row.
type erdArrow struct {
	child, parent       *gcard
	childCol, parentCol string
	isMargin            bool
	laneY               int
}

// computeERDLayout measures and positions the cards and resolves every FK
// arrow's route, returning a blueprint render() can paint for any selection.
// Returns nil for an empty table set.
func computeERDLayout(tables []string, schemas map[string][]db.Column, pks map[string][]string, fks map[string][]db.ForeignKey) *erdLayout {
	if len(tables) == 0 {
		return nil
	}
	in := map[string]bool{}
	for _, t := range tables {
		in[t] = true
	}

	cards := map[string]*gcard{}
	sorted := append([]string(nil), tables...)
	sort.Strings(sorted)
	for _, t := range sorted {
		pkSet := map[string]bool{}
		for _, c := range pks[t] {
			pkSet[c] = true
		}
		fkSet := map[string]bool{}
		for _, fk := range fks[t] {
			fkSet[fk.Column] = true
		}
		g := measureCard(t, schemas[t], pkSet, fkSet)
		cards[t] = &g
	}

	rank, maxRank := erdRanks(tables, fks)

	// Reserve a top-margin lane for each FK whose endpoints are not in
	// adjacent columns (those route over the top to avoid crossing cards).
	topMargin := 0
	for _, child := range sorted {
		for _, fk := range fks[child] {
			if !in[fk.RefTable] || fk.RefTable == child {
				continue
			}
			if abs(rank[child]-rank[fk.RefTable]) != 1 {
				topMargin++
			}
		}
	}

	canvasW, canvasH := placeCards(cards, sorted, rank, maxRank, topMargin)

	// Resolve arrows: endpoint columns, route, and lane assignment for
	// over-the-top arrows. childCol mirrors the historical behaviour of
	// attaching at the child's first FK row (every FK of a child shares it).
	var arrows []erdArrow
	lane := 0
	for _, child := range sorted {
		cg := cards[child]
		childCol := cg.firstFK()
		if childCol == "" {
			childCol = cg.firstPK()
		}
		for _, fk := range fks[child] {
			pg := cards[fk.RefTable]
			if pg == nil || fk.RefTable == child {
				continue
			}
			a := erdArrow{
				child:     cg,
				parent:    pg,
				childCol:  childCol,
				parentCol: pg.firstPK(),
				isMargin:  abs(rank[child]-rank[fk.RefTable]) != 1,
			}
			if a.isMargin {
				a.laneY = lane
				lane++
			}
			arrows = append(arrows, a)
		}
	}

	out := make([]*gcard, 0, len(sorted))
	for _, t := range sorted {
		out = append(out, cards[t])
	}
	return &erdLayout{cards: out, arrows: arrows, canvasW: canvasW, canvasH: canvasH}
}

// addCol records that column col of card is a connection endpoint, so it can be
// kept readable on a dimmed card.
func addCol(m map[string]map[string]bool, card, col string) {
	if m[card] == nil {
		m[card] = map[string]bool{}
	}
	m[card][col] = true
}

// render paints the layout into a fresh canvas. With no selection every card is
// vivid (primary border) and every arrow is grey. When selected names a card it
// stays vivid while every other card is dimmed to the arrow grey, the arrows
// touching the selection turn primary (blue), and the columns at the far ends
// of those arrows (on the dimmed cards) stay readable — marker/name in the
// foreground, type in primary — so each relationship is legible against the
// fade. The layout is colour-agnostic, so this re-paints cheaply on every
// selection change.
func (l *erdLayout) render(selected string) *gcanvas {
	canv := newGcanvas(l.canvasW, l.canvasH)
	conn := string(colorBorderUnfocused)
	cardFg := string(colorPrimary)
	// Columns on dimmed cards that connect to the selection: the far endpoints
	// of every arrow touching it (the parent's PK when the selection is the
	// child, the child's FK when the selection is the parent).
	hlCols := map[string]map[string]bool{}
	if selected != "" {
		for _, a := range l.arrows {
			switch {
			case a.child.name == selected:
				addCol(hlCols, a.parent.name, a.parentCol)
			case a.parent.name == selected:
				addCol(hlCols, a.child.name, a.childCol)
			}
		}
	}
	// Arrows first (so cards paint over any shared edge cell); arrowheads live
	// in gutters/margins and never under cards. The dimmed (grey) arrows are
	// painted before the highlighted (blue) ones touching the selection: where
	// two arrows share a cell the later pass wins (see hline/vline), so a
	// highlighted connector always renders on top instead of being buried under
	// a crossing grey arrow.
	for _, a := range l.arrows {
		if selected != "" && (a.child.name == selected || a.parent.name == selected) {
			continue // highlighted arrows are painted last, on top
		}
		canv.drawArrowRouted(a, conn)
	}
	for _, a := range l.arrows {
		if selected == "" || !(a.child.name == selected || a.parent.name == selected) {
			continue
		}
		canv.drawArrowRouted(a, cardFg)
	}
	focused := l.focus != ""
	for _, c := range l.cards {
		// The drill-in glyph shows on every card except the focus root, and only
		// in a focused ERD (whole-schema view has no root, so no icons). The
		// icon column is reserved on all cards when focused so titles align.
		showIcon := focused && c.name != l.focus
		canv.drawCard(c, cardFg, selected != "" && c.name != selected, hlCols[c.name], showIcon, focused)
	}
	return canv
}

// renderGraphERD lays out table cards and draws FK→PK arrows, returning the
// diagram canvas (the panel clips/scrolls a window of it) and the laid-out
// cards (final positions + PK/FK sets) for mouse hit-testing. With no tables
// both are nil. It is a convenience wrapper over computeERDLayout + render with
// no selection; the panel keeps the layout to re-paint on highlight.
func renderGraphERD(tables []string, schemas map[string][]db.Column, pks map[string][]string, fks map[string][]db.ForeignKey) (*gcanvas, []*gcard) {
	l := computeERDLayout(tables, schemas, pks, fks)
	if l == nil {
		return nil, nil
	}
	return l.render(""), l.cards
}

// abs is a tiny local helper (math.Abs needs float conversion).
func abs[T int](x T) T {
	if x < 0 {
		return -x
	}
	return x
}
