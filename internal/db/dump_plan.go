package db

// DumpTableSpec is one table's schema/data inclusion for a selective backup.
type DumpTableSpec struct {
	Name   string
	Schema bool
	Data   bool
}

// DumpPlan describes what a native :backup should include.
//
// A zero-value plan (Tables == nil) means a full database dump — the historical
// :backup behaviour, including views and other non-table objects the tool emits
// for a whole-database invocation.
//
// When Tables is non-nil, only listed tables are dumped: Schema controls
// CREATE TABLE (and related DDL), Data controls INSERT/COPY rows. A table with
// both false is omitted. Routines/events (mysqldump) are included on the
// schema pass when any schema tables are selected.
type DumpPlan struct {
	Tables []DumpTableSpec // nil = full database dump
}

// IsFull reports whether this is an unrestricted whole-database dump.
func (p DumpPlan) IsFull() bool {
	return p.Tables == nil
}

// SchemaTables returns table names that should include DDL, in plan order.
func (p DumpPlan) SchemaTables() []string {
	var out []string
	for _, t := range p.Tables {
		if t.Schema && t.Name != "" {
			out = append(out, t.Name)
		}
	}
	return out
}

// DataTables returns table names that should include row data, in plan order.
func (p DumpPlan) DataTables() []string {
	var out []string
	for _, t := range p.Tables {
		if t.Data && t.Name != "" {
			out = append(out, t.Name)
		}
	}
	return out
}

// HasContent reports whether the plan would dump anything.
func (p DumpPlan) HasContent() bool {
	if p.IsFull() {
		return true
	}
	for _, t := range p.Tables {
		if (t.Schema || t.Data) && t.Name != "" {
			return true
		}
	}
	return false
}
