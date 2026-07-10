package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Postgres implements the DB interface for PostgreSQL databases.
type Postgres struct {
	config ConnectionConfig
	db     *sql.DB
	tunnel *SSHTunnel
}

// NewPostgres creates a new PostgreSQL database handler.
func NewPostgres(cfg ConnectionConfig) *Postgres {
	return &Postgres{config: cfg}
}

func (p *Postgres) Connect() error {
	if p.config.SSHHost != "" {
		tunnel, err := NewSSHTunnel(p.config)
		if err != nil {
			return fmt.Errorf("ssh tunnel: %w", err)
		}
		p.tunnel = tunnel
	}

	config, err := p.connConfig()
	if err != nil {
		return err
	}

	if p.tunnel != nil {
		config.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return p.tunnel.DialContext(ctx, network, addr)
		}
	}

	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	p.db = db
	return db.Ping()
}

// connConfig builds a pgx.ConnConfig from the connection parameters. It uses
// pgx.ParseConfig on a keyword/value DSN so that quoting and escaping are
// handled correctly.
func (p *Postgres) connConfig() (*pgx.ConnConfig, error) {
	host := p.config.Host
	if host == "" {
		host = "localhost"
	}
	port := p.config.Port
	if port == 0 {
		port = 5432
	}

	var parts []string
	parts = append(parts, "host="+quoteDSNValue(host))
	parts = append(parts, fmt.Sprintf("port=%d", port))
	if p.config.Username != "" {
		parts = append(parts, "user="+quoteDSNValue(p.config.Username))
	}
	if p.config.Password != "" {
		parts = append(parts, "password="+quoteDSNValue(p.config.Password))
	}
	if p.config.Database != "" {
		parts = append(parts, "dbname="+quoteDSNValue(p.config.Database))
	}
	parts = append(parts, "sslmode=disable")
	if p.config.ReadOnly {
		// Sent in the startup packet so every pooled connection is read-only.
		parts = append(parts, "default_transaction_read_only=on")
	}

	dsn := strings.Join(parts, " ")
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}
	return config, nil
}

// quoteDSNValue wraps a DSN keyword value in single quotes if it contains
// characters that require quoting per the libpq keyword/value format.
func quoteDSNValue(v string) string {
	if v == "" {
		return "''"
	}
	if !strings.ContainsAny(v, " '\\") {
		return v
	}
	escaped := strings.ReplaceAll(v, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return "'" + escaped + "'"
}

func (p *Postgres) Close() error {
	if p.db != nil {
		p.db.Close()
	}
	if p.tunnel != nil {
		p.tunnel.Close()
	}
	return nil
}

// Databases returns all non-template databases accessible on the server.
func (p *Postgres) Databases() ([]string, error) {
	rows, err := p.db.Query(
		`SELECT datname FROM pg_database
		 WHERE datistemplate = false
		 ORDER BY datname`,
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

// UseDatabase switches to a different database by re-opening the connection
// pool. The SSH tunnel (if any) is preserved.
func (p *Postgres) UseDatabase(name string) error {
	p.config.Database = name
	if p.db != nil {
		p.db.Close()
	}

	config, err := p.connConfig()
	if err != nil {
		return err
	}
	if p.tunnel != nil {
		config.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return p.tunnel.DialContext(ctx, network, addr)
		}
	}

	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	p.db = db
	return db.Ping()
}

func (p *Postgres) Tables() ([]string, error) {
	rows, err := p.db.Query(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = current_schema()
		 ORDER BY table_name`,
	)
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

func (p *Postgres) TableRowCounts() (map[string]int64, error) {
	rows, err := p.db.Query(
		`SELECT relname, n_live_tup
		 FROM pg_stat_user_tables
		 ORDER BY relname`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		counts[name] = n
	}
	return counts, rows.Err()
}

func (p *Postgres) TableSchema(table string) ([]Column, error) {
	rows, err := p.db.Query(
		`SELECT column_name, data_type FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = $1
		 ORDER BY ordinal_position`,
		table,
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

func (p *Postgres) PrimaryKeys(table string) ([]string, error) {
	rows, err := p.db.Query(
		`SELECT kcu.column_name
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		   ON tc.constraint_name = kcu.constraint_name
		   AND tc.table_schema = kcu.table_schema
		 WHERE tc.table_schema = current_schema()
		   AND tc.table_name = $1
		   AND tc.constraint_type = 'PRIMARY KEY'
		 ORDER BY kcu.ordinal_position`,
		table,
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

func (p *Postgres) ForeignKeys(table string) ([]ForeignKey, error) {
	rows, err := p.db.Query(
		`SELECT kcu.column_name, ccu.table_name, ccu.column_name
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		   ON tc.constraint_name = kcu.constraint_name
		   AND tc.table_schema = kcu.table_schema
		 JOIN information_schema.constraint_column_usage ccu
		   ON tc.constraint_name = ccu.constraint_name
		 WHERE tc.table_schema = current_schema()
		   AND tc.table_name = $1
		   AND tc.constraint_type = 'FOREIGN KEY'
		 ORDER BY kcu.ordinal_position`,
		table,
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

func (p *Postgres) TableColumnInfo(table string) ([]TableColumnInfo, error) {
	rows, err := p.db.Query(
		`SELECT
		   c.column_name,
		   c.data_type,
		   c.is_nullable = 'NO',
		   c.column_default,
		   EXISTS(
		     SELECT 1 FROM information_schema.key_column_usage kcu
		     JOIN information_schema.table_constraints tc
		       ON kcu.constraint_name = tc.constraint_name
		       AND kcu.table_schema = tc.table_schema
		     WHERE tc.table_schema = current_schema()
		       AND tc.table_name = c.table_name
		       AND kcu.column_name = c.column_name
		       AND tc.constraint_type = 'PRIMARY KEY'
		   ) AS is_pk,
		   c.column_default LIKE 'nextval(%' AS is_serial
		 FROM information_schema.columns c
		 WHERE c.table_schema = current_schema() AND c.table_name = $1
		 ORDER BY c.ordinal_position`,
		table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []TableColumnInfo
	for rows.Next() {
		var name, dataType string
		var notNull, isPK, isSerial bool
		var colDefault sql.NullString
		if err := rows.Scan(&name, &dataType, &notNull, &colDefault, &isPK, &isSerial); err != nil {
			return nil, err
		}
		cols = append(cols, TableColumnInfo{
			Name:          name,
			Type:          dataType,
			NotNull:       notNull,
			PrimaryKey:    isPK,
			AutoIncrement: isSerial,
			HasDefault:    colDefault.Valid,
			DefaultValue:  colDefault.String,
		})
	}
	return cols, rows.Err()
}

func (p *Postgres) Execute(query string) (Result, error) {
	return p.ExecuteContext(context.Background(), query)
}

func (p *Postgres) ExecuteContext(ctx context.Context, query string) (Result, error) {
	if err := rejectWriteIfReadOnly(p.config, query); err != nil {
		return Result{}, err
	}
	return executeRows(ctx, p.db, query)
}

func (p *Postgres) Exec(query string, args ...interface{}) (ExecResult, error) {
	if err := rejectWriteIfReadOnly(p.config, query); err != nil {
		return ExecResult{}, err
	}
	res, err := p.db.Exec(query, args...)
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
// session-scoped settings (SET search_path, SET constraint_exclusion, ...)
// set by a dump persist across statements.
func (p *Postgres) Session() (SessionRunner, error) {
	if p.config.ReadOnly {
		return nil, ErrReadOnly
	}
	conn, err := p.db.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	return &sqlConnSession{conn: conn}, nil
}

func (p *Postgres) Begin() (Tx, error) {
	if p.config.ReadOnly {
		return nil, ErrReadOnly
	}
	return beginTx(p.db)
}

// Indexes returns the secondary indexes on a table from pg_index. The primary
// key (indisprimary) is excluded. Columns and the partial-index predicate are
// extracted from the human-readable pg_get_indexdef output; unique comes from
// indisunique.
func (p *Postgres) Indexes(table string) ([]Index, error) {
	rows, err := p.db.Query(
		`SELECT c.relname AS index_name,
		        pg_get_indexdef(i.indexrelid) AS indexdef,
		        i.indisunique,
		        i.indisprimary
		 FROM pg_index i
		 JOIN pg_class c ON c.oid = i.indexrelid
		 JOIN pg_class t ON t.oid = i.indrelid
		 JOIN pg_namespace n ON n.oid = t.relnamespace
		 WHERE t.relname = $1 AND n.nspname = current_schema()
		 ORDER BY c.relname`,
		table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var idxs []Index
	for rows.Next() {
		var name, indexdef string
		var unique, primary bool
		if err := rows.Scan(&name, &indexdef, &unique, &primary); err != nil {
			return nil, err
		}
		if primary {
			continue
		}
		cols, partial := parsePostgresIndexDef(indexdef)
		idxs = append(idxs, Index{
			Name:    name,
			Columns: cols,
			Unique:  unique,
			Partial: partial,
		})
	}
	return idxs, rows.Err()
}

// parsePostgresIndexDef extracts the indexed column list and any partial WHERE
// predicate from a CREATE INDEX statement produced by pg_get_indexdef. The
// first parenthesized group after the table name is the column list; a trailing
// WHERE clause (case-insensitive) is the partial predicate. Parsing is lenient:
// on any surprise the whole definition is returned as a single "column" so the
// index is still listed.
func parsePostgresIndexDef(indexdef string) (columns []string, partial string) {
	open := strings.Index(indexdef, "(")
	if open < 0 {
		return []string{indexdef}, ""
	}
	// Find the matching close paren (no nested parens in plain column lists;
	// expression indexes may nest, so balance).
	depth := 0
	close := -1
	for i := open; i < len(indexdef); i++ {
		switch indexdef[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = i
			}
		}
		if close >= 0 {
			break
		}
	}
	if close < 0 {
		return []string{indexdef}, ""
	}
	inner := strings.TrimSpace(indexdef[open+1 : close])
	for _, c := range splitTopLevelCommas(inner) {
		columns = append(columns, strings.TrimSpace(c))
	}
	if columns == nil {
		columns = []string{inner}
	}

	// Look for a WHERE after the close paren.
	rest := strings.TrimSpace(indexdef[close+1:])
	if len(rest) >= 5 && strings.EqualFold(rest[:5], "WHERE") {
		partial = strings.TrimSpace(rest[5:])
	}
	return columns, partial
}

// splitTopLevelCommas splits on commas that are not nested inside parens, so
// expression indexes like "(a+b), c" split correctly.
func splitTopLevelCommas(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// Triggers returns user-defined triggers from pg_trigger. Internal triggers
// (tgisinternal, e.g. those created for foreign keys) are excluded. The full
// CREATE TRIGGER text from pg_get_triggerdef is parsed for timing/event and
// kept as the statement body.
func (p *Postgres) Triggers(table string) ([]Trigger, error) {
	rows, err := p.db.Query(
		`SELECT t.tgname, pg_get_triggerdef(t.oid)
		 FROM pg_trigger t
		 JOIN pg_class c ON c.oid = t.tgrelid
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relname = $1 AND n.nspname = current_schema()
		   AND NOT t.tgisinternal
		 ORDER BY t.tgname`,
		table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var triggers []Trigger
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return nil, err
		}
		timing, event := parsePostgresTriggerDef(def)
		triggers = append(triggers, Trigger{
			Name:      name,
			Timing:    timing,
			Event:     event,
			Statement: def,
		})
	}
	return triggers, rows.Err()
}

// parsePostgresTriggerDef extracts the timing and event from a pg_get_triggerdef
// string of the form "TRIGGER name BEFORE UPDATE ON tbl ...".
func parsePostgresTriggerDef(def string) (timing, event string) {
	for _, t := range []string{"INSTEAD OF", "BEFORE", "AFTER"} {
		idx := strings.Index(strings.ToUpper(def), " "+t+" ")
		if idx < 0 {
			continue
		}
		rest := strings.TrimLeft(def[idx+1+len(t):], " \t\r\n")
		for _, ev := range []string{"INSERT", "UPDATE", "DELETE", "TRUNCATE"} {
			if _, ok := matchWord(rest, ev); ok {
				return t, ev
			}
		}
	}
	return "", ""
}

// ViewDefinition returns the pretty-printed definition of a view from
// pg_views, or "" if the named relation is not a view.
func (p *Postgres) ViewDefinition(view string) (string, error) {
	var def sql.NullString
	err := p.db.QueryRow(
		`SELECT definition FROM pg_views
		 WHERE schemaname = current_schema() AND viewname = $1`,
		view,
	).Scan(&def)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return def.String, nil
}
