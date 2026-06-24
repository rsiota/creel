package ui

import (
	"fmt"
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

// Toggle shows or hides the panel.
func (b *BookmarkPanel) Toggle() {
	b.visible = !b.visible
	b.cursor = 0
	b.scrollRow = 0
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
	if len(b.entries) == 0 || b.cursor < 0 || b.cursor >= len(b.entries) {
		return ""
	}
	return b.entries[b.cursor].Query
}

// CursorIndex returns the current cursor position relative to the full
// entry list, or -1 if empty.
func (b BookmarkPanel) CursorIndex() int {
	if len(b.entries) == 0 {
		return -1
	}
	return b.cursor
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
	if b.cursor < len(b.entries)-1 {
		b.cursor++
		b.adjustScroll()
	}
}

func (b *BookmarkPanel) adjustScroll() {
	maxVisible := b.height - 4
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

	title := titleStyle.Render("Bookmarks")

	maxVisible := b.height - 5
	if maxVisible < 1 {
		maxVisible = 1
	}

	end := b.scrollRow + maxVisible
	if end > len(b.entries) {
		end = len(b.entries)
	}

	var rows []string
	for i := b.scrollRow; i < end; i++ {
		e := b.entries[i]
		ts := mutedStyle.Render(bookmarks.FormatTime(e.SavedAt))
		queryStr := truncateForDisplay(e.Query, b.width-30)

		marker := " "
		lineStyle := normalStyle
		if i == b.cursor {
			marker = "→"
			lineStyle = panelSelectedStyle
		}

		line := fmt.Sprintf("%s  %s  %s", marker, ts, queryStr)
		rows = append(rows, lineStyle.Render(line))
	}

	if len(b.entries) == 0 {
		rows = append(rows, mutedStyle.Render("  No bookmarks yet."))
	}

	scrollInfo := ""
	if len(b.entries) > maxVisible {
		scrollInfo = mutedStyle.Render(fmt.Sprintf(" %d-%d of %d", b.scrollRow+1, end, len(b.entries)))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		strings.Join(rows, "\n"),
		scrollInfo,
	)

	panel := lipgloss.NewStyle().
		Width(b.width - 2).
		Height(b.height - 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Render(content)

	return panel
}
