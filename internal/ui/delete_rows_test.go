package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruben/gsql/internal/config"
	"github.com/ruben/gsql/internal/db"
)

func setupDeleteTestDB(t *testing.T) *db.Connection {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath}
	conn, err := db.New(cfg)
	if err != nil {
		t.Fatalf("new sqlite: %v", err)
	}
	if err := conn.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(); os.Remove(dbPath) })

	if _, err := conn.DB().Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := conn.DB().Exec(`INSERT INTO users (id, name) VALUES (1, 'alice'), (2, 'bob'), (3, 'carol')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return conn
}

func newDeleteRowsModel(conn *db.Connection) Model {
	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = conn
	m.state = stateWorkspace
	m.focus = FocusResults
	m.results.SetSize(80, 20)
	m.results.SetResult(
		[]string{"id", "name"},
		[][]string{{"1", "alice"}, {"2", "bob"}, {"3", "carol"}},
		"3 rows",
	)
	m.results.SetEditable("users", []string{"id"})
	m.lastQuery = "SELECT * FROM users LIMIT 200"
	m.baseQuery = m.lastQuery
	return m
}

func TestDeleteCursorRowFlow(t *testing.T) {
	conn := setupDeleteTestDB(t)
	m := newDeleteRowsModel(conn)

	// dd opens confirmation for cursor row (row 0, id=1).
	m = press(m, keyRunes('d'))
	if m.resultsPendingD != true {
		t.Fatal("expected resultsPendingD after first d")
	}
	m = press(m, keyRunes('d'))
	if m.deleteRowsConfirmTable != "users" {
		t.Fatalf("expected confirm table=users, got %q", m.deleteRowsConfirmTable)
	}
	if m.deleteRowsConfirmCount != 1 {
		t.Fatalf("expected count=1, got %d", m.deleteRowsConfirmCount)
	}

	// Confirm with y.
	updated, cmd := m.Update(keyRunes('y'))
	m = updated.(Model)
	if m.deleteRowsConfirmTable != "" {
		t.Fatal("confirm should be cleared after y")
	}
	if cmd == nil {
		t.Fatal("expected async delete command")
	}

	msg := cmd().(deleteRowsResultMsg)
	if msg.err != nil {
		t.Fatalf("delete failed: %v", msg.err)
	}
	if msg.count != 1 {
		t.Fatalf("expected count=1, got %d", msg.count)
	}

	// Feed result back to set status message.
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if !strings.Contains(m.deleteRowsMsg, "deleted 1 row") {
		t.Fatalf("deleteRowsMsg = %q", m.deleteRowsMsg)
	}

	// Verify row 1 is actually gone from the database.
	res, err := conn.DB().Execute(`SELECT COUNT(*) FROM users WHERE id = 1`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Rows[0][0] != "0" {
		t.Errorf("expected id=1 deleted, got count %s", res.Rows[0][0])
	}
}

func TestDeleteMarkedRowsFlow(t *testing.T) {
	conn := setupDeleteTestDB(t)
	m := newDeleteRowsModel(conn)

	// Mark rows 0 and 1.
	m = press(m, keyRunes(' '))
	m = press(m, keyRunes('j'))
	m = press(m, keyRunes(' '))
	if m.results.MarkCount() != 2 {
		t.Fatalf("expected 2 marks, got %d", m.results.MarkCount())
	}

	// dd should target the 2 marked rows.
	m = press(m, keyRunes('d'))
	m = press(m, keyRunes('d'))
	if m.deleteRowsConfirmCount != 2 {
		t.Fatalf("expected count=2, got %d", m.deleteRowsConfirmCount)
	}

	// Confirm.
	updated, cmd := m.Update(keyRunes('y'))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected async delete command")
	}

	msg := cmd().(deleteRowsResultMsg)
	if msg.count != 2 {
		t.Fatalf("expected count=2, got %d", msg.count)
	}

	// Feed result back to clear marks and set status.
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.results.MarkCount() != 0 {
		t.Errorf("marks should be cleared after delete, got %d", m.results.MarkCount())
	}

	res, err := conn.DB().Execute(`SELECT COUNT(*) FROM users`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Rows[0][0] != "1" {
		t.Errorf("expected 1 remaining row, got %s", res.Rows[0][0])
	}
}

func TestDeleteRowsCancel(t *testing.T) {
	conn := setupDeleteTestDB(t)
	m := newDeleteRowsModel(conn)

	// dd then n cancels.
	m = press(m, keyRunes('d'))
	m = press(m, keyRunes('d'))
	if m.deleteRowsConfirmTable == "" {
		t.Fatal("expected confirm dialog")
	}
	m = press(m, keyRunes('n'))
	if m.deleteRowsConfirmTable != "" {
		t.Fatal("confirm should be cleared after n")
	}

	// Verify no rows were deleted.
	res, err := conn.DB().Execute(`SELECT COUNT(*) FROM users`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Rows[0][0] != "3" {
		t.Errorf("expected 3 rows after cancel, got %s", res.Rows[0][0])
	}
}

func TestDeleteRowsNotEditable(t *testing.T) {
	m := newResultsWorkspaceModel()
	m.results.ClearEditable()

	// dd on non-editable results should not open confirm.
	m = press(m, keyRunes('d'))
	m = press(m, keyRunes('d'))
	if m.deleteRowsConfirmTable != "" {
		t.Fatal("should not open delete confirm on non-editable results")
	}
}

func TestDeletePendingDClearedOnOtherKey(t *testing.T) {
	m := newResultsWorkspaceModel()

	// First d sets pending.
	m = press(m, keyRunes('d'))
	if !m.resultsPendingD {
		t.Fatal("expected resultsPendingD after first d")
	}
	// Pressing j should clear it.
	m = press(m, keyRunes('j'))
	if m.resultsPendingD {
		t.Fatal("expected resultsPendingD cleared after j")
	}
	// Pressing d now starts fresh (first d again).
	m = press(m, keyRunes('d'))
	if !m.resultsPendingD {
		t.Fatal("expected resultsPendingD set again")
	}
	if m.deleteRowsConfirmTable != "" {
		t.Fatal("should not open confirm — this is only the first d")
	}
}

func TestBuildDeleteQuery(t *testing.T) {
	tests := []struct {
		name    string
		table   string
		pkNames []string
		pkTypes []string
		tuples  [][]string
		want    string
	}{
		{
			name:    "single PK integer",
			table:   "users",
			pkNames: []string{"id"},
			pkTypes: []string{"INTEGER"},
			tuples:  [][]string{{"1"}, {"3"}, {"5"}},
			want:    "DELETE FROM users WHERE id IN (1, 3, 5)",
		},
		{
			name:    "single PK single row",
			table:   "users",
			pkNames: []string{"id"},
			pkTypes: []string{"INTEGER"},
			tuples:  [][]string{{"42"}},
			want:    "DELETE FROM users WHERE id IN (42)",
		},
		{
			name:    "single PK text",
			table:   "countries",
			pkNames: []string{"code"},
			pkTypes: []string{"VARCHAR"},
			tuples:  [][]string{{"US"}, {"UK"}},
			want:    "DELETE FROM countries WHERE code IN ('US', 'UK')",
		},
		{
			name:    "composite PK",
			table:   "orders",
			pkNames: []string{"user_id", "product_id"},
			pkTypes: []string{"INTEGER", "INTEGER"},
			tuples:  [][]string{{"1", "2"}, {"3", "4"}},
			want:    "DELETE FROM orders WHERE (user_id, product_id) IN ((1, 2), (3, 4))",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildDeleteQuery(tc.table, tc.pkNames, tc.pkTypes, tc.tuples)
			if got != tc.want {
				t.Errorf("buildDeleteQuery() = %q, want %q", got, tc.want)
			}
		})
	}
}
