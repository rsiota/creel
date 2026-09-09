package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	jsonTreeCollapsedGlyph = "▸"
	jsonTreeExpandedGlyph  = "▾"
	jsonTreeRootPath       = "."
	jsonTreeMaxLines       = 16
)

// jsonTreeRow is one visible line in the inspector JSON tree.
type jsonTreeRow struct {
	path     string
	depth    int
	foldable bool
	open     bool
	// label is the key / index shown before the value (empty on the root row).
	label string
	// summary is set for containers ("{n keys}" / "[n]"); leafText for scalars.
	summary  string
	leafText string // already syntax-highlighted when non-empty
}

// parseJSONContainer unmarshals raw when it is a JSON object or array.
func parseJSONContainer(raw string) (interface{}, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || (raw[0] != '{' && raw[0] != '[') {
		return nil, false
	}
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false
	}
	switch v.(type) {
	case map[string]interface{}, []interface{}:
		return v, true
	default:
		return nil, false
	}
}

// isJSONValue reports whether raw is a JSON object or array (same gate as
// formatJSON / the inspector foldable tree).
func isJSONValue(raw string) bool {
	_, ok := parseJSONContainer(raw)
	return ok
}

func jsonContainerSummary(v interface{}) string {
	switch t := v.(type) {
	case map[string]interface{}:
		n := len(t)
		if n == 1 {
			return "{1 key}"
		}
		return fmt.Sprintf("{%d keys}", n)
	case []interface{}:
		return fmt.Sprintf("[%d]", len(t))
	default:
		return "{…}"
	}
}

func jsonKeySeg(k string) string { return strconv.Quote(k) }
func jsonIndexSeg(i int) string  { return strconv.Itoa(i) }

func sortedJSONKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// buildJSONTreeRows flattens v into visible rows given which paths are open.
// The root path is always included; nested containers appear only when their
// ancestors (and themselves, for children) are open.
func buildJSONTreeRows(v interface{}, open map[string]bool) []jsonTreeRow {
	if open == nil {
		open = map[string]bool{jsonTreeRootPath: true}
	}
	var rows []jsonTreeRow
	appendJSONContainerRow(&rows, v, jsonTreeRootPath, 0, "", open)
	return rows
}

func appendJSONContainerRow(rows *[]jsonTreeRow, v interface{}, path string, depth int, label string, open map[string]bool) {
	isOpen := open[path]
	*rows = append(*rows, jsonTreeRow{
		path:     path,
		depth:    depth,
		foldable: true,
		open:     isOpen,
		label:    label,
		summary:  jsonContainerSummary(v),
	})
	if !isOpen {
		return
	}
	switch t := v.(type) {
	case map[string]interface{}:
		for _, k := range sortedJSONKeys(t) {
			childPath := path + "/" + jsonKeySeg(k)
			appendJSONValueRow(rows, t[k], childPath, depth+1, k, true, open)
		}
	case []interface{}:
		for i, item := range t {
			childPath := path + "/" + jsonIndexSeg(i)
			appendJSONValueRow(rows, item, childPath, depth+1, strconv.Itoa(i), false, open)
		}
	}
}

func appendJSONValueRow(rows *[]jsonTreeRow, v interface{}, path string, depth int, label string, isKey bool, open map[string]bool) {
	switch v.(type) {
	case map[string]interface{}, []interface{}:
		display := label
		if isKey {
			display = strconv.Quote(label)
		}
		appendJSONContainerRow(rows, v, path, depth, display, open)
	default:
		display := label
		if isKey {
			display = strconv.Quote(label)
		}
		*rows = append(*rows, jsonTreeRow{
			path:     path,
			depth:    depth,
			foldable: false,
			label:    display,
			leafText: formatJSONTreeLeaf(v),
		})
	}
}

func formatJSONTreeLeaf(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return highlightJSON("null")
	case bool:
		if t {
			return highlightJSON("true")
		}
		return highlightJSON("false")
	case float64:
		// encoding/json decodes numbers as float64; Marshal preserves compact form.
		b, err := json.Marshal(t)
		if err != nil {
			return highlightJSON(fmt.Sprint(t))
		}
		return highlightJSON(string(b))
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			return highlightJSON(strconv.Quote(t))
		}
		return highlightJSON(string(b))
	case json.Number:
		return highlightJSON(t.String())
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return lipgloss.NewStyle().Foreground(colorFg).Render(fmt.Sprint(t))
		}
		return highlightJSON(string(b))
	}
}

// jsonTreeCollapsedLine is the one-line summary when the field fold is closed.
func jsonTreeCollapsedLine(raw string, width int) (string, bool) {
	v, ok := parseJSONContainer(raw)
	if !ok {
		return "", false
	}
	fg := lipgloss.NewStyle().Foreground(colorFg)
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	line := fg.Render(jsonTreeCollapsedGlyph+" ") + muted.Render(jsonContainerSummary(v))
	return padPlainToWidth(line, width), true
}

// renderJSONTreeContent renders visible rows for an open field fold. cursor is
// the selected row index in the full row list; scroll is the first visible row.
func renderJSONTreeContent(rows []jsonTreeRow, width, cursor, scroll int) string {
	if len(rows) == 0 {
		return padPlainToWidth("", width)
	}
	if scroll < 0 {
		scroll = 0
	}
	if scroll >= len(rows) {
		scroll = len(rows) - 1
	}
	end := scroll + jsonTreeMaxLines
	if end > len(rows) {
		end = len(rows)
	}

	muted := lipgloss.NewStyle().Foreground(colorMuted)
	fg := lipgloss.NewStyle().Foreground(colorFg)
	sel := lipgloss.NewStyle().Background(colorPrimary).Foreground(colorBg)

	var lines []string
	for i := scroll; i < end; i++ {
		r := rows[i]
		indent := strings.Repeat("  ", r.depth)
		var body string
		switch {
		case r.foldable:
			glyph := jsonTreeCollapsedGlyph
			if r.open {
				glyph = jsonTreeExpandedGlyph
			}
			// Glyph alone marks foldability; skip "{n keys}" / "[n]" on tree
			// rows (collapsed field line still shows the summary).
			if r.label == "" {
				body = fg.Render(glyph)
			} else {
				body = fg.Render(glyph+" ") + highlightJSON(r.label)
			}
		default:
			body = highlightJSON(r.label) + muted.Render(": ") + r.leafText
		}
		plain := indent + stripANSIForWidth(body)
		if i == cursor {
			lines = append(lines, sel.Width(width).Render(truncateCell(plain, width)))
			continue
		}
		styled := indent + body
		if lipgloss.Width(styled) > width {
			lines = append(lines, padPlainToWidth(ansi.Truncate(styled, width, "…"), width))
		} else {
			lines = append(lines, padPlainToWidth(styled, width))
		}
	}
	return strings.Join(lines, "\n")
}

// jsonTreeValueContent renders the inspector value box for a JSON field.
func jsonTreeValueContent(raw string, width int, expanded bool, open map[string]bool, cursor, scroll int) (string, bool) {
	if !expanded {
		return jsonTreeCollapsedLine(raw, width)
	}
	v, ok := parseJSONContainer(raw)
	if !ok {
		return "", false
	}
	rows := buildJSONTreeRows(v, open)
	return renderJSONTreeContent(rows, width, cursor, scroll), true
}

func padPlainToWidth(line string, width int) string {
	w := lipgloss.Width(line)
	if w > width {
		return ansi.Truncate(line, width, "…")
	}
	if w < width {
		return line + strings.Repeat(" ", width-w)
	}
	return line
}

// stripANSIForWidth returns plain text for selection/truncation sizing.
func stripANSIForWidth(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
