package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/db"
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
// re-focuses the ERD on the focused table, and g/G/ctrl+d/ctrl+u page the
// view; in the Mermaid view j/k/g/G/ctrl+d/ctrl+u scroll the source. esc/q
// close.
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
		e.graph = layout.render("", e.focusName)
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
}

// MermaidLines returns the Mermaid source (used by the app's copy/save handlers
// and `:erd save`, since Mermaid is the exportable format).
func (e ERDPanel) MermaidLines() []string { return e.mermaid }

// Hide hides the panel.
func (e *ERDPanel) Hide() { e.visible = false }

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

// promptHeight is the number of rows reserved below the graph body for a
// modal prompt (currently the "/" search bar). The body and all geometry
// derived from contentHeight shrink by this so the prompt never overlaps the
// diagram.
func (e ERDPanel) promptHeight() int {
	if e.searching {
		return 1
	}
	return 0
}

// The panel is frameless and fills the whole workspace, so the content area
// is the full size minus any prompt row (no border/padding overhead).
func (e ERDPanel) contentHeight() int {
	h := e.height - e.promptHeight()
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

// contentToCanvas maps a screen cell (sx, sy) within the panel's content area
// to the canvas cell it covers, inverting the scroll + centring from View().
// ok is false when there is no graph, the Mermaid view is showing, or the point
// lands in the empty centred margin rather than over the diagram.
func (e ERDPanel) contentToCanvas(sx, sy int) (cx, cy int, ok bool) {
	cw, ch := e.contentWidth(), e.contentHeight()
	if e.graph == nil || e.merm || sx < 0 || sx >= cw || sy < 0 || sy >= ch {
		return 0, 0, false
	}
	bodyW, bodyH, offX, offY := e.placedBounds()
	cx = sx - offX + e.scrollX
	cy = sy - offY + e.scrollY
	// Reject the centred margin around a diagram smaller than the viewport.
	if cx < e.scrollX || cx >= e.scrollX+bodyW || cy < e.scrollY || cy >= e.scrollY+bodyH {
		return 0, 0, false
	}
	return cx, cy, true
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
		e.graph = e.layout.render(e.selected, e.focusName)
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
	if c.x < e.scrollX {
		e.scrollX = c.x
	}
	if c.x+c.w > e.scrollX+cw {
		e.scrollX = c.x + c.w - cw
	}
	if c.y < e.scrollY {
		e.scrollY = c.y
	}
	if c.y+c.h > e.scrollY+ch {
		e.scrollY = c.y + c.h - ch
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
	e.graph = e.layout.render(e.selected, e.focusName)
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
	sx := c.x + c.w/2 - cw/2
	if sx < 0 {
		sx = 0
	} else if sx > maxX {
		sx = maxX
	}
	sy := c.y + c.h/2 - ch/2
	if sy < 0 {
		sy = 0
	} else if sy > maxY {
		sy = maxY
	}
	e.scrollX = sx
	e.scrollY = sy
	// Park the cursor (vertical paging anchor) on the card; adjustScroll won't
	// fight it on the next keypress since the row is inside the view.
	if n := e.lineCount(); n > 0 {
		e.cursor = c.y + c.h/2
		if e.cursor > n-1 {
			e.cursor = n - 1
		}
	}
	if e.cursor < 0 {
		e.cursor = 0
	}
	return e
}

// toggleHighlight selects (or deselects) a card, re-rendering the canvas so the
// card's border and its FK arrows pick up the accent colour. Clicking the
// already-selected card, or nil (empty space), clears the selection. The
// keyboard focus follows the clicked card so the two stay in sync.
func (e ERDPanel) toggleHighlight(c *gcard) ERDPanel {
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
		e.graph = e.layout.render(e.selected, e.focusName)
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
// j/k/h/l move the keyboard focus between cards (viewport follows), Space
// toggles highlight on it, and g/G/ctrl+d/ctrl+u page the view; in the Mermaid
// view j/k/g/G/ctrl+d/ctrl+u scroll the source lines.
func (e ERDPanel) Update(msg tea.KeyMsg) ERDPanel {
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
// that direction, Space toggles highlight on the focused card, and g/G and
// ctrl+d/ctrl+u page the viewport (the focus stays put and the next j/k/h/l
// pulls it back into view).
func (e ERDPanel) updateGraph(msg tea.KeyMsg) ERDPanel {
	vh := e.contentHeight()
	switch msg.String() {
	case "j", "down":
		e = e.moveFocus(erdFocusDown)
	case "k", "up":
		e = e.moveFocus(erdFocusUp)
	case "h", "left":
		e = e.moveFocus(erdFocusLeft)
	case "l", "right":
		e = e.moveFocus(erdFocusRight)
	case " ":
		e = e.toggleHighlight(e.focusCard())
	case "/":
		e = e.startSearch()
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
	e.graph = e.layout.render(e.selected, e.focusName)
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
	e.graph = e.layout.render(e.selected, e.focusName)
	return e
}

// applySearch re-runs the query against the cards and focuses the current
// searchIndex match (clamped). With no match the focus is left where it is so
// the view does not jump away on a transient typo.
func (e ERDPanel) applySearch() ERDPanel {
	ms := e.searchMatches()
	if len(ms) == 0 {
		e.graph = e.layout.render(e.selected, e.focusName)
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
		if e.searching {
			return lipgloss.JoinVertical(lipgloss.Left, body, e.searchPrompt(cw))
		}
		return body
	}
	return lipgloss.Place(cw, ch, lipgloss.Center, lipgloss.Center, mutedStyle.Render("(no tables)"))
}

// joinERDLines joins diagram source lines for copy/save.
func joinERDLines(lines []string) string { return strings.Join(lines, "\n") + "\n" }
