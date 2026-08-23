package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/db"
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
	w, h int
	// ox, oy is the logical coordinate of the canvas's top-left cell. The drawing
	// primitives take logical (card-space) coordinates and shift by (ox, oy) to
	// land in the cell grid, so a card dragged up/left of the (0,0) origin —
	// which makes the diagram's bounding box start at a negative logical coord —
	// still renders instead of being clipped. (0,0) means logical == rendered.
	ox, oy int
	cells  [][]gcell
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
	rx, ry := x-c.ox, y-c.oy
	if !c.inBounds(rx, ry) {
		return
	}
	c.cells[ry][rx].ch = r
	c.cells[ry][rx].fg = fg
	c.cells[ry][rx].bold = bold
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
	ry := y - c.oy
	for x := x1; x <= x2; x++ {
		rx := x - c.ox
		if c.inBounds(rx, ry) {
			c.cells[ry][rx].bg = bg
		}
	}
}

func (c *gcanvas) hline(x1, x2, y int, fg string) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	ry := y - c.oy
	for x := x1; x <= x2; x++ {
		rx := x - c.ox
		if !c.inBounds(rx, ry) {
			continue
		}
		if x > x1 {
			c.cells[ry][rx].con |= erdLeft
		}
		if x < x2 {
			c.cells[ry][rx].con |= erdRight
		}
		c.cells[ry][rx].fg = fg
	}
}

func (c *gcanvas) vline(x, y1, y2 int, fg string) {
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	rx := x - c.ox
	for y := y1; y <= y2; y++ {
		ry := y - c.oy
		if !c.inBounds(rx, ry) {
			continue
		}
		if y > y1 {
			c.cells[ry][rx].con |= erdUp
		}
		if y < y2 {
			c.cells[ry][rx].con |= erdDown
		}
		c.cells[ry][rx].fg = fg
	}
}

// addConn ORs a connection direction into a cell's line mask, bending a
// straight segment into an elbow (or tee) where routes meet. Used to turn an
// arrow's vertical run into the corner that feeds its arrowhead, so the line
// meets the triangle's base at a 90° angle instead of a dangling stub.
func (c *gcanvas) addConn(x, y int, d erdDir) {
	rx, ry := x-c.ox, y-c.oy
	if c.inBounds(rx, ry) {
		c.cells[ry][rx].con |= d
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
	// collapsed folds the card to a header-only bar (top border + title +
	// bottom border). The columns stay in cols so an expand restores them and
	// the FK/PK endpoints still resolve; h shrinks to erdCollapsedH while
	// fullH remembers the measured height for the restore.
	collapsed bool
	fullH     int
}

// erdCollapsedH is the height of a folded card: top border + title + bottom
// border (no separator, no column rows).
const erdCollapsedH = 3

// colRowY returns the canvas y of a column's text row, or the card's vertical
// centre if the column is absent. A collapsed card has no column rows, so every
// arrow endpoint falls back to the title row's centre — the router must not
// assume the column rows exist. Layout: row 0 = top border, row 1 = title,
// row 2 = separator, rows 3.. = columns.
func (c *gcard) colRowY(col string) int {
	if c.collapsed {
		return c.y + c.h/2 // header-only: arrows attach at the title row's centre
	}
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

const erdCardRightPad = 1 // space between the type and the right border
const erdFocusIcon = '◎'  // header glyph: click to re-focus the ERD on this table

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
	return gcard{name: name, cols: cols, pkSet: pkSet, fkSet: fkSet, w: w, h: h, fullH: h}
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
// Card borders/title use the primary colour, arrows the muted grey, and column
// types the muted colour. When a card is selected the other cards are dimmed to
// muted (border, title, names, markers, types) so the selection and its blue FK
// arrows stand out; the connected columns on those dimmed cards (the arrow
// endpoints) stay readable — name in the foreground, type in primary. Dimmed
// text uses muted rather than borderUnfocused: the latter is a near-bg border
// tint that collapses to illegible contrast on light themes. These reference
// the palette vars (set by applyPalette) at draw time, so they pick up theme
// changes.

// drawCard paints a card's border, title, separator, and columns. fg is the
// border/title colour (primary). When dim is set the whole card — border,
// title, separator, names, markers, and types — is drawn in muted so a
// non-selected card fades out behind the focused one; columns listed in
// hlCols (the arrow endpoints touching the selection) override the dim, with
// marker/name in the foreground and type in primary. The border colour itself
// is the caller's fg (render picks accent for the keyboard focus, muted for a
// dimmed card, primary otherwise) — dim only fades the interior (title text,
// separator, columns), so a focused card keeps its accent border even while
// dimmed.
func (c *gcanvas) drawCard(g *gcard, fg string, dim bool, hlCols map[string]bool, showIcon, reserveIcon bool) {
	grey := string(colorMuted)
	border := fg
	// Column names always take an explicit fg (not "") so paintBg's theme
	// background can't leave them on the terminal's default foreground.
	text, sep, typeC := string(colorFg), string(colorMuted), string(colorMuted)
	if dim {
		text, sep, typeC = grey, grey, grey
	}
	// Top border (plain — the title sits on its own row below it).
	c.setCh(g.x, g.y, '┌', border, false)
	c.setCh(g.x+g.w-1, g.y, '┐', border, false)
	for x := g.x + 1; x < g.x+g.w-1; x++ {
		c.setCh(x, g.y, '─', border, false)
	}
	// Title row: a subtle background tint fills the row between the borders,
	// with the table name centred on top (bold/primary).
	c.fillBg(g.x+1, g.x+g.w-2, g.y+1, string(colorStripe))
	c.setCh(g.x, g.y+1, '│', border, false)
	c.setCh(g.x+g.w-1, g.y+1, '│', border, false)
	// A muted ▸ at the title's left edge flags a folded card (painted before the
	// centred name so a wide name wins the cell instead of being clipped).
	if g.collapsed {
		c.setCh(g.x+1, g.y+1, '▸', string(colorMuted), false)
	}
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
	// A collapsed card stops at the title: its bottom border sits directly
	// under the title (no separator, no column rows), so h == erdCollapsedH.
	if g.collapsed {
		c.setCh(g.x, g.y+g.h-1, '└', border, false)
		c.setCh(g.x+g.w-1, g.y+g.h-1, '┘', border, false)
		for x := g.x + 1; x < g.x+g.w-1; x++ {
			c.setCh(x, g.y+g.h-1, '─', border, false)
		}
		return
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
	c.setCh(g.x, g.y+g.h-1, '└', border, false)
	c.setCh(g.x+g.w-1, g.y+g.h-1, '┘', border, false)
	for x := g.x + 1; x < g.x+g.w-1; x++ {
		c.setCh(x, g.y+g.h-1, '─', border, false)
	}
}

// drawArrowRouted paints one resolved FK arrow in colour fg. A dynamic polyline
// (set after a free-form drag) is painted by drawArrowPoly; otherwise the legacy
// three-mode routing (side elbow for adjacent columns, over-top lane otherwise)
// resolved at layout time is used, unchanged from the initial ranked layout.
func (c *gcanvas) drawArrowRouted(a erdArrow, fg string) {
	if len(a.pts) > 0 {
		c.drawArrowPoly(a.pts, a.headSide, fg)
		return
	}
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

// drawArrowPoly paints a dynamically-routed arrow as an orthogonal polyline:
// consecutive vertices are joined by horizontal/vertical runs (shared vertices
// self-merge into elbow glyphs via the connection masks), and the final vertex
// carries the arrowhead. The path is colour-agnostic; render() picks fg.
func (c *gcanvas) drawArrowPoly(pts []erdPoint, headSide erdDir, fg string) {
	n := len(pts)
	if n < 2 {
		return
	}
	for i := 1; i < n; i++ {
		p, q := pts[i-1], pts[i]
		switch {
		case p.y == q.y:
			c.hline(p.x, q.x, p.y, fg)
		case p.x == q.x:
			c.vline(p.x, p.y, q.y, fg)
		}
	}
	last := pts[n-1]
	head := arrowheadR()
	if headSide == erdLeft {
		head = arrowheadL()
	}
	c.setCh(last.x, last.y, head, fg, true)
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

// erdPoint is one vertex of an arrow's polyline path on the canvas.
type erdPoint struct{ x, y int }

// erdLayout is the positioned, route-resolved blueprint of a diagram: the
// laid-out cards plus the resolved FK arrows (endpoints + route + lane). It is
// independent of colour/highlight, so render() can re-paint it for any
// selection without redoing the layout maths.
type erdLayout struct {
	cards   []*gcard
	arrows  []erdArrow
	canvasW int
	canvasH int
	// originX, originY is the logical coordinate of the rendered canvas's
	// top-left cell. The initial ranked layout places every card at
	// non-negative coords, so this stays (0,0) until a free-form drag pushes a
	// card up/left of the origin — then it goes negative and the canvas grows
	// (and render shifts its cell grid) to contain the moved card. Mouse
	// hit-testing shifts back by the origin so card positions stay logical.
	originX int
	originY int
	// focus is the root table a focused ERD is centred on ("" = the whole
	// schema). It drives the per-card drill-in icon (shown on every card but
	// the root) and is stable across selection re-paints, so render reads it
	// without breaking the cheap re-paint-on-highlight property.
	focus string
}

// erdArrow is one resolved FK→PK connection. The initial ranked layout resolves
// the three-mode legacy fields (isMargin/laneY) once at layout time; a free-form
// card drag re-routes dynamically, storing a colour-agnostic polyline in pts
// (which the drawer prefers over the legacy fields when set). This keeps the
// proven initial layout untouched while letting arrows avoid cards the user has
// moved into their path.
type erdArrow struct {
	child, parent       *gcard
	childCol, parentCol string
	isMargin            bool
	laneY               int
	// pts is the dynamically-routed polyline (endpoints inclusive; the final
	// vertex carries the arrowhead). Nil for the initial ranked layout, set by
	// rerouteArrows after a card moves. headSide is the arrowhead's facing side.
	pts      []erdPoint
	headSide erdDir
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

// --- dynamic routing (free-form card drag) ---------------------------------
//
// After a card is moved with the mouse, the layout-time routing assumptions
// (ranked columns, clear gutters, an over-the-top margin) no longer hold. The
// dynamic router recomputes each arrow's polyline from the cards' live
// positions so a moved card never erases an arrow that used to cross its new
// rectangle (cards paint over arrows — a hidden line reads as a missing
// relationship, so routing around obstacles is mandatory, not cosmetic).
//
// Strategy (Level B): for each FK, first try a clean single-elbow side channel
// — horizontal out of the child, a vertical run through a free column between
// the two cards, and a stub into the arrowhead at the parent. When the cards'
// X ranges overlap or no clear channel exists, fall back to an over/under lane
// (a horizontal row outside every card's vertical extent, with risers in the
// nearest clear gutters). This always produces a visible, correct arrow; it is
// not bend-optimal in pathological layouts, which is the Level C (A*) territory
// the roadmap explicitly defers.

// routeArrow resolves a polyline from child's FK row to the arrowhead at the
// parent's PK row, avoiding every card in `all` except child and parent. The
// returned pts are endpoints-inclusive; the final vertex is the arrowhead cell.
func routeArrow(child, parent *gcard, childCol, parentCol string, all []*gcard, lanes *lanePacker) ([]erdPoint, erdDir) {
	cy := child.colRowY(childCol)
	py := parent.colRowY(parentCol)
	others := make([]*gcard, 0, len(all))
	for _, c := range all {
		if c != nil && c != child && c != parent {
			others = append(others, c)
		}
	}
	if pts, side, ok := routeSide(child, parent, cy, py, others); ok {
		return pts, side
	}
	return routeLane(child, parent, cy, py, all, lanes)
}

// routeSide tries a four-vertex elbow through a free vertical channel between
// the child and parent. ok is false when their X ranges overlap or no clear
// channel exists. Channels are searched nearest-to-parent first so arrows stay
// tight against the referenced table, matching the ranked-layout gutters.
func routeSide(child, parent *gcard, cy, py int, others []*gcard) ([]erdPoint, erdDir, bool) {
	switch {
	case parent.x+parent.w <= child.x: // parent sits fully left of child
		exitX := child.x - 1         // gutter cell touching the child's left border
		headX := parent.x + parent.w // arrowhead tip touches the parent's right border
		for vertX := headX + 1; vertX <= exitX; vertX++ {
			if segClearH(others, exitX, vertX, cy) &&
				segClearV(others, vertX, cy, py) &&
				segClearH(others, vertX, headX, py) {
				return []erdPoint{{exitX, cy}, {vertX, cy}, {vertX, py}, {headX, py}}, erdLeft, true
			}
		}
	case child.x+child.w <= parent.x: // parent sits fully right of child
		exitX := child.x + child.w
		headX := parent.x - 1
		for vertX := headX - 1; vertX >= exitX; vertX-- {
			if segClearH(others, exitX, vertX, cy) &&
				segClearV(others, vertX, cy, py) &&
				segClearH(others, vertX, headX, py) {
				return []erdPoint{{exitX, cy}, {vertX, cy}, {vertX, py}, {headX, py}}, erdRight, true
			}
		}
	}
	return nil, 0, false
}

// routeLane routes over (or, when there is no headroom above, under) a free
// horizontal lane: each card rises through the nearest clear gutter to the
// lane, the line crosses it, and the parent's riser bends into the arrowhead.
// The lanePacker assigns distinct rows to arrows whose spans would otherwise
// collide on the same lane. Riser columns are checked against the full card set
// (including child and parent) so a riser never lands inside an overlapping
// card — the case that forces lane routing in the first place.
func routeLane(child, parent *gcard, cy, py int, all []*gcard, lanes *lanePacker) ([]erdPoint, erdDir) {
	if lanes == nil {
		lanes = newLanePacker(all...)
	}
	parentLeft := parent.x+parent.w/2 <= child.x+child.w/2
	childRiserX := child.x + child.w
	if parentLeft {
		childRiserX = child.x - 1
	}
	// Parent arrowhead sits at the border gutter facing the child; its riser runs
	// one cell outside that. The stub between them is short and clear in the
	// common case; if the riser had to move outward (blocked gutter), the stub
	// may lengthen but stays attached to the head.
	headX := parent.x - 1
	headSide := erdRight
	parentVertX0 := headX - 1
	if parentLeft {
		headX = parent.x + parent.w
		headSide = erdLeft
		parentVertX0 = headX + 1
	}
	// Claim a lane row, then settle each riser in the nearest clear column for
	// the span from its card's row to the lane. Because the lane lies entirely
	// above (or below) every card, the riser columns don't affect lane validity,
	// so one claim suffices regardless of how far the risers move outward.
	laneY := lanes.claim(childRiserX, parentVertX0)
	childRiserX = nearestClearRiser(childRiserX, all, cy, laneY)
	parentVertX := nearestClearRiser(parentVertX0, all, py, laneY)
	return []erdPoint{
		{childRiserX, cy},
		{childRiserX, laneY},
		{parentVertX, laneY},
		{parentVertX, py},
		{headX, py},
	}, headSide
}

// segClearH reports whether row y across [x0,x1] (inclusive) is free of every
// card in cards (none occupies any cell on that span).
func segClearH(cards []*gcard, x0, x1, y int) bool {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	for _, c := range cards {
		if c.y <= y && y < c.y+c.h && c.x <= x1 && x0 < c.x+c.w {
			return false
		}
	}
	return true
}

// segClearV reports whether column x across [y0,y1] (inclusive) is free of
// every card in cards.
func segClearV(cards []*gcard, x int, y0, y1 int) bool {
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for _, c := range cards {
		if c.x <= x && x < c.x+c.w && c.y <= y1 && y0 < c.y+c.h {
			return false
		}
	}
	return true
}

// erdMaxRiserReach bounds the outward search for a clear riser column, keeping
// the router bounded on pathological layouts (it gives up rather than loops).
const erdMaxRiserReach = 64

// nearestClearRiser returns the column nearest to start whose vertical run over
// [y0,y1] is clear of every card in others, searching start, start±1, start±2,
// …. If none is clear within erdMaxRiserReach, start is returned (the route may
// then overlap a card — a rare, dense-layout fallback).
func nearestClearRiser(start int, others []*gcard, y0, y1 int) int {
	if segClearV(others, start, y0, y1) {
		return start
	}
	for d := 1; d <= erdMaxRiserReach; d++ {
		if segClearV(others, start-d, y0, y1) {
			return start - d
		}
		if segClearV(others, start+d, y0, y1) {
			return start + d
		}
	}
	return start
}

// lanePacker assigns distinct horizontal rows to over/under lane routes so
// arrows whose lane spans overlap don't pile on the same row. Above-lanes (in
// the headroom above the topmost card) are preferred — they read as natural
// arcs and need no canvas growth; once they're exhausted, below-lanes stack
// beneath the bottommost card, growing the canvas downward (scrollable, no
// coordinate shift). One row per arrow is reserved (conservative; post-drag
// few arrows need a lane, so the over-reservation is negligible).
type lanePacker struct {
	aboveBusy  map[int]bool
	belowNext  int
	topMost    int
	bottomMost int
}

func newLanePacker(cards ...*gcard) *lanePacker {
	lp := &lanePacker{aboveBusy: map[int]bool{}, topMost: -1}
	for _, c := range cards {
		if c == nil {
			continue
		}
		if lp.topMost < 0 || c.y < lp.topMost {
			lp.topMost = c.y
		}
		if c.y+c.h > lp.bottomMost {
			lp.bottomMost = c.y + c.h
		}
	}
	if lp.topMost < 0 {
		lp.topMost = 0
	}
	return lp
}

// claim returns a row for an over/under lane: the nearest free above-row to
// the cards (so arcs stay close) when headroom exists, otherwise the next free
// below-row (which grows the canvas; the caller sizes the layout to fit).
func (lp *lanePacker) claim(_, _ int) int {
	for r := lp.topMost - 1; r >= 0; r-- {
		if !lp.aboveBusy[r] {
			lp.aboveBusy[r] = true
			return r
		}
	}
	r := lp.bottomMost + lp.belowNext
	lp.belowNext++
	return r
}

// rerouteArrows recomputes every arrow's polyline from the cards' live
// positions (used after a free-form drag) and resizes the canvas to fit both
// the moved cards and any lane rows the router added. The initial layout's
// legacy routing is left intact until this is called. The canvas may now extend
// up/left of the (0,0) origin when a card is dragged there: originX/originY
// record that negative extent (clamped to 0 when nothing reaches past it), the
// canvas dimensions become (max - origin), and render shifts its cell grid by
// the origin — so a card roaming in any direction stays on-canvas instead of
// the leftmost/topmost one being trapped against the border.
func rerouteArrows(l *erdLayout) {
	if l == nil {
		return
	}
	lanes := newLanePacker(l.cards...)
	// Bounding box in logical (card-space) coords. The max grows from the
	// canvas's existing logical extent (rendered size + origin) so a partial
	// re-route such as esc-cancel never shrinks the diagram below what's laid
	// out; the min (the origin) only drops below 0 when a card or route reaches
	// past the top/left edge.
	maxW := l.canvasW + l.originX
	maxH := l.canvasH + l.originY
	minX, minY := 0, 0
	for _, c := range l.cards {
		if c == nil {
			continue
		}
		if c.x+c.w > maxW {
			maxW = c.x + c.w
		}
		if c.y+c.h > maxH {
			maxH = c.y + c.h
		}
		if c.x < minX {
			minX = c.x
		}
		if c.y < minY {
			minY = c.y
		}
	}
	for i := range l.arrows {
		a := &l.arrows[i]
		if a.child == nil || a.parent == nil {
			continue
		}
		pts, side := routeArrow(a.child, a.parent, a.childCol, a.parentCol, l.cards, lanes)
		a.pts = pts
		a.headSide = side
		for _, p := range pts {
			if p.x >= maxW {
				maxW = p.x + 1
			}
			if p.y >= maxH {
				maxH = p.y + 1
			}
			if p.x < minX {
				minX = p.x
			}
			if p.y < minY {
				minY = p.y
			}
		}
	}
	// The origin never moves right/down of (0,0): the initial ranked layout
	// places every card at non-negative coords, so the canvas only extends
	// beyond it as a card is dragged up/left.
	if minX > 0 {
		minX = 0
	}
	if minY > 0 {
		minY = 0
	}
	l.originX = minX
	l.originY = minY
	l.canvasW = maxW - minX
	l.canvasH = maxH - minY
}

// relayout re-runs the dependency-ranked layout (placeCards) on the cards'
// current sizes — contracting each column to reclaim the vertical space a
// folded card's body freed — then re-routes every arrow around the new boxes.
// It is the "contract the diagram" step behind zM/zR (and any future bulk
// resize), reusing the proven initial-layout maths on the live collapse
// states. Free-form drag positions are reset to the ranked columns: a bulk
// fold is a re-organize, not a per-card nudge, so a card the user dragged is
// snapped back into its rank column to keep the contraction predictable. The
// per-card zc/zo/za stay sticky (in-place) so they keep composing with drag.
func relayout(l *erdLayout) {
	if l == nil || len(l.cards) == 0 {
		return
	}
	cards := map[string]*gcard{}
	order := make([]string, 0, len(l.cards))
	rank := map[string]int{}
	maxRank := 0
	for _, c := range l.cards {
		if c == nil {
			continue
		}
		cards[c.name] = c
		order = append(order, c.name)
		rank[c.name] = c.rank
		if c.rank > maxRank {
			maxRank = c.rank
		}
	}
	// Reserve a top-margin lane per non-adjacent FK (matching the initial
	// layout) so over-the-top routes stay above the cards and the origin stays
	// (0,0) instead of growing upward.
	topMargin := 0
	for _, a := range l.arrows {
		if a.child == nil || a.parent == nil {
			continue
		}
		if abs(a.child.rank-a.parent.rank) != 1 {
			topMargin++
		}
	}
	// placeCards produces non-negative coords; reset the origin so rerouteArrows
	// recomputes the canvas bounds from the fresh positions (a stale negative
	// origin from a prior drag would otherwise keep the canvas oversized).
	l.originX, l.originY = 0, 0
	canvasW, canvasH := placeCards(cards, order, rank, maxRank, topMargin)
	l.canvasW = canvasW
	l.canvasH = canvasH
	rerouteArrows(l)
}

// addCol records that column col of card is a connection endpoint, so it can be
// kept readable on a dimmed card.
func addCol(m map[string]map[string]bool, card, col string) {
	if m[card] == nil {
		m[card] = map[string]bool{}
	}
	m[card][col] = true
}

// erdPath describes an active multi-hop FK-path highlight for render: the
// cards on the path (kept vivid while the rest dim) and the edges between
// consecutive path cards (drawn in primary on top of the grey connectors). A
// zero value (nil maps) means no path — render falls back to the single-card
// selected highlight.
type erdPath struct {
	cards map[string]bool
	edges map[string]bool // edgeKey(a, b) for consecutive path cards
}

// edgeKey returns the undirected key for an FK arrow between two tables, so a
// path edge matches regardless of which side is the FK child/parent.
func edgeKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// pathHighlight builds an erdPath from an ordered list of card names.
func pathHighlight(cards []string) erdPath {
	p := erdPath{cards: map[string]bool{}, edges: map[string]bool{}}
	for _, c := range cards {
		p.cards[c] = true
	}
	for i := 1; i < len(cards); i++ {
		p.edges[edgeKey(cards[i-1], cards[i])] = true
	}
	return p
}

// erdShortestPath returns the shortest FK path (ordered table names) between
// from and to over the layout's resolved arrows, treating FKs as undirected
// edges and constrained to the cards present in the layout. Ties are broken by
// sorted neighbour order for determinism. Returns nil if from == to, either
// endpoint is absent, or no path connects them.
func erdShortestPath(l *erdLayout, from, to string) []string {
	if l == nil || from == "" || to == "" || from == to {
		return nil
	}
	adj := map[string]map[string]bool{}
	for _, a := range l.arrows {
		c, p := a.child.name, a.parent.name
		if adj[c] == nil {
			adj[c] = map[string]bool{}
		}
		if adj[p] == nil {
			adj[p] = map[string]bool{}
		}
		adj[c][p] = true
		adj[p][c] = true
	}
	if adj[from] == nil || adj[to] == nil {
		return nil
	}
	prev := map[string]string{}
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == to {
			break
		}
		nb := make([]string, 0, len(adj[cur]))
		for n := range adj[cur] {
			nb = append(nb, n)
		}
		sort.Strings(nb)
		for _, n := range nb {
			if !seen[n] {
				seen[n] = true
				prev[n] = cur
				queue = append(queue, n)
			}
		}
	}
	if !seen[to] {
		return nil
	}
	var path []string
	for c := to; ; c = prev[c] {
		path = append(path, c)
		if c == from {
			break
		}
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// erdJoinHop resolves the FK edge between two consecutive tables on a path.
func erdJoinHop(fks map[string][]db.ForeignKey, l *erdLayout, left, right string) (child, parent, childCol, parentCol string, ok bool) {
	for _, fk := range fks[right] {
		if strings.EqualFold(fk.RefTable, left) {
			return right, left, fk.Column, fk.RefColumn, true
		}
	}
	for _, fk := range fks[left] {
		if strings.EqualFold(fk.RefTable, right) {
			return left, right, fk.Column, fk.RefColumn, true
		}
	}
	if l != nil {
		for _, a := range l.arrows {
			switch {
			case a.child.name == left && a.parent.name == right:
				return left, right, a.childCol, a.parentCol, true
			case a.child.name == right && a.parent.name == left:
				return right, left, a.childCol, a.parentCol, true
			}
		}
	}
	return "", "", "", "", false
}

// erdPathJoinSQL builds a SELECT with JOINs along an ordered FK path.
func erdPathJoinSQL(driver db.Driver, l *erdLayout, fks map[string][]db.ForeignKey, path []string) (string, error) {
	if len(path) < 2 {
		return "", fmt.Errorf("path too short for JOIN")
	}
	var b strings.Builder
	b.WriteString("SELECT *\nFROM ")
	b.WriteString(quoteIdentD(driver, path[0]))
	for i := 0; i < len(path)-1; i++ {
		left, right := path[i], path[i+1]
		child, parent, fkCol, pkCol, ok := erdJoinHop(fks, l, left, right)
		if !ok {
			return "", fmt.Errorf("no FK between %s and %s", left, right)
		}
		qc := quoteIdentD(driver, child)
		qp := quoteIdentD(driver, parent)
		b.WriteString("\nJOIN ")
		b.WriteString(quoteIdentD(driver, right))
		b.WriteString(" ON ")
		b.WriteString(qc)
		b.WriteString(".")
		b.WriteString(quoteIdentD(driver, fkCol))
		b.WriteString(" = ")
		b.WriteString(qp)
		b.WriteString(".")
		b.WriteString(quoteIdentD(driver, pkCol))
	}
	b.WriteString(";")
	return b.String(), nil
}

// render paints the layout into a fresh canvas. With no selection every card is
// vivid (primary border) and every arrow is muted. When selected names a card it
// stays vivid while every other card is dimmed to muted, the arrows touching
// the selection turn primary (blue), and the columns at the far ends of those
// arrows (on the dimmed cards) stay readable — marker/name in the foreground,
// type in primary — so each relationship is legible against the fade. The
// layout is colour-agnostic, so this re-paints cheaply on every selection
// change. focusName is the keyboard-focused card ("" = none); its border is
// drawn in the accent colour so the cursor stays visible — even on a dimmed
// card — without dimming or recolouring anything else.
func (l *erdLayout) render(selected, focusName string, p erdPath) *gcanvas {
	canv := newGcanvas(l.canvasW, l.canvasH)
	canv.ox = l.originX
	canv.oy = l.originY
	conn := string(colorMuted)
	cardFg := string(colorPrimary)
	accent := string(colorAccent)
	pathActive := len(p.cards) > 0
	// Columns on dimmed cards that connect to the selection: the far endpoints
	// of every arrow touching it (the parent's PK when the selection is the
	// child, the child's FK when the selection is the parent). Path mode spans
	// many cards, so it does not single out connected columns.
	hlCols := map[string]map[string]bool{}
	if selected != "" && !pathActive {
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
	// in gutters/margins and never under cards. Grey connectors are painted
	// before the vivid (path/selection) ones so the latter always win a shared
	// cell (last-writer, see hline/vline) and render on top of crossings.
	onPath := func(a erdArrow) bool { return pathActive && p.edges[edgeKey(a.child.name, a.parent.name)] }
	touchesSel := func(a erdArrow) bool {
		return selected != "" && !pathActive && (a.child.name == selected || a.parent.name == selected)
	}
	for _, a := range l.arrows {
		if onPath(a) || touchesSel(a) {
			continue
		}
		canv.drawArrowRouted(a, conn)
	}
	for _, a := range l.arrows {
		if onPath(a) || touchesSel(a) {
			canv.drawArrowRouted(a, cardFg)
		}
	}
	hasRoot := l.focus != ""
	for _, c := range l.cards {
		// Border colour: path cards and the selection are vivid (primary); every
		// other card is grey when a path/selection is active. The keyboard focus
		// always wins the border in accent so the cursor stays visible, even on
		// a dimmed card — selection still outranks focus.
		border := cardFg
		dim := false
		if pathActive {
			on := p.cards[c.name]
			dim = !on
			if !on {
				border = conn
			}
		} else if selected != "" && c.name != selected {
			border = conn
			dim = true
		}
		if focusName != "" && c.name == focusName && c.name != selected {
			border = accent
		}
		// The drill-in glyph shows on every card except the focus root, and only
		// in a focused ERD (whole-schema view has no root, so no icons). The
		// icon column is reserved on all cards when focused so titles align.
		showIcon := hasRoot && c.name != l.focus
		canv.drawCard(c, border, dim, hlCols[c.name], showIcon, hasRoot)
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
	return l.render("", "", erdPath{}), l.cards
}

// abs is a tiny local helper (math.Abs needs float conversion).
func abs[T int](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

// clampInt pins x to the closed range [lo, hi].
func clampInt(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}
