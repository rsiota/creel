package db

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DiagnosisFinding is one rule-based observation from an EXPLAIN plan.
type DiagnosisFinding struct {
	Severity string // high, medium, info
	Issue    string
	Table    string
	Detail   string
	Hint     string
}

// IndexLookup fetches secondary indexes for a table during diagnosis.
type IndexLookup func(table string) ([]Index, error)

var (
	pgSeqScanRe    = regexp.MustCompile(`(?i)Seq Scan on ([^\s(]+)`)
	pgIndexScanRe  = regexp.MustCompile(`(?i)(?:Index(?: Only)? Scan|Bitmap (?:Index|Heap) Scan).*?\bon\s+([^\s(]+)`)
	pgFilterColRe  = regexp.MustCompile(`(?i)Filter:\s*\((?:[^\)]*?([\w]+)\s*(?:=|<>|!=|<|>|<=|>=|~~|!~~|LIKE|ILIKE))`)
	sqliteScanRe   = regexp.MustCompile(`(?i)\bSCAN(?: TABLE)?\s+(\S+)`)
	sqliteSearchRe = regexp.MustCompile(`(?i)\bSEARCH(?: TABLE)?\s+(\S+)\s+USING\s+`)
	mysqlUsingTmpRe = regexp.MustCompile(`(?i)Using temporary`)
	mysqlFilesortRe = regexp.MustCompile(`(?i)Using filesort`)
)

// DiagnoseExplain inspects an EXPLAIN / EXPLAIN QUERY PLAN result and returns
// ordered findings (highest severity first). indexes may be nil (skips index
// annotations). Pure heuristics — no AI.
func DiagnoseExplain(driver Driver, plan Result, indexes IndexLookup) []DiagnosisFinding {
	var findings []DiagnosisFinding
	switch driver {
	case DriverPostgres:
		findings = diagnosePostgres(plan)
	case DriverMySQL:
		findings = diagnoseMySQL(plan)
	case DriverSQLite:
		findings = diagnoseSQLite(plan)
	default:
		return nil
	}
	if indexes != nil {
		annotateIndexes(findings, indexes)
	}
	sortFindings(findings)
	if len(findings) == 0 {
		findings = append(findings, DiagnosisFinding{
			Severity: "info",
			Issue:    "No obvious problems",
			Detail:   "Plan has no sequential scans, full table scans, or common red flags",
			Hint:     "Use g e / :explain for the full plan, or :aiexplain for a prose walkthrough",
		})
	}
	return findings
}

func diagnosePostgres(plan Result) []DiagnosisFinding {
	var findings []DiagnosisFinding
	seenSeq := map[string]bool{}
	for _, line := range planLines(plan) {
		if m := pgSeqScanRe.FindStringSubmatch(line); len(m) == 2 {
			table := stripPGIdent(m[1])
			if table == "" || seenSeq[table] {
				continue
			}
			seenSeq[table] = true
			f := DiagnosisFinding{
				Severity: "high",
				Issue:    "Sequential scan",
				Table:    table,
				Detail:   TruncateQuery(strings.TrimSpace(line), 100),
				Hint:     "Add an index matching the filter/join, or confirm a full scan is intentional",
			}
			if cols := pgFilterColumns(line); len(cols) > 0 {
				f.Hint = fmt.Sprintf("Consider an index on (%s) if this filter is selective", strings.Join(cols, ", "))
			}
			// Peek following lines for Filter on nested plan nodes.
			findings = append(findings, f)
			continue
		}
	}
	// Second pass: attach Filter columns from indented lines under Seq Scan.
	findings = enrichPostgresFilters(plan, findings)
	return dedupeFindings(findings)
}

func enrichPostgresFilters(plan Result, findings []DiagnosisFinding) []DiagnosisFinding {
	lines := planLines(plan)
	for i, line := range lines {
		if !pgSeqScanRe.MatchString(line) {
			continue
		}
		table := ""
		if m := pgSeqScanRe.FindStringSubmatch(line); len(m) == 2 {
			table = stripPGIdent(m[1])
		}
		var filterCols []string
		for j := i + 1; j < len(lines) && j <= i+3; j++ {
			l := lines[j]
			if !strings.HasPrefix(strings.TrimLeft(l, " "), " ") && !strings.HasPrefix(l, "  ") {
				// next top-level node — stop if not indented more than seq scan
			}
			trim := strings.TrimSpace(l)
			if strings.HasPrefix(trim, "Filter:") || strings.HasPrefix(trim, "Index Cond:") {
				filterCols = append(filterCols, pgFilterColumns(trim)...)
			}
			if pgSeqScanRe.MatchString(l) || pgIndexScanRe.MatchString(l) {
				break
			}
		}
		if len(filterCols) == 0 {
			continue
		}
		for k := range findings {
			if findings[k].Table == table && findings[k].Issue == "Sequential scan" {
				findings[k].Hint = fmt.Sprintf("Consider an index on (%s) if this filter is selective", strings.Join(uniqueStrings(filterCols), ", "))
			}
		}
	}
	return findings
}

func diagnoseMySQL(plan Result) []DiagnosisFinding {
	col := mysqlExplainCols(plan.Columns)
	var findings []DiagnosisFinding
	seen := map[string]bool{}
	for _, row := range plan.Rows {
		get := func(name string) string {
			i, ok := col[name]
			if !ok || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}
		table := get("table")
		typ := strings.ToLower(get("type"))
		key := get("key")
		extra := get("extra")
		rowsEst := get("rows")

		if typ == "all" || (typ == "" && key == "" && table != "" && table != "null") {
			issueKey := "seq:" + table
			if table != "" && !seen[issueKey] {
				seen[issueKey] = true
				f := DiagnosisFinding{
					Severity: "high",
					Issue:    "Full table scan",
					Table:    table,
					Detail:   fmt.Sprintf("type=%s key=%s rows=%s", nullDash(typ), nullDash(key), nullDash(rowsEst)),
					Hint:     "Add an index matching WHERE/JOIN columns, or confirm a full scan is intentional",
				}
				if n, err := strconv.ParseInt(rowsEst, 10, 64); err == nil && n >= 100000 {
					f.Detail += " (large)"
					f.Severity = "high"
				}
				findings = append(findings, f)
			}
		} else if typ == "index" {
			issueKey := "indexscan:" + table
			if table != "" && !seen[issueKey] {
				seen[issueKey] = true
				findings = append(findings, DiagnosisFinding{
					Severity: "medium",
					Issue:    "Full index scan",
					Table:    table,
					Detail:   fmt.Sprintf("type=index key=%s rows=%s", nullDash(key), nullDash(rowsEst)),
					Hint:     "May still read many rows; a more selective index or tighter WHERE can help",
				})
			}
		}

		if mysqlUsingTmpRe.MatchString(extra) {
			findings = append(findings, DiagnosisFinding{
				Severity: "medium",
				Issue:    "Using temporary",
				Table:    table,
				Detail:   TruncateQuery(extra, 100),
				Hint:     "GROUP BY / DISTINCT may spill to a temp table; covering indexes can avoid it",
			})
		}
		if mysqlFilesortRe.MatchString(extra) {
			findings = append(findings, DiagnosisFinding{
				Severity: "medium",
				Issue:    "Using filesort",
				Table:    table,
				Detail:   TruncateQuery(extra, 100),
				Hint:     "An index matching ORDER BY can avoid filesort",
			})
		}
	}
	return dedupeFindings(findings)
}

func diagnoseSQLite(plan Result) []DiagnosisFinding {
	var findings []DiagnosisFinding
	seen := map[string]bool{}
	for _, line := range planLines(plan) {
		// Prefer the detail column when present (EXPLAIN QUERY PLAN).
		detail := line
		if m := sqliteScanRe.FindStringSubmatch(detail); len(m) == 2 {
			table := strings.Trim(m[1], `"'`)
			if strings.Contains(strings.ToUpper(detail), "USING INDEX") || strings.Contains(strings.ToUpper(detail), "USING COVERING INDEX") {
				continue
			}
			if table == "" || seen[table] {
				continue
			}
			seen[table] = true
			findings = append(findings, DiagnosisFinding{
				Severity: "high",
				Issue:    "Table scan",
				Table:    table,
				Detail:   TruncateQuery(detail, 100),
				Hint:     "Create an index for the filter/join columns used against this table",
			})
		}
		_ = sqliteSearchRe // documented for contrast; SEARCH USING INDEX is healthy
	}
	return dedupeFindings(findings)
}

func annotateIndexes(findings []DiagnosisFinding, indexes IndexLookup) {
	cache := map[string][]Index{}
	for i := range findings {
		t := findings[i].Table
		if t == "" || findings[i].Severity == "info" {
			continue
		}
		idxs, ok := cache[t]
		if !ok {
			var err error
			idxs, err = indexes(t)
			if err != nil {
				cache[t] = nil
				continue
			}
			cache[t] = idxs
		}
		if len(idxs) == 0 {
			if findings[i].Hint == "" {
				findings[i].Hint = "No secondary indexes on " + t
			} else if !strings.Contains(findings[i].Hint, "No secondary") {
				findings[i].Hint += " · no secondary indexes on " + t
			}
			continue
		}
		summary := formatIndexSummary(idxs)
		if findings[i].Hint != "" && !strings.Contains(findings[i].Hint, "Existing indexes") {
			findings[i].Hint += " · existing: " + summary
		} else if findings[i].Hint == "" {
			findings[i].Hint = "Existing indexes: " + summary
		}
	}
}

func formatIndexSummary(idxs []Index) string {
	parts := make([]string, 0, len(idxs))
	for _, idx := range idxs {
		if len(parts) >= 4 {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", idx.Name, strings.Join(idx.Columns, ",")))
	}
	return strings.Join(parts, ", ")
}

func planLines(plan Result) []string {
	if len(plan.Columns) == 1 {
		out := make([]string, 0, len(plan.Rows))
		for _, row := range plan.Rows {
			if len(row) > 0 {
				out = append(out, row[0])
			}
		}
		return out
	}
	// SQLite EXPLAIN QUERY PLAN: id | parent | notused | detail
	detailIdx := -1
	for i, c := range plan.Columns {
		if strings.EqualFold(c.Name, "detail") {
			detailIdx = i
			break
		}
	}
	if detailIdx >= 0 {
		out := make([]string, 0, len(plan.Rows))
		for _, row := range plan.Rows {
			if detailIdx < len(row) {
				out = append(out, row[detailIdx])
			}
		}
		return out
	}
	out := make([]string, 0, len(plan.Rows))
	for _, row := range plan.Rows {
		out = append(out, strings.Join(row, " | "))
	}
	return out
}

func mysqlExplainCols(cols []Column) map[string]int {
	out := make(map[string]int, len(cols))
	for i, c := range cols {
		out[strings.ToLower(c.Name)] = i
	}
	return out
}

func stripPGIdent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func pgFilterColumns(line string) []string {
	var cols []string
	for _, m := range pgFilterColRe.FindAllStringSubmatch(line, -1) {
		if len(m) >= 2 {
			cols = append(cols, m[1])
		}
	}
	return uniqueStrings(cols)
}

func nullDash(s string) string {
	if s == "" || strings.EqualFold(s, "null") {
		return "—"
	}
	return s
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func dedupeFindings(in []DiagnosisFinding) []DiagnosisFinding {
	seen := map[string]bool{}
	var out []DiagnosisFinding
	for _, f := range in {
		key := f.Severity + "\x00" + f.Issue + "\x00" + f.Table + "\x00" + f.Detail
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

func sortFindings(findings []DiagnosisFinding) {
	rank := func(s string) int {
		switch strings.ToLower(s) {
		case "high":
			return 0
		case "medium":
			return 1
		default:
			return 2
		}
	}
	for i := 0; i < len(findings); i++ {
		for j := i + 1; j < len(findings); j++ {
			if rank(findings[j].Severity) < rank(findings[i].Severity) {
				findings[i], findings[j] = findings[j], findings[i]
			}
		}
	}
}
