package ui

import (
	"fmt"

	"github.com/ruben/gsql/internal/db"
)

// buildTruncateQuery returns the SQL statement that removes all rows from a table.
func buildTruncateQuery(driver db.Driver, table string) string {
	switch driver {
	case db.DriverSQLite:
		return fmt.Sprintf("DELETE FROM %s", table)
	default:
		return fmt.Sprintf("TRUNCATE TABLE %s", table)
	}
}
