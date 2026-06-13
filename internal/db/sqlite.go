package db

import (
	"database/sql"
	"fmt"
	"time"

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

func (s *SQLite) Execute(query string) (Result, error) {
	start := time.Now()

	rows, err := s.db.Query(query)
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
