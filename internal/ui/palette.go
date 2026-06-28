package ui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// paletteItem is a single searchable entry in the command palette.
type paletteItem struct {
	display string // key display (e.g. "X", "ctrl+r")
	desc    string // human description
	section string // section title for context
	token   string // dispatch token to replay ("" = not executable)
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
				token:   b.execToken(),
			})
		}
	}
	return items
}

// execToken returns the dispatch token to replay when this binding is selected
// in the palette, or "" if the binding cannot be directly executed. Bindings
// with multiple tokens (alternative keys for different actions) and
// double-press chords (dd, yy) are not directly executable.
func (b Binding) execToken() string {
	if len(b.Tokens) != 1 {
		return ""
	}
	t := b.Tokens[0]
	// Detect double-press chords (dd, yy): the display starts with the
	// token doubled once spaces are removed.
	compact := strings.ReplaceAll(b.Display, " ", "")
	if strings.HasPrefix(compact, t+t) {
		return ""
	}
	return t
}

// refilter rebuilds the filtered list from the current input using fuzzy
// matching over the description, key display, and section title.
func (p *palette) refilter() {
	if p.input == "" {
		p.filtered = p.items
		p.cursor = 0
		return
	}
	type scored struct {
		item  paletteItem
		score int
	}
	var results []scored
	for _, it := range p.items {
		hay := it.desc + " " + it.display + " " + it.section
		if idx, score := fuzzyMatch(p.input, hay); idx != nil {
			results = append(results, scored{it, score})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score < results[j].score
	})
	p.filtered = make([]paletteItem, len(results))
	for i, r := range results {
		p.filtered[i] = r.item
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

// selectedToken returns the replay token for the highlighted item, or "".
func (p palette) selectedToken() string {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return ""
	}
	return p.filtered[p.cursor].token
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
		token := p.selectedToken()
		p.visible = false
		if token == "" {
			return p, nil
		}
		kmsg, ok := synthesizeKeyMsg(token)
		if !ok {
			return p, nil
		}
		return p, func() tea.Msg { return kmsg }
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
		if len(it.display) > keyW {
			keyW = len(it.display)
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
		key := it.display + strings.Repeat(" ", keyW-len(it.display))
		var line string
		if i == p.cursor {
			line = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBg).
				Render("❯ " + key + "  " + it.desc + "  " + it.section)
		} else {
			keyStr := lipgloss.NewStyle().Foreground(colorLabel).Render(key)
			desc := mutedStyle.Render(it.desc)
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
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(body)

	return panel
}

// renderPalettePrompt renders the chevron-style fuzzy-search prompt used by
// all pickers: a bold "❯ " followed by the current input and a cursor bar.
func renderPalettePrompt(input string, filtering bool) string {
	cursor := lipgloss.NewStyle().Foreground(colorFg).Render("█")
	if filtering {
		cursor = lipgloss.NewStyle().Foreground(colorFg).Render("▏")
	}
	return lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("❯ ") +
		input + cursor
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
