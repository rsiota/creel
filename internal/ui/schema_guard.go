package ui

import (
	"fmt"

	"github.com/ruben/gsql/internal/db"
)

// GuardColumnAction returns an error message if a column action should not
// run, or "" if it's allowed. Used by the schema editor to pre-validate
// drop/rename/modify attempts before building DDL.
func GuardColumnAction(action db.SchemaAction, col db.TableColumnInfo) string {
	switch action {
	case db.SchemaDropColumn:
		if err := db.ValidateDropColumn(col); err != nil {
			return err.Error()
		}
	case db.SchemaRenameColumn:
		if col.PrimaryKey && col.AutoIncrement {
			return fmt.Sprintf("renaming auto-increment column %q is not supported", col.Name)
		}
	case db.SchemaModifyType, db.SchemaModifyNullable, db.SchemaModifyDefault:
		if col.AutoIncrement {
			return fmt.Sprintf("cannot modify auto-increment column %q", col.Name)
		}
	}
	return ""
}
