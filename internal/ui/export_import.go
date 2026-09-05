package ui

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rsiota/creel/internal/db"
)

// exportToCSV writes the current page to a CSV file in ~/Downloads. It is the
// fast path for the `x` key: instant, all columns, current page only (never a
// re-query). The configured dialog is exportResults via `g X`, which adds
// format, column, and whole-table scope options.
func (m *Model) exportToCSV() tea.Cmd {
	return m.exportResults(fmtCSV, nil, scopePage)
}

// exportResults writes rows to ~/Downloads in the given format.
//
// cols selects the column projection: nil means all columns (no projection,
// preserving column order); a non-empty list projects to those columns in the
// given order (names not in the result set are dropped). scope selects the row
// source:
//
//   - scopePage:   the rows currently in memory (the visible page).
//   - scopeAll:    re-query the source table with no LIMIT (whole table), or
//     re-run the user's query for custom (non-table) result sets.
//   - scopeMarked: re-query the marked rows by primary key (may span pages).
//
// The re-query paths run asynchronously and deliver exportDoneMsg; the
// in-memory path sets exportMsg directly and returns nil.
//
// The requested scope is normalized against what is actually available (e.g.
// scopeMarked with no marks falls back to scopePage), so callers can pass a
// default scope without checking preconditions themselves.
func (m *Model) exportResults(format exportFormat, cols []string, scope exportScope) tea.Cmd {
	if m.results.NumRows() == 0 {
		m.exportMsg = "nothing to export"
		return nil
	}
	if m.connection == nil {
		m.exportMsg = "not connected"
		return nil
	}

	allCols := m.results.columns
	table := m.results.SourceTable()
	selIdx, selNames := resolveExportColumns(allCols, cols)
	if len(selIdx) == 0 {
		m.exportMsg = "no matching columns to export"
		return nil
	}

	hasMarks := m.results.IsEditable() && m.results.MarkCount() > 0
	if scope == scopeMarked && !hasMarks {
		scope = scopePage
	}
	if scope == scopeAll && table == "" && strings.TrimSpace(m.lastQuery) == "" {
		scope = scopePage
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := exportFilename(table, timestamp, format)
	driver := m.connection.Config().Driver

	// Marked rows: SELECT <cols> FROM t WHERE pk IN (...).
	if scope == scopeMarked {
		tuples := m.results.MarkedPKs()
		pkNames := m.results.PKColumns()
		pkTypes := m.results.PKTypes()
		clause := buildPKInClause(pkNames, pkTypes, tuples)
		query := fmt.Sprintf("SELECT %s FROM %s WHERE %s",
			buildSelectClause(driver, selNames, allCols), table, clause)
		conn := m.connection
		return func() tea.Msg {
			result, err := conn.DB().Execute(query)
			if err != nil {
				return exportDoneMsg{err: err}
			}
			path, count, err := writeExport(format, columnNamesFromResult(result), result.Rows, filename)
			return exportDoneMsg{path: path, count: count, err: err}
		}
	}

	// Whole table: SELECT <cols> FROM t (no LIMIT) — not capped at page size.
	if scope == scopeAll && table != "" {
		query := fmt.Sprintf("SELECT %s FROM %s",
			buildSelectClause(driver, selNames, allCols), table)
		conn := m.connection
		return func() tea.Msg {
			result, err := conn.DB().Execute(query)
			if err != nil {
				return exportDoneMsg{err: err}
			}
			path, count, err := writeExport(format, columnNamesFromResult(result), result.Rows, filename)
			return exportDoneMsg{path: path, count: count, err: err}
		}
	}

	// Whole result (custom query, no backing table): re-run the user's query
	// and project columns in Go. Running it directly (no subquery wrap) avoids
	// the JOIN/derived-table caveats that paging relies on.
	if scope == scopeAll {
		query := strings.TrimRight(m.lastQuery, ";")
		conn := m.connection
		return func() tea.Msg {
			result, err := conn.DB().Execute(query)
			if err != nil {
				return exportDoneMsg{err: err}
			}
			pcols, prows := projectResult(result, selIdx)
			path, count, err := writeExport(format, pcols, prows, filename)
			return exportDoneMsg{path: path, count: count, err: err}
		}
	}

	// Current page: project the in-memory rows.
	prows := projectRows(m.results.rows, selIdx)
	path, count, err := writeExport(format, selNames, prows, filename)
	m.exportMsg = exportStatusMessage(path, count, err)
	return nil
}

// defaultExportScope picks the most useful scope when one is not specified
// explicitly (the `x` key pins scopePage; the overlay and :export use this).
// Marked rows win, then whole table, then the current page.
func (m *Model) defaultExportScope() exportScope {
	if m.results.IsEditable() && m.results.MarkCount() > 0 {
		return scopeMarked
	}
	if m.results.SourceTable() != "" {
		return scopeAll
	}
	return scopePage
}

// resolveExportColumns maps requested column names to indices in the result
// set. A nil/empty requested list means all columns (in result order). A
// non-empty list is resolved in request order so `:export csv email,id` yields
// those columns in that order; unknown names are dropped. The returned index
// slice aligns 1:1 with names.
func resolveExportColumns(allCols, requested []string) (idx []int, names []string) {
	if len(requested) == 0 {
		idx = make([]int, len(allCols))
		names = allCols
		for i := range allCols {
			idx[i] = i
		}
		return
	}
	lower := make(map[string]int, len(allCols))
	for i, c := range allCols {
		lower[strings.ToLower(c)] = i
	}
	for _, r := range requested {
		if i, ok := lower[strings.ToLower(r)]; ok {
			idx = append(idx, i)
			names = append(names, allCols[i])
		}
	}
	return
}

// buildSelectClause renders the SELECT column list for a re-query. When the
// selected columns are exactly the full set in natural order it returns "*"
// (matching the previous behaviour and the DB's natural ordering); otherwise
// it lists the columns, driver-quoted, preserving the requested order.
func buildSelectClause(driver db.Driver, selNames, allCols []string) string {
	if stringSliceEqual(selNames, allCols) {
		return "*"
	}
	parts := make([]string, len(selNames))
	for i, n := range selNames {
		parts[i] = quoteIdentD(driver, n)
	}
	return strings.Join(parts, ", ")
}

// stringSliceEqual reports whether two string slices are element-wise equal.
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func columnNamesFromResult(r db.Result) []string {
	out := make([]string, len(r.Columns))
	for i, c := range r.Columns {
		out[i] = c.Name
	}
	return out
}

// projectResult projects a re-queried result set to the selected column
// indices (used for the custom-query whole-result path).
func projectResult(r db.Result, idx []int) ([]string, [][]string) {
	cols := make([]string, len(idx))
	for j, i := range idx {
		cols[j] = r.Columns[i].Name
	}
	rows := make([][]string, len(r.Rows))
	for ri, row := range r.Rows {
		nr := make([]string, len(idx))
		for j, i := range idx {
			if i < len(row) {
				nr[j] = row[i]
			}
		}
		rows[ri] = nr
	}
	return cols, rows
}

// projectRows projects in-memory rows to the selected column indices (used for
// the current-page path).
func projectRows(rows [][]string, idx []int) [][]string {
	out := make([][]string, len(rows))
	for ri, row := range rows {
		nr := make([]string, len(idx))
		for j, i := range idx {
			if i < len(row) {
				nr[j] = row[i]
			}
		}
		out[ri] = nr
	}
	return out
}

// exportFilename builds a safe filename for an export in the given format.
func exportFilename(table, timestamp string, format exportFormat) string {
	name := table
	if name == "" {
		name = "query"
	}
	ext := exportFormatExt[format]
	return fmt.Sprintf("creel_%s_%s.%s", name, timestamp, ext)
}

// exportStatusMessage renders the result of an export for the status bar.
func exportStatusMessage(path string, count int, err error) string {
	if err != nil {
		return fmt.Sprintf("export failed: %v", err)
	}
	return fmt.Sprintf("exported %d rows → %s", count, path)
}

// userDownloadsDir resolves ~/Downloads, creating it if needed. Tests replace
// it so export/backup helpers never write into the real Downloads folder.
var userDownloadsDir = defaultUserDownloadsDir

func defaultUserDownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// SwapUserDownloadsDir replaces the Downloads directory resolver; the returned
// func restores the previous resolver.
func SwapUserDownloadsDir(fn func() (string, error)) func() {
	prev := userDownloadsDir
	userDownloadsDir = fn
	return func() { userDownloadsDir = prev }
}

// writeCSV serializes columns and rows to a CSV file in ~/Downloads and
// returns the absolute path and row count.
func writeCSV(cols []string, rows [][]string, filename string) (string, int, error) {
	dir, err := userDownloadsDir()
	if err != nil {
		return "", 0, err
	}
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(cols); err != nil {
		return path, 0, err
	}
	for _, row := range rows {
		// Normalize NULL display values to empty fields in CSV.
		out := make([]string, len(row))
		for i, v := range row {
			if v == "NULL" {
				out[i] = ""
			} else {
				out[i] = v
			}
		}
		if err := w.Write(out); err != nil {
			return path, 0, err
		}
	}
	w.Flush()
	return path, len(rows), w.Error()
}

// writeExport serializes columns and rows in the given format to a file in
// ~/Downloads and returns the absolute path and row count. It dispatches to a
// per-format writer; CSV reuses writeCSV to keep the existing NULL → empty
// behaviour identical.
func writeExport(format exportFormat, cols []string, rows [][]string, filename string) (string, int, error) {
	if format == fmtCSV {
		return writeCSV(cols, rows, filename)
	}
	content, err := serializeFormat(format, cols, rows)
	if err != nil {
		return "", 0, err
	}
	return writeFile(filename, content, len(rows))
}

// writeFile writes content to ~/Downloads/<filename> and returns the absolute
// path and row count.
func writeFile(filename, content string, count int) (string, int, error) {
	dir, err := userDownloadsDir()
	if err != nil {
		return "", 0, err
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return path, 0, err
	}
	return path, count, nil
}

// serializeFormat renders columns and rows in the given format to a string, for
// writing to a file (writeExport) or for testing.
func serializeFormat(format exportFormat, cols []string, rows [][]string) (string, error) {
	switch format {
	case fmtJSON:
		return serializeJSON(cols, rows), nil
	case fmtJSONL:
		return serializeJSONL(cols, rows), nil
	case fmtMarkdown:
		return serializeMarkdown(cols, rows), nil
	case fmtTSV:
		return serializeTSV(cols, rows), nil
	case fmtCSV:
		return serializeCSV(cols, rows)
	}
	return "", fmt.Errorf("unsupported export format: %s", format)
}

// cellValue returns the value to emit for a tabular (CSV/TSV/Markdown) export:
// the NULL sentinel becomes an empty cell, matching the existing CSV behaviour.
func cellValue(v string) string {
	if v == "NULL" {
		return ""
	}
	return v
}

// serializeCSV renders columns and rows as CSV to a string, for testing and for
// the in-memory export path. NULL cells become empty fields.
func serializeCSV(cols []string, rows [][]string) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(cols); err != nil {
		return "", err
	}
	for _, row := range rows {
		out := make([]string, len(row))
		for i, v := range row {
			out[i] = cellValue(v)
		}
		if err := w.Write(out); err != nil {
			return "", err
		}
	}
	w.Flush()
	return buf.String(), nil
}

// serializeJSON renders the result set as a JSON array of objects keyed by
// column name. NULL cells become JSON null; all other values are strings
// (result rows are already stringly-typed).
func serializeJSON(cols []string, rows [][]string) string {
	out := make([]map[string]interface{}, len(rows))
	for r, row := range rows {
		obj := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			if i < len(row) && row[i] != "NULL" {
				obj[col] = row[i]
			} else {
				obj[col] = nil
			}
		}
		out[r] = obj
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b) + "\n"
}

// serializeJSONL renders the result set as one JSON object per line (JSON
// Lines), convenient for streaming/log-style consumption.
func serializeJSONL(cols []string, rows [][]string) string {
	var b strings.Builder
	for _, row := range rows {
		obj := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			if i < len(row) && row[i] != "NULL" {
				obj[col] = row[i]
			} else {
				obj[col] = nil
			}
		}
		line, _ := json.Marshal(obj)
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// serializeMarkdown renders the result set as a GitHub-flavoured Markdown table.
// Pipes in values are escaped so they don't break the table layout.
func serializeMarkdown(cols []string, rows [][]string) string {
	var b strings.Builder
	escape := func(v string) string { return strings.ReplaceAll(cellValue(v), "|", "\\|") }

	writeRow := func(cells []string) {
		b.WriteString("|")
		for _, c := range cells {
			b.WriteString(" " + escape(c) + " |")
		}
		b.WriteString("\n")
	}

	writeRow(cols)
	b.WriteString("|")
	for range cols {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, row := range rows {
		writeRow(row)
	}
	return b.String()
}

// serializeTSV renders the result set as tab-separated values.
func serializeTSV(cols []string, rows [][]string) string {
	var b strings.Builder
	writeTSVRow := func(cells []string) {
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = cellValue(c)
		}
		b.WriteString(strings.Join(parts, "\t"))
		b.WriteString("\n")
	}
	writeTSVRow(cols)
	for _, row := range rows {
		writeTSVRow(row)
	}
	return b.String()
}

// execExportDump starts a table-by-table SQL dump of the selected tables,
// writing to ~/Downloads with a timestamped filename. It writes the header and
// first table, then chains per-table commands via exportProgressMsg so the
// status bar can show live progress.
func (m *Model) execExportDump(tables []string) tea.Cmd {
	if m.connection == nil || len(tables) == 0 {
		return nil
	}
	conn := m.connection
	driver := conn.Config().Driver
	realDBName := conn.Config().Database
	fileLabel := filepath.Base(realDBName)
	if fileLabel == "" {
		fileLabel = "database"
	}
	timestamp := time.Now().Format("2006-01-02")
	ext := string(m.exportPicker.CurrentFormat())
	filename := fmt.Sprintf("%s_%s.%s", fileLabel, timestamp, ext)
	total := len(tables)
	database := conn.DB()

	return func() tea.Msg {
		dir, err := userDownloadsDir()
		if err != nil {
			return exportProgressMsg{err: err, total: total}
		}
		path := filepath.Join(dir, filename)
		f, err := os.Create(path)
		if err != nil {
			return exportProgressMsg{err: err, total: total}
		}
		bw := bufio.NewWriter(f)
		if err := db.DumpHeader(bw, driver, realDBName, total); err != nil {
			f.Close()
			return exportProgressMsg{err: err, path: path, total: total}
		}
		if err := db.DumpTable(bw, database, driver, tables[0]); err != nil {
			f.Close()
			return exportProgressMsg{err: err, path: path, total: total}
		}
		bw.Flush()
		return exportProgressMsg{
			file:   f,
			bw:     bw,
			path:   path,
			index:  0,
			total:  total,
			tables: tables,
			name:   tables[0],
		}
	}
}

// exBackup shells out to mysqldump (MySQL) or pg_dump (PostgreSQL) for the
// current database, writing ~/Downloads/<db>_YYYY-MM-DD.sql. Password goes in
// a 0600 defaults/.pgpass file, never argv. When the DB is on the SSH host,
// the dump tool runs remotely and streams back over SSH. Live byte counts
// update the status bar while the dump runs.
func (m *Model) exBackup() tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	cfg := m.connection.Config()
	switch cfg.Driver {
	case db.DriverMySQL:
		if err := db.MysqlDumpGuard(cfg); err != nil {
			m.schemaMsg = err.Error()
			return nil
		}
	case db.DriverPostgres:
		if err := db.PgDumpGuard(cfg); err != nil {
			m.schemaMsg = err.Error()
			return nil
		}
	default:
		m.schemaMsg = ":backup uses mysqldump or pg_dump; use X for a SQL dump"
		return nil
	}
	// Local CLI is only required when we cannot run it on the SSH host.
	bin := ""
	needLocal := strings.TrimSpace(cfg.SSHHost) == "" || !db.MysqlHostOnSSHTarget(cfg.Host)
	if needLocal {
		var err error
		switch cfg.Driver {
		case db.DriverMySQL:
			bin, err = db.FindMysqlDump()
			if err != nil {
				m.schemaMsg = "mysqldump is not on PATH"
				return nil
			}
		case db.DriverPostgres:
			bin, err = db.FindPgDump()
			if err != nil {
				m.schemaMsg = "pg_dump is not on PATH"
				return nil
			}
		}
	}
	fileLabel := filepath.Base(cfg.Database)
	if fileLabel == "" {
		fileLabel = "database"
	}
	filename := fmt.Sprintf("%s_%s.sql", fileLabel, time.Now().Format("2006-01-02"))
	conn := m.connection
	driver := cfg.Driver
	m.backupStarted = time.Now()
	m.exportMsg = "Backing up…"

	progress := make(chan backupProgressMsg, 1)
	done := make(chan backupDoneMsg, 1)
	go func() {
		dir, err := userDownloadsDir()
		if err != nil {
			done <- backupDoneMsg{err: err}
			return
		}
		path := filepath.Join(dir, filename)
		var last int64
		onBytes := func(n int64) {
			last = n
			select {
			case progress <- backupProgressMsg{bytes: n, path: path}:
			default: // drop if the UI hasn't consumed the previous tick yet
			}
		}
		switch driver {
		case db.DriverMySQL:
			err = db.RunMysqlDump(bin, cfg, path, conn, onBytes)
		case db.DriverPostgres:
			err = db.RunPgDump(bin, cfg, path, conn, onBytes)
		}
		done <- backupDoneMsg{path: path, bytes: last, err: err}
	}()
	return waitForBackupProgress(progress, done)
}

func waitForBackupProgress(progress <-chan backupProgressMsg, done <-chan backupDoneMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-done:
			return msg
		case msg := <-progress:
			return backupProgressWrapper{msg: msg, progress: progress, done: done}
		}
	}
}

// backupProgressStatus formats the live status-bar line during :backup.
func backupProgressStatus(n int64, started time.Time) string {
	size := db.FormatDumpSize(n)
	if started.IsZero() {
		return fmt.Sprintf("Backing up… %s", size)
	}
	elapsed := time.Since(started).Seconds()
	if elapsed < 0.25 || n <= 0 {
		return fmt.Sprintf("Backing up… %s", size)
	}
	bps := float64(n) / elapsed
	return fmt.Sprintf("Backing up… %s · %s/s", size, db.FormatDumpSize(int64(bps)))
}

// exRestore shells out to the mysql client (MySQL) or psql (PostgreSQL) to
// load a SQL dump into the current database. Password goes in a 0600
// defaults/.pgpass file, never argv. When the DB is on the SSH host, the
// client runs remotely and the dump streams over SSH stdin. Live byte counts
// update the status bar while the restore runs.
func (m *Model) exRestore(path string) tea.Cmd {
	if m.connection == nil {
		m.schemaMsg = "not connected"
		return nil
	}
	if m.isReadOnly() {
		m.schemaMsg = "read-only: restore disabled"
		return nil
	}
	cfg := m.connection.Config()
	switch cfg.Driver {
	case db.DriverMySQL:
		if err := db.MysqlRestoreGuard(cfg); err != nil {
			m.schemaMsg = err.Error()
			return nil
		}
	case db.DriverPostgres:
		if err := db.PgRestoreGuard(cfg); err != nil {
			m.schemaMsg = err.Error()
			return nil
		}
	default:
		m.schemaMsg = ":restore uses mysql or psql; use I to import"
		return nil
	}
	expanded, err := expandTilde(filepath.Clean(path))
	if err != nil {
		m.schemaMsg = err.Error()
		return nil
	}
	if st, err := os.Stat(expanded); err != nil {
		m.schemaMsg = err.Error()
		return nil
	} else if st.IsDir() {
		m.schemaMsg = "path is a directory"
		return nil
	}
	// Local client is only required when we cannot run it on the SSH host.
	bin := ""
	needLocal := strings.TrimSpace(cfg.SSHHost) == "" || !db.MysqlHostOnSSHTarget(cfg.Host)
	if needLocal {
		var err error
		switch cfg.Driver {
		case db.DriverMySQL:
			bin, err = db.FindMysql()
			if err != nil {
				m.schemaMsg = "mysql is not on PATH"
				return nil
			}
		case db.DriverPostgres:
			bin, err = db.FindPsql()
			if err != nil {
				m.schemaMsg = "psql is not on PATH"
				return nil
			}
		}
	}
	conn := m.connection
	driver := cfg.Driver
	m.restoreStarted = time.Now()
	m.exportMsg = "Restoring…"

	progress := make(chan restoreProgressMsg, 1)
	done := make(chan restoreDoneMsg, 1)
	go func() {
		var last int64
		onBytes := func(n int64) {
			last = n
			select {
			case progress <- restoreProgressMsg{bytes: n, path: expanded}:
			default:
			}
		}
		var runErr error
		switch driver {
		case db.DriverMySQL:
			runErr = db.RunMysqlRestore(bin, cfg, expanded, conn, onBytes)
		case db.DriverPostgres:
			runErr = db.RunPgRestore(bin, cfg, expanded, conn, onBytes)
		}
		done <- restoreDoneMsg{path: expanded, bytes: last, err: runErr}
	}()
	return waitForRestoreProgress(progress, done)
}

func waitForRestoreProgress(progress <-chan restoreProgressMsg, done <-chan restoreDoneMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-done:
			return msg
		case msg := <-progress:
			return restoreProgressWrapper{msg: msg, progress: progress, done: done}
		}
	}
}

// restoreProgressStatus formats the live status-bar line during :restore.
func restoreProgressStatus(n int64, started time.Time) string {
	size := db.FormatDumpSize(n)
	if started.IsZero() {
		return fmt.Sprintf("Restoring… %s", size)
	}
	elapsed := time.Since(started).Seconds()
	if elapsed < 0.25 || n <= 0 {
		return fmt.Sprintf("Restoring… %s", size)
	}
	bps := float64(n) / elapsed
	return fmt.Sprintf("Restoring… %s · %s/s", size, db.FormatDumpSize(int64(bps)))
}

// dumpTableCmd returns a command that writes a single table to an open dump
// file and reports progress via exportProgressMsg.
func dumpTableCmd(f *os.File, bw *bufio.Writer, database db.DB, driver db.Driver, table string, index, total int, tables []string, path string) tea.Cmd {
	return func() tea.Msg {
		if err := db.DumpTable(bw, database, driver, table); err != nil {
			f.Close()
			return exportProgressMsg{err: err, path: path, index: index, total: total, tables: tables, name: table}
		}
		bw.Flush()
		return exportProgressMsg{
			file:   f,
			bw:     bw,
			path:   path,
			index:  index,
			total:  total,
			tables: tables,
			name:   table,
		}
	}
}

// dumpFooterCmd returns a command that writes the dump footer, flushes, and
// closes the file, reporting completion via exportDumpMsg.
func dumpFooterCmd(f *os.File, bw *bufio.Writer, driver db.Driver, total int, path string) tea.Cmd {
	return func() tea.Msg {
		if err := db.DumpFooter(bw, driver); err != nil {
			f.Close()
			return exportDumpMsg{path: path, err: err}
		}
		bw.Flush()
		f.Close()
		return exportDumpMsg{path: path, tables: total}
	}
}

// execImportSQL runs an async SQL import from the given file path, reporting
// execImportSQL starts an async SQL import from the given file path. It runs
// ImportSQL in a goroutine that streams byte-progress over a channel; a polling
// command turns each update into an importProgressMsg for the status bar, and
// the final result is delivered as importDoneMsg.
func (m *Model) execImportSQL(rawPath string) tea.Cmd {
	if m.connection == nil {
		return nil
	}
	database := m.connection.DB()
	filename := filepath.Base(rawPath)

	// progress is a buffered channel: ImportSQL writes progress updates, and
	// waitForImportProgress reads them. A buffer of 1 lets the first update
	// land without blocking if the receiver hasn't been scheduled yet.
	progress := make(chan importProgressMsg, 1)
	done := make(chan importDoneMsg, 1)

	// Run the import in a goroutine so the TUI stays responsive.
	go func() {
		f, err := os.Open(rawPath)
		if err != nil {
			done <- importDoneMsg{filename: filename, err: err}
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			done <- importDoneMsg{filename: filename, err: err}
			return
		}
		totalSize := stat.Size()

		result, err := db.ImportSQL(f, database, totalSize, func(read int64, total int64) {
			progress <- importProgressMsg{filename: filename, read: read, total: total}
		})
		if err != nil {
			done <- importDoneMsg{filename: filename, err: err}
			return
		}
		done <- importDoneMsg{result: result, filename: filename}
	}()

	// Start polling for progress / completion.
	return waitForImportProgress(progress, done)
}

// waitForImportProgress is a tea.Cmd that blocks until either a progress update
// or the final result arrives from the import goroutine. It returns the
// appropriate message and, for progress updates, re-issues itself to continue
// polling.
func waitForImportProgress(progress <-chan importProgressMsg, done <-chan importDoneMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-done:
			return msg
		case msg := <-progress:
			return importProgressWrapper{msg: msg, progress: progress, done: done}
		}
	}
}

// importProgressWrapper carries a progress update together with the channels
// needed to re-issue the polling command.
type importProgressWrapper struct {
	msg      importProgressMsg
	progress <-chan importProgressMsg
	done     <-chan importDoneMsg
}
