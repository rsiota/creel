package db

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// Format identifies an export format for table data.
type Format string

const (
	FormatSQL  Format = "sql"
	FormatCSV  Format = "csv"  // reserved for future use
	FormatJSON Format = "json" // reserved for future use
)

// insertBatchSize is the maximum number of rows per multi-value INSERT
// statement in a SQL dump. Keeping batches bounded avoids enormous single
// statements on large tables.
const insertBatchSize = 100

// DumpTables writes the schema and data of the given tables to w in the
// specified format. Tables are emitted in the given order.
//
// For FormatSQL the output is a logical SQL dump in the source driver's own
// dialect (SQLite syntax for SQLite sources, MySQL syntax for MySQL sources).
// Each table is preceded by DROP TABLE IF EXISTS, followed by CREATE TABLE
// (native DDL when the driver exposes it), then batched INSERT statements.
//
// For incremental / streaming use, callers may instead invoke DumpHeader,
// DumpTable, and DumpFooter directly.
func DumpTables(w io.Writer, database DB, driver Driver, dbName string, tables []string, format Format) error {
	switch format {
	case FormatSQL:
		return dumpSQL(w, database, driver, dbName, tables)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

func dumpSQL(w io.Writer, database DB, driver Driver, dbName string, tables []string) error {
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	if err := DumpHeader(bw, driver, dbName, len(tables)); err != nil {
		return err
	}
	for _, table := range tables {
		if err := DumpTable(bw, database, driver, table); err != nil {
			return fmt.Errorf("dump table %s: %w", table, err)
		}
	}
	return DumpFooter(bw, driver)
}

// DumpHeader writes the dump preamble: comments and (MySQL) session-variable
// setup comments. It is the first stage of an incremental dump.
func DumpHeader(w io.Writer, driver Driver, dbName string, tableCount int) error {
	fmt.Fprintf(w, "-- creel SQL dump\n")
	fmt.Fprintf(w, "-- driver: %s\n", driver)
	fmt.Fprintf(w, "-- database: %s\n", dbName)
	fmt.Fprintf(w, "-- tables: %d\n", tableCount)
	fmt.Fprintf(w, "\n")

	// MySQL dumps use version-gated session setup comments (the same header
	// block emitted by mysqldump) so the dump restores cleanly regardless of
	// the target server's defaults. SQLite has no analogous session state.
	if driver == DriverMySQL {
		fmt.Fprintln(w, "/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;")
		fmt.Fprintln(w, "/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;")
		fmt.Fprintln(w, "/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;")
		fmt.Fprintln(w, "SET NAMES utf8mb4;")
		fmt.Fprintln(w, "/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;")
		fmt.Fprintln(w, "/*!40101 SET @OLD_SQL_MODE='NO_AUTO_VALUE_ON_ZERO', SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;")
		fmt.Fprintln(w, "/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;")
		fmt.Fprintln(w)
	}
	return nil
}

// DumpFooter writes the dump epilogue: (MySQL) session-variable restore. It is
// the last stage of an incremental dump.
func DumpFooter(w io.Writer, driver Driver) error {
	if driver == DriverMySQL {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;")
		fmt.Fprintln(w, "/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;")
		fmt.Fprintln(w, "/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;")
		fmt.Fprintln(w, "/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;")
		fmt.Fprintln(w, "/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;")
		fmt.Fprintln(w, "/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;")
	}
	return nil
}

// DumpTable writes a single table's DROP, CREATE, and INSERT statements.
func DumpTable(w io.Writer, database DB, driver Driver, table string) error {
	fmt.Fprintf(w, "--\n-- Table: %s\n--\n", table)
	fmt.Fprintf(w, "DROP TABLE IF EXISTS %s;\n", quoteIdent(driver, table))

	createSQL, err := tableCreateSQL(database, driver, table)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s;\n\n", createSQL)

	result, err := database.Execute("SELECT * FROM " + quoteIdent(driver, table))
	if err != nil {
		return err
	}
	if len(result.Rows) == 0 {
		fmt.Fprintln(w)
		return nil
	}

	colTypes := make([]string, len(result.Columns))
	colNames := make([]string, len(result.Columns))
	for i, c := range result.Columns {
		colTypes[i] = c.Type
		colNames[i] = quoteIdent(driver, c.Name)
	}
	colList := strings.Join(colNames, ", ")

	// Lock the table for writing during the inserts, then release.
	tableIdent := quoteIdent(driver, table)
	if driver == DriverMySQL {
		fmt.Fprintf(w, "LOCK TABLES %s WRITE;\n", tableIdent)
		fmt.Fprintf(w, "/*!40000 ALTER TABLE %s DISABLE KEYS */;\n", tableIdent)
	}
	for i, row := range result.Rows {
		if i%insertBatchSize == 0 {
			if i > 0 {
				fmt.Fprintln(w, ";")
			}
			fmt.Fprintf(w, "INSERT INTO %s (%s) VALUES\n", tableIdent, colList)
		} else {
			fmt.Fprintln(w, ",")
		}
		values := make([]string, len(row))
		for j, val := range row {
			if data, ok := result.Blobs[BlobKey{Row: i, Col: j}]; ok {
				values[j] = BlobSQLLiteral(data, colTypes[j])
			} else {
				values[j] = formatSQLValue(val, colTypes[j], driver)
			}
		}
		fmt.Fprintf(w, "  (%s)", strings.Join(values, ", "))
	}
	fmt.Fprintln(w, ";")
	if driver == DriverMySQL {
		fmt.Fprintf(w, "/*!40000 ALTER TABLE %s ENABLE KEYS */;\n", tableIdent)
		fmt.Fprintln(w, "UNLOCK TABLES;")
	}
	fmt.Fprintln(w)
	return nil
}

// tableCreateSQL prefers the driver's native CREATE TABLE (MySQL SHOW CREATE
// TABLE, SQLite sqlite_master) so indexes, named FKs, ON DELETE/UPDATE, CHECKs,
// and engine/charset options survive a dump. When the driver has no native
// definition, the statement is reconstructed from column and FK metadata.
func tableCreateSQL(database DB, driver Driver, table string) (string, error) {
	ddl, err := database.TableDefinition(table)
	if err != nil {
		return "", err
	}
	ddl = strings.TrimRight(strings.TrimSpace(ddl), ";")
	if ddl != "" {
		return ddl, nil
	}
	cols, err := database.TableColumnInfo(table)
	if err != nil {
		return "", err
	}
	fks, err := database.ForeignKeys(table)
	if err != nil {
		return "", err
	}
	return buildCreateTableFromInfo(driver, table, cols, fks), nil
}

// buildCreateTableFromInfo reconstructs a CREATE TABLE statement from column
// metadata. Single-column primary keys are emitted inline; composite primary
// keys use a table-level constraint. Foreign keys are appended as table-level
// constraints. Used when TableDefinition is empty (Postgres, or catalogs that
// store no original DDL).
func buildCreateTableFromInfo(driver Driver, table string, cols []TableColumnInfo, fks []ForeignKey) string {
	var pkCols []string
	for _, c := range cols {
		if c.PrimaryKey {
			pkCols = append(pkCols, c.Name)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s (", quoteIdent(driver, table))

	first := true
	for _, c := range cols {
		if !first {
			b.WriteString(",")
		}
		first = false
		fmt.Fprintf(&b, "\n    %s %s", quoteIdent(driver, c.Name), c.Type)

		if c.PrimaryKey && len(pkCols) == 1 {
			b.WriteString(" PRIMARY KEY")
			if c.AutoIncrement {
				if driver == DriverSQLite {
					b.WriteString(" AUTOINCREMENT")
				} else if driver == DriverMySQL {
					b.WriteString(" AUTO_INCREMENT")
				}
				// PostgreSQL: auto-increment comes from the sequence default; no keyword.
			}
		}
		if c.NotNull {
			b.WriteString(" NOT NULL")
		}
		if c.HasDefault {
			b.WriteString(" DEFAULT ")
			b.WriteString(formatDefault(c.DefaultValue))
		}
	}

	if len(pkCols) > 1 {
		quoted := make([]string, len(pkCols))
		for i, c := range pkCols {
			quoted[i] = quoteIdent(driver, c)
		}
		fmt.Fprintf(&b, ",\n    PRIMARY KEY (%s)", strings.Join(quoted, ", "))
	}

	for _, fk := range fks {
		fmt.Fprintf(&b, ",\n    FOREIGN KEY (%s) REFERENCES %s (%s)",
			quoteIdent(driver, fk.Column),
			quoteIdent(driver, fk.RefTable),
			quoteIdent(driver, fk.RefColumn),
		)
	}

	b.WriteString("\n)")
	return b.String()
}

// formatSQLValue renders a cell value as a SQL literal. The string "NULL" is
// treated as SQL NULL (consistent with how Execute represents null cells).
// Numeric column types are left unquoted; date/time types are normalized from
// the ISO-8601 format the driver emits (parseTime) to 'YYYY-MM-DD HH:MM:SS',
// which both MySQL and SQLite accept. SQLite/Postgres strings double embedded
// quotes; MySQL strings also backslash-escape \, ', ", and control chars
// (mysqldump / Sequel Ace style) so a value like App\Models\User round-trips.
func formatSQLValue(value, colType string, driver Driver) string {
	if value == "NULL" {
		return "NULL"
	}
	if isNumericType(colType) {
		return value
	}
	v := value
	if IsDateTimeType(colType) {
		v = FormatDateTimeLiteral(v)
	}
	if driver == DriverMySQL {
		return mysqlQuoteString(v)
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// mysqlQuoteString quotes v as a MySQL string literal, matching mysqldump:
// backslash, quote, newline, carriage return, tab, NUL, and Ctrl-Z are escaped.
// Without this, a PHP namespace like App\Models\User is imported as
// AppModelsUser, and a trailing backslash swallows the closing quote.
func mysqlQuoteString(v string) string {
	var b strings.Builder
	b.Grow(len(v) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case 0:
			b.WriteString(`\0`)
		case 0x1a:
			b.WriteString(`\Z`)
		default:
			b.WriteByte(v[i])
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// IsDateTimeType reports whether a column type stores a date, time, or
// timestamp. Values of these types may arrive as ISO-8601 strings (the driver
// formats parsed time.Time values that way when scanned into a string) and must
// be re-rendered as plain SQL datetime literals.
func IsDateTimeType(dbType string) bool {
	t := strings.ToLower(strings.TrimSpace(dbType))
	if i := strings.IndexByte(t, '('); i > 0 {
		t = t[:i]
	}
	switch t {
	case "datetime", "timestamp", "date", "time",
		"timestamp without time zone", "timestamp with time zone",
		"time without time zone", "time with time zone":
		return true
	}
	return false
}

// IsBooleanType reports whether a column type is an explicit boolean, including
// MySQL's BOOLEAN alias tinyint(1) / tinyint(1) unsigned. Plain TINYINT /
// INTEGER without a (1) width are not treated as boolean — those often store
// counts or enums; callers can still combine this with a column-name heuristic.
func IsBooleanType(dbType string) bool {
	t := strings.ToLower(strings.TrimSpace(dbType))
	if t == "" {
		return false
	}
	// MySQL information_schema.column_type: "tinyint(1)", "tinyint(1) unsigned".
	// Match before stripping parens so tinyint(4) stays non-boolean.
	if strings.HasPrefix(t, "tinyint") && strings.Contains(t, "(1)") {
		return true
	}
	if i := strings.IndexByte(t, '('); i > 0 {
		t = t[:i]
	}
	switch t {
	case "bool", "boolean":
		return true
	}
	return false
}

// FormatDateTimeLiteral converts an ISO-8601 timestamp (e.g.
// "2026-05-08T18:38:00Z") into a 'YYYY-MM-DD HH:MM:SS' literal accepted by both
// MySQL and SQLite. The wall-clock time is preserved exactly (no timezone
// conversion), so dumps round-trip. Values that don't look like ISO timestamps
// are returned unchanged.
func FormatDateTimeLiteral(value string) string {
	if !strings.Contains(value, "T") {
		return value
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02 15:04:05")
		}
	}
	// Mechanical fallback: 'T' → space, drop trailing 'Z'.
	v := strings.Replace(value, "T", " ", 1)
	v = strings.TrimSuffix(v, "Z")
	return v
}

// isNumericType reports whether a database column type should be treated as
// numeric (and therefore left unquoted in generated SQL). This mirrors the
// logic in the UI package but lives here so the dumper is self-contained.
func isNumericType(dbType string) bool {
	if dbType == "" {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(dbType))
	// Strip size specifiers while keeping a trailing UNSIGNED/SIGNED:
	// DECIMAL(10,2), BIGINT(20) UNSIGNED, BIGINT UNSIGNED.
	if i := strings.IndexByte(t, '('); i > 0 {
		if j := strings.IndexByte(t[i:], ')'); j >= 0 {
			t = t[:i] + t[i+j+1:]
		} else {
			t = t[:i]
		}
	}
	t = strings.TrimSpace(t)
	t = strings.TrimSuffix(t, " unsigned")
	t = strings.TrimSuffix(t, " signed")
	t = strings.TrimSpace(t)
	switch t {
	case "int", "integer", "tinyint", "smallint", "mediumint", "bigint",
		"unsigned", "int unsigned", "unsigned big int",
		"unsigned int", "unsigned bigint", "unsigned tinyint",
		"unsigned smallint", "unsigned mediumint",
		"real", "double", "float", "decimal", "numeric", "boolean", "bool",
		"double precision", "serial", "bigserial", "smallserial", "money":
		return true
	}
	return false
}
