package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

const crossSearchMaxResults = 200

// startCrossSearch kicks off the async cross-table search. It returns a
// crossSearchStartMsg which the handler converts into the actual search batch.
func (m Model) startCrossSearch() tea.Cmd {
	return func() tea.Msg {
		return crossSearchStartMsg{}
	}
}

// runCrossSearchBatch searches a batch of tables for the query string and
// returns partial results. The caller handles accumulating results and
// re-invoking for the next batch.
func (m Model) runCrossSearchBatch(query string, batchStart int) tea.Cmd {
	conn := m.connection
	if conn == nil {
		return nil
	}
	d := conn.DB()
	driver := conn.Config().Driver
	tables := m.tables
	columnCache := m.columnCache
	batchSize := 3
	end := batchStart + batchSize
	if end > len(tables) {
		end = len(tables)
	}
	batch := tables[batchStart:end]

	return func() tea.Msg {
		var results []SearchResult
		for _, table := range batch {
			if len(results) >= crossSearchMaxResults {
				break
			}
			cols := columnCache[table]
			if cols == nil {
				fetched, err := d.TableSchema(table)
				if err != nil {
					continue
				}
				cols = fetched
			}
			// Build WHERE clause casting each text-compatible column to CHAR.
			var conditions []string
			for _, col := range cols {
				lower := strings.ToLower(col.Type)
				if strings.Contains(lower, "blob") || strings.Contains(lower, "binary") {
					continue
				}
				escapedQuery := strings.ReplaceAll(query, "'", "''")
				if driver == db.DriverMySQL {
					conditions = append(conditions, fmt.Sprintf("CAST(`%s` AS CHAR) LIKE '%%%s%%'",
						strings.ReplaceAll(col.Name, "`", "``"), escapedQuery))
				} else {
					conditions = append(conditions, fmt.Sprintf("CAST(\"%s\" AS TEXT) LIKE '%%%s%%'",
						strings.ReplaceAll(col.Name, "\"", "\"\""), escapedQuery))
				}
			}
			if len(conditions) == 0 {
				continue
			}
			queryStr := fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT 20",
				table, strings.Join(conditions, " OR "))
			result, err := d.Execute(queryStr)
			if err != nil {
				continue
			}
			for _, row := range result.Rows {
				if len(results) >= crossSearchMaxResults {
					break
				}
				for ci, col := range result.Columns {
					if ci < len(row) && strings.Contains(strings.ToLower(row[ci]), strings.ToLower(query)) {
						val := row[ci]
						if len(val) > 80 {
							val = val[:80]
						}
						results = append(results, SearchResult{
							Table:  table,
							Column: col.Name,
							Value:  val,
							Row:    row,
						})
						break // one match per row
					}
				}
			}
		}
		done := end >= len(tables)
		return crossSearchResultMsg{
			results:    results,
			tablesDone: len(batch),
			done:       done,
			batchEnd:   end,
		}
	}
}
