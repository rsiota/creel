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
	for i, r := range s {
		c.setCh(x+i, y, r, fg, bold)
	}
}

// hline/vline mark connection masks (and a colour) for orthogonal segments;
// the glyph is derived at emit time so overlapping segments merge into tees/crosses.
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
		if c.cells[y][x].fg == "" {
			c.cells[y][x].fg = fg
		}
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
		if c.cells[y][x].fg == "" {
			c.cells[y][x].fg = fg
		}
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
		end := x + 1
		for end < x1 {
			_, efg, ebold := cellGlyph(row[end])
			if efg != fg || ebold != bold {
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
		if fg != "" || bold {
			st := lipgloss.NewStyle()
			if fg != "" {
				st = st.Foreground(lipgloss.Color(fg))
			}
			if bold {
				st = st.Bold(true)
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
// centre if the column is absent.
func (c *gcard) colRowY(col string) int {
	const titleRows = 2 // title + separator
	for i, cc := range c.cols {
		if cc.Name == col {
			return c.y + 1 + titleRows + i
		}
	}
	return c.y + c.h/2
}

// colNameX returns the canvas x where a column's name begins (border + pad
// + marker + space = +3). Used to align over-the-top arrows with the column.
func (c *gcard) colNameX(col string) int {
	return c.x + 3
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

// measureCard computes the card's width/height from its columns. Columns show
// a PK (◆) / FK (◇) marker, the name, and a sanitized type.
func measureCard(name string, cols []db.Column, pkSet, fkSet map[string]bool) gcard {
	maxLine := len(name)
	for _, cc := range cols {
		line := colLine(cc, pkSet, fkSet)
		if l := lineWidth(line); l > maxLine {
			maxLine = l
		}
	}
	w := maxLine + 4 // border(2) + padding(2)
	if w < 10 {
		w = 10
	}
	h := 1 + 2 + len(cols) + 1 // top border + title + separator + columns + bottom border
	return gcard{name: name, cols: cols, pkSet: pkSet, fkSet: fkSet, w: w, h: h}
}

// colLine renders a column's in-card line as a slice of (text, style) segments.
// We return a simple string and track visible width via rune count.
func colLine(c db.Column, pkSet, fkSet map[string]bool) string {
	marker := " "
	if pkSet[c.Name] {
		marker = "◆"
	} else if fkSet[c.Name] {
		marker = "◇"
	}
	return marker + " " + c.Name + "  " + erdType(c.Type)
}

func lineWidth(s string) int { return len([]rune(s)) }

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
			c.y = y
			c.rank = r
			y += c.h + 1
		}
	}
	return canvasW, canvasH
}

// --- drawing ----------------------------------------------------------------

const (
	erdCardBorder = "primary"
	erdCardTitle  = "primary"
	erdArrowFg    = "accent"
)

func (c *gcanvas) drawCard(g *gcard) {
	fg := erdCardBorder
	// Top border with embedded title.
	titleCell := g.x + 2
	c.setCh(g.x, g.y, '╭', fg, false)
	c.setCh(g.x+g.w-1, g.y, '╮', fg, false)
	titleText := " " + g.name + " "
	c.putText(titleCell, g.y, titleText, erdCardTitle, true)
	// fill top border between title and right corner
	for x := titleCell + len([]rune(titleText)); x < g.x+g.w-1; x++ {
		c.setCh(x, g.y, '─', fg, false)
	}
	for x := g.x + 1; x < titleCell; x++ {
		c.setCh(x, g.y, '─', fg, false)
	}
	// Title→columns separator.
	c.setCh(g.x, g.y+1, '│', fg, false)
	c.setCh(g.x+g.w-1, g.y+1, '│', fg, false)
	for x := g.x + 1; x < g.x+g.w-1; x++ {
		c.setCh(x, g.y+1, '─', fg, false)
	}
	// Column rows.
	for i, col := range g.cols {
		y := g.y + 1 + 2 + i
		c.setCh(g.x, y, '│', fg, false)
		c.setCh(g.x+g.w-1, y, '│', fg, false)
		line := colLine(col, g.pkSet, g.fkSet)
		c.putText(g.x+1, y, line, "", false)
	}
	// Bottom border.
	c.setCh(g.x, g.y+g.h-1, '╰', fg, false)
	c.setCh(g.x+g.w-1, g.y+g.h-1, '╯', fg, false)
	for x := g.x + 1; x < g.x+g.w-1; x++ {
		c.setCh(x, g.y+g.h-1, '─', fg, false)
	}
}

// drawArrow connects child→parent (the FK sits on child, referencing parent's
// PK). Adjacent-column pairs get a clean elbow in the gutter between them;
// everything else routes over the top margin in a dedicated lane.
func (c *gcanvas) drawArrow(child, parent *gcard, rank map[string]int, laneY int) {
	childCol := child.firstFK()
	if childCol == "" {
		childCol = child.firstPK()
	}
	parentCol := parent.firstPK()

	if abs(child.rank-parent.rank) == 1 {
		if parent.x+parent.w <= child.x {
			c.drawSideArrowLeft(child, parent, childCol, parentCol)
			return
		}
		if child.x+child.w <= parent.x {
			c.drawSideArrowRight(child, parent, childCol, parentCol)
			return
		}
	}
	c.drawMarginArrow(child, parent, childCol, parentCol, laneY)
}

// drawSideArrowLeft: parent sits to the left of child. Elbow in the gutter.
func (c *gcanvas) drawSideArrowLeft(child, parent *gcard, childCol, parentCol string) {
	fg := erdArrowFg
	cy := child.colRowY(childCol)
	py := parent.colRowY(parentCol)
	exitX := child.x - 1          // gutter cell touching child's left border
	entryX := parent.x + parent.w // gutter cell touching parent's right border
	// horizontal along child's FK row, then vertical hugging parent, arrowhead in.
	c.hline(entryX, exitX, cy, fg)
	c.vline(entryX, cy, py, fg)
	c.setCh(entryX, py, '◂', fg, false)
}

func (c *gcanvas) drawSideArrowRight(child, parent *gcard, childCol, parentCol string) {
	fg := erdArrowFg
	cy := child.colRowY(childCol)
	py := parent.colRowY(parentCol)
	exitX := child.x + child.w // gutter cell touching child's right border
	entryX := parent.x - 1     // gutter cell touching parent's left border
	c.hline(exitX, entryX, cy, fg)
	c.vline(entryX, cy, py, fg)
	c.setCh(entryX, py, '▸', fg, false)
}

// drawMarginArrow routes over the top margin: up from child's FK column,
// across a lane, down into the parent's PK column.
func (c *gcanvas) drawMarginArrow(child, parent *gcard, childCol, parentCol string, laneY int) {
	fg := erdArrowFg
	cx := child.colNameX(childCol)
	px := parent.colNameX(parentCol)
	bottomY := child.y - 1
	if bottomY < laneY+1 {
		bottomY = laneY + 1
	}
	c.vline(cx, laneY, bottomY, fg)
	c.hline(cx, px, laneY, fg)
	pBottom := parent.y - 1
	if pBottom < laneY+1 {
		pBottom = laneY + 1
	}
	c.vline(px, laneY, pBottom, fg)
	c.setCh(px, pBottom, '▾', fg, false)
}

// --- entry point ------------------------------------------------------------

// renderGraphERD lays out table cards and draws FK→PK arrows, returning the
// diagram canvas (the panel clips/scrolls a window of it).
func renderGraphERD(tables []string, schemas map[string][]db.Column, pks map[string][]string, fks map[string][]db.ForeignKey) *gcanvas {
	if len(tables) == 0 {
		return nil
	}
	in := map[string]bool{}
	for _, t := range tables {
		in[t] = true
	}

	// Build cards.
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
	canv := newGcanvas(canvasW, canvasH)

	// Arrows first (so cards paint over any shared edge cell), then cards,
	// then arrowheads live in gutters/margins (never under cards).
	lane := 0
	for _, child := range sorted {
		cg := cards[child]
		for _, fk := range fks[child] {
			pg := cards[fk.RefTable]
			if pg == nil || fk.RefTable == child {
				continue
			}
			isMargin := abs(rank[child]-rank[fk.RefTable]) != 1
			laneY := -1
			if isMargin {
				laneY = lane
				lane++
			}
			canv.drawArrow(cg, pg, rank, laneY)
		}
	}
	for _, t := range sorted {
		canv.drawCard(cards[t])
	}
	return canv
}

// abs is a tiny local helper (math.Abs needs float conversion).
func abs[T int](x T) T {
	if x < 0 {
		return -x
	}
	return x
}
