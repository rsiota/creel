package db

import (
	"fmt"
)

// Driver represents a supported database driver type.
type Driver string

const (
	DriverSQLite Driver = "sqlite"
	DriverMySQL  Driver = "mysql"
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
	// Exec runs a statement that doesn't return rows (INSERT, UPDATE, DELETE).
	Exec(query string, args ...interface{}) (ExecResult, error)
	// Databases returns the list of databases accessible through this connection.
	// For single-file drivers (e.g. SQLite) this returns the configured database only.
	Databases() ([]string, error)
	// UseDatabase switches the active database, re-opening the connection if needed.
	UseDatabase(name string) error
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

// TableColumnInfo describes column metadata needed for inserts.
type TableColumnInfo struct {
	Name          string
	Type          string
	NotNull       bool
	PrimaryKey    bool
	AutoIncrement bool
	HasDefault    bool
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
