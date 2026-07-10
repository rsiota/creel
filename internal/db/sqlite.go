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

func (s *SQLite) Begin() (Tx, error) { return beginTx(s.db) }

// Indexes returns the indexes created on a table via PRAGMA index_list /
// index_info. The primary-key auto-index (origin 'pk') is excluded since the
// PK is shown in its own section; unique-constraint indexes (origin 'u') are
// included because they are real user-visible indexes.
//
// SQLite connections are limited to MaxOpenConns(1), so the index_list and
// index_info cursors must not be open simultaneously — the inner query would
// wait forever for the single connection held by the outer cursor. We collect
// the index_list rows into memory first, close that cursor, then query each
// index's columns in turn.
func (s *SQLite) Indexes(table string) ([]Index, error) {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA index_list("%s")`, table))
	if err != nil {
		return nil, err
	}

	type rawIndex struct {
		name   string
		unique bool
		origin string
	}
	var raw []rawIndex
	for rows.Next() {
		var seq, unique int
		var name, origin string
		var partial int
		// PRAGMA index_list columns: seq, name, unique, origin, partial
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			rows.Close()
			return nil, err
		}
		if origin == "pk" {
			continue
		}
		raw = append(raw, rawIndex{name: name, unique: unique != 0, origin: origin})
	}
	if cerr := rows.Close(); cerr != nil {
		return nil, cerr
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	idxs := make([]Index, 0, len(raw))
	for _, r := range raw {
		cols, err := s.indexColumns(r.name)
		if err != nil {
			return nil, err
		}
		idxs = append(idxs, Index{
			Name:    r.name,
			Columns: cols,
			Unique:  r.unique,
		})
	}
	return idxs, nil
}

// indexColumns returns the column names for a SQLite index.
func (s *SQLite) indexColumns(indexName string) ([]string, error) {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA index_info("%s")`, indexName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var seqno, cid int
		var name string
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// Triggers returns triggers from sqlite_master. The timing/event are parsed
// from the trigger SQL; the full SQL is kept as the statement body.
func (s *SQLite) Triggers(table string) ([]Trigger, error) {
	rows, err := s.db.Query(
		`SELECT name, sql FROM sqlite_master WHERE type='trigger' AND tbl_name = ? ORDER BY name`,
		table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var triggers []Trigger
	for rows.Next() {
		var name string
		var sqlStr sql.NullString
		if err := rows.Scan(&name, &sqlStr); err != nil {
			return nil, err
		}
		timing, event := parseSQLiteTriggerTiming(sqlStr.String)
		triggers = append(triggers, Trigger{
			Name:      name,
			Timing:    timing,
			Event:     event,
			Statement: sqlStr.String,
		})
	}
	return triggers, rows.Err()
}

// parseSQLiteTriggerTiming extracts the timing (BEFORE/AFTER/INSTEAD OF) and
// event (INSERT/UPDATE/DELETE) from a CREATE TRIGGER statement. It is
// intentionally lenient: malformed input yields empty strings rather than an
// error so a trigger is always listed even if its SQL is unusual.
func parseSQLiteTriggerTiming(createSQL string) (timing, event string) {
	// Find the timing keyword first, then the event that follows it.
	for _, t := range []string{"INSTEAD OF", "BEFORE", "AFTER"} {
		idx := strings.Index(strings.ToUpper(createSQL), t)
		if idx < 0 {
			continue
		}
		rest := strings.TrimLeft(createSQL[idx+len(t):], " \t\r\n")
		for _, ev := range []string{"INSERT", "UPDATE", "DELETE"} {
			if _, ok := matchWord(rest, ev); ok {
				return t, ev
			}
		}
	}
	return "", ""
}

// matchWord reports whether s starts with word (case-insensitive), after
// optional whitespace, returning the remainder.
func matchWord(s, word string) (string, bool) {
	s = strings.TrimLeft(s, " \t\r\n")
	if len(s) >= len(word) && strings.EqualFold(s[:len(word)], word) {
		rest := s[len(word):]
		if rest == "" || !isLetterDigit(rest[0]) {
			return rest, true
		}
	}
	return s, false
}

func isLetterDigit(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// ViewDefinition returns the CREATE VIEW SQL for a view, or "" if the named
// relation is not a view.
func (s *SQLite) ViewDefinition(view string) (string, error) {
	var sqlStr sql.NullString
	err := s.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='view' AND name=?`,
		view,
	).Scan(&sqlStr)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return sqlStr.String, nil
}
