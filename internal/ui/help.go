package ui

import (
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
// switch pages; ? or esc (or any unmapped key) closes it. Scrolling is the
// overflow safety net: offsets are clamped at render time, so the panel never
// overflows its borders regardless of terminal size.
type HelpPanel struct {
	visible bool
	page    int // helpPageKeys | helpPageCommands
	keysOff int // scroll offset (lines) for the Keys page
	cmdsOff int // scroll offset (lines) for the Commands page
	width   int
	height  int
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

// HandleKey routes a keypress to the open help overlay. It returns true for
// navigation keys (scroll / page-switch), which the caller treats as consumed
// (the overlay stays open). It returns false for close keys (esc, ?, q) AND for
// any unmapped key — the caller then hides the overlay, preserving the old
// "any key dismisses" feel while adding navigation.
func (h *HelpPanel) HandleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "esc", "?", "q", "ctrl+c":
		return false
	case "tab":
		h.page = (h.page + 1) % helpPageCount
		return true
	case "shift+tab":
		h.page = (h.page - 1 + helpPageCount) % helpPageCount
		return true
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
	off := len(h.pageLines(helpContentWidth(h.width))) - h.scrollPage()
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

// View renders the help overlay, sized to fit the terminal.
func (h HelpPanel) View() string {
	if !h.visible {
		return ""
	}

	// helpContentWidth shares the panel's inner width with the mouse hit-test.
	// viewportH leaves room for header + blank + tabbar + blank (above the body)
	// and a blank + pos line (below), plus the overlay's border and vertical
	// padding.
	contentW := helpContentWidth(h.width)
	viewportH := h.height - 2 - 2*helpPadY - 6
	if viewportH < 4 {
		viewportH = 4
	}

	bodyAll := h.pageLines(contentW)

	// Clamp the offset now that the content length is known, then slice.
	maxOff := len(bodyAll) - viewportH
	if maxOff < 0 {
		maxOff = 0
	}
	off := h.curOff()
	if off > maxOff {
		off = maxOff
	}
	end := off + viewportH
	if end > len(bodyAll) {
		end = len(bodyAll)
	}
	bodyVisible := bodyAll[off:end]
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
	pos := ""
	if maxOff > 0 {
		pct := off * 100 / maxOff
		pos = mutedStyle.Render("scroll " + itoa(off+1) + "–" + itoa(end) + "/" + itoa(len(bodyAll)) + "  " + itoa(pct) + "%")
	}

	// Always seven layers (pos is "" when nothing scrolls) so the overlay height
	// is identical on both pages — the top border stays put when switching tabs.
	layers := []string{header, "", tabbar, "", body, "", pos}
	content := lipgloss.JoinVertical(lipgloss.Left, layers...)

	// The overlay fills the screen below the status bar: a full-width/full-
	// height bordered box (border is drawn outside the Width/Height, so pass the
	// terminal size minus the border's 2 rows/cols).
	panel := lipgloss.NewStyle().
		Width(h.width-2).
		Height(h.height-2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(helpPadY, helpPadX).
		Render(content)

	return panel
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
	}
}

// pageLines returns every line of the active page's body (before slicing to the
// viewport). Both pages are single full-width columns: the Keys page is a
// cheat sheet (one line per binding, descriptions truncated only on very
// narrow terminals), the Commands page wraps long descriptions.

func (h HelpPanel) pageLines(contentW int) []string {
	if h.page == helpPageCommands {
		return renderCommandsLines(contentW)
	}
	return renderKeysLines(contentW)
}

// renderKeysLines lays the keybinding sections out as a single full-width
// column: title, one line per binding (key padded to the global key width,
// description filling the rest), blank separator. Single-column keeps
// descriptions readable (they're truncated only on very narrow terminals) and
// lets scrolling — not cramped columns — absorb the length.
func renderKeysLines(contentW int) []string {
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
	var out []string
	for _, s := range sections {
		out = append(out, titleStyle.Render(s.Title))
		for _, bd := range s.Items {
			key := bd.Display + strings.Repeat(" ", max(0, keyW-runeLen(bd.Display)))
			keyStr := lipgloss.NewStyle().Foreground(colorLabel).Render(key)
			descStr := lipgloss.NewStyle().Foreground(colorFg).Render(truncateRunes(bd.Desc, descW))
			out = append(out, "  "+keyStr+strings.Repeat(" ", gap)+descStr)
		}
		out = append(out, "")
	}
	return out
}

// renderCommandsLines renders the ":" commands one per line at full width. The
// usage column is sized to the longest usage; descriptions wrap to the remaining
// width (rather than being hard-truncated), so long descriptions stay readable.
func renderCommandsLines(contentW int) []string {
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

	var out []string
	for _, c := range cmds {
		usage := c.usage + strings.Repeat(" ", max(0, usageW-runeLen(c.usage)))
		usageStr := lipgloss.NewStyle().Foreground(colorLabel).Render(usage)
		wrapped := wrapRunes(c.desc, descW)
		for i, w := range wrapped {
			descStr := lipgloss.NewStyle().Foreground(colorFg).Render(w)
			if i == 0 {
				out = append(out, "  "+usageStr+strings.Repeat(" ", gap)+descStr)
			} else {
				out = append(out, indent+descStr)
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
