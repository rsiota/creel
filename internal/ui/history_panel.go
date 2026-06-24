package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/history"
)

// HistoryPanel renders a scrollable list of past queries.
type HistoryPanel struct {
	entries   []history.Entry
	cursor    int
	scrollRow int
	width     int
	height    int
	visible   bool
}

// NewHistoryPanel creates a new history panel.
func NewHistoryPanel() HistoryPanel {
	return HistoryPanel{}
}

// SetEntries populates the panel with history entries (most recent first).
func (h *HistoryPanel) SetEntries(entries []history.Entry) {
	// Reverse so most recent is at top.
	h.entries = make([]history.Entry, len(entries))
	for i, e := range entries {
		h.entries[len(entries)-1-i] = e
	}
	h.cursor = 0
	h.scrollRow = 0
}

// Toggle shows or hides the panel.
func (h *HistoryPanel) Toggle() {
	h.visible = !h.visible
	h.cursor = 0
	h.scrollRow = 0
}

// IsVisible returns whether the panel is currently shown.
func (h HistoryPanel) IsVisible() bool {
	return h.visible
}

// SetSize sets the dimensions of the panel.
func (h *HistoryPanel) SetSize(width, height int) {
	h.width = width
	h.height = height
}

// SelectedQuery returns the query at the cursor, or empty string if none.
func (h HistoryPanel) SelectedQuery() string {
	if len(h.entries) == 0 || h.cursor < 0 || h.cursor >= len(h.entries) {
		return ""
	}
	return h.entries[h.cursor].Query
}

// CursorUp moves the selection up.
func (h *HistoryPanel) CursorUp() {
	if h.cursor > 0 {
		h.cursor--
		h.adjustScroll()
	}
}

// CursorDown moves the selection down.
func (h *HistoryPanel) CursorDown() {
	if h.cursor < len(h.entries)-1 {
		h.cursor++
		h.adjustScroll()
	}
}

func (h *HistoryPanel) adjustScroll() {
	maxVisible := h.height - 4
	if maxVisible < 1 {
		maxVisible = 1
	}
	if h.cursor < h.scrollRow {
		h.scrollRow = h.cursor
	}
	if h.cursor >= h.scrollRow+maxVisible {
		h.scrollRow = h.cursor - maxVisible + 1
	}
}

// View renders the history panel.
func (h HistoryPanel) View() string {
	if !h.visible {
		return ""
	}

	title := titleStyle.Render("Query History")

	maxVisible := h.height - 5
	if maxVisible < 1 {
		maxVisible = 1
	}

	end := h.scrollRow + maxVisible
	if end > len(h.entries) {
		end = len(h.entries)
	}

	var rows []string
	for i := h.scrollRow; i < end; i++ {
		e := h.entries[i]
		status := successStyle.Render("✓")
		if !e.Success {
			status = errorStyle.Render("✗")
		}

		ts := mutedStyle.Render(history.FormatTime(e.RunAt))
		queryStr := truncateForDisplay(e.Query, h.width-30)

		marker := " "
		lineStyle := normalStyle
		if i == h.cursor {
			marker = "→"
			lineStyle = panelSelectedStyle
		}

		line := fmt.Sprintf("%s %s %s  %s", marker, status, ts, queryStr)
		rows = append(rows, lineStyle.Render(line))
	}

	if len(h.entries) == 0 {
		rows = append(rows, mutedStyle.Render("  No query history yet."))
	}

	scrollInfo := ""
	if len(h.entries) > maxVisible {
		scrollInfo = mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", h.scrollRow+1, end, len(h.entries)))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		strings.Join(rows, "\n"),
		scrollInfo,
	)

	panel := lipgloss.NewStyle().
		Width(h.width - 2).
		Height(h.height - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Render(content)

	return panel
}

func truncateForDisplay(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
