package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

type sqlTokenKind int

const (
	tokenPlain sqlTokenKind = iota
	tokenKeyword
	tokenString
	tokenNumber
	tokenComment
	tokenOperator
)

type sqlToken struct {
	text string
	kind sqlTokenKind
}

var (
	sqlKeywordStyle  lipgloss.Style
	sqlStringStyle   lipgloss.Style
	sqlNumberStyle   lipgloss.Style
	sqlCommentStyle  lipgloss.Style
	sqlOperatorStyle lipgloss.Style
	sqlPlainStyle    lipgloss.Style

	sqlKeywordSet = buildSQLKeywordSet()
)

// rebuildSQLHighlightStyles re-creates the SQL highlight styles from the
// active palette. Called by applyPalette so a theme switch recolours
// highlighted SQL on the next render.
func rebuildSQLHighlightStyles() {
	sqlKeywordStyle = lipgloss.NewStyle().Foreground(colorPrimary)
	sqlStringStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	sqlNumberStyle = lipgloss.NewStyle().Foreground(colorEdit)
	sqlCommentStyle = lipgloss.NewStyle().Foreground(colorMuted)
	sqlOperatorStyle = lipgloss.NewStyle().Foreground(colorAccent)
	sqlPlainStyle = lipgloss.NewStyle().Foreground(colorFg)
}

func buildSQLKeywordSet() map[string]bool {
	set := make(map[string]bool, len(sqlKeywords))
	for _, kw := range sqlKeywords {
		set[strings.ToUpper(kw)] = true
	}
	return set
}

func tokenizeSQL(s string) []sqlToken {
	if s == "" {
		return nil
	}

	runes := []rune(s)
	var tokens []sqlToken
	for i := 0; i < len(runes); {
		switch {
		case unicode.IsSpace(runes[i]):
			j := i + 1
			for j < len(runes) && unicode.IsSpace(runes[j]) {
				j++
			}
			tokens = append(tokens, sqlToken{text: string(runes[i:j]), kind: tokenPlain})
			i = j

		case i+1 < len(runes) && runes[i] == '-' && runes[i+1] == '-':
			j := i + 2
			for j < len(runes) && runes[j] != '\n' {
				j++
			}
			tokens = append(tokens, sqlToken{text: string(runes[i:j]), kind: tokenComment})
			i = j

		case i+1 < len(runes) && runes[i] == '/' && runes[i+1] == '*':
			j := i + 2
			for j+1 < len(runes) && !(runes[j] == '*' && runes[j+1] == '/') {
				j++
			}
			if j+1 < len(runes) {
				j += 2
			}
			tokens = append(tokens, sqlToken{text: string(runes[i:j]), kind: tokenComment})
			i = j

		case runes[i] == '\'' || runes[i] == '"':
			quote := runes[i]
			j := i + 1
			for j < len(runes) {
				if runes[j] == quote {
					if j+1 < len(runes) && runes[j+1] == quote {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			tokens = append(tokens, sqlToken{text: string(runes[i:j]), kind: tokenString})
			i = j

		case runes[i] == '`':
			j := i + 1
			for j < len(runes) && runes[j] != '`' {
				j++
			}
			if j < len(runes) {
				j++
			}
			tokens = append(tokens, sqlToken{text: string(runes[i:j]), kind: tokenString})
			i = j

		case unicode.IsDigit(runes[i]):
			j := i + 1
			for j < len(runes) && (unicode.IsDigit(runes[j]) || runes[j] == '.') {
				j++
			}
			tokens = append(tokens, sqlToken{text: string(runes[i:j]), kind: tokenNumber})
			i = j

		case strings.ContainsRune("=<>!+-*/%,;().", runes[i]):
			tokens = append(tokens, sqlToken{text: string(runes[i]), kind: tokenOperator})
			i++

		case unicode.IsLetter(runes[i]) || runes[i] == '_':
			j := i + 1
			for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_' || runes[j] == '$') {
				j++
			}
			word := string(runes[i:j])
			kind := tokenPlain
			if sqlKeywordSet[strings.ToUpper(word)] {
				kind = tokenKeyword
			}
			tokens = append(tokens, sqlToken{text: word, kind: kind})
			i = j

		default:
			tokens = append(tokens, sqlToken{text: string(runes[i]), kind: tokenPlain})
			i++
		}
	}
	return tokens
}

func styleForToken(kind sqlTokenKind) lipgloss.Style {
	switch kind {
	case tokenKeyword:
		return sqlKeywordStyle
	case tokenString:
		return sqlStringStyle
	case tokenNumber:
		return sqlNumberStyle
	case tokenComment:
		return sqlCommentStyle
	case tokenOperator:
		return sqlOperatorStyle
	default:
		return sqlPlainStyle
	}
}

// highlightRange renders syntax-colored text for runes [start, end) in line.
func highlightRange(line string, start, end int) string {
	if start >= end {
		return ""
	}

	tokens := tokenizeSQL(line)
	var b strings.Builder
	pos := 0
	for _, tok := range tokens {
		tokRunes := []rune(tok.text)
		tokStart := pos
		tokEnd := pos + len(tokRunes)
		pos = tokEnd

		clipStart := max(start, tokStart)
		clipEnd := min(end, tokEnd)
		if clipStart >= clipEnd {
			continue
		}

		sub := string(tokRunes[clipStart-tokStart : clipEnd-tokStart])
		b.WriteString(styleForToken(tok.kind).Render(sub))
	}
	return b.String()
}

// highlightSegment renders a wrapped editor segment with syntax colors.
func highlightSegment(s string) string {
	return highlightRange(s, 0, len([]rune(s)))
}

// highlightSubstring renders syntax colors for runes [start, end) in s.
func highlightSubstring(s string, start, end int) string {
	runes := []rune(s)
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start >= end {
		return ""
	}
	return highlightRange(s, start, end)
}
