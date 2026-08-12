package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIsBinaryType(t *testing.T) {
	yes := []string{"BLOB", "blob", "BYTEA", "VARBINARY", "VARBINARY(16)", "BINARY(32)", "TINYBLOB", "MEDIUMBLOB", "LONGBLOB", "IMAGE"}
	for _, typ := range yes {
		if !IsBinaryType(typ) {
			t.Errorf("IsBinaryType(%q) = false, want true", typ)
		}
	}
	no := []string{"TEXT", "VARCHAR(255)", "INTEGER", "INT", "REAL", "JSON", "JSONB", "BIT", ""}
	for _, typ := range no {
		if IsBinaryType(typ) {
			t.Errorf("IsBinaryType(%q) = true, want false", typ)
		}
	}
}

func TestBlobPlaceholder(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "<BLOB 0B>"},
		{512, "<BLOB 512B>"},
		{1024, "<BLOB 1.0KB>"},
		{1234, "<BLOB 1.2KB>"},
		{1024 * 1024, "<BLOB 1.0MB>"},
		{3 * 1024 * 1024, "<BLOB 3.0MB>"},
	}
	for _, c := range cases {
		if got := BlobPlaceholder(c.n); got != c.want {
			t.Errorf("BlobPlaceholder(%d) = %q, want %q", c.n, got, c.want)
		}
	}
	if !IsBlobPlaceholder("<BLOB 1.2KB>") {
		t.Error("IsBlobPlaceholder should accept a real placeholder")
	}
	if IsBlobPlaceholder("NULL") || IsBlobPlaceholder("<BLOB") || IsBlobPlaceholder("hello") {
		t.Error("IsBlobPlaceholder should reject non-placeholders")
	}
}

func TestBlobSQLLiteral(t *testing.T) {
	data := []byte{0xde, 0xad}
	if got := BlobSQLLiteral(data, "BLOB"); got != "X'dead'" {
		t.Errorf("SQLite/MySQL literal = %q, want X'dead'", got)
	}
	if got := BlobSQLLiteral(data, "BYTEA"); got != `'\xdead'` {
		t.Errorf("Postgres literal = %q, want '\\xdead'", got)
	}
	if got := BlobSQLLiteral(nil, "BLOB"); got != "X''" {
		t.Errorf("empty blob = %q, want X''", got)
	}
}

func TestTrimBlobs(t *testing.T) {
	blobs := map[BlobKey][]byte{
		{0, 1}: {1},
		{1, 1}: {2},
		{2, 1}: {3},
	}
	got := TrimBlobs(blobs, 2)
	if len(got) != 2 || got[BlobKey{2, 1}] != nil {
		t.Errorf("TrimBlobs = %v, want rows 0 and 1 only", got)
	}
	if TrimBlobs(nil, 10) != nil {
		t.Error("TrimBlobs(nil) should return nil")
	}
}

func TestExecuteRows_Blob(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "blob.db")
	s := NewSQLite(ConnectionConfig{Driver: DriverSQLite, Database: dbPath})
	if err := s.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if _, err := s.Exec(`CREATE TABLE files (id INTEGER PRIMARY KEY, name TEXT, data BLOB)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	payload := []byte{0x00, 0x01, 0xff, 0xfe, 'A'}
	if _, err := s.Exec(`INSERT INTO files (id, name, data) VALUES (1, 'bin', ?), (2, 'empty', ?), (3, 'nil', NULL)`,
		payload, []byte{}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := s.Execute(`SELECT id, name, data FROM files ORDER BY id`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(result.Rows))
	}

	// Text columns still scan as text.
	if result.Rows[0][1] != "bin" {
		t.Errorf("name = %q, want bin", result.Rows[0][1])
	}

	// Binary cell → placeholder + raw bytes in Blobs.
	wantPH := BlobPlaceholder(len(payload))
	if result.Rows[0][2] != wantPH {
		t.Errorf("blob cell = %q, want %q", result.Rows[0][2], wantPH)
	}
	got, ok := result.Blobs[BlobKey{Row: 0, Col: 2}]
	if !ok {
		t.Fatal("missing Blobs entry for row 0")
	}
	if string(got) != string(payload) {
		t.Errorf("blob bytes = %v, want %v", got, payload)
	}

	// Empty non-NULL blob.
	if result.Rows[1][2] != BlobPlaceholder(0) {
		t.Errorf("empty blob = %q, want %q", result.Rows[1][2], BlobPlaceholder(0))
	}
	empty, ok := result.Blobs[BlobKey{Row: 1, Col: 2}]
	if !ok || empty == nil {
		t.Errorf("empty blob missing from Blobs: ok=%v data=%v", ok, empty)
	} else if len(empty) != 0 {
		t.Errorf("empty blob len = %d, want 0", len(empty))
	}

	// NULL blob stays the NULL sentinel and is absent from Blobs.
	if result.Rows[2][2] != "NULL" {
		t.Errorf("NULL blob = %q, want NULL", result.Rows[2][2])
	}
	if _, ok := result.Blobs[BlobKey{Row: 2, Col: 2}]; ok {
		t.Error("NULL blob should not appear in Blobs")
	}
}

func TestDumpTable_BlobLiteral(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "dump-blob.db")
	s := NewSQLite(ConnectionConfig{Driver: DriverSQLite, Database: dbPath})
	if err := s.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if _, err := s.Exec(`CREATE TABLE files (id INTEGER PRIMARY KEY, data BLOB)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Exec(`INSERT INTO files (id, data) VALUES (1, ?)`, []byte{0xde, 0xad}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var buf strings.Builder
	if err := DumpTable(&buf, s, DriverSQLite, "files"); err != nil {
		t.Fatalf("DumpTable: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "X'dead'") {
		t.Errorf("dump missing blob literal:\n%s", out)
	}
	if strings.Contains(out, "<BLOB") {
		t.Errorf("dump should not embed placeholder:\n%s", out)
	}
}
