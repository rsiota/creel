package ui

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/db"
)

// exportToCSV writes result rows to a CSV file in ~/Downloads. If rows are
// marked, it re-queries the full set of marked rows by primary key; otherwise
// it exports the current page. Returns a command that delivers exportDoneMsg.
func (m *Model) exportToCSV() tea.Cmd {
	if m.results.NumRows() == 0 {
		m.exportMsg = "nothing to export"
		return nil
	}

	cols := m.results.columns
	table := m.results.SourceTable()

	// Marked rows: re-query for complete data (may span multiple pages).
	if m.results.IsEditable() && m.results.MarkCount() > 0 {
		tuples := m.results.MarkedPKs()
		pkNames := m.results.PKColumns()
		pkTypes := m.results.PKTypes()
		clause := buildPKInClause(pkNames, pkTypes, tuples)

		conn := m.connection
		query := fmt.Sprintf("SELECT * FROM %s WHERE %s", table, clause)
		timestamp := time.Now().Format("20060102_150405")
		filename := exportFilename(table, timestamp)

		return func() tea.Msg {
			result, err := conn.DB().Execute(query)
			if err != nil {
				return exportDoneMsg{err: err}
			}
			exportCols := make([]string, len(result.Columns))
			for i, c := range result.Columns {
				exportCols[i] = c.Name
			}
			path, count, err := writeCSV(exportCols, result.Rows, filename)
			return exportDoneMsg{path: path, count: count, err: err}
		}
	}

	// No marks: export current page in memory.
	timestamp := time.Now().Format("20060102_150405")
	filename := exportFilename(table, timestamp)
	rows := m.results.rows
	path, count, err := writeCSV(cols, rows, filename)
	m.exportMsg = exportStatusMessage(path, count, err)
	return nil
}

// exportFilename builds a safe filename for a CSV export.
func exportFilename(table, timestamp string) string {
	name := table
	if name == "" {
		name = "query"
	}
	return fmt.Sprintf("gsql_%s_%s.csv", name, timestamp)
}

// exportStatusMessage renders the result of an export for the status bar.
func exportStatusMessage(path string, count int, err error) string {
	if err != nil {
		return fmt.Sprintf("export failed: %v", err)
	}
	return fmt.Sprintf("exported %d rows → %s", count, path)
}

// writeCSV serializes columns and rows to a CSV file in ~/Downloads and
// returns the absolute path and row count.
func writeCSV(cols []string, rows [][]string, filename string) (string, int, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", 0, err
	}
	dir = filepath.Join(dir, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

// serializeCSV renders columns and rows as CSV to a string, for testing.
func serializeCSV(cols []string, rows [][]string) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(cols); err != nil {
		return "", err
	}
	for _, row := range rows {
		out := make([]string, len(row))
		for i, v := range row {
			if v == "NULL" {
				out[i] = ""
			} else {
				out[i] = v
			}
		}
		if err := w.Write(out); err != nil {
			return "", err
		}
	}
	w.Flush()
	return buf.String(), nil
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
		dir, err := os.UserHomeDir()
		if err != nil {
			return exportProgressMsg{err: err, total: total}
		}
		dir = filepath.Join(dir, "Downloads")
		if err := os.MkdirAll(dir, 0o755); err != nil {
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
