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
	PrimaryKey bool
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

// ValidateCreateTable checks table metadata before generating CREATE TABLE SQL.
// Unlike ValidateAddColumn, NOT NULL columns are allowed without a default —
// CREATE TABLE has no such restriction (the app may supply values on INSERT).
func ValidateCreateTable(table string, cols []ColumnDef, existingTables []string) error {
	if err := ValidateIdentifier(table); err != nil {
		return err
	}
	trimmedTable := strings.TrimSpace(table)
	for _, ex := range existingTables {
		if strings.EqualFold(ex, trimmedTable) {
			return fmt.Errorf("table %q already exists", trimmedTable)
		}
	}
	var seen []string
	nonEmpty := 0
	for _, col := range cols {
		name := strings.TrimSpace(col.Name)
		colType := strings.TrimSpace(col.Type)
		if name == "" && colType == "" {
			continue
		}
		if name == "" {
			return fmt.Errorf("column name is required")
		}
		if !identPattern.MatchString(name) {
			return fmt.Errorf("column name %q is invalid", name)
		}
		if colType == "" {
			return fmt.Errorf("column type is required for %q", name)
		}
		for _, s := range seen {
			if strings.EqualFold(s, name) {
				return fmt.Errorf("column %q is duplicated", name)
			}
		}
		seen = append(seen, name)
		nonEmpty++
	}
	if nonEmpty == 0 {
		return fmt.Errorf("at least one column is required")
	}
	return nil
}

// BuildCreateTableSQL generates a CREATE TABLE statement. Fully-blank column
// rows (no name and no type) are skipped silently so the form can keep a
// trailing empty row without submit failing.
func BuildCreateTableSQL(driver Driver, table string, cols []ColumnDef, existingTables []string) (string, error) {
	if err := ValidateCreateTable(table, cols, existingTables); err != nil {
		return "", err
	}
	table = strings.TrimSpace(table)
	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s (", quoteIdent(driver, table))
	first := true
	for _, col := range cols {
		name := strings.TrimSpace(col.Name)
		colType := strings.TrimSpace(col.Type)
		if name == "" && colType == "" {
			continue
		}
		if !first {
			b.WriteString(",")
		}
		first = false
		fmt.Fprintf(&b, "\n    %s %s", quoteIdent(driver, name), colType)
		if col.PrimaryKey {
			b.WriteString(" PRIMARY KEY")
		}
		if col.NotNull {
			b.WriteString(" NOT NULL")
		}
		if col.HasDefault {
			b.WriteString(" DEFAULT ")
			b.WriteString(formatDefault(col.Default))
		}
	}
	b.WriteString("\n)")
	return b.String(), nil
}

// ValidateIdentifier checks that a SQL identifier is safe to use unquoted beyond quoting.
func ValidateIdentifier(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !identPattern.MatchString(name) {
		return fmt.Errorf("name %q is invalid", name)
	}
	return nil
}

// ColumnDefFromInfo converts table metadata into a column definition.
func ColumnDefFromInfo(info TableColumnInfo) ColumnDef {
	col := ColumnDef{
		Name:    info.Name,
		Type:    info.Type,
		NotNull: info.NotNull,
	}
	if info.HasDefault {
		col.HasDefault = true
		col.Default = info.DefaultValue
	}
	return col
}

// ValidateRenameColumn checks a rename operation.
func ValidateRenameColumn(oldName, newName string, existing []string) error {
	if err := ValidateIdentifier(newName); err != nil {
		return err
	}
	if strings.EqualFold(oldName, newName) {
		return fmt.Errorf("new name is the same as the current name")
	}
	for _, ex := range existing {
		if strings.EqualFold(ex, newName) {
			return fmt.Errorf("column %q already exists", newName)
		}
	}
	return nil
}

// ValidateRenameTable checks a table rename operation.
func ValidateRenameTable(oldName, newName string, existing []string) error {
	if err := ValidateIdentifier(oldName); err != nil {
		return err
	}
	if err := ValidateIdentifier(newName); err != nil {
		return err
	}
	if strings.EqualFold(oldName, newName) {
		return fmt.Errorf("new name is the same as the current name")
	}
	for _, ex := range existing {
		if strings.EqualFold(ex, newName) {
			return fmt.Errorf("table %q already exists", newName)
		}
	}
	return nil
}

// BuildRenameTableSQL generates a statement that renames a table.
func BuildRenameTableSQL(driver Driver, oldName, newName string, existing []string) (string, error) {
	if err := ValidateRenameTable(oldName, newName, existing); err != nil {
		return "", err
	}
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	switch driver {
	case DriverSQLite, DriverPostgres:
		return fmt.Sprintf("ALTER TABLE %s RENAME TO %s",
			quoteIdent(driver, oldName),
			quoteIdent(driver, newName),
		), nil
	case DriverMySQL:
		return fmt.Sprintf("RENAME TABLE %s TO %s",
			quoteIdent(driver, oldName),
			quoteIdent(driver, newName),
		), nil
	default:
		return "", fmt.Errorf("rename table is not supported for %s", driver)
	}
}

// BuildRenameColumnSQL generates an ALTER TABLE ... RENAME COLUMN statement.
func BuildRenameColumnSQL(driver Driver, table, oldName, newName string, existing []string) (string, error) {
	if strings.TrimSpace(table) == "" {
		return "", fmt.Errorf("table name is required")
	}
	if err := ValidateIdentifier(oldName); err != nil {
		return "", err
	}
	if err := ValidateRenameColumn(oldName, newName, existing); err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
		quoteIdent(driver, table),
		quoteIdent(driver, oldName),
		quoteIdent(driver, strings.TrimSpace(newName)),
	), nil
}

// ValidateModifyColumn checks a MySQL MODIFY COLUMN definition.
func ValidateModifyColumn(col ColumnDef) error {
	if err := ValidateIdentifier(col.Name); err != nil {
		return err
	}
	if strings.TrimSpace(col.Type) == "" {
		return fmt.Errorf("column type is required")
	}
	return nil
}

// BuildModifyColumnSQL generates an ALTER TABLE statement to change a column's
// type, nullability, and/or default. MySQL uses MODIFY COLUMN; PostgreSQL uses
// one or more ALTER COLUMN clauses chained in a single statement.
func BuildModifyColumnSQL(driver Driver, table string, col ColumnDef) (string, error) {
	if driver != DriverMySQL && driver != DriverPostgres {
		return "", fmt.Errorf("modify column is not supported for %s", driver)
	}
	if strings.TrimSpace(table) == "" {
		return "", fmt.Errorf("table name is required")
	}
	if err := ValidateModifyColumn(col); err != nil {
		return "", err
	}
	if driver == DriverPostgres {
		return buildPostgresAlterColumn(table, col)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ALTER TABLE %s MODIFY COLUMN %s %s",
		quoteIdent(driver, table),
		quoteIdent(driver, strings.TrimSpace(col.Name)),
		strings.TrimSpace(col.Type),
	)
	if col.NotNull {
		b.WriteString(" NOT NULL")
	} else {
		b.WriteString(" NULL")
	}
	if col.HasDefault {
		b.WriteString(" DEFAULT ")
		b.WriteString(formatDefault(col.Default))
	}
	return b.String(), nil
}

// buildPostgresAlterColumn chains ALTER COLUMN clauses into a single ALTER
// TABLE statement. PostgreSQL handles type, nullability, and default changes
// as separate sub-commands.
func buildPostgresAlterColumn(table string, col ColumnDef) (string, error) {
	var clauses []string
	colIdent := quoteIdent(DriverPostgres, strings.TrimSpace(col.Name))
	clauses = append(clauses, fmt.Sprintf("ALTER COLUMN %s TYPE %s", colIdent, strings.TrimSpace(col.Type)))
	if col.NotNull {
		clauses = append(clauses, fmt.Sprintf("ALTER COLUMN %s SET NOT NULL", colIdent))
	} else {
		clauses = append(clauses, fmt.Sprintf("ALTER COLUMN %s DROP NOT NULL", colIdent))
	}
	if col.HasDefault {
		clauses = append(clauses, fmt.Sprintf("ALTER COLUMN %s SET DEFAULT %s", colIdent, formatDefault(col.Default)))
	}
	return fmt.Sprintf("ALTER TABLE %s %s",
		quoteIdent(DriverPostgres, strings.TrimSpace(table)),
		strings.Join(clauses, ", ")), nil
}

// ValidateDropColumn checks whether a column can be dropped.
func ValidateDropColumn(info TableColumnInfo) error {
	if info.PrimaryKey {
		return fmt.Errorf("cannot drop primary key column %q", info.Name)
	}
	if info.AutoIncrement {
		return fmt.Errorf("cannot drop auto-increment column %q", info.Name)
	}
	return nil
}

// BuildDropTableSQL generates a DROP TABLE statement.
func BuildDropTableSQL(driver Driver, table string) (string, error) {
	if strings.TrimSpace(table) == "" {
		return "", fmt.Errorf("table name is required")
	}
	return fmt.Sprintf("DROP TABLE %s", quoteIdent(driver, table)), nil
}

// BuildDropDatabaseSQL generates a DROP DATABASE statement (MySQL and PostgreSQL).
// Dropping a database permanently deletes every table and all data within it.
func BuildDropDatabaseSQL(driver Driver, name string) (string, error) {
	if driver != DriverMySQL && driver != DriverPostgres {
		return "", fmt.Errorf("drop database is not supported for %s", driver)
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("database name is required")
	}
	if err := ValidateIdentifier(name); err != nil {
		return "", err
	}
	// Guard against dropping system databases.
	switch strings.ToLower(name) {
	case "mysql", "information_schema", "performance_schema", "sys":
		return "", fmt.Errorf("cannot drop system database %q", name)
	case "postgres", "template0", "template1":
		return "", fmt.Errorf("cannot drop system database %q", name)
	}
	return fmt.Sprintf("DROP DATABASE %s", quoteIdent(driver, name)), nil
}

// BuildCreateDatabaseSQL generates a CREATE DATABASE statement (MySQL and PostgreSQL).
func BuildCreateDatabaseSQL(driver Driver, name string) (string, error) {
	if driver != DriverMySQL && driver != DriverPostgres {
		return "", fmt.Errorf("create database is not supported for %s", driver)
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("database name is required")
	}
	if err := ValidateIdentifier(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("CREATE DATABASE %s", quoteIdent(driver, name)), nil
}

// BuildDropColumnSQL generates an ALTER TABLE ... DROP COLUMN statement.
func BuildDropColumnSQL(driver Driver, table, column string, info TableColumnInfo) (string, error) {
	if driver != DriverMySQL && driver != DriverPostgres {
		return "", fmt.Errorf("drop column is not supported for %s", driver)
	}
	if strings.TrimSpace(table) == "" {
		return "", fmt.Errorf("table name is required")
	}
	if err := ValidateIdentifier(column); err != nil {
		return "", err
	}
	if err := ValidateDropColumn(info); err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s",
		quoteIdent(driver, table),
		quoteIdent(driver, column),
	), nil
}

func quoteIdent(driver Driver, name string) string {
	switch driver {
	case DriverSQLite, DriverPostgres:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	default:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
}

func formatDefault(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "''"
	}
	if strings.EqualFold(trimmed, "NULL") {
		return "NULL"
	}
	if strings.EqualFold(trimmed, "true") || strings.EqualFold(trimmed, "false") {
		return trimmed
	}
	if numericDefaultPattern.MatchString(trimmed) {
		return trimmed
	}
	if isSQLFunctionDefault(trimmed) {
		return trimmed
	}
	// PostgreSQL expression defaults (e.g. nextval('...'::regclass)).
	if isExpressionDefault(trimmed) {
		return trimmed
	}
	escaped := strings.ReplaceAll(trimmed, `'`, `''`)
	return "'" + escaped + "'"
}

// isSQLFunctionDefault reports whether a default value is a SQL function
// expression (e.g. CURRENT_TIMESTAMP, now()) that must be left unquoted.
var sqlFunctionDefaultPattern = regexp.MustCompile(`(?i)^(?:CURRENT_TIMESTAMP(?:\(\d*\))?|CURRENT_DATE|CURRENT_TIME|NOW\(\)|CURDATE\(\)|CURTIME\(\)|UUID\(\)|UNIX_TIMESTAMP\(\)|GEN_RANDOM_UUID\(\)|UUID_GENERATE_V4\(\))$`)

func isSQLFunctionDefault(value string) bool {
	return sqlFunctionDefaultPattern.MatchString(value)
}

// expressionDefaultPattern matches function-call-like default values that
// should be left unquoted (e.g. nextval('...'::regclass)).
var expressionDefaultPattern = regexp.MustCompile(`(?i)^[a-z_]+\(`)

func isExpressionDefault(value string) bool {
	return expressionDefaultPattern.MatchString(strings.TrimSpace(value))
}

var numericDefaultPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)

// SchemaAction identifies a schema change operation on a table or column.
type SchemaAction string

const (
	SchemaAddColumn       SchemaAction = "add_column"
	SchemaCreateTable     SchemaAction = "create_table"
	SchemaRenameTable     SchemaAction = "rename_table"
	SchemaRenameColumn    SchemaAction = "rename_column"
	SchemaModifyType      SchemaAction = "modify_type"
	SchemaModifyNullable  SchemaAction = "modify_nullable"
	SchemaModifyDefault   SchemaAction = "modify_default"
	SchemaDropColumn      SchemaAction = "drop_column"
	SchemaDropTable       SchemaAction = "drop_table"
)

// SchemaSupports reports whether a driver supports a schema action.
func SchemaSupports(driver Driver, action SchemaAction) bool {
	switch action {
	case SchemaAddColumn, SchemaCreateTable, SchemaRenameTable, SchemaRenameColumn, SchemaDropTable:
		return true
	case SchemaModifyType, SchemaModifyNullable, SchemaModifyDefault, SchemaDropColumn:
		return driver == DriverMySQL || driver == DriverPostgres
	default:
		return false
	}
}

// SchemaActionLabel returns a human-readable label for a schema action.
func SchemaActionLabel(action SchemaAction) string {
	switch action {
	case SchemaAddColumn:
		return "Add column"
	case SchemaCreateTable:
		return "Create table"
	case SchemaRenameTable:
		return "Rename table"
	case SchemaRenameColumn:
		return "Rename column"
	case SchemaModifyType:
		return "Change type"
	case SchemaModifyNullable:
		return "Change nullable"
	case SchemaModifyDefault:
		return "Change default"
	case SchemaDropColumn:
		return "Drop column"
	case SchemaDropTable:
		return "Drop table"
	default:
		return string(action)
	}
}

// ColumnSchemaActions returns column-level actions available for a driver.
func ColumnSchemaActions(driver Driver) []SchemaAction {
	var actions []SchemaAction
	for _, a := range []SchemaAction{
		SchemaRenameColumn,
		SchemaModifyType,
		SchemaModifyNullable,
		SchemaModifyDefault,
		SchemaDropColumn,
	} {
		if SchemaSupports(driver, a) {
			actions = append(actions, a)
		}
	}
	return actions
}

// SchemaActionNeedsConfirm reports whether an action requires a SQL confirm step.
// Destructive DDL (e.g. drop column) gets a preview; other changes run from the form.
func SchemaActionNeedsConfirm(action SchemaAction) bool {
	return action == SchemaDropColumn
}

// Placeholders returns a slice of n SQL parameter placeholders for the given
// driver. MySQL and SQLite use "?"; PostgreSQL uses "$1, $2, ...".
func Placeholders(driver Driver, n int) []string {
	ph := make([]string, n)
	for i := 0; i < n; i++ {
		if driver == DriverPostgres {
			ph[i] = fmt.Sprintf("$%d", i+1)
		} else {
			ph[i] = "?"
		}
	}
	return ph
}
