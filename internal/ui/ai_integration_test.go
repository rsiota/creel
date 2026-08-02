package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruben/creel/internal/ai"
	"github.com/ruben/creel/internal/db"
)

// TestSchemaContext_RealSQLite verifies the real db.DB → ai.SchemaIntrospector
// path end to end: it opens an in-memory-ish SQLite database, creates a table
// with a primary key and a foreign key, and checks that SchemaContext (via
// dbAdapter) renders the names, types, and relationship annotations. This
// exercises the same wiring :ai uses at runtime, not a fake.
func TestSchemaContext_RealSQLite(t *testing.T) {
	s := db.NewSQLite(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "ai.db")})
	if err := s.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if _, err := s.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, created_at TEXT)`); err != nil {
		t.Fatalf("create users: %v", err)
	}
	if _, err := s.Exec(`CREATE TABLE orders (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		total REAL,
		FOREIGN KEY (user_id) REFERENCES users(id))`); err != nil {
		t.Fatalf("create orders: %v", err)
	}

	got, err := ai.SchemaContext(dbAdapter{s})
	if err != nil {
		t.Fatalf("SchemaContext: %v", err)
	}

	for _, want := range []string{
		"CREATE TABLE users",
		"CREATE TABLE orders",
		"email TEXT",
		"created_at TEXT",
		"user_id INTEGER",
		"FK -> users.id",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context missing %q\nfull:\n%s", want, got)
		}
	}
}

// TestCachedAIIntrospector_ServedFromMemory proves the cache-backed
// introspector serves the exact same schema context as the live dbAdapter —
// without touching the connection. We populate the caches from one database
// and hand the introspector a DIFFERENT (empty) database as its live fallback;
// if any lookup fell through to the live DB the context would be empty, so
// matching the real DB's output proves every lookup hit the cache.
func TestCachedAIIntrospector_ServedFromMemory(t *testing.T) {
	real := db.NewSQLite(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "real.db")})
	if err := real.Connect(); err != nil {
		t.Fatalf("connect real: %v", err)
	}
	t.Cleanup(func() { _ = real.Close() })
	for _, ddl := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, total REAL, FOREIGN KEY (user_id) REFERENCES users(id))`,
	} {
		if _, err := real.Exec(ddl); err != nil {
			t.Fatalf("exec %q: %v", ddl, err)
		}
	}

	// Populate the caches the way prefetchSchemas does (columns + PKs + FKs).
	tables, _ := real.Tables()
	columns := map[string][]db.Column{}
	pks := map[string][]string{}
	fks := map[string][]db.ForeignKey{}
	for _, t := range tables {
		columns[t], _ = real.TableSchema(t)
		pks[t], _ = real.PrimaryKeys(t)
		fks[t], _ = real.ForeignKeys(t)
	}

	// The live fallback is a fresh, EMPTY database — a cache miss would show.
	live := db.NewSQLite(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "empty.db")})
	if err := live.Connect(); err != nil {
		t.Fatalf("connect live: %v", err)
	}
	t.Cleanup(func() { _ = live.Close() })

	cached := cachedAIIntrospector{
		tables:  tables,
		columns: columns,
		pks:     pks,
		fks:     fks,
		live:    dbAdapter{live},
	}

	want, err := ai.SchemaContext(dbAdapter{real})
	if err != nil {
		t.Fatalf("reference SchemaContext: %v", err)
	}
	got, err := ai.SchemaContext(cached)
	if err != nil {
		t.Fatalf("cached SchemaContext: %v", err)
	}
	if got != want {
		t.Errorf("cached context differs from live reference\nwant:\n%s\ngot:\n%s", want, got)
	}
}

// TestCachedAIIntrospector_ColdFallback proves a cold cache (no entries)
// transparently falls through to the live connection, so the model still sees
// a complete schema before the background prefetch has populated anything.
func TestCachedAIIntrospector_ColdFallback(t *testing.T) {
	real := db.NewSQLite(db.ConnectionConfig{Driver: db.DriverSQLite, Database: filepath.Join(t.TempDir(), "real.db")})
	if err := real.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = real.Close() })
	if _, err := real.Exec(`CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	cold := cachedAIIntrospector{live: dbAdapter{real}} // nil maps / empty tables
	got, err := ai.SchemaContext(cold)
	if err != nil {
		t.Fatalf("SchemaContext: %v", err)
	}
	if !strings.Contains(got, "CREATE TABLE widgets") || !strings.Contains(got, "name TEXT") {
		t.Errorf("cold cache did not fall back to the live DB: %q", got)
	}
}
