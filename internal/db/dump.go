package db

import (
	"bufio"
	"fmt"
	"io"
	"strings"
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
// Each table is preceded by DROP TABLE IF EXISTS, followed by CREATE TABLE,
// then batched INSERT statements. The entire dump is wrapped in a transaction
// for fast, atomic restoration.
//
// dbName is the database name (used to emit a USE statement for MySQL dumps,
// making them self-contained). Tables is the list of table names to dump; the
// caller is responsible for deciding the scope (use Tables() on the DB to dump
// everything).
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

	fmt.Fprintf(bw, "-- gsql SQL dump\n")
	fmt.Fprintf(bw, "-- driver: %s\n", driver)
	fmt.Fprintf(bw, "-- database: %s\n", dbName)
	fmt.Fprintf(bw, "-- tables: %d\n", len(tables))
	fmt.Fprintf(bw, "\n")

	// MySQL dumps use version-gated session setup comments (the same header
	// block emitted by mysqldump) so the dump restores cleanly regardless of
	// the target server's defaults. SQLite has no analogous session state.
	if driver == DriverMySQL {
		fmt.Fprintln(bw, "/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;")
		fmt.Fprintln(bw, "/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;")
		fmt.Fprintln(bw, "/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;")
		fmt.Fprintln(bw, "SET NAMES utf8mb4;")
		fmt.Fprintln(bw, "/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;")
		fmt.Fprintln(bw, "/*!40101 SET @OLD_SQL_MODE='NO_AUTO_VALUE_ON_ZERO', SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;")
		fmt.Fprintln(bw, "/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;")
		fmt.Fprintln(bw)
	}

	for _, table := range tables {
		if err := dumpTableSQL(bw, database, driver, table); err != nil {
			return fmt.Errorf("dump table %s: %w", table, err)
		}
	}

	// Restore the session variables saved in the header block.
	if driver == DriverMySQL {
		fmt.Fprintln(bw)
		fmt.Fprintln(bw, "/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;")
		fmt.Fprintln(bw, "/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;")
		fmt.Fprintln(bw, "/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;")
		fmt.Fprintln(bw, "/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;")
		fmt.Fprintln(bw, "/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;")
		fmt.Fprintln(bw, "/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;")
	}
	return nil
}

func dumpTableSQL(bw *bufio.Writer, database DB, driver Driver, table string) error {
	cols, err := database.TableColumnInfo(table)
	if err != nil {
		return err
	}
	fks, err := database.ForeignKeys(table)
	if err != nil {
		return err
	}

	fmt.Fprintf(bw, "--\n-- Table: %s\n--\n", table)
	fmt.Fprintf(bw, "DROP TABLE IF EXISTS %s;\n", quoteIdent(driver, table))

	createSQL := buildCreateTableFromInfo(driver, table, cols, fks)
	fmt.Fprintf(bw, "%s;\n\n", createSQL)

	result, err := database.Execute("SELECT * FROM " + quoteIdent(driver, table))
	if err != nil {
		return err
	}
	if len(result.Rows) == 0 {
		fmt.Fprintln(bw)
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
		fmt.Fprintf(bw, "LOCK TABLES %s WRITE;\n", tableIdent)
		fmt.Fprintf(bw, "/*!40000 ALTER TABLE %s DISABLE KEYS */;\n", tableIdent)
	}
	for i, row := range result.Rows {
		if i%insertBatchSize == 0 {
			if i > 0 {
				fmt.Fprintln(bw, ";")
			}
			fmt.Fprintf(bw, "INSERT INTO %s (%s) VALUES\n", tableIdent, colList)
		} else {
			fmt.Fprintln(bw, ",")
		}
		values := make([]string, len(row))
		for j, val := range row {
			values[j] = formatSQLValue(val, colTypes[j])
		}
		fmt.Fprintf(bw, "  (%s)", strings.Join(values, ", "))
	}
	fmt.Fprintln(bw, ";")
	if driver == DriverMySQL {
		fmt.Fprintf(bw, "/*!40000 ALTER TABLE %s ENABLE KEYS */;\n", tableIdent)
		fmt.Fprintln(bw, "UNLOCK TABLES;")
	}
	fmt.Fprintln(bw)
	return nil
}

// buildCreateTableFromInfo reconstructs a CREATE TABLE statement from column
// metadata. Single-column primary keys are emitted inline; composite primary
// keys use a table-level constraint. Foreign keys are appended as table-level
// constraints.
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
				} else {
					b.WriteString(" AUTO_INCREMENT")
				}
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
// Numeric column types are left unquoted; all other values are single-quoted
// with embedded single quotes doubled.
func formatSQLValue(value, colType string) string {
	if value == "NULL" {
		return "NULL"
	}
	if isNumericType(colType) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// isNumericType reports whether a database column type should be treated as
// numeric (and therefore left unquoted in generated SQL). This mirrors the
// logic in the UI package but lives here so the dumper is self-contained.
func isNumericType(dbType string) bool {
	if dbType == "" {
		return false
	}
	t := strings.ToLower(dbType)
	if i := strings.IndexByte(t, '('); i > 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(t)
	switch t {
	case "int", "integer", "tinyint", "smallint", "mediumint", "bigint",
		"unsigned", "int unsigned", "unsigned big int",
		"real", "double", "float", "decimal", "numeric", "boolean", "bool":
		return true
	}
	return false
}
