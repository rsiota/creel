package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruben/gsql/internal/bookmarks"
)

// BookmarkPanel renders a scrollable list of saved queries.
type BookmarkPanel struct {
	entries   []bookmarks.Bookmark
	cursor    int
	scrollRow int
	width     int
	height    int
	visible   bool

	filter    string
	filtering bool
}

// NewBookmarkPanel creates a new bookmark panel.
func NewBookmarkPanel() BookmarkPanel {
	return BookmarkPanel{}
}

// SetEntries populates the panel with bookmarks (most recent first).
func (b *BookmarkPanel) SetEntries(entries []bookmarks.Bookmark) {
	b.entries = make([]bookmarks.Bookmark, len(entries))
	for i, e := range entries {
		b.entries[len(entries)-1-i] = e
	}
	b.cursor = 0
	b.scrollRow = 0
}

// IsFiltering returns whether the panel is in fuzzy-filter mode.
func (b BookmarkPanel) IsFiltering() bool {
	return b.filtering
}

// StartFilter begins fuzzy-filter mode.
func (b *BookmarkPanel) StartFilter() {
	b.filtering = true
	b.filter = ""
	b.cursor = 0
	b.scrollRow = 0
}

// CancelFilter exits fuzzy-filter mode.
func (b *BookmarkPanel) CancelFilter() {
	b.filtering = false
	b.filter = ""
	b.cursor = 0
	b.scrollRow = 0
}

// Toggle shows or hides the panel.
func (b *BookmarkPanel) Toggle() {
	b.visible = !b.visible
	b.cursor = 0
	b.scrollRow = 0
	b.filtering = true
	b.filter = ""
}

// IsVisible returns whether the panel is currently shown.
func (b BookmarkPanel) IsVisible() bool {
	return b.visible
}

// SetSize sets the dimensions of the panel.
func (b *BookmarkPanel) SetSize(width, height int) {
	b.width = width
	b.height = height
}

// SelectedQuery returns the query at the cursor, or empty string if none.
func (b BookmarkPanel) SelectedQuery() string {
	entries := b.filteredEntries()
	if len(entries) == 0 || b.cursor < 0 || b.cursor >= len(entries) {
		return ""
	}
	return entries[b.cursor].entry.Query
}

// CursorIndex returns the index of the selected entry within the full
// entry list (not the filtered list), or -1 if empty.
func (b BookmarkPanel) CursorIndex() int {
	entries := b.filteredEntries()
	if len(entries) == 0 || b.cursor < 0 || b.cursor >= len(entries) {
		return -1
	}
	return entries[b.cursor].origIdx
}

// filteredBookmark holds an entry plus the fuzzy match indices for highlighting
// and its original position in the full entries slice.
type filteredBookmark struct {
	entry    bookmarks.Bookmark
	matchIdx []int
	origIdx  int
}

// filteredEntries returns entries matching the fuzzy filter, best match first.
func (b BookmarkPanel) filteredEntries() []filteredBookmark {
	if b.filter == "" {
		out := make([]filteredBookmark, len(b.entries))
		for i, e := range b.entries {
			out[i] = filteredBookmark{entry: e, origIdx: i}
		}
		return out
	}
	type scored struct {
		item  filteredBookmark
		score int
	}
	var results []scored
	for i, e := range b.entries {
		idx, score := fuzzyMatch(b.filter, e.Query)
		if idx != nil {
			results = append(results, scored{filteredBookmark{entry: e, matchIdx: idx, origIdx: i}, score})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score < results[j].score
		}
		return results[i].item.entry.SavedAt.After(results[j].item.entry.SavedAt)
	})
	out := make([]filteredBookmark, len(results))
	for i, r := range results {
		out[i] = r.item
	}
	return out
}

// CursorUp moves the selection up.
func (b *BookmarkPanel) CursorUp() {
	if b.cursor > 0 {
		b.cursor--
		b.adjustScroll()
	}
}

// CursorDown moves the selection down.
func (b *BookmarkPanel) CursorDown() {
	n := len(b.filteredEntries())
	if b.cursor < n-1 {
		b.cursor++
		b.adjustScroll()
	}
}

func (b *BookmarkPanel) adjustScroll() {
	maxVisible := b.height - 3
	if maxVisible < 1 {
		maxVisible = 1
	}
	if b.cursor < b.scrollRow {
		b.scrollRow = b.cursor
	}
	if b.cursor >= b.scrollRow+maxVisible {
		b.scrollRow = b.cursor - maxVisible + 1
	}
}

// View renders the bookmark panel.
func (b BookmarkPanel) View() string {
	if !b.visible {
		return ""
	}

	entries := b.filteredEntries()

	// Reserve one row for the filter prompt (always visible).
	avail := b.height - 3
	if avail < 1 {
		avail = 1
	}

	maxVisible := avail
	end := b.scrollRow + maxVisible
	if end > len(entries) {
		end = len(entries)
	}

	var rows []string
	for i := b.scrollRow; i < end; i++ {
		e := entries[i].entry
		ts := bookmarks.FormatTime(e.SavedAt)
		queryStr := truncateForDisplay(e.Query, b.width-26)
		matched := highlightMatches(queryStr, entries[i].matchIdx)

		isSelected := i == b.cursor
		if isSelected {
			line := fmt.Sprintf("❯ %s  %s", ts, queryStr)
			rows = append(rows, selectedStyle.Render(line))
		} else {
			styledTs := mutedStyle.Render(ts)
			line := fmt.Sprintf("  %s  %s", styledTs, matched)
			rows = append(rows, normalStyle.Render(line))
		}
	}

	if len(entries) == 0 {
		rows = append(rows, mutedStyle.Render("  (no matches)"))
	}

	content := strings.Join(rows, "\n")
	prompt := " " + renderPalettePrompt(b.filter, true)
	content = lipgloss.JoinVertical(lipgloss.Left, prompt, content)

	panel := lipgloss.NewStyle().
		Width(b.width - 2).
		Height(b.height - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Render(content)

	return panel
}
