package db

import (
	"fmt"
	"regexp"
	"strings"
)

// ColumnDef describes a column for DDL operations such as ADD COLUMN.
type ColumnDef struct {
	Name       string
	Type       string
	NotNull    bool
	HasDefault bool
	Default    string
}

var identPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ValidateAddColumn checks column metadata before generating ADD COLUMN SQL.
func ValidateAddColumn(col ColumnDef, existing []string) error {
	name := strings.TrimSpace(col.Name)
	if name == "" {
		return fmt.Errorf("column name is required")
	}
	if !identPattern.MatchString(name) {
		return fmt.Errorf("column name %q is invalid", name)
	}
	for _, ex := range existing {
		if strings.EqualFold(ex, name) {
			return fmt.Errorf("column %q already exists", name)
		}
	}
	if strings.TrimSpace(col.Type) == "" {
		return fmt.Errorf("column type is required")
	}
	if col.NotNull && !col.HasDefault {
		return fmt.Errorf("NOT NULL columns require a default value")
	}
	return nil
}

// BuildAddColumnSQL generates an ALTER TABLE ... ADD COLUMN statement.
func BuildAddColumnSQL(driver Driver, table string, col ColumnDef, existing []string) (string, error) {
	if strings.TrimSpace(table) == "" {
		return "", fmt.Errorf("table name is required")
	}
	if err := ValidateAddColumn(col, existing); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "ALTER TABLE %s ADD COLUMN %s %s",
		quoteIdent(driver, table),
		quoteIdent(driver, strings.TrimSpace(col.Name)),
		strings.TrimSpace(col.Type),
	)
	if col.NotNull {
		b.WriteString(" NOT NULL")
	}
	if col.HasDefault {
		b.WriteString(" DEFAULT ")
		b.WriteString(formatDefault(col.Default))
	}
	return b.String(), nil
}

func quoteIdent(driver Driver, name string) string {
	switch driver {
	case DriverSQLite:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	default:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
}

func formatDefault(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.EqualFold(trimmed, "NULL") {
		return "NULL"
	}
	if numericDefaultPattern.MatchString(trimmed) {
		return trimmed
	}
	escaped := strings.ReplaceAll(trimmed, `'`, `''`)
	return "'" + escaped + "'"
}

var numericDefaultPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
