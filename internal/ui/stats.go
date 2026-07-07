package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchColumnStats computes statistics for the cursor column and displays
// them as a transient status-bar message. For simple SELECT * FROM <table>
// queries, stats are computed server-side across the full filtered result
// set. For arbitrary queries, stats are computed client-side on the current
// page.
func (m *Model) fetchColumnStats() tea.Cmd {
	if m.results.NumRows() == 0 || m.connection == nil {
		return nil
	}
	col := m.results.CursorCol()
	colName := m.results.ColumnName(col)
	if colName == "" {
		return nil
	}
	dbType := m.results.ColumnType(col)
	numeric := isNumericType(dbType)

	// Server-side stats on the full filtered result set.
	if m.canFilter() {
		table := parseSimpleSelectTable(m.baseQuery)
		var aggregate string
		if numeric {
			aggregate = fmt.Sprintf(
				"SELECT COUNT(%s), COUNT(DISTINCT %s), MIN(%s), MAX(%s), SUM(%s), AVG(%s) FROM %s",
				colName, colName, colName, colName, colName, colName, table)
		} else {
			aggregate = fmt.Sprintf(
				"SELECT COUNT(%s), COUNT(DISTINCT %s), MIN(%s), MAX(%s) FROM %s",
				colName, colName, colName, colName, table)
		}
		if len(m.filters) > 0 {
			aggregate += " WHERE " + strings.Join(m.filters, " AND ")
		}

		conn := m.connection
		return func() tea.Msg {
			result, err := conn.DB().Execute(aggregate)
			if err != nil || len(result.Rows) == 0 {
				return statsMsg{column: colName, stats: "stats error"}
			}
			row := result.Rows[0]
			stats := formatColumnStats(row, numeric)
			return statsMsg{column: colName, stats: stats}
		}
	}

	// Client-side fallback: stats on the current page.
	row := computeClientStats(m.results, col, numeric)
	stats := formatColumnStats(row, numeric)
	m.statsMsg = fmt.Sprintf("%s: %s  (page only)", colName, stats)
	return nil
}

// fetchTotalRows runs an async COUNT(*) on the current table and returns a
// countMsg. Returns nil if no table can be resolved.
func (m *Model) fetchTotalRows() tea.Cmd {
	if m.connection == nil || !m.canFilter() {
		return nil
	}
	table := parseSimpleSelectTable(m.baseQuery)
	if table == "" {
		return nil
	}
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if len(m.filters) > 0 {
		countQuery += " WHERE " + strings.Join(m.filters, " AND ")
	}
	conn := m.connection
	return func() tea.Msg {
		result, err := conn.DB().Execute(countQuery)
		if err != nil || len(result.Rows) == 0 {
			return countMsg{total: 0, err: fmt.Errorf("count failed")}
		}
		n, _ := strconv.Atoi(result.Rows[0][0])
		return countMsg{total: n}
	}
}

// computeClientStats iterates the visible rows for a column and returns a
// stats row in the same layout as the server-side query: count, distinct,
// min, max, [sum, avg].
func computeClientStats(r ResultsTable, col int, numeric bool) []string {
	count := 0
	seen := make(map[string]bool)
	hasMin, hasMax := false, false
	minVal, maxVal := 0.0, 0.0
	var sum float64
	for rowIdx := 0; rowIdx < r.NumRows(); rowIdx++ {
		val := r.RowValue(rowIdx, col)
		if val == "" || val == "NULL" {
			continue
		}
		count++
		seen[val] = true
		if numeric {
			n, ok := parseFloat(val)
			if ok {
				sum += n
				if !hasMin || n < minVal {
					minVal = n
					hasMin = true
				}
				if !hasMax || n > maxVal {
					maxVal = n
					hasMax = true
				}
			}
		} else {
			if !hasMin || val < fmt.Sprintf("%v", minVal) {
				minVal = 0
				hasMin = true
			}
		}
	}
	distinct := fmt.Sprintf("%d", len(seen))
	row := []string{fmt.Sprintf("%d", count), distinct}
	if numeric {
		if hasMin {
			row = append(row, fmt.Sprintf("%g", minVal))
		} else {
			row = append(row, "NULL")
		}
		if hasMax {
			row = append(row, fmt.Sprintf("%g", maxVal))
		} else {
			row = append(row, "NULL")
		}
		row = append(row, fmt.Sprintf("%g", sum))
		if count > 0 {
			row = append(row, fmt.Sprintf("%g", sum/float64(count)))
		} else {
			row = append(row, "NULL")
		}
	} else {
		if mi, ma := minString(seen), maxString(seen); mi != "" || ma != "" {
			row = append(row, mi, ma)
		} else {
			row = append(row, "NULL", "NULL")
		}
	}
	return row
}

// formatColumnStats renders a stats row as a compact status-bar string.
func formatColumnStats(row []string, numeric bool) string {
	get := func(i int) string {
		if i < len(row) {
			return row[i]
		}
		return "?"
	}
	if numeric {
		return fmt.Sprintf("count %s · distinct %s · min %s · max %s · sum %s · avg %s",
			get(0), get(1), get(2), get(3), get(4), get(5))
	}
	return fmt.Sprintf("count %s · distinct %s · min %s · max %s",
		get(0), get(1), get(2), get(3))
}

// parseFloat parses a numeric cell value (tolerating integers and decimals).
func parseFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err == nil
}

// minString / maxString find the lexicographically smallest/largest key.
func minString(m map[string]bool) string {
	first := true
	var result string
	for k := range m {
		if first || k < result {
			result = k
			first = false
		}
	}
	return result
}

func maxString(m map[string]bool) string {
	first := true
	var result string
	for k := range m {
		if first || k > result {
			result = k
			first = false
		}
	}
	return result
}
