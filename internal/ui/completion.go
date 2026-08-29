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
	kindSchema
	kindTable
	kindColumn
)

// completionItem is a single autocomplete candidate.
type completionItem struct {
	text     string
	kind     completionKind
	table    string // owning table for columns; may be "schema.table"
	schema   string // owning schema for tables from TablesInSchema; empty = active
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
	activeSchema  string // current connection schema for schema.table filtering
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
// overlay-dim colour — a touch softer than ERD card dim, since content behind
// popups need not stay readable. Used behind long-lived editing overlays.
func dimBackground(view string) string {
	return lipgloss.NewStyle().Foreground(colorOverlayDim).Render(ansi.Strip(view))
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
		out[i] = completionItem{
			text: r.Item.text, kind: r.Item.kind, table: r.Item.table,
			schema: r.Item.schema, matchIdx: r.MatchIdx,
		}
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

// completionKindLabel is a short muted suffix for mixed catalogs: schemas and
// tables are tagged; columns show their owning table (or "column"); keywords
// stay unlabeled (ALL CAPS already reads as SQL).
func completionKindLabel(it completionItem) string {
	switch it.kind {
	case kindSchema:
		return "schema"
	case kindTable:
		if it.schema != "" {
			return it.schema
		}
		return "table"
	case kindColumn:
		if it.table != "" {
			return it.table
		}
		return "column"
	default:
		return ""
	}
}

// renderCompletion renders the popup box. Candidates only — no echo of the
// typed prefix (the editor already shows what the user is typing). Border
// matches the muted import-path completion dropdown (colorBorder). Table and
// column rows carry a muted kind/owner label so mixed lists stay scannable.
// The selected row uses a highlight background (same cue as path completion)
// so up/down navigation is obvious — bold alone was too subtle.
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

	visible := c.candidates[start:end]
	nameWidth := 0
	labelWidth := 0
	for _, item := range visible {
		if w := lipgloss.Width(item.text); w > nameWidth {
			nameWidth = w
		}
		if w := lipgloss.Width(completionKindLabel(item)); w > labelWidth {
			labelWidth = w
		}
	}

	labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
	selectedRow := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorFg).
		Background(colorHighlight).
		Padding(0, 1)
	normalName := lipgloss.NewStyle().Foreground(colorFg)
	var lines []string
	for i, item := range visible {
		idx := start + i
		pad := nameWidth - lipgloss.Width(item.text)
		if pad < 0 {
			pad = 0
		}
		label := completionKindLabel(item)
		labelPad := labelWidth - lipgloss.Width(label)
		if labelPad < 0 {
			labelPad = 0
		}

		if idx == c.selected {
			// Plain text on a full-row highlight — nested match colours would
			// fight the background and mute the selection cue.
			row := item.text + strings.Repeat(" ", pad)
			if labelWidth > 0 {
				if label != "" {
					row += "  " + label + strings.Repeat(" ", labelPad)
				} else {
					row += "  " + strings.Repeat(" ", labelWidth)
				}
			}
			lines = append(lines, selectedRow.Render(row))
			continue
		}

		name := item.text
		if len(item.matchIdx) > 0 {
			name = highlightMatches(item.text, item.matchIdx)
		} else {
			name = normalName.Render(item.text)
		}
		row := name + strings.Repeat(" ", pad)
		if labelWidth > 0 {
			if label != "" {
				row += "  " + labelStyle.Render(label) + strings.Repeat(" ", labelPad)
			} else {
				row += "  " + strings.Repeat(" ", labelWidth)
			}
		}
		lines = append(lines, lipgloss.NewStyle().Padding(0, 1).Render(row))
	}

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(panelBorder()).
		BorderForeground(colorBorder).
		Padding(0, 0).
		Render(content)

	return box
}
