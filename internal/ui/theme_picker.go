package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// swatchColors are the palette slots shown as colored dots beside each theme
// name, giving a static at-a-glance preview of a theme's identity alongside
// the live full-UI preview that moving the cursor triggers.
var swatchColors = []func(p colorPalette) lipgloss.Color{
	func(p colorPalette) lipgloss.Color { return p.primary },
	func(p colorPalette) lipgloss.Color { return p.accent },
	func(p colorPalette) lipgloss.Color { return p.success },
	func(p colorPalette) lipgloss.Color { return p.mark },
	func(p colorPalette) lipgloss.Color { return p.fg },
}

// ThemePicker is a single-select overlay for choosing the color theme (opened
// with g c in the workspace). It supports type-to-filter (fuzzy match against
// each theme's display name) and scrolls when the catalog exceeds the panel.
// Moving the cursor (or changing the filter) live-previews the theme by
// applying its palette immediately; enter persists the choice to the config,
// and esc reverts to the theme that was active when the picker opened.
//
// It mirrors the column-visibility picker's shape (filter prompt + scrollable
// list, arrow-key navigation so every letter can filter), with two additions:
// cursor/filter changes call applyPalette for live preview, and it remembers
// the open-time theme so esc can undo a preview even after cycling.
type ThemePicker struct {
	items     []string // theme keys (curated first, then generated, sorted)
	cursor    int
	scrollRow int
	filter    string
	visible   bool
	// appliedAtOpen is the theme key active when Show was called, so esc can
	// revert a live-previewed change. Empty means the default theme.
	appliedAtOpen string
}

// NewThemePicker returns a theme picker over the available theme keys.
func NewThemePicker() ThemePicker {
	return ThemePicker{items: themeNames()}
}

// Show reveals the picker with the cursor on the active theme, filter cleared.
// An empty activeTheme is treated as the default theme. The value is recorded
// so esc can revert a live preview.
func (p *ThemePicker) Show(activeTheme string) {
	p.visible = true
	if activeTheme == "" {
		activeTheme = defaultThemeName
	}
	p.appliedAtOpen = activeTheme
	p.filter = ""
	p.cursor = 0
	p.scrollRow = 0
	for i, name := range p.items {
		if name == activeTheme {
			p.cursor = i
			break
		}
	}
	p.adjustScroll()
}

// Hide hides the picker without changing the active palette.
func (p *ThemePicker) Hide() { p.visible = false }

// IsVisible reports whether the picker is shown.
func (p ThemePicker) IsVisible() bool { return p.visible }

// AppliedAtOpen returns the theme key that was active when the picker was
// opened, for esc-revert.
func (p ThemePicker) AppliedAtOpen() string { return p.appliedAtOpen }

// filteredItems returns the theme keys whose display name fuzzy-matches the
// filter (ranked), or all keys when the filter is empty.
func (p ThemePicker) filteredItems() []string {
	if p.filter == "" {
		return p.items
	}
	ranked := fuzzyRank(p.filter, p.items,
		func(s string) string { return themeDisplay(s) },
		func(a, b fuzzyResult[string]) bool { return themeDisplay(a.Item) < themeDisplay(b.Item) })
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.Item
	}
	return out
}

// Selected returns the theme key under the cursor within the filtered list
// (falls back to the first match, or "" if the filter has no matches).
func (p ThemePicker) Selected() string {
	items := p.filteredItems()
	if len(items) == 0 {
		return ""
	}
	if p.cursor < 0 || p.cursor >= len(items) {
		return items[0]
	}
	return items[p.cursor]
}

// Up moves the cursor up and live-applies the newly selected palette so the
// UI behind the picker updates immediately.
func (p *ThemePicker) Up() {
	if p.cursor > 0 {
		p.cursor--
	}
	p.adjustScroll()
	applyPalette(paletteForTheme(p.Selected()))
}

// Down moves the cursor down and live-applies the newly selected palette.
func (p *ThemePicker) Down() {
	if p.cursor < len(p.filteredItems())-1 {
		p.cursor++
	}
	p.adjustScroll()
	applyPalette(paletteForTheme(p.Selected()))
}

// FilterAddChar appends ch to the filter, resets the cursor to the top match,
// and live-previews it.
func (p *ThemePicker) FilterAddChar(ch string) {
	p.filter += ch
	p.cursor = 0
	p.scrollRow = 0
	if items := p.filteredItems(); len(items) > 0 {
		applyPalette(paletteForTheme(items[0]))
	}
}

// FilterBackspace removes the last filter character and re-previews the top
// match.
func (p *ThemePicker) FilterBackspace() {
	if len(p.filter) == 0 {
		return
	}
	p.filter = p.filter[:len(p.filter)-1]
	p.cursor = 0
	p.scrollRow = 0
	if items := p.filteredItems(); len(items) > 0 {
		applyPalette(paletteForTheme(items[0]))
	}
}

// adjustScroll keeps the cursor within the visible window. The window size is
// the panel content height minus one row for the filter prompt.
func (p *ThemePicker) adjustScroll() {
	_, popupH := popupDim()
	maxVisible := popupH - 3 // 2 border + 1 prompt row
	if maxVisible < 1 {
		maxVisible = 1
	}
	if p.cursor < p.scrollRow {
		p.scrollRow = p.cursor
	}
	if p.cursor >= p.scrollRow+maxVisible {
		p.scrollRow = p.cursor - maxVisible + 1
	}
}

// Commit hides the picker and returns the selected theme key for the caller to
// persist. Returns "" (without hiding) if the filter has no matches, so enter
// is a no-op in that case. The palette is already applied (preview), so no
// re-apply is needed.
func (p *ThemePicker) Commit() string {
	name := p.Selected()
	if name != "" {
		p.visible = false
	}
	return name
}

// View renders the picker as a centered bordered overlay matching the
// column-visibility picker's footprint (popupDim). A filter prompt leads,
// followed by a scrolling window of theme rows; each row shows the theme's
// display name on the left and a row of colored swatch dots right-aligned to
// the panel's edge. The border and prompt use the live palette, so they too
// re-theme as the user cycles. Keybindings are shown on the status bar.
func (p ThemePicker) View() string {
	if !p.visible {
		return ""
	}

	popupW, popupH := popupDim()
	contentW := popupW - 4 // 2 border + 2 padding
	listH := popupH - 3    // 2 border + 1 prompt row

	items := p.filteredItems()
	n := len(items)
	start := p.scrollRow
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	end := start + listH
	if end > n {
		end = n
	}

	var rows []string
	for i := start; i < end; i++ {
		key := items[i]
		disp := themeDisplay(key)
		pal := themes[key]
		swatch := renderSwatch(pal)

		var marker, nameStyled string
		if i == p.cursor {
			marker = lipgloss.NewStyle().Foreground(colorPrimary).Render("›")
			nameStyled = lipgloss.NewStyle().Foreground(colorFg).Bold(true).Render(disp)
		} else {
			marker = lipgloss.NewStyle().Foreground(colorMuted).Render(" ")
			nameStyled = lipgloss.NewStyle().Foreground(colorMuted).Render(disp)
		}
		left := marker + " " + nameStyled

		gap := contentW - lipgloss.Width(left) - lipgloss.Width(swatch)
		if gap < 1 {
			gap = 1
		}
		rows = append(rows, left+strings.Repeat(" ", gap)+swatch)
	}
	if len(rows) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Render("  no matches"))
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}

	prompt := renderPalettePrompt(p.filter, true)
	content := lipgloss.JoinVertical(lipgloss.Left, prompt, strings.Join(rows, "\n"))

	return lipgloss.NewStyle().
		Width(popupW-2).
		Height(popupH-2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(content)
}

// renderSwatch builds a row of colored dots (one per swatchColors slot) for the
// given palette, used as a static at-a-glance preview beside each theme name.
func renderSwatch(pal colorPalette) string {
	var sw strings.Builder
	for j, pick := range swatchColors {
		if j > 0 {
			sw.WriteByte(' ')
		}
		sw.WriteString(lipgloss.NewStyle().Foreground(pick(pal)).Render("●"))
	}
	return sw.String()
}
