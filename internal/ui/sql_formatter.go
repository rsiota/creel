package ui

import (
	"strings"
)

// formatKeywordSet extends sqlKeywordSet with keywords needed for formatting.
var formatKeywordSet map[string]bool

func init() {
	formatKeywordSet = make(map[string]bool, len(sqlKeywordSet)+8)
	for k, v := range sqlKeywordSet {
		formatKeywordSet[k] = v
	}
	for _, k := range []string{"ASC", "DESC", "TRUNCATE", "RETURNING", "REPLACE"} {
		formatKeywordSet[k] = true
	}
}

// majorClauseKeywords start a new line at base indentation.
var majorClauseKeywords = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true,
	"GROUP": true, "ORDER": true, "HAVING": true,
	"LIMIT": true, "OFFSET": true,
	"UNION": true, "INTERSECT": true, "EXCEPT": true,
	"VALUES": true, "SET": true,
	"RETURNING": true,
}

// indentKeywords start a new line with one level of indentation.
var indentKeywords = map[string]bool{
	"ON": true, "USING": true,
	"AND": true, "OR": true,
	"WHEN": true, "ELSE": true,
}

// joinStartKeywords trigger a newline at base indent for the join modifier
// (LEFT, RIGHT, INNER, FULL, CROSS). JOIN itself follows without a newline.
var joinStartKeywords = map[string]bool{
	"LEFT": true, "RIGHT": true, "INNER": true,
	"FULL": true, "CROSS": true,
}

// joinModifierKeywords may appear between a join start and JOIN.
var joinModifierKeywords = map[string]bool{
	"LEFT": true, "RIGHT": true, "INNER": true,
	"FULL": true, "CROSS": true, "OUTER": true,
}

// funcLikeKeywords are keywords followed by ( with no space.
var funcLikeKeywords = map[string]bool{
	"COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true,
	"COALESCE": true, "NULLIF": true, "CAST": true,
	"LENGTH": true, "LOWER": true, "UPPER": true, "TRIM": true,
	"SUBSTR": true, "SUBSTRING": true, "REPLACE": true,
	"ABS": true, "ROUND": true,
}

type fmtToken struct {
	text     string
	isString bool
}

// tokenizeForFormat splits a SQL string into tokens for the formatter.
// Unlike the highlighter's tokenizer, this skips whitespace and handles
// multi-character operators (<=, >=, !=, <>, ||).
func tokenizeForFormat(sql string) []fmtToken {
	var tokens []fmtToken
	runes := []rune(sql)
	i := 0
	n := len(runes)

	for i < n {
		ch := runes[i]

		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}

		if ch == '\'' {
			j := i + 1
			for j < n {
				if runes[j] == '\'' {
					if j+1 < n && runes[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			tokens = append(tokens, fmtToken{text: string(runes[i:j]), isString: true})
			i = j
			continue
		}

		if ch == '"' {
			j := i + 1
			for j < n && runes[j] != '"' {
				j++
			}
			if j < n {
				j++
			}
			tokens = append(tokens, fmtToken{text: string(runes[i:j])})
			i = j
			continue
		}

		if ch == '`' {
			j := i + 1
			for j < n && runes[j] != '`' {
				j++
			}
			if j < n {
				j++
			}
			tokens = append(tokens, fmtToken{text: string(runes[i:j])})
			i = j
			continue
		}

		if isWordChar(ch) {
			j := i
			for j < n && isWordChar(runes[j]) {
				j++
			}
			tokens = append(tokens, fmtToken{text: string(runes[i:j])})
			i = j
			continue
		}

		if i+1 < n {
			two := string(runes[i : i+2])
			if two == "<=" || two == ">=" || two == "!=" || two == "<>" || two == "||" {
				tokens = append(tokens, fmtToken{text: two})
				i += 2
				continue
			}
		}

		tokens = append(tokens, fmtToken{text: string(ch)})
		i++
	}

	return tokens
}

// formatSQL reformats a SQL string with consistent keyword capitalization
// and line breaks before major clauses.
func formatSQL(input string) string {
	tokens := tokenizeForFormat(input)
	if len(tokens) == 0 {
		return strings.TrimSpace(input)
	}

	const indentStr = "    "
	var lines []string
	var current strings.Builder
	lineHasContent := false

	flushLine := func() {
		line := strings.TrimRight(current.String(), " \t")
		if line != "" {
			lines = append(lines, line)
		}
		current.Reset()
		lineHasContent = false
	}

	writeToken := func(tok fmtToken) {
		upper := strings.ToUpper(tok.text)
		if !tok.isString && formatKeywordSet[upper] {
			current.WriteString(upper)
		} else {
			current.WriteString(tok.text)
		}
		lineHasContent = true
	}

	needSpaceBefore := func(tok fmtToken, prevText string) bool {
		if !lineHasContent {
			return false
		}
		prevUpper := strings.ToUpper(prevText)
		if tok.text == ")" || tok.text == "," {
			return false
		}
		if prevText == "(" {
			return false
		}
		if tok.text == "(" && funcLikeKeywords[prevUpper] {
			return false
		}
		if prevText == "." || tok.text == "." {
			return false
		}
		return true
	}

	for i, tok := range tokens {
		upper := strings.ToUpper(tok.text)

		if tok.text == ";" {
			if lineHasContent {
				current.WriteString(";")
			}
			flushLine()
			continue
		}

		if tok.isString {
			prevText := ""
			if i > 0 {
				prevText = tokens[i-1].text
			}
			if needSpaceBefore(tok, prevText) {
				current.WriteString(" ")
			}
			current.WriteString(tok.text)
			lineHasContent = true
			continue
		}

		if majorClauseKeywords[upper] {
			if upper == "FROM" && i > 0 && strings.ToUpper(tokens[i-1].text) == "DELETE" {
				if lineHasContent {
					current.WriteString(" ")
				}
				writeToken(tok)
				continue
			}
			if lineHasContent || len(lines) > 0 {
				flushLine()
			}
			writeToken(tok)
			continue
		}

		if upper == "JOIN" {
			prevUpper := ""
			if i > 0 {
				prevUpper = strings.ToUpper(tokens[i-1].text)
			}
			if joinModifierKeywords[prevUpper] {
				if lineHasContent {
					current.WriteString(" ")
				}
				writeToken(tok)
				continue
			}
			if lineHasContent || len(lines) > 0 {
				flushLine()
			}
			writeToken(tok)
			continue
		}

		if joinStartKeywords[upper] {
			if lineHasContent || len(lines) > 0 {
				flushLine()
			}
			writeToken(tok)
			continue
		}

		if indentKeywords[upper] {
			if lineHasContent || len(lines) > 0 {
				flushLine()
				current.WriteString(indentStr)
			}
			writeToken(tok)
			continue
		}

		prevText := ""
		if i > 0 {
			prevText = tokens[i-1].text
		}
		if needSpaceBefore(tok, prevText) {
			current.WriteString(" ")
		}
		writeToken(tok)
	}

	flushLine()
	return strings.Join(lines, "\n")
}
