package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsiota/creel/internal/db"
)

func TestResultsBlobCell(t *testing.T) {
	r := ResultsTable{}
	r.SetResult(
		[]string{"id", "data"},
		[][]string{{"1", db.BlobPlaceholder(5)}, {"2", "NULL"}},
		"2 rows",
	)
	r.SetBlobs(map[db.BlobKey][]byte{
		{Row: 0, Col: 1}: {0x00, 0x01, 0xff, 0xfe, 'A'},
	})

	if !r.IsBlobCell(0, 1) {
		t.Error("row 0 col 1 should be a blob cell")
	}
	if r.IsBlobCell(1, 1) {
		t.Error("NULL cell should not be a blob cell")
	}
	data, ok := r.BlobData(0, 1)
	if !ok || len(data) != 5 {
		t.Fatalf("BlobData = %v ok=%v", data, ok)
	}

	// StartEdit is a no-op on blob cells.
	r.editable = true
	r.pkColumns = []string{"id"}
	r.cursorRow, r.cursorCol = 0, 1
	r.StartEdit()
	if r.editing {
		t.Error("StartEdit should refuse blob cells")
	}

	// SetResult clears blobs.
	r.SetResult([]string{"id"}, [][]string{{"1"}}, "")
	if r.IsBlobCell(0, 0) || r.blobs != nil {
		t.Error("SetResult should clear blobs")
	}
}

func TestCopyAsInsertBlobLiteral(t *testing.T) {
	r := ResultsTable{}
	r.SetResult(
		[]string{"id", "data"},
		[][]string{{"1", db.BlobPlaceholder(2)}},
		"",
	)
	r.sourceTable = "files"
	r.SetColumnTypes(map[string]string{"id": "INTEGER", "data": "BLOB"})
	r.SetBlobs(map[db.BlobKey][]byte{
		{Row: 0, Col: 1}: {0xde, 0xad},
	})

	sql, count := r.CopyAsInsert()
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	if !strings.Contains(sql, "X'dead'") {
		t.Errorf("INSERT missing blob literal:\n%s", sql)
	}
	if strings.Contains(sql, "<BLOB") {
		t.Errorf("INSERT should not embed the placeholder:\n%s", sql)
	}
}

func TestExSaveBlob(t *testing.T) {
	m := &Model{}
	m.results.SetResult(
		[]string{"id", "data"},
		[][]string{{"1", db.BlobPlaceholder(4)}},
		"",
	)
	payload := []byte{1, 2, 3, 4}
	m.results.SetBlobs(map[db.BlobKey][]byte{
		{Row: 0, Col: 1}: payload,
	})
	m.results.cursorRow, m.results.cursorCol = 0, 1

	dir := t.TempDir()
	out := filepath.Join(dir, "out.bin")
	m.exSaveBlob(out)
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("file = %v, want %v", got, payload)
	}
	if !strings.Contains(m.schemaMsg, "wrote") {
		t.Errorf("schemaMsg = %q", m.schemaMsg)
	}

	// Non-blob cell.
	m.results.cursorCol = 0
	m.exSaveBlob(filepath.Join(dir, "nope.bin"))
	if m.schemaMsg != "cursor cell is not a binary value" {
		t.Errorf("non-blob -> %q", m.schemaMsg)
	}
}

func TestBuildInsertQueryBlob(t *testing.T) {
	columns := []db.TableColumnInfo{
		{Name: "id", Type: "INTEGER", PrimaryKey: true, AutoIncrement: true},
		{Name: "data", Type: "BLOB", NotNull: true},
	}
	query, args, err := buildInsertQuery(db.DriverSQLite, "files", columns,
		map[string]string{"data": db.BlobPlaceholder(2)},
		map[string][]byte{"data": {0xde, 0xad}},
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if query != "INSERT INTO files (data) VALUES (?)" {
		t.Errorf("query = %q", query)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d", len(args))
	}
	b, ok := args[0].([]byte)
	if !ok || string(b) != string([]byte{0xde, 0xad}) {
		t.Errorf("args[0] = %#v, want []byte{de ad}", args[0])
	}
}
