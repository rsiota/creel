package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

const (
	crossSearchMaxResults  = 200
	crossSearchPageRows    = 20
	crossSearchBatchTables = 3
)

// crossSearchPos is a table plus the SQL OFFSET of the next unread match page.
type crossSearchPos struct {
	table  int
	offset int
}

// crossSearchScan is the cursor for one :grep run. The forward pass visits
// each table once (one page of rows). Tables that fill that page are queued
// in deferred so a later page can read the rest, after every table has been
// visited — that keeps the first screen spread across the schema.
type crossSearchScan struct {
	nextTable   int
	tableOffset int
	deferred    []crossSearchPos
}

// startCrossSearch kicks off the async cross-table search. It returns a
// crossSearchStartMsg which the handler converts into the actual search batch.
func (m Model) startCrossSearch() tea.Cmd {
	return func() tea.Msg {
		return crossSearchStartMsg{}
	}
}

// runCrossSearchBatch searches the next few tables (or deferred pages) for
// query. The caller accumulates hits and re-invokes until the hit budget is
// full or the scan is exhausted. gen ties the batch to the active search so
// Hide / a newer StartSearch / Continue can drop stale deliveries.
func (m Model) runCrossSearchBatch(query string, gen uint64) tea.Cmd {
	conn := m.connection
	if conn == nil {
		return nil
	}
	limit := m.crossSearch.hitLimit
	if limit <= 0 {
		limit = crossSearchMaxResults
	}
	remaining := limit - len(m.crossSearch.results)
	scan := crossSearchScan{
		nextTable:   m.crossSearch.nextTable,
		tableOffset: m.crossSearch.tableOffset,
		deferred:    append([]crossSearchPos(nil), m.crossSearch.deferred...),
	}
	if remaining <= 0 {
		more := scan.more(len(m.tables))
		return func() tea.Msg {
			return crossSearchResultMsg{
				gen:         gen,
				done:        !more,
				capped:      more,
				nextTable:   scan.nextTable,
				tableOffset: scan.tableOffset,
				deferred:    scan.deferred,
			}
		}
	}
	d := conn.DB()
	driver := conn.Config().Driver
	tables := append([]string(nil), m.tables...)
	columnCache := m.columnCache
	likePat := escapeLikePattern(query)
	needle := strings.ToLower(query)

	return func() tea.Msg {
		var results []SearchResult
		skipped := 0
		tablesDone := 0
		for range crossSearchBatchTables {
			if len(results) >= remaining {
				break
			}
			kind, idx, offset, ok := scan.peek(len(tables))
			if !ok {
				break
			}
			table := tables[idx]
			cols := columnCache[table]
			if cols == nil {
				fetched, err := d.TableSchema(table)
				if err != nil {
					skipped++
					scan.fail(kind)
					if kind == scanForward {
						tablesDone++
					}
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
				scan.finishPage(kind, 0, false, false)
				if kind == scanForward {
					tablesDone++
				}
				continue
			}
			// ORDER BY every column so OFFSET resumes the same row sequence.
			order := make([]string, len(cols))
			for i := range cols {
				order[i] = fmt.Sprintf("%d", i+1)
			}
			queryStr := fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
				quoteCrossSearchIdent(driver, table), strings.Join(conditions, " OR "),
				strings.Join(order, ", "), crossSearchPageRows, offset)
			result, err := d.Execute(queryStr)
			if err != nil {
				skipped++
				scan.fail(kind)
				if kind == scanForward {
					tablesDone++
				}
				continue
			}
			walked := 0
			capped := false
			for _, row := range result.Rows {
				if len(results) >= remaining {
					capped = true
					break
				}
				walked++
				for ci, col := range result.Columns {
					if ci < len(row) && strings.Contains(strings.ToLower(row[ci]), needle) {
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
			full := !capped && len(result.Rows) == crossSearchPageRows
			scan.finishPage(kind, walked, full, capped)
			if kind == scanForward && !capped {
				tablesDone++
			}
			if capped {
				break
			}
		}
		more := scan.more(len(tables))
		return crossSearchResultMsg{
			gen:         gen,
			results:     results,
			tablesDone:  tablesDone,
			skipped:     skipped,
			capped:      more && len(results) >= remaining,
			done:        !more,
			nextTable:   scan.nextTable,
			tableOffset: scan.tableOffset,
			deferred:    scan.deferred,
		}
	}
}

const (
	scanForward = iota
	scanDeferred
)

func (s crossSearchScan) peek(nTables int) (kind, table, offset int, ok bool) {
	if s.nextTable < nTables {
		return scanForward, s.nextTable, s.tableOffset, true
	}
	if len(s.deferred) > 0 {
		d := s.deferred[0]
		return scanDeferred, d.table, d.offset, true
	}
	return 0, 0, 0, false
}

func (s crossSearchScan) more(nTables int) bool {
	return s.nextTable < nTables || len(s.deferred) > 0
}

// fail drops the current forward table or deferred page after an error.
func (s *crossSearchScan) fail(kind int) {
	if kind == scanDeferred {
		if len(s.deferred) > 0 {
			s.deferred = s.deferred[1:]
		}
		return
	}
	s.nextTable++
	s.tableOffset = 0
}

// finishPage advances the cursor after a page. walked is how many SQL rows
// were consumed (OFFSET counts those). full means the page was a complete
// LIMIT, so further rows may exist. capped means the hit budget stopped
// the read before the page ended.
func (s *crossSearchScan) finishPage(kind, walked int, full, capped bool) {
	if kind == scanDeferred {
		if len(s.deferred) == 0 {
			return
		}
		if capped {
			s.deferred[0].offset += walked
			return
		}
		if full {
			item := s.deferred[0]
			item.offset += crossSearchPageRows
			s.deferred = append(s.deferred[1:], item)
			return
		}
		s.deferred = s.deferred[1:]
		return
	}
	if capped {
		s.tableOffset += walked
		return
	}
	if full {
		s.deferred = append(s.deferred, crossSearchPos{
			table:  s.nextTable,
			offset: s.tableOffset + crossSearchPageRows,
		})
	}
	s.nextTable++
	s.tableOffset = 0
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
