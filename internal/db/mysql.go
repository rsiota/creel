package db

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

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

func (m *MySQL) Execute(query string) (Result, error) {
	start := time.Now()

	rows, err := m.db.Query(query)
	if err != nil {
		return Result{}, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return Result{}, err
	}

	cols := make([]Column, len(colNames))
	for i, name := range colNames {
		cols[i] = Column{Name: name}
	}

	colTypes, _ := rows.ColumnTypes()
	for i, ct := range colTypes {
		cols[i].Type = ct.DatabaseTypeName()
	}

	var resultRows [][]string
	for rows.Next() {
		rawValues := make([]sql.NullString, len(cols))
		scanArgs := make([]interface{}, len(cols))
		for i := range rawValues {
			scanArgs[i] = &rawValues[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return Result{}, err
		}

		row := make([]string, len(cols))
		for i, v := range rawValues {
			if v.Valid {
				row[i] = v.String
			} else {
				row[i] = "NULL"
			}
		}
		resultRows = append(resultRows, row)
	}

	if err := rows.Err(); err != nil {
		return Result{}, err
	}

	elapsed := time.Since(start)
	noun := "rows"
	if len(resultRows) == 1 {
		noun = "row"
	}

	return Result{
		Columns: cols,
		Rows:    resultRows,
		Message: fmt.Sprintf("%d %s in %s", len(resultRows), noun, elapsed.Round(time.Millisecond)),
		Elapsed: elapsed.String(),
	}, nil
}

func (m *MySQL) Exec(query string, args ...interface{}) (ExecResult, error) {
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
