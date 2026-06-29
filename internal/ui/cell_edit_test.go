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

func TestIsCellTruncated(t *testing.T) {
	r := NewResultsTable()
	r.SetSize(80, 20)
	r.SetResult(
		[]string{"id", "note"},
		[][]string{
			{"1", "short"},
			{"2", strings.Repeat("x", 50)}, // exceeds maxCellWidth (40)
		},
		"2 rows",
	)

	if r.IsCellTruncated(0, 1) {
		t.Error("short value should not be truncated")
	}
	if !r.IsCellTruncated(1, 1) {
		t.Error("50-char value should be truncated at default width")
	}
	if r.IsCellTruncated(1, 0) {
		t.Error("short id should not be truncated")
	}
	// Out of range is safe.
	if r.IsCellTruncated(5, 5) {
		t.Error("out-of-range cell should not report truncated")
	}
}

func TestCellEditPopupLifecycle(t *testing.T) {
	p := NewCellEditPopup()
	if p.IsVisible() {
		t.Fatal("popup should start hidden")
	}

	p.Show("hello world", 2, 3, "body")
	if !p.IsVisible() {
		t.Fatal("popup should be visible after Show")
	}
	if p.Value() != "hello world" {
		t.Errorf("value = %q, want %q", p.Value(), "hello world")
	}
	if p.Row() != 2 || p.Col() != 3 {
		t.Errorf("row/col = (%d,%d), want (2,3)", p.Row(), p.Col())
	}

	p.Hide()
	if p.IsVisible() {
		t.Fatal("popup should be hidden after Hide")
	}
}

// TestCellEditPopupRouting verifies that pressing e on a truncated cell opens
// the expanded popup (rather than the inline editor), that E force-opens it on
// any cell, and that ctrl+s stages the value into the dirtyCells pipeline.
func TestCellEditPopupRouting(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := db.ConnectionConfig{Driver: db.DriverSQLite, Database: dbPath}
	sqlite, err := db.New(cfg)
	if err != nil {
		t.Fatalf("new sqlite: %v", err)
	}
	if err := sqlite.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		sqlite.Close()
		os.Remove(dbPath)
	})

	longVal := strings.Repeat("z", 60)
	if _, err := sqlite.DB().Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := sqlite.DB().Exec(`INSERT INTO notes (id, body) VALUES (1, ?)`, longVal); err != nil {
		t.Fatalf("insert: %v", err)
	}

	m := NewModel(&config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.connection = sqlite
	m.state = stateWorkspace
	m.focus = FocusResults
	m.lastQuery = "SELECT * FROM notes LIMIT 100"
	m.baseQuery = m.lastQuery
	m.results = NewResultsTable()
	m.results.SetSize(100, 20)
	m.results.SetResult(
		[]string{"id", "body"},
		[][]string{{"1", longVal}},
		"1 row",
	)
	m.results.SetEditable("notes", []string{"id"})

	// Cursor on the truncated body cell (col 1).
	m.results.SetCursor(0, 1)

	// 'e' on a truncated cell should open the popup, not the inline editor.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = updated.(Model)
	if !m.cellEdit.IsVisible() {
		t.Fatal("popup should be open after 'e' on truncated cell")
	}
	if m.results.IsEditing() {
		t.Fatal("inline editor should NOT be active when popup opens")
	}

	// Edit the value in the popup and stage it with ctrl+s.
	m.cellEdit.ta.SetValue("edited long body")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)
	if m.cellEdit.IsVisible() {
		t.Fatal("popup should close after ctrl+s")
	}
	// The value should be staged as a dirty cell.
	got := m.results.RowValue(0, 1)
	if got != "edited long body" {
		t.Errorf("staged value = %q, want %q", got, "edited long body")
	}
	if !m.results.HasDirtyCells() {
		t.Error("dirty cells should be present after staging")
	}

	// 'E' force-opens the popup even on a non-truncated cell.
	m.results.SetCursor(0, 0) // id column (short)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	m = updated.(Model)
	if !m.cellEdit.IsVisible() {
		t.Fatal("popup should open after 'E' on short cell")
	}

	// esc cancels and closes.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.cellEdit.IsVisible() {
		t.Fatal("popup should close after esc")
	}
}
