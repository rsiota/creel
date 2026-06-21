package ui

import "fmt"

// buildDeleteQuery returns a DELETE statement that removes specific rows by
// primary key. The WHERE clause reuses buildPKInClause so single and composite
// PKs are both handled with correct type quoting.
func buildDeleteQuery(table string, pkNames, pkTypes []string, tuples [][]string) string {
	clause := buildPKInClause(pkNames, pkTypes, tuples)
	return fmt.Sprintf("DELETE FROM %s WHERE %s", table, clause)
}
