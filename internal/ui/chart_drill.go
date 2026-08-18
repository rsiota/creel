package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const histBarDash = "–"

// drillChartBar keeps rows that match the selected bar (or hist bin) and
// restores the results grid. Line/scatter charts have no bar to drill.
func (m *Model) drillChartBar() tea.Cmd {
	c := m.chartPanel
	if c.kind == chartKindLine || c.kind == chartKindScatter {
		m.schemaMsg = "enter opens rows for a bar — use :bar, :freq, :pie, or :hist"
		return nil
	}
	if !m.canFilter() {
		m.schemaMsg = "can't filter this query — enter needs a simple SELECT"
		return nil
	}
	vis := c.visibleBars()
	if c.cursor < 0 || c.cursor >= len(vis) {
		return nil
	}
	col := c.filterCol
	if col == "" {
		m.schemaMsg = "can't tell which column that bar came from"
		return nil
	}
	dbType := ""
	if idx := m.resultColumnIndex(col); idx >= 0 {
		dbType = m.results.ColumnType(idx)
	}
	clause := chartBarFilter(col, dbType, vis[c.cursor], c.bars, c.kind)
	if clause == "" {
		m.schemaMsg = "nothing to filter on that bar"
		return nil
	}
	m.filters = removeColumnFilters(m.filters, col)
	m.filters = append(m.filters, clause)
	m.chartPanel.Hide()
	m.applyFilteredQuery()
	m.page = 0
	m.preserveCursorCol()
	return m.runPageQuery()
}

func chartBarFilter(col, dbType string, bar chartBar, all []chartBar, kind chartKind) string {
	if bar.label == otherBarLabel {
		return chartOtherFilter(col, dbType, all)
	}
	if bar.label == "(null)" {
		if isNumericType(dbType) {
			return col + " IS NULL"
		}
		return fmt.Sprintf("(%s IS NULL OR %s = '')", col, col)
	}
	if kind == chartKindHist {
		if lo, hi, last, ok := histBarRange(bar, all); ok {
			if last {
				return fmt.Sprintf("%s >= %s AND %s <= %s", col, lo, col, hi)
			}
			return fmt.Sprintf("%s >= %s AND %s < %s", col, lo, col, hi)
		}
	}
	return buildQuickFilter(col, bar.label, dbType, false)
}

func histBarRange(bar chartBar, all []chartBar) (lo, hi string, last, ok bool) {
	i := strings.Index(bar.label, histBarDash)
	if i <= 0 {
		return "", "", false, false
	}
	lo, hi = bar.label[:i], bar.label[i+len(histBarDash):]
	if lo == "" || hi == "" {
		return "", "", false, false
	}
	last = len(all) > 0 && all[len(all)-1].label == bar.label
	return lo, hi, last, true
}

func chartOtherFilter(col, dbType string, all []chartBar) string {
	if len(all) <= chartBarLimit {
		return ""
	}
	var vals []string
	hasNull := false
	for _, b := range all[chartBarLimit:] {
		if b.label == "(null)" {
			hasNull = true
			continue
		}
		vals = append(vals, b.label)
	}
	var parts []string
	if hasNull {
		parts = append(parts, chartBarFilter(col, dbType, chartBar{label: "(null)"}, nil, chartKindBar))
	}
	if len(vals) == 1 {
		parts = append(parts, buildQuickFilter(col, vals[0], dbType, false))
	} else if len(vals) > 1 {
		formatted := make([]string, len(vals))
		for i, v := range vals {
			formatted[i] = formatFilterValue(v, dbType)
		}
		parts = append(parts, fmt.Sprintf("%s IN (%s)", col, strings.Join(formatted, ", ")))
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}
