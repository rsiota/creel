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
	return fmt.Sprintf("%s:%s@%s(%s:%d)/%s?parseTime=true",
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
