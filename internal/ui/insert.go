package ui

import (
	"fmt"
	"strings"

	"github.com/rsiota/creel/internal/db"
)

// insertColumn represents one column included in a pending INSERT.
type insertColumn struct {
	Name  string
	Value string
	Type  string
}

// buildInsertQuery builds a parameterized INSERT from column values.
// Empty optional columns are omitted; required columns without values return an error.
// DateTime values stored as ISO-8601 (e.g. "2026-01-07T15:04:30Z") are normalized to
// "YYYY-MM-DD HH:MM:SS" which both MySQL and SQLite accept.
//
// blobs (optional) supplies raw []byte for binary columns; when present for a
// column name it overrides the string in values (which would be the display
// placeholder).
func buildInsertQuery(driver db.Driver, table string, columns []db.TableColumnInfo, values map[string]string, blobs map[string][]byte) (string, []interface{}, error) {
	if table == "" {
		return "", nil, fmt.Errorf("no table for insert")
	}

	valueSet := make(map[string]string, len(values))
	for k, v := range values {
		valueSet[strings.ToLower(k)] = v
	}
	blobSet := make(map[string][]byte, len(blobs))
	for k, v := range blobs {
		blobSet[strings.ToLower(k)] = v
	}

	var included []insertColumn
	var args []interface{}
	for _, col := range columns {
		key := strings.ToLower(col.Name)
		if data, ok := blobSet[key]; ok {
			included = append(included, insertColumn{Name: col.Name, Type: col.Type})
			args = append(args, data)
			continue
		}
		val, hasValue := valueSet[key]
		if hasValue && val != "" {
			included = append(included, insertColumn{Name: col.Name, Value: val, Type: col.Type})
			if val == "NULL" {
				args = append(args, nil)
			} else if db.IsDateTimeType(col.Type) {
				args = append(args, db.FormatDateTimeLiteral(val))
			} else {
				args = append(args, val)
			}
			continue
		}
		if col.AutoIncrement {
			continue
		}
		if col.NotNull && !col.HasDefault {
			return "", nil, fmt.Errorf("column %q is required", col.Name)
		}
	}

	if len(included) == 0 {
		return "", nil, fmt.Errorf("no values to insert")
	}

	colNames := make([]string, len(included))
	for i, col := range included {
		colNames[i] = col.Name
	}
	placeholders := db.Placeholders(driver, len(included))

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "),
	)
	return query, args, nil
}

// insertValuesByName maps inspector insert values from column indices to names.
func insertValuesByName(results ResultsTable, values map[int]string) map[string]string {
	out := make(map[string]string, len(values))
	for col, val := range values {
		name := results.ColumnName(col)
		if name != "" {
			out[name] = val
		}
	}
	return out
}
