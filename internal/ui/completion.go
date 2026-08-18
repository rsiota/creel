package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// sqlKeywords is the list of SQL keywords offered by autocompletion.
var sqlKeywords = []string{
	"SELECT", "FROM", "WHERE", "INSERT", "INTO", "VALUES", "UPDATE", "SET",
	"DELETE", "CREATE", "TABLE", "DROP", "ALTER", "ADD", "COLUMN", "INDEX",
	"JOIN", "INNER", "LEFT", "RIGHT", "OUTER", "FULL", "ON", "AS", "AND",
	"OR", "NOT", "NULL", "IS", "IN", "BETWEEN", "LIKE", "ORDER", "BY",
	"GROUP", "HAVING", "LIMIT", "OFFSET", "DISTINCT", "UNION", "ALL",
	"CASE", "WHEN", "THEN", "ELSE", "END", "COUNT", "SUM", "AVG", "MIN",
	"MAX", "PRIMARY", "KEY", "FOREIGN", "REFERENCES", "DEFAULT", "UNIQUE",
	"CONSTRAINT", "CHECK", "CASCADE", "IF", "EXISTS", "WITH", "RECURSIVE",
	"PRAGMA", "BEGIN", "COMMIT", "ROLLBACK", "TRANSACTION", "EXPLAIN",
	"INTEGER", "TEXT", "REAL", "BLOB", "VARCHAR", "DATETIME", "BOOLEAN",
}

// completionKind labels the source of a candidate.
type completionKind int

const (
	kindKeyword completionKind = iota
	kindTable
	kindColumn
)

// completionItem is a single autocomplete candidate.
type completionItem struct {
	text     string
	kind     completionKind
	table    string // owning table for columns; empty otherwise
	matchIdx []int
}

// completion holds the popup state for editor autocompletion.
type completion struct {
	visible       bool
	candidates    []completionItem
	allCandidates []completionItem
	selected      int
	partial       string
	wordStart     int
}

// minAutoTriggerChars is the minimum word length to auto-trigger the popup.
const minAutoTriggerChars = 1

// isWordChar returns true for characters allowed in SQL identifiers.
func isWordChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}

// placeOverlay overlays fg on top of bg at the given (x, y) position.
// It handles ANSI-styled strings by using grapheme-aware cutting.
func placeOverlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		targetY := y + i
		if targetY < 0 || targetY >= len(bgLines) {
			continue
		}
		bgLines[targetY] = overlayLine(bgLines[targetY], fgLine, x)
	}
	return strings.Join(bgLines, "\n")
}

// overlayLine overlays fgLine on bgLine at horizontal position x.
func overlayLine(bgLine, fgLine string, x int) string {
	bgWidth := lipgloss.Width(bgLine)
	fgWidth := lipgloss.Width(fgLine)

	if x >= bgWidth {
		return bgLine + strings.Repeat(" ", x-bgWidth) + fgLine
	}

	left := ansi.Cut(bgLine, 0, x)
	rightStart := x + fgWidth
	right := ""
	if rightStart < bgWidth {
		right = ansi.Cut(bgLine, rightStart, bgWidth)
	}
	return left + fgLine + right
}

// dimBackground strips all ANSI styling from the view and re-renders it in the
// muted colour. Used behind long-lived editing overlays to focus attention.
func dimBackground(view string) string {
	return lipgloss.NewStyle().Foreground(colorMuted).Render(ansi.Strip(view))
}

// filterCandidates returns candidates whose text fuzzy-matches partial
// (case-insensitive subsequence), sorted by match score (lower = better).
// When partial is empty, all candidates are returned sorted by kind then text.
func filterCandidates(all []completionItem, partial string) []completionItem {
	if partial == "" {
		out := make([]completionItem, len(all))
		copy(out, all)
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].kind != out[j].kind {
				return out[i].kind < out[j].kind
			}
			return out[i].text < out[j].text
		})
		return out
	}
	ranked := fuzzyRank(partial, all,
		func(it completionItem) string { return it.text },
		func(a, b fuzzyResult[completionItem]) bool { return a.Item.text < b.Item.text })
	out := make([]completionItem, len(ranked))
	for i, r := range ranked {
		out[i] = completionItem{text: r.Item.text, kind: r.Item.kind, table: r.Item.table, matchIdx: r.MatchIdx}
	}
	return out
}

// move adjusts the selection, wrapping around.
func (c *completion) move(delta int) {
	n := len(c.candidates)
	if n == 0 {
		return
	}
	c.selected = (c.selected + delta + n) % n
}

// maxCompletionItems is the maximum visible rows in the popup.
const maxCompletionItems = 8

// renderCompletion renders the popup box styled like the sidebar fuzzy picker.
func (c completion) renderCompletion() string {
	if !c.visible || len(c.candidates) == 0 {
		return ""
	}

	start := 0
	if c.selected >= maxCompletionItems {
		start = c.selected - maxCompletionItems + 1
	}
	end := start + maxCompletionItems
	if end > len(c.candidates) {
		end = len(c.candidates)
	}

	var lines []string
	for i := start; i < end; i++ {
		item := c.candidates[i]
		text := item.text
		if len(item.matchIdx) > 0 {
			text = highlightMatches(item.text, item.matchIdx)
		}
		var line string
		if i == c.selected {
			line = lipgloss.NewStyle().Bold(true).Padding(0, 1).Render(text)
		} else {
			line = normalStyle.Render(text)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	content = lipgloss.JoinVertical(lipgloss.Left, content, renderPalettePrompt(c.partial, true))

	box := lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 0).
		Render(content)

	return box
}
