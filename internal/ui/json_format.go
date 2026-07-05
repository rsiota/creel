package ui

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// formatJSON validates raw as a JSON object or array and returns a
// 2-space-indented version. Scalars (strings, numbers, bools) are not
// reformatted. Returns ok=false when raw is not a JSON object/array.
func formatJSON(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || (raw[0] != '{' && raw[0] != '[') {
		return "", false
	}
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "", false
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", false
	}
	return string(pretty), true
}

// compactJSON validates raw as JSON and returns a compact (single-line)
// representation. Returns ok=false when raw is not valid JSON.
func compactJSON(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 {
		return "", false
	}
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "", false
	}
	compact, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return string(compact), true
}

// highlightJSON applies lipgloss syntax colors to pretty-printed JSON text:
//   - keys    → colorAccent  (purple)
//   - strings → colorSuccess (green)
//   - numbers → colorEdit    (orange)
//   - booleans/null → colorPrimary (blue)
//   - punctuation    → colorMuted  (grey)
//
// The function is token-based and safe to run on any text (including
// truncated lines); unrecognized characters pass through unstyled.
func highlightJSON(s string) string {
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent)
	strStyle := lipgloss.NewStyle().Foreground(colorSuccess)
	numStyle := lipgloss.NewStyle().Foreground(colorEdit)
	litStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	punctStyle := lipgloss.NewStyle().Foreground(colorMuted)

	var b strings.Builder
	i, n := 0, len(s)
	for i < n {
		c := s[i]
		switch {
		case c == '"':
			j := i + 1
			for j < n {
				if s[j] == '\\' && j+1 < n {
					j += 2
					continue
				}
				if s[j] == '"' {
					j++
					break
				}
				j++
			}
			tok := s[i:j]
			// Peek past trailing whitespace for ':' → this is a key.
			k := j
			for k < n && (s[k] == ' ' || s[k] == '\t') {
				k++
			}
			if k < n && s[k] == ':' {
				b.WriteString(keyStyle.Render(tok))
			} else {
				b.WriteString(strStyle.Render(tok))
			}
			i = j

		case c == '-' || (c >= '0' && c <= '9'):
			j := i + 1
			for j < n {
				ch := s[j]
				if (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' ||
					ch == '+' || ch == 'e' || ch == 'E' {
					j++
					continue
				}
				break
			}
			b.WriteString(numStyle.Render(s[i:j]))
			i = j

		case c == 't' && strings.HasPrefix(s[i:], "true"):
			b.WriteString(litStyle.Render("true"))
			i += 4
		case c == 'f' && strings.HasPrefix(s[i:], "false"):
			b.WriteString(litStyle.Render("false"))
			i += 5
		case c == 'n' && strings.HasPrefix(s[i:], "null"):
			b.WriteString(litStyle.Render("null"))
			i += 4

		case c == '{' || c == '}' || c == '[' || c == ']' ||
			c == ',' || c == ':':
			b.WriteString(punctStyle.Render(string(c)))
			i++

		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
