package ui

import (
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
type palette struct {
	visible  bool
	input    string
	cursor   int
	items    []paletteItem
	filtered []paletteItem
}

// maxPaletteItems is the maximum number of results shown at once.
const maxPaletteItems = 16

const (
	maxPaletteBookmarks = 40
	maxPaletteQueryLen  = 72
)

// Open shows the palette, building items from the keybinding registry plus
// optional jump targets in src.
func (p *palette) Open(src paletteJumpSrc) {
	p.visible = true
	p.input = ""
	p.cursor = 0
	p.items = buildPaletteItems(src)
	p.filtered = p.items
}

// Hide hides the palette.
func (p *palette) Hide() { p.visible = false }

// IsVisible reports whether the palette is shown.
func (p palette) IsVisible() bool { return p.visible }

// buildPaletteItems flattens the keybinding registry and jump targets into
// palette entries. Bindings come first so an empty filter still feels like the
// command palette; jump rows follow and are easy to reach by typing.
func buildPaletteItems(src paletteJumpSrc) []paletteItem {
	var items []paletteItem
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
	return items
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
	"==":  {"=", "="},
	"g d": {"g", "d"},
	"g b": {"g", "b"},
	"g r": {"g", "r"},
	"g f": {"g", "f"},
	"g s": {"g", "s"},
	"g e": {"g", "e"},
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
// binding replays its one token. Multi-token "alternative" bindings (e.g.
// "g t / g T", "ctrl+e / \") have no single replay sequence and return nil —
// they'd need to be split into separate one-action entries to become
// palette-reachable.
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
		p.filtered = p.items
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

	keyW := 0
	for _, it := range p.filtered {
		if w := runeLen(it.display); w > keyW {
			keyW = w
		}
	}
	if keyW < 6 {
		keyW = 6
	}

	start := 0
	if p.cursor >= maxPaletteItems {
		start = p.cursor - maxPaletteItems + 1
	}
	end := start + maxPaletteItems
	if end > len(p.filtered) {
		end = len(p.filtered)
	}

	var lines []string
	for i := start; i < end; i++ {
		it := p.filtered[i]
		desc, section := fitPaletteRow(it.desc, it.section, keyW, innerW)
		key := it.display + strings.Repeat(" ", keyW-runeLen(it.display))
		var line string
		if i == p.cursor {
			line = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBg).
				Render("❯ " + key + "  " + desc + sectionSuffix(section))
		} else {
			keyStr := lipgloss.NewStyle().Foreground(colorPrimary).Render(key)
			descStr := lipgloss.NewStyle().Foreground(colorLabel).Render(desc)
			secStr := ""
			if section != "" {
				secStr = "  " + lipgloss.NewStyle().Foreground(colorMuted).Render(section)
			}
			line = "  " + keyStr + "  " + descStr + secStr
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, mutedStyle.Render("  no matches"))
	}
	// Pad to a fixed row count so the panel height never changes.
	for len(lines) < maxPaletteItems {
		lines = append(lines, "")
	}

	prompt := renderPalettePrompt(p.input, true)

	body := prompt + "\n" + strings.Join(lines, "\n")

	panel := lipgloss.NewStyle().
		Width(width-2).
		Height(height-2).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(body)

	return panel
}

// fitPaletteRow clamps desc (and drops/truncates section if needed) so
// "❯ " + key + "  " + desc + "  " + section fits in innerW columns.
func fitPaletteRow(desc, section string, keyW, innerW int) (string, string) {
	const prefix = 2 // "❯ " / "  "
	const gap = 2    // between key and desc
	fixed := prefix + keyW + gap
	budget := innerW - fixed
	if budget < 4 {
		budget = 4
	}
	if section == "" {
		return clampPaletteText(desc, budget), ""
	}
	secNeed := 2 + runeLen(section) // "  " + section
	if budget > secNeed+4 {
		return clampPaletteText(desc, budget-secNeed), section
	}
	// Not enough room for the section label — keep the description.
	return clampPaletteText(desc, budget), ""
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
