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
// with g c in the workspace). Moving the cursor live-previews the theme by
// applying its palette immediately; enter persists the choice to the config,
// and esc reverts to the theme that was active when the picker opened.
//
// It mirrors FormatPicker's shape, with two additions: cursor movement calls
// applyPalette for live preview, and it remembers the open-time theme so esc
// can undo a preview even after cycling through several themes.
type ThemePicker struct {
	items     []string
	cursor    int
	scrollRow int
	visible   bool
	// appliedAtOpen is the theme name active when Show was called, so esc can
	// revert a live-previewed change. Empty means the default theme.
	appliedAtOpen string
}

// NewThemePicker returns a theme picker over the available theme names.
func NewThemePicker() ThemePicker {
	return ThemePicker{items: themeNames()}
}

// Show reveals the picker with the cursor on the active theme. An empty
// activeTheme is treated as the default theme. The value is recorded so esc
// can revert a live preview.
func (p *ThemePicker) Show(activeTheme string) {
	p.visible = true
	if activeTheme == "" {
		activeTheme = defaultThemeName
	}
	p.appliedAtOpen = activeTheme
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

// AppliedAtOpen returns the theme name that was active when the picker was
// opened, for esc-revert.
func (p ThemePicker) AppliedAtOpen() string { return p.appliedAtOpen }

// Selected returns the theme name under the cursor (falls back to the first).
func (p ThemePicker) Selected() string {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return p.items[0]
	}
	return p.items[p.cursor]
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
	if p.cursor < len(p.items)-1 {
		p.cursor++
	}
	p.adjustScroll()
	applyPalette(paletteForTheme(p.Selected()))
}

// adjustScroll keeps the cursor within the visible window. The window size is
// the panel's content height (popupDim height minus the top/bottom borders).
func (p *ThemePicker) adjustScroll() {
	_, popupH := popupDim()
	maxVisible := popupH - 2
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

// Commit hides the picker and returns the selected theme name for the caller
// to persist. The palette is already applied (preview), so no re-apply is
// needed.
func (p *ThemePicker) Commit() string {
	name := p.Selected()
	p.visible = false
	return name
}

// View renders the picker as a centered bordered overlay. It matches both the
// width and height of the column-visibility picker (opened with v in the
// results panel), derived from popupDim, so the two popups share a footprint.
// Only the visible window of themes is rendered, so the picker scrolls when
// there are more themes than fit (j/k move the cursor and the window follows);
// short lists are top-aligned and padded to the full height. Each row shows
// the theme name on the left and a row of colored swatch dots sampled from
// that theme's palette right-aligned to the panel's edge; the border uses the
// live palette, so it too re-themes as the user cycles. Keybindings are shown
// on the status bar, so the picker itself has no title or footer.
func (p ThemePicker) View() string {
	if !p.visible {
		return ""
	}

	// Match the column-visibility picker's footprint (popupDim). contentW is
	// the usable width between the border and horizontal padding (swatch dots
	// are right-aligned against it); contentH is the usable height between the
	// top/bottom borders (vertical padding is 0) and doubles as the max number
	// of visible rows.
	popupW, popupH := popupDim()
	contentW := popupW - 4 // 2 border + 2 padding
	contentH := popupH - 2 // 2 border (Padding(0,1) adds no vertical padding)

	// Render only the visible window [scrollRow, scrollRow+contentH) so the
	// picker scrolls when there are more themes than fit; pad with blank lines
	// when fewer.
	n := len(p.items)
	start := p.scrollRow
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	end := start + contentH
	if end > n {
		end = n
	}

	var rows []string
	for i := start; i < end; i++ {
		name := p.items[i]
		pal := themes[name]
		swatch := renderSwatch(pal)

		var marker, nameStyled string
		if i == p.cursor {
			marker = lipgloss.NewStyle().Foreground(colorPrimary).Render("›")
			nameStyled = lipgloss.NewStyle().Foreground(colorFg).Bold(true).Render(name)
		} else {
			marker = lipgloss.NewStyle().Foreground(colorMuted).Render(" ")
			nameStyled = lipgloss.NewStyle().Foreground(colorMuted).Render(name)
		}
		left := marker + " " + nameStyled

		gap := contentW - lipgloss.Width(left) - lipgloss.Width(swatch)
		if gap < 1 {
			gap = 1
		}
		rows = append(rows, left+strings.Repeat(" ", gap)+swatch)
	}
	for len(rows) < contentH {
		rows = append(rows, "")
	}
	content := strings.Join(rows, "\n")

	return lipgloss.NewStyle().
		Width(popupW-2).
		Height(contentH).
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
