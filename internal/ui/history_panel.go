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

	filter    string
	filtering bool
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

// IsFiltering returns whether the panel is in fuzzy-filter mode.
func (h HistoryPanel) IsFiltering() bool {
	return h.filtering
}

// StartFilter begins fuzzy-filter mode.
func (h *HistoryPanel) StartFilter() {
	h.filtering = true
	h.filter = ""
	h.cursor = 0
	h.scrollRow = 0
}

// CancelFilter exits fuzzy-filter mode.
func (h *HistoryPanel) CancelFilter() {
	h.filtering = false
	h.filter = ""
	h.cursor = 0
	h.scrollRow = 0
}

// Toggle shows or hides the panel.
func (h *HistoryPanel) Toggle() {
	h.visible = !h.visible
	h.cursor = 0
	h.scrollRow = 0
	h.filtering = true
	h.filter = ""
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
	entries := h.filteredEntries()
	if len(entries) == 0 || h.cursor < 0 || h.cursor >= len(entries) {
		return ""
	}
	return entries[h.cursor].entry.Query
}

// filteredEntry holds an entry plus the fuzzy match indices for highlighting.
type filteredEntry struct {
	entry    history.Entry
	matchIdx []int
	origIdx  int // permanent position in h.entries (most-recent-first); +1 is the :rerun rank
}

// filteredEntries returns entries matching the fuzzy filter, best match first.
func (h HistoryPanel) filteredEntries() []filteredEntry {
	if h.filter == "" {
		out := make([]filteredEntry, len(h.entries))
		for i, e := range h.entries {
			out[i] = filteredEntry{entry: e, origIdx: i}
		}
		return out
	}
	ranked := fuzzyRank(h.filter, h.entries,
		func(e history.Entry) string { return e.Query },
		func(a, b fuzzyResult[history.Entry]) bool { return a.Item.RunAt.After(b.Item.RunAt) })
	out := make([]filteredEntry, len(ranked))
	for i, r := range ranked {
		// r.Index is the entry's original position in h.entries; keep it so
		// the displayed number (origIdx+1) stays stable under filtering.
		out[i] = filteredEntry{entry: r.Item, matchIdx: r.MatchIdx, origIdx: r.Index}
	}
	return out
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
	n := len(h.filteredEntries())
	if h.cursor < n-1 {
		h.cursor++
		h.adjustScroll()
	}
}

func (h *HistoryPanel) adjustScroll() {
	maxVisible := h.height - 3
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

	entries := h.filteredEntries()

	// Reserve one row for the filter prompt (always visible).
	avail := h.height - 3
	if avail < 1 {
		avail = 1
	}

	maxVisible := avail
	end := h.scrollRow + maxVisible
	if end > len(entries) {
		end = len(entries)
	}

	// numW is the column width for the 1-based :rerun rank each row shows.
	numW := len(fmt.Sprintf("%d", len(entries)))
	if numW < 1 {
		numW = 1
	}

	var rows []string
	for i := h.scrollRow; i < end; i++ {
		e := entries[i].entry
		ts := history.FormatTime(e.RunAt)
		queryStr := truncateForDisplay(e.Query, h.width-26-numW-2)
		matched := highlightMatches(queryStr, entries[i].matchIdx)
		numStr := fmt.Sprintf("%*d", numW, entries[i].origIdx+1)

		isSelected := i == h.cursor
		if isSelected {
			line := fmt.Sprintf("❯ %s  %s  %s", numStr, ts, queryStr)
			rows = append(rows, selectedStyle.Render(line))
		} else {
			styledNum := mutedStyle.Render(numStr)
			styledTs := mutedStyle.Render(ts)
			line := fmt.Sprintf("  %s  %s  %s", styledNum, styledTs, matched)
			rows = append(rows, normalStyle.Render(line))
		}
	}

	if len(entries) == 0 {
		rows = append(rows, mutedStyle.Render("  (no matches)"))
	}

	content := strings.Join(rows, "\n")
	prompt := " " + renderPalettePrompt(h.filter, true)
	content = lipgloss.JoinVertical(lipgloss.Left, prompt, content)

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
