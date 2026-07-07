package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// SQLite implements the DB interface for SQLite databases.
type SQLite struct {
	config ConnectionConfig
	db     *sql.DB
}

// NewSQLite creates a new SQLite database handler.
func NewSQLite(cfg ConnectionConfig) *SQLite {
	return &SQLite{config: cfg}
}

func (s *SQLite) Connect() error {
	db, err := sql.Open("sqlite", s.config.Database)
	if err != nil {
		return fmt.Errorf("failed to open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s.db = db
	return db.Ping()
}

func (s *SQLite) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Databases returns the single configured database for SQLite.
func (s *SQLite) Databases() ([]string, error) {
	return []string{s.config.Database}, nil
}

// UseDatabase is not supported for single-file SQLite databases.
func (s *SQLite) UseDatabase(name string) error {
	return fmt.Errorf("switching databases is not supported for SQLite")
}

func (s *SQLite) Tables() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type IN ('table','view') ORDER BY name`)
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

func (s *SQLite) TableRowCounts() (map[string]int64, error) {
	tables, err := s.Tables()
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(tables))
	for _, t := range tables {
		var n int64
		err := s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, strings.ReplaceAll(t, `"`, `""`))).Scan(&n)
		if err == nil {
			counts[t] = n
		}
	}
	return counts, nil
}

func (s *SQLite) TableSchema(table string) ([]Column, error) {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []Column
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, Column{Name: name, Type: dataType})
	}
	return cols, rows.Err()
}

func (s *SQLite) PrimaryKeys(table string) ([]string, error) {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pks []string
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		if pk > 0 {
			pks = append(pks, name)
		}
	}
	return pks, rows.Err()
}

func (s *SQLite) ForeignKeys(table string) ([]ForeignKey, error) {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []ForeignKey
	for rows.Next() {
		var id, seq int
		var refTable, localCol, refCol, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &refTable, &localCol, &refCol, &onUpdate, &onDelete, &match); err != nil {
			return nil, err
		}
		if refCol == "" {
			refCol = "id"
		}
		fks = append(fks, ForeignKey{
			Column:    localCol,
			RefTable:  refTable,
			RefColumn: refCol,
		})
	}
	return fks, rows.Err()
}

func isIntegerType(typeName string) bool {
	t := strings.ToUpper(strings.TrimSpace(typeName))
	return strings.Contains(t, "INT")
}

func (s *SQLite) TableColumnInfo(table string) ([]TableColumnInfo, error) {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []TableColumnInfo
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, TableColumnInfo{
			Name:          name,
			Type:          dataType,
			NotNull:       notNull != 0,
			PrimaryKey:    pk != 0,
			AutoIncrement: pk != 0 && isIntegerType(dataType),
			HasDefault:    dfltValue.Valid,
			DefaultValue:  dfltValue.String,
		})
	}
	return cols, rows.Err()
}

func (s *SQLite) Execute(query string) (Result, error) {
	return s.ExecuteContext(context.Background(), query)
}

func (s *SQLite) ExecuteContext(ctx context.Context, query string) (Result, error) {
	return executeRows(ctx, s.db, query)
}

func (s *SQLite) Exec(query string, args ...interface{}) (ExecResult, error) {
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

// Session returns a runner for import. SQLite uses SetMaxOpenConns(1), so
// every statement already runs on the same connection and session state
// persists without — and must not — pinning a dedicated connection (which
// would starve the single-connection pool and deadlock concurrent UI queries
// during a long import).
func (s *SQLite) Session() (SessionRunner, error) {
	return &sqlDBSession{db: s.db}, nil
}
