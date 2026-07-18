package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruben/gsql/internal/ai"
	"github.com/ruben/gsql/internal/db"
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
