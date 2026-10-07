package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// paletteJumpKind identifies a non-keybinding palette action.
type paletteJumpKind int

const (
	paletteJumpNone paletteJumpKind = iota
	paletteJumpTable
	paletteJumpBookmark
	paletteJumpTheme
)

// paletteItem is a single searchable entry in the command palette.
type paletteItem struct {
	display string         // key display (e.g. "X", "ctrl+r") or jump label
	desc    string         // human description
	section string         // section title for context
	replay  []string       // key sequence to replay through dispatch (nil = not a binding)
	jump    paletteJumpKind
	payload string         // table/theme name or full SQL for jump items
}

// paletteJumpSrc supplies connection-scoped targets when opening the palette.
// Themes are always loaded from themeNames(); nil/empty slices omit a category.
type paletteJumpSrc struct {
	Tables    []string
	Bookmarks []string // queries, most-recent first
}

// paletteJumpMsg is emitted when the user confirms a jump-anywhere item.
type paletteJumpMsg struct {
	kind    paletteJumpKind
	payload string
}

// palette is the fuzzy-searchable command palette overlay (Ctrl+P).
// It lists keybindings plus jump targets (tables, bookmarks, themes),
// lets the user fuzzy-filter, and on Enter either replays a binding or jumps.
//
// Contextual mode (g m / :menu) lists only executable bindings for the
// focused panel plus Global — a short action menu, not the full jump catalog.
type palette struct {
	visible      bool
	input        string
	cursor       int
	items        []paletteItem
	filtered     []paletteItem
	contextual   bool   // true when opened via OpenContextual
	contextLabel string // panel section title for the header (e.g. "Results")
}

// maxPaletteItems is the maximum number of results shown at once.
const maxPaletteItems = 16

const (
	maxPaletteBookmarks = 40
	maxPaletteQueryLen  = 72
)

// palettePopupDim is the outer size of the ctrl+p jump-anywhere overlay.
// Kept shorter than the connection-form shell (popupDim) so the prompt plus
// maxPaletteItems rows fill the frame without empty padding at the bottom.
func palettePopupDim() (w, h int) {
	return 71, maxPaletteItems + 3 // items + prompt + border
}

// Open shows the palette, building items from the keybinding registry plus
// optional jump targets in src.
func (p *palette) Open(src paletteJumpSrc) {
	p.visible = true
	p.contextual = false
	p.contextLabel = ""
	p.input = ""
	p.cursor = 0
	p.items = buildPaletteItems(src)
	sortPaletteItems(p.items)
	p.refilter()
}

// OpenContextual shows an action menu for the given registry sections (typically
// the focused panel plus "Global"). Only bindings with a non-nil replay sequence
// are listed — navigation clusters stay on `?`. Section order matches sections;
// within a section, registry order is preserved.
func (p *palette) OpenContextual(sections []string, label string) {
	p.visible = true
	p.contextual = true
	p.contextLabel = label
	p.input = ""
	p.cursor = 0
	p.items = buildContextualPaletteItems(sections)
	p.refilter()
}

// Hide hides the palette.
func (p *palette) Hide() { p.visible = false }

// IsVisible reports whether the palette is shown.
func (p palette) IsVisible() bool { return p.visible }

// IsContextual reports whether the palette was opened as a panel action menu.
func (p palette) IsContextual() bool { return p.contextual }

// Jump-target sections are listed first so Ctrl+P surfaces tables and bookmarks
// before the long keybinding catalog. Themes are omitted from the empty filter
// (see refilter) so ~570 theme names do not bury jump targets and bindings.
var paletteJumpSections = []string{"Tables", "Bookmarks", "Themes"}

// buildPaletteItems flattens jump targets and the keybinding registry into
// palette entries grouped for discoverability.
func buildPaletteItems(src paletteJumpSrc) []paletteItem {
	var items []paletteItem
	for _, t := range src.Tables {
		if t == "" {
			continue
		}
		items = append(items, paletteItem{
			display: "table",
			desc:    t,
			section: "Tables",
			jump:    paletteJumpTable,
			payload: t,
		})
	}
	for i, q := range src.Bookmarks {
		if i >= maxPaletteBookmarks {
			break
		}
		if strings.TrimSpace(q) == "" {
			continue
		}
		items = append(items, paletteItem{
			display: "bookmark",
			desc:    flattenPaletteQuery(q),
			section: "Bookmarks",
			jump:    paletteJumpBookmark,
			payload: q,
		})
	}
	for _, name := range themeNames() {
		items = append(items, paletteItem{
			display: "theme",
			desc:    themeDisplay(name),
			section: "Themes",
			jump:    paletteJumpTheme,
			payload: name,
		})
	}
	for _, sec := range registry() {
		for _, b := range sec.Items {
			items = append(items, paletteItem{
				display: b.Display,
				desc:    b.Desc,
				section: sec.Title,
				replay:  b.replayTokens(),
			})
		}
	}
	return items
}

// buildContextualPaletteItems collects executable bindings from the named
// registry sections, in the order sections is given, preserving registry order
// within each section. Jump targets and non-replayable rows are omitted.
func buildContextualPaletteItems(sections []string) []paletteItem {
	want := make(map[string]int, len(sections))
	for i, s := range sections {
		if _, dup := want[s]; dup {
			continue
		}
		want[s] = i
	}
	bySec := make([][]paletteItem, len(sections))
	for _, sec := range registry() {
		idx, ok := want[sec.Title]
		if !ok {
			continue
		}
		for _, b := range sec.Items {
			replay := b.replayTokens()
			if len(replay) == 0 {
				continue
			}
			bySec[idx] = append(bySec[idx], paletteItem{
				display: b.Display,
				desc:    b.Desc,
				section: sec.Title,
				replay:  replay,
			})
		}
	}
	var items []paletteItem
	for _, group := range bySec {
		items = append(items, group...)
	}
	return items
}

func paletteSectionRank(section string) int {
	for i, s := range paletteJumpSections {
		if s == section {
			return i
		}
	}
	for i, sec := range registry() {
		if sec.Title == section {
			return len(paletteJumpSections) + i
		}
	}
	return len(paletteJumpSections) + len(registry()) + 1
}

func sortPaletteItems(items []paletteItem) {
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := paletteSectionRank(items[i].section), paletteSectionRank(items[j].section)
		if ri != rj {
			return ri < rj
		}
		if items[i].section != items[j].section {
			return items[i].section < items[j].section
		}
		return items[i].desc < items[j].desc
	})
}

// flattenPaletteQuery collapses whitespace to a single line and truncates for
// the palette description column. The full query stays in payload.
func flattenPaletteQuery(q string) string {
	var b strings.Builder
	space := false
	for _, r := range q {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	s := b.String()
	if runeLen(s) <= maxPaletteQueryLen {
		return s
	}
	return truncateCell(s, maxPaletteQueryLen)
}

// chordReplays maps a binding's Display to the explicit key sequence the
// command palette replays for it. It covers chords (g d, g e, …) and double-
// presses (dd, y y, ==) that can't be expressed as a single dispatch token.
// The palette replays these through the normal dispatch with tea.Sequence, so
// the stateful pending-G/pending-D flag set by the first key is consumed by
// the second — there is no parallel code path. Keyed by Display;
// TestChordReplaysAreRealBindings pins that every key is a real binding, so a
// rename can't silently strand a chord.
var chordReplays = map[string][]string{
	"g x": {"g", "x"},
	"g c": {"g", "c"},
	"g m": {"g", "m"},
	"g t": {"g", "t"},
	"g T": {"g", "T"},
	"g g": {"g", "g"},
	"==":  {"=", "="},
	"g d": {"g", "d"},
	"g b": {"g", "b"},
	"g r": {"g", "r"},
	"g R": {"g", "R"},
	"g f": {"g", "f"},
	"g s": {"g", "s"},
	"g e": {"g", "e"},
	"g E": {"g", "E"},
	"g H": {"g", "H"},
	"g /": {"g", "/"},
	"g X": {"g", "X"},
	"dd":  {"d", "d"},
	"y y": {"y", "y"},
	"y r": {"y", "r"},
}

// replayTokens returns the key sequence the command palette should replay to
// invoke this binding, or nil if it isn't directly executable. Chords and
// double-presses (g d, dd, …) are looked up in chordReplays; a single-token
// binding replays its one token. Multi-token rows that still pack several
// unrelated actions into one Display (e.g. "j/k, ↑/↓", "ctrl+h/j/k/l") return
// nil — split them into one-action entries to make them palette-reachable.
func (b Binding) replayTokens() []string {
	if seq, ok := chordReplays[b.Display]; ok {
		return seq
	}
	if len(b.Tokens) != 1 {
		return nil
	}
	t := b.Tokens[0]
	// Detect double-press chords (dd, yy) not listed in chordReplays: the
	// display starts with the token doubled once spaces are removed.
	compact := strings.ReplaceAll(b.Display, " ", "")
	if strings.HasPrefix(compact, t+t) {
		return nil
	}
	return []string{t}
}

// refilter rebuilds the filtered list from the current input using fuzzy
// matching over the description, key display, and section title.
func (p *palette) refilter() {
	if p.input == "" {
		// Empty filter: hide the theme catalog so tables/bookmarks/bindings
		// stay discoverable. Themes still appear once the user types.
		out := make([]paletteItem, 0, len(p.items))
		for _, it := range p.items {
			if it.jump == paletteJumpTheme {
				continue
			}
			out = append(out, it)
		}
		p.filtered = out
		sortPaletteItems(p.filtered)
		p.cursor = 0
		return
	}
	ranked := fuzzyRank(p.input, p.items,
		func(it paletteItem) string {
			// Include payload so theme slugs / full SQL stay searchable even
			// when the visible desc is shortened.
			return it.desc + " " + it.display + " " + it.section + " " + it.payload
		},
		nil)
	p.filtered = make([]paletteItem, len(ranked))
	for i, r := range ranked {
		p.filtered[i] = r.Item
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = max(0, len(p.filtered)-1)
	}
}

// moveCursor adjusts the selection, wrapping around.
func (p *palette) moveCursor(delta int) {
	n := len(p.filtered)
	if n == 0 {
		return
	}
	p.cursor = (p.cursor + delta + n) % n
}

// selectedItem returns the highlighted palette row, or a zero item.
func (p palette) selectedItem() paletteItem {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return paletteItem{}
	}
	return p.filtered[p.cursor]
}

// selectedReplay returns the replay key sequence for the highlighted item, or
// nil if it isn't directly executable.
func (p palette) selectedReplay() []string {
	return p.selectedItem().replay
}

// selectedDisplay returns the display string of the highlighted item.
func (p palette) selectedDisplay() string {
	return p.selectedItem().display
}

// Update processes a keypress while the palette is open. It returns the
// updated palette state and an optional tea.Cmd (non-nil when confirming a
// binding replay or jump-anywhere action).
func (p palette) Update(msg tea.KeyMsg) (palette, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		p.visible = false
		return p, nil
	case "enter":
		it := p.selectedItem()
		p.visible = false
		if it.jump != paletteJumpNone && it.payload != "" {
			kind, payload := it.jump, it.payload
			return p, func() tea.Msg { return paletteJumpMsg{kind: kind, payload: payload} }
		}
		if len(it.replay) == 0 {
			return p, nil
		}
		return p, replayKeySequence(it.replay)
	case "up", "ctrl+p":
		p.moveCursor(-1)
		return p, nil
	case "down", "ctrl+n":
		p.moveCursor(1)
		return p, nil
	case "backspace":
		if len(p.input) > 0 {
			r := []rune(p.input)
			p.input = string(r[:len(r)-1])
			p.refilter()
		}
		return p, nil
	}
	if ch, ok := keyFilterChar(msg); ok {
		p.input += ch
		p.refilter()
	}
	return p, nil
}

// View renders the palette panel (without background — the caller overlays it).
func (p palette) View(width, height int) string {
	if !p.visible {
		return ""
	}

	// Inner content width: panel Width(width-2) with Padding(0, 1).
	innerW := width - 4
	if innerW < 24 {
		innerW = 24
	}

	keyW, descW, secW := paletteColumnWidths(p.items, innerW)

	listMax := maxPaletteItems
	if p.contextual {
		// Reserve one body row for the "Results actions" header.
		listMax = maxPaletteItems - 1
	}
	start := 0
	if p.cursor >= listMax {
		start = p.cursor - listMax + 1
	}
	end := start + listMax
	if end > len(p.filtered) {
		end = len(p.filtered)
	}

	var lines []string
	for i := start; i < end; i++ {
		lines = append(lines, renderPaletteItemLine(p.filtered[i], keyW, descW, secW, i == p.cursor))
	}
	if len(lines) == 0 {
		lines = append(lines, mutedStyle.Render("  no matches"))
	}
	// Pad to a fixed row count so the panel height never changes.
	for len(lines) < listMax {
		lines = append(lines, "")
	}

	prompt := renderPalettePrompt(p.input, true)
	body := prompt + "\n" + strings.Join(lines, "\n")
	if p.contextual {
		header := lipgloss.NewStyle().Foreground(colorMuted).Render(contextualActionHeader(p.contextLabel, len(p.items), len(p.filtered), p.input != ""))
		body = header + "\n" + body
	}

	panel := lipgloss.NewStyle().
		Width(width-2).
		Height(height-2).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(body)

	return panel
}

const (
	paletteKeyColW = 12
	paletteSecColW = 16
)

// paletteColumnWidths returns fixed key/section column widths; description
// absorbs whatever space remains inside innerW.
func paletteColumnWidths(_ []paletteItem, innerW int) (keyW, descW, secW int) {
	const (
		prefix   = 2
		colGap   = 2
		minDescW = 8
	)
	keyW = paletteKeyColW
	secW = paletteSecColW
	fixed := prefix + keyW + colGap + secW + colGap
	descW = innerW - fixed
	if descW < minDescW {
		descW = minDescW
	}
	return keyW, descW, secW
}

func padRunes(s string, width int) string {
	if w := runeLen(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// renderPaletteItemLine renders one palette row in three fixed-width columns:
// key, description, and a right-aligned section label.
func renderPaletteItemLine(it paletteItem, keyW, descW, secW int, selected bool) string {
	const colGap = 2
	prefix := "  "
	if selected {
		prefix = "❯ "
	}
	gap := strings.Repeat(" ", colGap)
	key := padRunes(clampPaletteText(it.display, keyW), keyW)
	desc := padRunes(clampPaletteText(it.desc, descW), descW)
	sec := clampPaletteText(it.section, secW)
	sec = strings.Repeat(" ", secW-runeLen(sec)) + sec
	full := prefix + key + gap + desc + gap + sec

	if selected {
		return lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(colorBg).
			Render(full)
	}
	keyStr := lipgloss.NewStyle().Foreground(colorPrimary).Render(key)
	descStr := lipgloss.NewStyle().Foreground(colorLabel).Render(desc)
	secStr := lipgloss.NewStyle().Foreground(colorMuted).Render(sec)
	return prefix + keyStr + gap + descStr + gap + secStr
}

// fitPaletteRow clamps desc and section for tests and legacy callers.
func fitPaletteRow(desc, section string, keyW, innerW int) (string, string) {
	_, descW, secW := paletteColumnWidths([]paletteItem{
		{display: strings.Repeat("x", keyW), section: section},
	}, innerW)
	return clampPaletteText(desc, descW), clampPaletteText(section, secW)
}

func sectionSuffix(section string) string {
	if section == "" {
		return ""
	}
	return "  " + section
}

// clampPaletteText truncates with an ellipsis when s exceeds width; short
// strings are returned unchanged (no padding).
func clampPaletteText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runeLen(s) <= width {
		return s
	}
	return truncateCell(s, width)
}

// contextualActionHeader is the muted title above the contextual action menu
// prompt, e.g. "Results actions (18)" or "Results actions (3/18)" while
// filtering.
func contextualActionHeader(label string, total, matched int, filtering bool) string {
	if label == "" {
		label = "Panel"
	}
	if filtering {
		return fmt.Sprintf("%s actions (%d/%d)", label, matched, total)
	}
	return fmt.Sprintf("%s actions (%d)", label, total)
}

// renderPalettePrompt renders the chevron-style fuzzy-search prompt used by
// all pickers: a bold "❯ " followed by the current input and a trailing
// cursor. The cursor is a single overlay cell — reverse when resting,
// underline while filtering/typing — so it never shifts the text (an inserted
// glyph like "▏" would).
func renderPalettePrompt(input string, filtering bool) string {
	chevron := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("❯ ")
	text := lipgloss.NewStyle().Foreground(colorFg).Render(input)
	cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
	if filtering {
		cursor = lipgloss.NewStyle().Underline(true).Render(" ")
	}
	return chevron + text + cursor
}

// renderPaletteRow renders a single list row with the palette's selection
// style: blue background for the selected row, plain otherwise. content is
// the pre-formatted line text (markers, checkboxes, etc.).
func renderPaletteRow(content string, selected bool) string {
	if selected {
		return lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(colorBg).
			Render("❯ " + ansi.Strip(content))
	}
	// Explicit theme fg: paintBg fills the theme background under every cell,
	// so unstyled text inherits the terminal default FG and can be illegible
	// on light themes (same class of bug as highlightMatches / CursorLine).
	return lipgloss.NewStyle().Foreground(colorFg).Render("  " + ansi.Strip(content))
}

// renderPaletteRowWithTick renders a row like renderPaletteRow, but places
// the tick (✓ or space) right-aligned within the given width. width is the
// total available content width (the area between the panel's padding); the
// 2-char "❯ "/"  " prefix is reserved internally so the rendered row never
// exceeds width and the tick stays on the same line.
func renderPaletteRowWithTick(content string, tick string, selected bool, width int) string {
	const prefixW = 2 // "❯ " when selected, "  " otherwise
	avail := width - prefixW
	gap := avail - lipgloss.Width(content) - lipgloss.Width(tick)
	if gap < 1 {
		gap = 1
	}
	pad := strings.Repeat(" ", gap)
	if selected {
		line := ansi.Strip(content) + pad + ansi.Strip(tick)
		return lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(colorBg).
			Render("❯ " + line)
	}
	fg := lipgloss.NewStyle().Foreground(colorFg)
	// Match highlighting may already style content; leave those SGR spans
	// alone and only paint plain (unstyled) values with theme fg.
	if ansi.Strip(content) == content {
		return fg.Render("  "+content+pad) + tick
	}
	return fg.Render("  ") + content + fg.Render(pad) + tick
}
