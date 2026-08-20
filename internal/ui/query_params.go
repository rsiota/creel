package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// queryParams holds named values substituted into SQL before execution.
// Placeholders look like :name (letter-leading identifier). Postgres ::casts,
// string/backtick literals, and comments are left alone. Cleared on disconnect.
func (m *Model) ensureQueryParams() {
	if m.queryParams == nil {
		m.queryParams = make(map[string]string)
	}
}

// expandQueryParams replaces :name placeholders in sql with SQL literals from
// m.queryParams. Returns sql unchanged when it has no placeholders. An
// undefined name is an error so a typo never reaches the driver as raw text.
func (m *Model) expandQueryParams(sql string) (string, error) {
	return expandQueryParams(sql, m.queryParams)
}

// expandQueryParams is the pure substitution helper (exported to tests via
// package-level visibility).
func expandQueryParams(sql string, params map[string]string) (string, error) {
	if !strings.Contains(sql, ":") {
		return sql, nil
	}
	var b strings.Builder
	b.Grow(len(sql))
	runes := []rune(sql)
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == '\'':
			end := scanSQLString(runes, i, '\'')
			b.WriteString(string(runes[i:end]))
			i = end
		case r == '"':
			end := scanSQLString(runes, i, '"')
			b.WriteString(string(runes[i:end]))
			i = end
		case r == '`':
			end := scanSQLString(runes, i, '`')
			b.WriteString(string(runes[i:end]))
			i = end
		case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
			end := i + 2
			for end < len(runes) && runes[end] != '\n' {
				end++
			}
			b.WriteString(string(runes[i:end]))
			i = end
		case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
			end := i + 2
			for end+1 < len(runes) && !(runes[end] == '*' && runes[end+1] == '/') {
				end++
			}
			if end+1 < len(runes) {
				end += 2
			}
			b.WriteString(string(runes[i:end]))
			i = end
		case r == ':':
			if i+1 < len(runes) && runes[i+1] == ':' {
				// Postgres cast (::type) — not a parameter.
				b.WriteString("::")
				i += 2
				continue
			}
			if i+1 < len(runes) && isParamNameStart(runes[i+1]) {
				j := i + 1
				for j < len(runes) && isParamNameRune(runes[j]) {
					j++
				}
				name := string(runes[i+1 : j])
				val, ok := params[name]
				if !ok {
					return "", fmt.Errorf("undefined parameter :%s (set with :param %s <value>)", name, name)
				}
				b.WriteString(sqlEscape(val, ""))
				i = j
				continue
			}
			b.WriteRune(r)
			i++
		default:
			b.WriteRune(r)
			i++
		}
	}
	return b.String(), nil
}

func isParamNameStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isParamNameRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// scanSQLString returns the index just past the closing quote (or len if
// unclosed). Handles doubled quotes as escapes (SQL standard / MySQL).
func scanSQLString(runes []rune, start int, quote rune) int {
	i := start + 1
	for i < len(runes) {
		if runes[i] == quote {
			if i+1 < len(runes) && runes[i+1] == quote {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return len(runes)
}

// validParamName reports whether name is a legal :param identifier.
func validParamName(name string) bool {
	if name == "" {
		return false
	}
	runes := []rune(name)
	if !isParamNameStart(runes[0]) {
		return false
	}
	for _, r := range runes[1:] {
		if !isParamNameRune(r) {
			return false
		}
	}
	return true
}

// exParam implements :param / :params — list, get, set, or clear named query
// parameters used as :name placeholders in the editor.
//
//	:param                  list all
//	:param name             show one
//	:param name value…      set (remaining args joined; quotes via shell split)
//	:param!                 clear all
//	:param! name            clear one
func (m *Model) exParam(args []string, force bool) tea.Cmd {
	m.ensureQueryParams()

	if force {
		if len(args) == 0 {
			n := len(m.queryParams)
			m.queryParams = make(map[string]string)
			if n == 0 {
				m.schemaMsg = "no parameters to clear"
			} else {
				m.schemaMsg = fmt.Sprintf("cleared %d parameter%s", n, pluralIf(n != 1, "s"))
			}
			return nil
		}
		name := args[0]
		if !validParamName(name) {
			m.schemaMsg = fmt.Sprintf("invalid parameter name %q", name)
			return nil
		}
		if _, ok := m.queryParams[name]; !ok {
			m.schemaMsg = fmt.Sprintf("parameter :%s is not set", name)
			return nil
		}
		delete(m.queryParams, name)
		m.schemaMsg = fmt.Sprintf("cleared :%s", name)
		return nil
	}

	switch len(args) {
	case 0:
		m.schemaMsg = m.formatQueryParams()
		return nil
	case 1:
		name := args[0]
		if !validParamName(name) {
			m.schemaMsg = fmt.Sprintf("invalid parameter name %q", name)
			return nil
		}
		val, ok := m.queryParams[name]
		if !ok {
			m.schemaMsg = fmt.Sprintf("parameter :%s is not set", name)
			return nil
		}
		m.schemaMsg = fmt.Sprintf(":%s = %s", name, displayParamValue(val))
		return nil
	default:
		name := args[0]
		if !validParamName(name) {
			m.schemaMsg = fmt.Sprintf("invalid parameter name %q", name)
			return nil
		}
		val := strings.Join(args[1:], " ")
		m.queryParams[name] = val
		m.schemaMsg = fmt.Sprintf(":%s = %s", name, displayParamValue(val))
		return nil
	}
}

func displayParamValue(val string) string {
	if val == "NULL" {
		return "NULL"
	}
	if isBareNumeric(val) {
		return val
	}
	return "'" + strings.ReplaceAll(val, "'", "''") + "'"
}

func (m Model) formatQueryParams() string {
	if len(m.queryParams) == 0 {
		return "no parameters (:param name value)"
	}
	names := make([]string, 0, len(m.queryParams))
	for n := range m.queryParams {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf(":%s=%s", n, displayParamValue(m.queryParams[n]))
	}
	return "params: " + strings.Join(parts, "  ")
}

// paramStatusLabel is the compact status-bar indicator when any params are set.
func (m Model) paramStatusLabel() string {
	n := len(m.queryParams)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("PARAM %d", n)
}

// completeParam suggests existing parameter names for :param / :param!.
func completeParam(m *Model, args []string, partial string) []string {
	if len(m.queryParams) == 0 {
		return nil
	}
	// Only complete the first argument (the name).
	if len(args) > 0 {
		return nil
	}
	names := make([]string, 0, len(m.queryParams))
	for n := range m.queryParams {
		names = append(names, n)
	}
	sort.Strings(names)
	return filterPrefix(names, partial)
}

func filterPrefix(names []string, partial string) []string {
	if partial == "" {
		return names
	}
	pl := strings.ToLower(partial)
	var out []string
	for _, n := range names {
		if strings.HasPrefix(strings.ToLower(n), pl) {
			out = append(out, n)
		}
	}
	return out
}
