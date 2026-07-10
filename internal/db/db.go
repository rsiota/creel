package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Driver represents a supported database driver type.
type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverMySQL    Driver = "mysql"
	DriverPostgres Driver = "postgres"
)

// ConnectionConfig holds the parameters needed to connect to a database.
type ConnectionConfig struct {
	Name     string `yaml:"name" json:"name"`
	Driver   Driver `yaml:"driver" json:"driver"`
	Database string `yaml:"database" json:"database"`
	Host     string `yaml:"host,omitempty" json:"host,omitempty"`
	Port     int    `yaml:"port,omitempty" json:"port,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`

	// SSH tunnel (optional)
	SSHHost       string `yaml:"ssh_host,omitempty" json:"ssh_host,omitempty"`
	SSHPort       int    `yaml:"ssh_port,omitempty" json:"ssh_port,omitempty"`
	SSHUser       string `yaml:"ssh_user,omitempty" json:"ssh_user,omitempty"`
	SSHPassword   string `yaml:"ssh_password,omitempty" json:"ssh_password,omitempty"`
	SSHKeyPath    string `yaml:"ssh_key_path,omitempty" json:"ssh_key_path,omitempty"`
	SSHPassphrase string `yaml:"ssh_passphrase,omitempty" json:"ssh_passphrase,omitempty"`
}

// Connection wraps an open database connection with metadata.
type Connection struct {
	config ConnectionConfig
	db     DB
}

// DB is the interface that all database drivers must implement.
type DB interface {
	// Connect establishes a connection to the database.
	Connect() error
	// Close closes the database connection.
	Close() error
	// Tables returns the list of tables and views in the database.
	Tables() ([]string, error)
	// TableRowCounts returns approximate row counts for all tables.
	// MySQL uses information_schema (fast, approximate); SQLite runs COUNT(*)
	// per table. Tables with inaccessible rows are omitted from the map.
	TableRowCounts() (map[string]int64, error)
	// TableSchema returns the column names and types for a given table.
	TableSchema(table string) ([]Column, error)
	// PrimaryKeys returns the primary key column names for a table.
	PrimaryKeys(table string) ([]string, error)
	// ForeignKeys returns outbound foreign keys defined on a table.
	ForeignKeys(table string) ([]ForeignKey, error)
	// TableColumnInfo returns detailed column metadata for inserts and validation.
	TableColumnInfo(table string) ([]TableColumnInfo, error)
	// Execute runs a query and returns the result set.
	Execute(query string) (Result, error)
	// ExecuteContext runs a query with cancellation support.
	ExecuteContext(ctx context.Context, query string) (Result, error)
	// Exec runs a statement that doesn't return rows (INSERT, UPDATE, DELETE).
	Exec(query string, args ...interface{}) (ExecResult, error)
	// Databases returns the list of databases accessible through this connection.
	// For single-file drivers (e.g. SQLite) this returns the configured database only.
	Databases() ([]string, error)
	// UseDatabase switches the active database, re-opening the connection if needed.
	UseDatabase(name string) error
	// Session returns a runner that executes statements on a single pinned
	// connection so session-scoped settings persist across calls. This matters
	// for imports: a MySQL dump sets FOREIGN_KEY_CHECKS=0, SQL_MODE, etc., which
	// are per-connection and would otherwise be lost across a connection pool.
	// The caller must Close the returned runner when done.
	Session() (SessionRunner, error)
	// Begin starts a transaction. Statements run on the returned Tx are atomic;
	// Commit or Rollback must be called exactly once to finish it.
	Begin() (Tx, error)
	// Indexes returns the secondary indexes on a table. The primary-key index is
	// excluded where the driver can distinguish it; use PrimaryKeys for the PK.
	Indexes(table string) ([]Index, error)
	// Triggers returns the triggers defined on a table. Drivers that do not
	// support triggers return an empty slice (and nil error).
	Triggers(table string) ([]Trigger, error)
	// ViewDefinition returns the defining SELECT statement of a view, or "" if
	// the named relation is not a view or has no retrievable definition.
	ViewDefinition(view string) (string, error)
}

// Tx runs statements within a single database transaction. Commit or
// Rollback must be called exactly once to finish it.
type Tx interface {
	// Exec runs a statement that doesn't return rows (INSERT, UPDATE, DELETE).
	Exec(query string, args ...interface{}) (ExecResult, error)
	// Commit makes the transaction's changes permanent.
	Commit() error
	// Rollback discards the transaction's changes.
	Rollback() error
}

// SessionRunner executes statements on a single underlying connection and is
// returned by DB.Session. Closing it releases the pinned connection.
type SessionRunner interface {
	// Exec runs a statement that doesn't return rows.
	Exec(query string, args ...interface{}) (ExecResult, error)
	// Close releases the pinned connection.
	Close() error
}

// sqlConnSession is a SessionRunner backed by a single pooled *sql.Conn.
// All statements run on the same physical connection, so per-connection
// session state (MySQL FOREIGN_KEY_CHECKS, SQL_MODE, ...) persists.
type sqlConnSession struct {
	conn *sql.Conn
}

func (s *sqlConnSession) Exec(query string, args ...interface{}) (ExecResult, error) {
	res, err := s.conn.ExecContext(context.Background(), query, args...)
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec error: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{RowsAffected: affected}, nil
}

func (s *sqlConnSession) Close() error { return s.conn.Close() }

// sqlDBSession is a SessionRunner that delegates to a *sql.DB without pinning a
// single connection. It is used by drivers (e.g. SQLite with
// SetMaxOpenConns(1)) that already guarantee all statements run on one
// connection, so session state persists without — and must not — hold a
// dedicated connection that would starve the pool. Close is a no-op.
type sqlDBSession struct {
	db *sql.DB
}

func (s *sqlDBSession) Exec(query string, args ...interface{}) (ExecResult, error) {
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec error: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{RowsAffected: affected}, nil
}

func (s *sqlDBSession) Close() error { return nil }

// sqlTx adapts a *sql.Tx to the Tx interface. All three drivers share this
// since they wrap a *sql.DB underneath.
type sqlTx struct{ tx *sql.Tx }

func (t *sqlTx) Exec(query string, args ...interface{}) (ExecResult, error) {
	res, err := t.tx.Exec(query, args...)
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec error: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{RowsAffected: affected}, nil
}

func (t *sqlTx) Commit() error   { return t.tx.Commit() }
func (t *sqlTx) Rollback() error { return t.tx.Rollback() }

// beginTx starts a transaction on the given pool and adapts it to the Tx
// interface. Shared by all drivers since they wrap a *sql.DB.
func beginTx(database *sql.DB) (Tx, error) {
	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx: tx}, nil
}

// Column describes a single column in a table or result set.
type Column struct {
	Name string
	Type string
}

// ForeignKey describes an outbound foreign key on a table column.
type ForeignKey struct {
	Column    string
	RefTable  string
	RefColumn string
}

// Index describes a secondary index on a table. Columns holds the indexed
// columns (or expressions) in order. Partial is the index's filter clause
// (e.g. a PostgreSQL WHERE predicate); it is empty when the index is not
// partial or the driver does not expose the predicate.
type Index struct {
	Name    string
	Columns []string
	Unique  bool
	Partial string
}

// Trigger describes a database trigger on a table. Timing is BEFORE, AFTER, or
// INSTEAD OF; Event is INSERT, UPDATE, or DELETE. Statement holds the trigger
// body (or the full CREATE TRIGGER text) for display.
type Trigger struct {
	Name      string
	Timing    string
	Event     string
	Statement string
}

// TableColumnInfo describes column metadata needed for inserts and schema display.
type TableColumnInfo struct {
	Name          string
	Type          string
	NotNull       bool
	PrimaryKey    bool
	AutoIncrement bool
	HasDefault    bool
	DefaultValue  string
}

// Result holds the output of a query execution.
type Result struct {
	Columns []Column
	Rows    [][]string
	Message string
	Elapsed string
}

// ExecResult holds the output of a write operation (INSERT/UPDATE/DELETE).
type ExecResult struct {
	RowsAffected int64
}

// New creates a new database connection based on the driver type.
func New(cfg ConnectionConfig) (*Connection, error) {
	var database DB

	switch cfg.Driver {
	case DriverSQLite:
		database = NewSQLite(cfg)
	case DriverMySQL:
		database = NewMySQL(cfg)
	case DriverPostgres:
		database = NewPostgres(cfg)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
	}

	return &Connection{
		config: cfg,
		db:     database,
	}, nil
}

// Connect opens the underlying database connection.
func (c *Connection) Connect() error {
	return c.db.Connect()
}

// Close closes the underlying database connection.
func (c *Connection) Close() error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

// DB returns the underlying database interface for direct operations.
func (c *Connection) DB() DB {
	return c.db
}

// Config returns the connection configuration.
func (c *Connection) Config() ConnectionConfig {
	return c.config
}

// UseDatabase switches the active database on the underlying driver and keeps
// the wrapper's config in sync so that Config().Database always reflects the
// active database.
func (c *Connection) UseDatabase(name string) error {
	if err := c.db.UseDatabase(name); err != nil {
		return err
	}
	c.config.Database = name
	return nil
}

// executeRows runs a query against a *sql.DB with context support and builds a
// Result. It is shared by all drivers.
func executeRows(ctx context.Context, database *sql.DB, query string) (Result, error) {
	start := time.Now()

	rows, err := database.QueryContext(ctx, query)
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
