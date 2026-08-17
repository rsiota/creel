package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// largeMinimapPanel is a 40×20 viewport over a 200×100 canvas, large enough
// that the mini-map must show. Cards sit at known canvas positions so mapping
// and click-through tests have something to paint / hit.
func largeMinimapPanel() ERDPanel {
	e := ERDPanel{width: 40, height: 20, focusName: "a"}
	e.graph = newGcanvas(200, 100)
	e.cards = []*gcard{
		{name: "a", x: 10, y: 10, w: 16, h: 8},
		{name: "b", x: 80, y: 40, w: 16, h: 8},
		{name: "c", x: 150, y: 80, w: 16, h: 8},
		// Sits in the bottom-right of the unscrolled viewport, under where the
		// mini-map overlays — used to prove clicks don't fall through.
		{name: "corner", x: 26, y: 13, w: 12, h: 6},
	}
	return e
}

func TestScaleSpan(t *testing.T) {
	gotS, gotL := scaleSpan(0, 50, 100, 10)
	if gotS != 0 || gotL != 5 {
		t.Errorf("scaleSpan(0,50,100,10) = (%d,%d) want (0,5)", gotS, gotL)
	}
	gotS, gotL = scaleSpan(50, 50, 100, 10)
	if gotS != 5 || gotL != 5 {
		t.Errorf("scaleSpan(50,50,100,10) = (%d,%d) want (5,5)", gotS, gotL)
	}
	// Tiny card still paints at least one cell.
	gotS, gotL = scaleSpan(0, 1, 200, 10)
	if gotL < 1 {
		t.Errorf("tiny span collapsed to %d, want >= 1", gotL)
	}
	gotS, gotL = scaleSpan(-10, 5, 100, 10)
	if gotL != 0 {
		t.Errorf("fully negative span = (%d,%d) want (0,0)", gotS, gotL)
	}
	gotS, gotL = scaleSpan(200, 10, 100, 10)
	if gotL != 0 {
		t.Errorf("fully past-end span = (%d,%d) want (0,0)", gotS, gotL)
	}
}

func TestLerp(t *testing.T) {
	if got := lerp(0, 9, 199); got != 0 {
		t.Errorf("lerp start = %d want 0", got)
	}
	if got := lerp(9, 9, 199); got != 199 {
		t.Errorf("lerp end = %d want 199", got)
	}
	if got := lerp(0, 0, 199); got != 0 {
		t.Errorf("lerp nMax=0 = %d want 0", got)
	}
}

func TestMinimapHiddenWhenDiagramFits(t *testing.T) {
	e := ERDPanel{width: 80, height: 24}
	e.graph = newGcanvas(20, 10)
	if e.minimapVisible() {
		t.Fatal("mini-map shown for a diagram that fits the viewport")
	}
	if _, _, _, _, ok := e.minimapBounds(); ok {
		t.Fatal("minimapBounds ok for a fitting diagram")
	}
	if e.renderMinimap() != "" {
		t.Fatal("renderMinimap non-empty for a fitting diagram")
	}
}

func TestMinimapHiddenInMermaid(t *testing.T) {
	e := largeMinimapPanel()
	if !e.minimapVisible() {
		t.Fatal("precondition: large panel should show a mini-map")
	}
	e.merm = true
	if e.minimapVisible() {
		t.Fatal("mini-map shown in the Mermaid view")
	}
}

func TestMinimapHiddenWhenViewportTiny(t *testing.T) {
	e := ERDPanel{width: 5, height: 3}
	e.graph = newGcanvas(40, 30)
	if e.minimapVisible() {
		t.Fatal("mini-map shown in a viewport too small to host it")
	}
}

// Expanded cards make a ranked ERD much taller than it is wide. Aspect-fitting
// that world into the map cap used to produce a 2–4 cell sliver (or hide the
// overlay) that looked like the right-hand tables were cropped.
func TestMinimapTallDiagramKeepsHorizontalSpan(t *testing.T) {
	e := ERDPanel{width: 80, height: 24, focusName: "left"}
	e.graph = newGcanvas(120, 400)
	e.cards = []*gcard{
		{name: "left", x: 0, y: 0, w: 24, h: 40},
		{name: "mid", x: 48, y: 160, w: 24, h: 40},
		{name: "right", x: 96, y: 340, w: 24, h: 40},
	}
	if !e.minimapVisible() {
		t.Fatal("mini-map hidden for a tall expanded canvas that overflows the viewport")
	}
	iw, ih := e.minimapInnerSize()
	if iw < erdMinimapMinFitW {
		t.Errorf("inner width %d want >= %d (tall schema must not crop horizontally)", iw, erdMinimapMinFitW)
	}
	if ih < 1 {
		t.Errorf("inner height %d want >= 1", ih)
	}
	wx, _, ww, _ := e.minimapWorld()
	left, _ := scaleSpan(0-wx, 24, ww, iw)
	right, rw := scaleSpan(96-wx, 24, ww, iw)
	if right <= left {
		t.Errorf("right card mx=%d left mx=%d — columns collapsed (cropped)", right, left)
	}
	if right+rw > iw {
		t.Errorf("right card clipped: mx=%d mw=%d innerW=%d", right, rw, iw)
	}
}

func TestMinimapShownWhenDiagramOverflows(t *testing.T) {
	e := largeMinimapPanel()
	if !e.minimapVisible() {
		t.Fatal("mini-map hidden for a 200×100 canvas in a 40×20 viewport")
	}
	x, y, w, h, ok := e.minimapBounds()
	if !ok {
		t.Fatal("minimapBounds not ok")
	}
	if w < erdMinimapMinInnerW+2 || h < erdMinimapMinInnerH+2 {
		t.Errorf("map size %d×%d is smaller than the minimum", w, h)
	}
	// Cards bbox is ~156×78, so the overlay is aspect-fitted, not stretched
	// to the 26×10 cap.
	iw, ih := e.minimapInnerSize()
	if iw >= erdMinimapMaxW-2 {
		t.Errorf("inner width %d stretched to the cap; want a proportional box", iw)
	}
	if iw < erdMinimapMinFitW || ih < 1 {
		t.Errorf("inner size %d×%d want at least %d wide", iw, ih, erdMinimapMinFitW)
	}
	cw, ch := e.contentWidth(), e.contentHeight()
	if x+w > cw || y+h > ch {
		t.Errorf("map [%d,%d %d×%d] overflows content %d×%d", x, y, w, h, cw, ch)
	}
	// Bottom-right placement with a 1-cell margin.
	if x != cw-w-erdMinimapMargin || y != ch-h-erdMinimapMargin {
		t.Errorf("map origin (%d,%d) want bottom-right (%d,%d)", x, y, cw-w-erdMinimapMargin, ch-h-erdMinimapMargin)
	}
}

func TestMinimapRenderHasViewportAndCards(t *testing.T) {
	e := largeMinimapPanel()
	out := ansi.Strip(e.renderMinimap())
	if !strings.Contains(out, "┌") || !strings.Contains(out, "█") {
		t.Errorf("mini-map missing border or card blocks:\n%s", out)
	}
	view := ansi.Strip(e.View())
	if !strings.Contains(view, "█") {
		t.Errorf("View() missing mini-map card blocks:\n%s", view)
	}
}

func TestMinimapPanCentersOnClick(t *testing.T) {
	e := largeMinimapPanel()
	x, y, w, h, ok := e.minimapBounds()
	if !ok {
		t.Fatal("precondition: map not visible")
	}
	// Click the inner centre → viewport should jump toward the cards' centre.
	sx, sy := x+w/2, y+h/2
	e = e.minimapPan(sx, sy)
	if e.scrollX < 40 || e.scrollX > 100 {
		t.Errorf("centre click scrollX=%d want around the card cluster", e.scrollX)
	}

	// Click the top-left inner cell → scroll clamps toward the origin.
	x, y, _, _, ok = e.minimapBounds()
	if !ok {
		t.Fatal("map disappeared after centre pan")
	}
	e = e.minimapPan(x+1, y+1)
	if e.scrollX != 0 || e.scrollY != 0 {
		t.Errorf("top-left click scroll=(%d,%d) want (0,0)", e.scrollX, e.scrollY)
	}
}

func TestMinimapClickDoesNotHitCardUnderneath(t *testing.T) {
	m := Model{erdPanel: largeMinimapPanel()}
	x, y, w, h, ok := m.erdPanel.minimapBounds()
	if !ok {
		t.Fatal("precondition: map not visible")
	}
	// The overlay covers the bottom-right, where the "corner" card sits at
	// scroll (0,0). A click on the map must pan, not highlight that card.
	sx, sy := x+w/2, y+h/2
	if !m.erdPanel.minimapContains(sx, sy) {
		t.Fatalf("centre of map (%d,%d) not inside bounds", sx, sy)
	}
	mm, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, X: sx, Y: sy})
	m = mm.(Model)
	if !m.erdPanel.minimapDrag {
		t.Error("press on mini-map did not start minimapDrag")
	}
	if m.erdPanel.selected != "" {
		t.Errorf("press on mini-map selected %q; want none (no click-through)", m.erdPanel.selected)
	}
	if m.erdPanel.dragPending != "" || m.erdPanel.dragCard != "" {
		t.Errorf("press on mini-map started a card drag (pending=%q card=%q)", m.erdPanel.dragPending, m.erdPanel.dragCard)
	}
	if m.erdPanel.scrollX == 0 && m.erdPanel.scrollY == 0 {
		t.Error("press on mini-map centre did not pan")
	}
	mm, _ = m.handleERDMouse(tea.MouseMsg{Type: tea.MouseRelease, Action: tea.MouseActionRelease, X: sx, Y: sy})
	m = mm.(Model)
	if m.erdPanel.minimapDrag {
		t.Error("release did not clear minimapDrag")
	}
	if m.erdPanel.selected != "" {
		t.Errorf("release selected %q; want none", m.erdPanel.selected)
	}
}

func TestMinimapDragPans(t *testing.T) {
	m := Model{erdPanel: largeMinimapPanel()}
	x, y, w, h, ok := m.erdPanel.minimapBounds()
	if !ok {
		t.Fatal("precondition: map not visible")
	}
	// Press on the left inner edge, drag to the right inner edge.
	pressX, pressY := x+1, y+h/2
	mm, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, X: pressX, Y: pressY})
	m = mm.(Model)
	afterPress := m.erdPanel.scrollX
	relX := x + w - 2
	mm, _ = m.handleERDMouse(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionMotion, X: relX, Y: pressY})
	m = mm.(Model)
	if m.erdPanel.scrollX <= afterPress {
		t.Errorf("drag right: scrollX=%d did not increase from %d", m.erdPanel.scrollX, afterPress)
	}
}

func TestMinimapHoverDoesNotSetUnderlyingCard(t *testing.T) {
	m := Model{erdPanel: largeMinimapPanel()}
	x, y, w, h, ok := m.erdPanel.minimapBounds()
	if !ok {
		t.Fatal("precondition: map not visible")
	}
	sx, sy := x+w/2, y+h/2
	mm, _ := m.handleERDMouse(tea.MouseMsg{Type: tea.MouseMotion, Action: tea.MouseActionMotion, X: sx, Y: sy})
	m = mm.(Model)
	if m.erdPanel.hoverCard != "" {
		t.Errorf("hover over mini-map set hoverCard=%q; want empty", m.erdPanel.hoverCard)
	}
}

func TestMinimapContainsFalseOutside(t *testing.T) {
	e := largeMinimapPanel()
	x, y, _, _, ok := e.minimapBounds()
	if !ok {
		t.Fatal("precondition: map not visible")
	}
	if e.minimapContains(0, 0) {
		t.Error("top-left of the viewport is not the mini-map")
	}
	if !e.minimapContains(x, y) {
		t.Error("map origin should be inside")
	}
}
