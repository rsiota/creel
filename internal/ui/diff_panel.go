package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// DiffPanel shows a scrollable result-set diff between two tabs.
type DiffPanel struct {
	visible       bool
	diff          resultDiff
	changesOnly   bool
	scroll        int
	cursor        int
	width         int
	height        int
	visibleIdx    []int // indexes into diff.Entries respecting changesOnly
}

// IsVisible reports whether the panel is shown.
func (p DiffPanel) IsVisible() bool {
	return p.visible
}

// Show populates the panel with a computed diff.
func (p *DiffPanel) Show(d resultDiff) {
	p.visible = true
	p.diff = d
	p.changesOnly = true
	p.scroll = 0
	p.cursor = 0
	p.rebuildVisible()
}

// Hide hides the panel.
func (p *DiffPanel) Hide() {
	p.visible = false
}

// SetSize sets the panel dimensions including border.
func (p *DiffPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *DiffPanel) contentHeight() int {
	// header (2) + separator + rows; reserve 3 lines for chrome inside border
	h := p.height - borderOverhead - 3
	if h < 1 {
		h = 1
	}
	return h
}

func (p *DiffPanel) rebuildVisible() {
	p.visibleIdx = p.visibleIdx[:0]
	for i, e := range p.diff.Entries {
		if p.changesOnly && e.Kind == diffSame {
			continue
		}
		p.visibleIdx = append(p.visibleIdx, i)
	}
	if p.cursor >= len(p.visibleIdx) {
		p.cursor = len(p.visibleIdx) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.adjustScroll(p.contentHeight())
}

// Update handles keyboard input inside the diff overlay.
func (p DiffPanel) Update(msg tea.KeyMsg) DiffPanel {
	n := len(p.visibleIdx)
	vh := p.contentHeight()
	switch msg.String() {
	case "j", "down":
		if p.cursor < n-1 {
			p.cursor++
			p.adjustScroll(vh)
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
			p.adjustScroll(vh)
		}
	case "g":
		p.cursor = 0
		p.scroll = 0
	case "G":
		p.cursor = n - 1
		p.adjustScroll(vh)
	case "ctrl+d":
		p.cursor += vh / 2
		if p.cursor >= n {
			p.cursor = n - 1
		}
		p.adjustScroll(vh)
	case "ctrl+u":
		p.cursor -= vh / 2
		if p.cursor < 0 {
			p.cursor = 0
		}
		p.adjustScroll(vh)
	case "a":
		p.changesOnly = !p.changesOnly
		p.rebuildVisible()
	}
	return p
}

func (p *DiffPanel) adjustScroll(vh int) {
	if p.cursor < p.scroll {
		p.scroll = p.cursor
	}
	if p.cursor >= p.scroll+vh {
		p.scroll = p.cursor - vh + 1
	}
	if p.scroll < 0 {
		p.scroll = 0
	}
}

// View renders the diff overlay.
func (p DiffPanel) View() string {
	contentW := p.width - borderOverhead - 2
	if contentW < 1 {
		contentW = 1
	}
	vh := p.contentHeight()

	titleStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	header := titleStyle.Render(fmt.Sprintf("diff  %s  →  %s", p.diff.LeftTitle, p.diff.RightTitle))
	sub := muted.Render(p.diff.summary())
	if p.changesOnly {
		sub += muted.Render("  · changes only (a = all)")
	} else {
		sub += muted.Render("  · all rows (a = changes)")
	}

	lines := p.bodyLines(contentW)
	var visible []string
	end := p.scroll + vh
	if end > len(lines) {
		end = len(lines)
	}
	for i := p.scroll; i < end; i++ {
		line := lines[i]
		if i == p.cursor {
			line = lipgloss.NewStyle().Background(colorCursorRow).Render(padDiffLine(line, contentW))
		}
		visible = append(visible, line)
	}
	for len(visible) < vh {
		visible = append(visible, "")
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		truncateDiffLine(header, contentW),
		truncateDiffLine(sub, contentW),
		muted.Render(strings.Repeat("─", contentW)),
		lipgloss.JoinVertical(lipgloss.Left, visible...),
	)

	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		Render(body)
}

func (p DiffPanel) bodyLines(contentW int) []string {
	if len(p.visibleIdx) == 0 {
		msg := "(no differences)"
		if !p.changesOnly {
			msg = "(both tabs empty or identical)"
		}
		return []string{lipgloss.NewStyle().Foreground(colorMuted).Render(msg)}
	}

	colW := p.colWidths(contentW)
	addStyle := lipgloss.NewStyle().Foreground(colorSuccess)
	delStyle := lipgloss.NewStyle().Foreground(colorError)
	chgStyle := lipgloss.NewStyle().Foreground(colorAccent)
	sameStyle := lipgloss.NewStyle().Foreground(colorMuted)
	markChg := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	var lines []string
	for _, ei := range p.visibleIdx {
		e := p.diff.Entries[ei]
		var mark string
		var sty lipgloss.Style
		switch e.Kind {
		case diffAdded:
			mark, sty = "+", addStyle
		case diffRemoved:
			mark, sty = "-", delStyle
		case diffChanged:
			mark, sty = "~", chgStyle
		default:
			mark, sty = " ", sameStyle
		}

		cells := e.RightCells
		if e.Kind == diffRemoved {
			cells = e.LeftCells
		}
		parts := make([]string, len(p.diff.Cols))
		for i, col := range p.diff.Cols {
			val := ""
			if i < len(cells) {
				val = cells[i]
			}
			if e.Kind == diffChanged && i < len(e.ChangedCols) && e.ChangedCols[i] {
				left := ""
				if i < len(e.LeftCells) {
					left = e.LeftCells[i]
				}
				right := ""
				if i < len(e.RightCells) {
					right = e.RightCells[i]
				}
				parts[i] = markChg.Render(truncateCell(left+"→"+right, colW[i]))
			} else {
				parts[i] = sty.Render(truncateCell(val, colW[i]))
			}
			_ = col
		}
		sep := lipgloss.NewStyle().Foreground(colorBorder).Render("│")
		line := sty.Render(mark) + " " + strings.Join(parts, " "+sep+" ")
		lines = append(lines, truncateDiffLine(line, contentW))
	}
	return lines
}

func (p DiffPanel) colWidths(contentW int) []int {
	n := len(p.diff.Cols)
	if n == 0 {
		return nil
	}
	// mark + spaces + separators between cols
	overhead := 2 + 3*(n-1)
	avail := contentW - overhead
	if avail < n {
		avail = n
	}
	base := avail / n
	widths := make([]int, n)
	for i := range widths {
		widths[i] = base
	}
	// Cap very wide columns using content, then redistribute leftover later if needed.
	for i, col := range p.diff.Cols {
		w := runeLen(col)
		for _, ei := range p.visibleIdx {
			e := p.diff.Entries[ei]
			for _, cells := range [][]string{e.LeftCells, e.RightCells} {
				if i < len(cells) && runeLen(cells[i]) > w {
					w = runeLen(cells[i])
				}
			}
			if e.Kind == diffChanged && i < len(e.ChangedCols) && e.ChangedCols[i] {
				left, right := "", ""
				if i < len(e.LeftCells) {
					left = e.LeftCells[i]
				}
				if i < len(e.RightCells) {
					right = e.RightCells[i]
				}
				if cw := runeLen(left + "→" + right); cw > w {
					w = cw
				}
			}
		}
		if w < 4 {
			w = 4
		}
		if w < widths[i] {
			widths[i] = w
		}
	}
	return widths
}

func padDiffLine(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}

func truncateDiffLine(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}
