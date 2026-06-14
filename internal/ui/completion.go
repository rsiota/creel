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
	text string
	kind completionKind
}

// completion holds the popup state for editor autocompletion.
type completion struct {
	visible        bool
	candidates     []completionItem
	allCandidates  []completionItem
	selected       int
	partial        string
	wordStart      int
}

// minAutoTriggerChars is the minimum word length to auto-trigger the popup.
const minAutoTriggerChars = 2

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
// (case-insensitive), sorted alphabetically with keywords last.
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
	lower := strings.ToLower(partial)
	var filtered []completionItem
	for _, item := range all {
		if strings.HasPrefix(strings.ToLower(item.text), lower) {
			filtered = append(filtered, item)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].kind != filtered[j].kind {
			return filtered[i].kind < filtered[j].kind
		}
		return filtered[i].text < filtered[j].text
	})
	return filtered
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

// renderCompletion renders the popup box.
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
		var line string
		if i == c.selected {
			line = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBg).
				Bold(true).
				Render("→ " + item.text)
		} else {
			style := lipgloss.NewStyle()
			switch item.kind {
			case kindTable:
				style = style.Foreground(colorFg)
			case kindColumn:
				style = style.Foreground(colorSuccess)
			default:
				style = style.Foreground(colorPrimary)
			}
			line = "  " + style.Render(item.text)
		}
		lines = append(lines, line)
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 0).
		Render(strings.Join(lines, "\n"))

	return box
}
