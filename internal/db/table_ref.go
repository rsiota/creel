package db

import "strings"

// SplitTableRef splits "schema.table" into parts. Bare names return ("", name).
// Dots inside quoted identifiers are not supported.
func SplitTableRef(name string) (schema, table string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	if i := strings.LastIndex(name, "."); i > 0 && i < len(name)-1 {
		return name[:i], name[i+1:]
	}
	return "", name
}

// QuoteTableRef returns a driver-quoted schema.table (or bare table when schema
// is empty) for use in SQL identifiers.
func QuoteTableRef(driver Driver, schema, table string) string {
	if schema == "" {
		return quoteIdent(driver, table)
	}
	return quoteIdent(driver, schema) + "." + quoteIdent(driver, table)
}
