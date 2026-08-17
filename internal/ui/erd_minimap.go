package ui

// ERD mini-map: a tiny overview of the whole diagram, overlaid in the
// bottom-right when the graph is larger than the viewport. Cards render as
// filled blocks; a box-drawing rectangle traces the current viewport.
// Click or drag the map to pan — click and drag are the same action (unlike
// card drag, where a press with no motion must remain a click), so pan
// happens on press and on every subsequent motion. See docs/tui-mouse.md.

const (
	erdMinimapMaxW      = 28
	erdMinimapMaxH      = 12
	erdMinimapMinInnerW = 6
	erdMinimapMinInnerH = 4
	erdMinimapMargin    = 1
	// Floor after aspect-fit so a tall expanded ERD isn't squeezed into a
	// 2–4 cell sliver that looks like the right-hand tables were cropped.
	erdMinimapMinFitW = 16
)

// minimapVisible reports whether the overlay should paint. Hidden in the
// Mermaid view, when the diagram already fits, or when the viewport is too
// small to host a useful map. A tall expanded layout still shows: the map
// keeps a proportional box, with a width floor so rank columns aren't cropped.
func (e ERDPanel) minimapVisible() bool {
	if e.graph == nil || e.merm {
		return false
	}
	cw, ch := e.contentWidth(), e.contentHeight()
	if e.graph.w <= cw && e.graph.h <= ch {
		return false
	}
	iw, ih := e.minimapInnerSize()
	return iw > 0 && ih > 0
}

// minimapInnerSize is the map's interior in cells (excluding the 1-cell
// border). Aspect-fits the card/viewport world into a cap of erdMinimapMaxW/H,
// further limited so the overlay always leaves a margin around itself. Tall
// expanded diagrams floor the width at erdMinimapMinFitW so the schema isn't
// squeezed into a sliver that looks horizontally cropped.
func (e ERDPanel) minimapInnerSize() (iw, ih int) {
	if e.graph == nil {
		return 0, 0
	}
	cw, ch := e.contentWidth(), e.contentHeight()
	maxInnerW := erdMinimapMaxW - 2
	maxInnerH := erdMinimapMaxH - 2
	if capW := cw - 2*erdMinimapMargin - 2; capW < maxInnerW {
		maxInnerW = capW
	}
	if capH := ch - 2*erdMinimapMargin - 2; capH < maxInnerH {
		maxInnerH = capH
	}
	if maxInnerW < erdMinimapMinInnerW || maxInnerH < erdMinimapMinInnerH {
		return 0, 0
	}
	gw, gh := e.minimapWorldSize()
	if gw < 1 {
		gw = 1
	}
	if gh < 1 {
		gh = 1
	}
	// Fit the world aspect into the cap: compare maxInnerW/gw vs maxInnerH/gh.
	if maxInnerW*gh <= maxInnerH*gw {
		iw = maxInnerW
		ih = max(1, gh*maxInnerW/gw)
		if ih > maxInnerH {
			ih = maxInnerH
		}
	} else {
		ih = maxInnerH
		iw = max(1, gw*maxInnerH/gh)
		if iw > maxInnerW {
			iw = maxInnerW
		}
	}
	// Tall (expanded) diagrams aspect-fit to a skinny strip; bump the width
	// so rank columns stay distinguishable. Wide/short diagrams already sit
	// at or above this floor and keep their proportional box.
	if iw < erdMinimapMinFitW {
		iw = min(erdMinimapMinFitW, maxInnerW)
	}
	return iw, ih
}

// minimapWorld is the rendered-canvas rectangle the overlay represents:
// the bounding box of every card. Empty arrow lanes outside the cards are
// omitted — they used to inflate the height, shrink the aspect-fitted
// width, and crop the schema horizontally. The current viewport is drawn
// inside this world (clamped); it is not unioned in, so panning does not
// resize the map.
func (e ERDPanel) minimapWorld() (x, y, w, h int) {
	if e.graph == nil {
		return 0, 0, 1, 1
	}
	gw, gh := e.graph.w, e.graph.h
	minX, minY := gw, gh
	maxX, maxY := 0, 0
	ox, oy := e.canvasOrigin()
	for _, c := range e.cards {
		if c == nil {
			continue
		}
		rx, ry := c.x-ox, c.y-oy
		if rx < minX {
			minX = rx
		}
		if ry < minY {
			minY = ry
		}
		if rx+c.w > maxX {
			maxX = rx + c.w
		}
		if ry+c.h > maxY {
			maxY = ry + c.h
		}
	}
	if maxX <= minX || maxY <= minY {
		return 0, 0, gw, gh
	}
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > gw {
		maxX = gw
	}
	if maxY > gh {
		maxY = gh
	}
	w, h = maxX-minX, maxY-minY
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return minX, minY, w, h
}

func (e ERDPanel) minimapWorldSize() (w, h int) {
	_, _, w, h = e.minimapWorld()
	return w, h
}

// minimapBounds returns the overlay's screen rectangle (including border)
// within the content area. ok is false when the map is hidden.
func (e ERDPanel) minimapBounds() (x, y, w, h int, ok bool) {
	if !e.minimapVisible() {
		return 0, 0, 0, 0, false
	}
	iw, ih := e.minimapInnerSize()
	w, h = iw+2, ih+2
	cw, ch := e.contentWidth(), e.contentHeight()
	x = cw - w - erdMinimapMargin
	y = ch - h - erdMinimapMargin
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y, w, h, true
}

// minimapContains reports whether a content-area cell sits on the overlay
// (border included). Used to steal mouse events from cards underneath.
func (e ERDPanel) minimapContains(sx, sy int) bool {
	x, y, w, h, ok := e.minimapBounds()
	if !ok {
		return false
	}
	return sx >= x && sx < x+w && sy >= y && sy < y+h
}

// minimapPan jumps the viewport so the canvas point under (sx, sy) sits at
// the centre of the view (clamped). Points outside the map clamp to its
// inner edge, so a drag that leaves the overlay still pans to that side.
func (e ERDPanel) minimapPan(sx, sy int) ERDPanel {
	x, y, w, h, ok := e.minimapBounds()
	if !ok || e.graph == nil {
		return e
	}
	e.hoverCard = ""
	iw, ih := w-2, h-2
	if iw < 1 || ih < 1 {
		return e
	}
	ix := clampInt(sx-x-1, 0, iw-1)
	iy := clampInt(sy-y-1, 0, ih-1)
	wx, wy, ww, wh := e.minimapWorld()
	cx := wx + lerp(ix, iw-1, ww-1)
	cy := wy + lerp(iy, ih-1, wh-1)
	cw, ch := e.contentWidth(), e.contentHeight()
	e.scrollX = cx - cw/2
	e.scrollY = cy - ch/2
	e.clampScroll()
	// Keep the keyboard paging cursor inside the new view, matching Wheel.
	vh := e.contentHeight()
	if e.cursor < e.scrollY {
		e.cursor = e.scrollY
	}
	if vh > 0 && e.cursor >= e.scrollY+vh {
		e.cursor = e.scrollY + vh - 1
	}
	return e
}

// lerp maps n in [0, nMax] onto [0, dstMax]. nMax <= 0 returns 0.
func lerp(n, nMax, dstMax int) int {
	if nMax <= 0 || dstMax <= 0 {
		return 0
	}
	if n < 0 {
		n = 0
	}
	if n > nMax {
		n = nMax
	}
	return n * dstMax / nMax
}

// scaleSpan maps the half-open interval [start, start+length) from a src-wide
// range onto a dst-wide range. Empty or out-of-range inputs yield a zero
// span; in-range results are at least 1 cell so a tiny card still paints.
func scaleSpan(start, length, src, dst int) (outStart, outLen int) {
	if dst <= 0 || src <= 0 || length <= 0 {
		return 0, 0
	}
	end := start + length
	if end <= 0 || start >= src {
		return 0, 0
	}
	if start < 0 {
		start = 0
	}
	if end > src {
		end = src
	}
	outStart = start * dst / src
	outEnd := end * dst / src
	if outEnd > dst {
		outEnd = dst
	}
	if outEnd <= outStart {
		outEnd = outStart + 1
	}
	if outEnd > dst {
		outStart = dst - 1
		if outStart < 0 {
			outStart = 0
		}
		outEnd = dst
	}
	return outStart, outEnd - outStart
}

// renderMinimap paints the overlay: bordered overview, card blocks, viewport
// rectangle. Empty string when the map is hidden.
func (e ERDPanel) renderMinimap() string {
	_, _, w, h, ok := e.minimapBounds()
	if !ok {
		return ""
	}
	iw, ih := w-2, h-2
	c := newGcanvas(w, h)
	border := string(colorBorder)
	muted := string(colorMuted)
	primary := string(colorPrimary)
	accent := string(colorAccent)
	stripe := string(colorStripe)

	drawBox(c, 0, 0, w, h, border)
	for row := 1; row <= ih; row++ {
		c.fillBg(1, iw, row, stripe)
	}

	wx, wy, ww, wh := e.minimapWorld()
	ox, oy := e.canvasOrigin()
	inPath := map[string]bool{}
	for _, n := range e.pathCards {
		inPath[n] = true
	}
	for _, card := range e.cards {
		if card == nil {
			continue
		}
		rx, ry := card.x-ox, card.y-oy
		mx, mw := scaleSpan(rx-wx, card.w, ww, iw)
		my, mh := scaleSpan(ry-wy, card.h, wh, ih)
		if mw < 1 || mh < 1 {
			continue
		}
		fg := muted
		switch {
		case card.name == e.focusName:
			fg = accent
		case card.name == e.selected || inPath[card.name]:
			fg = primary
		}
		fillBlock(c, 1+mx, 1+my, mw, mh, '█', fg)
	}

	vw := min(e.contentWidth(), e.graph.w)
	vh := min(e.contentHeight(), e.graph.h)
	vx, vW := scaleSpan(e.scrollX-wx, vw, ww, iw)
	vy, vH := scaleSpan(e.scrollY-wy, vh, wh, ih)
	drawBox(c, 1+vx, 1+vy, vW, vH, primary)
	return c.String()
}

func fillBlock(c *gcanvas, x, y, w, h int, ch rune, fg string) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			c.setCh(x+dx, y+dy, ch, fg, false)
		}
	}
}

func drawBox(c *gcanvas, x, y, w, h int, fg string) {
	if w < 1 || h < 1 {
		return
	}
	if w == 1 && h == 1 {
		c.setCh(x, y, '◆', fg, false)
		return
	}
	if w == 1 {
		for dy := 0; dy < h; dy++ {
			c.setCh(x, y+dy, '│', fg, false)
		}
		return
	}
	if h == 1 {
		for dx := 0; dx < w; dx++ {
			c.setCh(x+dx, y, '─', fg, false)
		}
		return
	}
	c.setCh(x, y, '┌', fg, false)
	c.setCh(x+w-1, y, '┐', fg, false)
	c.setCh(x, y+h-1, '└', fg, false)
	c.setCh(x+w-1, y+h-1, '┘', fg, false)
	for i := 1; i < w-1; i++ {
		c.setCh(x+i, y, '─', fg, false)
		c.setCh(x+i, y+h-1, '─', fg, false)
	}
	for i := 1; i < h-1; i++ {
		c.setCh(x, y+i, '│', fg, false)
		c.setCh(x+w-1, y+i, '│', fg, false)
	}
}
