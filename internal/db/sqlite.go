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
	if err := db.Ping(); err != nil {
		return err
	}
	// query_only makes SQLite reject writes at the engine level. With
	// MaxOpenConns(1) a single PRAGMA covers the whole connection.
	if s.config.ReadOnly {
		if _, err := db.Exec("PRAGMA query_only = ON"); err != nil {
			return fmt.Errorf("failed to enable read-only: %w", err)
		}
	}
	return nil
}

func (s *SQLite) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Ping verifies the SQLite connection is still usable.
func (s *SQLite) Ping() error {
	if s.db == nil {
		return fmt.Errorf("not connected")
	}
	return s.db.Ping()
}

// Databases returns the single configured database for SQLite.
func (s *SQLite) Databases() ([]string, error) {
	return []string{s.config.Database}, nil
}

// UseDatabase is not supported for single-file SQLite databases.
func (s *SQLite) UseDatabase(name string) error {
	return fmt.Errorf("switching databases is not supported for SQLite")
}

// Schemas is not supported for SQLite.
func (s *SQLite) Schemas() ([]string, error) {
	return nil, fmt.Errorf("schemas are not supported for SQLite")
}

// UseSchema is not supported for SQLite.
func (s *SQLite) UseSchema(name string) error {
	return fmt.Errorf("schemas are not supported for SQLite")
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

// Views returns SQLite views (type='view' in sqlite_master).
func (s *SQLite) Views() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type = 'view' ORDER BY name`)
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

// ReferencingForeignKeys returns FKs pointing AT the given table (the reverse
// of ForeignKeys). SQLite has no reverse-FK pragma, so every table's
// foreign_key_list is scanned and rows whose referenced table matches are
// collected. The match is case-insensitive to survive differing casing
// between a user-typed name and the FK definition. O(tables) pragmas — fine
// for typical schemas.
func (s *SQLite) ReferencingForeignKeys(table string) ([]Referrer, error) {
	tr, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	var tables []string
	for tr.Next() {
		var name string
		if err := tr.Scan(&name); err != nil {
			tr.Close()
			return nil, err
		}
		tables = append(tables, name)
	}
	tr.Close()
	if err := tr.Err(); err != nil {
		return nil, err
	}

	var refs []Referrer
	for _, t := range tables {
		fr, err := s.db.Query(fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, t))
		if err != nil {
			return nil, err
		}
		for fr.Next() {
			var id, seq int
			var refTable, localCol, refCol, onUpdate, onDelete, match string
			if err := fr.Scan(&id, &seq, &refTable, &localCol, &refCol, &onUpdate, &onDelete, &match); err != nil {
				fr.Close()
				return nil, err
			}
			if strings.EqualFold(refTable, table) {
				if refCol == "" {
					refCol = "id"
				}
				refs = append(refs, Referrer{Table: t, Column: localCol, RefColumn: refCol})
			}
		}
		fr.Close()
		if err := fr.Err(); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

// Uses returns objects (views, triggers) whose definitions reference the given
// table, via a textual scan of sqlite_master. SQLite has no stored functions
// or procedures, so those are not considered.
func (s *SQLite) Uses(table string) ([]Usage, error) {
	var defs []Usage

	// Views and triggers: the sql column holds the full CREATE statement.
	for _, kind := range []string{"view", "trigger"} {
		rr, err := s.db.Query(`SELECT name, sql FROM sqlite_master WHERE type = ?`, kind)
		if err != nil {
			return nil, fmt.Errorf("uses: %ss: %w", kind, err)
		}
		if err := scanNameBody(rr, kind, &defs); err != nil {
			rr.Close()
			return nil, err
		}
		rr.Close()
	}

	return definitionsReferencing(defs, table), nil
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
	if err := rejectWriteIfReadOnly(s.config, query); err != nil {
		return Result{}, err
	}
	return executeRows(ctx, s.db, query)
}

func (s *SQLite) Exec(query string, args ...interface{}) (ExecResult, error) {
	if err := rejectWriteIfReadOnly(s.config, query); err != nil {
		return ExecResult{}, err
	}
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
	if s.config.ReadOnly {
		return nil, ErrReadOnly
	}
	return &sqlDBSession{db: s.db}, nil
}

func (s *SQLite) Begin(level IsolationLevel) (Tx, error) {
	if s.config.ReadOnly {
		return nil, ErrReadOnly
	}
	return beginTx(s.db, level)
}

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
		name    string
		unique  bool
		origin  string
		partial bool
	}
	var raw []rawIndex
	for rows.Next() {
		var seq, unique int
		var name, origin string
		var partial int
		// PRAGMA index_list columns: seq, name, unique, origin, partial.
		// PRAGMA only reports a 0/1 partial flag; the actual predicate is
		// parsed from sqlite_master.sql below (see indexPartialPredicate).
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			rows.Close()
			return nil, err
		}
		if origin == "pk" {
			continue
		}
		raw = append(raw, rawIndex{name: name, unique: unique != 0, origin: origin, partial: partial != 0})
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
		ix := Index{
			Name:    r.name,
			Columns: cols,
			Unique:  r.unique,
		}
		if r.partial {
			pred, err := s.indexPartialPredicate(r.name)
			if err != nil {
				return nil, err
			}
			ix.Partial = pred
		}
		idxs = append(idxs, ix)
	}
	return idxs, nil
}

// indexPartialPredicate returns the WHERE predicate of a partial index by
// parsing its CREATE INDEX text from sqlite_master. It mirrors ViewDefinition;
// the caller only invokes this when PRAGMA index_list reports partial=1, which
// implies a user-created partial index. Auto-indexes (origin "u"/"pk") have
// NULL sql, so they are never queried here.
func (s *SQLite) indexPartialPredicate(indexName string) (string, error) {
	var sqlStr sql.NullString
	err := s.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`,
		indexName,
	).Scan(&sqlStr)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return splitIndexWhere(sqlStr.String), nil
}

// splitIndexWhere extracts the partial-index predicate (the text after a
// top-level WHERE keyword) from a CREATE INDEX statement, or "" when it has
// none. Parentheses are balanced so a WHERE-like token inside an expression
// index's column list is not mistaken for the predicate separator, and word
// boundaries on both sides keep the keyword out of identifiers.
func splitIndexWhere(createIndex string) string {
	const kw = "WHERE"
	depth := 0
	for i := 0; i+len(kw) <= len(createIndex); i++ {
		switch createIndex[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 {
			continue
		}
		if !strings.EqualFold(createIndex[i:i+len(kw)], kw) {
			continue
		}
		if i > 0 && isLetterDigit(createIndex[i-1]) {
			continue
		}
		if i+len(kw) < len(createIndex) && isLetterDigit(createIndex[i+len(kw)]) {
			continue
		}
		return strings.TrimSpace(createIndex[i+len(kw):])
	}
	return ""
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

// TableDefinition returns the original CREATE TABLE/VIEW SQL from sqlite_master,
// or "" if the relation has no stored DDL (e.g. sqlite_sequence).
func (s *SQLite) TableDefinition(table string) (string, error) {
	var sqlStr sql.NullString
	err := s.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE name=? AND type IN ('table','view')`,
		table,
	).Scan(&sqlStr)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(strings.TrimSpace(sqlStr.String), ";"), nil
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

// CheckConstraints returns CHECK constraints parsed from the table's CREATE
// statement in sqlite_master. SQLite has no catalog view for checks (PRAGMA
// only lists columns/indexes/FKs), so the DDL is parsed with a
// literal/comment-aware scanner. Column-level checks are associated with the
// column that opens their definition; table-level checks carry only a name
// when `CONSTRAINT name` introduces them. Parsing is lenient — malformed DDL
// yields a partial (possibly empty) slice rather than an error so a table is
// always listed.
func (s *SQLite) CheckConstraints(table string) ([]CheckConstraint, error) {
	var sqlStr sql.NullString
	err := s.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`,
		table,
	).Scan(&sqlStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseSQLiteCheckConstraints(sqlStr.String), nil
}

// parseSQLiteCheckConstraints extracts CHECK constraints from a CREATE TABLE
// statement. It is literal- and comment-aware (skipping '...' strings,
// "..."/`...`/[...] quoted identifiers, -- line comments, and /* */ block
// comments) and paren-depth-aware so a CHECK expression may itself contain
// parentheses and commas. Column-level checks are associated with the column
// name that opens their definition; table-level checks (including those named
// via CONSTRAINT) carry only a name when one is present.
func parseSQLiteCheckConstraints(createSQL string) []CheckConstraint {
	body := sqliteColumnListBody(createSQL)
	if body == "" {
		return nil
	}
	var out []CheckConstraint
	for _, entry := range sqliteTopLevelEntries(body) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, rest := sqliteConsumeConstraintName(entry)
		leading, after, ok := sqliteLeadingIdentifier(rest)
		if !ok {
			continue
		}
		switch strings.ToUpper(leading) {
		case "PRIMARY", "UNIQUE", "FOREIGN", "KEY":
			// A table constraint that is not a CHECK; skip.
			continue
		}
		expr := sqliteFindCheck(rest)
		if expr == "" {
			continue
		}
		if strings.EqualFold(leading, "CHECK") {
			// Table-level (possibly CONSTRAINT-named) check.
			out = append(out, CheckConstraint{Name: name, Expression: expr})
			continue
		}
		// Column-level check: leading identifier is the column name. Drop a
		// table-level name (only meaningful with CONSTRAINT).
		out = append(out, CheckConstraint{Column: leading, Expression: expr})
		_ = after
	}
	return out
}

// sqliteColumnListBody returns the text between the outer parentheses of a
// CREATE TABLE statement (the column/constraint list). A CREATE TABLE ... AS
// SELECT form has no such list and yields "". The scan skips literals and
// comments so a stray '(' inside them does not fool it.
func sqliteColumnListBody(s string) string {
	i := 0
	for i < len(s) {
		if end, skipped := sqliteSkipLiteral(s, i); skipped {
			i = end
			continue
		}
		if s[i] == '(' {
			// CREATE TABLE x AS SELECT ... has no column-list parens; the first
			// '(' would belong to the SELECT, and " AS " precedes it.
			if strings.Contains(strings.ToUpper(s[:i]), " AS ") {
				return ""
			}
			close, ok := sqliteScanBalanced(s, i)
			if !ok {
				return ""
			}
			return s[i+1 : close]
		}
		i++
	}
	return ""
}

// sqliteTopLevelEntries splits a column-list body at commas that sit at the
// body's own paren depth (depth 0), preserving nested commas inside CHECK
// expressions, function calls, and parenthesized types.
func sqliteTopLevelEntries(body string) []string {
	var entries []string
	depth := 0
	start := 0
	i := 0
	for i < len(body) {
		if end, skipped := sqliteSkipLiteral(body, i); skipped {
			i = end
			continue
		}
		switch body[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				entries = append(entries, body[start:i])
				start = i + 1
			}
		}
		i++
	}
	entries = append(entries, body[start:])
	return entries
}

// sqliteFindCheck scans s for the word CHECK (case-insensitive, whole-word
// bounded) followed by a parenthesized group and returns the group's inner
// text, trimmed. Returns "" when no check is found.
func sqliteFindCheck(s string) string {
	const needle = "check"
	i := 0
	for i < len(s) {
		if end, skipped := sqliteSkipLiteral(s, i); skipped {
			i = end
			continue
		}
		if i+len(needle) <= len(s) && strings.EqualFold(s[i:i+len(needle)], needle) {
			beforeOk := i == 0 || !isLetterDigit(s[i-1])
			afterIdx := i + len(needle)
			afterOk := afterIdx >= len(s) || !isLetterDigit(s[afterIdx])
			if beforeOk && afterOk {
				j := afterIdx
				for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
					j++
				}
				if j < len(s) && s[j] == '(' {
					if close, ok := sqliteScanBalanced(s, j); ok {
						return strings.TrimSpace(s[j+1 : close])
					}
				}
			}
		}
		i++
	}
	return ""
}

// sqliteSkipLeadingNoise trims leading whitespace and any comments that
// follow it, repeatedly, so a leading `-- ...` or `/* ... */` before a real
// token does not fool the identifier parser. It intentionally does NOT skip
// string literals or quoted identifiers — those may be the actual column
// name that opens the entry.
func sqliteSkipLeadingNoise(s string) string {
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		if len(s) >= 2 && s[0] == '-' && s[1] == '-' {
			j := 2
			for j < len(s) && s[j] != '\n' {
				j++
			}
			s = s[j:]
			continue
		}
		if len(s) >= 2 && s[0] == '/' && s[1] == '*' {
			j := 2
			for j+1 < len(s) && !(s[j] == '*' && s[j+1] == '/') {
				j++
			}
			if j+1 < len(s) {
				s = s[j+2:]
			} else {
				s = ""
			}
			continue
		}
		return s
	}
}

// sqliteConsumeConstraintName peels a leading `CONSTRAINT name` from an entry,
// returning the name and the remainder. If the entry does not start with
// CONSTRAINT, name is "" and rest is the noise-trimmed entry.
func sqliteConsumeConstraintName(entry string) (name, rest string) {
	rest = sqliteSkipLeadingNoise(entry)
	id, after, ok := sqliteLeadingIdentifier(rest)
	if !ok || !strings.EqualFold(id, "CONSTRAINT") {
		return "", rest
	}
	rest = sqliteSkipLeadingNoise(after)
	nm, after2, ok2 := sqliteLeadingIdentifier(rest)
	if !ok2 {
		return "", rest
	}
	return nm, sqliteSkipLeadingNoise(after2)
}

// sqliteLeadingIdentifier returns the first identifier at the start of s
// (after skipping whitespace and comments), handling double-quoted, backtick,
// and [...] quoted forms by returning their unquoted text. ok is false when no
// identifier starts s.
func sqliteLeadingIdentifier(s string) (id, rest string, ok bool) {
	s = sqliteSkipLeadingNoise(s)
	if s == "" {
		return "", s, false
	}
	switch s[0] {
	case '"', '`':
		q := s[0]
		j := 1
		for j < len(s) {
			if s[j] == q {
				if j+1 < len(s) && s[j+1] == q {
					j += 2
					continue
				}
				return s[1:j], s[j+1:], true
			}
			j++
		}
		return s[1:j], s[j:], true
	case '[':
		j := 1
		for j < len(s) && s[j] != ']' {
			j++
		}
		if j < len(s) {
			return s[1:j], s[j+1:], true
		}
		return s[1:j], s[j:], true
	}
	j := 0
	for j < len(s) && isLetterDigit(s[j]) {
		j++
	}
	if j == 0 {
		return "", s, false
	}
	return s[:j], s[j:], true
}

// sqliteScanBalanced returns the index of the ')' matching the '(' at open,
// skipping literals and comments. ok is false if it is unbalanced.
func sqliteScanBalanced(s string, open int) (int, bool) {
	if open < 0 || open >= len(s) || s[open] != '(' {
		return -1, false
	}
	depth := 0
	i := open
	for i < len(s) {
		if end, skipped := sqliteSkipLiteral(s, i); skipped {
			i = end
			continue
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
		i++
	}
	return -1, false
}

// sqliteSkipLiteral reports whether a string literal, quoted identifier, or
// SQL comment begins at i; if so it returns the index just past it.
func sqliteSkipLiteral(s string, i int) (end int, skipped bool) {
	if i >= len(s) {
		return i, false
	}
	switch s[i] {
	case '\'': // '' escapes a single quote
		j := i + 1
		for j < len(s) {
			if s[j] == '\'' {
				if j+1 < len(s) && s[j+1] == '\'' {
					j += 2
					continue
				}
				return j + 1, true
			}
			j++
		}
		return j, true
	case '"', '`': // quoted identifier; doubled-quote escape
		q := s[i]
		j := i + 1
		for j < len(s) {
			if s[j] == q {
				if j+1 < len(s) && s[j+1] == q {
					j += 2
					continue
				}
				return j + 1, true
			}
			j++
		}
		return j, true
	case '[': // SQLite bracketed identifier
		j := i + 1
		for j < len(s) && s[j] != ']' {
			j++
		}
		if j < len(s) {
			return j + 1, true
		}
		return j, true
	case '-':
		if i+1 < len(s) && s[i+1] == '-' { // -- line comment
			j := i + 2
			for j < len(s) && s[j] != '\n' {
				j++
			}
			return j, true
		}
	case '/':
		if i+1 < len(s) && s[i+1] == '*' { // /* block comment */
			j := i + 2
			for j+1 < len(s) && !(s[j] == '*' && s[j+1] == '/') {
				j++
			}
			if j+1 < len(s) {
				return j + 2, true
			}
			return len(s), true
		}
	}
	return i, false
}
