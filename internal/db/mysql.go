package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"

	_ "github.com/go-sql-driver/mysql"
)

// MySQL implements the DB interface for MySQL databases.
type MySQL struct {
	config  ConnectionConfig
	db      *sql.DB
	tunnel  *SSHTunnel
	dialNet string
}

// NewMySQL creates a new MySQL database handler.
func NewMySQL(cfg ConnectionConfig) *MySQL {
	return &MySQL{config: cfg}
}

var sshDialCounter uint64

func (m *MySQL) Connect() error {
	// If SSH tunnel is configured, establish it and register a custom dialer.
	if m.config.SSHHost != "" {
		tunnel, err := NewSSHTunnel(m.config)
		if err != nil {
			return fmt.Errorf("ssh tunnel: %w", err)
		}
		m.tunnel = tunnel

		// Register a unique dial network name for this connection.
		dialNet := fmt.Sprintf("ssh+%d", atomic.AddUint64(&sshDialCounter, 1))
		mysqlRegisterDialContext(dialNet, tunnel)
		m.dialNet = dialNet
	}

	db, err := sql.Open("mysql", m.dsn())
	if err != nil {
		return fmt.Errorf("failed to open mysql: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	m.db = db
	return db.Ping()
}

func (m *MySQL) dsn() string {
	netName := "tcp"
	if m.dialNet != "" {
		netName = m.dialNet
	}
	// maxAllowedPacket=0 asks the driver to use the server's value so large
	// dump INSERTs are not capped at the driver's 64MiB default.
	return fmt.Sprintf("%s:%s@%s(%s:%d)/%s?parseTime=true&maxAllowedPacket=0",
		m.config.Username,
		m.config.Password,
		netName,
		m.config.Host,
		m.config.Port,
		m.config.Database,
	)
}

// Databases returns all user-accessible schemas on the server.
func (m *MySQL) Databases() ([]string, error) {
	rows, err := m.db.Query(
		`SELECT schema_name FROM information_schema.schemata
		 WHERE schema_name NOT IN ('information_schema','performance_schema','mysql','sys')
		 ORDER BY schema_name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		dbs = append(dbs, name)
	}
	return dbs, rows.Err()
}

// UseDatabase switches to a different schema by re-opening the connection pool.
// The SSH tunnel (if any) is preserved.
func (m *MySQL) UseDatabase(name string) error {
	m.config.Database = name
	if m.db != nil {
		m.db.Close()
	}
	db, err := sql.Open("mysql", m.dsn())
	if err != nil {
		return fmt.Errorf("failed to open mysql: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	m.db = db
	return db.Ping()
}

// Schemas returns MySQL schemas (identical to Databases — MySQL equates the two).
func (m *MySQL) Schemas() ([]string, error) {
	return m.Databases()
}

// UseSchema switches schema by delegating to UseDatabase.
func (m *MySQL) UseSchema(name string) error {
	return m.UseDatabase(name)
}

func (m *MySQL) Close() error {
	if m.db != nil {
		m.db.Close()
	}
	if m.tunnel != nil {
		m.tunnel.Close()
	}
	return nil
}

func (m *MySQL) Tables() ([]string, error) {
	rows, err := m.db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema = ? ORDER BY table_name`, m.config.Database)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// Views returns MySQL views in the current database.
func (m *MySQL) Views() ([]string, error) {
	rows, err := m.db.Query(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = ? AND table_type = 'VIEW'
		 ORDER BY table_name`,
		m.config.Database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		views = append(views, name)
	}
	return views, rows.Err()
}

func (m *MySQL) TableRowCounts() (map[string]int64, error) {
	rows, err := m.db.Query(
		`SELECT table_name, table_rows FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE'`,
		m.config.Database,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var tableRows sql.NullInt64
		if err := rows.Scan(&name, &tableRows); err != nil {
			return nil, err
		}
		if tableRows.Valid {
			counts[name] = tableRows.Int64
		}
	}
	return counts, rows.Err()
}

func (m *MySQL) TableSchema(table string) ([]Column, error) {
	rows, err := m.db.Query(
		`SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`,
		m.config.Database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []Column
	for rows.Next() {
		var name, dataType string
		if err := rows.Scan(&name, &dataType); err != nil {
			return nil, err
		}
		cols = append(cols, Column{Name: name, Type: dataType})
	}
	return cols, rows.Err()
}

func (m *MySQL) PrimaryKeys(table string) ([]string, error) {
	rows, err := m.db.Query(
		`SELECT column_name FROM information_schema.key_column_usage
		 WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'
		 ORDER BY ordinal_position`,
		m.config.Database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pks []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		pks = append(pks, name)
	}
	return pks, rows.Err()
}

func (m *MySQL) ForeignKeys(table string) ([]ForeignKey, error) {
	rows, err := m.db.Query(
		`SELECT column_name, referenced_table_name, referenced_column_name
		 FROM information_schema.key_column_usage
		 WHERE table_schema = ? AND table_name = ? AND referenced_table_name IS NOT NULL
		 ORDER BY ordinal_position`,
		m.config.Database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []ForeignKey
	for rows.Next() {
		var col, refTable, refCol string
		if err := rows.Scan(&col, &refTable, &refCol); err != nil {
			return nil, err
		}
		fks = append(fks, ForeignKey{
			Column:    col,
			RefTable:  refTable,
			RefColumn: refCol,
		})
	}
	return fks, rows.Err()
}

// ReferencingForeignKeys returns FKs pointing AT the given table (the reverse
// of ForeignKeys) by querying the referenced side of key_column_usage.
func (m *MySQL) ReferencingForeignKeys(table string) ([]Referrer, error) {
	rows, err := m.db.Query(
		`SELECT table_name, column_name, referenced_column_name
		 FROM information_schema.key_column_usage
		 WHERE table_schema = ? AND referenced_table_name = ?
		 ORDER BY table_name, ordinal_position`,
		m.config.Database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []Referrer
	for rows.Next() {
		var tbl, col, refCol string
		if err := rows.Scan(&tbl, &col, &refCol); err != nil {
			return nil, err
		}
		refs = append(refs, Referrer{Table: tbl, Column: col, RefColumn: refCol})
	}
	return refs, rows.Err()
}

// Uses returns objects (views, functions, procedures, triggers) whose
// definitions reference the given table, via a textual scan of
// information_schema. routine_type distinguishes functions from procedures.
func (m *MySQL) Uses(table string) ([]Usage, error) {
	schema := m.config.Database
	var defs []Usage

	// Views.
	vr, err := m.db.Query(`SELECT table_name, view_definition FROM information_schema.views WHERE table_schema = ?`, schema)
	if err != nil {
		return nil, fmt.Errorf("uses: views: %w", err)
	}
	if err := scanNameBody(vr, "view", &defs); err != nil {
		vr.Close()
		return nil, err
	}
	vr.Close()

	// Functions and procedures.
	rr, err := m.db.Query(`SELECT routine_name, routine_type, routine_definition FROM information_schema.routines WHERE routine_schema = ?`, schema)
	if err != nil {
		return nil, fmt.Errorf("uses: routines: %w", err)
	}
	for rr.Next() {
		var name, typ, body sql.NullString
		if err := rr.Scan(&name, &typ, &body); err != nil {
			rr.Close()
			return nil, err
		}
		defs = append(defs, Usage{Kind: strings.ToLower(typ.String), Name: name.String, Body: body.String})
	}
	rr.Close()
	if err := rr.Err(); err != nil {
		return nil, err
	}

	// Triggers.
	tr, err := m.db.Query(`SELECT trigger_name, action_statement FROM information_schema.triggers WHERE trigger_schema = ?`, schema)
	if err != nil {
		return nil, fmt.Errorf("uses: triggers: %w", err)
	}
	if err := scanNameBody(tr, "trigger", &defs); err != nil {
		tr.Close()
		return nil, err
	}
	tr.Close()

	return definitionsReferencing(defs, table), nil
}

func (m *MySQL) TableColumnInfo(table string) ([]TableColumnInfo, error) {
	rows, err := m.db.Query(
		`SELECT column_name, column_type, is_nullable, column_default, column_key, extra
		 FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY ordinal_position`,
		m.config.Database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []TableColumnInfo
	for rows.Next() {
		var name, dataType, nullable, colKey, extra string
		var colDefault sql.NullString
		if err := rows.Scan(&name, &dataType, &nullable, &colDefault, &colKey, &extra); err != nil {
			return nil, err
		}
		cols = append(cols, TableColumnInfo{
			Name:          name,
			Type:          dataType,
			NotNull:       nullable == "NO",
			PrimaryKey:    colKey == "PRI",
			AutoIncrement: strings.Contains(strings.ToLower(extra), "auto_increment"),
			HasDefault:    colDefault.Valid,
			DefaultValue:  colDefault.String,
		})
	}
	return cols, rows.Err()
}

func (m *MySQL) Execute(query string) (Result, error) {
	return m.ExecuteContext(context.Background(), query)
}

func (m *MySQL) ExecuteContext(ctx context.Context, query string) (Result, error) {
	if err := rejectWriteIfReadOnly(m.config, query); err != nil {
		return Result{}, err
	}
	return executeRows(ctx, m.db, query)
}

func (m *MySQL) Exec(query string, args ...interface{}) (ExecResult, error) {
	if err := rejectWriteIfReadOnly(m.config, query); err != nil {
		return ExecResult{}, err
	}
	res, err := m.db.Exec(query, args...)
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec error: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{RowsAffected: affected}, nil
}

// Session returns a runner pinned to a single pooled connection so that
// session-scoped settings (FOREIGN_KEY_CHECKS, SQL_MODE, ...) set by a dump
// persist across statements instead of being lost across the connection pool.
func (m *MySQL) Session() (SessionRunner, error) {
	if m.config.ReadOnly {
		return nil, ErrReadOnly
	}
	conn, err := m.db.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	return &sqlConnSession{conn: conn}, nil
}

func (m *MySQL) Begin() (Tx, error) {
	if m.config.ReadOnly {
		return nil, ErrReadOnly
	}
	return beginTx(m.db)
}

// Indexes returns the secondary indexes on a table from information_schema.
// The PRIMARY index is excluded (shown in its own section). Multi-column
// indexes collapse their columns into a single Index entry, ordered by
// seq_in_index.
func (m *MySQL) Indexes(table string) ([]Index, error) {
	rows, err := m.db.Query(
		`SELECT index_name, column_name, non_unique
		 FROM information_schema.statistics
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY index_name, seq_in_index`,
		m.config.Database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	order := []string{}
	byName := map[string]*Index{}
	for rows.Next() {
		var indexName, column string
		var nonUnique int
		if err := rows.Scan(&indexName, &column, &nonUnique); err != nil {
			return nil, err
		}
		if indexName == "PRIMARY" {
			continue
		}
		idx, ok := byName[indexName]
		if !ok {
			idx = &Index{Name: indexName, Unique: nonUnique == 0}
			byName[indexName] = idx
			order = append(order, indexName)
		}
		idx.Columns = append(idx.Columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	idxs := make([]Index, len(order))
	for i, name := range order {
		idxs[i] = *byName[name]
	}
	return idxs, nil
}

// Triggers returns triggers from information_schema.triggers.
func (m *MySQL) Triggers(table string) ([]Trigger, error) {
	rows, err := m.db.Query(
		`SELECT trigger_name, action_timing, event_manipulation, action_statement
		 FROM information_schema.triggers
		 WHERE trigger_schema = ? AND event_object_table = ?
		 ORDER BY trigger_name`,
		m.config.Database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var triggers []Trigger
	for rows.Next() {
		var name, timing, event, statement string
		if err := rows.Scan(&name, &timing, &event, &statement); err != nil {
			return nil, err
		}
		triggers = append(triggers, Trigger{
			Name:      name,
			Timing:    timing,
			Event:     event,
			Statement: statement,
		})
	}
	return triggers, rows.Err()
}

// TableDefinition returns SHOW CREATE TABLE output for a table (or view).
// That statement includes indexes, named foreign keys, ON DELETE/UPDATE,
// CHECK constraints, ENGINE, CHARSET, and COLLATE — unlike reconstructed DDL.
func (m *MySQL) TableDefinition(table string) (string, error) {
	var name, ddl string
	err := m.db.QueryRow("SHOW CREATE TABLE " + quoteIdent(DriverMySQL, table)).Scan(&name, &ddl)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("show create table: %w", err)
	}
	return strings.TrimRight(strings.TrimSpace(ddl), ";"), nil
}

// ViewDefinition returns the body of a view from information_schema.views, or
// "" if the named relation is not a view.
func (m *MySQL) ViewDefinition(view string) (string, error) {
	var def sql.NullString
	err := m.db.QueryRow(
		`SELECT view_definition FROM information_schema.views
		 WHERE table_schema = ? AND table_name = ?`,
		m.config.Database, view,
	).Scan(&def)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return def.String, nil
}

// CheckConstraints returns CHECK constraints from information_schema. The
// check_constraints table holds the clause but not the table, so it is joined
// to table_constraints (same constraint name + schema) to scope to this table.
// Requires MySQL 8.0.16+ (which enforces checks); older versions store them
// as ignored table options and return nothing here. check_clause is the raw
// expression as written (often already parenthesized), and MySQL does not tie
// a check to a single column, so Column is left empty.
func (m *MySQL) CheckConstraints(table string) ([]CheckConstraint, error) {
	rows, err := m.db.Query(
		`SELECT cc.constraint_name, cc.check_clause
		 FROM information_schema.check_constraints cc
		 JOIN information_schema.table_constraints tc
		   ON cc.constraint_name = tc.constraint_name
		  AND cc.constraint_schema = tc.constraint_schema
		 WHERE tc.table_schema = ? AND tc.table_name = ?
		 ORDER BY cc.constraint_name`,
		m.config.Database, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CheckConstraint
	for rows.Next() {
		var name string
		var clause sql.NullString
		if err := rows.Scan(&name, &clause); err != nil {
			return nil, err
		}
		expr := strings.TrimSpace(clause.String)
		// MySQL wraps the clause in parens; unwrap a single outer pair for a
		// cleaner display (matching the PostgreSQL path).
		if len(expr) >= 2 && expr[0] == '(' && expr[len(expr)-1] == ')' {
			expr = strings.TrimSpace(expr[1 : len(expr)-1])
		}
		out = append(out, CheckConstraint{Name: name, Expression: expr})
	}
	return out, rows.Err()
}
