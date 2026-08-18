package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rsiota/creel/internal/db"
)

// ERDPanel displays a generated entity-relationship diagram as a scrollable,
// read-only overlay — the static counterpart to the interactive `g r`
// relationship explorer. It has two views:
//
//   - graph (default): bordered table cards laid out in dependency-ranked
//     columns with box-drawing arrows from each FK to the PK it references.
//   - mermaid: the same schema as a Mermaid erDiagram source (renders inline
//     in GitHub/GitLab markdown) — toggle with `m`.
//
// `y`/`s` always copy/save the Mermaid source (the exportable text format);
// the graph is for looking. In the graph view j/k/h/l move a keyboard focus
// between table cards (the viewport follows), Space toggles highlight, Enter
// browses the focused table (SELECT *), `f` re-focuses the ERD on that table's
// neighbourhood, and g/G/ctrl+d/ctrl+u page the view; in the Mermaid view
// view; in the Mermaid view j/k/g/G/ctrl+d/ctrl+u scroll the source. esc/q
// close. A mini-map overlays the bottom-right when the diagram is larger
// than the viewport; click or drag it to pan.
type ERDPanel struct {
	visible   bool
	title     string
	layout    *erdLayout // positioned blueprint; re-rendered on highlight change
	graph     *gcanvas   // rendered canvas (layout.render(selected, focusName)); nil if no tables
	cards     []*gcard   // laid-out cards (positions + PK/FK sets) for hit-testing
	selected  string     // highlighted table name ("" = none)
	focusName string     // keyboard-focused card name ("" = none); shown with an accent border
	mermaid   []string   // Mermaid erDiagram source lines
	merm      bool       // show Mermaid source instead of the graph

	scrollY int // top visible row
	scrollX int // left visible column (graph view only)
	cursor  int // for relative paging
	width   int
	height  int

	// In-graph table search ("/" jump): typing filters the visible cards and
	// focuses the first match (tab cycles further matches); enter keeps the
	// focus, esc restores the pre-search focus. searchFocus snapshots focusName
	// when search opens so esc can undo.
	searching   bool
	searchQuery string
	searchFocus string
	searchIndex int

	// FK path-finding ("p"): pathFrom is the anchor (source) card; pathCards is
	// a traced shortest path (ordered names) shown vivid over a dimmed diagram.
	// pathMsg carries a transient no-path message for the status line.
	pathFrom  string
	pathCards []string
	pathMsg   string

	// Free-form card drag (mouse). A MouseLeft on a card body is recorded as a
	// pending drag; the first MouseMotion promotes it to an active drag and the
	// card follows the cursor (arrows re-route live around it), and MouseRelease
	// commits. A press with no motion is still a click — it runs the existing
	// highlight/recentre logic on release, so drag never steals a click. Esc
	// cancels an in-flight drag, restoring the card's pre-drag position.
	dragPending string // card name under a press not yet promoted ("" = none)
	dragCard    string // card name being dragged ("" = none)
	dragPressX  int    // canvas cell of the press (threshold + offset reference)
	dragPressY  int
	dragOffX    int // cursor-to-card-origin offset at grab (card stays under cursor)
	dragOffY    int
	dragOrigX   int // card position at press (esc-cancel restores it)
	dragOrigY   int

	// Hover tooltip (mouse-motion). The name of the card under the cursor,
	// or "" when it sits over empty space. Purely presentational — set by
	// button-less mouse motion (handled in mouse.go's handleERDMouse) and
	// cleared on any viewport-changing input (key, wheel, drag promote) so a
	// tooltip never lingers over a card that scrolled away. Requires
	// WithMouseAllMotion (see docs/tui-mouse.md); the rest of the app's mouse
	// handlers no-op on MouseMotion, so enabling all-motion is safe.
	hoverCard string

	// Mini-map pan-drag. Set on a MouseLeft that lands on the overlay; motion
	// then pans and release clears it. Distinct from card drag because click
	// and drag are the same action here (pan), so there is no pending/promote
	// step — see erd_minimap.go.
	minimapDrag bool

	// z-prefix fold/fit family. `z` is a vim-style prefix: `zz` fits the
	// diagram, `zc`/`zo`/`za` collapse/expand/toggle the focused card. The flag
	// is set on a bare `z` and consumed by the next key; an unrecognized second
	// key clears it and falls through to its normal action (so `zj` isn't
	// eaten). Reset on Show/Hide.
	zPrefix bool
}

func (e ERDPanel) IsVisible() bool { return e.visible }

// Show populates the panel with both representations and shows the graph view.
// It keeps the layout so the canvas can be re-painted on highlight changes.
func (e *ERDPanel) Show(title string, layout *erdLayout, mermaid []string) {
	e.visible = true
	e.title = title
	e.layout = layout
	e.selected = ""
	if layout != nil {
		e.cards = layout.cards
		e.focusName = e.initialFocus()
		e.graph = e.renderedGraph()
	} else {
		e.cards = nil
		e.graph = nil
		e.focusName = ""
	}
	e.mermaid = mermaid
	e.merm = false
	e.scrollY = 0
	e.scrollX = 0
	e.cursor = 0
	e.pathFrom = ""
	e.pathCards = nil
	e.pathMsg = ""
	e.dragPending = ""
	e.dragCard = ""
	e.minimapDrag = false
	e.zPrefix = false
	e.hoverCard = ""
}

// MermaidLines returns the Mermaid source (used by the app's copy/save handlers
// and `:erd save`, since Mermaid is the exportable format).
func (e ERDPanel) MermaidLines() []string { return e.mermaid }

// Hide hides the panel.
func (e *ERDPanel) Hide() {
	e.visible = false
	e.dragPending = ""
	e.dragCard = ""
	e.minimapDrag = false
	e.zPrefix = false
	e.hoverCard = ""
}

// SetSize sets the outer dimensions of the panel (including border).
func (e *ERDPanel) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.clampScroll()
}

// clampScroll keeps scrollY/scrollX within the diagram's scrollable range. A
// stale offset left over from a larger diagram or a terminal resize would
// otherwise push the windowed body past the canvas edge, which makes Place
// re-centre a too-short slice — eating the tables away from the top while they
// stay pinned in the middle. Called from SetSize so the model (and thus the
// mouse hit-tests, which read scrollY) stays consistent; View clamps again as
// a render-time safeguard.
func (e *ERDPanel) clampScroll() {
	ch := e.contentHeight()
	n := e.lineCount()
	maxY := n - ch
	if maxY < 0 {
		maxY = 0
	}
	if e.scrollY > maxY {
		e.scrollY = maxY
	}
	if e.scrollY < 0 {
		e.scrollY = 0
	}
	if e.graph != nil && !e.merm {
		cw := e.contentWidth()
		maxX := e.graph.w - cw
		if maxX < 0 {
			maxX = 0
		}
		if e.scrollX > maxX {
			e.scrollX = maxX
		}
		if e.scrollX < 0 {
			e.scrollX = 0
		}
	}
}

// statusHeight is the number of rows reserved below the graph body for a
// modal prompt/status line — the "/" jump bar while searching, or the FK-path
// status while a path is anchored or traced. The body and all geometry
// derived from contentHeight shrink by this so the line never overlaps the
// diagram.
func (e ERDPanel) statusHeight() int {
	if e.dragCard != "" || e.searching || e.pathFrom != "" || len(e.pathCards) > 0 {
		return 1
	}
	return 0
}

// The panel is frameless and fills the whole workspace, so the content area
// is the full size minus any status row (no border/padding overhead).
func (e ERDPanel) contentHeight() int {
	h := e.height - e.statusHeight()
	if h < 1 {
		return 1
	}
	return h
}

func (e ERDPanel) contentWidth() int {
	if e.width < 1 {
		return 1
	}
	return e.width
}

// --- mouse hit-testing ------------------------------------------------------
//
// The graph is rendered centred in the viewport (View uses lipgloss.Place with
// Center/Center) and offset by the scroll position. To route mouse events we
// invert that: placedBounds replicates the centring math (floor((size-body)/2),
// matching lipgloss v1's position.go), and contentToCanvas maps a screen cell
// back to the canvas cell it covers. cardAt/columnAt then resolve a canvas
// cell to a card and (if it lands on a column row) a column — the basis for
// the hover/click interactions built on top.

// placedBounds returns the windowed graph body's size and the (offX, offY)
// top-left cell it is centred at within the content area. This must match the
// centring View() applies via lipgloss.Place(Center, Center).
func (e ERDPanel) placedBounds() (bodyW, bodyH, offX, offY int) {
	cw, ch := e.contentWidth(), e.contentHeight()
	bodyW, bodyH = cw, ch
	if e.graph != nil {
		if w := e.graph.w - e.scrollX; w < bodyW {
			bodyW = w
		}
		if h := e.graph.h - e.scrollY; h < bodyH {
			bodyH = h
		}
	}
	if bodyW < 0 {
		bodyW = 0
	}
	if bodyH < 0 {
		bodyH = 0
	}
	offX = (cw - bodyW) / 2
	offY = (ch - bodyH) / 2
	return bodyW, bodyH, offX, offY
}

// canvasOrigin returns the logical coordinate of the rendered canvas's
// top-left cell — how far the diagram extends up/left of the (0,0) origin after
// a card is dragged there. It is (0,0) until such a drag, and lets the
// mouse↔canvas transforms and scroll maths stay aligned with render's shift.
func (e ERDPanel) canvasOrigin() (int, int) {
	if e.layout != nil {
		return e.layout.originX, e.layout.originY
	}
	return 0, 0
}

// contentToCanvas maps a screen cell (sx, sy) within the panel's content area
// to the canvas cell it covers, inverting the scroll + centring from View().
// ok is false when there is no graph, the Mermaid view is showing, or the point
// lands in the empty centred margin rather than over the diagram. The returned
// coordinate is logical (card-space), so it matches the cards' stored x/y even
// when a drag has shifted the canvas origin below (0,0).
func (e ERDPanel) contentToCanvas(sx, sy int) (cx, cy int, ok bool) {
	cw, ch := e.contentWidth(), e.contentHeight()
	if e.graph == nil || e.merm || sx < 0 || sx >= cw || sy < 0 || sy >= ch {
		return 0, 0, false
	}
	bodyW, bodyH, offX, offY := e.placedBounds()
	// Rendered cell under the pointer…
	rx := sx - offX + e.scrollX
	ry := sy - offY + e.scrollY
	// Reject the centred margin around a diagram smaller than the viewport.
	if rx < e.scrollX || rx >= e.scrollX+bodyW || ry < e.scrollY || ry >= e.scrollY+bodyH {
		return 0, 0, false
	}
	// …then shift into logical (card) space.
	ox, oy := e.canvasOrigin()
	return rx + ox, ry + oy, true
}

// contentToCanvasUnbounded maps a screen cell to a logical canvas cell without
// rejecting points beyond the current canvas bounds — used during a card drag,
// where the cursor legitimately targets cells outside the diagram (the card is
// dragged out there and the canvas grows, shifting its origin, to contain it).
// It still rejects points outside the panel's content area. Unlike the bounded
// mapping it does not clamp to the origin: a card can be dragged freely in any
// direction, including up/left past the (0,0) edge — the case that used to clamp
// at 0 and trap the leftmost/topmost card against the border.
func (e ERDPanel) contentToCanvasUnbounded(sx, sy int) (cx, cy int, ok bool) {
	cw, ch := e.contentWidth(), e.contentHeight()
	if e.graph == nil || e.merm || sx < 0 || sx >= cw || sy < 0 || sy >= ch {
		return 0, 0, false
	}
	_, _, offX, offY := e.placedBounds()
	rx := sx - offX + e.scrollX
	ry := sy - offY + e.scrollY
	ox, oy := e.canvasOrigin()
	return rx + ox, ry + oy, true
}

// cardAt returns the card covering canvas cell (cx, cy), or nil. Cards never
// overlap in the ranked-column layout, so the first hit wins.
func (e ERDPanel) cardAt(cx, cy int) *gcard {
	for _, c := range e.cards {
		if c == nil {
			continue
		}
		if cx >= c.x && cx < c.x+c.w && cy >= c.y && cy < c.y+c.h {
			return c
		}
	}
	return nil
}

// cardNamed returns the laid-out card with the given table name, or nil.
func (e ERDPanel) cardNamed(name string) *gcard {
	for _, c := range e.cards {
		if c != nil && c.name == name {
			return c
		}
	}
	return nil
}

// focusCard returns the currently keyboard-focused card, or nil.
func (e ERDPanel) focusCard() *gcard { return e.cardNamed(e.focusName) }

// isDrillTarget reports whether c offers the drill-in action: a non-root card
// in a focused ERD. Shared by the header click (drillInCard) and Enter.
func (e ERDPanel) isDrillTarget(c *gcard) bool {
	return e.layout != nil && e.layout.focus != "" && c != nil && c.name != e.layout.focus
}

// initialFocusCard is where the keyboard focus lands when the panel opens: the
// ERD's focus root when there is one, otherwise the top-most then left-most
// card. initialFocus is its name ("" if no cards).
func (e ERDPanel) initialFocusCard() *gcard {
	if e.layout != nil && e.layout.focus != "" {
		if c := e.cardNamed(e.layout.focus); c != nil {
			return c
		}
	}
	var pick *gcard
	for _, c := range e.cards {
		if c == nil {
			continue
		}
		if pick == nil || c.y < pick.y || (c.y == pick.y && c.x < pick.x) {
			pick = c
		}
	}
	return pick
}

func (e ERDPanel) initialFocus() string {
	if c := e.initialFocusCard(); c != nil {
		return c.name
	}
	return ""
}

// setFocus moves the keyboard focus to name and re-renders so the accent border
// follows. Used to keep the focus in sync with a mouse click.
func (e ERDPanel) setFocus(name string) ERDPanel {
	e.focusName = name
	if e.layout != nil {
		e.graph = e.renderedGraph()
	}
	return e
}

// ensureVisible scrolls the minimal amount to bring card fully into view (a
// no-op when the graph already fits the viewport, since scroll stays clamped at
// 0). Gentler than centerOnCard for step-by-step keyboard navigation.
func (e ERDPanel) ensureVisible(c *gcard) ERDPanel {
	if c == nil || e.graph == nil {
		return e
	}
	cw, ch := e.contentWidth(), e.contentHeight()
	maxX := e.graph.w - cw
	if maxX < 0 {
		maxX = 0
	}
	maxY := e.graph.h - ch
	if maxY < 0 {
		maxY = 0
	}
	ox, oy := e.canvasOrigin()
	if c.x-ox < e.scrollX {
		e.scrollX = c.x - ox
	}
	if c.x+c.w-ox > e.scrollX+cw {
		e.scrollX = c.x + c.w - ox - cw
	}
	if c.y-oy < e.scrollY {
		e.scrollY = c.y - oy
	}
	if c.y+c.h-oy > e.scrollY+ch {
		e.scrollY = c.y + c.h - oy - ch
	}
	if e.scrollX > maxX {
		e.scrollX = maxX
	}
	if e.scrollX < 0 {
		e.scrollX = 0
	}
	if e.scrollY > maxY {
		e.scrollY = maxY
	}
	if e.scrollY < 0 {
		e.scrollY = 0
	}
	return e
}

// moveFocus moves the keyboard focus one card in the given direction (spatial
// nearest-neighbour via erdNearestCard), scrolls the new card into view, and
// re-renders. With no current focus it lands on the initial card first.
func (e ERDPanel) moveFocus(dir int) ERDPanel {
	if e.layout == nil || len(e.cards) == 0 {
		return e
	}
	cur := e.focusCard()
	if cur == nil {
		cur = e.initialFocusCard()
		if cur == nil {
			return e
		}
	} else {
		next := erdNearestCard(e.cards, cur, dir)
		if next == nil {
			return e // no card in that direction
		}
		cur = next
	}
	e.focusName = cur.name
	e = e.ensureVisible(cur)
	e.graph = e.renderedGraph()
	return e
}

// drillInCard returns the card whose header row covers canvas cell (cx, cy)
// when that header offers a drill-in action — a non-root card in a focused ERD
// — or nil. The whole title row is the click target (the ◎ glyph is just the
// visual cue), giving a large, easy-to-hit area; the root card and the
// whole-schema view have no drill-in, so clicks there fall through to
// highlight/recentre as before.
func (e ERDPanel) drillInCard(cx, cy int) *gcard {
	if e.layout == nil || e.layout.focus == "" {
		return nil
	}
	for _, c := range e.cards {
		if c == nil || !e.isDrillTarget(c) {
			continue
		}
		if cy == c.y+1 && cx >= c.x && cx < c.x+c.w {
			return c
		}
	}
	return nil
}

// columnAt resolves canvas row cy to the column it lands on within card, plus
// its 0-based index. ok is false on the card's border/title/separator rows or
// outside the card. Layout: row 0 = top border, 1 = title, 2 = separator,
// rows 3.. = columns.
func (e ERDPanel) columnAt(c *gcard, cy int) (db.Column, int, bool) {
	if c == nil {
		return db.Column{}, 0, false
	}
	idx := cy - c.y - 3
	if idx < 0 || idx >= len(c.cols) {
		return db.Column{}, 0, false
	}
	return c.cols[idx], idx, true
}

// erdWheelLines is how many rows a vertical mouse-wheel notch scrolls.
const erdWheelLines = 3

// Wheel scrolls the diagram: dy rows (positive = down) and dx quarter-viewports
// (positive = right). Vertical moves scrollY directly and keeps the cursor (the
// keyboard paging anchor) inside the new view; horizontal moves scrollX. The
// Mermaid view scrolls vertically only.
func (e ERDPanel) Wheel(dy, dx int) ERDPanel {
	// Scrolling moves cards under a static cursor; drop the hover so a tooltip
	// doesn't linger over whatever card ends up beneath it.
	e.hoverCard = ""
	if dy != 0 {
		vh := e.contentHeight()
		maxY := e.lineCount() - vh
		if maxY < 0 {
			maxY = 0
		}
		e.scrollY += dy * erdWheelLines
		if e.scrollY < 0 {
			e.scrollY = 0
		}
		if e.scrollY > maxY {
			e.scrollY = maxY
		}
		if vh > 0 {
			if e.cursor < e.scrollY {
				e.cursor = e.scrollY
			}
			if e.cursor >= e.scrollY+vh {
				e.cursor = e.scrollY + vh - 1
			}
		}
	}
	if dx != 0 && !e.merm && e.graph != nil {
		cw := e.contentWidth()
		step := cw / 4
		if step < 1 {
			step = 1
		}
		max := e.graph.w - cw
		if max < 0 {
			max = 0
		}
		e.scrollX += dx * step
		if e.scrollX < 0 {
			e.scrollX = 0
		}
		if e.scrollX > max {
			e.scrollX = max
		}
	}
	return e
}

// centerOnCard scrolls so the card sits at the viewport's centre, clamped to the
// diagram's scrollable range. The vertical scroll is otherwise cursor-derived,
// so this sets scrollY directly and parks the cursor on the card to keep
// keyboard paging consistent.
func (e ERDPanel) centerOnCard(c *gcard) ERDPanel {
	if c == nil || e.graph == nil {
		return e
	}
	cw, ch := e.contentWidth(), e.contentHeight()
	maxX := e.graph.w - cw
	if maxX < 0 {
		maxX = 0
	}
	maxY := e.graph.h - ch
	if maxY < 0 {
		maxY = 0
	}
	// The card's rendered centre is its logical centre shifted by the canvas
	// origin, and scrollX/Y are rendered offsets, so subtract the origin before
	// clamping to the scrollable range (otherwise a card dragged up/left of the
	// (0,0) origin would re-centre to the wrong spot).
	ox, oy := e.canvasOrigin()
	sx := c.x + c.w/2 - ox - cw/2
	if sx < 0 {
		sx = 0
	} else if sx > maxX {
		sx = maxX
	}
	sy := c.y + c.h/2 - oy - ch/2
	if sy < 0 {
		sy = 0
	} else if sy > maxY {
		sy = maxY
	}
	e.scrollX = sx
	e.scrollY = sy
	// Park the cursor (vertical paging anchor) on the card's rendered row;
	// adjustScroll won't fight it on the next keypress since the row is inside
	// the view.
	if n := e.lineCount(); n > 0 {
		e.cursor = c.y + c.h/2 - oy
		if e.cursor > n-1 {
			e.cursor = n - 1
		}
	}
	if e.cursor < 0 {
		e.cursor = 0
	}
	return e
}

// fitToScreen scrolls so the bounding box of every card is centred in the
// viewport (clamped to the scrollable range) — a “zoom to fit” that brings a
// diagram sprawled by dragging or panning back into view. When the diagram
// already fits the viewport the scroll zeroes and View's centring takes over.
// The cursor (the vertical paging anchor) is parked at the bbox centre to keep
// keyboard paging consistent, mirroring centerOnCard.
func (e ERDPanel) fitToScreen() ERDPanel {
	if e.graph == nil {
		return e
	}
	minX, minY, maxX, maxY := 0, 0, 0, 0
	have := false
	for _, c := range e.cards {
		if c == nil {
			continue
		}
		rx2, cy2 := c.x+c.w, c.y+c.h
		if !have {
			minX, minY, maxX, maxY = c.x, c.y, rx2, cy2
			have = true
			continue
		}
		if c.x < minX {
			minX = c.x
		}
		if c.y < minY {
			minY = c.y
		}
		if rx2 > maxX {
			maxX = rx2
		}
		if cy2 > maxY {
			maxY = cy2
		}
	}
	cw, ch := e.contentWidth(), e.contentHeight()
	maxSX := e.graph.w - cw
	if maxSX < 0 {
		maxSX = 0
	}
	maxSY := e.graph.h - ch
	if maxSY < 0 {
		maxSY = 0
	}
	// Centre the bounding box in the viewport. minX/maxX are logical card coords;
	// shift by the origin to get rendered scroll targets (see centerOnCard).
	ox, oy := e.canvasOrigin()
	e.scrollX = clampInt((minX+maxX)/2-ox-cw/2, 0, maxSX)
	e.scrollY = clampInt((minY+maxY)/2-oy-ch/2, 0, maxSY)
	if n := e.lineCount(); n > 0 {
		e.cursor = clampInt((minY+maxY)/2-oy, 0, n-1)
	}
	return e
}

// toggleHighlight selects (or deselects) a card, re-rendering the canvas so the
// card's border and its FK arrows pick up the accent colour. Clicking the
// already-selected card, or nil (empty space), clears the selection. The
// keyboard focus follows the clicked card so the two stay in sync.
func (e ERDPanel) toggleHighlight(c *gcard) ERDPanel {
	// The single-card highlight and the FK path are mutually exclusive modes;
	// entering one clears the other.
	e.pathFrom = ""
	e.pathCards = nil
	e.pathMsg = ""
	switch {
	case c == nil:
		e.selected = ""
	case e.selected == c.name:
		e.selected = ""
	default:
		e.selected = c.name
	}
	if c != nil {
		e.focusName = c.name
	}
	if e.layout != nil {
		e.graph = e.renderedGraph()
	}
	return e
}

// --- free-form card drag (mouse) ------------------------------------------

// erdDragMaxBound clamps a dragged card's origin so it can't be flung far off
// the diagram (the canvas still grows to contain it). Generous enough that the
// user can spread a schema across a wide canvas.
const erdDragMaxBound = 1000

// dragBeginPress records a pending drag when a MouseLeft lands on a card body.// The card's live position is snapshotted (for esc-cancel) and the cursor's
// offset from the card origin is captured so the card tracks the cursor
// without jumping its top-left under the pointer. No move happens yet — the
// press promotes to a drag only on the first MouseMotion, leaving a plain
// click (release with no motion) to the existing highlight/recentre logic.
func (e ERDPanel) dragBeginPress(card *gcard, cx, cy int) ERDPanel {
	if card == nil {
		return e
	}
	e.dragPending = card.name
	e.dragPressX = cx
	e.dragPressY = cy
	e.dragOrigX = card.x
	e.dragOrigY = card.y
	e.dragOffX = cx - card.x
	e.dragOffY = cy - card.y
	return e
}

// dragPromote starts an active drag if a pending press has moved at least one
// cell from its press point. Returns the (possibly updated) panel and whether a
// drag is now active (the caller follows up with dragMove). Any motion promotes:
// MouseMotion fires on a cell change, so even one event means the user is
// dragging, not clicking.
func (e ERDPanel) dragPromote(cx, cy int) (ERDPanel, bool) {
	if e.dragCard != "" {
		return e, true
	}
	if e.dragPending == "" {
		return e, false
	}
	if abs(cx-e.dragPressX)+abs(cy-e.dragPressY) < 1 {
		return e, false
	}
	e.dragCard = e.dragPending
	e.hoverCard = "" // no tooltip while dragging
	return e, true
}

// dragMove updates the dragged card's position from the cursor, clamped to a
// generous bound so the card stays reachable (the canvas grows to fit). Every
// arrow is then re-routed around the new layout and the graph re-rendered, so
// relationships re-route live as the card moves — the “drawing” feel.
func (e ERDPanel) dragMove(cx, cy int) ERDPanel {
	c := e.cardNamed(e.dragCard)
	if c == nil {
		return e
	}
	c.x = clampInt(cx-e.dragOffX, -erdDragMaxBound, erdDragMaxBound)
	c.y = clampInt(cy-e.dragOffY, -erdDragMaxBound, erdDragMaxBound)
	if e.layout != nil {
		rerouteArrows(e.layout)
		e.graph = e.renderedGraph()
	}
	return e
}

// erdNudgeStep is how many cells H/J/K/L move the focused card. Two cells is
// snappy over SSH without overshooting a typical card.
const erdNudgeStep = 2

// nudgeFocusCard moves the keyboard-focused card by (dx, dy) cells — the
// no-mouse counterpart of drag. Arrows re-route and the viewport follows so
// the card stays on screen. With no focus it lands on the initial card first.
func (e ERDPanel) nudgeFocusCard(dx, dy int) ERDPanel {
	if e.layout == nil || len(e.cards) == 0 {
		return e
	}
	c := e.focusCard()
	if c == nil {
		c = e.initialFocusCard()
		if c == nil {
			return e
		}
		e.focusName = c.name
	}
	c.x = clampInt(c.x+dx, -erdDragMaxBound, erdDragMaxBound)
	c.y = clampInt(c.y+dy, -erdDragMaxBound, erdDragMaxBound)
	rerouteArrows(e.layout)
	e.graph = e.renderedGraph()
	return e.ensureVisible(c)
}

// dragCommit finalizes a drag on MouseRelease: the card stays where it was
// dropped (arrows were re-routed live during the move) and drag state clears.
// A final re-route keeps the result consistent if the terminal skipped any
// intermediate motion events.
func (e ERDPanel) dragCommit() ERDPanel {
	if e.dragCard == "" {
		e.dragPending = ""
		return e
	}
	e.dragCard = ""
	e.dragPending = ""
	if e.layout != nil {
		rerouteArrows(e.layout)
		e.graph = e.renderedGraph()
	}
	return e
}

// dragCancel aborts an in-flight drag (esc), restoring the dragged card to its
// pre-drag position and re-routing arrows back to the original layout.
func (e ERDPanel) dragCancel() ERDPanel {
	if c := e.cardNamed(e.dragCard); c != nil {
		c.x = e.dragOrigX
		c.y = e.dragOrigY
		if e.layout != nil {
			rerouteArrows(e.layout)
			e.graph = e.renderedGraph()
		}
	}
	e.dragCard = ""
	e.dragPending = ""
	return e
}

// dragStatusLine renders the one-line drag status shown at the bottom of the
// graph while a card is being dragged.
func (e ERDPanel) dragStatusLine(width int) string {
	msg := lipgloss.NewStyle().Foreground(colorAccent).Render("◇ dragging "+e.dragCard) +
		" " + mutedStyle.Render("(release to drop, esc to cancel)")
	return lipgloss.NewStyle().Width(width).Render(" " + msg)
}

// --- card collapse/expand ("zc"/"zo"/"za") --------------------------------

// setCollapsed folds (or unfolds) the named card to a header-only bar and
// re-routes every arrow around the changed box. The card keeps its columns and
// its (x,y) position — collapse is an in-place shrink, so it composes with a
// free-form drag and never fights a card the user has moved; `zz` (fit) or a
// drag can tighten a layout left sparse by folding. No-op when there is no
// such card or it is already in the requested state.
func (e ERDPanel) setCollapsed(name string, collapsed bool) ERDPanel {
	card := e.cardNamed(name)
	if card == nil || card.collapsed == collapsed {
		return e
	}
	card.collapsed = collapsed
	if collapsed {
		card.h = erdCollapsedH
	} else {
		card.h = card.fullH
	}
	if e.layout != nil {
		rerouteArrows(e.layout)
		e.graph = e.renderedGraph()
		if fc := e.focusCard(); fc != nil {
			e = e.ensureVisible(fc)
		}
	}
	return e
}

// toggleCollapsed flips the named card's fold state.
func (e ERDPanel) toggleCollapsed(name string) ERDPanel {
	card := e.cardNamed(name)
	if card == nil {
		return e
	}
	return e.setCollapsed(name, !card.collapsed)
}

// setAllCollapsed folds (or unfolds) every card at once, re-runs the ranked
// layout so the columns contract to reclaim the space the folded bodies freed
// (arrows re-route around the new boxes), and reframes the viewport so the
// compact result stays visible — the "collapse all / expand all" counterpart
// to the per-card zc/zo/za. A no-op when there are no cards or nothing changes.
// Free-form drag positions are reset to the ranked columns (a bulk fold is a
// re-organize); the sticky per-card folds are unaffected.
func (e ERDPanel) setAllCollapsed(collapsed bool) ERDPanel {
	if e.layout == nil || len(e.cards) == 0 {
		return e
	}
	changed := false
	for _, c := range e.cards {
		if c == nil || c.collapsed == collapsed {
			continue
		}
		c.collapsed = collapsed
		if collapsed {
			c.h = erdCollapsedH
		} else {
			c.h = c.fullH
		}
		changed = true
	}
	if changed {
		relayout(e.layout)
		e.graph = e.renderedGraph()
		e = e.fitToScreen()
	}
	return e
}

// lineCount is the number of scrollable rows in the active view.
func (e ERDPanel) lineCount() int {
	if e.merm {
		return len(e.mermaid)
	}
	if e.graph != nil {
		return e.graph.h
	}
	return 1
}

// erdFocus directions for keyboard navigation between cards.
const (
	erdFocusUp = iota
	erdFocusDown
	erdFocusLeft
	erdFocusRight
)

// Update routes keys to the active view. `m` toggles Mermaid; in the graph view
// j/k/h/l move the keyboard focus between cards (viewport follows), H/J/K/L
// nudge the focused card, Space toggles highlight on it, and g/G/ctrl+d/ctrl+u
// page the view; in the Mermaid view j/k/g/G/ctrl+d/ctrl+u scroll the source
// lines.
func (e ERDPanel) Update(msg tea.KeyMsg) ERDPanel {
	// Any keyboard input clears the hover tooltip: the diagram may shift under
	// a static cursor (focus move, fold, scroll), so the cached hovered card is
	// no longer trustworthy until the next mouse motion re-establishes it.
	e.hoverCard = ""
	// While the in-graph "/" search bar is open it consumes all keys (including
	// esc/enter, which the app-level ERD block would otherwise grab).
	if e.searching {
		return e.updateSearch(msg)
	}
	if msg.String() == "m" {
		e.merm = !e.merm
		e.scrollX = 0
		e.cursor = 0
		e.scrollY = 0
		return e
	}
	if e.merm {
		return e.updateMermaid(msg)
	}
	if e.graph != nil {
		return e.updateGraph(msg)
	}
	return e
}

// updateGraph handles keys in the graph view: j/k/h/l focus the nearest card in
// that direction, H/J/K/L nudge the focused card (same path as a mouse drag),
// Space toggles highlight on the focused card, and g/G and ctrl+d/ctrl+u page
// the viewport (the focus stays put and the next j/k/h/l pulls it back into
// view). `z` is a vim-style prefix: `zz` fits the diagram, `zc`/`zo`/`za`
// collapse/expand/toggle the focused card, `zM`/`zR` collapse/expand all cards
// (re-routing arrows and reframing); `z` + an unrecognized key clears the
// prefix and falls through to that key's normal action.
func (e ERDPanel) updateGraph(msg tea.KeyMsg) ERDPanel {
	// Resolve a pending z-prefix. An unrecognized second key clears it and falls
	// through to the switch below, so `zj` still moves focus down.
	s := msg.String()
	if e.zPrefix {
		e.zPrefix = false
		switch s {
		case "z":
			return e.fitToScreen()
		case "c":
			return e.setCollapsed(e.focusName, true)
		case "o":
			return e.setCollapsed(e.focusName, false)
		case "a":
			return e.toggleCollapsed(e.focusName)
		case "M":
			return e.setAllCollapsed(true)
		case "R":
			return e.setAllCollapsed(false)
		}
	}
	vh := e.contentHeight()
	switch s {
	case "j", "down":
		e = e.moveFocus(erdFocusDown)
	case "k", "up":
		e = e.moveFocus(erdFocusUp)
	case "h", "left":
		e = e.moveFocus(erdFocusLeft)
	case "l", "right":
		e = e.moveFocus(erdFocusRight)
	case "H":
		e = e.nudgeFocusCard(-erdNudgeStep, 0)
	case "J":
		e = e.nudgeFocusCard(0, erdNudgeStep)
	case "K":
		e = e.nudgeFocusCard(0, -erdNudgeStep)
	case "L":
		e = e.nudgeFocusCard(erdNudgeStep, 0)
	case " ":
		e = e.toggleHighlight(e.focusCard())
	case "z":
		e.zPrefix = true
	case "/":
		e = e.startSearch()
	case "p":
		e = e.togglePath()
	case "g":
		e.scrollY = 0
	case "G":
		if e.graph != nil {
			max := e.graph.h - vh
			if max < 0 {
				max = 0
			}
			e.scrollY = max
		}
	case "ctrl+d":
		e = e.pageGraph(vh / 2)
	case "ctrl+u":
		e = e.pageGraph(-vh / 2)
	}
	return e
}

// pageGraph scrolls the graph viewport by delta rows, clamped to the diagram.
func (e ERDPanel) pageGraph(delta int) ERDPanel {
	if e.graph == nil {
		return e
	}
	max := e.graph.h - e.contentHeight()
	if max < 0 {
		max = 0
	}
	e.scrollY += delta
	if e.scrollY < 0 {
		e.scrollY = 0
	}
	if e.scrollY > max {
		e.scrollY = max
	}
	return e
}

// --- in-graph table search ("/" jump) -------------------------------------

// startSearch opens the "/" jump bar, snapshotting the current focus so esc
// can restore it.
func (e ERDPanel) startSearch() ERDPanel {
	e.searching = true
	e.searchQuery = ""
	e.searchFocus = e.focusName
	e.searchIndex = 0
	return e
}

// cancelSearch closes the jump bar and restores the pre-search focus.
func (e ERDPanel) cancelSearch() ERDPanel {
	e.searching = false
	e.searchQuery = ""
	e.searchIndex = 0
	if e.searchFocus != "" {
		e.focusName = e.searchFocus
		e.searchFocus = ""
		if c := e.cardNamed(e.focusName); c != nil {
			e = e.centerOnCard(c)
		}
	}
	e.graph = e.renderedGraph()
	return e
}

// searchMatches returns the cards whose names contain the query
// (case-insensitive), in layout order.
func (e ERDPanel) searchMatches() []*gcard {
	if e.searchQuery == "" {
		return nil
	}
	q := strings.ToLower(e.searchQuery)
	var ms []*gcard
	for _, c := range e.cards {
		if strings.Contains(strings.ToLower(c.name), q) {
			ms = append(ms, c)
		}
	}
	return ms
}

// focusMatch points the keyboard focus at a card, centres it, and re-renders.
func (e ERDPanel) focusMatch(c *gcard) ERDPanel {
	e.focusName = c.name
	e = e.centerOnCard(c)
	e.graph = e.renderedGraph()
	return e
}

// applySearch re-runs the query against the cards and focuses the current
// searchIndex match (clamped). With no match the focus is left where it is so
// the view does not jump away on a transient typo.
func (e ERDPanel) applySearch() ERDPanel {
	ms := e.searchMatches()
	if len(ms) == 0 {
		e.graph = e.renderedGraph()
		return e
	}
	if e.searchIndex >= len(ms) {
		e.searchIndex = 0
	}
	return e.focusMatch(ms[e.searchIndex])
}

// updateSearch handles keys while the "/" jump bar is open.
func (e ERDPanel) updateSearch(msg tea.KeyMsg) ERDPanel {
	switch msg.String() {
	case "esc", "ctrl+c":
		return e.cancelSearch()
	case "enter":
		// Confirm: keep the current focus, close the bar.
		e.searching = false
		e.searchQuery = ""
		e.searchFocus = ""
		e.searchIndex = 0
		return e
	case "backspace":
		if len(e.searchQuery) > 0 {
			e.searchQuery = e.searchQuery[:len(e.searchQuery)-1]
			e.searchIndex = 0
			e = e.applySearch()
		}
		return e
	case "tab":
		ms := e.searchMatches()
		if len(ms) > 0 {
			e.searchIndex = (e.searchIndex + 1) % len(ms)
			return e.focusMatch(ms[e.searchIndex])
		}
		return e
	case "shift+tab":
		ms := e.searchMatches()
		if len(ms) > 0 {
			e.searchIndex = (e.searchIndex - 1 + len(ms)) % len(ms)
			return e.focusMatch(ms[e.searchIndex])
		}
		return e
	}
	// Append a printable rune to the query.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && msg.Runes[0] >= 0x20 {
		e.searchQuery += string(msg.Runes[0])
		e.searchIndex = 0
		e = e.applySearch()
	}
	return e
}

// searchPrompt renders the one-line "/" jump bar shown at the bottom of the
// graph while a search is active.
func (e ERDPanel) searchPrompt(width int) string {
	q := e.searchQuery
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Render("/"))
	if q == "" {
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" "))
	} else {
		ms := e.searchMatches()
		qStyle := lipgloss.NewStyle().Foreground(colorPrimary)
		if len(ms) == 0 {
			qStyle = lipgloss.NewStyle().Foreground(colorError)
		}
		b.WriteString(qStyle.Render(q))
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" "))
		switch len(ms) {
		case 0:
			b.WriteString(" " + mutedStyle.Render("(no match)"))
		case 1:
			// single match: no hint needed
		default:
			b.WriteString(" " + mutedStyle.Render(fmt.Sprintf("(%d matches, tab to cycle)", len(ms))))
		}
	}
	return lipgloss.NewStyle().Width(width).Render(" " + b.String())
}

// --- FK path-finding ("p") ------------------------------------------------

// renderedGraph paints the current panel state (single highlight, keyboard
// focus, and/or a traced FK path) into a fresh canvas. Centralising this means
// every interaction (move, search, path toggle) re-renders consistently:
// a traced path takes precedence over the single-card highlight, and an
// anchored-but-untraced source reuses the highlight render to preview its
// direct relations.
func (e ERDPanel) renderedGraph() *gcanvas {
	sel := e.selected
	var p erdPath
	switch {
	case len(e.pathCards) > 0:
		p = pathHighlight(e.pathCards)
		sel = "" // path takes precedence over the single-card highlight
	case e.pathFrom != "":
		sel = e.pathFrom // anchor reuses the highlight render
	}
	return e.layout.render(sel, e.focusName, p)
}

// clearPath resets all path state (anchor, traced cards, message) and
// re-renders. Used by esc (step back out of path mode) and whenever another
// mode (highlight, drill-in) supersedes the path.
func (e ERDPanel) clearPath() ERDPanel {
	e.pathFrom = ""
	e.pathCards = nil
	e.pathMsg = ""
	e.graph = e.renderedGraph()
	return e
}

// togglePath is the "p" action, cycling through three states:
//   - idle → anchor the focused card as the path source (superseding a
//     single-card highlight);
//   - anchored → trace the shortest FK path from the source to the focused
//     card (pressing p on the anchor itself cancels);
//   - traced → clear the path back to idle.
//
// A failed trace leaves the anchor in place and notes a no-path message.
func (e ERDPanel) togglePath() ERDPanel {
	e.pathMsg = "" // clear any prior no-path note
	switch {
	case len(e.pathCards) > 0:
		e.pathFrom = ""
		e.pathCards = nil
	case e.pathFrom != "":
		target := e.focusName
		if target == "" || target == e.pathFrom {
			e.pathFrom = "" // p on the anchor cancels
		} else {
			if path := erdShortestPath(e.layout, e.pathFrom, target); len(path) > 0 {
				e.pathCards = path
			} else {
				e.pathMsg = fmt.Sprintf("no FK path: %s → %s", e.pathFrom, target)
			}
		}
	default:
		if c := e.focusCard(); c != nil {
			e.pathFrom = c.name
			e.selected = "" // path mode supersedes the single-card highlight
		}
	}
	e.graph = e.renderedGraph()
	return e
}

// pathStatusLine renders the one-line FK-path status shown at the bottom of
// the graph while path mode is active: the source anchor, the traced chain
// with its hop count, or a no-path note.
func (e ERDPanel) pathStatusLine(width int) string {
	var msg string
	switch {
	case e.pathMsg != "":
		msg = lipgloss.NewStyle().Foreground(colorError).Render("◆ " + e.pathMsg)
	case len(e.pathCards) > 0:
		hops := len(e.pathCards) - 1
		hp := "hops"
		if hops == 1 {
			hp = "hop"
		}
		chain := lipgloss.NewStyle().Foreground(colorPrimary).Render("◆ " + strings.Join(e.pathCards, " → "))
		msg = chain + " " + mutedStyle.Render(fmt.Sprintf("(%d %s, i insert JOIN, p to clear)", hops, hp))
	default: // anchored, not yet traced
		msg = lipgloss.NewStyle().Foreground(colorAccent).Render("◆ path start: "+e.pathFrom) +
			" " + mutedStyle.Render("(move to target, p to trace)")
	}
	return lipgloss.NewStyle().Width(width).Render(" " + msg)
}

// updateMermaid handles keys in the Mermaid source view: j/k/g/G/ctrl+d/ctrl+u
// move a row cursor that drives the scroll, matching the original behaviour.
func (e ERDPanel) updateMermaid(msg tea.KeyMsg) ERDPanel {
	n := e.lineCount()
	vh := e.contentHeight()
	if vh < 1 {
		vh = 1
	}
	switch msg.String() {
	case "j", "down":
		if e.cursor < n-1 {
			e.cursor++
		}
	case "k", "up":
		if e.cursor > 0 {
			e.cursor--
		}
	case "g":
		e.cursor = 0
	case "G":
		e.cursor = n - 1
	case "ctrl+d":
		e.cursor += vh / 2
		if e.cursor >= n {
			e.cursor = n - 1
		}
	case "ctrl+u":
		e.cursor -= vh / 2
		if e.cursor < 0 {
			e.cursor = 0
		}
	}
	e.adjustScroll(vh)
	return e
}

// erdNearestCard returns the card nearest to from in the given direction, or
// nil. Down/Up consider cards whose vertical centre is strictly below/above;
// Left/Right those whose horizontal centre is strictly left/right. Within that
// half-plane the nearest is chosen by perpendicular distance first (so within a
// column j/k steps through the stack and h/l jumps to the neighbouring rank
// column), then by distance along the motion axis.
func erdNearestCard(cards []*gcard, from *gcard, dir int) *gcard {
	fx := from.x + from.w/2
	fy := from.y + from.h/2
	var best *gcard
	bestPerp, bestAlong := 0, 0
	for _, c := range cards {
		if c == nil || c == from {
			continue
		}
		cx := c.x + c.w/2
		cy := c.y + c.h/2
		var along, perp int
		switch dir {
		case erdFocusDown:
			if cy <= fy {
				continue
			}
			along, perp = cy-fy, abs(fx-cx)
		case erdFocusUp:
			if cy >= fy {
				continue
			}
			along, perp = fy-cy, abs(fx-cx)
		case erdFocusRight:
			if cx <= fx {
				continue
			}
			along, perp = cx-fx, abs(fy-cy)
		case erdFocusLeft:
			if cx >= fx {
				continue
			}
			along, perp = fx-cx, abs(fy-cy)
		default:
			continue
		}
		if best == nil || perp < bestPerp || (perp == bestPerp && along < bestAlong) {
			best, bestPerp, bestAlong = c, perp, along
		}
	}
	return best
}

func (e *ERDPanel) adjustScroll(vh int) {
	if e.cursor < e.scrollY {
		e.scrollY = e.cursor
	}
	if e.cursor >= e.scrollY+vh {
		e.scrollY = e.cursor - vh + 1
	}
	if e.scrollY < 0 {
		e.scrollY = 0
	}
}

// canvasToScreen is the inverse of contentToCanvas: it maps a logical canvas
// cell to the screen cell it currently occupies within the panel's content
// area. Used to place the hover tooltip beside the card under the cursor.
func (e ERDPanel) canvasToScreen(cx, cy int) (sx, sy int) {
	_, _, offX, offY := e.placedBounds()
	ox, oy := e.canvasOrigin()
	sx = cx - ox + offX - e.scrollX
	sy = cy - oy + offY - e.scrollY
	return
}

// tooltipText renders the hover tooltip for a card: a bordered box listing the
// table's columns with PK/FK markers and types. It mirrors the card's own
// column rows but is always fully visible, which is the point — collapsed
// cards (▸) hide their columns, and cards partly scrolled out of view clip
// them, so hovering reveals the full list without drilling in or expanding.
// The column data model (db.Column) carries only name + type, so there are no
// comments/defaults to show; markers (◆ PK bold-accent, ◇ FK primary) match
// the card glyphs so the tooltip reads as the same card, in detail.
// fkTargets maps each FK column name on c to its drawn reference target
// ("refTable.refColumn"), derived from the layout's resolved arrows. Only FKs
// whose parent is in the drawn set have an arrow, so this matches what the
// diagram actually shows (a focused ERD omits arrows to absent tables).
func (e ERDPanel) fkTargets(c *gcard) map[string]string {
	m := map[string]string{}
	if e.layout == nil || c == nil {
		return m
	}
	for _, a := range e.layout.arrows {
		if a.child == c && a.childCol != "" && a.parent != nil {
			m[a.childCol] = a.parent.name + "." + a.parentCol
		}
	}
	return m
}

// tooltipText renders the hover tooltip for a card. It surfaces ONLY
// information not already painted on the card, so it never reads as redundant
// noise:
//   - Collapsed card (▸): its columns are hidden, so the tooltip reveals them
//     — marker, name, and type — and annotates each FK column with the
//     table.column it references (a detail the card never shows, even when
//     expanded).
//   - Expanded card: columns are already visible, so the tooltip lists only its
//     FK references (col → refTable.refColumn). Returns "" when the card has
//     no FKs, so no tooltip renders — there is nothing to add.
//
// Column comments/nullability/default aren't available: db.Column carries only
// name + type, and the ERD cards don't load the richer TableColumnInfo.
func (e ERDPanel) tooltipText(c *gcard) string {
	fks := e.fkTargets(c)
	var rows []string
	if c.collapsed {
		rows = e.tooltipRowsCollapsed(c, fks)
	} else if len(fks) > 0 {
		rows = e.tooltipRowsFKRefs(fks, c.cols)
	} else {
		return "" // expanded card with no FK references: nothing to add
	}
	if len(rows) == 0 {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	// Rows carry per-segment ANSI colour, so pad the rendered strings' display
	// width with trailing spaces (not the raw text) to keep the border rectangular.
	all := append([]string{titleStyle.Render(c.name)}, rows...)
	maxW := 0
	for _, r := range all {
		if w := lipgloss.Width(r); w > maxW {
			maxW = w
		}
	}
	for i := range all {
		if d := maxW - lipgloss.Width(all[i]); d > 0 {
			all[i] += strings.Repeat(" ", d)
		}
	}
	content := lipgloss.JoinVertical(lipgloss.Left, all...)
	return lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)
}

// tooltipRowsCollapsed builds one styled row per column for a collapsed card's
// reveal: marker + name + type, with FK columns annotated " → refTable.refCol"
// (the one detail the card never shows). Names/types are padded to fixed
// columns so the rows line up before any FK suffix.
func (e ERDPanel) tooltipRowsCollapsed(c *gcard, fks map[string]string) []string {
	nameW, typeW := 0, 0
	for _, col := range c.cols {
		if w := len([]rune(col.Name)); w > nameW {
			nameW = w
		}
		if w := len([]rune(strings.ToUpper(erdType(col.Type)))); w > typeW {
			typeW = w
		}
	}
	pad := func(s string, w int) string {
		if d := w - len([]rune(s)); d > 0 {
			return s + strings.Repeat(" ", d)
		}
		return s
	}
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	nameStyle := lipgloss.NewStyle().Foreground(colorFg)
	pkStyle := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	fkStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	var rows []string
	for _, col := range c.cols {
		marker, mstyle := "·", mutedStyle
		if c.pkSet[col.Name] {
			marker, mstyle = "◆", pkStyle
		} else if c.fkSet[col.Name] {
			marker, mstyle = "◇", fkStyle
		}
		row := mstyle.Render(marker+" ") +
			nameStyle.Render(pad(col.Name, nameW)) + "  " +
			mutedStyle.Render(pad(strings.ToUpper(erdType(col.Type)), typeW))
		if t, ok := fks[col.Name]; ok {
			row += mutedStyle.Render(" → " + t)
		}
		rows = append(rows, row)
	}
	return rows
}

// tooltipRowsFKRefs builds one styled row per FK reference for an expanded
// card: "◇ col → refTable.refCol". The columns themselves are already visible
// on the card, so only the reference targets (which aren't) are listed. cols
// sets the display order; only entries present in fks are shown.
func (e ERDPanel) tooltipRowsFKRefs(fks map[string]string, cols []db.Column) []string {
	nameStyle := lipgloss.NewStyle().Foreground(colorFg)
	fkStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	var rows []string
	for _, col := range cols {
		t, ok := fks[col.Name]
		if !ok {
			continue
		}
		rows = append(rows, fkStyle.Render("◇ ")+nameStyle.Render(col.Name)+mutedStyle.Render(" → "+t))
	}
	return rows
}

// overlayTooltip composites the hover tooltip onto the rendered graph body,
// placed beside the hovered card. It prefers the card's right side (one-column
// gap) and flips left when that would overflow the viewport, then clamps so the
// box always stays on-screen. Position is recomputed from the card's *current*
// screen location each render, so it tracks scrolls/folds correctly for the
// frame between a motion event and the next viewport-changing input (which
// clears hoverCard). Returns body unchanged if the card can't be resolved.
func (e ERDPanel) overlayTooltip(body string, c *gcard, cw, ch int) string {
	tip := e.tooltipText(c)
	tw := lipgloss.Width(tip)
	th := lipgloss.Height(tip)
	if tw <= 0 || th <= 0 {
		return body
	}
	leftX, topY := e.canvasToScreen(c.x, c.y)
	cardRight := leftX + c.w
	tipX := cardRight + 1 // right of the card, one-column gap
	if tipX+tw > cw {
		tipX = leftX - tw - 1 // flip to the left
	}
	if tipX < 0 {
		tipX = 0
	}
	if tipX+tw > cw {
		tipX = cw - tw // clamp (may overlap the card on very small screens)
	}
	if tipX < 0 {
		tipX = 0
	}
	tipY := topY
	if tipY+th > ch {
		tipY = ch - th
	}
	if tipY < 0 {
		tipY = 0
	}
	return placeOverlay(body, tip, tipX, tipY)
}

// View renders the active view edge to edge — no border, title, or footer,
// so the diagram fills the whole workspace (the status line remains visible
// below it). The graph is centred in the viewport when it's smaller than the
// available area; when it's larger, the scroll/pan position applies as usual.
// The body is padded to the full content width/height so the overlay fully
// covers the workspace behind it.
func (e ERDPanel) View() string {
	cw := e.contentWidth()
	ch := e.contentHeight()
	n := e.lineCount()
	if ch < 1 {
		ch = 1
	}
	maxY := n - ch
	if maxY < 0 {
		maxY = 0
	}
	if e.scrollY > maxY {
		e.scrollY = maxY
	}
	end := e.scrollY + ch
	if end > n {
		end = n
	}

	if e.merm {
		var visible []string
		for i := e.scrollY; i < end; i++ {
			visible = append(visible, e.mermaid[i])
		}
		for len(visible) < ch {
			visible = append(visible, "")
		}
		body := lipgloss.JoinVertical(lipgloss.Left, visible...)
		return lipgloss.NewStyle().Width(cw).Height(ch).Render(body)
	}
	if e.graph != nil {
		// Window returns at most cw×ch (clipped to the canvas), so when the
		// diagram is smaller than the viewport Place centres it; when it fills
		// or exceeds the viewport, Place is a no-op and scroll/pan applies.
		body := e.graph.Window(e.scrollX, cw, e.scrollY, ch)
		body = lipgloss.Place(cw, ch, lipgloss.Center, lipgloss.Center, body)
		// Hover tooltip overlays the graph (never the Mermaid view). Placed
		// beside the hovered card; stale hovers are already cleared by the
		// key/wheel/drag paths, so a non-empty hoverCard is current.
		if e.hoverCard != "" {
			if c := e.cardNamed(e.hoverCard); c != nil {
				body = e.overlayTooltip(body, c, cw, ch)
			}
		}
		if mm := e.renderMinimap(); mm != "" {
			if mx, my, _, _, ok := e.minimapBounds(); ok {
				body = placeOverlay(body, mm, mx, my)
			}
		}
		if e.dragCard != "" {
			return lipgloss.JoinVertical(lipgloss.Left, body, e.dragStatusLine(cw))
		}
		if e.searching {
			return lipgloss.JoinVertical(lipgloss.Left, body, e.searchPrompt(cw))
		}
		if e.pathFrom != "" || len(e.pathCards) > 0 {
			return lipgloss.JoinVertical(lipgloss.Left, body, e.pathStatusLine(cw))
		}
		return body
	}
	return lipgloss.Place(cw, ch, lipgloss.Center, lipgloss.Center, mutedStyle.Render("(no tables)"))
}

// joinERDLines joins diagram source lines for copy/save.
func joinERDLines(lines []string) string { return strings.Join(lines, "\n") + "\n" }
