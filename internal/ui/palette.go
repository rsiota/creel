package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// paletteItem is a single searchable entry in the command palette.
type paletteItem struct {
	display string   // key display (e.g. "X", "ctrl+r")
	desc    string   // human description
	section string   // section title for context
	replay  []string // key sequence to replay through dispatch (nil = not executable)
}

// palette is the fuzzy-searchable command palette overlay (Ctrl+P).
// It lists every keybinding from the registry, lets the user fuzzy-filter
// by description/key/section, and on Enter replays the selected binding's
// key through the normal dispatch.
type palette struct {
	visible  bool
	input    string
	cursor   int
	items    []paletteItem
	filtered []paletteItem
}

// maxPaletteItems is the maximum number of results shown at once.
const maxPaletteItems = 16

// Open shows the palette, building the item list from the keybinding registry.
func (p *palette) Open() {
	p.visible = true
	p.input = ""
	p.cursor = 0
	p.items = buildPaletteItems()
	p.filtered = p.items
}

// Hide hides the palette.
func (p *palette) Hide() { p.visible = false }

// IsVisible reports whether the palette is shown.
func (p palette) IsVisible() bool { return p.visible }

// buildPaletteItems flattens the keybinding registry into palette entries.
func buildPaletteItems() []paletteItem {
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
	return items
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
		func(it paletteItem) string { return it.desc + " " + it.display + " " + it.section },
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

// selectedReplay returns the replay key sequence for the highlighted item, or
// nil if it isn't directly executable.
func (p palette) selectedReplay() []string {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return nil
	}
	return p.filtered[p.cursor].replay
}

// selectedDisplay returns the display string of the highlighted item.
func (p palette) selectedDisplay() string {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return ""
	}
	return p.filtered[p.cursor].display
}

// Update processes a keypress while the palette is open. It returns the
// updated palette state and an optional tea.Cmd (non-nil when replaying a
// key on Enter).
func (p palette) Update(msg tea.KeyMsg) (palette, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		p.visible = false
		return p, nil
	case "enter":
		seq := p.selectedReplay()
		p.visible = false
		if len(seq) == 0 {
			return p, nil
		}
		return p, replayKeySequence(seq)
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
	if msg.Type == tea.KeyRunes {
		p.input += msg.String()
		p.refilter()
	}
	return p, nil
}

// View renders the palette panel (without background — the caller overlays it).
func (p palette) View(width, height int) string {
	if !p.visible {
		return ""
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
		key := it.display + strings.Repeat(" ", keyW-runeLen(it.display))
		var line string
		if i == p.cursor {
			line = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBg).
				Render("❯ " + key + "  " + it.desc + "  " + it.section)
		} else {
			keyStr := lipgloss.NewStyle().Foreground(colorPrimary).Render(key)
			desc := lipgloss.NewStyle().Foreground(colorLabel).Render(it.desc)
			secStr := lipgloss.NewStyle().Foreground(colorMuted).Render(it.section)
			line = "  " + keyStr + "  " + desc + "  " + secStr
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
			Render("❯ " + content)
	}
	return "  " + content
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
	line := content + strings.Repeat(" ", gap) + tick
	if selected {
		return lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(colorBg).
			Render("❯ " + line)
	}
	return "  " + line
}
