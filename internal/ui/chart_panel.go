package ui

import (
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// chartBar is one horizontal bar: a label axis value and a numeric magnitude.
type chartBar struct {
	label string
	value float64
}

// ChartPanel renders a simple chart (currently horizontal bars) in the
// results-panel slot. Esc/q closes it and restores the grid.
type ChartPanel struct {
	visible bool
	title   string // e.g. "bar · users × amount"
	bars    []chartBar
	skipped int // rows skipped (NULL / non-numeric values)
	scroll  int
	width   int
	height  int
}

// NewChartPanel returns a hidden chart panel.
func NewChartPanel() ChartPanel { return ChartPanel{} }

// IsVisible reports whether the chart is shown in place of the results grid.
func (c ChartPanel) IsVisible() bool { return c.visible }

// ShowBar populates a horizontal bar chart and makes the panel visible.
func (c *ChartPanel) ShowBar(title string, bars []chartBar, skipped int) {
	c.visible = true
	c.title = title
	c.bars = bars
	c.skipped = skipped
	c.scroll = 0
}

// Hide closes the chart panel.
func (c *ChartPanel) Hide() { c.visible = false }

// SetSize sets the exterior panel dimensions (including border), matching the
// results slot so the chart drops in without shifting the workspace.
func (c *ChartPanel) SetSize(w, h int) { c.width = w; c.height = h }

func (c ChartPanel) contentHeight() int {
	h := c.height - borderOverhead
	if h < 1 {
		h = 1
	}
	return h
}

func (c ChartPanel) contentWidth() int {
	w := c.width - borderOverhead
	if w < 10 {
		w = 10
	}
	return w
}

// Update handles scrolling within the chart.
func (c ChartPanel) Update(msg tea.KeyMsg) ChartPanel {
	n := len(c.bars)
	vh := c.viewport()
	switch msg.String() {
	case "j", "down":
		if c.scroll+vh < n {
			c.scroll++
		}
	case "k", "up":
		if c.scroll > 0 {
			c.scroll--
		}
	case "g":
		c.scroll = 0
	case "G":
		c.scroll = n - vh
		if c.scroll < 0 {
			c.scroll = 0
		}
	case "ctrl+d":
		c.scroll += vh / 2
		if max := n - vh; max < 0 {
			c.scroll = 0
		} else if c.scroll > max {
			c.scroll = max
		}
	case "ctrl+u":
		c.scroll -= vh / 2
		if c.scroll < 0 {
			c.scroll = 0
		}
	}
	return c
}

// viewport is how many bar rows fit under the title/footer.
func (c ChartPanel) viewport() int {
	// title + optional skipped footer + blank separator ≈ 3 reserved lines
	vh := c.contentHeight() - 3
	if vh < 1 {
		vh = 1
	}
	return vh
}

// View renders the bordered chart panel.
func (c ChartPanel) View() string {
	inner := c.contentWidth()
	lines := c.bodyLines(inner)
	for len(lines) < c.contentHeight() {
		lines = append(lines, "")
	}
	if len(lines) > c.contentHeight() {
		lines = lines[:c.contentHeight()]
	}
	return lipgloss.NewStyle().
		Width(inner).
		Height(c.contentHeight()).
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Render(strings.Join(lines, "\n"))
}

func (c ChartPanel) bodyLines(inner int) []string {
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	titleStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	barStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	labelStyle := lipgloss.NewStyle().Foreground(colorFg)
	valStyle := lipgloss.NewStyle().Foreground(colorLabel)

	var out []string
	out = append(out, titleStyle.Render(truncateCell(c.title, inner)))

	if len(c.bars) == 0 {
		out = append(out, muted.Render(truncateCell("no numeric values to chart", inner)))
		return out
	}

	maxVal := 0.0
	for _, b := range c.bars {
		if b.value > maxVal {
			maxVal = b.value
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	labelW := 0
	for _, b := range c.bars {
		if w := lipgloss.Width(b.label); w > labelW {
			labelW = w
		}
	}
	if labelW > inner/3 {
		labelW = inner / 3
	}
	if labelW < 4 {
		labelW = 4
	}

	// " label │████…  value"
	valSample := formatChartValue(maxVal)
	valW := lipgloss.Width(valSample)
	if valW < 4 {
		valW = 4
	}
	// spaces: 1 before label area ends + " │" + 1 after bar + value
	barW := inner - labelW - valW - 4
	if barW < 4 {
		barW = 4
	}

	vh := c.viewport()
	start := c.scroll
	end := start + vh
	if end > len(c.bars) {
		end = len(c.bars)
	}
	for _, b := range c.bars[start:end] {
		filled := 0
		if maxVal > 0 {
			filled = int(math.Round(float64(barW) * b.value / maxVal))
		}
		if filled < 0 {
			filled = 0
		}
		if filled > barW {
			filled = barW
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)
		label := truncateCell(b.label, labelW)
		line := labelStyle.Render(label) +
			muted.Render(" │") +
			barStyle.Render(bar) +
			" " +
			valStyle.Render(padLeft(formatChartValue(b.value), valW))
		out = append(out, line)
	}

	if c.skipped > 0 {
		out = append(out, muted.Render(truncateCell(
			fmt.Sprintf("skipped %d non-numeric/NULL row%s", c.skipped, pluralIf(c.skipped != 1, "s")),
			inner)))
	}
	return out
}

func formatChartValue(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e12 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.4g", v)
}

func padLeft(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}
