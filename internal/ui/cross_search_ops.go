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
// re-invoking for the next batch. remaining is how many more hits fit under
// the global cap (based on results already shown). gen ties the batch to the
// active search so Hide / a newer StartSearch can drop stale deliveries.
func (m Model) runCrossSearchBatch(query string, batchStart int, gen uint64) tea.Cmd {
	conn := m.connection
	if conn == nil {
		return nil
	}
	remaining := crossSearchMaxResults - len(m.crossSearch.results)
	if remaining <= 0 {
		return func() tea.Msg {
			return crossSearchResultMsg{
				gen:      gen,
				done:     true,
				capped:   true,
				batchEnd: batchStart,
			}
		}
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
	likePat := escapeLikePattern(query)

	return func() tea.Msg {
		var results []SearchResult
		skipped := 0
		capped := false
		for _, table := range batch {
			if len(results) >= remaining {
				capped = true
				break
			}
			cols := columnCache[table]
			if cols == nil {
				fetched, err := d.TableSchema(table)
				if err != nil {
					skipped++
					continue
				}
				cols = fetched
			}
			// Prefer typed text/JSON columns; skip binary and non-text types so
			// CAST-heavy scans don't thrash large numeric/date tables.
			var conditions []string
			for _, col := range cols {
				if !isCrossSearchableType(col.Type) {
					continue
				}
				ident := quoteCrossSearchIdent(driver, col.Name)
				if driver == db.DriverMySQL {
					conditions = append(conditions, fmt.Sprintf(
						"CAST(%s AS CHAR) LIKE '%%%s%%' ESCAPE '!'", ident, likePat))
				} else {
					conditions = append(conditions, fmt.Sprintf(
						"CAST(%s AS TEXT) LIKE '%%%s%%' ESCAPE '!'", ident, likePat))
				}
			}
			if len(conditions) == 0 {
				continue
			}
			queryStr := fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT 20",
				quoteCrossSearchIdent(driver, table), strings.Join(conditions, " OR "))
			result, err := d.Execute(queryStr)
			if err != nil {
				skipped++
				continue
			}
			for _, row := range result.Rows {
				if len(results) >= remaining {
					capped = true
					break
				}
				for ci, col := range result.Columns {
					if ci < len(row) && strings.Contains(strings.ToLower(row[ci]), strings.ToLower(query)) {
						results = append(results, SearchResult{
							Table:  table,
							Column: col.Name,
							Value:  row[ci],
							Row:    row,
						})
						break // one match per row
					}
				}
			}
			if capped {
				break
			}
		}
		done := end >= len(tables) || capped
		return crossSearchResultMsg{
			gen:        gen,
			results:    results,
			tablesDone: len(batch),
			skipped:    skipped,
			capped:     capped,
			done:       done,
			batchEnd:   end,
		}
	}
}

// isCrossSearchableType reports whether a column should be included in :grep.
// Text/JSON/UUID-like types are searched; binary, numeric, and temporal types
// are skipped so CAST(... AS TEXT) does not scan every integer column.
func isCrossSearchableType(colType string) bool {
	if db.IsBinaryType(colType) {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(colType))
	if i := strings.IndexByte(t, '('); i > 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(t)
	// SQLite often stores undeclared / affinity-free columns with an empty type.
	if t == "" || t == "string" {
		return true
	}
	switch t {
	case "text", "varchar", "char", "nvarchar", "nchar", "character",
		"character varying", "citext", "name", "xml",
		"json", "jsonb", "clob", "tinytext", "mediumtext", "longtext",
		"uuid", "enum", "set":
		return true
	}
	return false
}

// escapeLikePattern escapes !, %, and _ for a LIKE pattern used with ESCAPE '!',
// then doubles single quotes for SQL string literals. '!' avoids backslash
// quoting differences across MySQL/Postgres/SQLite.
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `!`, `!!`)
	s = strings.ReplaceAll(s, `%`, `!%`)
	s = strings.ReplaceAll(s, `_`, `!_`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return s
}

// quoteCrossSearchIdent quotes an identifier for the active driver.
func quoteCrossSearchIdent(driver db.Driver, name string) string {
	switch driver {
	case db.DriverMySQL:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	default:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
}
