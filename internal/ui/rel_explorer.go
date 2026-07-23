package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// relDir labels a relationship edge's direction relative to a row.
type relDir int

const (
	relOutbound relDir = iota // this row → a referenced parent row
	relInbound                // child rows → this row
)

// relNode is one relationship edge, resolved enough to display a live count and
// to load the related rows: target{Table,Column} is the WHERE clause to build,
// and sourceColumn is the row column whose value filters it. Shared by :refs
// (count column) and the explorer (edge nodes).
type relNode struct {
	dir          relDir
	targetTable  string // related table
	targetColumn string // WHERE column on targetTable
	sourceColumn string // row column whose value we filter by
	count        string // "14", "-", "?", "" while loading
}

// maxExplorerDepth bounds how deep the tree can grow, preventing runaway
// recursion on self-referential FKs and keeping the view readable. Root is
// depth 0; edges hang at depth 1, their child rows at depth 2, and so on.
const maxExplorerDepth = 8

// explorerChildLimit caps how many child rows an expanded edge loads inline.
// Beyond it, a synthetic "(+N more — Enter to open in grid)" node is appended
// so the tree stays snappy on wide fan-outs; Enter on the edge opens the full
// set in the grid.
const explorerChildLimit = 50

type nodeKind int

const (
	nodeRow  nodeKind = iota // an actual record (the root or a child row)
	nodeEdge                 // a relationship edge (expandable to child rows)
)

// expNode is one node in the explorer tree. Row nodes represent a record;
// edge nodes represent a relationship hanging off a row. Both are expandable:
// expanding a row loads its edges; expanding an edge loads its child rows.
// Children are loaded lazily (children == nil means "not loaded yet").
type expNode struct {
	kind     nodeKind
	depth    int
	parent   *expNode
	expanded bool
	loading  bool       // a child load is in flight
	children []*expNode // nil = not loaded; non-nil (incl. empty via synth) = loaded
	err      string     // populates a synthetic error child when set

	// row node fields
	table   string
	rowVals map[string]string // column(lower) → value
	label   string            // display identity (PK tuple or first cell)

	// edge node fields
	edge      relNode
	filterVal string // resolved value to filter targetTable by

	// shared
	drillQuery string // query Enter runs to open this node in the grid ("" = none)
	synthetic  bool   // marker/placeholder node (no expand, inert Enter)
}

// isEdge reports whether this is a relationship edge node.
func (n *expNode) isEdge() bool { return n.kind == nodeEdge }

// RelExplorer is a docked right-slot panel that turns the focused results row
// into a navigable, expandable object graph — the literal "browse a row like a
// folder" model. The root is the focused grid row; its inbound + outbound FK edges are
// the first level. Expand an edge (→) to see the child rows inline; expand a
// child row to see its edges, and so on, depth-capped. Enter opens a node's
// data in the grid (a specific row, or all of an edge's children) and re-roots
// the tree there. The indentation is the breadcrumb — you never lose context,
// which is the flaw the earlier drill-in/back model had.
type RelExplorer struct {
	visible bool
	root    *expNode
	cursor  int // index into the flattened visible list
	scroll  int
	width   int
	height  int

	depth    int  // queryStack depth at last root load (breadcrumb hint)
	loading  bool // initial root load in flight
	err      string
	emptyMsg string // why there is no root (no source table, etc.)

	docked  bool   // panel mode: rendered in the right slot, cursor-driven
	anchor  string // the results row (table + PK tuple) the current tree is rooted at
	focused bool   // mirror of Model.focus == FocusExplorer, for the border color
}

// NewRelExplorer returns a hidden explorer panel.
func NewRelExplorer() RelExplorer { return RelExplorer{} }

func (e RelExplorer) IsVisible() bool { return e.visible }
func (e RelExplorer) IsDocked() bool   { return e.docked }
// ShowDocked reveals the explorer as a right-slot panel (non-modal,
// cursor-driven).
func (e *RelExplorer) ShowDocked() { e.visible = true; e.docked = true }
func (e *RelExplorer) Hide()       { e.visible = false; e.docked = false }

// SetSize sets the panel's exterior dimensions (including border).
func (e *RelExplorer) SetSize(w, h int) { e.width = w; e.height = h }

func (e *RelExplorer) contentHeight() int {
	h := e.height - borderOverhead
	if h < 1 {
		h = 1
	}
	return h
}

// nodeViewport is the number of tree lines that fit in the panel (its full
// allocated content height).
func (e RelExplorer) nodeViewport() int {
	vh := e.contentHeight()
	if vh < 1 {
		vh = 1
	}
	return vh
}

// adjustScroll keeps the visible window FIXED (a stable page) while the cursor
// is inside it, and only flips the page when the cursor leaves it — landing the
// cursor at the opposite edge of the new page so it has room to roam before the
// next flip. This is the "keep it fixed, scroll when you hit the edge" model:
// unlike bottom-pinning, it does not scroll on every cursor move once you near
// the edge. n is the total visible-node count, used to clamp the last page.
func (e *RelExplorer) adjustScroll(vh, n int) {
	if vh <= 0 {
		return
	}
	switch {
	case e.cursor < e.scroll:
		// Exited the top: flip back one page, cursor at the bottom of it.
		e.scroll = e.cursor - vh + 1
	case e.cursor >= e.scroll+vh:
		// Exited the bottom: flip forward one page, cursor at the top of it.
		e.scroll = e.cursor
	}
	maxScroll := n - vh
	if maxScroll < 0 {
		maxScroll = 0
	}
	if e.scroll > maxScroll {
		e.scroll = maxScroll
	}
	if e.scroll < 0 {
		e.scroll = 0
	}
}

// clampScroll re-clamps both cursor and scroll after a tree change (load,
// expand, collapse). It also pins the cursor back into range if the visible
// list shrank under it.
func (e *RelExplorer) clampScroll() {
	n := len(e.visibleNodes())
	if e.cursor >= n {
		e.cursor = n - 1
	}
	if e.cursor < 0 {
		e.cursor = 0
	}
	e.adjustScroll(e.nodeViewport(), n)
}

// markLoading clears the tree for an in-flight root load (open, drill-in,
// back, retarget) — the next explorerLoadedMsg repopulates root.
func (e *RelExplorer) markLoading() {
	e.root = nil
	e.cursor = 0
	e.scroll = 0
	e.loading = true
	e.err = ""
	e.emptyMsg = ""
}

// applyRoot installs a freshly loaded root (the focused row + its first-level
// edges), resetting the cursor to the root.
func (e *RelExplorer) applyRoot(root *expNode, depth int) {
	e.loading = false
	e.err = ""
	e.root = root
	e.depth = depth
	e.cursor = 0
	e.scroll = 0
	e.clampScroll()
}

// applyEmpty shows a reason instead of a tree (no source table, no row, etc.).
func (e *RelExplorer) applyEmpty(depth int, msg string) {
	e.loading = false
	e.root = nil
	e.depth = depth
	e.emptyMsg = msg
	e.cursor = 0
	e.scroll = 0
}

// applyRootError shows a root-load failure.
func (e *RelExplorer) applyRootError(depth int, err error) {
	e.loading = false
	e.root = nil
	e.depth = depth
	if err != nil {
		e.err = err.Error()
	}
}

// applyChildren attaches lazily-loaded children to a node, wiring parent/depth.
// Called from explorerChildrenMsg; the node may have been collapsed while the
// load was in flight — we still attach so a re-expand is instant.
func (e *RelExplorer) applyChildren(parent *expNode, children []*expNode) {
	parent.loading = false
	parent.err = ""
	for _, c := range children {
		c.parent = parent
		c.depth = parent.depth + 1
	}
	parent.children = children
	e.clampScroll()
}

// applyFold handles an expansion that has nothing to show — a pure back-edge
// (every child an ancestor) or a row with no relationships worth listing.
// Rather than render an empty list or a marker, the node is folded back up so
// the tree shows nothing for it.
func (e *RelExplorer) applyFold(parent *expNode) {
	parent.loading = false
	parent.expanded = false
	parent.children = nil
	e.clampScroll()
}

// applyChildrenError records a child-load failure as a synthetic child node.
func (e *RelExplorer) applyChildrenError(parent *expNode, err error) {
	parent.loading = false
	msg := "error loading children"
	if err != nil {
		msg = err.Error()
	}
	parent.children = []*expNode{synthNode("✗ "+msg, parent)}
	e.clampScroll()
}

// synthNode builds an inert placeholder child (no expand, no drill) used for
// "no relationships", "no rows", "+N more", and load errors.
func synthNode(label string, parent *expNode) *expNode {
	n := &expNode{kind: nodeRow, synthetic: true, label: label}
	if parent != nil {
		n.parent = parent
		n.depth = parent.depth + 1
	}
	return n
}

// visibleNodes flattens the tree into the on-screen list, descending only into
// expanded subtrees. The cursor indexes this slice.
func (e RelExplorer) visibleNodes() []*expNode {
	if e.root == nil {
		return nil
	}
	var out []*expNode
	var walk func(n *expNode)
	walk = func(n *expNode) {
		out = append(out, n)
		if n.expanded && n.children != nil {
			for _, c := range n.children {
				walk(c)
			}
		}
	}
	walk(e.root)
	return out
}

// selectedNode returns the node under the cursor, or nil.
func (e RelExplorer) selectedNode() *expNode {
	v := e.visibleNodes()
	if e.cursor < 0 || e.cursor >= len(v) {
		return nil
	}
	return v[e.cursor]
}

// cursorToNode moves the cursor to a specific node (used by ← to jump to parent).
func (e *RelExplorer) cursorToNode(n *expNode) {
	for i, x := range e.visibleNodes() {
		if x == n {
			e.cursor = i
			e.clampScroll()
			return
		}
	}
}

// cursorToFirstChild moves the cursor to a node's first visible child (used by
// → on an already-expanded node).
func (e *RelExplorer) cursorToFirstChild(parent *expNode) {
	for i, x := range e.visibleNodes() {
		if x.parent == parent {
			e.cursor = i
			e.clampScroll()
			return
		}
	}
}

// Update handles in-panel vertical movement (j/k/g/G/ctrl+d/u) over the
// flattened visible list. Expand/collapse/activate are Model methods handled
// by the app dispatch, since they issue async loads or navigate the grid.
func (e RelExplorer) Update(msg tea.KeyMsg) RelExplorer {
	n := len(e.visibleNodes())
	vh := e.nodeViewport()
	clamp := func() {
		if e.cursor >= n {
			e.cursor = n - 1
		}
		if e.cursor < 0 {
			e.cursor = 0
		}
	}
	switch msg.String() {
	case "j", "down":
		if e.cursor < n-1 {
			e.cursor++
			e.adjustScroll(vh, n)
		}
	case "k", "up":
		if e.cursor > 0 {
			e.cursor--
			e.adjustScroll(vh, n)
		}
	case "g":
		e.cursor = 0
		e.scroll = 0
	case "G":
		e.cursor = n - 1
		e.adjustScroll(vh, n)
	case "ctrl+d":
		e.cursor += vh / 2
		clamp()
		e.adjustScroll(vh, n)
	case "ctrl+u":
		e.cursor -= vh / 2
		clamp()
		e.adjustScroll(vh, n)
	}
	return e
}

// View renders just the windowed tree — no title or footer; the panel is the
// navigable object graph itself. It fills its fixed allocated height exactly,
// with no reserved footer padding.
func (e RelExplorer) View() string {
	vh := e.nodeViewport()
	body := e.bodyLines()
	for len(body) < vh {
		body = append(body, "")
	}
	// Render an exact e.width × e.height panel with no interior padding, so the
	// tree's chevrons anchor flush to the left border rather than sitting in a
	// panel gutter (every line carries its own glyph, so a gutter would only
	// push them right). lipgloss Width excludes border: Width(e.width-
	// borderOverhead) leaves e.width-2 = inner for text, and the border brings
	// the total to e.width. Height excludes border, so Height(nodeViewport) +
	// border = e.height. This lets the docked slot place View() directly (no
	// second border).
	return lipgloss.NewStyle().
		Width(e.width - borderOverhead).
		Height(vh).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(e.borderColor()).
		Render(strings.Join(body, "\n"))
}

// borderColor mirrors the inspector/assistant: the accent color when this
// panel holds focus, the dim unfocused color otherwise.
func (e RelExplorer) borderColor() lipgloss.Color {
	if e.focused {
		return colorPrimary
	}
	return colorBorderUnfocused
}

// bodyLines returns the windowed, rendered tree (or a status line).
func (e RelExplorer) bodyLines() []string {
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	switch {
	case e.loading:
		return []string{muted.Render("loading…")}
	case e.err != "":
		return []string{lipgloss.NewStyle().Foreground(colorError).Render(e.err)}
	case e.root == nil:
		msg := e.emptyMsg
		if msg == "" {
			msg = "no relationships"
		}
		return []string{muted.Render(msg)}
	}

	inner := e.width - 2 // border only (no interior padding); text anchors flush left
	if inner < 10 {
		inner = 10
	}
	vis := e.visibleNodes()
	all := make([]string, 0, len(vis))
	for _, n := range vis {
		all = append(all, n.renderLine(e.cursorNodeIndex(n), inner))
	}
	vh := e.nodeViewport()
	start := e.scroll
	if start < 0 {
		start = 0
	}
	end := start + vh
	if end > len(all) {
		end = len(all)
	}
	if start > end {
		start = end
	}
	return all[start:end]
}

// cursorNodeIndex is the index of a node in the visible list, or -1. Used only
// to decide whether to render a node as selected.
func (e RelExplorer) cursorNodeIndex(n *expNode) int {
	for i, x := range e.visibleNodes() {
		if x == n {
			if i == e.cursor {
				return i
			}
			return -1
		}
	}
	return -1
}

// renderLine formats one tree node with depth indentation, an expand glyph,
// and a kind-specific label. Each line carries 1 cell of padding per side via
// the line style (not the panel) so the chevron sits one cell in from the
// border — the same spacing as the sidebar/table explorer — while a selected
// line's highlight fills through that padding to reach the border. Depth
// indentation is the only thing that shifts a line further right.
func (n *expNode) renderLine(selectedIdx int, inner int) string {
	indent := strings.Repeat("  ", n.depth)
	glyph := icons.collapsed
	switch {
	case n.loading:
		glyph = "⟳"
	case n.expanded:
		glyph = icons.expanded
	case n.synthetic:
		glyph = "·"
	}
	content := indent + glyph + " " + n.displayLabel()
	// inner-2 reserves one cell of padding per side, applied by the line style.
	body := truncateCell(content, inner-2)
	if selectedIdx >= 0 {
		return lipgloss.NewStyle().
			Background(colorPrimary).Foreground(colorBg).
			Padding(0, 1).
			Render(body)
	}
	style := lipgloss.NewStyle().Foreground(colorFg)
	if n.synthetic {
		style = lipgloss.NewStyle().Foreground(colorMuted)
	}
	return style.Padding(0, 1).Render(body)
}

// displayLabel builds the text after the glyph for each node kind.
func (n *expNode) displayLabel() string {
	if n.isEdge() {
		count := n.edge.count
		if count == "" {
			count = "?"
		}
		return fmt.Sprintf("%s (%s)", n.edge.targetTable, count)
	}
	// row node
	if n.depth == 0 {
		return n.table + n.label // root: "users · #1" (label carries " · …")
	}
	return n.label
}

func isNumericCount(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// padRight and runeLen are package-level helpers (explain_panel.go /
// results_table.go); the explorer reuses them for alignment.
