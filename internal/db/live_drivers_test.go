package db

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// liveDBEnabled is set in CI (and optionally locally) to exercise real
// MySQL/Postgres servers. Without it, live tests Skip so `go test ./...`
// stays offline-friendly.
func liveDBEnabled() bool {
	return os.Getenv("CREEL_LIVE_DB") == "1"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envPort(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func livePostgresConfig() ConnectionConfig {
	return ConnectionConfig{
		Driver:   DriverPostgres,
		Host:     envOr("CREEL_PG_HOST", "127.0.0.1"),
		Port:     envPort("CREEL_PG_PORT", 5432),
		Username: envOr("CREEL_PG_USER", "postgres"),
		Password: envOr("CREEL_PG_PASSWORD", "postgres"),
		Database: envOr("CREEL_PG_DATABASE", "postgres"),
		SSLMode:  "disable",
	}
}

func liveMySQLConfig() ConnectionConfig {
	return ConnectionConfig{
		Driver:   DriverMySQL,
		Host:     envOr("CREEL_MYSQL_HOST", "127.0.0.1"),
		Port:     envPort("CREEL_MYSQL_PORT", 3306),
		Username: envOr("CREEL_MYSQL_USER", "root"),
		Password: envOr("CREEL_MYSQL_PASSWORD", "root"),
		Database: envOr("CREEL_MYSQL_DATABASE", "creel"),
		SSLMode:  "disable",
	}
}

func liveConnect(t *testing.T, cfg ConnectionConfig) *Connection {
	t.Helper()
	if !liveDBEnabled() {
		t.Skip("set CREEL_LIVE_DB=1 to run live MySQL/Postgres tests")
	}
	if cfg.Driver == DriverMySQL && cfg.Database != "" {
		if err := ensureMySQLDatabase(cfg); err != nil {
			if os.Getenv("CI") == "" {
				t.Skipf("live mysql unavailable: %v", err)
			}
			t.Fatalf("ensure mysql database %q: %v", cfg.Database, err)
		}
	}
	conn, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Connect(); err != nil {
		// CI must have both services; locally allow running just the driver
		// that is actually listening.
		if os.Getenv("CI") == "" {
			t.Skipf("live %s unavailable: %v", cfg.Driver, err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// ensureMySQLDatabase creates cfg.Database when the server is reachable but
// the schema is missing (CI's mysql image creates it; a local DBngin/Homebrew
// server often does not).
func ensureMySQLDatabase(cfg ConnectionConfig) error {
	want := cfg.Database
	cfg.Database = ""
	conn, err := New(cfg)
	if err != nil {
		return err
	}
	if err := conn.Connect(); err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.DB().Exec("CREATE DATABASE IF NOT EXISTS " + quoteIdent(DriverMySQL, want))
	return err
}

func mustExec(t *testing.T, d DB, q string) {
	t.Helper()
	if _, err := d.Exec(q); err != nil {
		t.Fatalf("exec %s: %v", q, err)
	}
}

func hasString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestLivePostgresConnectAndSchema(t *testing.T) {
	conn := liveConnect(t, livePostgresConfig())
	d := conn.DB()
	if err := d.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	dbs, err := d.Databases()
	if err != nil {
		t.Fatalf("databases: %v", err)
	}
	if len(dbs) == 0 {
		t.Fatal("expected at least one database")
	}
	if err := conn.EnsureActiveSchema(); err != nil {
		t.Fatal(err)
	}
	if got := conn.Config().Schema; got != "public" {
		t.Fatalf("Schema = %q, want public", got)
	}
}

func TestLiveMySQLConnectAndTables(t *testing.T) {
	conn := liveConnect(t, liveMySQLConfig())
	d := conn.DB()
	if err := d.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	// Create a throwaway table so Tables() is non-empty on a fresh service DB.
	mustExec(t, d, `CREATE TABLE IF NOT EXISTS creel_live_smoke (
		id INT PRIMARY KEY,
		name VARCHAR(32) NOT NULL
	)`)
	t.Cleanup(func() { _, _ = d.Exec(`DROP TABLE IF EXISTS creel_live_smoke`) })
	tables, err := d.Tables()
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	if !hasString(tables, "creel_live_smoke") {
		t.Fatalf("expected creel_live_smoke in %v", tables)
	}
}

// TestLivePostgresCatalog builds a throwaway schema and exercises the catalog
// APIs that the TUI uses for structure, ERD, :locks/:who, and EXPLAIN — including
// the foreign-schema path (qualified names without UseSchema).
func TestLivePostgresCatalog(t *testing.T) {
	conn := liveConnect(t, livePostgresConfig())
	d := conn.DB()

	mustExec(t, d, `DROP SCHEMA IF EXISTS creel_live CASCADE`)
	mustExec(t, d, `CREATE SCHEMA creel_live`)
	t.Cleanup(func() {
		if conn.Config().Schema == "creel_live" {
			_ = d.UseSchema("public")
		}
		_, _ = d.Exec(`DROP SCHEMA IF EXISTS creel_live CASCADE`)
	})

	mustExec(t, d, `CREATE TABLE creel_live.parent (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('open', 'closed'))
	)`)
	mustExec(t, d, `CREATE TABLE creel_live.child (
		id INTEGER PRIMARY KEY,
		parent_id INTEGER NOT NULL REFERENCES creel_live.parent(id),
		note TEXT
	)`)
	mustExec(t, d, `CREATE UNIQUE INDEX parent_name_idx ON creel_live.parent(name)`)
	mustExec(t, d, `CREATE INDEX parent_open_idx ON creel_live.parent(status) WHERE status = 'open'`)
	mustExec(t, d, `CREATE INDEX child_parent_idx ON creel_live.child(parent_id)`)
	mustExec(t, d, `CREATE VIEW creel_live.active_parent AS
		SELECT id, name FROM creel_live.parent WHERE status = 'open'`)
	mustExec(t, d, `CREATE FUNCTION creel_live.noop() RETURNS trigger AS $$
		BEGIN RETURN NEW; END;
	$$ LANGUAGE plpgsql`)
	mustExec(t, d, `CREATE TRIGGER parent_trg
		AFTER UPDATE ON creel_live.parent
		FOR EACH ROW EXECUTE FUNCTION creel_live.noop()`)

	schemas, err := d.Schemas()
	if err != nil {
		t.Fatalf("Schemas: %v", err)
	}
	if !hasString(schemas, "creel_live") {
		t.Fatalf("Schemas() missing creel_live: %v", schemas)
	}

	// Foreign-schema path: stay in public, read creel_live via qualified names.
	inSchema, err := d.TablesInSchema("creel_live")
	if err != nil {
		t.Fatalf("TablesInSchema: %v", err)
	}
	for _, name := range []string{"parent", "child", "active_parent"} {
		if !hasString(inSchema, name) {
			t.Fatalf("TablesInSchema missing %q: %v", name, inSchema)
		}
	}

	cols, err := d.TableSchemaInSchema("creel_live", "parent")
	if err != nil {
		t.Fatalf("TableSchemaInSchema: %v", err)
	}
	if !hasColumn(cols, "id") || !hasColumn(cols, "status") {
		t.Fatalf("TableSchemaInSchema columns = %v", cols)
	}
	qcols, err := d.TableSchema("creel_live.parent")
	if err != nil {
		t.Fatalf("TableSchema(qualified): %v", err)
	}
	if len(qcols) != len(cols) {
		t.Fatalf("TableSchema(qualified) = %v, want same as InSchema %v", qcols, cols)
	}

	pks, err := d.PrimaryKeys("creel_live.parent")
	if err != nil {
		t.Fatalf("PrimaryKeys: %v", err)
	}
	if !equalStrings(pks, []string{"id"}) {
		t.Fatalf("PrimaryKeys = %v, want [id]", pks)
	}

	fks, err := d.ForeignKeys("creel_live.child")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	assertFK(t, fks, "parent_id", "parent", "id")

	idxs, err := d.Indexes("creel_live.child")
	if err != nil {
		t.Fatalf("Indexes(child): %v", err)
	}
	assertIndex(t, idxs, "child_parent_idx", false, []string{"parent_id"})

	pidxs, err := d.Indexes("creel_live.parent")
	if err != nil {
		t.Fatalf("Indexes(parent): %v", err)
	}
	assertIndex(t, pidxs, "parent_name_idx", true, []string{"name"})
	open, ok := findIndex(pidxs, "parent_open_idx")
	if !ok {
		t.Fatalf("missing partial index parent_open_idx in %v", pidxs)
	}
	if !strings.Contains(strings.ToLower(open.Partial), "status") {
		t.Fatalf("partial predicate = %q, want status filter", open.Partial)
	}

	checks, err := d.CheckConstraints("creel_live.parent")
	if err != nil {
		t.Fatalf("CheckConstraints: %v", err)
	}
	if len(checks) == 0 || !strings.Contains(strings.ToLower(checks[0].Expression), "status") {
		t.Fatalf("CheckConstraints = %+v, want status check", checks)
	}

	def, err := d.ViewDefinition("creel_live.active_parent")
	if err != nil {
		t.Fatalf("ViewDefinition: %v", err)
	}
	if !strings.Contains(strings.ToLower(def), "from") {
		t.Fatalf("ViewDefinition = %q", def)
	}

	trigs, err := d.Triggers("creel_live.parent")
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(trigs) != 1 || trigs[0].Name != "parent_trg" {
		t.Fatalf("Triggers = %+v, want parent_trg", trigs)
	}
	if trigs[0].Timing != "AFTER" || trigs[0].Event != "UPDATE" {
		t.Fatalf("trigger timing/event = %s %s", trigs[0].Timing, trigs[0].Event)
	}

	info, err := d.TableColumnInfo("creel_live.parent")
	if err != nil {
		t.Fatalf("TableColumnInfo: %v", err)
	}
	if len(info) == 0 || !info[0].PrimaryKey {
		t.Fatalf("TableColumnInfo[0] = %+v, want PK", firstCol(info))
	}

	// Switch into the schema: unqualified catalog + reverse refs + :uses.
	if err := d.UseSchema("creel_live"); err != nil {
		t.Fatalf("UseSchema: %v", err)
	}
	tables, err := d.Tables()
	if err != nil {
		t.Fatalf("Tables after UseSchema: %v", err)
	}
	if !hasString(tables, "parent") || !hasString(tables, "child") {
		t.Fatalf("Tables() after UseSchema = %v", tables)
	}
	views, err := d.Views()
	if err != nil {
		t.Fatalf("Views: %v", err)
	}
	if !hasString(views, "active_parent") {
		t.Fatalf("Views() = %v, want active_parent", views)
	}
	refs, err := d.ReferencingForeignKeys("parent")
	if err != nil {
		t.Fatalf("ReferencingForeignKeys: %v", err)
	}
	if len(refs) != 1 || refs[0].Table != "child" || refs[0].Column != "parent_id" {
		t.Fatalf("ReferencingForeignKeys = %+v", refs)
	}
	uses, err := d.Uses("parent")
	if err != nil {
		t.Fatalf("Uses: %v", err)
	}
	if !hasUsageKind(uses, "view") {
		t.Fatalf("Uses(parent) = %+v, want a view", uses)
	}

	assertLocksOK(t, d)
	assertSelfSession(t, d)
	assertExplain(t, d, DriverPostgres, `EXPLAIN SELECT * FROM child WHERE parent_id = 1`)
	if err := d.KillSession("0"); err == nil {
		t.Fatal("KillSession(0) should reject an invalid pid")
	}

	if _, err := d.TableSizes(); err != nil {
		t.Fatalf("TableSizes: %v", err)
	}
}

// TestLiveMySQLCatalog builds throwaway tables (and a second database) and
// exercises catalog, FK/index, foreign-schema, :locks/:who, and EXPLAIN.
func TestLiveMySQLCatalog(t *testing.T) {
	conn := liveConnect(t, liveMySQLConfig())
	d := conn.DB()
	home := conn.Config().Database

	mustExec(t, d, `DROP TABLE IF EXISTS creel_live_child`)
	mustExec(t, d, `DROP VIEW IF EXISTS creel_live_active_parent`)
	mustExec(t, d, `DROP TABLE IF EXISTS creel_live_parent`)
	mustExec(t, d, `DROP DATABASE IF EXISTS creel_live_other`)
	t.Cleanup(func() {
		if conn.Config().Database != home {
			_ = d.UseDatabase(home)
		}
		_, _ = d.Exec(`DROP TABLE IF EXISTS creel_live_child`)
		_, _ = d.Exec(`DROP TRIGGER IF EXISTS creel_live_parent_trg`)
		_, _ = d.Exec(`DROP VIEW IF EXISTS creel_live_active_parent`)
		_, _ = d.Exec(`DROP TABLE IF EXISTS creel_live_parent`)
		_, _ = d.Exec(`DROP DATABASE IF EXISTS creel_live_other`)
	})

	mustExec(t, d, `CREATE TABLE creel_live_parent (
		id INT PRIMARY KEY,
		name VARCHAR(64) NOT NULL,
		status VARCHAR(16) NOT NULL,
		CONSTRAINT creel_live_parent_status CHECK (status IN ('open', 'closed'))
	) ENGINE=InnoDB`)
	mustExec(t, d, `CREATE TABLE creel_live_child (
		id INT PRIMARY KEY,
		parent_id INT NOT NULL,
		note VARCHAR(64),
		CONSTRAINT creel_live_child_parent_fk
			FOREIGN KEY (parent_id) REFERENCES creel_live_parent(id)
	) ENGINE=InnoDB`)
	mustExec(t, d, `CREATE UNIQUE INDEX creel_live_parent_name_idx ON creel_live_parent(name)`)
	mustExec(t, d, `CREATE INDEX creel_live_child_note_idx ON creel_live_child(note)`)
	mustExec(t, d, `CREATE VIEW creel_live_active_parent AS
		SELECT id, name FROM creel_live_parent WHERE status = 'open'`)
	mustExec(t, d, `CREATE TRIGGER creel_live_parent_trg
		AFTER UPDATE ON creel_live_parent
		FOR EACH ROW SET @creel_live_flag = 1`)

	tables, err := d.Tables()
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if !hasString(tables, "creel_live_parent") || !hasString(tables, "creel_live_child") {
		t.Fatalf("Tables() = %v", tables)
	}

	cols, err := d.TableSchema("creel_live_parent")
	if err != nil {
		t.Fatalf("TableSchema: %v", err)
	}
	if !hasColumn(cols, "id") || !hasColumn(cols, "status") {
		t.Fatalf("TableSchema = %v", cols)
	}

	pks, err := d.PrimaryKeys("creel_live_parent")
	if err != nil {
		t.Fatalf("PrimaryKeys: %v", err)
	}
	if !equalStrings(pks, []string{"id"}) {
		t.Fatalf("PrimaryKeys = %v, want [id]", pks)
	}

	fks, err := d.ForeignKeys("creel_live_child")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	assertFK(t, fks, "parent_id", "creel_live_parent", "id")

	idxs, err := d.Indexes("creel_live_child")
	if err != nil {
		t.Fatalf("Indexes(child): %v", err)
	}
	assertIndex(t, idxs, "creel_live_child_note_idx", false, []string{"note"})

	pidxs, err := d.Indexes("creel_live_parent")
	if err != nil {
		t.Fatalf("Indexes(parent): %v", err)
	}
	assertIndex(t, pidxs, "creel_live_parent_name_idx", true, []string{"name"})

	checks, err := d.CheckConstraints("creel_live_parent")
	if err != nil {
		t.Fatalf("CheckConstraints: %v", err)
	}
	if len(checks) == 0 || !strings.Contains(strings.ToLower(checks[0].Expression), "status") {
		t.Fatalf("CheckConstraints = %+v, want status check", checks)
	}

	ddl, err := d.TableDefinition("creel_live_child")
	if err != nil {
		t.Fatalf("TableDefinition: %v", err)
	}
	if !strings.Contains(strings.ToUpper(ddl), "CREATE TABLE") {
		t.Fatalf("TableDefinition = %q", ddl)
	}

	def, err := d.ViewDefinition("creel_live_active_parent")
	if err != nil {
		t.Fatalf("ViewDefinition: %v", err)
	}
	if def == "" {
		t.Fatal("ViewDefinition empty")
	}

	trigs, err := d.Triggers("creel_live_parent")
	if err != nil {
		t.Fatalf("Triggers: %v", err)
	}
	if len(trigs) != 1 || trigs[0].Name != "creel_live_parent_trg" {
		t.Fatalf("Triggers = %+v", trigs)
	}

	refs, err := d.ReferencingForeignKeys("creel_live_parent")
	if err != nil {
		t.Fatalf("ReferencingForeignKeys: %v", err)
	}
	if len(refs) != 1 || refs[0].Table != "creel_live_child" {
		t.Fatalf("ReferencingForeignKeys = %+v", refs)
	}
	uses, err := d.Uses("creel_live_parent")
	if err != nil {
		t.Fatalf("Uses: %v", err)
	}
	if !hasUsageKind(uses, "view") && !hasUsageKind(uses, "trigger") {
		t.Fatalf("Uses = %+v, want view or trigger", uses)
	}

	// Foreign-schema (other database) without switching.
	mustExec(t, d, `CREATE DATABASE creel_live_other`)
	mustExec(t, d, `CREATE TABLE creel_live_other.t (
		id INT PRIMARY KEY,
		label VARCHAR(32) NOT NULL
	)`)
	otherTables, err := d.TablesInSchema("creel_live_other")
	if err != nil {
		t.Fatalf("TablesInSchema: %v", err)
	}
	if !hasString(otherTables, "t") {
		t.Fatalf("TablesInSchema(creel_live_other) = %v", otherTables)
	}
	ocols, err := d.TableSchemaInSchema("creel_live_other", "t")
	if err != nil {
		t.Fatalf("TableSchemaInSchema: %v", err)
	}
	if !hasColumn(ocols, "label") {
		t.Fatalf("TableSchemaInSchema = %v", ocols)
	}
	if err := d.UseDatabase("creel_live_other"); err != nil {
		t.Fatalf("UseDatabase: %v", err)
	}
	switched, err := d.Tables()
	if err != nil {
		t.Fatalf("Tables after UseDatabase: %v", err)
	}
	if !hasString(switched, "t") {
		t.Fatalf("Tables() after UseDatabase = %v", switched)
	}
	if err := d.UseDatabase(home); err != nil {
		t.Fatalf("UseDatabase(%s): %v", home, err)
	}

	assertLocksOK(t, d)
	assertSelfSession(t, d)
	assertExplain(t, d, DriverMySQL, `EXPLAIN SELECT * FROM creel_live_child WHERE parent_id = 1`)
	if err := d.KillSession("0"); err == nil {
		t.Fatal("KillSession(0) should reject an invalid pid")
	}

	if _, err := d.TableSizes(); err != nil {
		t.Fatalf("TableSizes: %v", err)
	}
}

func hasColumn(cols []Column, name string) bool {
	for _, c := range cols {
		if c.Name == name {
			return true
		}
	}
	return false
}

func firstCol(info []TableColumnInfo) TableColumnInfo {
	if len(info) == 0 {
		return TableColumnInfo{}
	}
	return info[0]
}

func findIndex(idxs []Index, name string) (Index, bool) {
	for _, ix := range idxs {
		if ix.Name == name {
			return ix, true
		}
	}
	return Index{}, false
}

func assertIndex(t *testing.T, idxs []Index, name string, unique bool, cols []string) {
	t.Helper()
	ix, ok := findIndex(idxs, name)
	if !ok {
		t.Fatalf("missing index %q in %v", name, idxs)
	}
	if ix.Unique != unique {
		t.Fatalf("index %q unique = %v, want %v", name, ix.Unique, unique)
	}
	if !equalStrings(ix.Columns, cols) {
		t.Fatalf("index %q columns = %v, want %v", name, ix.Columns, cols)
	}
}

func assertFK(t *testing.T, fks []ForeignKey, col, refTable, refCol string) {
	t.Helper()
	for _, fk := range fks {
		if fk.Column == col && fk.RefTable == refTable && fk.RefColumn == refCol {
			return
		}
	}
	t.Fatalf("missing FK %s → %s.%s in %+v", col, refTable, refCol, fks)
}

func hasUsageKind(uses []Usage, kind string) bool {
	for _, u := range uses {
		if strings.EqualFold(u.Kind, kind) {
			return true
		}
	}
	return false
}

func assertLocksOK(t *testing.T, d DB) {
	t.Helper()
	locks, err := d.Locks()
	if err != nil {
		t.Fatalf("Locks: %v", err)
	}
	_ = locks // empty is the expected idle-server case
}

func assertSelfSession(t *testing.T, d DB) {
	t.Helper()
	sessions, err := d.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("Sessions() empty")
	}
	for _, s := range sessions {
		if s.Self {
			return
		}
	}
	t.Fatalf("Sessions() missing Self entry: %+v", sessions)
}

func assertExplain(t *testing.T, d DB, driver Driver, query string) {
	t.Helper()
	plan, err := d.Execute(query)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	if len(plan.Rows) == 0 {
		t.Fatal("EXPLAIN returned no rows")
	}
	findings := DiagnoseExplain(driver, plan, d.Indexes)
	if len(findings) == 0 {
		t.Fatal("DiagnoseExplain returned nothing")
	}
}
