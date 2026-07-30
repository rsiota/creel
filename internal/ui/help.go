package ui

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HelpPanel renders a full-screen, scrollable, tabbed help overlay:
//
//   - Keys     — every keybinding (registry()), arranged in as few columns as
//     fit the viewport so descriptions align, with more columns
//     added only when the content would otherwise scroll forever.
//   - Commands — every ":" command (exCommands()), one per line at full width
//     with descriptions wrapped, so nothing is truncated.
//
// Both pages scroll vertically (↑/↓ or j/k, PgUp/PgDn, g/G). Tab / shift+tab
// switch pages; ? or esc (or any unmapped key) closes it.
//
// Search (/): typing `/query` live-highlights matches on the current page and
// scrolls the first match into view; n / N cycle matches; esc clears. Scrolling
// is the overflow safety net: offsets are clamped at render time, so the panel
// never overflows its borders regardless of terminal size.
type HelpPanel struct {
	visible bool
	page    int // helpPageKeys | helpPageCommands
	keysOff int // scroll offset (lines) for the Keys page
	cmdsOff int // scroll offset (lines) for the Commands page
	width   int
	height  int

	// Search state. query is the active pattern ("" = no search); typing is
	// true while the / prompt has focus. matchRe is the compiled case-
	// insensitive regex (nil when query is ""). matchIdx is the cursor into
	// the match list (recomputed on demand from the current page's rows).
	query   string
	typing  bool
	matchRe *regexp.Regexp
	matchIdx int
}

const (
	helpPageKeys = iota
	helpPageCommands
	helpPageCount
)

// NewHelpPanel creates a hidden help panel.
func NewHelpPanel() HelpPanel {
	return HelpPanel{}
}

// Toggle shows or hides the help panel. Opening always resets to the Keys page
// at the top, so re-opening feels fresh rather than resuming a mid-scroll.
func (h *HelpPanel) Toggle() {
	if h.visible {
		h.visible = false
		return
	}
	h.Show()
}

// Show forces the panel visible, on the Keys page, scrolled to the top.
func (h *HelpPanel) Show() {
	h.visible = true
	h.page = helpPageKeys
	h.keysOff = 0
	h.cmdsOff = 0
	h.clearSearch()
}

// Hide forces the panel hidden.
func (h *HelpPanel) Hide() { h.visible = false }

// IsVisible reports whether the help panel is shown.
func (h HelpPanel) IsVisible() bool { return h.visible }

// SetSize stores the terminal dimensions for sizing/scrolling the overlay,
// clamping the current page's offset so a shrink can't leave it past the end.
func (h *HelpPanel) SetSize(width, height int) {
	h.width = width
	h.height = height
}

// Typing reports whether the / search prompt has focus (so the caller can keep
// the help overlay open even on keys it would otherwise dismiss).
func (h HelpPanel) Typing() bool { return h.typing }

// HandleKey routes a keypress to the open help overlay. It returns true for
// navigation keys (scroll / page-switch / search), which the caller treats as
// consumed (the overlay stays open). It returns false for close keys (esc, ?,
// q) AND for any unmapped key — the caller then hides the overlay, preserving
// the old "any key dismisses" feel while adding navigation.
func (h *HelpPanel) HandleKey(msg tea.KeyMsg) bool {
	// While the / prompt is focused, every key is consumed by the search.
	if h.typing {
		switch msg.String() {
		case "esc", "ctrl+c":
			h.clearSearch()
			return true
		case "enter":
			h.typing = false
			h.scrollToCurrentMatch()
			return true
		case "backspace":
			if len(h.query) > 0 {
				h.query = h.query[:len(h.query)-1]
				h.afterQueryChange()
			}
			return true
		case "up", "down", "left", "right", "pgup", "pgdown", "home", "end":
			return true // swallow navigation while typing
		}
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			h.query += msg.String()
			h.afterQueryChange()
			return true
		}
		return true
	}

	switch msg.String() {
	case "esc", "?", "q", "ctrl+c":
		return false
	case "/":
		// Start a fresh search (vim clears the pattern on a new /).
		h.typing = true
		h.query = ""
		h.matchRe = nil
		h.matchIdx = 0
		return true
	case "tab":
		h.page = (h.page + 1) % helpPageCount
		h.afterPageChange()
		return true
	case "shift+tab":
		h.page = (h.page - 1 + helpPageCount) % helpPageCount
		h.afterPageChange()
		return true
	case "n":
		// Only consumed when a search is active; otherwise falls through to
		// "unmapped → dismiss" (the old behaviour).
		if h.matchRe != nil {
			h.advanceMatch(1)
			return true
		}
	case "N":
		if h.matchRe != nil {
			h.advanceMatch(-1)
			return true
		}
	case "j", "down":
		h.setCurOff(h.curOff() + 1)
		return true
	case "k", "up":
		h.setCurOff(h.curOff() - 1)
		return true
	case "pgdown", "ctrl+d", " ", "f":
		h.setCurOff(h.curOff() + h.scrollPage())
		return true
	case "pgup", "ctrl+u", "b":
		h.setCurOff(h.curOff() - h.scrollPage())
		return true
	case "g":
		h.setCurOff(0)
		return true
	case "G":
		h.setCurOff(h.maxOff()) // jump to the bottom
		return true
	}
	return false
}

// curOff / setCurOff read/write the offset for the active page. The lower bound
// is clamped here; the upper bound is clamped in View (it depends on content
// length, which is only known once the page is rendered).
func (h HelpPanel) curOff() int {
	if h.page == helpPageCommands {
		return h.cmdsOff
	}
	return h.keysOff
}

func (h *HelpPanel) setCurOff(v int) {
	if v < 0 {
		v = 0
	}
	// Clamp the upper bound too, so the stored offset always tracks the
	// rendered position. Without this, G parked a sentinel (1<<30) past the
	// end and every later up-scroll / j was re-clamped to the same bottom line
	// — scrolling looked frozen until the offset climbed back into range.
	if max := h.maxOff(); v > max {
		v = max
	}
	if h.page == helpPageCommands {
		h.cmdsOff = v
	} else {
		h.keysOff = v
	}
}

// maxOff is the largest valid scroll offset for the active page: the page's
// line count minus the viewport height. pageLines depends on the (width-
// derived) content width, so this mirrors View's clamping and lets the scroll
// and jump handlers keep the stored offset in range rather than deferring it
// to render time.
func (h HelpPanel) maxOff() int {
	off := len(h.pageRows(helpContentWidth(h.width))) - h.scrollPage()
	if off < 0 {
		return 0
	}
	return off
}

// scrollPage is the number of lines PgUp/PgDn jump — the viewport height.
func (h HelpPanel) scrollPage() int {
	v := h.height - 2 - 2*helpPadY - 6
	if v < 4 {
		return 4
	}
	return v
}

// ScrollBy moves the active page's viewport by delta lines (negative = up).
// The offset is clamped at render time, so callers (e.g. the mouse wheel) can
// pass unbounded deltas. Used by mouse-wheel scrolling.
func (h *HelpPanel) ScrollBy(delta int) {
	h.setCurOff(h.curOff() + delta)
}

// rebuildMatchRe compiles the active query into a case-insensitive regex. An
// empty/whitespace query clears the regex (no search). regexp.QuoteMeta keeps
// it a literal substring match (no accidental regex metacharacters).
func (h *HelpPanel) rebuildMatchRe() {
	q := strings.TrimSpace(h.query)
	if q == "" {
		h.matchRe = nil
		return
	}
	h.matchRe = regexp.MustCompile("(?i)" + regexp.QuoteMeta(q))
}

// afterQueryChange is called whenever the query text changes (typing or
// backspace): recompile the regex, reset the cursor to the first match, and
// scroll it into view.
func (h *HelpPanel) afterQueryChange() {
	h.rebuildMatchRe()
	h.matchIdx = 0
	h.scrollToCurrentMatch()
}

// afterPageChange is called when switching tabs: the match list is page-
// specific, so reset the cursor and recenter on the first match of the new
// page (the query itself stays active).
func (h *HelpPanel) afterPageChange() {
	h.matchIdx = 0
	h.scrollToCurrentMatch()
}

func (h *HelpPanel) clearSearch() {
	h.query = ""
	h.typing = false
	h.matchRe = nil
	h.matchIdx = 0
}

// currentRows returns the rows of the active page at the stored width.
func (h HelpPanel) currentRows() []helpRow {
	return h.pageRows(helpContentWidth(h.width))
}

// pageRows returns every row of the requested page's body (before slicing to
// the viewport). Both pages are single full-width columns: the Keys page is a
// cheat sheet (one row per binding), the Commands page wraps long descriptions.
func (h HelpPanel) pageRows(contentW int) []helpRow {
	if h.page == helpPageCommands {
		return renderCommandsRows(contentW)
	}
	return renderKeysRows(contentW)
}

// matches returns the line indices on the current page whose searchable text
// matches the active query (empty when there is no query or no hits).
func (h HelpPanel) matches() []int {
	if h.matchRe == nil {
		return nil
	}
	rows := h.currentRows()
	var out []int
	for i, r := range rows {
		if h.matchRe.MatchString(r.searchText()) {
			out = append(out, i)
		}
	}
	return out
}

// currentMatchLine returns the absolute line index of the current match, or -1.
func (h HelpPanel) currentMatchLine() int {
	ms := h.matches()
	if len(ms) == 0 {
		return -1
	}
	idx := h.matchIdx
	if idx < 0 || idx >= len(ms) {
		idx = 0
	}
	return ms[idx]
}

// advanceMatch moves the match cursor by dir (+1 = next, -1 = prev), wrapping
// around, and scrolls the new match into view.
func (h *HelpPanel) advanceMatch(dir int) {
	ms := h.matches()
	if len(ms) == 0 {
		return
	}
	if h.matchIdx < 0 || h.matchIdx >= len(ms) {
		h.matchIdx = 0
	}
	n := len(ms)
	h.matchIdx = ((h.matchIdx+dir)%n + n) % n
	h.scrollToCurrentMatch()
}

// scrollToCurrentMatch scrolls the active page so the current match is in the
// upper third of the viewport (a little context above, most below). No-op when
// there is no match.
func (h *HelpPanel) scrollToCurrentMatch() {
	target := h.currentMatchLine()
	if target < 0 {
		return
	}
	vp := h.scrollPage()
	off := target - vp/3
	if off < 0 {
		off = 0
	}
	h.setCurOff(off)
}

// View renders the help overlay, sized to fit the terminal.
func (h HelpPanel) View() string {
	if !h.visible {
		return ""
	}

	contentW := helpContentWidth(h.width)
	viewportH := h.height - 2 - 2*helpPadY - 6
	if viewportH < 4 {
		viewportH = 4
	}

	rows := h.pageRows(contentW)
	curMatch := h.currentMatchLine()

	// Clamp the offset now that the content length is known, then slice.
	maxOff := len(rows) - viewportH
	if maxOff < 0 {
		maxOff = 0
	}
	off := h.curOff()
	if off > maxOff {
		off = maxOff
	}
	end := off + viewportH
	if end > len(rows) {
		end = len(rows)
	}

	var bodyVisible []string
	for i := off; i < end; i++ {
		bodyVisible = append(bodyVisible, renderHelpRow(rows[i], h.matchRe, i == curMatch))
	}
	for len(bodyVisible) < viewportH {
		bodyVisible = append(bodyVisible, "")
	}
	body := strings.Join(bodyVisible, "\n")

	title := "Keybindings"
	if h.page == helpPageCommands {
		title = "Commands"
	}
	header := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(title)
	tabbar := h.renderTabBar()
	pos := h.renderStatusLine(off, end, len(rows), maxOff)

	// Always seven layers (pos is "" when nothing scrolls and there's no
	// search) so the overlay height is identical on both pages — the top
	// border stays put when switching tabs.
	layers := []string{header, "", tabbar, "", body, "", pos}
	content := lipgloss.JoinVertical(lipgloss.Left, layers...)

	// The overlay fills the screen below the status bar: a full-width/full-
	// height bordered box (border is drawn outside the Width/Height, so pass the
	// terminal size minus the border's 2 rows/cols).
	panel := lipgloss.NewStyle().
		Width(h.width-2).
		Height(h.height-2).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(helpPadY, helpPadX).
		Render(content)

	return panel
}

// renderStatusLine renders the bottom-of-overlay line: the / search prompt
// while typing, the match readout while a search is active, otherwise the
// scroll-position readout.
func (h HelpPanel) renderStatusLine(off, end, total, maxOff int) string {
	switch {
	case h.typing:
		// Mirror the ":" ex-line prompt: primary "/" + the query + an accent
		// underline cursor.
		return lipgloss.NewStyle().Foreground(colorPrimary).Render("/"+h.query) +
			lipgloss.NewStyle().Foreground(colorAccent).Underline(true).Render(" ")
	case h.matchRe != nil:
		ms := h.matches()
		if len(ms) == 0 {
			return mutedStyle.Render("/" + h.query + "  no matches")
		}
		idx := h.matchIdx + 1
		if idx < 1 || idx > len(ms) {
			idx = 1
		}
		return mutedStyle.Render("/" + h.query + "  match " + itoa(idx) + "/" + itoa(len(ms)) + "  (n/N)")
	default:
		if maxOff > 0 {
			pct := off * 100 / maxOff
			return mutedStyle.Render("scroll " + itoa(off+1) + "–" + itoa(end) + "/" + itoa(total) + "  " + itoa(pct) + "%")
		}
		return ""
	}
}

// helpTabRow is the screen row (0-indexed, from the top of the overlay area)
// on which the tab bar renders: border(1) + padding-top(helpPadY) + header(1)
// + blank(1). The overlay fills the screen below the status bar and starts at
// the top row, so this is constant.
const (
	helpPadX   = 2 // inner horizontal padding of the overlay
	helpPadY   = 1 // inner vertical padding
	helpTabRow = helpPadY + 3
)

// helpTabLabel is the text of tab i (without the surrounding spaces the
// renderer adds). Shared by renderTabBar and helpTabAt so mouse clicks track
// the rendered layout exactly.
func helpTabLabel(i int) string {
	if i == helpPageCommands {
		return "Commands"
	}
	return "Keys"
}

// renderTabBar renders the page tabs with the active one highlighted.
func (h HelpPanel) renderTabBar() string {
	var parts []string
	for i := 0; i < helpPageCount; i++ {
		l := helpTabLabel(i)
		var s string
		if i == h.page {
			s = lipgloss.NewStyle().
				Bold(true).
				Background(colorPrimary).
				Foreground(colorBg).
				Render(" " + l + " ")
		} else {
			s = lipgloss.NewStyle().Foreground(colorMuted).Render(" " + l + " ")
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// helpContentWidth is the panel's inner content width, shared by View and the
// mouse hit-test so they agree on the panel's on-screen size.
func helpContentWidth(termW int) int {
	w := termW - 2 - 2*helpPadX // 2 = border, 2*helpPadX = padding
	if w < 40 {
		w = 40
	}
	return w
}

// helpPanelLeft is the screen column of the overlay's left border. The
// overlay fills the screen width, so the border starts at column 0; termW is
// kept for callers/tests but unused.
func helpPanelLeft(termW int) int {
	return 0
}

// helpTabAt returns the tab index at screen column x, or -1 if x isn't on a
// tab. panelLeft is the panel's left offset; tabs start at panelLeft + border
// + padding-left and are laid out as " <label> " separated by single spaces
// (matching renderTabBar).
func helpTabAt(panelLeft, x int) int {
	cur := panelLeft + 1 + helpPadX // border + left padding
	for i := 0; i < helpPageCount; i++ {
		w := 1 + len(helpTabLabel(i)) + 1
		if x >= cur && x < cur+w {
			return i
		}
		cur += w + 1 // tab + separator space
	}
	return -1
}

// SetPage switches the help overlay to page p (no-op if out of range). Used
// by the mouse-click handler.
func (h *HelpPanel) SetPage(p int) {
	if p >= 0 && p < helpPageCount {
		h.page = p
		h.afterPageChange()
	}
}

// ── Row model ──────────────────────────────────────────────────────────────

// helpSegment is one styled run of text within a help row. style is the
// lipgloss style to render it with (a zero-value style renders as plain text).
type helpSegment struct {
	text  string
	style lipgloss.Style
}

// helpRow is one rendered line, as a sequence of segments. Splitting a line
// into segments lets search match on the concatenated plain text and highlight
// matched substrings per-segment (so it lands on the actual characters, not on
// ANSI escapes) without losing each part's colour.
type helpRow []helpSegment

// searchText returns the concatenated plain text of a row, for matching.
func (r helpRow) searchText() string {
	var b strings.Builder
	for _, s := range r {
		b.WriteString(s.text)
	}
	return b.String()
}

// helpSearchStyles builds the two highlight styles fresh on each render. They
// must NOT be package-level vars: those would capture the color vars at
// package-load time, before applyPalette() (in init()) sets them — leaving
// the styles colourless (the same reason styles.go rebuilds its sb*/shared
// styles inside applyPalette).
//
// Highlight is applied to the matched SUBSTRING only, never the rest of the
// line. Both styles put light text on a blue background (a Doom/Nord-style
// search highlight), but at different intensities for contrast:
//   - current match: bright primary blue + dark bold text (sharp, ~6:1);
//   - other matches: dark search blue + white text (softer, still ~5:1).
// Using colorPrimary for the others too made white-on-light-blue unreadable
// (~1.6:1), so the non-current background stays on the darker colorSearch.
func helpSearchStyles() (match, curMatch lipgloss.Style) {
	return lipgloss.NewStyle().Background(colorSearch).Foreground(colorFg),
		lipgloss.NewStyle().Background(colorPrimary).Foreground(colorBg).Bold(true)
}

// renderHelpRow renders a row, optionally highlighting query matches. With no
// regex it renders each segment in its own style. When searching, each matched
// substring gets a background (subtle for non-current matches, strong for the
// current match); the rest of the line keeps its normal styling.
func renderHelpRow(row helpRow, re *regexp.Regexp, isCurrent bool) string {
	match, curMatch := helpSearchStyles()
	var b strings.Builder
	for _, seg := range row {
		b.WriteString(renderHelpSegment(seg, re, isCurrent, match, curMatch))
	}
	return b.String()
}

// renderHelpSegment renders one segment. The segment keeps its own style for
// all non-matched text; matched substrings use the match style (or curMatch
// when isCurrent). FindAllStringIndex returns byte offsets into the original
// (un-lowered) text, so this is correct for non-ASCII descriptions.
func renderHelpSegment(seg helpSegment, re *regexp.Regexp, isCurrent bool, match, curMatch lipgloss.Style) string {
	base := seg.style
	m := match
	if isCurrent {
		m = curMatch
	}
	if re == nil {
		return base.Render(seg.text)
	}
	locs := re.FindAllStringIndex(seg.text, -1)
	if len(locs) == 0 {
		return base.Render(seg.text)
	}
	var b strings.Builder
	prev := 0
	for _, loc := range locs {
		if loc[0] > prev {
			b.WriteString(base.Render(seg.text[prev:loc[0]]))
		}
		b.WriteString(m.Render(seg.text[loc[0]:loc[1]]))
		prev = loc[1]
	}
	if prev < len(seg.text) {
		b.WriteString(base.Render(seg.text[prev:]))
	}
	return b.String()
}

// renderKeysRows lays the keybinding sections out as a single full-width
// column: title, one row per binding (key padded to the global key width,
// description filling the rest), blank separator. Single-column keeps
// descriptions readable (they're truncated only on very narrow terminals) and
// lets scrolling — not cramped columns — absorb the length.
func renderKeysRows(contentW int) []helpRow {
	sections := registry()
	keyW := 0
	for _, s := range sections {
		for _, b := range s.Items {
			if w := runeLen(b.Display); w > keyW {
				keyW = w
			}
		}
	}
	const gap = 4
	descW := contentW - keyW - gap - 2 // 2 leading spaces
	if descW < 8 {
		descW = 8
	}
	labelStyle := lipgloss.NewStyle().Foreground(colorLabel)
	fgStyle := lipgloss.NewStyle().Foreground(colorFg)
	plain := lipgloss.NewStyle()
	var out []helpRow
	for _, s := range sections {
		out = append(out, helpRow{{text: s.Title, style: titleStyle}})
		for _, bd := range s.Items {
			key := bd.Display + strings.Repeat(" ", max(0, keyW-runeLen(bd.Display)))
			desc := truncateRunes(bd.Desc, descW)
			out = append(out, helpRow{
				{text: "  ", style: plain},
				{text: key, style: labelStyle},
				{text: strings.Repeat(" ", gap), style: plain},
				{text: desc, style: fgStyle},
			})
		}
		out = append(out, helpRow{}) // blank separator
	}
	return out
}

// renderCommandsRows renders the ":" commands one per line at full width. The
// usage column is sized to the longest usage; descriptions wrap to the remaining
// width (rather than being hard-truncated), so long descriptions stay readable.
func renderCommandsRows(contentW int) []helpRow {
	cmds := exCommands()
	usageW := 0
	for _, c := range cmds {
		if w := runeLen(c.usage); w > usageW {
			usageW = w
		}
	}
	const gap = 4
	descW := contentW - usageW - gap - 2 // 2 leading spaces
	if descW < 8 {
		descW = 8
	}
	indent := strings.Repeat(" ", 2+usageW+gap)
	labelStyle := lipgloss.NewStyle().Foreground(colorLabel)
	fgStyle := lipgloss.NewStyle().Foreground(colorFg)
	plain := lipgloss.NewStyle()

	var out []helpRow
	for _, c := range cmds {
		usage := c.usage + strings.Repeat(" ", max(0, usageW-runeLen(c.usage)))
		wrapped := wrapRunes(c.desc, descW)
		for i, w := range wrapped {
			if i == 0 {
				out = append(out, helpRow{
					{text: "  ", style: plain},
					{text: usage, style: labelStyle},
					{text: strings.Repeat(" ", gap), style: plain},
					{text: w, style: fgStyle},
				})
			} else {
				out = append(out, helpRow{
					{text: indent, style: plain},
					{text: w, style: fgStyle},
				})
			}
		}
	}
	return out
}

// wrapRunes word-wraps s into lines of at most width runes. A single word
// longer than width is left intact on its own line (descriptions are short
// prose, so this is rare). Returns [""] for empty input so callers always get
// at least one line to render.
func wrapRunes(s string, width int) []string {
	if width <= 1 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if runeLen(cur)+1+runeLen(w) <= width {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	lines = append(lines, cur)
	return lines
}

// truncateRunes clips s to max visible runes with a trailing "…" when clipped.
// Used on plain (unstyled) description strings before styling, so rune slicing
// is safe.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runeLen(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) > max-1 {
		r = r[:max-1]
	}
	return string(r) + "…"
}

// itoa is a tiny strconv.Itoa-free int→string for the scroll-position readout
// (help.go otherwise needs no strconv import).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
