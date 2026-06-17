package ui

import (
	"fmt"
	"strings"
)

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func buildForeignKeyQuery(refTable, refColumn, value string) string {
	return fmt.Sprintf("SELECT * FROM %s WHERE %s = %s",
		refTable, refColumn, quoteSQLString(value))
}
